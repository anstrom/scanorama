// Package db provides scan-related database operations for scanorama.
package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
)

// ScanJobRepository handles scan job operations.
type ScanJobRepository struct {
	db *DB
}

// NewScanJobRepository creates a new scan job repository.
func NewScanJobRepository(db *DB) *ScanJobRepository {
	return &ScanJobRepository{db: db}
}

// Create creates a new scan job.
func (r *ScanJobRepository) Create(ctx context.Context, job *ScanJob) error {
	query := `
		INSERT INTO scan_jobs (
			id, network_id, status,
			started_at, completed_at, scan_stats
		)
		VALUES (
			:id, :network_id, :status,
			:started_at, :completed_at, :scan_stats
		)
		RETURNING created_at`

	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}

	rows, err := r.db.NamedQueryContext(ctx, query, job)
	if err != nil {
		return sanitizeDBError("create scan job", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "error", err)
		}
	}()

	if rows.Next() {
		if err := rows.Scan(&job.CreatedAt); err != nil {
			return sanitizeDBError("scan created scan job", err)
		}
	}

	return nil
}

// UpdateStatus updates a scan job status.
func (r *ScanJobRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string, errorMsg *string) error {
	var query string
	var args []interface{}

	now := time.Now()

	switch status {
	case ScanJobStatusRunning:
		query = `
			UPDATE scan_jobs
			SET
			    status = $1,
			    started_at = $2
			WHERE id = $3`
		args = []interface{}{status, now, id}
	case ScanJobStatusCompleted, ScanJobStatusFailed:
		if errorMsg != nil {
			query = `
				UPDATE scan_jobs
				SET
				    status = $1,
				    completed_at = $2,
				    error_message = $3
				WHERE id = $4`
			args = []interface{}{status, now, *errorMsg, id}
		} else {
			query = `
				UPDATE scan_jobs
				SET
				    status = $1,
				    completed_at = $2
				WHERE id = $3`
			args = []interface{}{status, now, id}
		}
	default:
		query = `UPDATE scan_jobs SET status = $1 WHERE id = $2`
		args = []interface{}{status, id}
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return sanitizeDBError("update scan job status", err)
	}

	return nil
}

// GetByID retrieves a scan job by ID.
func (r *ScanJobRepository) GetByID(ctx context.Context, id uuid.UUID) (*ScanJob, error) {
	var job ScanJob
	query := `
		SELECT *
		FROM scan_jobs
		WHERE id = $1`

	if err := r.db.GetContext(ctx, &job, query, id); err != nil {
		return nil, sanitizeDBError("get scan job", err)
	}

	return &job, nil
}

// PortScanRepository handles port scan operations.
type PortScanRepository struct {
	db *DB
}

// NewPortScanRepository creates a new port scan repository.
func NewPortScanRepository(db *DB) *PortScanRepository {
	return &PortScanRepository{db: db}
}

