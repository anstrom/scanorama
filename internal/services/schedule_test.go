// Package services contains same-package unit tests for ScheduleService and
// ValidateCronExpression.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// ---------------------------------------------------------------------------
// Hand-rolled mock for scheduleRepository
// ---------------------------------------------------------------------------

type mockScheduleRepo struct {
	listSchedulesFn func(
		ctx context.Context, filters db.ScheduleFilters, offset, limit int,
	) ([]*db.Schedule, int64, error)
	createScheduleFn  func(ctx context.Context, input db.CreateScheduleInput) (*db.Schedule, error)
	getScheduleFn     func(ctx context.Context, id uuid.UUID) (*db.Schedule, error)
	updateScheduleFn  func(ctx context.Context, id uuid.UUID, input db.UpdateScheduleInput) (*db.Schedule, error)
	deleteScheduleFn  func(ctx context.Context, id uuid.UUID) error
	enableScheduleFn  func(ctx context.Context, id uuid.UUID) error
	disableScheduleFn func(ctx context.Context, id uuid.UUID) error
}

func (m *mockScheduleRepo) ListSchedules(
	ctx context.Context, filters db.ScheduleFilters, offset, limit int,
) ([]*db.Schedule, int64, error) {
	return m.listSchedulesFn(ctx, filters, offset, limit)
}

func (m *mockScheduleRepo) CreateSchedule(ctx context.Context, input db.CreateScheduleInput) (*db.Schedule, error) {
	return m.createScheduleFn(ctx, input)
}

func (m *mockScheduleRepo) GetSchedule(ctx context.Context, id uuid.UUID) (*db.Schedule, error) {
	return m.getScheduleFn(ctx, id)
}

func (m *mockScheduleRepo) UpdateSchedule(
	ctx context.Context, id uuid.UUID, input db.UpdateScheduleInput,
) (*db.Schedule, error) {
	return m.updateScheduleFn(ctx, id, input)
}

func (m *mockScheduleRepo) DeleteSchedule(ctx context.Context, id uuid.UUID) error {
	return m.deleteScheduleFn(ctx, id)
}

func (m *mockScheduleRepo) EnableSchedule(ctx context.Context, id uuid.UUID) error {
	return m.enableScheduleFn(ctx, id)
}

func (m *mockScheduleRepo) DisableSchedule(ctx context.Context, id uuid.UUID) error {
	return m.disableScheduleFn(ctx, id)
}

// ---------------------------------------------------------------------------
// ValidateCronExpression
// ---------------------------------------------------------------------------

func TestValidateCronExpression_Empty_Error(t *testing.T) {
	err := ValidateCronExpression("")

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeValidation), "expected CodeValidation, got %v", err)
}

func TestValidateCronExpression_Invalid_Error(t *testing.T) {
	err := ValidateCronExpression("not-a-cron")

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeValidation), "expected CodeValidation, got %v", err)
}

func TestValidateCronExpression_Valid_Nil(t *testing.T) {
	// "0 * * * *" — fire at minute 0 of every hour, a well-formed 5-field expression.
	err := ValidateCronExpression("0 * * * *")

	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// NewScheduleService
// ---------------------------------------------------------------------------

func TestNewScheduleService(t *testing.T) {
	repo := &mockScheduleRepo{}
	svc := NewScheduleService(repo, slog.Default())
	require.NotNil(t, svc)
}

// ---------------------------------------------------------------------------
// ListSchedules
// ---------------------------------------------------------------------------

func TestListSchedules_Delegates(t *testing.T) {
	ctx := context.Background()
	wantSchedules := []*db.Schedule{
		{ID: uuid.New(), Name: "nightly-scan", CronExpression: "0 2 * * *"},
		{ID: uuid.New(), Name: "hourly-discovery", CronExpression: "0 * * * *"},
	}
	wantTotal := int64(2)
	filters := db.ScheduleFilters{Enabled: true, JobType: db.ScheduledJobTypeScan}

	var gotFilters db.ScheduleFilters
	var gotOffset, gotLimit int

	repo := &mockScheduleRepo{
		listSchedulesFn: func(
			ctx context.Context, f db.ScheduleFilters, offset, limit int,
		) ([]*db.Schedule, int64, error) {
			gotFilters = f
			gotOffset = offset
			gotLimit = limit
			return wantSchedules, wantTotal, nil
		},
	}

	svc := NewScheduleService(repo, slog.Default())
	got, total, err := svc.ListSchedules(ctx, filters, 0, 10)

	require.NoError(t, err)
	assert.Equal(t, wantSchedules, got)
	assert.Equal(t, wantTotal, total)
	assert.Equal(t, filters, gotFilters)
	assert.Equal(t, 0, gotOffset)
	assert.Equal(t, 10, gotLimit)
}

// ---------------------------------------------------------------------------
// CreateSchedule
// ---------------------------------------------------------------------------

func TestCreateSchedule_InvalidCron_Error(t *testing.T) {
	ctx := context.Background()

	// createScheduleFn intentionally left nil — must not be called.
	repo := &mockScheduleRepo{}
	svc := NewScheduleService(repo, slog.Default())

	input := db.CreateScheduleInput{
		Name:           "bad-schedule",
		CronExpression: "every monday at noon",
	}

	got, err := svc.CreateSchedule(ctx, input)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.IsCode(err, errors.CodeValidation), "expected CodeValidation, got %v", err)
}

