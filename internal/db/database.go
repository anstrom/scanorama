// Package db provides database connectivity and data models for scanorama.
// It handles database migrations, host management, scan results storage,
// and provides the core data access layer for the application.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"github.com/anstrom/scanorama/internal/errors"
)

// sanitizeDBError converts raw database errors into safe, sanitized errors
// that don't expose internal SQL details or credentials to API clients.
// The original error is preserved in the Cause field for internal debugging.
func sanitizeDBError(operation string, err error) error {
	if err == nil {
		return nil
	}

	// Handle specific known database errors
	if err == sql.ErrNoRows {
		return errors.NewDatabaseError(errors.CodeNotFound, "Resource not found")
	}

	// Check for PostgreSQL-specific errors
	if pqErr, ok := err.(*pq.Error); ok {
		var dbErr *errors.DatabaseError
		switch pqErr.Code {
		case "23505": // unique_violation
			dbErr = errors.NewDatabaseError(errors.CodeConflict, "Resource already exists")
		case "23503": // foreign_key_violation
			dbErr = errors.NewDatabaseError(errors.CodeValidation, "Referenced resource does not exist")
		case "23502": // not_null_violation
			dbErr = errors.NewDatabaseError(errors.CodeValidation, "Required field is missing")
		case "23514": // check_violation
			dbErr = errors.NewDatabaseError(errors.CodeValidation, "Data validation failed")
		case "57014": // query_canceled
			dbErr = errors.NewDatabaseError(errors.CodeCanceled, "Database operation was canceled")
		case "57P01": // admin_shutdown
			dbErr = errors.NewDatabaseError(errors.CodeDatabaseConnection, "Database connection lost")
		case "08000", "08003", "08006": // connection errors
			dbErr = errors.NewDatabaseError(errors.CodeDatabaseConnection, "Database connection error")
		default:
			// Unknown PostgreSQL error - use generic sanitized error
			msg := fmt.Sprintf("Database operation failed: %s", operation)
			dbErr = errors.NewDatabaseError(errors.CodeDatabaseQuery, msg)
		}
		// Preserve original error for internal logging
		dbErr.Operation = operation
		dbErr.Cause = err
		return dbErr
	}

	// For all other errors, return a generic sanitized error without details
	dbErr := errors.NewDatabaseError(errors.CodeDatabaseQuery, fmt.Sprintf("Database operation failed: %s", operation))
	dbErr.Operation = operation
	// Store the original error as Cause for internal logging, but it won't be exposed to API
	dbErr.Cause = err
	return dbErr
}

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
		SSLMode:         "disable",
		MaxOpenConns:    defaultMaxOpenConns,
		MaxIdleConns:    defaultMaxIdleConns,
		ConnMaxLifetime: defaultConnMaxLifetime * time.Minute,
		ConnMaxIdleTime: defaultConnMaxIdleTime * time.Minute,
	}
}

// Connect establishes a connection to PostgreSQL.
// Returns sanitized errors that don't leak credentials or DSN details.
func Connect(ctx context.Context, config *Config) (*DB, error) {
	// Build DSN - PostgreSQL lib/pq handles special characters in values correctly
	// when using key=value format (values with spaces/special chars are auto-escaped)
	dsn := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		config.Host, config.Port, config.Database,
		config.Username, config.Password, config.SSLMode,
	)

	db, err := sqlx.ConnectContext(ctx, "postgres", dsn)
	if err != nil {
		// Return sanitized error without DSN to prevent credential leakage in logs
		return nil, errors.ErrDatabaseConnection(err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)
	db.SetConnMaxIdleTime(config.ConnMaxIdleTime)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		// Close the connection before returning error
		if closeErr := db.Close(); closeErr != nil {
			// Don't log raw error - it might contain connection details
			log.Printf("Failed to close database connection after ping failure")
		}
		return nil, errors.WrapDatabaseError(errors.CodeDatabaseConnection, "Failed to verify database connection", err)
	}

	// Log success without credentials - only safe connection details
	log.Printf("Successfully connected to database at %s:%d/%s", config.Host, config.Port, config.Database)
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
		return sanitizeDBError("create scan target", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	if rows.Next() {
		if err := rows.Scan(&target.CreatedAt, &target.UpdatedAt); err != nil {
			return sanitizeDBError("scan created scan target", err)
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
			return nil, sanitizeDBError("get scan target", err)
		}
		return nil, sanitizeDBError("get scan target", err)
	}

	return &target, nil
}

// GetAll retrieves all scan targets.
func (r *ScanTargetRepository) GetAll(ctx context.Context) ([]*ScanTarget, error) {
	var targets []*ScanTarget
	query := `SELECT * FROM scan_targets ORDER BY name`

	if err := r.db.SelectContext(ctx, &targets, query); err != nil {
		return nil, sanitizeDBError("get scan targets", err)
	}

	return targets, nil
}

