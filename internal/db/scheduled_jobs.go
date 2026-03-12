// Package db provides scheduled job database operations for scanorama.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/errors"
)

// Schedule represents a scheduled task.
type Schedule struct {
	ID             uuid.UUID              `json:"id" db:"id"`
	Name           string                 `json:"name" db:"name"`
	Description    string                 `json:"description" db:"description"`
	CronExpression string                 `json:"cron_expression" db:"cron_expression"`
	JobType        string                 `json:"job_type" db:"job_type"`
	JobConfig      map[string]interface{} `json:"job_config" db:"job_config"`
	Enabled        bool                   `json:"enabled" db:"enabled"`
	CreatedAt      time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at" db:"updated_at"`
	LastRun        *time.Time             `json:"last_run" db:"last_run"`
	NextRun        *time.Time             `json:"next_run" db:"next_run"`
}

// ScheduleFilters represents filters for listing schedules.
type ScheduleFilters struct {
	Enabled bool
	JobType string
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

// ListSchedules retrieves schedules with filtering and pagination.
func (db *DB) ListSchedules(
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
	if err := db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count schedules: %w", err)
	}

	listQuery := fmt.Sprintf(`
		SELECT id, name, type, cron_expression, config, enabled, last_run, next_run, created_at
		FROM scheduled_jobs %s
		ORDER BY name ASC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)

	args = append(args, limit, offset)
	listArgs := args
	rows, err := db.QueryContext(ctx, listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list schedules: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("failed to close schedule rows: %v", err)
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
func (db *DB) CreateSchedule(ctx context.Context, scheduleData interface{}) (*Schedule, error) {
	data, ok := scheduleData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid schedule data format")
	}

	name, _ := data["name"].(string)
	jobType, _ := data["job_type"].(string)
	cronExpression, _ := data["cron_expression"].(string)

	enabled := true
	if v, ok := data["enabled"].(bool); ok {
		enabled = v
	}

	// Marshal job_config map to JSON for the JSONB column.
	var configJSON []byte
	var err error
	if jobConfig, ok := data["job_config"]; ok && jobConfig != nil {
		configJSON, err = json.Marshal(jobConfig)
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

	row := db.QueryRowContext(ctx, query, id, name, jobType, cronExpression, configJSON, enabled)
	s, err := scanScheduleRow(row)
	if err != nil {
		return nil, sanitizeDBError("create schedule", err)
	}

	return s, nil
}

// GetSchedule retrieves a schedule by ID.
func (db *DB) GetSchedule(ctx context.Context, id uuid.UUID) (*Schedule, error) {
	query := `
		SELECT id, name, type, cron_expression, config, enabled, last_run, next_run, created_at
		FROM scheduled_jobs
		WHERE id = $1`

	row := db.QueryRowContext(ctx, query, id)
	s, err := scanScheduleRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.ErrNotFoundWithID("schedule", id.String())
		}
		return nil, fmt.Errorf("failed to get schedule: %w", err)
	}

	return s, nil
}

// buildScheduleSetParts builds the SET clause parts and args for an UpdateSchedule query.
// It returns an error only if JSON marshaling of job_config fails.
func buildScheduleSetParts(data map[string]interface{}) (setParts []string, args []interface{}, err error) {
	argIndex := 1

	if v, ok := data["name"]; ok && v != nil {
		setParts = append(setParts, fmt.Sprintf("name = $%d", argIndex))
		args = append(args, v)
		argIndex++
	}

	if v, ok := data["job_type"]; ok && v != nil {
		setParts = append(setParts, fmt.Sprintf("type = $%d", argIndex))
		args = append(args, v)
		argIndex++
	}

	if v, ok := data["cron_expression"]; ok && v != nil {
		setParts = append(setParts, fmt.Sprintf("cron_expression = $%d", argIndex))
		args = append(args, v)
		argIndex++
	}

	if v, ok := data["job_config"]; ok && v != nil {
		configJSON, jsonErr := json.Marshal(v)
		if jsonErr != nil {
			return nil, nil, fmt.Errorf("failed to marshal job config: %w", jsonErr)
		}
		setParts = append(setParts, fmt.Sprintf("config = $%d", argIndex))
		args = append(args, configJSON)
		argIndex++
	}

	if v, ok := data["enabled"]; ok && v != nil {
		setParts = append(setParts, fmt.Sprintf("enabled = $%d", argIndex))
		args = append(args, v)
		argIndex++
	}

	if v, ok := data["next_run"]; ok && v != nil {
		setParts = append(setParts, fmt.Sprintf("next_run = $%d", argIndex))
		args = append(args, v)
		_ = argIndex // argIndex intentionally not incremented after last field
	}

	return
}

// UpdateSchedule updates an existing schedule.
func (db *DB) UpdateSchedule(ctx context.Context, id uuid.UUID, scheduleData interface{}) (*Schedule, error) {
	data, ok := scheduleData.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid schedule data format")
	}

	// Check existence first.
	var exists bool
	if err := db.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM scheduled_jobs WHERE id = $1)", id,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("failed to check schedule existence: %w", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("schedule", id.String())
	}

	setParts, args, err := buildScheduleSetParts(data)
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

	row := db.QueryRowContext(ctx, queryBuilder.String(), args...)
	s, err := scanScheduleRow(row)
	if err != nil {
		return nil, fmt.Errorf("failed to update schedule: %w", err)
	}

	return s, nil
}

// DeleteSchedule deletes a schedule by ID.
func (db *DB) DeleteSchedule(ctx context.Context, id uuid.UUID) error {
	result, err := db.ExecContext(ctx, "DELETE FROM scheduled_jobs WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to delete schedule: %w", err)
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
func (db *DB) EnableSchedule(ctx context.Context, id uuid.UUID) error {
	result, err := db.ExecContext(ctx,
		"UPDATE scheduled_jobs SET enabled = true WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to enable schedule: %w", err)
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
func (db *DB) DisableSchedule(ctx context.Context, id uuid.UUID) error {
	result, err := db.ExecContext(ctx,
		"UPDATE scheduled_jobs SET enabled = false WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("failed to disable schedule: %w", err)
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