func TestCreateSchedule_Valid_DelegatesToRepo(t *testing.T) {
	ctx := context.Background()
	input := db.CreateScheduleInput{
		Name:           "hourly-scan",
		JobType:        db.ScheduledJobTypeScan,
		CronExpression: "0 * * * *",
		Enabled:        true,
	}
	want := &db.Schedule{ID: uuid.New(), Name: "hourly-scan", CronExpression: "0 * * * *"}

	var gotInput db.CreateScheduleInput
	repo := &mockScheduleRepo{
		createScheduleFn: func(ctx context.Context, in db.CreateScheduleInput) (*db.Schedule, error) {
			gotInput = in
			return want, nil
		},
	}

	svc := NewScheduleService(repo, slog.Default())
	got, err := svc.CreateSchedule(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, input, gotInput)
}

// ---------------------------------------------------------------------------
// GetSchedule
// ---------------------------------------------------------------------------

func TestGetSchedule_Delegates(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	want := &db.Schedule{ID: id, Name: "daily-discovery", CronExpression: "0 3 * * *"}

	var gotID uuid.UUID
	repo := &mockScheduleRepo{
		getScheduleFn: func(ctx context.Context, in uuid.UUID) (*db.Schedule, error) {
			gotID = in
			return want, nil
		},
	}

	svc := NewScheduleService(repo, slog.Default())
	got, err := svc.GetSchedule(ctx, id)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, id, gotID)
}

// ---------------------------------------------------------------------------
// UpdateSchedule
// ---------------------------------------------------------------------------

func TestUpdateSchedule_NilCronExpression_SkipsValidation(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	enabled := false
	// CronExpression is nil — the service must skip cron validation entirely.
	input := db.UpdateScheduleInput{Enabled: &enabled}
	want := &db.Schedule{ID: id, Enabled: false}

	var gotID uuid.UUID
	var gotInput db.UpdateScheduleInput
	repo := &mockScheduleRepo{
		updateScheduleFn: func(ctx context.Context, in uuid.UUID, inp db.UpdateScheduleInput) (*db.Schedule, error) {
			gotID = in
			gotInput = inp
			return want, nil
		},
	}

	svc := NewScheduleService(repo, slog.Default())
	got, err := svc.UpdateSchedule(ctx, id, input)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, id, gotID)
	assert.Equal(t, input, gotInput)
}

func TestUpdateSchedule_NonNilInvalidCron_Error(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	badCron := "every tuesday"
	input := db.UpdateScheduleInput{CronExpression: &badCron}

	// updateScheduleFn intentionally left nil — must not be called.
	repo := &mockScheduleRepo{}
	svc := NewScheduleService(repo, slog.Default())

	got, err := svc.UpdateSchedule(ctx, id, input)

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.IsCode(err, errors.CodeValidation), "expected CodeValidation, got %v", err)
}

func TestUpdateSchedule_NonNilValidCron_Delegates(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	newCron := "30 6 * * 1" // every Monday at 06:30
	input := db.UpdateScheduleInput{CronExpression: &newCron}
	want := &db.Schedule{ID: id, CronExpression: newCron}

	var gotID uuid.UUID
	var gotInput db.UpdateScheduleInput
	repo := &mockScheduleRepo{
		updateScheduleFn: func(ctx context.Context, in uuid.UUID, inp db.UpdateScheduleInput) (*db.Schedule, error) {
			gotID = in
			gotInput = inp
			return want, nil
		},
	}

	svc := NewScheduleService(repo, slog.Default())
	got, err := svc.UpdateSchedule(ctx, id, input)

	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, id, gotID)
	assert.Equal(t, input, gotInput)
}

