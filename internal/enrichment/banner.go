// Package enrichment provides post-scan host enrichment: banner grabbing, TLS
// certificate extraction, DNS resolution, and SNMP probing.
package enrichment

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
)

const (
	bannerReadBytes   = 1024
	bannerDialTimeout = 5 * time.Second
	bannerReadTimeout = 3 * time.Second
	bannerConcurrency = 10
)

// tlsPorts is the set of port numbers that should receive a TLS handshake
// rather than a plain TCP banner read.
var tlsPorts = map[int]bool{
	443: true, 8443: true, 465: true, 636: true,
	993: true, 995: true, 5986: true, 8883: true,
}

// BannerTarget describes a host and its open TCP ports to probe.
type BannerTarget struct {
	HostID uuid.UUID
	IP     string
	Ports  []int
}

// BannerGrabber grabs TCP/TLS banners from open ports and stores the results.
type BannerGrabber struct {
	repo        *db.BannerRepository
	logger      *slog.Logger
	fingerprint *Fingerprinter
}

// NewBannerGrabber creates a new BannerGrabber.
// extraFingerprintPath is an optional path to a JSON file with custom fingerprint
// rules; pass an empty string to use only the built-in rules.
func NewBannerGrabber(repo *db.BannerRepository, logger *slog.Logger, extraFingerprintPath string) *BannerGrabber {
	return &BannerGrabber{
		repo:        repo,
		logger:      logger,
		fingerprint: NewFingerprinter(extraFingerprintPath),
	}
}

// EnrichHosts grabs banners for all targets concurrently.
// Errors are logged rather than returned — enrichment is best-effort.
func (g *BannerGrabber) EnrichHosts(ctx context.Context, targets []BannerTarget) {
	if len(targets) == 0 {
		return
	}

	sem := make(chan struct{}, bannerConcurrency)
	var wg sync.WaitGroup

	for _, t := range targets {
		for _, port := range t.Ports {
			wg.Add(1)
			sem <- struct{}{}
			go func(target BannerTarget, p int) {
				defer wg.Done()
				defer func() { <-sem }()
				g.grabOne(ctx, target, p)
			}(t, port)
		}
	}

	wg.Wait()
}

func (g *BannerGrabber) grabOne(ctx context.Context, t BannerTarget, port int) {
	addr := fmt.Sprintf("%s:%d", t.IP, port)

	if tlsPorts[port] {
		g.grabTLS(ctx, t, port, addr)
	} else {
		g.grabPlain(ctx, t, port, addr)
	}
}

// grabPlain reads the initial banner bytes from a plain TCP connection.
func (g *BannerGrabber) grabPlain(ctx context.Context, t BannerTarget, port int, addr string) {
	dialer := net.Dialer{Timeout: bannerDialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return // host/port unreachable — not an error worth logging
	}
	defer conn.Close() //nolint:errcheck

	if err := conn.SetDeadline(time.Now().Add(bannerReadTimeout)); err != nil {
		return
	}

	buf := make([]byte, bannerReadBytes)
	n, _ := conn.Read(buf) // partial reads are fine

	var rawBanner *string
	if n > 0 {
		s := strings.TrimSpace(string(buf[:n]))
		rawBanner = &s
	}

	banner := &db.PortBanner{
		HostID:    t.HostID,
		Port:      port,
		Protocol:  db.ProtocolTCP,
		RawBanner: rawBanner,
	}

	// Always attempt fingerprint matching: port-only rules (Pattern="") fire even
	// when no banner was received (binary protocols that wait for the client to speak).
	bannerText := ""
	if rawBanner != nil {
		bannerText = *rawBanner
	}
	svc, ver := g.fingerprint.Match(port, bannerText)
	if svc == "" && rawBanner != nil {
		svc, ver = parseServiceFromBanner(*rawBanner, port)
	}
	if svc != "" {
		banner.Service = &svc
	}
	if ver != "" {
		banner.Version = &ver
	}

	if err := g.repo.UpsertPortBanner(ctx, banner); err != nil {
		g.logger.Warn("failed to store port banner",
			"host", t.IP, "port", port, "error", err)
	}
}

