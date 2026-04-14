// Package db provides typed repositories for banner and certificate records.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// BannerRepository handles port banner and TLS certificate operations.
type BannerRepository struct {
	db *DB
}

// NewBannerRepository creates a new BannerRepository.
func NewBannerRepository(db *DB) *BannerRepository {
	return &BannerRepository{db: db}
}

// UpsertPortBanner inserts or replaces a port banner record.
func (r *BannerRepository) UpsertPortBanner(ctx context.Context, b *PortBanner) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	b.ScannedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO port_banners (id, host_id, port, protocol, raw_banner, service, version, scanned_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (host_id, port, protocol) DO UPDATE SET
			raw_banner = EXCLUDED.raw_banner,
			service    = EXCLUDED.service,
			version    = EXCLUDED.version,
			scanned_at = EXCLUDED.scanned_at`,
		b.ID, b.HostID, b.Port, b.Protocol, b.RawBanner, b.Service, b.Version, b.ScannedAt)
	if err != nil {
		return sanitizeDBError("upsert port banner", err)
	}
	return nil
}

// UpsertNSEPortData writes NSE-derived fields (http_title, ssh_key_fingerprint,
// and optionally a raw banner) for a port. On conflict it never overwrites an
// existing raw_banner (ZGrab2 data takes precedence), but updates NSE-specific
// columns with any new non-null value.
func (r *BannerRepository) UpsertNSEPortData(ctx context.Context, b *PortBanner) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	b.ScannedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO port_banners
			(id, host_id, port, protocol, raw_banner, http_title, ssh_key_fingerprint, scanned_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (host_id, port, protocol) DO UPDATE SET
			raw_banner          = COALESCE(port_banners.raw_banner, EXCLUDED.raw_banner),
			http_title          = COALESCE(EXCLUDED.http_title, port_banners.http_title),
			ssh_key_fingerprint = COALESCE(EXCLUDED.ssh_key_fingerprint, port_banners.ssh_key_fingerprint),
			scanned_at          = EXCLUDED.scanned_at`,
		b.ID, b.HostID, b.Port, b.Protocol, b.RawBanner, b.HTTPTitle, b.SSHKeyFingerprint, b.ScannedAt)
	if err != nil {
		return sanitizeDBError("upsert nse port data", err)
	}
	return nil
}

// UpsertHTTPPortData writes HTTP-specific banner fields (status code, redirect,
// response headers, raw banner, service, version) for a port. On conflict it
// overwrites all HTTP-related columns but leaves http_title and
// ssh_key_fingerprint untouched (those are owned by NSE storage).
func (r *BannerRepository) UpsertHTTPPortData(ctx context.Context, b *PortBanner) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	b.ScannedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO port_banners
			(id, host_id, port, protocol, raw_banner, service, version,
			 http_status_code, http_redirect, http_response_headers, scanned_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (host_id, port, protocol) DO UPDATE SET
			raw_banner            = EXCLUDED.raw_banner,
			service               = EXCLUDED.service,
			version               = EXCLUDED.version,
			http_status_code      = EXCLUDED.http_status_code,
			http_redirect         = EXCLUDED.http_redirect,
			http_response_headers = EXCLUDED.http_response_headers,
			scanned_at            = EXCLUDED.scanned_at`,
		b.ID, b.HostID, b.Port, b.Protocol, b.RawBanner, b.Service, b.Version,
		b.HTTPStatusCode, b.HTTPRedirect, b.HTTPResponseHeaders, b.ScannedAt)
	if err != nil {
		return sanitizeDBError("upsert http port data", err)
	}
	return nil
}

// UpsertSSHPortData writes SSH-specific banner fields (server version and host
// key fingerprint) for a port. On conflict it overwrites these columns but
// leaves http_* columns untouched.
func (r *BannerRepository) UpsertSSHPortData(ctx context.Context, b *PortBanner) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	b.ScannedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO port_banners
			(id, host_id, port, protocol, raw_banner, service, version,
			 ssh_key_fingerprint, scanned_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (host_id, port, protocol) DO UPDATE SET
			raw_banner          = EXCLUDED.raw_banner,
			service             = EXCLUDED.service,
			version             = EXCLUDED.version,
			ssh_key_fingerprint = EXCLUDED.ssh_key_fingerprint,
			scanned_at          = EXCLUDED.scanned_at`,
		b.ID, b.HostID, b.Port, b.Protocol, b.RawBanner, b.Service, b.Version,
		b.SSHKeyFingerprint, b.ScannedAt)
	if err != nil {
		return sanitizeDBError("upsert ssh port data", err)
	}
	return nil
}

