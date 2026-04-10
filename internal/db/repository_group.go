// Package db provides a typed repository for host group database operations.
package db

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"

	"github.com/anstrom/scanorama/internal/errors"
)

// isNoRows reports whether err is a sql.ErrNoRows sentinel.
func isNoRows(err error) bool {
	return stderrors.Is(err, sql.ErrNoRows)
}

// GroupRepository handles host group operations.
type GroupRepository struct {
	db *DB
}

// NewGroupRepository creates a new GroupRepository.
func NewGroupRepository(db *DB) *GroupRepository {
	return &GroupRepository{db: db}
}

// CreateGroup creates a new host group.
func (r *GroupRepository) CreateGroup(ctx context.Context, input CreateGroupInput) (*HostGroup, error) {
	id := uuid.New()
	now := time.Now().UTC()

	columns := []string{"id", "name", "description", "created_at", "updated_at"}
	placeholders := []string{"$1", "$2", "$3", "$4", "$5"}
	args := []interface{}{id, input.Name, input.Description, now, now}
	argIndex := 6

	if input.Color != "" {
		columns = append(columns, "color")
		placeholders = append(placeholders, fmt.Sprintf("$%d", argIndex))
		args = append(args, input.Color)
		argIndex++
	}

	if input.FilterRule != nil {
		columns = append(columns, "filter_rule")
		placeholders = append(placeholders, fmt.Sprintf("$%d", argIndex))
		args = append(args, *input.FilterRule)
		argIndex++
	}
	_ = argIndex

	query := fmt.Sprintf(
		"INSERT INTO host_groups (%s) VALUES (%s)",
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		if pqErr := pq.As(err, pqerror.UniqueViolation); pqErr != nil {
			return nil, errors.ErrConflictWithReason("group", fmt.Sprintf("group name %q already exists", input.Name))
		}
		return nil, sanitizeDBError("create group", err)
	}
	return r.GetGroup(ctx, id)
}

// GetGroup retrieves a host group by ID, including its member count.
func (r *GroupRepository) GetGroup(ctx context.Context, id uuid.UUID) (*HostGroup, error) {
	const query = `
		SELECT
			hg.id,
			hg.name,
			hg.description,
			COALESCE(hg.filter_rule::text, 'null') AS filter_rule,
			COALESCE(hg.color, '') AS color,
			COUNT(hgm.host_id) AS member_count,
			hg.created_at,
			hg.updated_at
		FROM host_groups hg
		LEFT JOIN host_group_members hgm ON hg.id = hgm.group_id
		WHERE hg.id = $1
		GROUP BY hg.id, hg.name, hg.description, hg.filter_rule, hg.color, hg.created_at, hg.updated_at
	`
	g := &HostGroup{}
	var filterRuleJSON string
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&g.ID, &g.Name, &g.Description, &filterRuleJSON, &g.Color, &g.MemberCount, &g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, errors.ErrNotFoundWithID("group", id.String())
		}
		return nil, sanitizeDBError("get group", err)
	}
	if filterRuleJSON != "null" && filterRuleJSON != "" {
		g.FilterRule = JSONB(filterRuleJSON)
	}
	return g, nil
}

// ListGroups returns all host groups with member counts.
func (r *GroupRepository) ListGroups(ctx context.Context) ([]*HostGroup, error) {
	const query = `
		SELECT
			hg.id,
			hg.name,
			hg.description,
			COALESCE(hg.filter_rule::text, 'null') AS filter_rule,
			COALESCE(hg.color, '') AS color,
			COUNT(hgm.host_id) AS member_count,
			hg.created_at,
			hg.updated_at
		FROM host_groups hg
		LEFT JOIN host_group_members hgm ON hg.id = hgm.group_id
		GROUP BY hg.id, hg.name, hg.description, hg.filter_rule, hg.color, hg.created_at, hg.updated_at
		ORDER BY hg.name
	`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, sanitizeDBError("list groups", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Warn("failed to close rows", "error", closeErr)
		}
	}()

	var groups []*HostGroup
	for rows.Next() {
		g := &HostGroup{}
		var filterRuleJSON string
		if err := rows.Scan(
			&g.ID, &g.Name, &g.Description, &filterRuleJSON, &g.Color, &g.MemberCount, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan group row: %w", err)
		}
		if filterRuleJSON != "null" && filterRuleJSON != "" {
			g.FilterRule = JSONB(filterRuleJSON)
		}
		groups = append(groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate groups: %w", err)
	}
	return groups, nil
}

