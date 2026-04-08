// Package db provides typed repository for discovery job database operations.
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

// DiscoveryRepository implements DiscoveryStore against a *DB connection.
type DiscoveryRepository struct {
	db *DB
}

// NewDiscoveryRepository creates a new DiscoveryRepository.
func NewDiscoveryRepository(db *DB) *DiscoveryRepository {
	return &DiscoveryRepository{db: db}
}

// ListDiscoveryJobs retrieves discovery jobs with filtering and pagination.
func (r *DiscoveryRepository) ListDiscoveryJobs(
	ctx context.Context,
	filters DiscoveryFilters,
	offset, limit int,
) ([]*DiscoveryJob, int64, error) {
	countQuery := `
		SELECT COUNT(*)
		FROM discovery_jobs
		WHERE ($1 = '' OR status = $1)
		  AND ($2 = '' OR method = $2)`

	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, filters.Status, filters.Method).Scan(&total); err != nil {
		return nil, 0, sanitizeDBError("count discovery jobs", err)
	}

	listQuery := `
		SELECT id, network_id, network, method, started_at, completed_at,
		       hosts_discovered, hosts_responsive, status, created_at
		FROM discovery_jobs
		WHERE ($1 = '' OR status = $1)
		  AND ($2 = '' OR method = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.db.QueryContext(ctx, listQuery, filters.Status, filters.Method, limit, offset)
	if err != nil {
		return nil, 0, sanitizeDBError("list discovery jobs", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "error", err)
		}
	}()

	jobs := []*DiscoveryJob{}
	for rows.Next() {
		job := &DiscoveryJob{}
		if err := rows.Scan(
			&job.ID,
			&job.NetworkID,
			&job.Network,
			&job.Method,
			&job.StartedAt,
			&job.CompletedAt,
			&job.HostsDiscovered,
			&job.HostsResponsive,
			&job.Status,
			&job.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan discovery job row: %w", err)
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate discovery job rows: %w", err)
	}

	return jobs, total, nil
}

// CreateDiscoveryJob creates a new discovery job.
func (r *DiscoveryRepository) CreateDiscoveryJob(
	ctx context.Context, input CreateDiscoveryJobInput,
) (*DiscoveryJob, error) {
	if len(input.Networks) == 0 {
		return nil, fmt.Errorf("networks are required and must be a string array")
	}

	method := input.Method
	if method == "" {
		method = DiscoveryMethodTCP
	}

	network := input.Networks[0]

	jobID := uuid.New()
	now := time.Now().UTC()

	query := `
		INSERT INTO discovery_jobs
		    (id, network_id, network, method, status, created_at, hosts_discovered, hosts_responsive)
		VALUES ($1, $2, $3, $4, 'pending', $5, 0, 0)
		RETURNING id, network_id, network, method, started_at, completed_at,
		          hosts_discovered, hosts_responsive, status, created_at
	`

	job := &DiscoveryJob{}
	err := r.db.QueryRowContext(ctx, query, jobID, input.NetworkID, network, method, now).Scan(
		&job.ID,
		&job.NetworkID,
		&job.Network,
		&job.Method,
		&job.StartedAt,
		&job.CompletedAt,
		&job.HostsDiscovered,
		&job.HostsResponsive,
		&job.Status,
		&job.CreatedAt,
	)
	if err != nil {
		return nil, sanitizeDBError("create discovery job", err)
	}

	return job, nil
}

// GetDiscoveryJob retrieves a discovery job by ID.
func (r *DiscoveryRepository) GetDiscoveryJob(ctx context.Context, id uuid.UUID) (*DiscoveryJob, error) {
	query := `
		SELECT id, network_id, network, method, started_at, completed_at,
		       hosts_discovered, hosts_responsive, status, created_at
		FROM discovery_jobs
		WHERE id = $1`

	job := &DiscoveryJob{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&job.ID,
		&job.NetworkID,
		&job.Network,
		&job.Method,
		&job.StartedAt,
		&job.CompletedAt,
		&job.HostsDiscovered,
		&job.HostsResponsive,
		&job.Status,
		&job.CreatedAt,
	)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, errors.ErrNotFoundWithID("discovery job", id.String())
		}
		return nil, sanitizeDBError("get discovery job", err)
	}

	return job, nil
}

// UpdateDiscoveryJob updates an existing discovery job.
func (r *DiscoveryRepository) UpdateDiscoveryJob(
	ctx context.Context, id uuid.UUID, input UpdateDiscoveryJobInput,
) (*DiscoveryJob, error) {
	// Check existence first.
	var exists bool
	if err := r.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM discovery_jobs WHERE id = $1)", id,
	).Scan(&exists); err != nil {
		return nil, sanitizeDBError("check discovery job existence", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("discovery job", id.String())
	}

	// Build dynamic SET clause from non-nil pointer fields.
	var setParts []string
	var args []interface{}
	argIndex := 1

	if input.Method != nil {
		setParts = append(setParts, fmt.Sprintf("method = $%d", argIndex))
		args = append(args, *input.Method)
		argIndex++
	}
	if input.Status != nil {
		setParts = append(setParts, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, *input.Status)
		argIndex++
	}
	if input.HostsDiscovered != nil {
		setParts = append(setParts, fmt.Sprintf("hosts_discovered = $%d", argIndex))
		args = append(args, *input.HostsDiscovered)
		argIndex++
	}
	if input.HostsResponsive != nil {
		setParts = append(setParts, fmt.Sprintf("hosts_responsive = $%d", argIndex))
		args = append(args, *input.HostsResponsive)
		argIndex++
	}
	if input.StartedAt != nil {
		setParts = append(setParts, fmt.Sprintf("started_at = $%d", argIndex))
		args = append(args, *input.StartedAt)
		argIndex++
	}
	if input.CompletedAt != nil {
		setParts = append(setParts, fmt.Sprintf("completed_at = $%d", argIndex))
		args = append(args, *input.CompletedAt)
		argIndex++
	}

	if len(setParts) == 0 {
		return nil, fmt.Errorf("no valid fields to update")
	}

	args = append(args, id)

	var queryBuilder strings.Builder
	queryBuilder.WriteString("UPDATE discovery_jobs SET ")
	queryBuilder.WriteString(strings.Join(setParts, ", "))
	queryBuilder.WriteString(" WHERE id = $")
	queryBuilder.WriteString(strconv.Itoa(argIndex))
	queryBuilder.WriteString(` RETURNING id, network_id, network, method, started_at, completed_at,
		hosts_discovered, hosts_responsive, status, created_at`)

	job := &DiscoveryJob{}
	err := r.db.QueryRowContext(ctx, queryBuilder.String(), args...).Scan(
		&job.ID,
		&job.NetworkID,
		&job.Network,
		&job.Method,
		&job.StartedAt,
		&job.CompletedAt,
		&job.HostsDiscovered,
		&job.HostsResponsive,
		&job.Status,
		&job.CreatedAt,
	)
	if err != nil {
		return nil, sanitizeDBError("update discovery job", err)
	}

	return job, nil
}

// DeleteDiscoveryJob deletes a discovery job by ID.
func (r *DiscoveryRepository) DeleteDiscoveryJob(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM discovery_jobs WHERE id = $1", id)
	if err != nil {
		return sanitizeDBError("delete discovery job", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.ErrNotFoundWithID("discovery job", id.String())
	}

	return nil
}

// StartDiscoveryJob starts discovery job execution.
func (r *DiscoveryRepository) StartDiscoveryJob(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE discovery_jobs
		SET status = 'running', started_at = NOW()
		WHERE id = $1 AND status = 'pending'`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return sanitizeDBError("start discovery job", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.ErrNotFoundWithID("discovery job", id.String())
	}

	return nil
}

