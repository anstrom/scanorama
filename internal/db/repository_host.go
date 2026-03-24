// Package db provides typed repository for host-related database operations.
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
			os_family, os_name, os_version, os_confidence, status, discovery_method,
			response_time_ms, discovery_count
		)
		VALUES (
			:id, :ip_address, :hostname, :mac_address, :vendor,
			:os_family, :os_name, :os_version, :os_confidence, :status, :discovery_method,
			:response_time_ms, :discovery_count
		)
		ON CONFLICT (ip_address)
		DO UPDATE SET
			hostname = EXCLUDED.hostname,
			mac_address = COALESCE(EXCLUDED.mac_address, hosts.mac_address),
			vendor = COALESCE(EXCLUDED.vendor, hosts.vendor),
			os_family = COALESCE(EXCLUDED.os_family, hosts.os_family),
			os_name = COALESCE(EXCLUDED.os_name, hosts.os_name),
			os_version = COALESCE(EXCLUDED.os_version, hosts.os_version),
			os_confidence = COALESCE(EXCLUDED.os_confidence, hosts.os_confidence),
			status = EXCLUDED.status,
			discovery_method = EXCLUDED.discovery_method,
			response_time_ms = EXCLUDED.response_time_ms,
			discovery_count = hosts.discovery_count + 1,
			last_seen = NOW()
		RETURNING
			first_seen, last_seen`

	if host.ID == uuid.Nil {
		host.ID = uuid.New()
	}

	rows, err := r.db.NamedQueryContext(ctx, query, host)
	if err != nil {
		return sanitizeDBError("create or update host", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "error", err)
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
	query := `
		SELECT *
		FROM hosts
		WHERE ip_address = $1`

	if err := r.db.GetContext(ctx, &host, query, ip); err != nil {
		return nil, sanitizeDBError("get host", err)
	}

	return &host, nil
}

// GetActiveHosts retrieves all active hosts.
func (r *HostRepository) GetActiveHosts(ctx context.Context) ([]*ActiveHost, error) {
	var hosts []*ActiveHost
	query := `
		SELECT *
		FROM active_hosts
		ORDER BY ip_address`

	if err := r.db.SelectContext(ctx, &hosts, query); err != nil {
		return nil, sanitizeDBError("get active hosts", err)
	}

	return hosts, nil
}

// HostFilters represents filters for listing hosts.
type HostFilters struct {
	Status    string
	OSFamily  string
	Network   string
	Search    string // searches ip_address and hostname
	SortBy    string // column to sort by: ip_address, hostname, os_family, status, last_seen, first_seen
	SortOrder string // asc or desc
}

// validHostSortColumns is the allowlist of columns that may be used in ORDER BY.
var validHostSortColumns = map[string]string{
	"ip_address": "h.ip_address",
	"hostname":   "h.hostname",
	"os_family":  "h.os_family",
	"status":     "h.status",
	"last_seen":  "h.last_seen",
	"first_seen": "h.first_seen",
}

// ListHosts retrieves hosts with filtering and pagination.
func (r *HostRepository) ListHosts(
	ctx context.Context, filters *HostFilters, offset, limit int,
) ([]*Host, int64, error) {
	// Build the base query with joins.
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
			COUNT(DISTINCT ps.id) as total_ports_scanned,
			COUNT(DISTINCT sj2.id) FILTER (WHERE sj2.status = 'completed') as scan_count
		FROM hosts h
		LEFT JOIN port_scans ps ON h.id = ps.host_id
		LEFT JOIN port_scans ps2 ON h.id = ps2.host_id
		LEFT JOIN scan_jobs sj2 ON sj2.id = ps2.job_id
	`

	// Build WHERE clause and arguments.
	whereClause, args := buildHostFilters(filters)

	// Get total count.
	total, err := r.getHostCount(ctx, whereClause, args)
	if err != nil {
		return nil, 0, err
	}

	// Add GROUP BY clause.
	groupByClause := `
		GROUP BY h.id, h.ip_address, h.hostname, h.mac_address, h.vendor, h.os_family,
			h.os_name, h.os_version, h.os_confidence, h.discovery_method,
			h.response_time_ms, h.ignore_scanning, h.first_seen, h.last_seen, h.status
	`

	// Resolve ORDER BY clause from validated sort parameters.
	orderCol := "h.last_seen"
	if col, ok := validHostSortColumns[filters.SortBy]; ok {
		orderCol = col
	}
	orderDir := "DESC"
	if strings.EqualFold(filters.SortOrder, "ASC") {
		orderDir = "ASC"
	}
	orderClause := fmt.Sprintf(" ORDER BY %s %s NULLS LAST", orderCol, orderDir)

	// Combine query parts.
	fullQuery := baseQuery + whereClause + groupByClause + orderClause + " LIMIT $" +
		fmt.Sprintf("%d", len(args)+1) + " OFFSET $" + fmt.Sprintf("%d", len(args)+2)

	args = append(args, limit, offset)

	// Execute query.
	rows, err := r.db.QueryContext(ctx, fullQuery, args...)
	if err != nil {
		return nil, 0, sanitizeDBError("execute hosts query", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "error", err)
		}
	}()

	hosts, err := r.scanHostRows(rows)
	if err != nil {
		return nil, 0, err
	}

	return hosts, total, nil
}

