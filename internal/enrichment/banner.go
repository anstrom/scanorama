// Package enrichment provides post-scan host enrichment: banner grabbing, TLS
// certificate extraction, DNS resolution, and SNMP probing.
package enrichment

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	zcrypto_x509 "github.com/zmap/zcrypto/x509"
	"github.com/zmap/zgrab2"
	zgrabSSH "github.com/zmap/zgrab2/lib/ssh"
	zgrabhttp "github.com/zmap/zgrab2/modules/http"

	"github.com/anstrom/scanorama/internal/db"
)

const (
	bannerReadBytes   = 1024
	bannerDialTimeout = 5 * time.Second
	bannerReadTimeout = 3 * time.Second
	bannerConcurrency = 10

	zgrabUserAgent    = "Mozilla/5.0 zgrab/0.x"
	zgrabMaxSizeKB    = 256
	zgrabMaxRedirects = 3

	serviceSSH = "ssh"
)

// tlsPorts is the set of port numbers that should receive a TLS/HTTPS probe.
var tlsPorts = map[int]bool{
	443: true, 8443: true, 465: true, 636: true,
	993: true, 995: true, 5986: true, 8883: true,
}

// httpServices is the set of nmap service names that trigger an HTTP probe
// on non-TLS ports.
var httpServices = map[string]bool{
	"http": true, "http-alt": true, "http-proxy": true,
	"https-alt": true, "http-mgmt": true,
}

// interestingHTTPHeaders are the response headers stored in the JSONB column.
var interestingHTTPHeaders = []string{
	"Server", "X-Powered-By", "Content-Type",
	"X-Frame-Options", "Strict-Transport-Security",
	"X-Content-Type-Options", "X-XSS-Protection",
	"Content-Security-Policy", "Location",
}

// PortInfo carries a port number and its nmap-detected service name.
type PortInfo struct {
	Number  int
	Service string // nmap-detected service name, e.g. "http", "ssh", "ftp"
}

// BannerTarget describes a host and its open TCP ports to probe.
type BannerTarget struct {
	HostID uuid.UUID
	IP     string
	Ports  []PortInfo
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
		for _, pi := range t.Ports {
			wg.Add(1)
			sem <- struct{}{}
			go func(target BannerTarget, p PortInfo) {
				defer wg.Done()
				defer func() { <-sem }()
				g.grabOne(ctx, target, p)
			}(t, pi)
		}
	}

	wg.Wait()
}

// grabOne dispatches to the appropriate grabber based on port and service.
// Precedence: TLS ports → zgrab2 HTTPS (fallback to stdlib TLS);
// SSH service → zgrab2 SSH (fallback to plain); HTTP service → zgrab2 HTTP
// (fallback to plain); everything else → plain TCP.
func (g *BannerGrabber) grabOne(ctx context.Context, t BannerTarget, pi PortInfo) {
	addr := fmt.Sprintf("%s:%d", t.IP, pi.Number)
	svc := strings.ToLower(pi.Service)

	switch {
	case tlsPorts[pi.Number]:
		if err := g.grabZGrabHTTPS(ctx, t, pi.Number); err != nil {
			g.logger.Debug("zgrab HTTPS failed, falling back to stdlib TLS",
				"host", t.IP, "port", pi.Number, "error", err)
			g.grabTLS(ctx, t, pi.Number, addr)
		}

	case svc == serviceSSH:
		if err := g.grabZGrabSSH(ctx, t, pi.Number, addr); err != nil {
			g.logger.Debug("zgrab SSH failed, falling back to plain grab",
				"host", t.IP, "port", pi.Number, "error", err)
			g.grabPlain(ctx, t, pi.Number, addr)
		}

	case httpServices[svc]:
		if err := g.grabZGrabHTTP(ctx, t, pi.Number); err != nil {
			g.logger.Debug("zgrab HTTP failed, falling back to plain grab",
				"host", t.IP, "port", pi.Number, "error", err)
			g.grabPlain(ctx, t, pi.Number, addr)
		}

	default:
		g.grabPlain(ctx, t, pi.Number, addr)
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

	if rawBanner == nil && svc == "" {
		return // nothing to store — no banner and no port-only rule matched
	}

	if err := g.repo.UpsertPortBanner(ctx, banner); err != nil {
		g.logger.Warn("failed to store port banner",
			"host", t.IP, "port", port, "error", err)
	}
}

// grabTLS performs a TLS handshake using the stdlib, extracts certificate info,
// and stores both a banner and a certificate record. Used as fallback when the
// zgrab2 HTTPS scanner fails.
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

	// Cipher suite from stdlib ConnectionState.
	cipherName := tls.CipherSuiteName(cs.CipherSuite)
	if cipherName != "" {
		cert.CipherSuite = &cipherName
	}

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

// ── ZGrab2 scanners ──────────────────────────────────────────────────────────

// zgrabHTTPScanner builds a configured zgrab2 HTTP scanner and its dialer group.
// useHTTPS controls whether TLS is used for the initial connection.
func zgrabHTTPScanner(port int, useHTTPS bool) (*zgrabhttp.Scanner, *zgrab2.DialerGroup, error) {
	flags := &zgrabhttp.Flags{
		BaseFlags: zgrab2.BaseFlags{
			Port:           uint(port), //nolint:gosec // port range is 1-65535
			ConnectTimeout: bannerDialTimeout,
			TargetTimeout:  bannerDialTimeout + bannerReadTimeout,
		},
		UseHTTPS:     useHTTPS,
		UserAgent:    zgrabUserAgent,
		MaxSize:      zgrabMaxSizeKB,
		MaxRedirects: zgrabMaxRedirects,
		Method:       "GET",
		Endpoint:     "/",
	}

	scanner := new(zgrabhttp.Scanner)
	if err := scanner.Init(flags); err != nil {
		return nil, nil, fmt.Errorf("init scanner: %w", err)
	}
	if err := scanner.InitPerSender(0); err != nil {
		return nil, nil, fmt.Errorf("init per-sender: %w", err)
	}

	dialGroup, err := scanner.GetDialerGroupConfig().GetDefaultDialerGroupFromConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("get dialer group: %w", err)
	}
	return scanner, dialGroup, nil
}