// UpdateGroup applies partial updates to an existing host group.
func (r *GroupRepository) UpdateGroup(ctx context.Context, id uuid.UUID, input UpdateGroupInput) (*HostGroup, error) {
	tx, err := r.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(); rbErr != nil {
			slog.Warn("rollback error", "error", rbErr)
		}
	}()

	var exists bool
	const existsQ = "SELECT EXISTS(SELECT 1 FROM host_groups WHERE id = $1)"
	if err := tx.QueryRowContext(ctx, existsQ, id).Scan(&exists); err != nil {
		return nil, sanitizeDBError("check group existence", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("group", id.String())
	}

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
	addStr("name", input.Name)
	addStr("description", input.Description)
	addStr("color", input.Color)

	if input.ClearFilter {
		setParts = append(setParts, "filter_rule = NULL")
	} else if input.FilterRule != nil {
		setParts = append(setParts, fmt.Sprintf("filter_rule = $%d", argIndex))
		args = append(args, *input.FilterRule)
		argIndex++
	}

	if len(setParts) == 0 {
		return nil, fmt.Errorf("no fields to update")
	}

	setParts = append(setParts, "updated_at = NOW()")

	var qb strings.Builder
	qb.WriteString("UPDATE host_groups SET ")
	qb.WriteString(strings.Join(setParts, ", "))
	fmt.Fprintf(&qb, " WHERE id = $%d", argIndex)
	args = append(args, id)

	_, err = tx.ExecContext(ctx, qb.String(), args...)
	if err != nil {
		if pqErr := pq.As(err, pqerror.UniqueViolation); pqErr != nil {
			return nil, errors.ErrConflictWithReason("group", "group name already exists")
		}
		return nil, sanitizeDBError("update group", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return r.GetGroup(ctx, id)
}

// DeleteGroup deletes a host group. Membership rows are removed by CASCADE.
func (r *GroupRepository) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM host_groups WHERE id = $1", id)
	if err != nil {
		return sanitizeDBError("delete group", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return errors.ErrNotFoundWithID("group", id.String())
	}
	return nil
}

// AddHostsToGroup adds hosts to a group. Already-existing memberships are silently ignored.
func (r *GroupRepository) AddHostsToGroup(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error {
	if len(hostIDs) == 0 {
		return nil
	}
	// Build a multi-row INSERT with ON CONFLICT DO NOTHING.
	placeholders := make([]string, len(hostIDs))
	args := make([]interface{}, 0, len(hostIDs)*2)
	for i, hid := range hostIDs {
		placeholders[i] = fmt.Sprintf("($%d, $%d)", i*2+1, i*2+2)
		args = append(args, hid, groupID)
	}
	query := fmt.Sprintf(
		"INSERT INTO host_group_members (host_id, group_id) VALUES %s ON CONFLICT DO NOTHING",
		strings.Join(placeholders, ", "),
	)
	_, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return sanitizeDBError("add hosts to group", err)
	}
	return nil
}

// RemoveHostsFromGroup removes hosts from a group.
func (r *GroupRepository) RemoveHostsFromGroup(ctx context.Context, groupID uuid.UUID, hostIDs []uuid.UUID) error {
	if len(hostIDs) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM host_group_members WHERE group_id = $1 AND host_id = ANY($2::uuid[])`,
		groupID, pq.Array(hostIDs),
	)
	if err != nil {
		return sanitizeDBError("remove hosts from group", err)
	}
	return nil
}

// GetGroupMembers returns a paginated list of hosts in the group.
func (r *GroupRepository) GetGroupMembers(
	ctx context.Context, groupID uuid.UUID, offset, limit int,
) ([]*Host, int64, error) {
	// Get total count.
	var total int64
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM host_group_members WHERE group_id = $1`,
		groupID,
	).Scan(&total)
	if err != nil {
		return nil, 0, sanitizeDBError("count group members", err)
	}

	const query = `
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
			h.os_detected_at,
			h.os_method,
			h.os_details,
			h.discovery_method,
			h.response_time_ms,
			h.response_time_min_ms,
			h.response_time_max_ms,
			h.response_time_avg_ms,
			h.ignore_scanning,
			h.first_seen,
			h.last_seen,
			h.status,
			h.status_changed_at,
			h.previous_status,
			h.timeout_count,
			h.tags
		FROM hosts h
		JOIN host_group_members hgm ON h.id = hgm.host_id
		WHERE hgm.group_id = $1
		ORDER BY h.ip_address
		LIMIT $2 OFFSET $3
	`
	rows, err := r.db.QueryContext(ctx, query, groupID, limit, offset)
	if err != nil {
		return nil, 0, sanitizeDBError("list group members", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			slog.Warn("failed to close rows", "error", closeErr)
		}
	}()

	var hosts []*Host
	for rows.Next() {
		host, err := scanGroupMemberRow(rows)
		if err != nil {
			return nil, 0, err
		}
		hosts = append(hosts, host)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate group members: %w", err)
	}
	return hosts, total, nil
}

// scanGroupMemberRow scans a single row from the GetGroupMembers SELECT into a
// *Host. It mirrors the nullable-variable pattern used by scanHostRows in
// repository_host.go but without the aggregate port-count columns.
func scanGroupMemberRow(rows *sql.Rows) (*Host, error) {
	host := &Host{}
	var vars hostScanVars

	err := rows.Scan(
		&host.ID,
		&vars.ipAddress,
		&vars.hostname,
		&vars.macAddressStr,
		&vars.vendor,
		&vars.osFamily,
		&vars.osName,
		&vars.osVersion,
		&vars.osConfidence,
		&vars.osDetectedAt,
		&vars.osMethod,
		&vars.osDetails,
		&vars.discoveryMethod,
		&vars.responseTimeMS,
		&vars.responseTimeMinMS,
		&vars.responseTimeMaxMS,
		&vars.responseTimeAvgMS,
		&vars.ignoreScanning,
		&host.FirstSeen,
		&host.LastSeen,
		&host.Status,
		&vars.statusChangedAt,
		&vars.previousStatus,
		&vars.timeoutCount,
		&host.Tags,
	)
	if err != nil {
		return nil, fmt.Errorf("scan group member row: %w", err)
	}

	host.IPAddress = IPAddr{IP: net.ParseIP(vars.ipAddress)}
	applyHostScanVars(host, vars)

	return host, nil
}
