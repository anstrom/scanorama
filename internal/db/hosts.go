// Package db provides host-related database operations for scanorama.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

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
			hostname = EXCLUDED.hostname,
			mac_address = COALESCE(EXCLUDED.mac_address, hosts.mac_address),
			vendor = COALESCE(EXCLUDED.vendor, hosts.vendor),
			os_family = COALESCE(EXCLUDED.os_family, hosts.os_family),
			os_version = COALESCE(EXCLUDED.os_version, hosts.os_version),
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
	Status   string
	OSFamily string
	Network  string
}

// ListHosts retrieves hosts with filtering and pagination.
func (db *DB) ListHosts(ctx context.Context, filters HostFilters, offset, limit int) ([]*Host, int64, error) {
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
			COUNT(DISTINCT ps.id) as total_ports_scanned
		FROM hosts h
		LEFT JOIN port_scans ps ON h.id = ps.host_id
	`

	// Build WHERE clause and arguments.
	whereClause, args := buildHostFilters(filters)

	// Get total count.
	total, err := db.getHostCount(ctx, whereClause, args)
	if err != nil {
		return nil, 0, err
	}

	// Add GROUP BY clause.
	groupByClause := `
		GROUP BY h.id, h.ip_address, h.hostname, h.mac_address, h.vendor, h.os_family,
			h.os_name, h.os_version, h.os_confidence, h.discovery_method,
			h.response_time_ms, h.ignore_scanning, h.first_seen, h.last_seen, h.status
	`

	// Combine query parts.
	fullQuery := baseQuery + whereClause + groupByClause + " ORDER BY h.last_seen DESC LIMIT $" +
		fmt.Sprintf("%d", len(args)+1) + " OFFSET $" + fmt.Sprintf("%d", len(args)+2)

	args = append(args, limit, offset)

	// Execute query.
	rows, err := db.QueryContext(ctx, fullQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to execute hosts query: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
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

	// Extract required IP address.
	ipAddress, ok := data["ip_address"].(string)
	if !ok {
		return nil, fmt.Errorf("ip_address is required")
	}

	hostID := uuid.New()
	now := time.Now().UTC()

	// Field mappings for optional fields.
	fieldMappings := map[string]string{
		"hostname":        "hostname",
		"vendor":          "vendor",
		"os_family":       "os_family",
		"os_name":         "os_name",
		"os_version":      "os_version",
		"ignore_scanning": "ignore_scanning",
		"status":          "status",
	}

	// Build dynamic insert with optional fields.
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
		// Check for PostgreSQL constraint violations.
		if pqErr, ok := err.(*pq.Error); ok {
			if pqErr.Code == "23505" && pqErr.Constraint == "unique_ip_address" {
				return nil, errors.ErrConflictWithReason("host", fmt.Sprintf("IP address %s already exists", ipAddress))
			}
		}
		return nil, fmt.Errorf("failed to create host: %w", err)
	}

	// Return the created host.
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

	return host, nil
}

// UpdateHost updates an existing host.
func (db *DB) UpdateHost(ctx context.Context, id uuid.UUID, hostData interface{}) (*Host, error) {
	// Convert the interface{} to host data.
	data, ok := hostData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid host data format")
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

	// Check if the host exists.
	var exists bool
	err = tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM hosts WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return nil, fmt.Errorf("failed to check host existence: %w", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("host", id.String())
	}

	// Build dynamic update query using field mapping.
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

	// If no fields to update, return error.
	if len(setParts) == 0 {
		return nil, fmt.Errorf("no valid fields to update")
	}

	// Always update the last_seen timestamp.
	setParts = append(setParts, "last_seen = NOW()")

	// Build and execute update query safely.
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

	// Commit transaction.
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Retrieve and return the updated host.
	return db.GetHost(ctx, id)
}

// DeleteHost deletes a host by ID.
func (db *DB) DeleteHost(ctx context.Context, id uuid.UUID) error {
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

	// Check if the host exists.
	var exists bool
	err = tx.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM hosts WHERE id = $1)", id).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check host existence: %w", err)
	}
	if !exists {
		return errors.ErrNotFoundWithID("host", id.String())
	}

	// Delete the host (CASCADE will handle related port_scans, services, etc.).
	_, err = tx.ExecContext(ctx, "DELETE FROM hosts WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete host: %w", err)
	}

	// Commit transaction.
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// getHostScansCount gets total count of scans for a specific host.
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

// processHostScanRow processes a single host scan row from query results.
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

	// Handle nullable fields.
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
	// Get total count.
	total, err := db.getHostScansCount(ctx, hostID)
	if err != nil {
		return nil, 0, err
	}

	// Get the actual scans with pagination.
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

// getHostCount gets total count of hosts matching filters.
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

// scanHostRows processes query result rows into Host structs.
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

		hosts = append(hosts, host)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate host rows: %w", err)
	}

	return hosts, nil
}