// grabZGrabHTTPS scans a TLS port using the zgrab2 HTTP module with TLS enabled.
// Stores TLS certificate data (including cipher suite) and HTTP response fields.
func (g *BannerGrabber) grabZGrabHTTPS(ctx context.Context, t BannerTarget, port int) error {
	scanner, dialGroup, err := zgrabHTTPScanner(port, true)
	if err != nil {
		return fmt.Errorf("build scanner: %w", err)
	}

	ip := net.ParseIP(t.IP)
	if ip == nil {
		return fmt.Errorf("invalid IP: %s", t.IP)
	}
	target := &zgrab2.ScanTarget{IP: ip, Port: uint(port)} //nolint:gosec

	_, result, scanErr := scanner.Scan(ctx, dialGroup, target)
	httpResults, ok := result.(*zgrabhttp.Results)
	if !ok || httpResults == nil || httpResults.Response == nil {
		if scanErr != nil {
			return fmt.Errorf("scan: %w", scanErr)
		}
		return fmt.Errorf("nil or unexpected result")
	}

	// Extract TLS info from the first available TLS log (initial HTTPS request).
	tlsLog := findTLSLog(httpResults)
	if tlsLog != nil && tlsLog.HandshakeLog != nil {
		g.storeZGrabCert(ctx, t, port, tlsLog)
	}

	// Store HTTP banner regardless of TLS extraction success.
	g.storeZGrabHTTPBanner(ctx, t, port, httpResults)

	// Return the scan error only when we got absolutely nothing useful.
	if scanErr != nil && tlsLog == nil {
		return fmt.Errorf("scan: %w", scanErr)
	}
	return nil
}

// grabZGrabHTTP scans a plain-HTTP port using the zgrab2 HTTP module.
func (g *BannerGrabber) grabZGrabHTTP(ctx context.Context, t BannerTarget, port int) error {
	scanner, dialGroup, err := zgrabHTTPScanner(port, false)
	if err != nil {
		return fmt.Errorf("build scanner: %w", err)
	}

	ip := net.ParseIP(t.IP)
	if ip == nil {
		return fmt.Errorf("invalid IP: %s", t.IP)
	}
	target := &zgrab2.ScanTarget{IP: ip, Port: uint(port)} //nolint:gosec

	_, result, scanErr := scanner.Scan(ctx, dialGroup, target)
	httpResults, ok := result.(*zgrabhttp.Results)
	if !ok || httpResults == nil || httpResults.Response == nil {
		if scanErr != nil {
			return fmt.Errorf("scan: %w", scanErr)
		}
		return fmt.Errorf("nil or unexpected result")
	}

	g.storeZGrabHTTPBanner(ctx, t, port, httpResults)
	return nil
}

