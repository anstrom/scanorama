// Package db provides typed repository for scan-related database operations.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"

	"github.com/anstrom/scanorama/internal/errors"
)

// sort direction constants used in ORDER BY clauses throughout this package.
const (
	sortOrderASC  = "ASC"
	sortOrderDESC = "DESC"
)

// isHostTarget reports whether target represents a single host: a bare IP
// address, a /32 (IPv4) / /128 (IPv6) CIDR, or a DNS hostname.
// Such targets must not be stored as rows in the networks table;
// they are passed directly to the scan runner via execution_details.
func isHostTarget(target string) bool {
	if net.ParseIP(target) != nil {
		return true // bare IP — always maps to /32 or /128
	}
	_, ipNet, err := net.ParseCIDR(target)
	if err != nil {
		// Not an IP or CIDR — it must be a hostname (already validated by the
		// service layer before reaching the DB).  Hostnames are host-scoped,
		// not network-scoped, so we treat them like bare IPs.
		return true
	}
	ones, bits := ipNet.Mask.Size()
	return ones == bits // /32 for IPv4, /128 for IPv6
}

// allHostTargets reports whether every entry in targets is a single host.
func allHostTargets(targets []string) bool {
	for _, t := range targets {
		if !isHostTarget(t) {
			return false
		}
	}
	return true
}

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
		scanTypeExpr := "COALESCE(sp.scan_type, sj.execution_details->>'scan_type', n.scan_type)"
		conditions = append(conditions, filterCondition{scanTypeExpr, filters.ScanType})
	}
	if filters.ProfileID != nil {
		conditions = append(conditions, filterCondition{"sj.profile_id", *filters.ProfileID})
	}

	return buildWhereClause(conditions)
}

// scanCountBaseQuery is the invariant part of the COUNT query used by getScanCount.
const scanCountBaseQuery = `SELECT COUNT(*) FROM scan_jobs sj ` +
	`LEFT JOIN networks n ON sj.network_id = n.id ` +
	`LEFT JOIN scan_profiles sp ON sj.profile_id = sp.id`

// getScanCount gets total count of scans matching filters.
// whereClause is produced by buildWhereClause and contains only parameterized
// placeholders ($N) with hardcoded column names — no user data is concatenated.
func (r *ScanRepository) getScanCount(ctx context.Context, whereClause string, args []interface{}) (int64, error) {
	q := scanCountBaseQuery
	if whereClause != "" {
		q += " " + whereClause
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, q, args...).Scan(&total); err != nil {
		return 0, sanitizeDBError("get scan count", err)
	}
	return total, nil
}

