// Package db provides typed repository for scheduled job database operations.
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

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/errors"
)

// ScheduleRepository implements ScheduleStore against a *DB connection.
type ScheduleRepository struct {
	db *DB
}

// NewScheduleRepository creates a new ScheduleRepository.
func NewScheduleRepository(db *DB) *ScheduleRepository {
	return &ScheduleRepository{db: db}
}

// scanScheduleRow scans a single row from the scheduled_jobs table into a Schedule.
// The DB columns are: id, name, type, cron_expression, config, enabled, last_run, next_run, created_at
// The Schedule struct keeps JobType / JobConfig field names but maps from the DB columns correctly.
func scanScheduleRow(row interface {
	Scan(dest ...interface{}) error
}) (*Schedule, error) {
	s := &Schedule{}
	var jobType string
	var configRaw []byte
	var description string // throwaway — not in DB

	if err := row.Scan(
		&s.ID,
		&s.Name,
		&jobType,
		&s.CronExpression,
		&configRaw,
		&s.Enabled,
		&s.LastRun,
		&s.NextRun,
		&s.CreatedAt,
	); err != nil {
		return nil, err
	}

	_ = description // never stored in DB
	s.JobType = jobType

	if len(configRaw) > 0 {
		if err := json.Unmarshal(configRaw, &s.JobConfig); err != nil {
			return nil, fmt.Errorf("failed to unmarshal job config: %w", err)
		}
	}

	return s, nil
}

// buildScheduleSetParts builds the SET clause parts and args for an UpdateSchedule query.
// It returns an error only if JSON marshaling of job_config fails.
func buildScheduleSetParts(input UpdateScheduleInput) (setParts []string, args []interface{}, err error) {
	argIndex := 1

	if input.Name != nil {
		setParts = append(setParts, fmt.Sprintf("name = $%d", argIndex))
		args = append(args, *input.Name)
		argIndex++
	}

	if input.JobType != nil {
		setParts = append(setParts, fmt.Sprintf("type = $%d", argIndex))
		args = append(args, *input.JobType)
		argIndex++
	}

	if input.CronExpression != nil {
		setParts = append(setParts, fmt.Sprintf("cron_expression = $%d", argIndex))
		args = append(args, *input.CronExpression)
		argIndex++
	}

	if input.JobConfig != nil {
		configJSON, jsonErr := json.Marshal(input.JobConfig)
		if jsonErr != nil {
			return nil, nil, fmt.Errorf("failed to marshal job config: %w", jsonErr)
		}
		setParts = append(setParts, fmt.Sprintf("config = $%d", argIndex))
		args = append(args, configJSON)
		argIndex++
	}

	if input.Enabled != nil {
		setParts = append(setParts, fmt.Sprintf("enabled = $%d", argIndex))
		args = append(args, *input.Enabled)
		argIndex++
	}

	if input.NextRun != nil {
		setParts = append(setParts, fmt.Sprintf("next_run = $%d", argIndex))
		args = append(args, *input.NextRun)
		_ = argIndex // argIndex intentionally not incremented after last field
	}

	return
}

// validScheduleSortColumns maps API sort keys to safe SQL column expressions.
var validScheduleSortColumns = map[string]string{
	"name":            "name",
	"enabled":         "enabled",
	"cron_expression": "cron_expression",
	"next_run":        "next_run",
	"last_run":        "last_run",
	"created_at":      "created_at",
}

// ListSchedules retrieves schedules with filtering and pagination.
func (r *ScheduleRepository) ListSchedules(
	ctx context.Context,
	filters ScheduleFilters,
	offset, limit int,
) ([]*Schedule, int64, error) {
	// Build WHERE conditions dynamically so a zero-value Enabled bool is not
	// mistakenly treated as "filter for disabled rows".  The handler only sets
	// Enabled when the query-param is explicitly provided, so we use a
	// separate EnabledSet sentinel via the JobType / Enabled zero-value logic:
	// we always include the enabled filter in the query but guard it with an
	// extra boolean arg ($3 for count, $3 for list) that says whether to apply it.
	//
	// Simpler approach: build the WHERE clause as a plain string conditionally.
	conditions := []string{}
	args := []interface{}{}
	argIdx := 1

	if filters.JobType != "" {
		conditions = append(conditions, fmt.Sprintf("type = $%d", argIdx))
		args = append(args, filters.JobType)
		argIdx++
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM scheduled_jobs %s", whereClause)

	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, sanitizeDBError("count schedules", err)
	}

	orderByClause := " ORDER BY name ASC NULLS LAST"
	if filters.SortBy != "" {
		if col, ok := validScheduleSortColumns[filters.SortBy]; ok {
			dir := sortOrderASC
			if strings.EqualFold(filters.SortOrder, sortOrderDESC) {
				dir = sortOrderDESC
			}
			orderByClause = fmt.Sprintf(" ORDER BY %s %s NULLS LAST", col, dir)
		}
	}
	listQuery := fmt.Sprintf(`
		SELECT id, name, type, cron_expression, config, enabled, last_run, next_run, created_at
		FROM scheduled_jobs %s%s
		LIMIT $%d OFFSET $%d`, whereClause, orderByClause, argIdx, argIdx+1)

	args = append(args, limit, offset)
	listArgs := args
	rows, err := r.db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, sanitizeDBError("list schedules", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Warn("failed to close rows", "error", err)
		}
	}()

	schedules := []*Schedule{}
	for rows.Next() {
		s, err := scanScheduleRow(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan schedule row: %w", err)
		}
		schedules = append(schedules, s)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("failed to iterate schedule rows: %w", err)
	}

	return schedules, total, nil
}

