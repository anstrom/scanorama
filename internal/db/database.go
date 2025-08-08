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

	"github.com/anstrom/scanorama/internal/errors"
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

// HostRepository handles host operations.
type HostRepository struct {
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

// Scan represents a scan configuration and execution state.
type Scan struct {
	ID          uuid.UUID              `json:"id" db:"id"`
	Name        string                 `json:"name" db:"name"`
	Description string                 `json:"description" db:"description"`
	Targets     []string               `json:"targets" db:"targets"`
	ScanType    string                 `json:"scan_type" db:"scan_type"`
	Ports       string                 `json:"ports" db:"ports"`
	ProfileID   *int64                 `json:"profile_id" db:"profile_id"`
	Options     map[string]interface{} `json:"options" db:"options"`
	ScheduleID  *int64                 `json:"schedule_id" db:"schedule_id"`
	Tags        []string               `json:"tags" db:"tags"`
	Status      string                 `json:"status" db:"status"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
	StartedAt   *time.Time             `json:"started_at" db:"started_at"`
	CompletedAt *time.Time             `json:"completed_at" db:"completed_at"`
}

// ScanFilters represents filters for listing scans.
type ScanFilters struct {
	Status    string
	ScanType  string
	ProfileID *int64
	Tags      []string
}

// ScanResult represents a scan result entry.
type ScanResult struct {
	ID       uuid.UUID `json:"id" db:"id"`
	ScanID   uuid.UUID `json:"scan_id" db:"scan_id"`
	HostID   uuid.UUID `json:"host_id" db:"host_id"`
	Port     int       `json:"port" db:"port"`
	Protocol string    `json:"protocol" db:"protocol"`
	State    string    `json:"state" db:"state"`
	Service  string    `json:"service" db:"service"`
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

// ListScans retrieves scans with filtering and pagination.
func (db *DB) ListScans(ctx context.Context, filters ScanFilters, offset, limit int) ([]*Scan, int64, error) {
	// For now, return empty results - this would normally query scan_jobs table
	// with appropriate joins and filtering
	scans := []*Scan{}
	total := int64(0)
	return scans, total, nil
}

// CreateScan creates a new scan record.
func (db *DB) CreateScan(ctx context.Context, scanData interface{}) (*Scan, error) {
	// Convert the interface{} to scan data
	data, ok := scanData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid scan data format")
	}

	scan := &Scan{
		ID:          uuid.New(),
		Name:        data["name"].(string),
		Description: data["description"].(string),
		Status:      "pending",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	// For now, just return the scan object - this would normally insert into database
	return scan, nil
}

// GetScan retrieves a scan by ID.
func (db *DB) GetScan(ctx context.Context, id uuid.UUID) (*Scan, error) {
	// For now, return not found error - this would normally query the database
	return nil, errors.ErrNotFoundWithID("scan", id.String())
}

// UpdateScan updates an existing scan.
func (db *DB) UpdateScan(ctx context.Context, id uuid.UUID, scanData interface{}) (*Scan, error) {
	// For now, return not found error - this would normally update the database
	return nil, errors.ErrNotFoundWithID("scan", id.String())
}

// DeleteScan deletes a scan by ID.
func (db *DB) DeleteScan(ctx context.Context, id uuid.UUID) error {
	// For now, return not found error - this would normally delete from database
	return errors.ErrNotFoundWithID("scan", id.String())
}

// GetScanResults retrieves scan results with pagination.
func (db *DB) GetScanResults(ctx context.Context, scanID uuid.UUID, offset, limit int) ([]*ScanResult, int64, error) {
	// For now, return empty results - this would normally query scan results
	results := []*ScanResult{}
	total := int64(0)
	return results, total, nil
}

// GetScanSummary retrieves aggregated scan statistics.
func (db *DB) GetScanSummary(ctx context.Context, scanID uuid.UUID) (*ScanSummary, error) {
	// For now, return empty summary - this would normally aggregate results
	summary := &ScanSummary{
		ScanID:      scanID,
		TotalHosts:  0,
		TotalPorts:  0,
		OpenPorts:   0,
		ClosedPorts: 0,
		Duration:    0,
	}
	return summary, nil
}

// StartScan starts scan execution.
func (db *DB) StartScan(ctx context.Context, id uuid.UUID) error {
	// For now, return not found error - this would normally update scan status
	return errors.ErrNotFoundWithID("scan", id.String())
}

// StopScan stops scan execution.
func (db *DB) StopScan(ctx context.Context, id uuid.UUID) error {
	// For now, return not found error - this would normally update scan status
	return errors.ErrNotFoundWithID("scan", id.String())
}

// DiscoveryFilters represents filters for listing discovery jobs.
type DiscoveryFilters struct {
	Status string
	Method string
}

// ListDiscoveryJobs retrieves discovery jobs with filtering and pagination.
func (db *DB) ListDiscoveryJobs(
	ctx context.Context,
	filters DiscoveryFilters,
	offset, limit int,
) ([]*DiscoveryJob, int64, error) {
	jobs := []*DiscoveryJob{}
	total := int64(0)
	return jobs, total, nil
}

// CreateDiscoveryJob creates a new discovery job.
func (db *DB) CreateDiscoveryJob(ctx context.Context, jobData interface{}) (*DiscoveryJob, error) {
	_, ok := jobData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid discovery job data format")
	}

	job := &DiscoveryJob{
		ID:        uuid.New(),
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
	}

	return job, nil
}

// GetDiscoveryJob retrieves a discovery job by ID.
func (db *DB) GetDiscoveryJob(ctx context.Context, id uuid.UUID) (*DiscoveryJob, error) {
	return nil, errors.ErrNotFoundWithID("discovery job", id.String())
}

// UpdateDiscoveryJob updates an existing discovery job.
func (db *DB) UpdateDiscoveryJob(ctx context.Context, id uuid.UUID, jobData interface{}) (*DiscoveryJob, error) {
	return nil, errors.ErrNotFoundWithID("discovery job", id.String())
}

// DeleteDiscoveryJob deletes a discovery job by ID.
func (db *DB) DeleteDiscoveryJob(ctx context.Context, id uuid.UUID) error {
	return errors.ErrNotFoundWithID("discovery job", id.String())
}

// StartDiscoveryJob starts discovery job execution.
func (db *DB) StartDiscoveryJob(ctx context.Context, id uuid.UUID) error {
	return errors.ErrNotFoundWithID("discovery job", id.String())
}

// StopDiscoveryJob stops discovery job execution.
func (db *DB) StopDiscoveryJob(ctx context.Context, id uuid.UUID) error {
	return errors.ErrNotFoundWithID("discovery job", id.String())
}

// HostFilters represents filters for listing hosts.
type HostFilters struct {
	Status   string
	OSFamily string
	Network  string
}

// ListHosts retrieves hosts with filtering and pagination.
func (db *DB) ListHosts(ctx context.Context, filters HostFilters, offset, limit int) ([]*Host, int64, error) {
	hosts := []*Host{}
	total := int64(0)
	return hosts, total, nil
}

// CreateHost creates a new host record.
func (db *DB) CreateHost(ctx context.Context, hostData interface{}) (*Host, error) {
	_, ok := hostData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid host data format")
	}

	host := &Host{
		ID:        uuid.New(),
		FirstSeen: time.Now().UTC(),
		LastSeen:  time.Now().UTC(),
	}

	return host, nil
}

// GetHost retrieves a host by ID.
func (db *DB) GetHost(ctx context.Context, id uuid.UUID) (*Host, error) {
	return nil, errors.ErrNotFoundWithID("host", id.String())
}

// UpdateHost updates an existing host.
func (db *DB) UpdateHost(ctx context.Context, id uuid.UUID, hostData interface{}) (*Host, error) {
	return nil, errors.ErrNotFoundWithID("host", id.String())
}

// DeleteHost deletes a host by ID.
func (db *DB) DeleteHost(ctx context.Context, id uuid.UUID) error {
	return errors.ErrNotFoundWithID("host", id.String())
}

// GetHostScans retrieves scans for a specific host with pagination.
func (db *DB) GetHostScans(ctx context.Context, hostID uuid.UUID, offset, limit int) ([]*Scan, int64, error) {
	scans := []*Scan{}
	total := int64(0)
	return scans, total, nil
}

// Profile represents a scan profile.
type Profile struct {
	ID          uuid.UUID              `json:"id" db:"id"`
	Name        string                 `json:"name" db:"name"`
	Description string                 `json:"description" db:"description"`
	ScanType    string                 `json:"scan_type" db:"scan_type"`
	Ports       string                 `json:"ports" db:"ports"`
	Options     map[string]interface{} `json:"options" db:"options"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at" db:"updated_at"`
}

// ProfileFilters represents filters for listing profiles.
type ProfileFilters struct {
	ScanType string
}

// ListProfiles retrieves profiles with filtering and pagination.
func (db *DB) ListProfiles(ctx context.Context, filters ProfileFilters, offset, limit int) ([]*Profile, int64, error) {
	profiles := []*Profile{}
	total := int64(0)
	return profiles, total, nil
}

// CreateProfile creates a new profile.
func (db *DB) CreateProfile(ctx context.Context, profileData interface{}) (*Profile, error) {
	_, ok := profileData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid profile data format")
	}

	profile := &Profile{
		ID:        uuid.New(),
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	return profile, nil
}

// GetProfile retrieves a profile by ID.
func (db *DB) GetProfile(ctx context.Context, id uuid.UUID) (*Profile, error) {
	return nil, errors.ErrNotFoundWithID("profile", id.String())
}

// UpdateProfile updates an existing profile.
func (db *DB) UpdateProfile(ctx context.Context, id uuid.UUID, profileData interface{}) (*Profile, error) {
	return nil, errors.ErrNotFoundWithID("profile", id.String())
}

// DeleteProfile deletes a profile by ID.
func (db *DB) DeleteProfile(ctx context.Context, id uuid.UUID) error {
	return errors.ErrNotFoundWithID("profile", id.String())
}

// Schedule represents a scheduled task.
type Schedule struct {
	ID             uuid.UUID              `json:"id" db:"id"`
	Name           string                 `json:"name" db:"name"`
	Description    string                 `json:"description" db:"description"`
	CronExpression string                 `json:"cron_expression" db:"cron_expression"`
	JobType        string                 `json:"job_type" db:"job_type"`
	JobConfig      map[string]interface{} `json:"job_config" db:"job_config"`
	Enabled        bool                   `json:"enabled" db:"enabled"`
	CreatedAt      time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at" db:"updated_at"`
	LastRun        *time.Time             `json:"last_run" db:"last_run"`
	NextRun        *time.Time             `json:"next_run" db:"next_run"`
}

// ScheduleFilters represents filters for listing schedules.
type ScheduleFilters struct {
	Enabled bool
	JobType string
}

// ListSchedules retrieves schedules with filtering and pagination.
func (db *DB) ListSchedules(
	ctx context.Context,
	filters ScheduleFilters,
	offset, limit int,
) ([]*Schedule, int64, error) {
	schedules := []*Schedule{}
	total := int64(0)
	return schedules, total, nil
}

// CreateSchedule creates a new schedule.
func (db *DB) CreateSchedule(ctx context.Context, scheduleData interface{}) (*Schedule, error) {
	_, ok := scheduleData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid schedule data format")
	}

	schedule := &Schedule{
		ID:        uuid.New(),
		Enabled:   true,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	return schedule, nil
}

// GetSchedule retrieves a schedule by ID.
func (db *DB) GetSchedule(ctx context.Context, id uuid.UUID) (*Schedule, error) {
	return nil, errors.ErrNotFoundWithID("schedule", id.String())
}

// UpdateSchedule updates an existing schedule.
func (db *DB) UpdateSchedule(ctx context.Context, id uuid.UUID, scheduleData interface{}) (*Schedule, error) {
	return nil, errors.ErrNotFoundWithID("schedule", id.String())
}

// DeleteSchedule deletes a schedule by ID.
func (db *DB) DeleteSchedule(ctx context.Context, id uuid.UUID) error {
	return errors.ErrNotFoundWithID("schedule", id.String())
}

// EnableSchedule enables a schedule.
func (db *DB) EnableSchedule(ctx context.Context, id uuid.UUID) error {
	return errors.ErrNotFoundWithID("schedule", id.String())
}

// DisableSchedule disables a schedule.
func (db *DB) DisableSchedule(ctx context.Context, id uuid.UUID) error {
	return errors.ErrNotFoundWithID("schedule", id.String())
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