// ListPortBanners returns all port banner records for a host.
func (r *BannerRepository) ListPortBanners(ctx context.Context, hostID uuid.UUID) ([]*PortBanner, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, host_id, port, protocol, raw_banner, service, version,
		       http_title, ssh_key_fingerprint,
		       http_status_code, http_redirect, http_response_headers,
		       scanned_at
		FROM port_banners
		WHERE host_id = $1
		ORDER BY port ASC`,
		hostID)
	if err != nil {
		return nil, sanitizeDBError("list port banners", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("error closing banner rows", "error", err)
		}
	}()

	banners := make([]*PortBanner, 0)
	for rows.Next() {
		b := &PortBanner{}
		if err := rows.Scan(
			&b.ID, &b.HostID, &b.Port, &b.Protocol,
			&b.RawBanner, &b.Service, &b.Version,
			&b.HTTPTitle, &b.SSHKeyFingerprint,
			&b.HTTPStatusCode, &b.HTTPRedirect, &b.HTTPResponseHeaders,
			&b.ScannedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan banner row: %w", err)
		}
		banners = append(banners, b)
	}
	return banners, rows.Err()
}

// UpsertCertificate inserts or replaces a TLS certificate record.
func (r *BannerRepository) UpsertCertificate(ctx context.Context, c *Certificate) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	c.ScannedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO certificates
			(id, host_id, port, subject_cn, sans, issuer, not_before, not_after,
			 key_type, tls_version, cipher_suite, raw_banner, scanned_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (host_id, port) DO UPDATE SET
			subject_cn   = EXCLUDED.subject_cn,
			sans         = EXCLUDED.sans,
			issuer       = EXCLUDED.issuer,
			not_before   = EXCLUDED.not_before,
			not_after    = EXCLUDED.not_after,
			key_type     = EXCLUDED.key_type,
			tls_version  = EXCLUDED.tls_version,
			cipher_suite = EXCLUDED.cipher_suite,
			raw_banner   = EXCLUDED.raw_banner,
			scanned_at   = EXCLUDED.scanned_at`,
		c.ID, c.HostID, c.Port, c.SubjectCN, pq.Array(c.SANs), c.Issuer,
		c.NotBefore, c.NotAfter, c.KeyType, c.TLSVersion, c.CipherSuite,
		c.RawBanner, c.ScannedAt)
	if err != nil {
		return sanitizeDBError("upsert certificate", err)
	}
	return nil
}

// scanCertRows iterates cert query rows and returns the Certificate slice.
func scanCertRows(rows *sql.Rows) ([]*Certificate, error) {
	certs := make([]*Certificate, 0)
	for rows.Next() {
		c := &Certificate{}
		var sans any
		if err := rows.Scan(
			&c.ID, &c.HostID, &c.Port, &c.SubjectCN, &sans, &c.Issuer,
			&c.NotBefore, &c.NotAfter, &c.KeyType, &c.TLSVersion,
			&c.CipherSuite, &c.RawBanner, &c.ScannedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan cert row: %w", err)
		}
		c.SANs = parsePostgreSQLArray(sans)
		certs = append(certs, c)
	}
	return certs, rows.Err()
}

// ListCertificates returns all TLS certificate records for a host.
func (r *BannerRepository) ListCertificates(ctx context.Context, hostID uuid.UUID) ([]*Certificate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, host_id, port, subject_cn, sans, issuer, not_before, not_after,
		       key_type, tls_version, cipher_suite, raw_banner, scanned_at
		FROM certificates
		WHERE host_id = $1
		ORDER BY port ASC`,
		hostID)
	if err != nil {
		return nil, sanitizeDBError("list certificates", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("error closing cert rows", "error", err)
		}
	}()
	return scanCertRows(rows)
}

// ListExpiringCertificates returns certificates expiring within the given number of days.
func (r *BannerRepository) ListExpiringCertificates(ctx context.Context, days int) ([]*Certificate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, host_id, port, subject_cn, sans, issuer, not_before, not_after,
		       key_type, tls_version, cipher_suite, raw_banner, scanned_at
		FROM certificates
		WHERE not_after IS NOT NULL
		  AND not_after BETWEEN NOW() AND NOW() + ($1 * INTERVAL '1 day')
		ORDER BY not_after ASC`,
		days)
	if err != nil {
		return nil, sanitizeDBError("list expiring certs", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("error closing cert rows", "error", err)
		}
	}()
	return scanCertRows(rows)
}

// ExpiringCertificate is a certificate record enriched with host IP and hostname.
type ExpiringCertificate struct {
	HostID    string    `json:"host_id"`
	HostIP    string    `json:"host_ip"`
	Hostname  string    `json:"hostname"`
	Port      int       `json:"port"`
	Protocol  string    `json:"protocol"`
	SubjectCN string    `json:"subject_cn"`
	NotAfter  time.Time `json:"not_after"`
	DaysLeft  int       `json:"days_left"`
}

// ExpiringCertificatesResponse is the response body for the expiring certs endpoint.
type ExpiringCertificatesResponse struct {
	Certificates []ExpiringCertificate `json:"certificates"`
	Days         int                   `json:"days"`
}

// hoursPerDay is the number of hours in a day, used to convert hours to days.
const hoursPerDay = 24

// ListExpiringCertificatesWithHosts returns certificates expiring within the
// given number of days, joined with the hosts table to include host IP and hostname.
func (r *BannerRepository) ListExpiringCertificatesWithHosts(
	ctx context.Context, days int,
) ([]ExpiringCertificate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			c.host_id::text,
			h.ip_address::text,
			COALESCE(h.hostname, '') AS hostname,
			c.port,
			COALESCE(c.subject_cn, '') AS subject_cn,
			c.not_after
		FROM certificates c
		JOIN hosts h ON h.id = c.host_id
		WHERE c.not_after IS NOT NULL
		  AND c.not_after BETWEEN NOW() AND NOW() + ($1 * INTERVAL '1 day')
		ORDER BY c.not_after ASC`,
		days)
	if err != nil {
		return nil, sanitizeDBError("list expiring certs with hosts", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("error closing expiring cert rows", "error", err)
		}
	}()

	var result []ExpiringCertificate
	now := time.Now().UTC()
	for rows.Next() {
		var c ExpiringCertificate
		if err := rows.Scan(
			&c.HostID, &c.HostIP, &c.Hostname,
			&c.Port, &c.SubjectCN, &c.NotAfter,
		); err != nil {
			return nil, fmt.Errorf("failed to scan expiring cert row: %w", err)
		}
		c.Protocol = "tcp"
		c.DaysLeft = int(c.NotAfter.Sub(now).Hours() / hoursPerDay)
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []ExpiringCertificate{}
	}
	return result, nil
}
