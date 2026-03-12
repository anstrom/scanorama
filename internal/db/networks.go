// Package db provides network and discovery-related database operations for scanorama.
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

// NetworkSummaryRepository handles network summary operations.
type NetworkSummaryRepository struct {
	db *DB
}

// NewNetworkSummaryRepository creates a new network summary repository.
func NewNetworkSummaryRepository(db *DB) *NetworkSummaryRepository {
	return &NetworkSummaryRepository{db: db}
}

// GetAll retrieves all network summaries.
func (r *NetworkSummaryRepository) GetAll(ctx context.Context) ([]*NetworkSummary, error) {
	var summaries []*NetworkSummary
	query := `
		SELECT *
		FROM network_summary
		ORDER BY target_name`

	if err := r.db.SelectContext(ctx, &summaries, query); err != nil {
		return nil, sanitizeDBError("get network summaries", err)
	}

	return summaries, nil
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
	countQuery := `
		SELECT COUNT(*)
		FROM discovery_jobs
		WHERE ($1 = '' OR status = $1)
		  AND ($2 = '' OR method = $2)`

	var total int64
	if err := db.QueryRowContext(ctx, countQuery, filters.Status, filters.Method).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count discovery jobs: %w", err)
	}

	listQuery := `
		SELECT id, network, method, started_at, completed_at,
		       hosts_discovered, hosts_responsive, status, created_at
		FROM discovery_jobs
		WHERE ($1 = '' OR status = $1)
		  AND ($2 = '' OR method = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`

	rows, err := db.QueryContext(ctx, listQuery, filters.Status, filters.Method, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list discovery jobs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("failed to close discovery jobs rows: %v", err)
		}
	}()

	jobs := []*DiscoveryJob{}
	for rows.Next() {
		job := &DiscoveryJob{}
		if err := rows.Scan(
			&job.ID,
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
func (db *DB) CreateDiscoveryJob(ctx context.Context, jobData interface{}) (*DiscoveryJob, error) {
	data, ok := jobData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid discovery job data format")
	}

	// Extract data from request.
	networks, ok := data["networks"].([]string)
	if !ok || len(networks) == 0 {
		return nil, fmt.Errorf("networks are required and must be a string array")
	}

	method := data["method"].(string)
	if method == "" {
		method = DiscoveryMethodTCP
	}

	// For simplicity, create one discovery job for the first network.
	// In a production system, you might create multiple jobs or handle multiple networks differently.
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
		Network:         NetworkAddr{}, // Would need proper parsing.
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
	query := `
		SELECT id, network, method, started_at, completed_at,
		       hosts_discovered, hosts_responsive, status, created_at
		FROM discovery_jobs
		WHERE id = $1`

	job := &DiscoveryJob{}
	err := db.QueryRowContext(ctx, query, id).Scan(
		&job.ID,
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
		if err == sql.ErrNoRows {
			return nil, errors.ErrNotFoundWithID("discovery job", id.String())
		}
		return nil, fmt.Errorf("failed to get discovery job: %w", err)
	}

	return job, nil
}

// UpdateDiscoveryJob updates an existing discovery job.
func (db *DB) UpdateDiscoveryJob(ctx context.Context, id uuid.UUID, jobData interface{}) (*DiscoveryJob, error) {
	data, ok := jobData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid discovery job data format")
	}

	// Check existence first.
	var exists bool
	if err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM discovery_jobs WHERE id = $1)", id,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("failed to check discovery job existence: %w", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("discovery job", id.String())
	}

	// Build dynamic SET clause.
	setParts := []string{}
	args := []interface{}{}
	argIndex := 1

	stringFields := map[string]string{
		"method": "method",
		"status": "status",
	}
	for dataKey, col := range stringFields {
		if val, ok := data[dataKey]; ok && val != nil {
			setParts = append(setParts, fmt.Sprintf("%s = $%d", col, argIndex))
			args = append(args, val)
			argIndex++
		}
	}

	intFields := map[string]string{
		"hosts_discovered": "hosts_discovered",
		"hosts_responsive": "hosts_responsive",
	}
	for dataKey, col := range intFields {
		if val, ok := data[dataKey]; ok && val != nil {
			setParts = append(setParts, fmt.Sprintf("%s = $%d", col, argIndex))
			args = append(args, val)
			argIndex++
		}
	}

	timeFields := map[string]string{
		"started_at":   "started_at",
		"completed_at": "completed_at",
	}
	for dataKey, col := range timeFields {
		if val, ok := data[dataKey]; ok && val != nil {
			setParts = append(setParts, fmt.Sprintf("%s = $%d", col, argIndex))
			args = append(args, val)
			argIndex++
		}
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
	queryBuilder.WriteString(` RETURNING id, network, method, started_at, completed_at,
		hosts_discovered, hosts_responsive, status, created_at`)

	job := &DiscoveryJob{}
	err := db.QueryRowContext(ctx, queryBuilder.String(), args...).Scan(
		&job.ID,
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
		return nil, fmt.Errorf("failed to update discovery job: %w", err)
	}

	return job, nil
}

// DeleteDiscoveryJob deletes a discovery job by ID.
func (db *DB) DeleteDiscoveryJob(ctx context.Context, id uuid.UUID) error {
	result, err := db.ExecContext(ctx, "DELETE FROM discovery_jobs WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete discovery job: %w", err)
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
func (db *DB) StartDiscoveryJob(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE discovery_jobs
		SET status = 'running', started_at = NOW()
		WHERE id = $1 AND status = 'pending'`

	result, err := db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to start discovery job: %w", err)
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

// StopDiscoveryJob stops discovery job execution.
func (db *DB) StopDiscoveryJob(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE discovery_jobs
		SET status = 'failed', completed_at = NOW()
		WHERE id = $1 AND status = 'running'`

	result, err := db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to stop discovery job: %w", err)
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