// CreateHost creates a new host record.
func (r *HostRepository) CreateHost(ctx context.Context, input CreateHostInput) (*Host, error) {
	if input.IPAddress == "" {
		return nil, fmt.Errorf("ip_address is required")
	}

	hostID := uuid.New()
	now := time.Now().UTC()

	// Start with required columns.
	columns := []string{"id", "ip_address", "first_seen", "last_seen", "ignore_scanning"}
	placeholders := []string{"$1", "$2", "$3", "$4", "$5"}
	args := []interface{}{hostID, input.IPAddress, now, now, input.IgnoreScanning}
	argIndex := 6

	// Add optional string fields only when non-empty.
	addStr := func(col, val string) {
		if val != "" {
			columns = append(columns, col)
			placeholders = append(placeholders, fmt.Sprintf("$%d", argIndex))
			args = append(args, val)
			argIndex++
		}
	}

	addStr("hostname", input.Hostname)
	addStr("vendor", input.Vendor)
	addStr("os_family", input.OSFamily)
	addStr("os_name", input.OSName)
	addStr("os_version", input.OSVersion)
	addStr("status", input.Status)

	query := fmt.Sprintf(
		"INSERT INTO hosts (%s) VALUES (%s)",
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		// Check for PostgreSQL constraint violations.
		if pqErr := pq.As(err, pqerror.UniqueViolation); pqErr != nil {
			if pqErr.Constraint == "unique_ip_address" {
				return nil, errors.ErrConflictWithReason("host",
					fmt.Sprintf("IP address %s already exists", input.IPAddress))
			}
		}
		return nil, sanitizeDBError("create host", err)
	}

	// Return the created host.
	return r.GetHost(ctx, hostID)
}

// fetchHostPorts runs the DISTINCT ON query to retrieve the latest known state
// for each (port, protocol) pair on the given host, and populates host.Ports
// and host.TotalPorts.
func (r *HostRepository) fetchHostPorts(ctx context.Context, hostID uuid.UUID, host *Host) error {
	portsQuery := `
		SELECT DISTINCT ON (port, protocol)
			port,
			protocol,
			state,
			COALESCE(service_name, '') AS service_name,
			scanned_at
		FROM port_scans
		WHERE host_id = $1
		ORDER BY port, protocol, scanned_at DESC
	`

	portRows, err := r.db.QueryContext(ctx, portsQuery, hostID)
	if err != nil {
		return sanitizeDBError("query host ports", err)
	}
	defer func() {
		if closeErr := portRows.Close(); closeErr != nil {
			slog.Warn("failed to close rows", "error", closeErr)
		}
	}()

	for portRows.Next() {
		var pi PortInfo
		if err := portRows.Scan(&pi.Port, &pi.Protocol, &pi.State, &pi.Service, &pi.LastSeen); err != nil {
			return fmt.Errorf("failed to scan port row: %w", err)
		}
		host.TotalPorts++
		host.Ports = append(host.Ports, pi)
	}
	if err := portRows.Err(); err != nil {
		return fmt.Errorf("failed to iterate port rows: %w", err)
	}

	return nil
}