// processScanRow processes a single scan row from query results.
func processScanRow(rows *sql.Rows) (*Scan, error) {
	scan := &Scan{}
	var name sql.NullString
	var networkCIDR sql.NullString
	var scanType sql.NullString
	var ports sql.NullString
	var profileID *string
	var description sql.NullString
	var execDetailsStr sql.NullString

	err := rows.Scan(
		&scan.ID,
		&name,
		&description,
		&networkCIDR,
		&scanType,
		&ports,
		&profileID,
		&scan.Status,
		&scan.CreatedAt,
		&scan.StartedAt,
		&scan.CompletedAt,
		&scan.ErrorMessage,
		&execDetailsStr,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	if description.Valid {
		scan.Description = description.String
	}

	scan.Name = name.String
	scan.ScanType = scanType.String
	scan.Ports = ports.String
	scan.Targets = scanTargetsFromExecDetails(execDetailsStr.String, networkCIDR.String)

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
var validScanSortColumns = map[string]string{
	"status":       "sj.status",
	"created_at":   "sj.created_at",
	"started_at":   "sj.started_at",
	"completed_at": "sj.completed_at",
	"scan_type":    "COALESCE(sp.scan_type, sj.execution_details->>'scan_type', n.scan_type, '')",
}

func (r *ScanRepository) ListScans(
	ctx context.Context, filters ScanFilters, offset, limit int,
) ([]*Scan, int64, error) {
	baseQuery := `
		SELECT
			sj.id,
			COALESCE(sj.execution_details->>'name', n.name, '')           AS name,
			COALESCE(sj.execution_details->>'description', n.description) AS description,
			n.cidr::text                                                   AS network_cidr,
			COALESCE(sp.scan_type, sj.execution_details->>'scan_type', n.scan_type, '') AS scan_type,
			COALESCE(sj.execution_details->>'ports', n.scan_ports, '')    AS ports,
			sj.profile_id,
			sj.status,
			sj.created_at,
			sj.started_at,
			sj.completed_at,
			sj.error_message,
			sj.execution_details::text                                     AS execution_details
		FROM scan_jobs sj
		LEFT JOIN networks n ON sj.network_id = n.id
		LEFT JOIN scan_profiles sp ON sj.profile_id = sp.id
	`

	whereClause, args := buildScanFilters(filters)

	total, err := r.getScanCount(ctx, whereClause, args)
	if err != nil {
		return nil, 0, err
	}

	argIndex := len(args)
	orderByClause := " ORDER BY sj.created_at DESC NULLS LAST"
	if filters.SortBy != "" {
		if col, ok := validScanSortColumns[filters.SortBy]; ok {
			dir := sortOrderASC
			if strings.EqualFold(filters.SortOrder, sortOrderDESC) {
				dir = sortOrderDESC
			}
			orderByClause = fmt.Sprintf(" ORDER BY %s %s NULLS LAST", col, dir)
		}
	}
	listQuery := fmt.Sprintf("%s %s%s LIMIT $%d OFFSET $%d",
		baseQuery, whereClause, orderByClause, argIndex+1, argIndex+2)
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

	scans := make([]*Scan, 0)
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
	// Use a savepoint so that a name-collision error (unique_violation on
	// networks_name_key) can be recovered from without aborting the outer
	// transaction. PostgreSQL marks a transaction as aborted on any error;
	// ROLLBACK TO SAVEPOINT restores a clean state for the retry.
	if _, err = tx.ExecContext(ctx, "SAVEPOINT sp_insert_network"); err != nil {
		return uuid.Nil, sanitizeDBError("savepoint before create network", err)
	}
	// ON CONFLICT (cidr) handles concurrent inserts for the same CIDR by
	// returning the winning row's id via RETURNING.
	id, err = insertNetwork(ctx, tx, id, name, cidr, description, ports, scanType)
	if err != nil {
		// Roll back to the clean state before inspecting the error — the
		// transaction is aborted after any error and must be restored before
		// any further SQL can run.
		if _, rbErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT sp_insert_network"); rbErr != nil {
			return uuid.Nil, sanitizeDBError("rollback savepoint after create network", rbErr)
		}
		// If the requested name is already taken by a different network,
		// fall back to using the CIDR string as the network name.
		if pqErr := pq.As(err); pqErr != nil &&
			pqErr.Code == pqerror.UniqueViolation &&
			pqErr.Constraint == "networks_name_key" {
			id, err = insertNetwork(ctx, tx, uuid.New(), cidr, cidr, description, ports, scanType)
		}
		if err != nil {
			return uuid.Nil, sanitizeDBError("create network", err)
		}
	}
	if _, err = tx.ExecContext(ctx, "RELEASE SAVEPOINT sp_insert_network"); err != nil {
		return uuid.Nil, sanitizeDBError("release savepoint after create network", err)
	}
	return id, nil
}

// insertNetwork executes the upsert that creates or reclaims a network row by CIDR.
// Returns the actual UUID from RETURNING id (may differ from the input id when
// ON CONFLICT fires and the existing row's id is returned instead).
func insertNetwork(ctx context.Context, tx *sql.Tx,
	id uuid.UUID, name, cidr, description, ports, scanType string) (uuid.UUID, error) {
	err := tx.QueryRowContext(ctx, `
		INSERT INTO networks (
			id, name, cidr, description,
			scan_ports, scan_type,
			is_active, scan_enabled, discovery_method
		) VALUES (
			$1, $2, $3, $4, $5, $6, true, true, 'tcp'
		)
		ON CONFLICT (cidr) DO UPDATE SET name = EXCLUDED.name
		RETURNING id
	`, id, name, cidr, description, ports, scanType).Scan(&id)
	return id, err
}

// createScanJob inserts a single scan_jobs row.
//
// networkID may be nil for host-targeted scans (all targets are /32 or /128).
// When networkID is nil, the full scan metadata (name, description, ports,
// scan_type) is stored in execution_details so GetScan can reconstruct the
// complete record without a networks JOIN.
//
// When networkID is non-nil and allTargets contains more than one entry, the
// full list is stored in execution_details["scan_targets"] so that GetScan
// returns every target instead of only the primary network CIDR.
func createScanJob(ctx context.Context, tx *sql.Tx, jobID uuid.UUID, networkID *uuid.UUID,
	profileID *string, now time.Time, osDetection bool, allTargets []string,
	name, description, ports, scanType string, source *string) error {
	details := map[string]interface{}{
		"os_detection": osDetection,
	}

	// Always persist the target list for host-only scans; also persist when
	// there are multiple network targets (single-network scans don't need it
	// because GetScan can read the CIDR directly from the networks JOIN).
	if networkID == nil || len(allTargets) > 1 {
		details["scan_targets"] = allTargets
	}

	// For host-only scans there is no networks row, so embed all scan metadata
	// in execution_details so it is available to GetScan and ListScans.
	if networkID == nil {
		details["name"] = name
		details["description"] = description
		details["ports"] = ports
		details["scan_type"] = scanType
	}

	execDetailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("marshal execution details: %w", err)
	}

	// Pass nil as the network_id argument when there is no linked network so
	// the driver sends a proper SQL NULL (uuid.Nil would insert a zero UUID).
	var networkIDArg interface{}
	if networkID != nil {
		networkIDArg = *networkID
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO scan_jobs (id, network_id, profile_id, status, created_at, execution_details, source)
		VALUES ($1, $2, $3, 'pending', $4, $5, $6)
	`, jobID, networkIDArg, profileID, now, string(execDetailsJSON), source)
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
	jobID := uuid.New()

	// Networks are CIDR ranges with a prefix length < 32 (IPv4) or < 128 (IPv6).
	// When every target is a single host (/32 / /128 or a bare IP) we must not
	// create a networks row — instead we store all metadata in execution_details
	// and leave network_id NULL.
	//
	// When the caller already knows the parent network (e.g. StartNetworkScan),
	// skip the lookup/create branch entirely and use the supplied ID directly.
	var networkID *uuid.UUID
	if input.NetworkID != nil {
		// Caller already resolved the parent network — use it directly.
		networkID = input.NetworkID
	} else if !allHostTargets(input.Targets) {
		nid, err := findOrCreateNetwork(ctx, tx, input.Name, input.Targets[0],
			input.Description, input.Ports, input.ScanType)
		if err != nil {
			return nil, err
		}
		networkID = &nid
	}

	if err := createScanJob(ctx, tx, jobID, networkID, input.ProfileID, now,
		input.OSDetection, input.Targets,
		input.Name, input.Description, input.Ports, input.ScanType, input.Source); err != nil {
		return nil, err
	}

	firstJobID := jobID

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
			COALESCE(sj.execution_details->>'name', n.name, '')                           AS name,
			COALESCE(sj.execution_details->>'description', n.description)                 AS description,
			n.cidr::text                                                                   AS network_cidr,
			COALESCE(sp.scan_type, sj.execution_details->>'scan_type', n.scan_type, '')   AS scan_type,
			COALESCE(sj.execution_details->>'ports', n.scan_ports, '')                    AS ports,
			sj.profile_id,
			sj.status,
			sj.created_at,
			sj.started_at,
			sj.completed_at,
			sj.error_message,
			COALESCE((sj.execution_details->>'os_detection')::boolean, false)              AS os_detection,
			sj.execution_details::text                                                     AS execution_details
		FROM scan_jobs sj
		LEFT JOIN networks n ON sj.network_id = n.id
		LEFT JOIN scan_profiles sp ON sj.profile_id = sp.id
		WHERE sj.id = $1
	`

	scan := &Scan{}
	var name sql.NullString
	var networkCIDR sql.NullString
	var scanType sql.NullString
	var ports sql.NullString
	var profileID *string
	var description sql.NullString
	var osDetection bool
	var execDetailsStr sql.NullString

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&scan.ID,
		&name,
		&description,
		&networkCIDR,
		&scanType,
		&ports,
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
	scan.Name = name.String
	scan.ScanType = scanType.String
	scan.Ports = ports.String
	scan.Targets = scanTargetsFromExecDetails(execDetailsStr.String, networkCIDR.String)
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
// updateHostOnlyScanDetails patches the execution_details JSON column of a
// scan_job that has no linked network row (network_id IS NULL).  It is
// automatically a no-op for network-backed scans because of the WHERE clause.
// When none of the editable metadata fields are present in input it returns
// immediately without touching the database.
func updateHostOnlyScanDetails(ctx context.Context, tx interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}, id uuid.UUID, input UpdateScanInput) error {
	if input.Name == nil && input.Description == nil && input.Ports == nil && input.ScanType == nil {
		return nil
	}

	// Convert *string to interface{}: a nil pointer becomes nil which
	// jsonb_build_object encodes as JSON null; jsonb_strip_nulls then drops
	// those keys so we never overwrite an existing value with null.
	val := func(s *string) interface{} {
		if s == nil {
			return nil
		}
		return *s
	}

	_, err := tx.ExecContext(ctx, `
		UPDATE scan_jobs
		SET execution_details = execution_details || jsonb_strip_nulls(jsonb_build_object(
			'name',        $1::text,
			'description', $2::text,
			'ports',       $3::text,
			'scan_type',   $4::text
		))
		WHERE id = $5
		  AND network_id IS NULL
	`, val(input.Name), val(input.Description), val(input.Ports), val(input.ScanType), id)
	if err != nil {
		return sanitizeDBError("update host-only scan details", err)
	}
	return nil
}

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
	if err := updateHostOnlyScanDetails(ctx, tx, id, input); err != nil {
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
			h.os_confidence,
			ps.scan_duration_ms
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

	results := make([]*ScanResult, 0)
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
			&result.ScanDurationMs,
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
		// The conditional UPDATE matched nothing. Distinguish between:
		//   (a) the scan no longer exists → NOT FOUND
		//   (b) the scan exists but is not pending → CONFLICT
		var currentStatus string
		queryErr := r.db.QueryRowContext(ctx,
			`SELECT status FROM scan_jobs WHERE id = $1`, id,
		).Scan(&currentStatus)
		if queryErr != nil {
			if stderrors.Is(queryErr, sql.ErrNoRows) {
				return errors.ErrNotFoundWithID("scan", id.String())
			}
			return sanitizeDBError("check scan status", queryErr)
		}
		return errors.ErrConflictWithReason("scan",
			fmt.Sprintf("scan is in state %q, expected %q", currentStatus, ScanJobStatusPending))
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
