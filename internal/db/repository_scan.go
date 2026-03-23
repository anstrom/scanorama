// Package db provides typed repository for scan-related database operations.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/errors"
)

// ScanRepository implements ScanStore against a *DB connection.
type ScanRepository struct {
	db *DB
}

// NewScanRepository creates a new ScanRepository.
func NewScanRepository(db *DB) *ScanRepository {
	return &ScanRepository{db: db}
}

// DB returns the underlying *DB connection.
// This is used by the scan handler to obtain a concrete *DB for the scan runner.
func (r *ScanRepository) DB() *DB {
	return r.db
}

// GetProfile satisfies the ScanStore interface by delegating to ProfileRepository.
func (r *ScanRepository) GetProfile(ctx context.Context, id string) (*ScanProfile, error) {
	return NewProfileRepository(r.db).GetProfile(ctx, id)
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
func (r *ScanRepository) getScanCount(ctx context.Context, whereClause string, args []interface{}) (int64, error) {
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM scan_jobs sj
		JOIN networks n ON sj.network_id = n.id
		LEFT JOIN scan_profiles sp ON sj.profile_id = sp.id %s`, whereClause)

	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
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

	if description.Valid {
		scan.Description = description.String
	} else {
		scan.Description = ""
	}

	scan.Targets = []string{targetsStr}

	if profileID != nil {
		scan.ProfileID = profileID
	}

	if scan.CompletedAt != nil {
		scan.UpdatedAt = *scan.CompletedAt
	} else if scan.StartedAt != nil {
		scan.UpdatedAt = *scan.StartedAt
	} else {
		scan.UpdatedAt = scan.CreatedAt
	}

	if scan.StartedAt != nil && scan.CompletedAt != nil {
		d := scan.CompletedAt.Sub(*scan.StartedAt).String()
		scan.DurationStr = &d
	}

	if scan.Ports != "" {
		p := scan.Ports
		scan.PortsScanned = &p
	}

	return scan, nil
}

// ListScans retrieves scans with filtering and pagination.
func (r *ScanRepository) ListScans(
	ctx context.Context, filters ScanFilters, offset, limit int,
) ([]*Scan, int64, error) {
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

	whereClause, args := buildScanFilters(filters)

	total, err := r.getScanCount(ctx, whereClause, args)
	if err != nil {
		return nil, 0, err
	}

	argIndex := len(args)
	listQuery := fmt.Sprintf("%s %s ORDER BY sj.created_at DESC LIMIT $%d OFFSET $%d",
		baseQuery, whereClause, argIndex+1, argIndex+2)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, listQuery, args...)
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

// findOrCreateNetwork finds an existing network by CIDR or creates a new one.
// Returns the network UUID to store as scan_jobs.network_id.
func findOrCreateNetwork(ctx context.Context, tx *sql.Tx,
	name, cidr, description, ports, scanType string) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM networks WHERE cidr = $1`, cidr).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !stderrors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, sanitizeDBError("look up network by CIDR", err)
	}

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
// createScanJob inserts a single scan_jobs row. When allTargets contains more
// than one entry the full list is stored in execution_details["scan_targets"]
// so that GetScan can reconstruct the complete target set for the scanner,
// which only sees the single network CIDR stored in the networks FK.
func createScanJob(ctx context.Context, tx *sql.Tx, jobID, networkID uuid.UUID,
	profileID *string, now time.Time, osDetection bool, allTargets []string) error {
	var execDetails string
	if len(allTargets) > 1 {
		targetsJSON, err := json.Marshal(allTargets)
		if err != nil {
			return fmt.Errorf("marshal scan targets: %w", err)
		}
		execDetails = fmt.Sprintf(`{"os_detection": %t, "scan_targets": %s}`, osDetection, targetsJSON)
	} else {
		execDetails = fmt.Sprintf(`{"os_detection": %t}`, osDetection)
	}
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
func (r *ScanRepository) CreateScan(ctx context.Context, input CreateScanInput) (*Scan, error) {
	if input.Name == "" {
		return nil, errors.NewScanError(errors.CodeValidation, "name is required")
	}
	if input.ScanType == "" {
		return nil, errors.NewScanError(errors.CodeValidation, "scan_type is required")
	}
	if len(input.Targets) == 0 {
		return nil, errors.NewScanError(errors.CodeValidation, "targets are required")
	}

	tx, err := r.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			slog.Warn("error rolling back transaction", "error", err)
		}
	}()

	now := time.Now().UTC()

	// Always create exactly one scan_job, regardless of how many targets were
	// supplied. The first target's network provides the required network_id FK.
	// When there are multiple targets the full list is persisted inside
	// execution_details["scan_targets"] and read back by GetScan, ensuring the
	// scanner receives every target rather than only the primary network CIDR.
	jobID := uuid.New()

	networkID, err := findOrCreateNetwork(ctx, tx, input.Name, input.Targets[0],
		input.Description, input.Ports, input.ScanType)
	if err != nil {
		return nil, err
	}

	if err := createScanJob(ctx, tx, jobID, networkID, input.ProfileID, now,
		input.OSDetection, input.Targets); err != nil {
		return nil, err
	}

	var firstJobID = jobID

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return buildScanResponse(firstJobID, input.Name, input.Description, input.Targets,
		input.ScanType, input.Ports, input.ProfileID, now), nil
}

