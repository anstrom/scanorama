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

// ListPortBanners returns all port banner records for a host.
func (r *BannerRepository) ListPortBanners(ctx context.Context, hostID uuid.UUID) ([]*PortBanner, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, host_id, port, protocol, raw_banner, service, version, scanned_at
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

	var banners []*PortBanner
	for rows.Next() {
		b := &PortBanner{}
		if err := rows.Scan(
			&b.ID, &b.HostID, &b.Port, &b.Protocol,
			&b.RawBanner, &b.Service, &b.Version, &b.ScannedAt,
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
			 key_type, tls_version, raw_banner, scanned_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT (host_id, port) DO UPDATE SET
			subject_cn  = EXCLUDED.subject_cn,
			sans        = EXCLUDED.sans,
			issuer      = EXCLUDED.issuer,
			not_before  = EXCLUDED.not_before,
			not_after   = EXCLUDED.not_after,
			key_type    = EXCLUDED.key_type,
			tls_version = EXCLUDED.tls_version,
			raw_banner  = EXCLUDED.raw_banner,
			scanned_at  = EXCLUDED.scanned_at`,
		c.ID, c.HostID, c.Port, c.SubjectCN, pq.Array(c.SANs), c.Issuer,
		c.NotBefore, c.NotAfter, c.KeyType, c.TLSVersion, c.RawBanner, c.ScannedAt)
	if err != nil {
		return sanitizeDBError("upsert certificate", err)
	}
	return nil
}

// scanCertRows iterates cert query rows and returns the Certificate slice.
func scanCertRows(rows *sql.Rows) ([]*Certificate, error) {
	var certs []*Certificate
	for rows.Next() {
		c := &Certificate{}
		var sans interface{}
		if err := rows.Scan(
			&c.ID, &c.HostID, &c.Port, &c.SubjectCN, &sans, &c.Issuer,
			&c.NotBefore, &c.NotAfter, &c.KeyType, &c.TLSVersion, &c.RawBanner, &c.ScannedAt,
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
		       key_type, tls_version, raw_banner, scanned_at
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
		       key_type, tls_version, raw_banner, scanned_at
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
