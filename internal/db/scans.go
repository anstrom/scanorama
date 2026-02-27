// Package db provides scan-related database operations for scanorama.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/errors"
)

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
		INSERT INTO scan_targets (
			id, name, network, description,
			scan_interval_seconds, scan_ports, scan_type, enabled
		)
		VALUES (
			:id, :name, :network, :description,
			:scan_interval_seconds, :scan_ports, :scan_type, :enabled
		)
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
	query := `
		SELECT *
		FROM scan_targets
		WHERE id = $1`

	if err := r.db.GetContext(ctx, &target, query, id); err != nil {
		return nil, sanitizeDBError("get scan target", err)
	}

	return &target, nil
}

// GetAll retrieves all scan targets.
func (r *ScanTargetRepository) GetAll(ctx context.Context) ([]*ScanTarget, error) {
	var targets []*ScanTarget
	query := `
		SELECT *
		FROM scan_targets
		ORDER BY name`

	if err := r.db.SelectContext(ctx, &targets, query); err != nil {
		return nil, sanitizeDBError("get scan targets", err)
	}

	return targets, nil
}

// GetEnabled retrieves all enabled scan targets.
func (r *ScanTargetRepository) GetEnabled(ctx context.Context) ([]*ScanTarget, error) {
	var targets []*ScanTarget
	query := `
		SELECT *
		FROM scan_targets
		WHERE enabled = true
		ORDER BY name`

	if err := r.db.SelectContext(ctx, &targets, query); err != nil {
		return nil, sanitizeDBError("get enabled scan targets", err)
	}

	return targets, nil
}