// ListDiscoveryJobsByNetwork retrieves discovery jobs linked to a specific network,
// ordered most-recent first, with pagination.
func (r *DiscoveryRepository) ListDiscoveryJobsByNetwork(
	ctx context.Context, networkID uuid.UUID, offset, limit int,
) ([]*DiscoveryJob, int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM discovery_jobs WHERE network_id = $1`, networkID,
	).Scan(&total); err != nil {
		return nil, 0, sanitizeDBError("count discovery jobs by network", err)
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, network_id, network, method, started_at, completed_at,
		       hosts_discovered, hosts_responsive, status, created_at
		FROM discovery_jobs
		WHERE network_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`,
		networkID, limit, offset,
	)
	if err != nil {
		return nil, 0, sanitizeDBError("list discovery jobs by network", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "error", err)
		}
	}()

	jobs := []*DiscoveryJob{}
	for rows.Next() {
		job := &DiscoveryJob{}
		if err := rows.Scan(
			&job.ID,
			&job.NetworkID,
			&job.Network,
			&job.Method,
			&job.StartedAt,
			&job.CompletedAt,
			&job.HostsDiscovered,
			&job.HostsResponsive,
			&job.Status,
			&job.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan discovery job row: %w", err)
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate discovery job rows: %w", err)
	}
	return jobs, total, nil
}

