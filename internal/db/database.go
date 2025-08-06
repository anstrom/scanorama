// Package db provides database connectivity and data models for scanorama.
// It handles database migrations, host management, scan results storage,
// and provides the core data access layer for the application.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // postgres driver
)

// Common errors for database operations.
var (
	ErrNotFound = fmt.Errorf("record not found")
)

const (
	// Default database configuration values.
	defaultPostgresPort    = 5432
	defaultMaxOpenConns    = 25
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 5
	defaultConnMaxIdleTime = 5
)

// DB wraps sqlx.DB with additional functionality.
type DB struct {
	*sqlx.DB
}

// Config holds database configuration.
type Config struct {
	Host            string        `yaml:"host" json:"host"`
	Port            int           `yaml:"port" json:"port"`
	Database        string        `yaml:"database" json:"database"`
	Username        string        `yaml:"username" json:"username"`
	Password        string        `yaml:"password" json:"password"`
	SSLMode         string        `yaml:"ssl_mode" json:"ssl_mode"`
	MaxOpenConns    int           `yaml:"max_open_conns" json:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns" json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime" json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn_max_idle_time" json:"conn_max_idle_time"`
}

// DefaultConfig returns the default database configuration.
// Database name, username, and password must be explicitly configured.
func DefaultConfig() Config {
	return Config{
		Host:            "localhost",
		Port:            defaultPostgresPort,
		Database:        "", // Must be configured
		Username:        "", // Must be configured
		Password:        "", // Must be configured
		SSLMode:         "prefer",
		MaxOpenConns:    defaultMaxOpenConns,
		MaxIdleConns:    defaultMaxIdleConns,
		ConnMaxLifetime: defaultConnMaxLifetime * time.Minute,
		ConnMaxIdleTime: defaultConnMaxIdleTime * time.Minute,
	}
}

// Connect establishes a connection to PostgreSQL.
func Connect(ctx context.Context, config *Config) (*DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		config.Host, config.Port, config.Database,
		config.Username, config.Password, config.SSLMode,
	)

	db, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{DB: db}, nil
}

// Repository provides database operations.
type Repository struct {
	db *DB
}

// NewRepository creates a new repository instance.
func NewRepository(db *DB) *Repository {
	return &Repository{db: db}
}

// ScanTargetRepository handles scan target operations.
type ScanTargetRepository struct {
	db *DB
}

// NewScanTargetRepository creates a new scan target repository.
func NewScanTargetRepository(db *DB) *ScanTargetRepository {
	return &ScanTargetRepository{db: db}
}

// Create creates a new scan target.
func (r *ScanTargetRepository) Create(ctx context.Context, target *ScanTarget) error {
	query := `
		INSERT INTO scan_targets (id, name, network, description, scan_interval_seconds, scan_ports, scan_type, enabled)
		VALUES (:id, :name, :network, :description, :scan_interval_seconds, :scan_ports, :scan_type, :enabled)
		RETURNING created_at, updated_at`

	if target.ID == uuid.Nil {
		target.ID = uuid.New()
	}

	rows, err := r.db.NamedQueryContext(ctx, query, target)
	if err != nil {
		return fmt.Errorf("failed to create scan target: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	if rows.Next() {
		if err := rows.Scan(&target.CreatedAt, &target.UpdatedAt); err != nil {
			return fmt.Errorf("failed to scan created scan target: %w", err)
		}
	}

	return nil
}

// GetByID retrieves a scan target by ID.
func (r *ScanTargetRepository) GetByID(ctx context.Context, id uuid.UUID) (*ScanTarget, error) {
	var target ScanTarget
	query := `SELECT * FROM scan_targets WHERE id = $1`

	if err := r.db.GetContext(ctx, &target, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("scan target not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get scan target: %w", err)
	}

	return &target, nil
}

// GetAll retrieves all scan targets.
func (r *ScanTargetRepository) GetAll(ctx context.Context) ([]*ScanTarget, error) {
	var targets []*ScanTarget
	query := `SELECT * FROM scan_targets ORDER BY name`

	if err := r.db.SelectContext(ctx, &targets, query); err != nil {
		return nil, fmt.Errorf("failed to get scan targets: %w", err)
	}

	return targets, nil
}

// GetEnabled retrieves all enabled scan targets.
func (r *ScanTargetRepository) GetEnabled(ctx context.Context) ([]*ScanTarget, error) {
	var targets []*ScanTarget
	query := `SELECT * FROM scan_targets WHERE enabled = true ORDER BY name`

	if err := r.db.SelectContext(ctx, &targets, query); err != nil {
		return nil, fmt.Errorf("failed to get enabled scan targets: %w", err)
	}

	return targets, nil
}

// Update updates a scan target.
func (r *ScanTargetRepository) Update(ctx context.Context, target *ScanTarget) error {
	query := `
		UPDATE scan_targets
		SET name = :name, network = :network, description = :description,
		    scan_interval_seconds = :scan_interval_seconds, scan_ports = :scan_ports,
		    scan_type = :scan_type, enabled = :enabled
		WHERE id = :id
		RETURNING updated_at`

	rows, err := r.db.NamedQueryContext(ctx, query, target)
	if err != nil {
		return fmt.Errorf("failed to update scan target: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	if rows.Next() {
		if err := rows.Scan(&target.UpdatedAt); err != nil {
			return fmt.Errorf("failed to scan updated scan target: %w", err)
		}
	}

	return nil
}

// Delete deletes a scan target.
func (r *ScanTargetRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM scan_targets WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete scan target: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("scan target not found")
	}

	return nil
}

// ScanJobRepository handles scan job database operations.
type ScanJobRepository struct {
	db *DB
}

// CreateInTransaction creates a scan job within an existing transaction.
func (r *ScanJobRepository) CreateInTransaction(ctx context.Context, tx *sqlx.Tx, job *ScanJob) error {
	query := `
		INSERT INTO scan_jobs (id, target_id, profile_id, status, started_at, completed_at, error_message, scan_stats)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at`

	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}

	err := tx.QueryRowContext(ctx, query,
		job.ID, job.TargetID, job.ProfileID, job.Status,
		job.StartedAt, job.CompletedAt, job.ErrorMessage, job.ScanStats).Scan(&job.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create scan job: %w", err)
	}

	return nil
}