// fetchHostScanCount queries the number of completed scan jobs that produced
// results for the given host and stores the count in host.ScanCount.
// Failures are non-fatal and are logged rather than returned.
func (r *HostRepository) fetchHostScanCount(ctx context.Context, hostID uuid.UUID, host *Host) {
	scanCountQuery := `
    SELECT COUNT(DISTINCT sj.id)
    FROM scan_jobs sj
    JOIN port_scans ps ON ps.job_id = sj.id
    WHERE ps.host_id = $1
      AND sj.status = 'completed'
`
	if err := r.db.QueryRowContext(ctx, scanCountQuery, hostID).Scan(&host.ScanCount); err != nil {
		// non-fatal — log and continue
		slog.Warn("failed to query scan count for host", "host_id", hostID, "error", err)
	}
}

// GetHost retrieves a host by ID.
func (r *HostRepository) GetHost(ctx context.Context, id uuid.UUID) (*Host, error) {
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

	err := r.db.QueryRowContext(ctx, query, id).Scan(
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
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, errors.ErrNotFoundWithID("host", id.String())
		}
		return nil, sanitizeDBError("get host", err)
	}

	// Convert IP address.
	host.IPAddress = IPAddr{IP: net.ParseIP(ipAddress)}

	// Handle nullable fields using helper functions.
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

	if err := r.fetchHostPorts(ctx, id, host); err != nil {
		return nil, err
	}

	r.fetchHostScanCount(ctx, id, host)

	return host, nil
}

// UpdateHost updates an existing host.
func (r *HostRepository) UpdateHost(ctx context.Context, id uuid.UUID, input UpdateHostInput) (*Host, error) {
	// Start a transaction.
	tx, err := r.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			slog.Warn("error rolling back transaction", "error", err)
		}
	}()

	// Check if the host exists.
	var exists bool
	err = tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM hosts WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return nil, sanitizeDBError("check host existence", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("host", id.String())
	}

	// Build dynamic SET clause from non-nil pointer fields.
	var setParts []string
	var args []interface{}
	argIndex := 1

	addStr := func(col string, val *string) {
		if val != nil {
			setParts = append(setParts, fmt.Sprintf("%s = $%d", col, argIndex))
			args = append(args, *val)
			argIndex++
		}
	}

	addStr("hostname", input.Hostname)
	addStr("vendor", input.Vendor)
	addStr("os_family", input.OSFamily)
	addStr("os_name", input.OSName)
	addStr("os_version", input.OSVersion)
	addStr("status", input.Status)

	if input.IgnoreScanning != nil {
		setParts = append(setParts, fmt.Sprintf("ignore_scanning = $%d", argIndex))
		args = append(args, *input.IgnoreScanning)
		argIndex++
	}

	// If no fields to update, return error.
	if len(setParts) == 0 {
		return nil, fmt.Errorf("no valid fields to update")
	}

	// Always update the last_seen timestamp.
	setParts = append(setParts, "last_seen = NOW()")

	// Build and execute update query safely.
	var queryBuilder strings.Builder
	queryBuilder.WriteString("UPDATE hosts SET ")
	queryBuilder.WriteString(strings.Join(setParts, ", "))
	queryBuilder.WriteString(" WHERE id = $")
	queryBuilder.WriteString(strconv.Itoa(argIndex))
	updateQuery := queryBuilder.String()

	args = append(args, id)

	_, err = tx.ExecContext(ctx, updateQuery, args...)
	if err != nil {
		return nil, sanitizeDBError("update host", err)
	}

	// Commit transaction.
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Retrieve and return the updated host.
	return r.GetHost(ctx, id)
}