// StopDiscoveryJob stops discovery job execution.
func (r *DiscoveryRepository) StopDiscoveryJob(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE discovery_jobs
		SET status = 'failed', completed_at = NOW()
		WHERE id = $1 AND status = 'running'`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return sanitizeDBError("stop discovery job", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.ErrNotFoundWithID("discovery job", id.String())
	}

	return nil
}

// GetDiscoveryDiff computes a diff for a single discovery run, returning the
// sets of new, gone, and changed hosts along with an unchanged count.
// Returns a not-found error (convertible to HTTP 404) when the job does not exist.
func (r *DiscoveryRepository) GetDiscoveryDiff(ctx context.Context, jobID uuid.UUID) (*DiscoveryDiff, error) {
	// ── existence check ────────────────────────────────────────────────────
	var exists bool
	if err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM discovery_jobs WHERE id = $1)`, jobID,
	).Scan(&exists); err != nil {
		return nil, sanitizeDBError("check discovery job for diff", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("discovery job", jobID.String())
	}

	// ── new hosts ──────────────────────────────────────────────────────────
	newHosts, err := r.queryDiffHosts(ctx, `
		SELECT h.id, host(h.ip_address) AS ip_address, h.hostname,
		       h.status, h.previous_status, h.vendor, h.mac_address::text, h.last_seen, h.first_seen
		FROM hosts h
		JOIN discovery_jobs dj ON dj.id = $1
		WHERE h.ip_address <<= dj.network::cidr
		  AND h.first_seen >= dj.created_at
		  AND (dj.completed_at IS NULL OR h.first_seen <= dj.completed_at)
		ORDER BY h.first_seen ASC`, jobID, "query new hosts for diff")
	if err != nil {
		return nil, err
	}

	// ── gone hosts ─────────────────────────────────────────────────────────
	goneHosts, err := r.queryDiffHosts(ctx, `
		SELECT h.id, host(h.ip_address) AS ip_address, h.hostname,
		       h.status, h.previous_status, h.vendor, h.mac_address::text, h.last_seen, h.first_seen
		FROM hosts h
		JOIN host_timeout_events te ON te.host_id = h.id
		WHERE te.discovery_run_id = $1
		ORDER BY h.last_seen DESC`, jobID, "query gone hosts for diff")
	if err != nil {
		return nil, err
	}

	// ── changed hosts ──────────────────────────────────────────────────────
	changedHosts, err := r.queryDiffHosts(ctx, `
		SELECT DISTINCT ON (h.id)
		       h.id, host(h.ip_address) AS ip_address, h.hostname,
		       h.status, h.previous_status, h.vendor, h.mac_address::text, h.last_seen, h.first_seen
		FROM hosts h
		JOIN host_status_events se ON se.host_id = h.id
		JOIN discovery_jobs dj ON dj.id = $1
		WHERE h.ip_address <<= dj.network::cidr
		  AND se.changed_at >= dj.created_at
		  AND (dj.completed_at IS NULL OR se.changed_at <= dj.completed_at)
		  AND se.to_status != 'gone'
		  AND h.first_seen < dj.created_at
		ORDER BY h.id, se.changed_at DESC`, jobID, "query changed hosts for diff")
	if err != nil {
		return nil, err
	}

	// ── total hosts in network ─────────────────────────────────────────────
	var total int
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM hosts h
		JOIN discovery_jobs dj ON dj.id = $1
		WHERE h.ip_address <<= dj.network::cidr`, jobID,
	).Scan(&total); err != nil {
		return nil, sanitizeDBError("count total hosts for diff", err)
	}

	unchanged := total - len(newHosts) - len(goneHosts) - len(changedHosts)
	if unchanged < 0 {
		unchanged = 0
	}

	return &DiscoveryDiff{
		JobID:          jobID,
		NewHosts:       newHosts,
		GoneHosts:      goneHosts,
		ChangedHosts:   changedHosts,
		UnchangedCount: unchanged,
	}, nil
}

// queryDiffHosts runs a single diff-host query and scans the results into a
// []DiffHost slice. It is a shared helper used by GetDiscoveryDiff.
func (r *DiscoveryRepository) queryDiffHosts(
	ctx context.Context, query string, jobID uuid.UUID, operation string,
) ([]DiffHost, error) {
	rows, err := r.db.QueryContext(ctx, query, jobID)
	if err != nil {
		return nil, sanitizeDBError(operation, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close diff-host rows", "error", err)
		}
	}()

	hosts := []DiffHost{}
	for rows.Next() {
		var h DiffHost
		if err := rows.Scan(
			&h.ID,
			&h.IPAddress,
			&h.Hostname,
			&h.Status,
			&h.PreviousStatus,
			&h.Vendor,
			&h.MACAddress,
			&h.LastSeen,
			&h.FirstSeen,
		); err != nil {
			return nil, fmt.Errorf("failed to scan diff host row: %w", err)
		}
		hosts = append(hosts, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate diff host rows: %w", err)
	}

	return hosts, nil
}

// queryDiffHostsTwo runs a diff-host query parameterised with two UUIDs
// ($1 = jobA, $2 = jobB) and scans the results into a []DiffHost slice.
// It is used by CompareDiscoveryRuns for queries that reference both runs.
func (r *DiscoveryRepository) queryDiffHostsTwo(
	ctx context.Context, query string, jobA, jobB uuid.UUID, operation string,
) ([]DiffHost, error) {
	rows, err := r.db.QueryContext(ctx, query, jobA, jobB)
	if err != nil {
		return nil, sanitizeDBError(operation, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close diff-host rows", "error", err)
		}
	}()

	hosts := []DiffHost{}
	for rows.Next() {
		var h DiffHost
		if err := rows.Scan(
			&h.ID,
			&h.IPAddress,
			&h.Hostname,
			&h.Status,
			&h.PreviousStatus,
			&h.Vendor,
			&h.MACAddress,
			&h.LastSeen,
			&h.FirstSeen,
		); err != nil {
			return nil, fmt.Errorf("failed to scan diff host row: %w", err)
		}
		hosts = append(hosts, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate diff host rows: %w", err)
	}

	return hosts, nil
}

// compareRunInfo holds the fields needed from a discovery_job row for comparison.
type compareRunInfo struct {
	network     string
	completedAt *time.Time
}

// loadCompareRunInfo loads the network and completedAt for a discovery job.
// Returns ErrNotFound if the job does not exist.
func (r *DiscoveryRepository) loadCompareRunInfo(
	ctx context.Context, id uuid.UUID,
) (*compareRunInfo, error) {
	var info compareRunInfo
	err := r.db.QueryRowContext(ctx,
		`SELECT network, completed_at FROM discovery_jobs WHERE id = $1`, id,
	).Scan(&info.network, &info.completedAt)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, errors.ErrNotFoundWithID("discovery job", id.String())
		}
		return nil, sanitizeDBError("get discovery run for compare", err)
	}
	return &info, nil
}

// compareCountHosts returns the number of hosts currently within the network
// of the given discovery job.
func (r *DiscoveryRepository) compareCountHosts(ctx context.Context, jobID uuid.UUID) (int, error) {
	var total int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM hosts h
		WHERE h.ip_address <<= (SELECT network::cidr FROM discovery_jobs WHERE id = $1)`, jobID,
	).Scan(&total)
	if err != nil {
		return 0, sanitizeDBError("count total hosts for compare", err)
	}
	return total, nil
}

