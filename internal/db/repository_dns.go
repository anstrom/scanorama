// Package db provides a typed repository for host DNS record operations.
package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// DNSRepository handles host_dns_records database operations.
type DNSRepository struct {
	db *DB
}

// NewDNSRepository creates a new DNSRepository.
func NewDNSRepository(db *DB) *DNSRepository {
	return &DNSRepository{db: db}
}

// UpsertDNSRecords replaces all DNS records for the given host.
// Existing records for the host are deleted and the new set is inserted.
func (r *DNSRepository) UpsertDNSRecords(ctx context.Context, hostID uuid.UUID, records []DNSRecord) error {
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("dns records: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM host_dns_records WHERE host_id = $1`, hostID,
	); err != nil {
		return fmt.Errorf("dns records: delete existing: %w", err)
	}

	if len(records) > 0 {
		// Batch insert all records in a single statement to avoid N+1 round-trips.
		// Each row binds 5 args: id, host_id, record_type, value, ttl.
		const argsPerRow = 5
		placeholders := make([]string, 0, len(records))
		args := make([]interface{}, 0, len(records)*argsPerRow)
		for i := range records {
			records[i].HostID = hostID
			if records[i].ID == uuid.Nil {
				records[i].ID = uuid.New()
			}
			b := i * argsPerRow
			placeholders = append(placeholders,
				fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,NOW())",
					b+1, b+2, b+3, b+4, b+5)) //nolint:mnd
			args = append(args,
				records[i].ID, records[i].HostID,
				records[i].RecordType, records[i].Value, records[i].TTL)
		}
		query := `INSERT INTO host_dns_records (id, host_id, record_type, value, ttl, resolved_at) VALUES ` +
			strings.Join(placeholders, ",")
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("dns records: batch insert: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("dns records: commit: %w", err)
	}
	return nil
}

// ListDNSRecords returns all DNS records for the given host, ordered by type then value.
func (r *DNSRepository) ListDNSRecords(ctx context.Context, hostID uuid.UUID) ([]DNSRecord, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, host_id, record_type, value, ttl, resolved_at
		FROM host_dns_records
		WHERE host_id = $1
		ORDER BY record_type, value`, hostID)
	if err != nil {
		return nil, sanitizeDBError("list dns records", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = fmt.Errorf("dns records: close rows: %w", cerr)
		}
	}()

	var records []DNSRecord
	for rows.Next() {
		var rec DNSRecord
		if err := rows.Scan(
			&rec.ID, &rec.HostID, &rec.RecordType, &rec.Value,
			&rec.TTL, &rec.ResolvedAt,
		); err != nil {
			return nil, sanitizeDBError("scan dns record row", err)
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, sanitizeDBError("iterate dns records", err)
	}
	if records == nil {
		records = []DNSRecord{}
	}
	return records, nil
}