// CreateBatch creates multiple port scan results in a transaction.
func (r *PortScanRepository) CreateBatch(ctx context.Context, scans []*PortScan) error {
	if len(scans) == 0 {
		return nil
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return sanitizeDBError("begin transaction", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Verify all host_ids exist to prevent foreign key constraint violations.
	hostIDs := make(map[uuid.UUID]bool)
	for _, scan := range scans {
		hostIDs[scan.HostID] = true
	}

	for hostID := range hostIDs {
		var exists bool
		verifyQuery := `SELECT EXISTS(SELECT 1 FROM hosts WHERE id = $1)`
		err := tx.QueryRowContext(ctx, verifyQuery, hostID).Scan(&exists)
		if err != nil {
			return sanitizeDBError("verify host existence", err)
		}
		if !exists {
			return fmt.Errorf("host %s does not exist, cannot create port scans", hostID)
		}
	}

	query := `
		INSERT INTO port_scans (
			id, job_id, host_id, port, protocol, state,
			service_name, service_version, service_product, banner
		)
		VALUES (
			:id, :job_id, :host_id, :port, :protocol, :state,
			:service_name, :service_version, :service_product, :banner
		)
		ON CONFLICT (job_id, host_id, port, protocol)
		DO UPDATE SET
			state = EXCLUDED.state,
			service_name = EXCLUDED.service_name,
			service_version = EXCLUDED.service_version,
			service_product = EXCLUDED.service_product,
			banner = EXCLUDED.banner,
			scanned_at = NOW()`

	for _, scan := range scans {
		if scan.ID == uuid.Nil {
			scan.ID = uuid.New()
		}

		_, err := tx.NamedExecContext(ctx, query, scan)
		if err != nil {
			return sanitizeDBError("insert port scan", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return sanitizeDBError("commit transaction", err)
	}

	return nil
}

// GetByHost retrieves all port scans for a host.
func (r *PortScanRepository) GetByHost(ctx context.Context, hostID uuid.UUID) ([]*PortScan, error) {
	var scans []*PortScan
	query := `
		SELECT *
		FROM port_scans
		WHERE host_id = $1
		ORDER BY port`

	if err := r.db.SelectContext(ctx, &scans, query, hostID); err != nil {
		return nil, sanitizeDBError("get port scans", err)
	}

	return scans, nil
}

// Scan represents a scan configuration and execution state.
type Scan struct {
	ID           uuid.UUID              `json:"id" db:"id"`
	Name         string                 `json:"name" db:"name"`
	Description  string                 `json:"description" db:"description"`
	Targets      []string               `json:"targets" db:"targets"`
	ScanType     string                 `json:"scan_type" db:"scan_type"`
	Ports        string                 `json:"ports" db:"ports"`
	ProfileID    *string                `json:"profile_id" db:"profile_id"`
	Options      map[string]interface{} `json:"options" db:"options"`
	ScheduleID   *int64                 `json:"schedule_id" db:"schedule_id"`
	Tags         []string               `json:"tags" db:"tags"`
	Status       string                 `json:"status" db:"status"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at" db:"updated_at"`
	StartedAt    *time.Time             `json:"started_at" db:"started_at"`
	CompletedAt  *time.Time             `json:"completed_at" db:"completed_at"`
	ErrorMessage *string                `json:"error_message,omitempty" db:"error_message"`
	DurationStr  *string                `json:"duration,omitempty" db:"-"`
	PortsScanned *string                `json:"ports_scanned,omitempty" db:"-"`
}

// ScanFilters represents filters for listing scans.
type ScanFilters struct {
	Status    string
	ScanType  string
	ProfileID *string
	Tags      []string
}

// ScanResult represents a scan result entry.
type ScanResult struct {
	ID           uuid.UUID `json:"id" db:"id"`
	ScanID       uuid.UUID `json:"scan_id" db:"scan_id"`
	HostID       uuid.UUID `json:"host_id" db:"host_id"`
	HostIP       string    `json:"host_ip" db:"host_ip"`
	Port         int       `json:"port" db:"port"`
	Protocol     string    `json:"protocol" db:"protocol"`
	State        string    `json:"state" db:"state"`
	Service      string    `json:"service" db:"service"`
	ScannedAt    time.Time `json:"scanned_at" db:"scanned_at"`
	OSName       string    `json:"os_name,omitempty" db:"os_name"`
	OSFamily     string    `json:"os_family,omitempty" db:"os_family"`
	OSVersion    string    `json:"os_version,omitempty" db:"os_version"`
	OSConfidence *int      `json:"os_confidence,omitempty" db:"os_confidence"`
}

// ScanSummary represents aggregated scan statistics.
type ScanSummary struct {
	ScanID      uuid.UUID `json:"scan_id"`
	TotalHosts  int       `json:"total_hosts"`
	TotalPorts  int       `json:"total_ports"`
	OpenPorts   int       `json:"open_ports"`
	ClosedPorts int       `json:"closed_ports"`
	Duration    int64     `json:"duration_ms"`
}