// CompareDiscoveryRuns computes the diff between two discovery runs
// (jobA = baseline, jobB = current).
// Returns ErrNotFound if either run doesn't exist, and a descriptive error if
// the networks differ.
// If jobA == jobB the result has all existing hosts as unchanged and empty
// new/gone/changed slices.
func (r *DiscoveryRepository) CompareDiscoveryRuns(
	ctx context.Context, jobA, jobB uuid.UUID,
) (*DiscoveryCompareDiff, error) {
	runA, err := r.loadCompareRunInfo(ctx, jobA)
	if err != nil {
		return nil, err
	}
	runB, err := r.loadCompareRunInfo(ctx, jobB)
	if err != nil {
		return nil, err
	}

	if runA.network != runB.network {
		return nil, fmt.Errorf(
			"cannot compare runs on different networks: %s vs %s",
			runA.network, runB.network,
		)
	}

	// Edge case: same run — all hosts unchanged, nothing new/gone/changed.
	if jobA == jobB {
		total, err := r.compareCountHosts(ctx, jobB)
		if err != nil {
			return nil, err
		}
		return &DiscoveryCompareDiff{
			RunAID: jobA, RunBID: jobB,
			NewHosts: []DiffHost{}, GoneHosts: []DiffHost{}, ChangedHosts: []DiffHost{},
			UnchangedCount: total,
		}, nil
	}

	// New hosts in B: first seen after A completed, on or before B completed.
	newHosts, err := r.queryDiffHostsTwo(ctx, `
		SELECT h.id, host(h.ip_address) AS ip_address, h.hostname,
		       h.status, h.previous_status, h.vendor, h.mac_address::text, h.last_seen, h.first_seen
		FROM hosts h
		WHERE h.ip_address <<= (SELECT network::cidr FROM discovery_jobs WHERE id = $2)
		  AND h.first_seen > COALESCE((SELECT completed_at FROM discovery_jobs WHERE id = $1),
		                              (SELECT created_at  FROM discovery_jobs WHERE id = $1))
		  AND h.first_seen <= COALESCE((SELECT completed_at FROM discovery_jobs WHERE id = $2), NOW())
		ORDER BY h.first_seen ASC`,
		jobA, jobB, "compare new hosts",
	)
	if err != nil {
		return nil, err
	}

	// Gone hosts in B: hosts with timeout events recorded against run B.
	goneHosts, err := r.queryDiffHosts(ctx, `
		SELECT h.id, host(h.ip_address) AS ip_address, h.hostname,
		       h.status, h.previous_status, h.vendor, h.mac_address::text, h.last_seen, h.first_seen
		FROM hosts h
		JOIN host_timeout_events te ON te.host_id = h.id
		WHERE te.discovery_run_id = $1
		ORDER BY h.last_seen DESC`,
		jobB, "compare gone hosts",
	)
	if err != nil {
		return nil, err
	}

	// Changed hosts in B: status changes between A.completed and B.completed,
	// excluding brand-new and gone hosts.
	changedHosts, err := r.queryDiffHostsTwo(ctx, `
		SELECT DISTINCT ON (h.id)
		       h.id, host(h.ip_address) AS ip_address, h.hostname,
		       h.status, h.previous_status, h.vendor, h.mac_address::text, h.last_seen, h.first_seen
		FROM hosts h
		JOIN host_status_events se ON se.host_id = h.id
		WHERE h.ip_address <<= (SELECT network::cidr FROM discovery_jobs WHERE id = $2)
		  AND se.changed_at > COALESCE((SELECT completed_at FROM discovery_jobs WHERE id = $1),
		                               (SELECT created_at  FROM discovery_jobs WHERE id = $1))
		  AND se.changed_at <= COALESCE((SELECT completed_at FROM discovery_jobs WHERE id = $2), NOW())
		  AND se.to_status != 'gone'
		  AND h.first_seen <= COALESCE((SELECT completed_at FROM discovery_jobs WHERE id = $1),
		                               (SELECT created_at  FROM discovery_jobs WHERE id = $1))
		ORDER BY h.id, se.changed_at DESC`,
		jobA, jobB, "compare changed hosts",
	)
	if err != nil {
		return nil, err
	}

	total, err := r.compareCountHosts(ctx, jobB)
	if err != nil {
		return nil, err
	}

	unchanged := total - len(newHosts) - len(goneHosts) - len(changedHosts)
	if unchanged < 0 {
		unchanged = 0
	}

	return &DiscoveryCompareDiff{
		RunAID:         jobA,
		RunBID:         jobB,
		NewHosts:       newHosts,
		GoneHosts:      goneHosts,
		ChangedHosts:   changedHosts,
		UnchangedCount: unchanged,
	}, nil
}
