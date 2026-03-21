// Package db provides scan-related database operations for scanorama.
package db

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/errors"
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

// buildScanFilters creates WHERE clause and args for scan filtering.
func buildScanFilters(filters ScanFilters) (whereClause string, args []interface{}) {
	var conditions []filterCondition

	if filters.Status != "" {
		conditions = append(conditions, filterCondition{"sj.status", filters.Status})
	}
	if filters.ScanType != "" {
		conditions = append(conditions, filterCondition{"COALESCE(sp.scan_type, n.scan_type)", filters.ScanType})
	}
	if filters.ProfileID != nil {
		conditions = append(conditions, filterCondition{"sj.profile_id", *filters.ProfileID})
	}

	return buildWhereClause(conditions)
}

// getScanCount gets total count of scans matching filters.
func (db *DB) getScanCount(ctx context.Context, whereClause string, args []interface{}) (int64, error) {
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM scan_jobs sj
		JOIN networks n ON sj.network_id = n.id
		LEFT JOIN scan_profiles sp ON sj.profile_id = sp.id %s`, whereClause)

	var total int64
	err := db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return 0, sanitizeDBError("get scan count", err)
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
		&scan.ErrorMessage,
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
		scan.ProfileID = profileID
	}

	// Set UpdatedAt (use CompletedAt if available, otherwise CreatedAt).
	if scan.CompletedAt != nil {
		scan.UpdatedAt = *scan.CompletedAt
	} else if scan.StartedAt != nil {
		scan.UpdatedAt = *scan.StartedAt
	} else {
		scan.UpdatedAt = scan.CreatedAt
	}

	// Compute DurationStr if both timestamps are available.
	if scan.StartedAt != nil && scan.CompletedAt != nil {
		d := scan.CompletedAt.Sub(*scan.StartedAt).String()
		scan.DurationStr = &d
	}

	// Compute PortsScanned from Ports if non-empty.
	if scan.Ports != "" {
		p := scan.Ports
		scan.PortsScanned = &p
	}

	return scan, nil
}

// ListScans retrieves scans with filtering and pagination.
func (db *DB) ListScans(ctx context.Context, filters ScanFilters, offset, limit int) ([]*Scan, int64, error) {
	// Build the base query with joins.
	baseQuery := `
		SELECT
			sj.id,
			n.name,
			n.description,
			n.cidr::text as targets,
			COALESCE(sp.scan_type, n.scan_type) as scan_type,
			n.scan_ports as ports,
			sj.profile_id,
			sj.status,
			sj.created_at,
			sj.started_at,
			sj.completed_at,
			sj.error_message
		FROM scan_jobs sj
		JOIN networks n ON sj.network_id = n.id
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
		return nil, 0, sanitizeDBError("list scans", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("error closing rows", "error", err)
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
	osDetection bool
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

	name, ok := data["name"].(string)
	if !ok || name == "" {
		return nil, errors.NewScanError(errors.CodeValidation, "name is required")
	}

	scanType, ok := data["scan_type"].(string)
	if !ok || scanType == "" {
		return nil, errors.NewScanError(errors.CodeValidation, "scan_type is required")
	}

	result := &scanData{
		name:     name,
		scanType: scanType,
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

	if pid, ok := data["profile_id"].(*string); ok && pid != nil {
		result.profileID = pid
	}

	if osDetection, ok := data["os_detection"].(bool); ok {
		result.osDetection = osDetection
	}

	return result, nil
}

// findOrCreateNetwork finds an existing network by CIDR or creates a new one.
// Returns the network UUID to store as scan_jobs.network_id.
func (db *DB) findOrCreateNetwork(ctx context.Context, tx *sql.Tx,
	name, cidr, description, ports, scanType string) (uuid.UUID, error) {
	// Reuse the network if the CIDR is already known.
	var id uuid.UUID
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM networks WHERE cidr = $1`, cidr).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !stderrors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, sanitizeDBError("look up network by CIDR", err)
	}

	// Not found — create a new network entry.
	// ON CONFLICT (name) DO NOTHING avoids aborting the outer transaction on a
	// name collision; we detect the no-op via RowsAffected and retry with the
	// CIDR string as the name (always unique because we verified the CIDR is new).
	id = uuid.New()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO networks (
			id, name, cidr, description,
			scan_ports, scan_type,
			is_active, scan_enabled, discovery_method
		) VALUES (
			$1, $2, $3, $4, $5, $6, true, true, 'tcp'
		)
		ON CONFLICT (name) DO NOTHING
	`, id, name, cidr, description, ports, scanType)
	if err != nil {
		return uuid.Nil, sanitizeDBError("create network", err)
	}

	if n, _ := result.RowsAffected(); n == 0 {
		// Name collision — fall back to using the CIDR string as the name.
		id = uuid.New()
		_, err = tx.ExecContext(ctx, `
			INSERT INTO networks (
				id, name, cidr, description,
				scan_ports, scan_type,
				is_active, scan_enabled, discovery_method
			) VALUES (
				$1, $2, $3, $4, $5, $6, true, true, 'tcp'
			)
		`, id, cidr, cidr, description, ports, scanType)
		if err != nil {
			return uuid.Nil, sanitizeDBError("create network", err)
		}
	}
	return id, nil
}

// createScanJob creates a scan job in the database.
func (db *DB) createScanJob(ctx context.Context, tx *sql.Tx, jobID, networkID uuid.UUID,
	profileID *string, now time.Time, osDetection bool) error {
	execDetails := fmt.Sprintf(`{"os_detection": %t}`, osDetection)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO scan_jobs (id, network_id, profile_id, status, created_at, execution_details)
		VALUES ($1, $2, $3, 'pending', $4, $5)
	`, jobID, networkID, profileID, now, execDetails)
	if err != nil {
		return sanitizeDBError("create scan job", err)
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
		scan.ProfileID = profileID
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
			slog.Warn("error rolling back transaction", "error", err)
		}
	}()

	now := time.Now().UTC()
	var firstJobID uuid.UUID

	// For each target CIDR, find or create a network then create the scan job.
	for i, target := range data.targets {
		jobID := uuid.New()

		if i == 0 {
			firstJobID = jobID
		}

		networkName := data.name
		if len(data.targets) > 1 {
			networkName = fmt.Sprintf("%s-target-%d", data.name, i+1)
		}

		networkID, err := db.findOrCreateNetwork(ctx, tx, networkName, target,
			data.description, data.ports, data.scanType)
		if err != nil {
			return nil, err
		}

		// Create scan job.
		if err := db.createScanJob(ctx, tx, jobID, networkID, data.profileID, now, data.osDetection); err != nil {
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
			n.name,
			n.description,
			n.cidr::text as targets,
			COALESCE(sp.scan_type, n.scan_type) as scan_type,
				n.scan_ports as ports,
				sj.profile_id,
				sj.status,
				sj.created_at,
				sj.started_at,
				sj.completed_at,
				sj.error_message,
				COALESCE((sj.execution_details->>'os_detection')::boolean, false) as os_detection
			FROM scan_jobs sj
			JOIN networks n ON sj.network_id = n.id
		LEFT JOIN scan_profiles sp ON sj.profile_id = sp.id
		WHERE sj.id = $1
	`

	scan := &Scan{}
	var targetsStr string
	var profileID *string
	var description sql.NullString
	var osDetection bool

	err := db.DB.QueryRowContext(ctx, query, id).Scan(
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
		&scan.ErrorMessage,
		&osDetection,
	)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, errors.ErrNotFoundWithID("scan", id.String())
		}
		return nil, sanitizeDBError("get scan", err)
	}

	// Handle nullable description.
	if description.Valid {
		scan.Description = description.String
	}

	// Parse targets from CIDR string.
	scan.Targets = []string{targetsStr}

	// Populate Options from execution_details.
	scan.Options = map[string]interface{}{
		"os_detection": osDetection,
	}

	// Set ProfileID if not null.
	if profileID != nil {
		scan.ProfileID = profileID
	}

	// Set UpdatedAt (use CompletedAt if available, otherwise StartedAt, otherwise CreatedAt).
	if scan.CompletedAt != nil {
		scan.UpdatedAt = *scan.CompletedAt
	} else if scan.StartedAt != nil {
		scan.UpdatedAt = *scan.StartedAt
	} else {
		scan.UpdatedAt = scan.CreatedAt
	}

	// Compute DurationStr if both timestamps are available.
	if scan.StartedAt != nil && scan.CompletedAt != nil {
		d := scan.CompletedAt.Sub(*scan.StartedAt).String()
		scan.DurationStr = &d
	}

	// Compute PortsScanned from Ports if non-empty.
	if scan.Ports != "" {
		p := scan.Ports
		scan.PortsScanned = &p
	}

	return scan, nil
}