// DeleteHost deletes a host by ID.
func (r *HostRepository) DeleteHost(ctx context.Context, id uuid.UUID) error {
	// Start a transaction.
	tx, err := r.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil {
			slog.Warn("error rolling back transaction", "error", err)
		}
	}()

	// Check if the host exists.
	var exists bool
	err = tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM hosts WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return sanitizeDBError("check host existence", err)
	}
	if !exists {
		return errors.ErrNotFoundWithID("host", id.String())
	}

	// Delete the host (CASCADE will handle related port_scans, services, etc.).
	_, err = tx.ExecContext(ctx, "DELETE FROM hosts WHERE id = $1", id)
	if err != nil {
		return sanitizeDBError("delete host", err)
	}

	// Commit transaction.
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// getHostScansCount gets total count of scans for a specific host.
func (r *HostRepository) getHostScansCount(ctx context.Context, hostID uuid.UUID) (int64, error) {
	countQuery := `
		SELECT COUNT(DISTINCT sj.id)
		FROM scan_jobs sj
		LEFT JOIN networks n ON sj.network_id = n.id
		WHERE (n.cidr IS NOT NULL AND n.cidr >>= (SELECT ip_address FROM hosts WHERE id = $1))
		   OR sj.id IN (
			   SELECT DISTINCT ps.job_id
			   FROM port_scans ps
			   WHERE ps.host_id = $1
		   )
	`

	var total int64
	err := r.db.QueryRowContext(ctx, countQuery, hostID).Scan(&total)
	if err != nil {
		return 0, sanitizeDBError("get host scans count", err)
	}
	return total, nil
}