// grabZGrabSSH probes an SSH port using zgrab2's SSH library.
// Extracts the server version string and host key fingerprint without
// attempting authentication (DontAuthenticate: true).
func (g *BannerGrabber) grabZGrabSSH(ctx context.Context, t BannerTarget, port int, addr string) error {
	var capturedFingerprint string

	connLog := &zgrabSSH.HandshakeLog{}
	sshCfg := &zgrabSSH.ClientConfig{
		Config: zgrabSSH.Config{
			ConnLog: connLog,
		},
		DontAuthenticate: true,
		Timeout:          bannerDialTimeout,
		HostKeyCallback: func(_ string, _ net.Addr, key zgrabSSH.PublicKey) error {
			capturedFingerprint = zgrabSSH.FingerprintSHA256(key)
			return nil
		},
	}

	dialer := net.Dialer{Timeout: bannerDialTimeout}
	netConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("tcp dial: %w", err)
	}
	defer netConn.Close() //nolint:errcheck

	if err := netConn.SetDeadline(time.Now().Add(bannerDialTimeout)); err != nil {
		return fmt.Errorf("set deadline: %w", err)
	}

	sshConn, _, _, err := zgrabSSH.NewClientConn(netConn, addr, sshCfg)
	if sshConn != nil {
		_ = sshConn.Close()
	}
	// err may be non-nil when DontAuthenticate causes the server to close the
	// connection after the key exchange — we still got the data we need.
	if err != nil && capturedFingerprint == "" && connLog.ServerID == nil {
		return fmt.Errorf("ssh handshake: %w", err)
	}

	var serverVersion string
	if connLog.ServerID != nil {
		serverVersion = connLog.ServerID.SoftwareVersion
	}

	raw := buildSSHBanner(serverVersion, capturedFingerprint)
	svc := serviceSSH
	banner := &db.PortBanner{
		HostID:    t.HostID,
		Port:      port,
		Protocol:  db.ProtocolTCP,
		RawBanner: &raw,
		Service:   &svc,
	}
	if serverVersion != "" {
		banner.Version = &serverVersion
	}
	if capturedFingerprint != "" {
		fp := capturedFingerprint
		banner.SSHKeyFingerprint = &fp
	}

	if err := g.repo.UpsertSSHPortData(ctx, banner); err != nil {
		g.logger.Warn("failed to store SSH banner",
			"host", t.IP, "port", port, "error", err)
	}
	return nil
}

// ── ZGrab2 result extraction helpers ────────────────────────────────────────

// findTLSLog returns the first non-nil TLSLog from an HTTP scan result,
// checking the redirect chain before the final response so that the certificate
// from the initial TLS connection is used when the server redirects.
func findTLSLog(results *zgrabhttp.Results) *zgrab2.TLSLog {
	for _, r := range results.RedirectResponseChain {
		if r.Request != nil && r.Request.TLSLog != nil {
			return r.Request.TLSLog
		}
	}
	if results.Response != nil && results.Response.Request != nil {
		return results.Response.Request.TLSLog
	}
	return nil
}

// populateCertFromParsed fills in leaf-certificate fields on cert from a
// parsed zcrypto x509 certificate. It is a no-op when parsed is nil.
func populateCertFromParsed(cert *db.Certificate, parsed *zcrypto_x509.Certificate) {
	if parsed == nil {
		return
	}
	if parsed.Subject.CommonName != "" {
		cn := parsed.Subject.CommonName
		cert.SubjectCN = &cn
	}
	cert.SANs = parsed.DNSNames
	if parsed.Issuer.CommonName != "" {
		issuer := parsed.Issuer.CommonName
		cert.Issuer = &issuer
	}
	cert.NotBefore = &parsed.NotBefore
	cert.NotAfter = &parsed.NotAfter
	if kt := keyTypeFromPublicKey(parsed.PublicKey); kt != "" {
		cert.KeyType = &kt
	}
}

// storeZGrabCert extracts TLS certificate and cipher suite from a zgrab2
// TLSLog and stores the result via UpsertCertificate.
func (g *BannerGrabber) storeZGrabCert(ctx context.Context, t BannerTarget, port int, tlsLog *zgrab2.TLSLog) {
	hs := tlsLog.HandshakeLog
	if hs == nil {
		return
	}

	cert := &db.Certificate{
		HostID: t.HostID,
		Port:   port,
	}

	// Cipher suite and TLS version from ServerHello.
	if hs.ServerHello != nil {
		cs := hs.ServerHello.CipherSuite.String()
		if cs != "" {
			cert.CipherSuite = &cs
		}
		tlsVer := hs.ServerHello.Version.String()
		if tlsVer != "" {
			cert.TLSVersion = &tlsVer
		}
	}

	// Leaf certificate from ServerCertificates.
	if hs.ServerCertificates != nil {
		populateCertFromParsed(cert, hs.ServerCertificates.Certificate.Parsed)
	}

	summary := buildTLSSummary(cert)
	cert.RawBanner = &summary

	if err := g.repo.UpsertCertificate(ctx, cert); err != nil {
		g.logger.Warn("failed to store zgrab certificate",
			"host", t.IP, "port", port, "error", err)
	}
}