// grabTLS performs a TLS handshake, extracts certificate info, and stores
// both a banner (server name from cert) and a certificate record.
func (g *BannerGrabber) grabTLS(ctx context.Context, t BannerTarget, port int, addr string) {
	dialer := net.Dialer{Timeout: bannerDialTimeout}
	rawConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return
	}
	defer rawConn.Close() //nolint:errcheck

	// Scanner intentionally accepts any TLS version and self-signed certs so
	// it can inspect endpoints that a strict client would reject.
	tlsConn := tls.Client(rawConn, &tls.Config{ // nosemgrep
		MinVersion:         tls.VersionTLS10, // nosemgrep
		InsecureSkipVerify: true,             //nolint:gosec // nosemgrep
		ServerName:         t.IP,
	})
	defer tlsConn.Close() //nolint:errcheck
	if err := tlsConn.SetDeadline(time.Now().Add(bannerDialTimeout)); err != nil {
		return
	}
	if err := tlsConn.Handshake(); err != nil {
		// TLS failed — fall back to plain grab on this port.
		g.grabPlain(ctx, t, port, addr)
		return
	}

	cs := tlsConn.ConnectionState()
	if len(cs.PeerCertificates) == 0 {
		return
	}

	leaf := cs.PeerCertificates[0]

	// Build certificate record.
	cert := &db.Certificate{
		HostID: t.HostID,
		Port:   port,
	}

	if leaf.Subject.CommonName != "" {
		cn := leaf.Subject.CommonName
		cert.SubjectCN = &cn
	}
	cert.SANs = leaf.DNSNames

	if leaf.Issuer.CommonName != "" {
		issuer := leaf.Issuer.CommonName
		cert.Issuer = &issuer
	}

	cert.NotBefore = &leaf.NotBefore
	cert.NotAfter = &leaf.NotAfter

	keyType := keyTypeFromCert(leaf)
	if keyType != "" {
		cert.KeyType = &keyType
	}

	tlsVer := tlsVersionString(cs.Version)
	cert.TLSVersion = &tlsVer

	summary := fmt.Sprintf("TLS %s CN=%s", tlsVer, leaf.Subject.CommonName)
	cert.RawBanner = &summary

	if err := g.repo.UpsertCertificate(ctx, cert); err != nil {
		g.logger.Warn("failed to store certificate",
			"host", t.IP, "port", port, "error", err)
	}

	// Also store a banner entry for this port.
	svc := "https"
	banner := &db.PortBanner{
		HostID:    t.HostID,
		Port:      port,
		Protocol:  db.ProtocolTCP,
		Service:   &svc,
		RawBanner: &summary,
	}
	if err := g.repo.UpsertPortBanner(ctx, banner); err != nil {
		g.logger.Warn("failed to store TLS banner",
			"host", t.IP, "port", port, "error", err)
	}
}

// portServiceHints maps well-known port numbers to their service names for
// use when banner text doesn't provide enough signal.
var portServiceHints = map[int]string{
	21: "ftp", 22: "ssh",
	25: "smtp", 110: "pop3", 143: "imap",
	465: "smtp", 587: "smtp",
	993: "imap", 995: "pop3",
}

// parseServiceFromBanner applies simple heuristics to identify the service and
// version from a raw TCP banner string.
func parseServiceFromBanner(banner string, port int) (service, version string) {
	b := strings.ToLower(banner)
	return parseBannerText(b, banner, port)
}

// parseBannerText contains the actual parsing logic, split out to keep cyclomatic
// complexity below the project limit.
func parseBannerText(lower, raw string, port int) (service, version string) {
	switch {
	case strings.HasPrefix(lower, "ssh-"):
		parts := strings.SplitN(raw, " ", 2)
		service = "ssh"
		if len(parts) >= 1 {
			version = strings.TrimPrefix(parts[0], "SSH-2.0-")
		}
	case strings.HasPrefix(lower, "220 ") && strings.Contains(lower, "ftp"):
		service, version = "ftp", raw
	case strings.HasPrefix(lower, "220 ") &&
		(strings.Contains(lower, "smtp") || strings.Contains(lower, "mail")):
		service = "smtp"
	case strings.HasPrefix(lower, "+ok"):
		service = "pop3"
	case strings.HasPrefix(lower, "* ok"):
		service = "imap"
	case strings.HasPrefix(lower, "http/") || strings.HasPrefix(lower, "http "):
		service = "http"
	default:
		service = portServiceHints[port]
	}
	return service, version
}

// keyTypeFromCert returns a short string describing the public key algorithm.
func keyTypeFromCert(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	switch cert.PublicKey.(type) {
	case *rsa.PublicKey:
		return "RSA"
	case *ecdsa.PublicKey:
		return "ECDSA"
	default:
		return ""
	}
}

// tlsVersionString converts a tls.Version uint16 to a readable string.
func tlsVersionString(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("TLS 0x%04X", v)
	}
}