// GetEnabled retrieves all enabled scan targets.
func (r *ScanTargetRepository) GetEnabled(ctx context.Context) ([]*ScanTarget, error) {
	var targets []*ScanTarget
	query := `SELECT * FROM scan_targets WHERE enabled = true ORDER BY name`

	if err := r.db.SelectContext(ctx, &targets, query); err != nil {
		return nil, sanitizeDBError("get enabled scan targets", err)
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
		return sanitizeDBError("update scan target", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	if rows.Next() {
		if err := rows.Scan(&target.UpdatedAt); err != nil {
			return sanitizeDBError("scan updated scan target", err)
		}
	}

	return nil
}

// Delete deletes a scan target.
func (r *ScanTargetRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `DELETE FROM scan_targets WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return sanitizeDBError("delete scan target", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return sanitizeDBError("get rows affected", err)
	}

	if rowsAffected == 0 {
		return errors.NewDatabaseError(errors.CodeNotFound, "Scan target not found")
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
		return sanitizeDBError("create scan job", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
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
		return sanitizeDBError("update scan job status", err)
	}

	return nil
}

// GetByID retrieves a scan job by ID.
func (r *ScanJobRepository) GetByID(ctx context.Context, id uuid.UUID) (*ScanJob, error) {
	var job ScanJob
	query := `SELECT * FROM scan_jobs WHERE id = $1`

	if err := r.db.GetContext(ctx, &job, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, sanitizeDBError("get scan job", err)
		}
		return nil, sanitizeDBError("get scan job", err)
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
		return sanitizeDBError("create or update host", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	if rows.Next() {
		if err := rows.Scan(&host.FirstSeen, &host.LastSeen); err != nil {
			return sanitizeDBError("scan created/updated host", err)
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
			return nil, sanitizeDBError("get host", err)
		}
		return nil, sanitizeDBError("get host", err)
	}

	return &host, nil
}

// GetActiveHosts retrieves all active hosts.
func (r *HostRepository) GetActiveHosts(ctx context.Context) ([]*ActiveHost, error) {
	var hosts []*ActiveHost
	query := `SELECT * FROM active_hosts ORDER BY ip_address`

	if err := r.db.SelectContext(ctx, &hosts, query); err != nil {
		return nil, sanitizeDBError("get active hosts", err)
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
		return sanitizeDBError("begin transaction", err)
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
	query := `SELECT * FROM port_scans WHERE host_id = $1 ORDER BY port`

	if err := r.db.SelectContext(ctx, &scans, query, hostID); err != nil {
		return nil, sanitizeDBError("get port scans", err)
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
		return nil, sanitizeDBError("get network summaries", err)
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
	ID        uuid.UUID `json:"id" db:"id"`
	ScanID    uuid.UUID `json:"scan_id" db:"scan_id"`
	HostID    uuid.UUID `json:"host_id" db:"host_id"`
	Port      int       `json:"port" db:"port"`
	Protocol  string    `json:"protocol" db:"protocol"`
	State     string    `json:"state" db:"state"`
	Service   string    `json:"service" db:"service"`
	ScannedAt time.Time `json:"scanned_at" db:"scanned_at"`
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
// filterCondition represents a single filter condition
type filterCondition struct {
	column string
	value  interface{}
}

// buildWhereClause creates WHERE clause and args from conditions map
func buildWhereClause(conditions []filterCondition) (whereClause string, args []interface{}) {
	if len(conditions) == 0 {
		return "", nil
	}

	clauses := make([]string, 0, len(conditions))
	for i, condition := range conditions {
		clauses = append(clauses, fmt.Sprintf("%s = $%d", condition.column, i+1))
		args = append(args, condition.value)
	}

	return "WHERE " + strings.Join(clauses, " AND "), args
}

// buildScanFilters creates WHERE clause and args for scan filtering
func buildScanFilters(filters ScanFilters) (whereClause string, args []interface{}) {
	var conditions []filterCondition

	if filters.Status != "" {
		conditions = append(conditions, filterCondition{"sj.status", filters.Status})
	}
	if filters.ScanType != "" {
		conditions = append(conditions, filterCondition{"COALESCE(sp.scan_type, 'connect')", filters.ScanType})
	}
	if filters.ProfileID != nil {
		conditions = append(conditions, filterCondition{"sj.profile_id", *filters.ProfileID})
	}

	return buildWhereClause(conditions)
}

// getScanCount gets total count of scans matching filters
func (db *DB) getScanCount(ctx context.Context, whereClause string, args []interface{}) (int64, error) {
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM scan_jobs sj
		JOIN scan_targets st ON sj.target_id = st.id
		LEFT JOIN scan_profiles sp ON sj.profile_id = sp.id %s`, whereClause)

	var total int64
	err := db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get scan count: %w", err)
	}
	return total, nil
}

// processScanRow processes a single scan row from query results
func processScanRow(rows *sql.Rows) (*Scan, error) {
	scan := &Scan{}
	var targetsStr string
	var profileID *string
	var description sql.NullString

	err := rows.Scan(
		&scan.ID,
		&scan.Name,
		&description,
		&targetsStr,
		&scan.ScanType,
		&scan.Ports,
		&profileID,
		&scan.Status,
		&scan.CreatedAt,
		&scan.StartedAt,
		&scan.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	// Handle nullable description
	if description.Valid {
		scan.Description = description.String
	} else {
		scan.Description = ""
	}

	// Parse targets from CIDR string
	scan.Targets = []string{targetsStr}

	// Set ProfileID if not null
	if profileID != nil {
		if id, err := strconv.ParseInt(*profileID, 10, 64); err == nil {
			scan.ProfileID = &id
		}
	}

	// Set UpdatedAt (use CompletedAt if available, otherwise CreatedAt)
	if scan.CompletedAt != nil {
		scan.UpdatedAt = *scan.CompletedAt
	} else if scan.StartedAt != nil {
		scan.UpdatedAt = *scan.StartedAt
	} else {
		scan.UpdatedAt = scan.CreatedAt
	}

	return scan, nil
}

func (db *DB) ListScans(ctx context.Context, filters ScanFilters, offset, limit int) ([]*Scan, int64, error) {
	// Build the base query with joins
	baseQuery := `
		SELECT
			sj.id,
			st.name,
			st.description,
			st.network::text as targets,
			COALESCE(sp.scan_type, 'connect') as scan_type,
			st.scan_ports as ports,
			sj.profile_id,
			sj.status,
			sj.created_at,
			sj.started_at,
			sj.completed_at
		FROM scan_jobs sj
		JOIN scan_targets st ON sj.target_id = st.id
		LEFT JOIN scan_profiles sp ON sj.profile_id = sp.id
	`

	// Build WHERE clause and arguments
	whereClause, args := buildScanFilters(filters)

	// Get total count
	total, err := db.getScanCount(ctx, whereClause, args)
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results
	argIndex := len(args)
	listQuery := fmt.Sprintf("%s %s ORDER BY sj.created_at DESC LIMIT $%d OFFSET $%d",
		baseQuery, whereClause, argIndex+1, argIndex+2)
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query scans: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var scans []*Scan
	for rows.Next() {
		scan, err := processScanRow(rows)
		if err != nil {
			return nil, 0, err
		}
		scans = append(scans, scan)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate scan rows: %w", err)
	}

	return scans, total, nil
}

// scanData holds extracted scan parameters
type scanData struct {
	name        string
	description string
	scanType    string
	ports       string
	targets     []string
	profileID   *string
}

// extractScanData extracts and validates scan data from interface
func extractScanData(input interface{}) (*scanData, error) {
	data, ok := input.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid scan data format")
	}

	targets, ok := data["targets"].([]string)
	if !ok {
		return nil, fmt.Errorf("targets must be a string array")
	}

	result := &scanData{
		name:     data["name"].(string),
		scanType: data["scan_type"].(string),
		targets:  targets,
	}

	// Use map-based extraction for optional fields
	optionalFields := map[string]interface{}{
		"description": &result.description,
		"ports":       &result.ports,
	}

	for key, dest := range optionalFields {
		if value, exists := data[key].(string); exists {
			*dest.(*string) = value
		}
	}

	if pid, ok := data["profile_id"].(*int64); ok && pid != nil {
		pidStr := strconv.FormatInt(*pid, 10)
		result.profileID = &pidStr
	}

	return result, nil
}