// UpdateScan updates an existing scan.
// updateScanTarget updates the scan_targets row linked to a scan job within a transaction.
func updateScanNetwork(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}, id uuid.UUID, data map[string]interface{}) error {
	networkFieldMappings := map[string]string{
		"name":        "name",
		"description": "description",
		"scan_type":   "scan_type",
		"ports":       "scan_ports",
	}

	setParts, args := buildUpdateQuery(data, networkFieldMappings)
	if len(setParts) == 0 {
		return nil
	}

	setParts = append(setParts, "updated_at = NOW()")
	paramNum := len(args) + 1

	var queryBuilder strings.Builder
	queryBuilder.WriteString("UPDATE networks SET ")
	queryBuilder.WriteString(strings.Join(setParts, ", "))
	queryBuilder.WriteString(" WHERE id = (SELECT network_id FROM scan_jobs WHERE id = $")
	queryBuilder.WriteString(strconv.Itoa(paramNum))
	queryBuilder.WriteString(")")

	args = append(args, id)
	if _, err := tx.ExecContext(ctx, queryBuilder.String(), args...); err != nil {
		return sanitizeDBError("update scan network", err)
	}
	return nil
}

// updateScanJob updates the scan_jobs row within a transaction.
func updateScanJob(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}, id uuid.UUID, data map[string]interface{}) error {
	jobFieldMappings := map[string]string{
		"status": "status",
	}
	jobSetParts, jobArgs := buildUpdateQuery(data, jobFieldMappings)

	if profileID, ok := data["profile_id"].(*string); ok && profileID != nil {
		jobSetParts = append(jobSetParts, fmt.Sprintf("profile_id = $%d", len(jobArgs)+1))
		jobArgs = append(jobArgs, *profileID)
	}

	if len(jobSetParts) == 0 {
		return nil
	}

	paramNum := len(jobArgs) + 1
	var jobQueryBuilder strings.Builder
	jobQueryBuilder.WriteString("UPDATE scan_jobs SET ")
	jobQueryBuilder.WriteString(strings.Join(jobSetParts, ", "))
	jobQueryBuilder.WriteString(" WHERE id = $")
	jobQueryBuilder.WriteString(strconv.Itoa(paramNum))

	jobArgs = append(jobArgs, id)
	if _, err := tx.ExecContext(ctx, jobQueryBuilder.String(), jobArgs...); err != nil {
		return sanitizeDBError("update scan job", err)
	}
	return nil
}