// scanTargetsFromExecDetails returns the stored scan_targets list from the
// execution_details JSON when present, falling back to the single network CIDR
// otherwise. This allows CreateScan to persist a multi-target list without a
// schema change.
func scanTargetsFromExecDetails(execDetailsJSON, networkCIDR string) []string {
	if execDetailsJSON == "" {
		return []string{networkCIDR}
	}
	var execDetails map[string]json.RawMessage
	if err := json.Unmarshal([]byte(execDetailsJSON), &execDetails); err != nil {
		return []string{networkCIDR}
	}
	rawTargets, ok := execDetails["scan_targets"]
	if !ok {
		return []string{networkCIDR}
	}
	var storedTargets []string
	if err := json.Unmarshal(rawTargets, &storedTargets); err != nil || len(storedTargets) == 0 {
		return []string{networkCIDR}
	}
	return storedTargets
}

// GetScan retrieves a scan by ID.
func (r *ScanRepository) GetScan(ctx context.Context, id uuid.UUID) (*Scan, error) {
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
			COALESCE((sj.execution_details->>'os_detection')::boolean, false) as os_detection,
			sj.execution_details::text as execution_details
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
	var execDetailsStr sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
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
		&execDetailsStr,
	)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, errors.ErrNotFoundWithID("scan", id.String())
		}
		return nil, sanitizeDBError("get scan", err)
	}

	if description.Valid {
		scan.Description = description.String
	}

	// Prefer the full target list stored in execution_details over the single
	// network CIDR. This is set by CreateScan when len(targets) > 1.
	scan.Targets = scanTargetsFromExecDetails(execDetailsStr.String, targetsStr)
	scan.Options = map[string]interface{}{"os_detection": osDetection}

	if profileID != nil {
		scan.ProfileID = profileID
	}

	if scan.CompletedAt != nil {
		scan.UpdatedAt = *scan.CompletedAt
	} else if scan.StartedAt != nil {
		scan.UpdatedAt = *scan.StartedAt
	} else {
		scan.UpdatedAt = scan.CreatedAt
	}

	if scan.StartedAt != nil && scan.CompletedAt != nil {
		d := scan.CompletedAt.Sub(*scan.StartedAt).String()
		scan.DurationStr = &d
	}

	if scan.Ports != "" {
		p := scan.Ports
		scan.PortsScanned = &p
	}

	return scan, nil
}

// updateScanNetwork updates the networks row linked to a scan job within a transaction.
func updateScanNetwork(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}, id uuid.UUID, input UpdateScanInput) error {
	var setParts []string
	var args []interface{}
	argIndex := 1

	if input.Name != nil {
		setParts = append(setParts, fmt.Sprintf("name = $%d", argIndex))
		args = append(args, *input.Name)
		argIndex++
	}
	if input.Description != nil {
		setParts = append(setParts, fmt.Sprintf("description = $%d", argIndex))
		args = append(args, *input.Description)
		argIndex++
	}
	if input.ScanType != nil {
		setParts = append(setParts, fmt.Sprintf("scan_type = $%d", argIndex))
		args = append(args, *input.ScanType)
		argIndex++
	}
	if input.Ports != nil {
		setParts = append(setParts, fmt.Sprintf("scan_ports = $%d", argIndex))
		args = append(args, *input.Ports)
		argIndex++
	}

	if len(setParts) == 0 {
		return nil
	}

	setParts = append(setParts, "updated_at = NOW()")

	var queryBuilder strings.Builder
	queryBuilder.WriteString("UPDATE networks SET ")
	queryBuilder.WriteString(strings.Join(setParts, ", "))
	queryBuilder.WriteString(" WHERE id = (SELECT network_id FROM scan_jobs WHERE id = $")
	queryBuilder.WriteString(strconv.Itoa(argIndex))
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
}, id uuid.UUID, input UpdateScanInput) error {
	var setParts []string
	var args []interface{}
	argIndex := 1

	if input.Status != nil {
		setParts = append(setParts, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, *input.Status)
		argIndex++
	}
	if input.ProfileID != nil {
		setParts = append(setParts, fmt.Sprintf("profile_id = $%d", argIndex))
		args = append(args, *input.ProfileID)
		argIndex++
	}

	if len(setParts) == 0 {
		return nil
	}

	var jobQueryBuilder strings.Builder
	jobQueryBuilder.WriteString("UPDATE scan_jobs SET ")
	jobQueryBuilder.WriteString(strings.Join(setParts, ", "))
	jobQueryBuilder.WriteString(" WHERE id = $")
	jobQueryBuilder.WriteString(strconv.Itoa(argIndex))

	args = append(args, id)
	if _, err := tx.ExecContext(ctx, jobQueryBuilder.String(), args...); err != nil {
		return sanitizeDBError("update scan job", err)
	}
	return nil
}