// createScanTarget creates a scan target in the database
func (db *DB) createScanTarget(ctx context.Context, tx *sql.Tx, targetID uuid.UUID,
	name, target, description, ports, scanType string, index, totalTargets int) error {
	var targetName string
	if totalTargets == 1 {
		targetName = name
	} else {
		targetName = fmt.Sprintf("%s-target-%d", name, index+1)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO scan_targets (id, name, network, description, scan_ports, scan_type, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, true)
	`, targetID, targetName, target, description, ports, scanType)
	if err != nil {
		return fmt.Errorf("failed to create scan target: %w", err)
	}
	return nil
}

// createScanJob creates a scan job in the database
func (db *DB) createScanJob(ctx context.Context, tx *sql.Tx, jobID, targetID uuid.UUID,
	profileID *string, now time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO scan_jobs (id, target_id, profile_id, status, created_at)
		VALUES ($1, $2, $3, 'pending', $4)
	`, jobID, targetID, profileID, now)
	if err != nil {
		return fmt.Errorf("failed to create scan job: %w", err)
	}
	return nil
}

// buildScanResponse builds the scan response object
func buildScanResponse(jobID uuid.UUID, name, description string, targets []string,
	scanType, ports string, profileID *string, now time.Time) *Scan {
	scan := &Scan{
		ID:          jobID,
		Name:        name,
		Description: description,
		Targets:     targets,
		ScanType:    scanType,
		Ports:       ports,
		Status:      "pending",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if profileID != nil {
		if id, err := strconv.ParseInt(*profileID, 10, 64); err == nil {
			scan.ProfileID = &id
		}
	}

	return scan
}

// CreateScan creates a new scan record.
func (db *DB) CreateScan(ctx context.Context, input interface{}) (*Scan, error) {
	// Extract and validate data
	data, err := extractScanData(input)
	if err != nil {
		return nil, err
	}

	// Start a transaction
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	now := time.Now().UTC()
	var firstJobID uuid.UUID

	// Create scan targets and jobs
	for i, target := range data.targets {
		targetID := uuid.New()
		jobID := uuid.New()

		if i == 0 {
			firstJobID = jobID
		}

		// Create scan target
		if err := db.createScanTarget(ctx, tx, targetID, data.name, target,
			data.description, data.ports, data.scanType, i, len(data.targets)); err != nil {
			return nil, err
		}

		// Create scan job
		if err := db.createScanJob(ctx, tx, jobID, targetID, data.profileID, now); err != nil {
			return nil, err
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Build and return response
	return buildScanResponse(firstJobID, data.name, data.description, data.targets,
		data.scanType, data.ports, data.profileID, now), nil
}

// GetScan retrieves a scan by ID.
func (db *DB) GetScan(ctx context.Context, id uuid.UUID) (*Scan, error) {
	query := `
		SELECT
			sj.id,
			st.name,
			st.description,
			st.network::text as targets,
			COALESCE(sp.scan_type, st.scan_type) as scan_type,
			st.scan_ports as ports,
			sj.profile_id,
			sj.status,
			sj.created_at,
			sj.started_at,
			sj.completed_at
		FROM scan_jobs sj
		JOIN scan_targets st ON sj.target_id = st.id
		LEFT JOIN scan_profiles sp ON sj.profile_id = sp.id
		WHERE sj.id = $1
	`

	scan := &Scan{}
	var targetsStr string
	var profileID *string

	err := db.DB.QueryRowContext(ctx, query, id).Scan(
		&scan.ID,
		&scan.Name,
		&scan.Description,
		&targetsStr,
		&scan.ScanType,
		&scan.Ports,
		&profileID,
		&scan.Status,
		&scan.CreatedAt,
		&scan.StartedAt,
		&scan.CompletedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.ErrNotFoundWithID("scan", id.String())
		}
		return nil, fmt.Errorf("failed to get scan: %w", err)
	}

	// Parse targets from CIDR string
	scan.Targets = []string{targetsStr}

	// Set ProfileID if not null
	if profileID != nil {
		if pid, err := strconv.ParseInt(*profileID, 10, 64); err == nil {
			scan.ProfileID = &pid
		}
	}

	// Set UpdatedAt (use CompletedAt if available, otherwise StartedAt, otherwise CreatedAt)
	if scan.CompletedAt != nil {
		scan.UpdatedAt = *scan.CompletedAt
	} else if scan.StartedAt != nil {
		scan.UpdatedAt = *scan.StartedAt
	} else {
		scan.UpdatedAt = scan.CreatedAt
	}

	return scan, nil
}

// UpdateScan updates an existing scan.
func (db *DB) UpdateScan(ctx context.Context, id uuid.UUID, scanData interface{}) (*Scan, error) {
	// Convert the interface{} to scan data
	data, ok := scanData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid scan data format")
	}

	// Start a transaction
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	// First, check if the scan exists
	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check scan existence: %w", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("scan", id.String())
	}

	// Build dynamic update for scan_targets using field mapping
	targetFieldMappings := map[string]string{
		"name":        "name",
		"description": "description",
		"scan_type":   "scan_type",
		"ports":       "scan_ports",
	}

	setParts, args := buildUpdateQuery(data, targetFieldMappings)

	// Update scan_targets if there are fields to update
	if len(setParts) > 0 {
		setParts = append(setParts, "updated_at = NOW()")
		setClause := strings.Join(setParts, ", ")
		paramNum := len(args) + 1

		// Build query safely without string concatenation
		var queryBuilder strings.Builder
		queryBuilder.WriteString("UPDATE scan_targets SET ")
		queryBuilder.WriteString(setClause)
		queryBuilder.WriteString(" WHERE id = (SELECT target_id FROM scan_jobs WHERE id = $")
		queryBuilder.WriteString(strconv.Itoa(paramNum))
		queryBuilder.WriteString(")")
		targetQuery := queryBuilder.String()

		args = append(args, id)

		_, err = tx.ExecContext(ctx, targetQuery, args...)
		if err != nil {
			return nil, fmt.Errorf("failed to update scan target: %w", err)
		}
	}

	// Update profile_id in scan_jobs if provided
	if profileID, ok := data["profile_id"].(*int64); ok && profileID != nil {
		profileIDStr := strconv.FormatInt(*profileID, 10)
		_, err = tx.ExecContext(ctx, `
			UPDATE scan_jobs SET profile_id = $1 WHERE id = $2`,
			profileIDStr, id)
		if err != nil {
			return nil, fmt.Errorf("failed to update profile ID: %w", err)
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Retrieve and return the updated scan
	return db.GetScan(ctx, id)
}

// DeleteScan deletes a scan by ID.
func (db *DB) DeleteScan(ctx context.Context, id uuid.UUID) error {
	// Start a transaction
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	// Check if the scan exists
	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check scan existence: %w", err)
	}
	if !exists {
		return errors.ErrNotFoundWithID("scan", id.String())
	}

	// Get the target_id before deleting the scan job
	var targetID uuid.UUID
	err = tx.QueryRowContext(ctx, "SELECT target_id FROM scan_jobs WHERE id = $1", id).Scan(&targetID)
	if err != nil {
		return fmt.Errorf("failed to get target ID: %w", err)
	}

	// Delete the scan job (this will cascade to related port_scans, services, etc.)
	_, err = tx.ExecContext(ctx, "DELETE FROM scan_jobs WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete scan job: %w", err)
	}

	// Check if the target is still referenced by other jobs
	var otherJobsCount int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM scan_jobs WHERE target_id = $1", targetID).Scan(&otherJobsCount)
	if err != nil {
		return fmt.Errorf("failed to check remaining jobs for target: %w", err)
	}

	// If no other jobs reference this target, delete it too
	if otherJobsCount == 0 {
		_, err = tx.ExecContext(ctx, "DELETE FROM scan_targets WHERE id = $1", targetID)
		if err != nil {
			return fmt.Errorf("failed to delete scan target: %w", err)
		}
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetScanResults retrieves scan results with pagination.
func (db *DB) GetScanResults(ctx context.Context, scanID uuid.UUID, offset, limit int) ([]*ScanResult, int64, error) {
	// Get total count first
	var total int64
	countQuery := `
		SELECT COUNT(*)
		FROM port_scans ps
		WHERE ps.job_id = $1
	`
	err := db.DB.QueryRowContext(ctx, countQuery, scanID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get scan results count: %w", err)
	}

	// Get paginated results
	query := `
		SELECT
			ps.id,
			ps.job_id,
			ps.host_id,
			ps.port,
			ps.protocol,
			ps.state,
			ps.service_name,
			ps.scanned_at
		FROM port_scans ps
		WHERE ps.job_id = $1
		ORDER BY ps.scanned_at DESC, ps.port ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := db.QueryContext(ctx, query, scanID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query scan results: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var results []*ScanResult
	for rows.Next() {
		result := &ScanResult{}
		var serviceName *string

		err := rows.Scan(
			&result.ID,
			&result.ScanID,
			&result.HostID,
			&result.Port,
			&result.Protocol,
			&result.State,
			&serviceName,
			&result.ScannedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan result row: %w", err)
		}

		if serviceName != nil {
			result.Service = *serviceName
		}

		results = append(results, result)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate scan result rows: %w", err)
	}

	return results, total, nil
}

// GetScanSummary retrieves aggregated scan statistics.
func (db *DB) GetScanSummary(ctx context.Context, scanID uuid.UUID) (*ScanSummary, error) {
	query := `
		SELECT
			COUNT(DISTINCT ps.host_id) as total_hosts,
			COUNT(ps.id) as total_ports,
			COUNT(ps.id) FILTER (WHERE ps.state = 'open') as open_ports,
			COUNT(ps.id) FILTER (WHERE ps.state = 'closed') as closed_ports,
			EXTRACT(EPOCH FROM (MAX(sj.completed_at) - MIN(sj.started_at)))::integer as duration_seconds
		FROM port_scans ps
		JOIN scan_jobs sj ON ps.job_id = sj.id
		WHERE ps.job_id = $1
		GROUP BY ps.job_id
	`

	summary := &ScanSummary{
		ScanID: scanID,
	}

	var durationSeconds *int
	err := db.DB.QueryRowContext(ctx, query, scanID).Scan(
		&summary.TotalHosts,
		&summary.TotalPorts,
		&summary.OpenPorts,
		&summary.ClosedPorts,
		&durationSeconds,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			// No results for this scan, return zero summary
			return summary, nil
		}
		return nil, fmt.Errorf("failed to get scan summary: %w", err)
	}

	if durationSeconds != nil {
		summary.Duration = int64(*durationSeconds)
	}

	return summary, nil
}

// StartScan starts scan execution.
func (db *DB) StartScan(ctx context.Context, id uuid.UUID) error {
	// Update scan job status to running
	query := `
		UPDATE scan_jobs
		SET status = 'running', started_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`

	result, err := db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to start scan: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.ErrNotFoundWithID("scan", id.String())
	}

	return nil
}

// StopScan stops scan execution.
func (db *DB) StopScan(ctx context.Context, id uuid.UUID) error {
	// Update scan job status to failed (stopped)
	query := `
		UPDATE scan_jobs
		SET status = 'failed', completed_at = NOW()
		WHERE id = $1 AND status = 'running'
	`

	result, err := db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to stop scan: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.ErrNotFoundWithID("scan", id.String())
	}

	return nil
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
	data, ok := jobData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid discovery job data format")
	}

	// Extract data from request
	networks, ok := data["networks"].([]string)
	if !ok || len(networks) == 0 {
		return nil, fmt.Errorf("networks are required and must be a string array")
	}

	method := data["method"].(string)
	if method == "" {
		method = DiscoveryMethodTCP
	}

	// For simplicity, create one discovery job for the first network
	// In a production system, you might create multiple jobs or handle multiple networks differently
	network := networks[0]

	jobID := uuid.New()
	now := time.Now().UTC()

	query := `
		INSERT INTO discovery_jobs (id, network, method, status, created_at, hosts_discovered, hosts_responsive)
		VALUES ($1, $2, $3, 'pending', $4, 0, 0)
	`

	_, err := db.ExecContext(ctx, query, jobID, network, method, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery job: %w", err)
	}

	job := &DiscoveryJob{
		ID:              jobID,
		Network:         NetworkAddr{}, // Would need proper parsing
		Method:          method,
		Status:          "pending",
		CreatedAt:       now,
		HostsDiscovered: 0,
		HostsResponsive: 0,
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
	// Build the base query with joins
	baseQuery := `
		SELECT
			h.id,
			h.ip_address,
			h.hostname,
			h.mac_address,
			h.vendor,
			h.os_family,
			h.os_name,
			h.os_version,
			h.os_confidence,
			h.discovery_method,
			h.response_time_ms,
			h.ignore_scanning,
			h.first_seen,
			h.last_seen,
			h.status,
			COUNT(DISTINCT ps.id) FILTER (WHERE ps.state = 'open') as open_ports,
			COUNT(DISTINCT ps.id) as total_ports_scanned
		FROM hosts h
		LEFT JOIN port_scans ps ON h.id = ps.host_id
	`

	// Build WHERE clause and arguments
	whereClause, args := buildHostFilters(filters)

	// Get total count
	total, err := db.getHostCount(ctx, whereClause, args)
	if err != nil {
		return nil, 0, err
	}

	// Add GROUP BY clause
	groupByClause := `
		GROUP BY h.id, h.ip_address, h.hostname, h.mac_address, h.vendor, h.os_family,
			h.os_name, h.os_version, h.os_confidence, h.discovery_method,
			h.response_time_ms, h.ignore_scanning, h.first_seen, h.last_seen, h.status
	`

	// Combine query parts
	fullQuery := baseQuery + whereClause + groupByClause + " ORDER BY h.last_seen DESC LIMIT $" +
		fmt.Sprintf("%d", len(args)+1) + " OFFSET $" + fmt.Sprintf("%d", len(args)+2)

	args = append(args, limit, offset)

	// Execute query
	rows, err := db.QueryContext(ctx, fullQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to execute hosts query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			// Log the error but don't override the main function error
			log.Printf("failed to close rows: %v", err)
		}
	}()

	hosts, err := db.scanHostRows(rows)
	if err != nil {
		return nil, 0, err
	}

	return hosts, total, nil
}

// CreateHost creates a new host record.
func (db *DB) CreateHost(ctx context.Context, hostData interface{}) (*Host, error) {
	data, ok := hostData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid host data format")
	}

	// Extract required IP address
	ipAddress, ok := data["ip_address"].(string)
	if !ok {
		return nil, fmt.Errorf("ip_address is required")
	}

	hostID := uuid.New()
	now := time.Now().UTC()

	// Field mappings for optional fields
	fieldMappings := map[string]string{
		"hostname":        "hostname",
		"vendor":          "vendor",
		"os_family":       "os_family",
		"os_name":         "os_name",
		"os_version":      "os_version",
		"ignore_scanning": "ignore_scanning",
		"status":          "status",
	}

	// Build dynamic insert with optional fields
	columns := []string{"id", "ip_address", "first_seen", "last_seen"}
	placeholders := []string{"$1", "$2", "$3", "$4"}
	args := []interface{}{hostID, ipAddress, now, now}
	argIndex := 5

	for requestField, dbField := range fieldMappings {
		if value, exists := data[requestField]; exists && value != nil {
			columns = append(columns, dbField)
			placeholders = append(placeholders, fmt.Sprintf("$%d", argIndex))
			args = append(args, value)
			argIndex++
		}
	}

	query := fmt.Sprintf(
		"INSERT INTO hosts (%s) VALUES (%s)",
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	_, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		// Check for PostgreSQL constraint violations
		if pqErr, ok := err.(*pq.Error); ok {
			// Check for unique constraint violation (error code 23505)
			if pqErr.Code == "23505" && pqErr.Constraint == "unique_ip_address" {
				return nil, errors.ErrConflictWithReason("host", fmt.Sprintf("IP address %s already exists", ipAddress))
			}
		}
		return nil, fmt.Errorf("failed to create host: %w", err)
	}

	// Return the created host
	return db.GetHost(ctx, hostID)
}

// GetHost retrieves a host by ID.
func (db *DB) GetHost(ctx context.Context, id uuid.UUID) (*Host, error) {
	query := `
		SELECT
			h.id,
			h.ip_address,
			h.hostname,
			h.mac_address,
			h.vendor,
			h.os_family,
			h.os_name,
			h.os_version,
			h.os_confidence,
			h.discovery_method,
			h.response_time_ms,
			h.ignore_scanning,
			h.first_seen,
			h.last_seen,
			h.status
		FROM hosts h
		WHERE h.id = $1
	`

	host := &Host{}
	var ipAddress string
	var hostname, macAddressStr, vendor, osFamily, osName, osVersion *string
	var osConfidence, responseTimeMS *int
	var discoveryMethod *string
	var ignoreScanning *bool

	err := db.DB.QueryRowContext(ctx, query, id).Scan(
		&host.ID,
		&ipAddress,
		&hostname,
		&macAddressStr,
		&vendor,
		&osFamily,
		&osName,
		&osVersion,
		&osConfidence,
		&discoveryMethod,
		&responseTimeMS,
		&ignoreScanning,
		&host.FirstSeen,
		&host.LastSeen,
		&host.Status,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.ErrNotFoundWithID("host", id.String())
		}
		return nil, fmt.Errorf("failed to get host: %w", err)
	}

	// Convert IP address
	host.IPAddress = IPAddr{IP: net.ParseIP(ipAddress)}

	// Handle nullable fields using helper functions
	assignStringPtr(&host.Hostname, hostname)
	assignMACAddress(&host.MACAddress, macAddressStr)
	assignStringPtr(&host.Vendor, vendor)
	assignStringPtr(&host.OSFamily, osFamily)
	assignStringPtr(&host.OSName, osName)
	assignStringPtr(&host.OSVersion, osVersion)
	assignIntPtr(&host.OSConfidence, osConfidence)
	assignStringPtr(&host.DiscoveryMethod, discoveryMethod)
	assignIntPtr(&host.ResponseTimeMS, responseTimeMS)
	assignBoolFromPtr(&host.IgnoreScanning, ignoreScanning)

	return host, nil
}

// parsePostgreSQLArray converts PostgreSQL array interface{} to []string
func parsePostgreSQLArray(arrayInterface interface{}) []string {
	if arrayInterface == nil {
		return nil
	}

	arr, ok := arrayInterface.([]interface{})
	if !ok {
		return nil
	}

	result := make([]string, len(arr))
	for i, v := range arr {
		if s, ok := v.(string); ok {
			result[i] = s
		}
	}
	return result
}

// buildUpdateQuery creates SQL SET clause and args from field mappings
func buildUpdateQuery(data map[string]interface{}, fieldMappings map[string]string) (
	setParts []string, args []interface{}) {
	argIndex := 1

	for requestField, dbField := range fieldMappings {
		if value, exists := data[requestField]; exists && value != nil {
			setParts = append(setParts, fmt.Sprintf("%s = $%d", dbField, argIndex))
			args = append(args, value)
			argIndex++
		}
	}

	return setParts, args
}

// UpdateHost updates an existing host.
func (db *DB) UpdateHost(ctx context.Context, id uuid.UUID, hostData interface{}) (*Host, error) {
	// Convert the interface{} to host data
	data, ok := hostData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid host data format")
	}

	// Start a transaction
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	// Check if the host exists
	var exists bool
	err = tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM hosts WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check host existence: %w", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("host", id.String())
	}

	// Build dynamic update query using field mapping
	fieldMappings := map[string]string{
		"hostname":        "hostname",
		"vendor":          "vendor",
		"os_family":       "os_family",
		"os_name":         "os_name",
		"os_version":      "os_version",
		"ignore_scanning": "ignore_scanning",
		"status":          "status",
	}

	setParts, args := buildUpdateQuery(data, fieldMappings)
	argIndex := len(args) + 1

	// If no fields to update, return error
	if len(setParts) == 0 {
		return nil, fmt.Errorf("no valid fields to update")
	}

	// Always update the updated_at timestamp
	setParts = append(setParts, "last_seen = NOW()")

	// Build and execute update query safely
	setClause := strings.Join(setParts, ", ")
	var queryBuilder strings.Builder
	queryBuilder.WriteString("UPDATE hosts SET ")
	queryBuilder.WriteString(setClause)
	queryBuilder.WriteString(" WHERE id = $")
	queryBuilder.WriteString(strconv.Itoa(argIndex))
	updateQuery := queryBuilder.String()

	args = append(args, id)

	_, err = tx.ExecContext(ctx, updateQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update host: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Retrieve and return the updated host
	return db.GetHost(ctx, id)
}

// DeleteHost deletes a host by ID.
func (db *DB) DeleteHost(ctx context.Context, id uuid.UUID) error {
	// Start a transaction
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	// Check if the host exists
	var exists bool
	err = tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM hosts WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check host existence: %w", err)
	}
	if !exists {
		return errors.ErrNotFoundWithID("host", id.String())
	}

	// Delete the host (CASCADE will handle related port_scans, services, etc.)
	_, err = tx.ExecContext(ctx, "DELETE FROM hosts WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// getHostScansCount gets total count of scans for a specific host
func (db *DB) getHostScansCount(ctx context.Context, hostID uuid.UUID) (int64, error) {
	countQuery := `
		SELECT COUNT(DISTINCT sj.id)
		FROM scan_jobs sj
		JOIN scan_targets st ON sj.target_id = st.id
		WHERE st.network >>= (SELECT ip_address FROM hosts WHERE id = $1)
		   OR sj.id IN (
			   SELECT DISTINCT ps.job_id
			   FROM port_scans ps
			   WHERE ps.host_id = $1
		   )
	`

	var total int64
	err := db.QueryRowContext(ctx, countQuery, hostID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get host scans count: %w", err)
	}
	return total, nil
}

// processHostScanRow processes a single host scan row from query results
func processHostScanRow(rows *sql.Rows) (*Scan, error) {
	scan := &Scan{}
	var scheduleID sql.NullInt64
	var profileID sql.NullInt64
	var description sql.NullString
	var startedAt sql.NullTime
	var completedAt sql.NullTime
	var options string

	err := rows.Scan(
		&scan.ID,
		&scan.Name,
		&description,
		&scan.Targets,
		&scan.ScanType,
		&scan.Ports,
		&profileID,
		&scan.Status,
		&startedAt,
		&completedAt,
		&scan.CreatedAt,
		&scheduleID,
		&scan.Tags,
		&options,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan host scan row: %w", err)
	}

	// Handle nullable fields
	if description.Valid {
		scan.Description = description.String
	}
	if profileID.Valid {
		scan.ProfileID = &profileID.Int64
	}
	if startedAt.Valid {
		scan.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		scan.CompletedAt = &completedAt.Time
	}
	if scheduleID.Valid {
		scan.ScheduleID = &scheduleID.Int64
	}

	return scan, nil
}

// GetHostScans retrieves scans for a specific host with pagination.
func (db *DB) GetHostScans(ctx context.Context, hostID uuid.UUID, offset, limit int) ([]*Scan, int64, error) {
	// Get total count
	total, err := db.getHostScansCount(ctx, hostID)
	if err != nil {
		return nil, 0, err
	}

	// Get the actual scans with pagination
	query := `
		SELECT DISTINCT
			sj.id,
			st.name,
			st.description,
			st.network::text as targets,
			COALESCE(sp.scan_type, st.scan_type, 'connect') as scan_type,
			st.scan_ports as ports,
			sj.profile_id,
			sj.status,
			sj.started_at,
			sj.completed_at,
			sj.created_at,
			COALESCE(sch.id, NULL) as schedule_id,
			'[]'::jsonb as tags,
			'{}' as options
		FROM scan_jobs sj
		JOIN scan_targets st ON sj.target_id = st.id
		LEFT JOIN scan_profiles sp ON sj.profile_id = sp.id
		LEFT JOIN scheduled_jobs sch ON st.id::text = sch.config->>'target_id'
		WHERE st.network >>= (SELECT ip_address FROM hosts WHERE id = $1)
		   OR sj.id IN (
			   SELECT DISTINCT ps.job_id
			   FROM port_scans ps
			   WHERE ps.host_id = $1
		   )
		ORDER BY sj.created_at DESC
		OFFSET $2 LIMIT $3
	`

	rows, err := db.QueryContext(ctx, query, hostID, offset, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get host scans: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var scans []*Scan
	for rows.Next() {
		scan, err := processHostScanRow(rows)
		if err != nil {
			return nil, 0, err
		}
		scans = append(scans, scan)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate host scan rows: %w", err)
	}

	return scans, total, nil
}

// ProfileFilters represents filters for listing profiles.
type ProfileFilters struct {
	ScanType string
}

// ListProfiles retrieves profiles with filtering and pagination.
func (db *DB) ListProfiles(ctx context.Context, filters ProfileFilters,
	offset, limit int) ([]*ScanProfile, int64, error) {
	// Build the base query
	baseQuery := `
		SELECT id, name, description, os_family, ports, scan_type,
		       timing, scripts, options, priority, built_in,
		       created_at, updated_at
		FROM scan_profiles`

	// Build WHERE clause based on filters
	var whereClause string
	var args []interface{}
	argIndex := 0

	if filters.ScanType != "" {
		whereClause += fmt.Sprintf(" AND scan_type = $%d", argIndex+1)
		args = append(args, filters.ScanType)
		argIndex++
	}

	// Remove leading "AND" if we have where clauses
	if whereClause != "" {
		whereClause = "WHERE" + whereClause[4:]
	}

	// Get total count
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM scan_profiles %s", whereClause)

	var total int64
	err := db.DB.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get profile count: %w", err)
	}

	// Get paginated results
	listQuery := fmt.Sprintf(
		"%s %s ORDER BY priority DESC, name ASC LIMIT $%d OFFSET $%d",
		baseQuery, whereClause, argIndex+1, argIndex+2)
	args = append(args, limit, offset)

	rows, err := db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query profiles: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Error closing rows: %v", err)
		}
	}()

	var profiles []*ScanProfile
	for rows.Next() {
		profile := &ScanProfile{}
		var description *string
		var timing *string
		var options []byte
		var osFamily, scripts interface{}

		err := rows.Scan(
			&profile.ID,
			&profile.Name,
			&description,
			&osFamily,
			&profile.Ports,
			&profile.ScanType,
			&timing,
			&scripts,
			&options,
			&profile.Priority,
			&profile.BuiltIn,
			&profile.CreatedAt,
			&profile.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan profile row: %w", err)
		}

		// Handle nullable fields
		if description != nil {
			profile.Description = *description
		}
		if timing != nil {
			profile.Timing = *timing
		}

		// Handle PostgreSQL arrays
		profile.OSFamily = parsePostgreSQLArray(osFamily)
		profile.Scripts = parsePostgreSQLArray(scripts)

		// Handle JSONB options
		if len(options) > 0 {
			profile.Options = JSONB(string(options))
		}

		profiles = append(profiles, profile)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate profile rows: %w", err)
	}

	return profiles, total, nil
}

// CreateProfile creates a new profile.
func (db *DB) CreateProfile(ctx context.Context, profileData interface{}) (*ScanProfile, error) {
	data, ok := profileData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid profile data format")
	}

	// Extract data from request
	name, ok := data["name"].(string)
	if !ok {
		return nil, fmt.Errorf("profile name is required")
	}

	description := ""
	if desc, ok := data["description"].(string); ok {
		description = desc
	}

	scanType, ok := data["scan_type"].(string)
	if !ok {
		return nil, fmt.Errorf("scan_type is required")
	}

	ports, ok := data["ports"].(string)
	if !ok {
		return nil, fmt.Errorf("ports is required")
	}

	// Extract options and timing
	var optionsJSON []byte
	var timingStr string

	if options, ok := data["options"].(map[string]interface{}); ok {
		if optionsBytes, err := json.Marshal(options); err == nil {
			optionsJSON = optionsBytes
		}
	}
	if optionsJSON == nil {
		optionsJSON = []byte("{}")
	}

	if timing, ok := data["timing"].(string); ok {
		timingStr = timing
	}

	// Generate profile ID from name (sanitized)
	profileID := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	now := time.Now().UTC()

	query := `
		INSERT INTO scan_profiles
		       (id, name, description, ports, scan_type, options, timing, priority, built_in, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, 0, false, $8, $9)`

	_, err := db.ExecContext(ctx, query, profileID, name, description,
		ports, scanType, optionsJSON, timingStr, now, now)
	if err != nil {
		return nil, fmt.Errorf("failed to create profile: %w", err)
	}

	profile := &ScanProfile{
		ID:          profileID,
		Name:        name,
		Description: description,
		Ports:       ports,
		ScanType:    scanType,
		Options:     JSONB(optionsJSON),
		Timing:      timingStr,
		Priority:    0,
		BuiltIn:     false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return profile, nil
}

// GetProfile retrieves a profile by ID.
func (db *DB) GetProfile(ctx context.Context, id string) (*ScanProfile, error) {
	query := `
		SELECT id, name, description, os_family, ports, scan_type,
		       timing, scripts, options, priority, built_in,
		       created_at, updated_at
		FROM scan_profiles
		WHERE id = $1`

	profile := &ScanProfile{}
	var description sql.NullString
	var timing sql.NullString
	var options []byte
	var osFamily, scripts interface{}

	err := db.DB.QueryRowContext(ctx, query, id).Scan(
		&profile.ID,
		&profile.Name,
		&description,
		&osFamily,
		&profile.Ports,
		&profile.ScanType,
		&timing,
		&scripts,
		&options,
		&profile.Priority,
		&profile.BuiltIn,
		&profile.CreatedAt,
		&profile.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.ErrNotFoundWithID("profile", id)
		}
		return nil, fmt.Errorf("failed to get profile: %w", err)
	}

	// Handle nullable fields
	if description.Valid {
		profile.Description = description.String
	}
	if timing.Valid {
		profile.Timing = timing.String
	}

	// Handle PostgreSQL arrays
	if osFamily != nil {
		if arr, ok := osFamily.(pq.StringArray); ok {
			profile.OSFamily = []string(arr)
		}
	}
	if scripts != nil {
		if arr, ok := scripts.(pq.StringArray); ok {
			profile.Scripts = []string(arr)
		}
	}

	// Handle JSONB options
	if len(options) > 0 {
		profile.Options = JSONB(string(options))
	}

	return profile, nil
}

// UpdateProfile updates an existing profile.
func (db *DB) UpdateProfile(ctx context.Context, id string, profileData interface{}) (*ScanProfile, error) {
	// Convert the interface{} to profile data
	data, ok := profileData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid profile data format")
	}

	// Start a transaction
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	// Check if the profile exists and is not built-in
	var exists bool
	var builtIn bool
	err = tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM scan_profiles WHERE id = $1),
		 COALESCE(built_in, false) FROM scan_profiles WHERE id = $1`,
		id).Scan(&exists, &builtIn)
	if err != nil {
		return nil, fmt.Errorf("failed to check profile existence: %w", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("profile", id)
	}
	if builtIn {
		return nil, fmt.Errorf("cannot update built-in profile")
	}

	// Build dynamic update query
	fieldMappings := map[string]string{
		"name":        "name",
		"description": "description",
		"scan_type":   "scan_type",
		"ports":       "ports",
		"timing":      "timing",
		"scripts":     "scripts",
		"options":     "options",
		"priority":    "priority",
	}

	setParts := []string{}
	args := []interface{}{}
	argCount := 1

	for dataField, dbField := range fieldMappings {
		if value, exists := data[dataField]; exists {
			setParts = append(setParts, fmt.Sprintf("%s = $%d", dbField, argCount))
			args = append(args, value)
			argCount++
		}
	}

	if len(setParts) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	// Always update the updated_at timestamp
	setParts = append(setParts, fmt.Sprintf("updated_at = $%d", argCount))
	args = append(args, time.Now().UTC())
	argCount++

	// Add WHERE clause argument
	args = append(args, id)

	setClause := strings.Join(setParts, ", ")
	var queryBuilder strings.Builder
	queryBuilder.WriteString("UPDATE scan_profiles SET ")
	queryBuilder.WriteString(setClause)
	queryBuilder.WriteString(" WHERE id = $")
	queryBuilder.WriteString(strconv.Itoa(argCount))
	updateQuery := queryBuilder.String()

	_, err = tx.ExecContext(ctx, updateQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update profile: %w", err)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Retrieve the updated profile
	return db.GetProfile(ctx, id)
}

// DeleteProfile deletes a profile by ID.
func (db *DB) DeleteProfile(ctx context.Context, id string) error {
	// Start a transaction
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	// Check if the profile exists and is not built-in
	var exists bool
	var builtIn bool
	err = tx.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM scan_profiles WHERE id = $1),
		 COALESCE(built_in, false) FROM scan_profiles WHERE id = $1`,
		id).Scan(&exists, &builtIn)
	if err != nil {
		return fmt.Errorf("failed to check profile existence: %w", err)
	}
	if !exists {
		return errors.ErrNotFoundWithID("profile", id)
	}
	if builtIn {
		return fmt.Errorf("cannot delete built-in profile")
	}

	// Check if profile is in use by any scan jobs
	var inUse bool
	err = tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE profile_id = $1)",
		id).Scan(&inUse)
	if err != nil {
		return fmt.Errorf("failed to check profile usage: %w", err)
	}
	if inUse {
		return fmt.Errorf("cannot delete profile that is in use by scan jobs")
	}

	// Delete the profile
	result, err := tx.ExecContext(ctx, "DELETE FROM scan_profiles WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete profile: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return errors.ErrNotFoundWithID("profile", id)
	}

	// Commit the transaction
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
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

// Helper functions for nullable field handling

// assignStringPtr assigns a nullable string pointer to a target string pointer
func assignStringPtr(target **string, source *string) {
	if source != nil && *source != "" {
		*target = source
	}
}

// assignMACAddress assigns a nullable MAC address string to a target MACAddr
func assignMACAddress(target **MACAddr, source *string) {
	if source != nil && *source != "" {
		if mac, err := net.ParseMAC(*source); err == nil {
			macAddr := MACAddr{HardwareAddr: mac}
			*target = &macAddr
		}
	}
}

// assignIntPtr assigns a nullable int pointer to a target int pointer
func assignIntPtr(target **int, source *int) {
	if source != nil {
		*target = source
	}
}

// assignBoolFromPtr assigns a nullable bool pointer to a target bool
func assignBoolFromPtr(target, source *bool) {
	if source != nil {
		*target = *source
	}
}

// buildHostFilters creates WHERE clause and args for host filtering
func buildHostFilters(filters HostFilters) (whereClause string, args []interface{}) {
	var conditions []filterCondition

	if filters.Status != "" {
		conditions = append(conditions, filterCondition{"h.status", filters.Status})
	}
	if filters.OSFamily != "" {
		conditions = append(conditions, filterCondition{"h.os_family", filters.OSFamily})
	}
	if filters.Network != "" {
		conditions = append(conditions, filterCondition{"h.ip_address <<", filters.Network})
	}

	return buildWhereClause(conditions)
}

// getHostCount gets total count of hosts matching filters
func (db *DB) getHostCount(ctx context.Context, whereClause string, args []interface{}) (int64, error) {
	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT h.id)
		FROM hosts h
		LEFT JOIN port_scans ps ON h.id = ps.host_id
		%s
	`, whereClause)

	var total int64
	err := db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("failed to get host count: %w", err)
	}
	return total, nil
}

