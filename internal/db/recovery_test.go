// Package db provides typed repository for database operations.
package db

import (
	"context"
	"fmt"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── RecoverStaleJobs ──────────────────────────────────────────────────────────

func TestRecoverStaleJobs(t *testing.T) {
	ctx := context.Background()

	t.Run("no stale jobs — both updates are no-ops", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectExec(`UPDATE scan_jobs`).
			WithArgs(staleJobMessage).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`UPDATE discovery_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		result, err := RecoverStaleJobs(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, 0, result.ScanJobsRecovered)
		assert.Equal(t, 0, result.DiscoveryJobsRecovered)
		assert.Equal(t, 0, result.Total())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("recovers stale scan jobs only", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectExec(`UPDATE scan_jobs`).
			WithArgs(staleJobMessage).
			WillReturnResult(sqlmock.NewResult(0, 3))
		mock.ExpectExec(`UPDATE discovery_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 0))

		result, err := RecoverStaleJobs(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, 3, result.ScanJobsRecovered)
		assert.Equal(t, 0, result.DiscoveryJobsRecovered)
		assert.Equal(t, 3, result.Total())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("recovers stale discovery jobs only", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectExec(`UPDATE scan_jobs`).
			WithArgs(staleJobMessage).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`UPDATE discovery_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 2))

		result, err := RecoverStaleJobs(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, 0, result.ScanJobsRecovered)
		assert.Equal(t, 2, result.DiscoveryJobsRecovered)
		assert.Equal(t, 2, result.Total())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("recovers both scan and discovery jobs", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectExec(`UPDATE scan_jobs`).
			WithArgs(staleJobMessage).
			WillReturnResult(sqlmock.NewResult(0, 5))
		mock.ExpectExec(`UPDATE discovery_jobs`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		result, err := RecoverStaleJobs(ctx, db)
		require.NoError(t, err)
		assert.Equal(t, 5, result.ScanJobsRecovered)
		assert.Equal(t, 1, result.DiscoveryJobsRecovered)
		assert.Equal(t, 6, result.Total())
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("scan_jobs update error is returned immediately", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectExec(`UPDATE scan_jobs`).
			WithArgs(staleJobMessage).
			WillReturnError(fmt.Errorf("connection reset"))

		result, err := RecoverStaleJobs(ctx, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recover stale scan jobs")
		assert.Contains(t, err.Error(), "connection reset")
		// discovery UPDATE must not be attempted after scan_jobs failure.
		assert.Equal(t, 0, result.ScanJobsRecovered)
		assert.Equal(t, 0, result.DiscoveryJobsRecovered)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("discovery_jobs update error is returned", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectExec(`UPDATE scan_jobs`).
			WithArgs(staleJobMessage).
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectExec(`UPDATE discovery_jobs`).
			WillReturnError(fmt.Errorf("timeout"))

		result, err := RecoverStaleJobs(ctx, db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "recover stale discovery jobs")
		assert.Contains(t, err.Error(), "timeout")
		// Scan count is populated even though the second update failed.
		assert.Equal(t, 2, result.ScanJobsRecovered)
		assert.Equal(t, 0, result.DiscoveryJobsRecovered)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("staleJobMessage is the expected sentinel string", func(t *testing.T) {
		// Pin the message so API consumers / UI copy doesn't silently change.
		assert.Equal(t, "interrupted by server restart", staleJobMessage)
	})
}

// ── RecoveryResult.Total ──────────────────────────────────────────────────────

func TestRecoveryResult_Total(t *testing.T) {
	cases := []struct {
		name string
		r    RecoveryResult
		want int
	}{
		{"zero value", RecoveryResult{}, 0},
		{"scans only", RecoveryResult{ScanJobsRecovered: 4}, 4},
		{"discovery only", RecoveryResult{DiscoveryJobsRecovered: 7}, 7},
		{"both", RecoveryResult{ScanJobsRecovered: 3, DiscoveryJobsRecovered: 2}, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.r.Total())
		})
	}
}