// ---------------------------------------------------------------------------
// DeleteSchedule
// ---------------------------------------------------------------------------

func TestDeleteSchedule_Delegates(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	called := false

	repo := &mockScheduleRepo{
		deleteScheduleFn: func(ctx context.Context, in uuid.UUID) error {
			called = true
			assert.Equal(t, id, in)
			return nil
		},
	}

	svc := NewScheduleService(repo, slog.Default())
	err := svc.DeleteSchedule(ctx, id)

	require.NoError(t, err)
	assert.True(t, called, "expected DeleteSchedule to be called on the repository")
}

// ---------------------------------------------------------------------------
// EnableSchedule
// ---------------------------------------------------------------------------

func TestEnableSchedule_Delegates(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	called := false

	repo := &mockScheduleRepo{
		enableScheduleFn: func(ctx context.Context, in uuid.UUID) error {
			called = true
			assert.Equal(t, id, in)
			return nil
		},
	}

	svc := NewScheduleService(repo, slog.Default())
	err := svc.EnableSchedule(ctx, id)

	require.NoError(t, err)
	assert.True(t, called, "expected EnableSchedule to be called on the repository")
}

// ---------------------------------------------------------------------------
// DisableSchedule
// ---------------------------------------------------------------------------

func TestDisableSchedule_Delegates(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	called := false

	repo := &mockScheduleRepo{
		disableScheduleFn: func(ctx context.Context, in uuid.UUID) error {
			called = true
			assert.Equal(t, id, in)
			return nil
		},
	}

	svc := NewScheduleService(repo, slog.Default())
	err := svc.DisableSchedule(ctx, id)

	require.NoError(t, err)
	assert.True(t, called, "expected DisableSchedule to be called on the repository")
}

// ---------------------------------------------------------------------------
// NextRun
// ---------------------------------------------------------------------------

func TestNextRun_GetScheduleError_Error(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()
	repoErr := fmt.Errorf("schedule not found")

	repo := &mockScheduleRepo{
		getScheduleFn: func(ctx context.Context, in uuid.UUID) (*db.Schedule, error) {
			return nil, repoErr
		},
	}

	svc := NewScheduleService(repo, slog.Default())
	got, err := svc.NextRun(ctx, id)

	require.Error(t, err)
	assert.Equal(t, repoErr, err)
	assert.True(t, got.IsZero(), "expected zero time on error, got %v", got)
}

func TestNextRun_InvalidCronInDB_Error(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	// A schedule that somehow ended up in the DB with a malformed cron expression.
	schedule := &db.Schedule{
		ID:             id,
		Name:           "broken-schedule",
		CronExpression: "not valid cron at all",
	}

	repo := &mockScheduleRepo{
		getScheduleFn: func(ctx context.Context, in uuid.UUID) (*db.Schedule, error) {
			return schedule, nil
		},
	}

	svc := NewScheduleService(repo, slog.Default())
	got, err := svc.NextRun(ctx, id)

	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeValidation), "expected CodeValidation, got %v", err)
	assert.True(t, got.IsZero(), "expected zero time on error, got %v", got)
}

func TestNextRun_Valid_ReturnsFutureTime(t *testing.T) {
	ctx := context.Background()
	id := uuid.New()

	// "0 * * * *" fires at the top of every hour — always at least a second away.
	schedule := &db.Schedule{
		ID:             id,
		Name:           "hourly-scan",
		CronExpression: "0 * * * *",
	}

	repo := &mockScheduleRepo{
		getScheduleFn: func(ctx context.Context, in uuid.UUID) (*db.Schedule, error) {
			return schedule, nil
		},
	}

	before := time.Now()

	svc := NewScheduleService(repo, slog.Default())
	got, err := svc.NextRun(ctx, id)

	require.NoError(t, err)
	assert.False(t, got.IsZero(), "NextRun should not return a zero time")
	assert.True(t, got.After(before), "NextRun result %v should be after the test start time %v", got, before)
}