// processHostScanRow processes a single host scan row from query results.
func processHostScanRow(rows *sql.Rows) (*Scan, error) {
	scan := &Scan{}
	var scheduleID sql.NullInt64
	var profileID sql.NullString
	var description sql.NullString
	var startedAt sql.NullTime
	var completedAt sql.NullTime
	var options string
	// network::text returns a single CIDR string — scan into a plain string
	// and wrap it in a slice afterward.
	var targetsStr string
	// '[]'::jsonb comes back from the driver as raw []byte — scan into that
	// and unmarshal afterward; scanning directly into *[]string is unsupported.
	var tagsJSON []byte

	err := rows.Scan(
		&scan.ID,
		&scan.Name,
		&description,
		&targetsStr,
		&scan.ScanType,
		&scan.Ports,
		&profileID,
		&scan.Status,
		&startedAt,
		&completedAt,
		&scan.CreatedAt,
		&scheduleID,
		&tagsJSON,
		&options,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan host scan row: %w", err)
	}

	// Handle nullable fields.
	scan.Targets = []string{targetsStr}

	// Unmarshal tags JSON; default to empty slice on null/invalid.
	if len(tagsJSON) > 0 {
		if err := json.Unmarshal(tagsJSON, &scan.Tags); err != nil {
			scan.Tags = []string{}
		}
	} else {
		scan.Tags = []string{}
	}

	if description.Valid {
		scan.Description = description.String
	}
	if profileID.Valid {
		scan.ProfileID = &profileID.String
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
func (r *HostRepository) GetHostScans(
	ctx context.Context, hostID uuid.UUID, offset, limit int,
) ([]*Scan, int64, error) {
	// Get total count.
	total, err := r.getHostScansCount(ctx, hostID)
	if err != nil {
		return nil, 0, err
	}

	// Get the actual scans with pagination.
	query := `
		SELECT DISTINCT
			sj.id,
			COALESCE(sj.execution_details->>'name', n.name, '')           AS name,
			COALESCE(sj.execution_details->>'description', n.description) AS description,
			COALESCE(n.cidr::text, sj.execution_details->'scan_targets'->>0, '') AS targets,
			COALESCE(sp.scan_type, sj.execution_details->>'scan_type', n.scan_type, 'connect') AS scan_type,
			COALESCE(sj.execution_details->>'ports', n.scan_ports, '')    AS ports,
			sj.profile_id,
			sj.status,
			sj.started_at,
			sj.completed_at,
			sj.created_at,
			COALESCE(sch.id, NULL) AS schedule_id,
			'[]'::jsonb AS tags,
			'{}' AS options
		FROM scan_jobs sj
		LEFT JOIN networks n ON sj.network_id = n.id
		LEFT JOIN scan_profiles sp ON sj.profile_id = sp.id
		LEFT JOIN scheduled_jobs sch ON n.id::text = sch.config->>'network_id'
		WHERE (n.cidr IS NOT NULL AND n.cidr >>= (SELECT ip_address FROM hosts WHERE id = $1))
		   OR sj.id IN (
			   SELECT DISTINCT ps.job_id
			   FROM port_scans ps
			   WHERE ps.host_id = $1
		   )
		ORDER BY sj.created_at DESC
		OFFSET $2 LIMIT $3
	`

	rows, err := r.db.QueryContext(ctx, query, hostID, offset, limit)
	if err != nil {
		return nil, 0, sanitizeDBError("get host scans", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("error closing rows", "error", err)
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

// assignStringPtr assigns a nullable string pointer to a target string pointer.
func assignStringPtr(target **string, source *string) {
	if source != nil && *source != "" {
		*target = source
	}
}

// assignMACAddress assigns a nullable MAC address string to a target MACAddr.
func assignMACAddress(target **MACAddr, source *string) {
	if source != nil && *source != "" {
		if mac, err := net.ParseMAC(*source); err == nil {
			macAddr := MACAddr{HardwareAddr: mac}
			*target = &macAddr
		}
	}
}

// assignIntPtr assigns a nullable int pointer to a target int pointer.
func assignIntPtr(target **int, source *int) {
	if source != nil {
		*target = source
	}
}

// assignBoolFromPtr assigns a nullable bool pointer to a target bool.
func assignBoolFromPtr(target, source *bool) {
	if source != nil {
		*target = *source
	}
}

// buildHostFilters creates WHERE clause and args for host filtering.
func buildHostFilters(filters *HostFilters) (whereClause string, args []interface{}) {
	var conditions []filterCondition

	if filters.Status != "" {
		conditions = append(conditions, filterCondition{"h.status", filters.Status})
	}
	if filters.OSFamily != "" {
		conditions = append(conditions, filterCondition{"h.os_family", filters.OSFamily})
	}
	whereClause, args = buildWhereClause(conditions)

	if filters.Network != "" {
		paramIdx := len(args) + 1
		networkFragment := fmt.Sprintf("h.ip_address <<= $%d", paramIdx)
		if whereClause == "" {
			whereClause = fmt.Sprintf("WHERE %s", networkFragment)
		} else {
			whereClause += fmt.Sprintf(" AND %s", networkFragment)
		}
		args = append(args, filters.Network)
	}

	// Search is an ILIKE across ip_address (cast to text) and hostname.
	// It cannot be represented as a simple equality filterCondition, so we
	// append it manually after the standard conditions have been built.
	if filters.Search != "" {
		paramIdx := len(args) + 1
		pattern := "%" + filters.Search + "%"
		searchFragment := fmt.Sprintf(
			" AND (h.ip_address::text ILIKE $%d OR h.hostname ILIKE $%d)",
			paramIdx, paramIdx,
		)
		if whereClause == "" {
			whereClause = fmt.Sprintf(
				"WHERE (h.ip_address::text ILIKE $%d OR h.hostname ILIKE $%d)",
				paramIdx, paramIdx,
			)
		} else {
			whereClause += searchFragment
		}
		args = append(args, pattern)
	}

	return whereClause, args
}

// getHostCount gets total count of hosts matching filters.
func (r *HostRepository) getHostCount(ctx context.Context, whereClause string, args []interface{}) (int64, error) {
	countQuery := fmt.Sprintf(`
		SELECT COUNT(DISTINCT h.id)
		FROM hosts h
		LEFT JOIN port_scans ps ON h.id = ps.host_id
		%s
	`, whereClause)

	var total int64
	err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total)
	if err != nil {
		return 0, sanitizeDBError("get host count", err)
	}
	return total, nil
}

// scanHostRows processes query result rows into Host structs.
func (r *HostRepository) scanHostRows(rows *sql.Rows) ([]*Host, error) {
	var hosts []*Host
	for rows.Next() {
		host := &Host{}
		var ipAddress string
		var hostname, macAddressStr, vendor, osFamily, osName, osVersion *string
		var osConfidence, responseTimeMS *int
		var discoveryMethod *string
		var ignoreScanning *bool
		var openPorts, totalPortsScanned int64
		var scanCount int64

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
			&scanCount,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan host row: %w", err)
		}

		// Convert IP address.
		host.IPAddress = IPAddr{IP: net.ParseIP(ipAddress)}

		// Handle nullable fields using helper functions.
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

		// Wire the aggregated port counts from the list query.
		// These are counts across all scan jobs — not latest-state — so they
		// are only used for the list view summary numbers, not the detail panel.
		host.TotalPorts = int(totalPortsScanned)
		host.ScanCount = int(scanCount)

		hosts = append(hosts, host)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate host rows: %w", err)
	}

	return hosts, nil
}