// UpdateScan updates an existing scan.
func (r *ScanRepository) UpdateScan(ctx context.Context, id uuid.UUID, input UpdateScanInput) (*Scan, error) {
	tx, err := r.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			slog.Warn("error rolling back transaction", "error", err)
		}
	}()

	var exists bool
	if err = tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE id = $1)", id).Scan(&exists); err != nil {
		return nil, sanitizeDBError("check scan existence", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("scan", id.String())
	}

	if err := updateScanNetwork(ctx, tx, id, input); err != nil {
		return nil, err
	}
	if err := updateScanJob(ctx, tx, id, input); err != nil {
		return nil, err
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return r.GetScan(ctx, id)
}

// DeleteScan deletes a scan by ID.
func (r *ScanRepository) DeleteScan(ctx context.Context, id uuid.UUID) error {
	tx, err := r.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			slog.Warn("error rolling back transaction", "error", err)
		}
	}()

	var exists bool
	if err = tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM scan_jobs WHERE id = $1)", id).Scan(&exists); err != nil {
		return sanitizeDBError("check scan existence", err)
	}
	if !exists {
		return errors.ErrNotFoundWithID("scan", id.String())
	}

	var status string
	if err = tx.QueryRowContext(ctx, "SELECT status FROM scan_jobs WHERE id = $1", id).Scan(&status); err != nil {
		return sanitizeDBError("get scan status", err)
	}
	if status == "running" {
		return errors.ErrConflictWithReason("scan", "cannot delete a running scan; stop it first")
	}

	if _, err = tx.ExecContext(ctx, "DELETE FROM scan_jobs WHERE id = $1", id); err != nil {
		return sanitizeDBError("delete scan job", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetScanResults retrieves scan results with pagination.
func (r *ScanRepository) GetScanResults(ctx context.Context, scanID uuid.UUID, offset, limit int) (
	[]*ScanResult, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM port_scans ps WHERE ps.job_id = $1`, scanID,
	).Scan(&total); err != nil {
		return nil, 0, sanitizeDBError("get scan results count", err)
	}

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

	rows, err := r.db.QueryContext(ctx, query, scanID, limit, offset)
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

		if err := rows.Scan(
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
		); err != nil {
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
func (r *ScanRepository) GetScanSummary(ctx context.Context, scanID uuid.UUID) (*ScanSummary, error) {
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

	summary := &ScanSummary{ScanID: scanID}

	var durationSeconds *int
	err := r.db.QueryRowContext(ctx, query, scanID).Scan(
		&summary.TotalHosts,
		&summary.TotalPorts,
		&summary.OpenPorts,
		&summary.ClosedPorts,
		&durationSeconds,
	)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
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
func (r *ScanRepository) StartScan(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE scan_jobs
		SET status = 'running', started_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`, id)
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
func (r *ScanRepository) CompleteScan(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE scan_jobs
		SET status = 'completed', completed_at = NOW()
		WHERE id = $1 AND status = 'running'
	`, id)
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
func (r *ScanRepository) StopScan(ctx context.Context, id uuid.UUID, errMsg ...string) error {
	var msg *string
	if len(errMsg) > 0 && errMsg[0] != "" {
		msg = &errMsg[0]
	}

	result, err := r.db.ExecContext(ctx, `
		UPDATE scan_jobs
		SET status = 'failed', completed_at = NOW(), error_message = COALESCE($2, error_message)
		WHERE id = $1 AND status = 'running'
	`, id, msg)
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