// storeZGrabHTTPBanner extracts HTTP response fields from a zgrab2 HTTP result
// and stores them via UpsertHTTPPortData.
func (g *BannerGrabber) storeZGrabHTTPBanner(
	ctx context.Context, t BannerTarget, port int, results *zgrabhttp.Results,
) {
	resp := results.Response
	if resp == nil {
		return
	}

	banner := &db.PortBanner{
		HostID:   t.HostID,
		Port:     port,
		Protocol: db.ProtocolTCP,
	}

	// HTTP status code.
	if resp.StatusCode > 0 {
		sc := int16(resp.StatusCode) //nolint:gosec // status codes are well within int16 range
		banner.HTTPStatusCode = &sc
	}

	// Final URL after redirects.
	if len(results.RedirectResponseChain) > 0 && resp.Request != nil && resp.Request.URL != nil {
		u := resp.Request.URL.String()
		banner.HTTPRedirect = &u
	}

	// Selected response headers as a JSONB map.
	banner.HTTPResponseHeaders = extractHeaders(resp.Header)

	// Service identification and raw banner summary.
	svc := "http"
	if resp.TLS != nil {
		svc = "https"
	}
	banner.Service = &svc

	// Server header as version if present.
	if serverHdr := resp.Header.Get("Server"); serverHdr != "" {
		banner.Version = &serverHdr
	}

	var raw string
	if resp.StatusCode > 0 {
		raw = fmt.Sprintf("HTTP %d %s", resp.StatusCode, svc)
	} else {
		raw = svc
	}
	banner.RawBanner = &raw

	if err := g.repo.UpsertHTTPPortData(ctx, banner); err != nil {
		g.logger.Warn("failed to store HTTP banner",
			"host", t.IP, "port", port, "error", err)
	}
}

// extractHeaders returns a JSON-encoded map of interesting HTTP headers.
// Returns nil when no interesting headers are present.
func extractHeaders(h interface{ Get(string) string }) db.JSONB {
	m := make(map[string]string, len(interestingHTTPHeaders))
	for _, name := range interestingHTTPHeaders {
		if v := h.Get(name); v != "" {
			m[name] = v
		}
	}
	if len(m) == 0 {
		return nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return db.JSONB(data)
}

// buildSSHBanner returns a short human-readable summary string for SSH banners.
func buildSSHBanner(version, fingerprint string) string {
	switch {
	case version != "" && fingerprint != "":
		return fmt.Sprintf("SSH %s %s", version, fingerprint)
	case version != "":
		return fmt.Sprintf("SSH %s", version)
	case fingerprint != "":
		return fmt.Sprintf("SSH %s", fingerprint)
	default:
		return "SSH"
	}
}

// buildTLSSummary returns a short human-readable summary for a certificate.
func buildTLSSummary(c *db.Certificate) string {
	tlsVer := ""
	if c.TLSVersion != nil {
		tlsVer = *c.TLSVersion
	}
	cn := ""
	if c.SubjectCN != nil {
		cn = *c.SubjectCN
	}
	return fmt.Sprintf("TLS %s CN=%s", tlsVer, cn)
}

// ── Legacy / fallback helpers ────────────────────────────────────────────────

// portServiceHints maps well-known port numbers to their service names for
// use when banner text doesn't provide enough signal.
var portServiceHints = map[int]string{
	21: "ftp", 22: serviceSSH,
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
		service = serviceSSH
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

// keyTypeFromCert returns a short string describing the public key algorithm
// for a stdlib x509.Certificate.
func keyTypeFromCert(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}
	return keyTypeFromPublicKey(cert.PublicKey)
}

// keyTypeFromPublicKey returns a short string describing the public key algorithm
// for any public key value (handles both stdlib and zcrypto certificates, since
// both use stdlib crypto/rsa and crypto/ecdsa key types).
func keyTypeFromPublicKey(pub any) string {
	switch pub.(type) {
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