// NewScanJobRepository creates a new scan job repository.
func NewScanJobRepository(db *DB) *ScanJobRepository {
	return &ScanJobRepository{db: db}
}

// Create creates a new scan job.
func (r *ScanJobRepository) Create(ctx context.Context, job *ScanJob) error {
	query := `
		INSERT INTO scan_jobs (id, target_id, status, started_at, completed_at, scan_stats)
		VALUES (:id, :target_id, :status, :started_at, :completed_at, :scan_stats)
		RETURNING created_at`

	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}

	rows, err := r.db.NamedQueryContext(ctx, query, job)
	if err != nil {
		return fmt.Errorf("failed to create scan job: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	if rows.Next() {
		if err := rows.Scan(&job.CreatedAt); err != nil {
			return fmt.Errorf("failed to scan created scan job: %w", err)
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
		query = `UPDATE scan_jobs SET status = $1, started_at = $2 WHERE id = $3`
		args = []interface{}{status, now, id}
	case ScanJobStatusCompleted, ScanJobStatusFailed:
		if errorMsg != nil {
			query = `UPDATE scan_jobs SET status = $1, completed_at = $2, error_message = $3 WHERE id = $4`
			args = []interface{}{status, now, *errorMsg, id}
		} else {
			query = `UPDATE scan_jobs SET status = $1, completed_at = $2 WHERE id = $3`
			args = []interface{}{status, now, id}
		}
	default:
		query = `UPDATE scan_jobs SET status = $1 WHERE id = $2`
		args = []interface{}{status, id}
	}

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update scan job status: %w", err)
	}

	return nil
}

// GetByID retrieves a scan job by ID.
func (r *ScanJobRepository) GetByID(ctx context.Context, id uuid.UUID) (*ScanJob, error) {
	var job ScanJob
	query := `SELECT * FROM scan_jobs WHERE id = $1`

	if err := r.db.GetContext(ctx, &job, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("scan job not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get scan job: %w", err)
	}

	return &job, nil
}

// HostRepository handles host database operations.
type HostRepository struct {
	db *DB
}

// CreateInTransaction creates a host within an existing transaction.
func (r *HostRepository) CreateInTransaction(ctx context.Context, tx *sqlx.Tx, host *Host) error {
	query := `
		INSERT INTO hosts (id, ip_address, hostname, mac_address, vendor, os_family, os_name, os_version,
			os_confidence, os_detected_at, os_method, os_details, discovery_method, response_time_ms,
			discovery_count, ignore_scanning, first_seen, last_seen, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING first_seen, last_seen`

	if host.ID == uuid.Nil {
		host.ID = uuid.New()
	}

	err := tx.QueryRowContext(ctx, query,
		host.ID, host.IPAddress, host.Hostname, host.MACAddress, host.Vendor,
		host.OSFamily, host.OSName, host.OSVersion, host.OSConfidence, host.OSDetectedAt,
		host.OSMethod, host.OSDetails, host.DiscoveryMethod, host.ResponseTimeMS,
		host.DiscoveryCount, host.IgnoreScanning, host.FirstSeen, host.LastSeen, host.Status).Scan(
		&host.FirstSeen, &host.LastSeen)
	if err != nil {
		return fmt.Errorf("failed to create host: %w", err)
	}

	return nil
}