func (db *DB) UpdateScan(ctx context.Context, id uuid.UUID, scanData interface{}) (*Scan, error) {
	data, ok := scanData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid scan data format")
	}

	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			slog.Warn("error rolling back transaction", "error", err)
		}
	}()

	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return nil, sanitizeDBError("check scan existence", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("scan", id.String())
	}

	if err := updateScanNetwork(ctx, tx, id, data); err != nil {
		return nil, err
	}

	if err := updateScanJob(ctx, tx, id, data); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

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
			slog.Warn("error rolling back transaction", "error", err)
		}
	}()

	// Check if the scan exists.
	var exists bool
	err = tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return sanitizeDBError("check scan existence", err)
	}
	if !exists {
		return errors.ErrNotFoundWithID("scan", id.String())
	}

	// Prevent deleting a scan that is currently running.
	var status string
	err = tx.QueryRowContext(ctx, "SELECT status FROM scan_jobs WHERE id = $1", id).Scan(&status)
	if err != nil {
		return sanitizeDBError("get scan status", err)
	}
	if status == "running" {
		return errors.ErrConflictWithReason("scan", "cannot delete a running scan; stop it first")
	}

	// Delete the scan job (cascades to port_scans, services, host_history, etc.).
	// Networks are persistent managed entities and are NOT deleted with the scan.
	_, err = tx.ExecContext(ctx, "DELETE FROM scan_jobs WHERE id = $1", id)
	if err != nil {
		return sanitizeDBError("delete scan job", err)
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
		return nil, 0, sanitizeDBError("get scan results count", err)
	}

	// Get paginated results.
	query := `
		SELECT
			ps.id,
			ps.job_id,
			ps.host_id,
			host(h.ip_address) AS host_ip,
			ps.port,
			ps.protocol,
			ps.state,
			ps.service_name,
			ps.scanned_at,
			COALESCE(h.os_name, '')    AS os_name,
			COALESCE(h.os_family, '')  AS os_family,
			COALESCE(h.os_version, '') AS os_version,
			h.os_confidence
		FROM port_scans ps
		LEFT JOIN hosts h ON h.id = ps.host_id
		WHERE ps.job_id = $1
		ORDER BY ps.scanned_at DESC, ps.port ASC
		LIMIT $2 OFFSET $3
	`

	rows, err := db.QueryContext(ctx, query, scanID, limit, offset)
	if err != nil {
		return nil, 0, sanitizeDBError("query scan results", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("error closing rows", "error", err)
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
			&result.HostIP,
			&result.Port,
			&result.Protocol,
			&result.State,
			&serviceName,
			&result.ScannedAt,
			&result.OSName,
			&result.OSFamily,
			&result.OSVersion,
			&result.OSConfidence,
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
		if stderrors.Is(err, sql.ErrNoRows) {
			// No results for this scan, return zero summary.
			return summary, nil
		}
		return nil, sanitizeDBError("get scan summary", err)
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
		return sanitizeDBError("start scan", err)
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

// CompleteScan marks a scan as successfully completed.
func (db *DB) CompleteScan(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE scan_jobs
		SET status = 'completed', completed_at = NOW()
		WHERE id = $1 AND status = 'running'
	`

	result, err := db.ExecContext(ctx, query, id)
	if err != nil {
		return sanitizeDBError("complete scan", err)
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

// StopScan stops scan execution and marks it as failed.
func (db *DB) StopScan(ctx context.Context, id uuid.UUID, errMsg ...string) error {
	var msg *string
	if len(errMsg) > 0 && errMsg[0] != "" {
		msg = &errMsg[0]
	}

	query := `
		UPDATE scan_jobs
		SET status = 'failed', completed_at = NOW(), error_message = COALESCE($2, error_message)
		WHERE id = $1 AND status = 'running'
	`

	result, err := db.ExecContext(ctx, query, id, msg)
	if err != nil {
		return sanitizeDBError("stop scan", err)
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