// scanHostRows processes query result rows into Host structs
func (db *DB) scanHostRows(rows *sql.Rows) ([]*Host, error) {
	var hosts []*Host
	for rows.Next() {
		host := &Host{}
		var ipAddress string
		var hostname, macAddressStr, vendor, osFamily, osName, osVersion *string
		var osConfidence, responseTimeMS *int
		var discoveryMethod *string
		var ignoreScanning *bool
		var openPorts, totalPortsScanned int64

		err := rows.Scan(
			&host.ID,
			&ipAddress,
			&hostname,
			&macAddressStr,
			&vendor,
			&osFamily,
			&osName,
			&osVersion,
			&osConfidence,
			&discoveryMethod,
			&responseTimeMS,
			&ignoreScanning,
			&host.FirstSeen,
			&host.LastSeen,
			&host.Status,
			&openPorts,
			&totalPortsScanned,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan host row: %w", err)
		}

		// Convert IP address
		host.IPAddress = IPAddr{IP: net.ParseIP(ipAddress)}

		// Handle nullable fields using helper functions
		assignStringPtr(&host.Hostname, hostname)
		assignMACAddress(&host.MACAddress, macAddressStr)
		assignStringPtr(&host.Vendor, vendor)
		assignStringPtr(&host.OSFamily, osFamily)
		assignStringPtr(&host.OSName, osName)
		assignStringPtr(&host.OSVersion, osVersion)
		assignIntPtr(&host.OSConfidence, osConfidence)
		assignStringPtr(&host.DiscoveryMethod, discoveryMethod)
		assignIntPtr(&host.ResponseTimeMS, responseTimeMS)
		assignBoolFromPtr(&host.IgnoreScanning, ignoreScanning)

		hosts = append(hosts, host)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate host rows: %w", err)
	}

	return hosts, nil
}

// Ping tests the database connection.
func (db *DB) Ping(ctx context.Context) error {
	return db.PingContext(ctx)
}

// BeginTx starts a new transaction.
func (db *DB) BeginTx(ctx context.Context) (*sqlx.Tx, error) {
	return db.BeginTxx(ctx, nil)
}