// GetByIPInTransaction retrieves a host by IP address within an existing transaction.
func (r *HostRepository) GetByIPInTransaction(ctx context.Context, tx *sqlx.Tx, ipAddress IPAddr) (*Host, error) {
	query := `
		SELECT id, ip_address, hostname, mac_address, vendor, os_family, os_name, os_version,
			os_confidence, os_detected_at, os_method, os_details, discovery_method, response_time_ms,
			discovery_count, ignore_scanning, first_seen, last_seen, status
		FROM hosts
		WHERE ip_address = $1`

	host := &Host{}
	err := tx.QueryRowContext(ctx, query, ipAddress).Scan(
		&host.ID, &host.IPAddress, &host.Hostname, &host.MACAddress, &host.Vendor,
		&host.OSFamily, &host.OSName, &host.OSVersion, &host.OSConfidence, &host.OSDetectedAt,
		&host.OSMethod, &host.OSDetails, &host.DiscoveryMethod, &host.ResponseTimeMS,
		&host.DiscoveryCount, &host.IgnoreScanning, &host.FirstSeen, &host.LastSeen, &host.Status)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("failed to get host by IP: %w", err)
	}

	return host, nil
}

// UpsertInTransaction creates or updates a host within an existing transaction.
func (r *HostRepository) UpsertInTransaction(ctx context.Context, tx *sqlx.Tx, host *Host) error {
	if host.ID == uuid.Nil {
		host.ID = uuid.New()
	}

	query := `
		INSERT INTO hosts (id, ip_address, hostname, mac_address, vendor, os_family, os_name, os_version,
			os_confidence, os_detected_at, os_method, os_details, discovery_method, response_time_ms,
			discovery_count, ignore_scanning, first_seen, last_seen, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		ON CONFLICT (ip_address)
		DO UPDATE SET
			hostname = COALESCE(EXCLUDED.hostname, hosts.hostname),
			mac_address = COALESCE(EXCLUDED.mac_address, hosts.mac_address),
			vendor = COALESCE(EXCLUDED.vendor, hosts.vendor),
			os_family = COALESCE(EXCLUDED.os_family, hosts.os_family),
			os_name = COALESCE(EXCLUDED.os_name, hosts.os_name),
			os_version = COALESCE(EXCLUDED.os_version, hosts.os_version),
			os_confidence = COALESCE(EXCLUDED.os_confidence, hosts.os_confidence),
			os_detected_at = COALESCE(EXCLUDED.os_detected_at, hosts.os_detected_at),
			os_method = COALESCE(EXCLUDED.os_method, hosts.os_method),
			os_details = COALESCE(EXCLUDED.os_details, hosts.os_details),
			discovery_method = COALESCE(EXCLUDED.discovery_method, hosts.discovery_method),
			response_time_ms = COALESCE(EXCLUDED.response_time_ms, hosts.response_time_ms),
			discovery_count = hosts.discovery_count + 1,
			ignore_scanning = COALESCE(EXCLUDED.ignore_scanning, hosts.ignore_scanning),
			last_seen = EXCLUDED.last_seen,
			status = EXCLUDED.status
		RETURNING first_seen, last_seen`

	err := tx.QueryRowContext(ctx, query,
		host.ID, host.IPAddress, host.Hostname, host.MACAddress, host.Vendor,
		host.OSFamily, host.OSName, host.OSVersion, host.OSConfidence, host.OSDetectedAt,
		host.OSMethod, host.OSDetails, host.DiscoveryMethod, host.ResponseTimeMS,
		host.DiscoveryCount, host.IgnoreScanning, host.FirstSeen, host.LastSeen, host.Status).Scan(
		&host.FirstSeen, &host.LastSeen)
	if err != nil {
		return fmt.Errorf("failed to upsert host: %w", err)
	}

	return nil
}

// PortScanRepository handles port scan operations.
type PortScanRepository struct {
	db *DB
}

// NewHostRepository creates a new host repository.
func NewHostRepository(db *DB) *HostRepository {
	return &HostRepository{db: db}
}