// CreateSchedule creates a new schedule.
func (r *ScheduleRepository) CreateSchedule(ctx context.Context, input CreateScheduleInput) (*Schedule, error) {
	// Marshal job_config map to JSON for the JSONB column.
	var configJSON []byte
	var err error
	if len(input.JobConfig) > 0 {
		configJSON, err = json.Marshal(input.JobConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal job config: %w", err)
		}
	} else {
		configJSON = []byte("{}")
	}

	id := uuid.New()

	query := `
		INSERT INTO scheduled_jobs (id, name, type, cron_expression, config, enabled, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		RETURNING id, name, type, cron_expression, config, enabled, last_run, next_run, created_at`

	row := r.db.QueryRowContext(ctx, query,
		id, input.Name, input.JobType, input.CronExpression, configJSON, input.Enabled)
	s, err := scanScheduleRow(row)
	if err != nil {
		return nil, sanitizeDBError("create schedule", err)
	}

	return s, nil
}

// GetSchedule retrieves a schedule by ID.
func (r *ScheduleRepository) GetSchedule(ctx context.Context, id uuid.UUID) (*Schedule, error) {
	query := `
		SELECT id, name, type, cron_expression, config, enabled, last_run, next_run, created_at
		FROM scheduled_jobs
		WHERE id = $1`

	row := r.db.QueryRowContext(ctx, query, id)
	s, err := scanScheduleRow(row)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, errors.ErrNotFoundWithID("schedule", id.String())
		}
		return nil, sanitizeDBError("get schedule", err)
	}

	return s, nil
}

// UpdateSchedule updates an existing schedule.
func (r *ScheduleRepository) UpdateSchedule(
	ctx context.Context, id uuid.UUID, input UpdateScheduleInput,
) (*Schedule, error) {
	// Check existence first.
	var exists bool
	if err := r.db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM scheduled_jobs WHERE id = $1)", id,
	).Scan(&exists); err != nil {
		return nil, sanitizeDBError("check schedule existence", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("schedule", id.String())
	}

	setParts, args, err := buildScheduleSetParts(input)
	if err != nil {
		return nil, err
	}

	if len(setParts) == 0 {
		return nil, fmt.Errorf("no valid fields to update")
	}

	args = append(args, id)
	argIndex := len(args)

	var queryBuilder strings.Builder
	queryBuilder.WriteString("UPDATE scheduled_jobs SET ")
	queryBuilder.WriteString(strings.Join(setParts, ", "))
	queryBuilder.WriteString(" WHERE id = $")
	queryBuilder.WriteString(strconv.Itoa(argIndex))
	queryBuilder.WriteString(`
		RETURNING id, name, type, cron_expression, config, enabled, last_run, next_run, created_at`)

	row := r.db.QueryRowContext(ctx, queryBuilder.String(), args...)
	s, err := scanScheduleRow(row)
	if err != nil {
		return nil, sanitizeDBError("update schedule", err)
	}

	return s, nil
}

// DeleteSchedule deletes a schedule by ID.
func (r *ScheduleRepository) DeleteSchedule(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM scheduled_jobs WHERE id = $1", id)
	if err != nil {
		return sanitizeDBError("delete schedule", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.ErrNotFoundWithID("schedule", id.String())
	}

	return nil
}

// EnableSchedule enables a schedule.
func (r *ScheduleRepository) EnableSchedule(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE scheduled_jobs SET enabled = true WHERE id = $1", id)
	if err != nil {
		return sanitizeDBError("enable schedule", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.ErrNotFoundWithID("schedule", id.String())
	}

	return nil
}

// DisableSchedule disables a schedule.
func (r *ScheduleRepository) DisableSchedule(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		"UPDATE scheduled_jobs SET enabled = false WHERE id = $1", id)
	if err != nil {
		return sanitizeDBError("disable schedule", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.ErrNotFoundWithID("schedule", id.String())
	}

	return nil
}
