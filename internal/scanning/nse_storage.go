package scanning

import (
	"context"
	"log/slog"
	"net"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/logging"
)

const nseStorageTimeout = 2 * time.Minute

// runNSEDataStorage writes NSE script data from the scan result into the
// port_banners and certificates tables. Runs as a best-effort background
// goroutine after every scan — errors are logged but never propagated.
func runNSEDataStorage(database *db.DB, hosts []Host) {
	defer func() {
		if r := recover(); r != nil {
			logging.Error("panic in NSE storage goroutine", "error", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), nseStorageTimeout)
	defer cancel()

	hostRepo := db.NewHostRepository(database)
	bannerRepo := db.NewBannerRepository(database)

	for i := range hosts {
		h := &hosts[i]
		if h.Status != "up" {
			continue
		}
		if !hasNSEData(h) {
			continue
		}

		ip := net.ParseIP(h.Address)
		if ip == nil {
			continue
		}
		dbHost, err := hostRepo.GetByIP(ctx, db.IPAddr{IP: ip})
		if err != nil {
			logging.Warn("nse storage: host not found by IP", "ip", h.Address, "error", err)
			continue
		}

		for j := range h.Ports {
			p := &h.Ports[j]
			if p.NSE == nil {
				continue
			}
			storePortNSE(ctx, bannerRepo, dbHost.ID, p)
		}
	}
}

// hasNSEData reports whether any port on the host has NSE output to persist.
func hasNSEData(h *Host) bool {
	for i := range h.Ports {
		if h.Ports[i].NSE != nil {
			return true
		}
	}
	return false
}

// storePortNSE writes the NSE data for a single port to the appropriate tables.
func storePortNSE(ctx context.Context, bannerRepo *db.BannerRepository, hostID uuid.UUID, p *Port) {
	nse := p.NSE

	// Persist http_title, ssh_key_fingerprint, and raw banner into port_banners.
	// UpsertNSEPortData uses COALESCE so that ZGrab2 raw banners are never
	// overwritten by the (typically shorter) NSE banner output.
	if nse.Banner != "" || nse.HTTPTitle != "" || nse.SSHKeyFingerprint != "" {
		b := &db.PortBanner{
			HostID:   hostID,
			Port:     int(p.Number),
			Protocol: p.Protocol,
		}
		if nse.Banner != "" {
			b.RawBanner = &nse.Banner
		}
		if nse.HTTPTitle != "" {
			b.HTTPTitle = &nse.HTTPTitle
		}
		if nse.SSHKeyFingerprint != "" {
			b.SSHKeyFingerprint = &nse.SSHKeyFingerprint
		}
		if err := bannerRepo.UpsertNSEPortData(ctx, b); err != nil {
			slog.Default().Warn("nse storage: failed to upsert port banner",
				"host_id", hostID, "port", p.Number, "error", err)
		}
	}

	// Persist ssl-cert data into the certificates table.
	if nse.SSLCert != nil {
		cert := nseToDBCert(hostID, int(p.Number), nse.SSLCert)
		if err := bannerRepo.UpsertCertificate(ctx, cert); err != nil {
			slog.Default().Warn("nse storage: failed to upsert certificate",
				"host_id", hostID, "port", p.Number, "error", err)
		}
	}
}

// nseToDBCert converts NSECertData to a db.Certificate ready for upsert.
func nseToDBCert(hostID uuid.UUID, port int, c *NSECertData) *db.Certificate {
	cert := &db.Certificate{
		HostID: hostID,
		Port:   port,
		SANs:   c.SANs,
	}
	if c.SubjectCN != "" {
		cert.SubjectCN = &c.SubjectCN
	}
	if c.Issuer != "" {
		cert.Issuer = &c.Issuer
	}
	if !c.NotBefore.IsZero() {
		cert.NotBefore = &c.NotBefore
	}
	if !c.NotAfter.IsZero() {
		cert.NotAfter = &c.NotAfter
	}
	if c.KeyType != "" {
		cert.KeyType = &c.KeyType
	}
	return cert
}