// Update updates a scan target.
func (r *ScanTargetRepository) Update(ctx context.Context, target *ScanTarget) error {
	query := `
		UPDATE scan_targets
		SET
			name = :name,
			network = :network,
			description = :description,
			scan_interval_seconds = :scan_interval_seconds,
			scan_ports = :scan_ports,
			scan_type = :scan_type,
			enabled = :enabled
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
	query := `
		DELETE FROM scan_targets
		WHERE id = $1`

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
		INSERT INTO scan_jobs (
			id, target_id, status,
			started_at, completed_at, scan_stats
		)
		VALUES (
			:id, :target_id, :status,
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

// buildScanFilters creates WHERE clause and args for scan filtering.
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

// getScanCount gets total count of scans matching filters.
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

// processScanRow processes a single scan row from query results.
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

	// Handle nullable description.
	if description.Valid {
		scan.Description = description.String
	} else {
		scan.Description = ""
	}

	// Parse targets from CIDR string.
	scan.Targets = []string{targetsStr}

	// Set ProfileID if not null.
	if profileID != nil {
		if id, err := strconv.ParseInt(*profileID, 10, 64); err == nil {
			scan.ProfileID = &id
		}
	}

	// Set UpdatedAt (use CompletedAt if available, otherwise CreatedAt).
	if scan.CompletedAt != nil {
		scan.UpdatedAt = *scan.CompletedAt
	} else if scan.StartedAt != nil {
		scan.UpdatedAt = *scan.StartedAt
	} else {
		scan.UpdatedAt = scan.CreatedAt
	}

	return scan, nil
}

// ListScans retrieves scans with filtering and pagination.
func (db *DB) ListScans(ctx context.Context, filters ScanFilters, offset, limit int) ([]*Scan, int64, error) {
	// Build the base query with joins.
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

	// Build WHERE clause and arguments.
	whereClause, args := buildScanFilters(filters)

	// Get total count.
	total, err := db.getScanCount(ctx, whereClause, args)
	if err != nil {
		return nil, 0, err
	}

	// Get paginated results.
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

// scanData holds extracted scan parameters.
type scanData struct {
	name        string
	description string
	scanType    string
	ports       string
	targets     []string
	profileID   *string
}

// extractScanData extracts and validates scan data from interface.
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

	// Use map-based extraction for optional fields.
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

// createScanTarget creates a scan target in the database.
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

// createScanJob creates a scan job in the database.
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

// buildScanResponse builds the scan response object.
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
	// Extract and validate data.
	data, err := extractScanData(input)
	if err != nil {
		return nil, err
	}

	// Start a transaction.
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

	// Create scan targets and jobs.
	for i, target := range data.targets {
		targetID := uuid.New()
		jobID := uuid.New()

		if i == 0 {
			firstJobID = jobID
		}

		// Create scan target.
		if err := db.createScanTarget(ctx, tx, targetID, data.name, target,
			data.description, data.ports, data.scanType, i, len(data.targets)); err != nil {
			return nil, err
		}

		// Create scan job.
		if err := db.createScanJob(ctx, tx, jobID, targetID, data.profileID, now); err != nil {
			return nil, err
		}
	}

	// Commit transaction.
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Build and return response.
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

	// Parse targets from CIDR string.
	scan.Targets = []string{targetsStr}

	// Set ProfileID if not null.
	if profileID != nil {
		if pid, err := strconv.ParseInt(*profileID, 10, 64); err == nil {
			scan.ProfileID = &pid
		}
	}

	// Set UpdatedAt (use CompletedAt if available, otherwise StartedAt, otherwise CreatedAt).
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
	// Convert the interface{} to scan data.
	data, ok := scanData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid scan data format")
	}

	// Start a transaction.
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	// First, check if the scan exists.
	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check scan existence: %w", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("scan", id.String())
	}

	// Build dynamic update for scan_targets using field mapping.
	targetFieldMappings := map[string]string{
		"name":        "name",
		"description": "description",
		"scan_type":   "scan_type",
		"ports":       "scan_ports",
	}

	setParts, args := buildUpdateQuery(data, targetFieldMappings)

	// Update scan_targets if there are fields to update.
	if len(setParts) > 0 {
		setParts = append(setParts, "updated_at = NOW()")
		setClause := strings.Join(setParts, ", ")
		paramNum := len(args) + 1

		// Build query safely without string concatenation.
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

	// Update profile_id in scan_jobs if provided.
	if profileID, ok := data["profile_id"].(*int64); ok && profileID != nil {
		profileIDStr := strconv.FormatInt(*profileID, 10)
		_, err = tx.ExecContext(ctx, `
			UPDATE scan_jobs SET profile_id = $1 WHERE id = $2`,
			profileIDStr, id)
		if err != nil {
			return nil, fmt.Errorf("failed to update profile ID: %w", err)
		}
	}

	// Commit transaction.
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Retrieve and return the updated scan.
	return db.GetScan(ctx, id)
}

// DeleteScan deletes a scan by ID.
func (db *DB) DeleteScan(ctx context.Context, id uuid.UUID) error {
	// Start a transaction.
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			log.Printf("Error rolling back transaction: %v", err)
		}
	}()

	// Check if the scan exists.
	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check scan existence: %w", err)
	}
	if !exists {
		return errors.ErrNotFoundWithID("scan", id.String())
	}

	// Get the target_id before deleting the scan job.
	var targetID uuid.UUID
	err = tx.QueryRowContext(ctx, "SELECT target_id FROM scan_jobs WHERE id = $1", id).Scan(&targetID)
	if err != nil {
		return fmt.Errorf("failed to get target ID: %w", err)
	}

	// Delete the scan job (this will cascade to related port_scans, services, etc.).
	_, err = tx.ExecContext(ctx, "DELETE FROM scan_jobs WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete scan job: %w", err)
	}

	// Check if the target is still referenced by other jobs.
	var otherJobsCount int
	err = tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM scan_jobs WHERE target_id = $1", targetID).Scan(&otherJobsCount)
	if err != nil {
		return fmt.Errorf("failed to check remaining jobs for target: %w", err)
	}

	// If no other jobs reference this target, delete it too.
	if otherJobsCount == 0 {
		_, err = tx.ExecContext(ctx, "DELETE FROM scan_targets WHERE id = $1", targetID)
		if err != nil {
			return fmt.Errorf("failed to delete scan target: %w", err)
		}
	}

	// Commit transaction.
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetScanResults retrieves scan results with pagination.
func (db *DB) GetScanResults(ctx context.Context, scanID uuid.UUID, offset, limit int) ([]*ScanResult, int64, error) {
	// Get total count first.
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

	// Get paginated results.
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
			// No results for this scan, return zero summary.
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
	// Update scan job status to running.
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
	// Update scan job status to failed (stopped).
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
