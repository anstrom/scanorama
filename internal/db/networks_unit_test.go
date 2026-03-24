// Package db provides network and discovery-related database operations for scanorama.
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

// discoveryJobColumns are the columns returned by ListDiscoveryJobs / GetDiscoveryJob queries.
var discoveryJobColumns = []string{
	"id", "network_id", "network", "method", "started_at", "completed_at",
	"hosts_discovered", "hosts_responsive", "status", "created_at",
}

// ── ListDiscoveryJobs ─────────────────────────────────────────────────────────

func TestListDiscoveryJobs(t *testing.T) {
	now := time.Now().UTC()
	id := uuid.New()

	t.Run("count query error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).WillReturnError(fmt.Errorf("count failed"))

		_, _, err := NewDiscoveryRepository(db).ListDiscoveryJobs(context.Background(), DiscoveryFilters{}, 0, 10)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "count discovery jobs")
	})

	t.Run("list query error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("list failed"))

		_, _, err := NewDiscoveryRepository(db).ListDiscoveryJobs(context.Background(), DiscoveryFilters{}, 0, 10)
		require.Error(t, err)
	})

	t.Run("empty result", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery(`SELECT`).
			WillReturnRows(sqlmock.NewRows(discoveryJobColumns))

		jobs, total, err := NewDiscoveryRepository(db).ListDiscoveryJobs(
			context.Background(), DiscoveryFilters{}, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(0), total)
		assert.Empty(t, jobs)
	})

	t.Run("returns rows", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT COUNT`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(discoveryJobColumns).AddRow(
				id, nil, nil, "tcp", nil, nil, 0, 0, "pending", now,
			))

		jobs, total, err := NewDiscoveryRepository(db).ListDiscoveryJobs(
			context.Background(), DiscoveryFilters{}, 0, 10)
		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, jobs, 1)
		assert.Equal(t, id, jobs[0].ID)
		assert.Equal(t, "tcp", jobs[0].Method)
		assert.Equal(t, "pending", jobs[0].Status)
	})
}

// ── CreateDiscoveryJob ────────────────────────────────────────────────────────

func TestCreateDiscoveryJob(t *testing.T) {
	validInput := CreateDiscoveryJobInput{
		Networks: []string{"10.0.0.0/8"},
		Method:   "tcp",
	}

	t.Run("empty networks returns validation error", func(t *testing.T) {
		db, _ := newMockDB(t)
		_, err := NewDiscoveryRepository(db).CreateDiscoveryJob(context.Background(), CreateDiscoveryJobInput{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "networks are required")
	})

	t.Run("db error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`INSERT INTO discovery_jobs`).
			WillReturnError(fmt.Errorf("insert failed"))

		_, err := NewDiscoveryRepository(db).CreateDiscoveryJob(context.Background(), validInput)
		require.Error(t, err)
	})

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		jobID := uuid.New()
		now := time.Now().UTC()
		mock.ExpectQuery(`INSERT INTO discovery_jobs`).
			WillReturnRows(sqlmock.NewRows(discoveryJobColumns).AddRow(
				jobID, nil, "10.0.0.0/8", "tcp", nil, nil, 0, 0, "pending", now,
			))

		job, err := NewDiscoveryRepository(db).CreateDiscoveryJob(context.Background(), validInput)
		require.NoError(t, err)
		require.NotNil(t, job)
		assert.Equal(t, "pending", job.Status)
		assert.Equal(t, "tcp", job.Method)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ── GetDiscoveryJob ───────────────────────────────────────────────────────────

func TestGetDiscoveryJob(t *testing.T) {
	now := time.Now().UTC()
	id := uuid.New()

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).WillReturnError(sql.ErrNoRows)

		_, err := NewDiscoveryRepository(db).GetDiscoveryJob(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("db error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).WillReturnError(fmt.Errorf("db error"))

		_, err := NewDiscoveryRepository(db).GetDiscoveryJob(context.Background(), id)
		require.Error(t, err)
		assert.False(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery(`SELECT`).WillReturnRows(
			sqlmock.NewRows(discoveryJobColumns).AddRow(
				id, nil, "10.0.0.0/8", "tcp", nil, nil, 0, 0, "pending", now,
			))

		job, err := NewDiscoveryRepository(db).GetDiscoveryJob(context.Background(), id)
		require.NoError(t, err)
		require.NotNil(t, job)
		assert.Equal(t, id, job.ID)
		assert.Equal(t, "tcp", job.Method)
		assert.Equal(t, "pending", job.Status)
	})
}

// ── DeleteDiscoveryJob ────────────────────────────────────────────────────────

func TestDeleteDiscoveryJob(t *testing.T) {
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`DELETE`).WillReturnResult(sqlmock.NewResult(1, 1))

		err := NewDiscoveryRepository(db).DeleteDiscoveryJob(context.Background(), id)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found when zero rows affected", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`DELETE`).WillReturnResult(sqlmock.NewResult(0, 0))

		err := NewDiscoveryRepository(db).DeleteDiscoveryJob(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("db error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`DELETE`).WillReturnError(fmt.Errorf("delete failed"))

		err := NewDiscoveryRepository(db).DeleteDiscoveryJob(context.Background(), id)
		require.Error(t, err)
	})
}

// ── StartDiscoveryJob ─────────────────────────────────────────────────────────

func TestStartDiscoveryJob(t *testing.T) {
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE discovery_jobs`).WillReturnResult(sqlmock.NewResult(0, 1))

		err := NewDiscoveryRepository(db).StartDiscoveryJob(context.Background(), id)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found when zero rows", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE discovery_jobs`).WillReturnResult(sqlmock.NewResult(0, 0))

		err := NewDiscoveryRepository(db).StartDiscoveryJob(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("db error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE discovery_jobs`).WillReturnError(fmt.Errorf("update failed"))

		err := NewDiscoveryRepository(db).StartDiscoveryJob(context.Background(), id)
		require.Error(t, err)
	})
}

// ── StopDiscoveryJob ──────────────────────────────────────────────────────────

func TestStopDiscoveryJob(t *testing.T) {
	id := uuid.New()

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE discovery_jobs`).WillReturnResult(sqlmock.NewResult(0, 1))

		err := NewDiscoveryRepository(db).StopDiscoveryJob(context.Background(), id)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found when zero rows", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE discovery_jobs`).WillReturnResult(sqlmock.NewResult(0, 0))

		err := NewDiscoveryRepository(db).StopDiscoveryJob(context.Background(), id)
		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound))
	})

	t.Run("db error propagates", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectExec(`UPDATE discovery_jobs`).WillReturnError(fmt.Errorf("update failed"))

		err := NewDiscoveryRepository(db).StopDiscoveryJob(context.Background(), id)
		require.Error(t, err)
	})
}