// CreateOrUpdate creates a new host or updates existing one.
func (r *HostRepository) CreateOrUpdate(ctx context.Context, host *Host) error {
	query := `
		INSERT INTO hosts (
			id, ip_address, hostname, mac_address, vendor,
			os_family, os_version, status, discovery_method,
			response_time_ms, discovery_count
		)
		VALUES (
			:id, :ip_address, :hostname, :mac_address, :vendor,
			:os_family, :os_version, :status, :discovery_method,
			:response_time_ms, :discovery_count
		)
		ON CONFLICT (ip_address)
		DO UPDATE SET
			hostname = COALESCE(EXCLUDED.hostname, hosts.hostname),
			mac_address = COALESCE(EXCLUDED.mac_address, hosts.mac_address),
			vendor = COALESCE(EXCLUDED.vendor, hosts.vendor),
			os_family = COALESCE(EXCLUDED.os_family, hosts.os_family),
			os_version = COALESCE(EXCLUDED.os_version, hosts.os_version),
			status = EXCLUDED.status,
			discovery_method = COALESCE(EXCLUDED.discovery_method, hosts.discovery_method),
			response_time_ms = COALESCE(EXCLUDED.response_time_ms, hosts.response_time_ms),
			discovery_count = COALESCE(EXCLUDED.discovery_count, hosts.discovery_count),
			last_seen = NOW()
		RETURNING id, first_seen, last_seen`

	if host.ID == uuid.Nil {
		host.ID = uuid.New()
	}

	rows, err := r.db.NamedQueryContext(ctx, query, host)
	if err != nil {
		return fmt.Errorf("failed to create or update host: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	if rows.Next() {
		if err := rows.Scan(&host.ID, &host.FirstSeen, &host.LastSeen); err != nil {
			return fmt.Errorf("failed to scan created/updated host: %w", err)
		}
	}

	return nil
}

// GetByIP retrieves a host by IP address.
func (r *HostRepository) GetByIP(ctx context.Context, ip IPAddr) (*Host, error) {
	var host Host
	query := `SELECT * FROM hosts WHERE ip_address = $1`

	if err := r.db.GetContext(ctx, &host, query, ip); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("host not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get host: %w", err)
	}

	return &host, nil
}

// GetActiveHosts retrieves all active hosts.
func (r *HostRepository) GetActiveHosts(ctx context.Context) ([]*ActiveHost, error) {
	var hosts []*ActiveHost
	query := `SELECT * FROM active_hosts ORDER BY ip_address`

	if err := r.db.SelectContext(ctx, &hosts, query); err != nil {
		return nil, fmt.Errorf("failed to get active hosts: %w", err)
	}

	return hosts, nil
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
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Verify all host_ids exist to prevent foreign key constraint violations
	hostIDs := make(map[uuid.UUID]bool)
	for _, scan := range scans {
		hostIDs[scan.HostID] = true
	}

	for hostID := range hostIDs {
		var exists bool
		verifyQuery := `SELECT EXISTS(SELECT 1 FROM hosts WHERE id = $1)`
		err := tx.QueryRowContext(ctx, verifyQuery, hostID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to verify host existence for %s: %w", hostID, err)
		}
		if !exists {
			return fmt.Errorf("host %s does not exist, cannot create port scans", hostID)
		}
	}

	// Verify all job_ids exist to prevent foreign key constraint violations
	jobIDs := make(map[uuid.UUID]bool)
	for _, scan := range scans {
		jobIDs[scan.JobID] = true
	}

	for jobID := range jobIDs {
		var exists bool
		verifyQuery := `SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE id = $1)`
		err := tx.QueryRowContext(ctx, verifyQuery, jobID).Scan(&exists)
		if err != nil {
			return fmt.Errorf("failed to verify job existence for %s: %w", jobID, err)
		}
		if !exists {
			return fmt.Errorf("job %s does not exist, cannot create port scans", jobID)
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
			return fmt.Errorf("failed to create port scan: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetByHost retrieves all port scans for a host.
func (r *PortScanRepository) GetByHost(ctx context.Context, hostID uuid.UUID) ([]*PortScan, error) {
	var scans []*PortScan
	query := `SELECT * FROM port_scans WHERE host_id = $1 ORDER BY port`

	if err := r.db.SelectContext(ctx, &scans, query, hostID); err != nil {
		return nil, fmt.Errorf("failed to get port scans: %w", err)
	}

	return scans, nil
}

// NetworkSummaryRepository handles network summary operations.
type NetworkSummaryRepository struct {
	db *DB
}

// NewNetworkSummaryRepository creates a new network summary repository.
func NewNetworkSummaryRepository(db *DB) *NetworkSummaryRepository {
	return &NetworkSummaryRepository{db: db}
}

// GetAll retrieves network summary for all targets.
func (r *NetworkSummaryRepository) GetAll(ctx context.Context) ([]*NetworkSummary, error) {
	var summaries []*NetworkSummary
	query := `SELECT * FROM network_summary ORDER BY target_name`

	if err := r.db.SelectContext(ctx, &summaries, query); err != nil {
		return nil, fmt.Errorf("failed to get network summaries: %w", err)
	}

	return summaries, nil
}

// Close closes the database connection.
func (db *DB) Close() error {
	return db.DB.Close()
}

// Ping tests the database connection.
func (db *DB) Ping(ctx context.Context) error {
	return db.PingContext(ctx)
}

// BeginTx starts a new transaction.
func (db *DB) BeginTx(ctx context.Context) (*sqlx.Tx, error) {
	return db.BeginTxx(ctx, nil)
}
