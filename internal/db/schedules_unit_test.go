// Package db provides unit tests for scheduled_jobs database operations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// scheduleColumns are the columns returned by scheduled_jobs queries.
var scheduleColumns = []string{
	"id", "name", "type", "cron_expression", "config",
	"enabled", "last_run", "next_run", "created_at",
}

// ── GetSchedule ───────────────────────────────────────────────────────────────

func TestGetSchedule_Unit(t *testing.T) {
	id := uuid.New()

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnError(sql.ErrNoRows)

		_, err := NewScheduleRepository(db).GetSchedule(context.Background(), id)

		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound),
			"expected CodeNotFound, got: %v", err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("db error"))

		_, err := NewScheduleRepository(db).GetSchedule(context.Background(), id)

		require.Error(t, err)
		assert.False(t, errors.IsCode(err, errors.CodeNotFound))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		now := time.Now()

		mock.ExpectQuery("SELECT").WillReturnRows(
			sqlmock.NewRows(scheduleColumns).AddRow(
				id, "Daily Scan", "scan", "0 0 * * *", []byte("{}"),
				true, nil, nil, now,
			),
		)

		s, err := NewScheduleRepository(db).GetSchedule(context.Background(), id)

		require.NoError(t, err)
		require.NotNil(t, s)
		assert.Equal(t, "scan", s.JobType)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ── DeleteSchedule ────────────────────────────────────────────────────────────

func TestDeleteSchedule_Unit(t *testing.T) {
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(1, 1))

		err := NewScheduleRepository(db).DeleteSchedule(context.Background(), id)

		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(0, 0))

		err := NewScheduleRepository(db).DeleteSchedule(context.Background(), id)

		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound),
			"expected CodeNotFound, got: %v", err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec("DELETE").WillReturnError(fmt.Errorf("connection refused"))

		err := NewScheduleRepository(db).DeleteSchedule(context.Background(), id)

		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ── EnableSchedule ────────────────────────────────────────────────────────────

func TestEnableSchedule_Unit(t *testing.T) {
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec("UPDATE scheduled_jobs").WillReturnResult(sqlmock.NewResult(0, 1))

		err := NewScheduleRepository(db).EnableSchedule(context.Background(), id)

		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec("UPDATE scheduled_jobs").WillReturnResult(sqlmock.NewResult(0, 0))

		err := NewScheduleRepository(db).EnableSchedule(context.Background(), id)

		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound),
			"expected CodeNotFound, got: %v", err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec("UPDATE scheduled_jobs").WillReturnError(fmt.Errorf("update failed"))

		err := NewScheduleRepository(db).EnableSchedule(context.Background(), id)

		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ── DisableSchedule ───────────────────────────────────────────────────────────

func TestDisableSchedule_Unit(t *testing.T) {
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec("UPDATE scheduled_jobs").WillReturnResult(sqlmock.NewResult(0, 1))

		err := NewScheduleRepository(db).DisableSchedule(context.Background(), id)

		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec("UPDATE scheduled_jobs").WillReturnResult(sqlmock.NewResult(0, 0))

		err := NewScheduleRepository(db).DisableSchedule(context.Background(), id)

		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound),
			"expected CodeNotFound, got: %v", err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec("UPDATE scheduled_jobs").WillReturnError(fmt.Errorf("update failed"))

		err := NewScheduleRepository(db).DisableSchedule(context.Background(), id)

		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ── CreateSchedule ────────────────────────────────────────────────────────────

func TestCreateSchedule_Unit(t *testing.T) {
	id := uuid.New()

	validInput := CreateScheduleInput{
		Name:           "S",
		JobType:        "scan",
		CronExpression: "* * * * *",
		Enabled:        true,
	}

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("INSERT INTO scheduled_jobs").
			WillReturnError(fmt.Errorf("insert failed"))

		_, err := NewScheduleRepository(db).CreateSchedule(context.Background(), validInput)

		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		now := time.Now()

		mock.ExpectQuery("INSERT INTO scheduled_jobs").WillReturnRows(
			sqlmock.NewRows(scheduleColumns).AddRow(
				id, "S", "scan", "* * * * *", []byte("{}"),
				true, nil, nil, now,
			),
		)

		s, err := NewScheduleRepository(db).CreateSchedule(context.Background(), validInput)

		require.NoError(t, err)
		require.NotNil(t, s)
		assert.Equal(t, "scan", s.JobType)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
