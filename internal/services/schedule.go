// Package services provides business logic for Scanorama operations.
// This file implements schedule management, including cron-expression
// validation and a NextRun helper.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// -- Repository interface --

// scheduleRepository is the DB-facing interface consumed by ScheduleService.
//
//nolint:dupl // interface mirrored by mockScheduleRepo in schedule_test.go; duplication is unavoidable
type scheduleRepository interface {
	ListSchedules(ctx context.Context, filters db.ScheduleFilters, offset, limit int) ([]*db.Schedule, int64, error)
	CreateSchedule(ctx context.Context, input db.CreateScheduleInput) (*db.Schedule, error)
	GetSchedule(ctx context.Context, id uuid.UUID) (*db.Schedule, error)
	UpdateSchedule(ctx context.Context, id uuid.UUID, input db.UpdateScheduleInput) (*db.Schedule, error)
	DeleteSchedule(ctx context.Context, id uuid.UUID) error
	EnableSchedule(ctx context.Context, id uuid.UUID) error
	DisableSchedule(ctx context.Context, id uuid.UUID) error
}

// -- Service --

// ScheduleService provides business logic for scheduled-job operations.
type ScheduleService struct {
	repo   scheduleRepository
	logger *slog.Logger
}

// NewScheduleService creates a new ScheduleService backed by the given repository.
func NewScheduleService(repo scheduleRepository, logger *slog.Logger) *ScheduleService {
	return &ScheduleService{
		repo:   repo,
		logger: logger,
	}
}

// -- Methods --

// ListSchedules retrieves schedules with optional filtering and pagination.
func (s *ScheduleService) ListSchedules(
	ctx context.Context,
	filters db.ScheduleFilters,
	offset, limit int,
) ([]*db.Schedule, int64, error) {
	return s.repo.ListSchedules(ctx, filters, offset, limit)
}

// CreateSchedule validates the cron expression and delegates creation to the
// repository.
func (s *ScheduleService) CreateSchedule(
	ctx context.Context,
	input db.CreateScheduleInput,
) (*db.Schedule, error) {
	if err := ValidateCronExpression(input.CronExpression); err != nil {
		return nil, err
	}
	return s.repo.CreateSchedule(ctx, input)
}

// GetSchedule retrieves a single schedule by its ID.
func (s *ScheduleService) GetSchedule(ctx context.Context, id uuid.UUID) (*db.Schedule, error) {
	return s.repo.GetSchedule(ctx, id)
}

// UpdateSchedule validates the cron expression if it is being changed, then
// delegates the update to the repository.
func (s *ScheduleService) UpdateSchedule(
	ctx context.Context,
	id uuid.UUID,
	input db.UpdateScheduleInput,
) (*db.Schedule, error) {
	if input.CronExpression != nil {
		if err := ValidateCronExpression(*input.CronExpression); err != nil {
			return nil, err
		}
	}
	return s.repo.UpdateSchedule(ctx, id, input)
}

// DeleteSchedule removes a schedule by ID.
func (s *ScheduleService) DeleteSchedule(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteSchedule(ctx, id)
}

// EnableSchedule enables a schedule so it will be picked up by the scheduler.
func (s *ScheduleService) EnableSchedule(ctx context.Context, id uuid.UUID) error {
	return s.repo.EnableSchedule(ctx, id)
}

// DisableSchedule prevents a schedule from being run by the scheduler.
func (s *ScheduleService) DisableSchedule(ctx context.Context, id uuid.UUID) error {
	return s.repo.DisableSchedule(ctx, id)
}

// NextRun fetches the schedule, parses its cron expression, and returns the
// next time the job would fire after the current UTC instant.
func (s *ScheduleService) NextRun(ctx context.Context, id uuid.UUID) (time.Time, error) {
	schedule, err := s.repo.GetSchedule(ctx, id)
	if err != nil {
		return time.Time{}, err
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(schedule.CronExpression)
	if err != nil {
		return time.Time{}, errors.NewScanError(
			errors.CodeValidation,
			fmt.Sprintf("failed to parse cron expression %q: %s", schedule.CronExpression, err.Error()),
		)
	}

	return sched.Next(time.Now().UTC()), nil
}

// -- Cron-expression validation --

// ValidateCronExpression checks that cronExpr is a non-empty, syntactically
// valid standard 5-field cron expression.
func ValidateCronExpression(cronExpr string) error {
	if cronExpr == "" {
		return errors.NewScanError(errors.CodeValidation, "cron expression is required")
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(cronExpr); err != nil {
		return errors.NewScanError(
			errors.CodeValidation,
			fmt.Sprintf("invalid cron expression %q: %s", cronExpr, err.Error()),
		)
	}
	return nil
}
