// Package db provides typed repository for SNMP data operations.
package db

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/errors"
)

// SNMPRepository handles SNMP data persistence.
type SNMPRepository struct {
	db *DB
}

// NewSNMPRepository creates a new SNMPRepository.
func NewSNMPRepository(db *DB) *SNMPRepository {
	return &SNMPRepository{db: db}
}

// UpsertSNMPData inserts or replaces SNMP data for a host.
func (r *SNMPRepository) UpsertSNMPData(ctx context.Context, d *HostSNMPData) error {
	if d.HostID == uuid.Nil {
		return fmt.Errorf("host ID is required")
	}
	d.CollectedAt = time.Now().UTC()

	interfaces := d.Interfaces
	if interfaces == nil {
		interfaces = JSONB("[]")
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO host_snmp_data
			(host_id, sys_name, sys_descr, sys_location, sys_contact,
			 sys_uptime, if_count, interfaces, community, collected_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (host_id) DO UPDATE SET
			sys_name     = EXCLUDED.sys_name,
			sys_descr    = EXCLUDED.sys_descr,
			sys_location = EXCLUDED.sys_location,
			sys_contact  = EXCLUDED.sys_contact,
			sys_uptime   = EXCLUDED.sys_uptime,
			if_count     = EXCLUDED.if_count,
			interfaces   = EXCLUDED.interfaces,
			community    = EXCLUDED.community,
			collected_at = EXCLUDED.collected_at`,
		d.HostID, d.SysName, d.SysDescr, d.SysLocation, d.SysContact,
		d.SysUptime, d.IfCount, interfaces, d.Community, d.CollectedAt)
	if err != nil {
		return sanitizeDBError("upsert SNMP data", err)
	}
	return nil
}

// GetSNMPData returns SNMP data for a host, or nil if none collected.
func (r *SNMPRepository) GetSNMPData(ctx context.Context, hostID uuid.UUID) (*HostSNMPData, error) {
	d := &HostSNMPData{}
	err := r.db.QueryRowContext(ctx, `
		SELECT host_id, sys_name, sys_descr, sys_location, sys_contact,
		       sys_uptime, if_count, interfaces, community, collected_at
		FROM host_snmp_data
		WHERE host_id = $1`,
		hostID).Scan(
		&d.HostID, &d.SysName, &d.SysDescr, &d.SysLocation, &d.SysContact,
		&d.SysUptime, &d.IfCount, &d.Interfaces, &d.Community, &d.CollectedAt)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, errors.ErrNotFound("snmp data")
		}
		return nil, sanitizeDBError("get SNMP data", err)
	}
	return d, nil
}

// ListHostsWithSNMP returns host IDs that have SNMP data collected.
func (r *SNMPRepository) ListHostsWithSNMP(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT host_id FROM host_snmp_data ORDER BY collected_at DESC")
	if err != nil {
		return nil, sanitizeDBError("list SNMP hosts", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("error closing snmp rows", "error", err)
		}
	}()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan host ID: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
