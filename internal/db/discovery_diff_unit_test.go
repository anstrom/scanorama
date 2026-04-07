// Package db provides unit tests for discovery diff database operations.
package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// diffHostColumns are the columns returned by the discovery diff host queries.
var diffHostColumns = []string{
	"id", "ip_address", "hostname", "status", "previous_status",
	"vendor", "mac_address", "last_seen", "first_seen",
}

// diffStrPtr returns a pointer to the given string — test helper for nullable columns.
func diffStrPtr(s string) *string { return &s }

// ── TestGetDiscoveryDiff_Unit ─────────────────────────────────────────────────

func TestGetDiscoveryDiff_Unit(t *testing.T) {
	ctx := context.Background()
	jobID := uuid.New()
	now := time.Now().UTC().Truncate(time.Millisecond)

	// ── job not found ─────────────────────────────────────────────────────────

	t.Run("job not found returns not-found error", func(t *testing.T) {
		mockDB, mock := newMockDB(t)

		mock.ExpectQuery("EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		diff, err := NewDiscoveryRepository(mockDB).GetDiscoveryDiff(ctx, jobID)

		require.Error(t, err)
		assert.Nil(t, diff)
		assert.True(t, errors.IsNotFound(err), "expected not-found error, got: %v", err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("existence check db error is propagated", func(t *testing.T) {
		mockDB, mock := newMockDB(t)

		mock.ExpectQuery("EXISTS").
			WillReturnError(fmt.Errorf("connection reset"))

		diff, err := NewDiscoveryRepository(mockDB).GetDiscoveryDiff(ctx, jobID)

		require.Error(t, err)
		assert.Nil(t, diff)
		assert.False(t, errors.IsNotFound(err))
		require.NoError(t, mock.ExpectationsWereMet())
	})

	// ── happy path — all three categories populated ───────────────────────────

	t.Run("happy path — new, gone, and changed hosts all populated", func(t *testing.T) {
		mockDB, mock := newMockDB(t)

		hostID1 := uuid.New()
		hostID2 := uuid.New()
		hostID3 := uuid.New()
		hostname := "server.local"
		vendor := "Cisco"
		prevStatus := "up"
		macAddr := "aa:bb:cc:dd:ee:ff"

		// 1. existence check
		mock.ExpectQuery("EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		// 2. new hosts — 1 row
		mock.ExpectQuery("first_seen >= dj").
			WillReturnRows(sqlmock.NewRows(diffHostColumns).AddRow(
				hostID1, "10.0.0.1", &hostname, "up", nil, &vendor, &macAddr, now, now,
			))

		// 3. gone hosts — 1 row
		mock.ExpectQuery("host_timeout_events").
			WillReturnRows(sqlmock.NewRows(diffHostColumns).AddRow(
				hostID2, "10.0.0.2", nil, "gone", &prevStatus, nil, nil, now, now,
			))

		// 4. changed hosts — 1 row
		mock.ExpectQuery("DISTINCT ON").
			WillReturnRows(sqlmock.NewRows(diffHostColumns).AddRow(
				hostID3, "10.0.0.3", nil, "up", diffStrPtr("down"), nil, nil, now, now,
			))

		// 5. total count — 10 hosts → unchanged = 10 - 1 - 1 - 1 = 7
		mock.ExpectQuery("COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(10))

		diff, err := NewDiscoveryRepository(mockDB).GetDiscoveryDiff(ctx, jobID)

		require.NoError(t, err)
		require.NotNil(t, diff)

		assert.Equal(t, jobID, diff.JobID)

		require.Len(t, diff.NewHosts, 1, "expected 1 new host")
		assert.Equal(t, hostID1, diff.NewHosts[0].ID)
		assert.Equal(t, "10.0.0.1", diff.NewHosts[0].IPAddress)
		assert.Equal(t, "up", diff.NewHosts[0].Status)
		require.NotNil(t, diff.NewHosts[0].Hostname)
		assert.Equal(t, hostname, *diff.NewHosts[0].Hostname)
		require.NotNil(t, diff.NewHosts[0].MACAddress)
		assert.Equal(t, macAddr, *diff.NewHosts[0].MACAddress)

		require.Len(t, diff.GoneHosts, 1, "expected 1 gone host")
		assert.Equal(t, hostID2, diff.GoneHosts[0].ID)
		assert.Equal(t, "gone", diff.GoneHosts[0].Status)
		require.NotNil(t, diff.GoneHosts[0].PreviousStatus)
		assert.Equal(t, prevStatus, *diff.GoneHosts[0].PreviousStatus)

		require.Len(t, diff.ChangedHosts, 1, "expected 1 changed host")
		assert.Equal(t, hostID3, diff.ChangedHosts[0].ID)
		assert.Equal(t, "up", diff.ChangedHosts[0].Status)

		assert.Equal(t, 7, diff.UnchangedCount)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	// ── all unchanged — empty categories ─────────────────────────────────────

	t.Run("all unchanged — empty new, gone, and changed slices", func(t *testing.T) {
		mockDB, mock := newMockDB(t)

		// 1. existence check
		mock.ExpectQuery("EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		// 2. new hosts: empty result set
		mock.ExpectQuery("first_seen >= dj").
			WillReturnRows(sqlmock.NewRows(diffHostColumns))

		// 3. gone hosts: empty result set
		mock.ExpectQuery("host_timeout_events").
			WillReturnRows(sqlmock.NewRows(diffHostColumns))

		// 4. changed hosts: empty result set
		mock.ExpectQuery("DISTINCT ON").
			WillReturnRows(sqlmock.NewRows(diffHostColumns))

		// 5. total count: 5 → unchanged = 5
		mock.ExpectQuery("COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(5))

		diff, err := NewDiscoveryRepository(mockDB).GetDiscoveryDiff(ctx, jobID)

		require.NoError(t, err)
		require.NotNil(t, diff)
		assert.Equal(t, jobID, diff.JobID)
		assert.Empty(t, diff.NewHosts)
		assert.Empty(t, diff.GoneHosts)
		assert.Empty(t, diff.ChangedHosts)
		assert.Equal(t, 5, diff.UnchangedCount)

		require.NoError(t, mock.ExpectationsWereMet())
	})

	// ── unchanged count clamped to zero ───────────────────────────────────────

	t.Run("unchanged count clamped to zero when subset counts exceed total", func(t *testing.T) {
		mockDB, mock := newMockDB(t)

		hostID1 := uuid.New()
		hostID2 := uuid.New()

		// 1. existence check
		mock.ExpectQuery("EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		// 2. new hosts: 1 row
		mock.ExpectQuery("first_seen >= dj").
			WillReturnRows(sqlmock.NewRows(diffHostColumns).AddRow(
				hostID1, "10.0.0.1", nil, "up", nil, nil, nil, now, now,
			))

		// 3. gone hosts: 1 row
		mock.ExpectQuery("host_timeout_events").
			WillReturnRows(sqlmock.NewRows(diffHostColumns).AddRow(
				hostID2, "10.0.0.2", nil, "gone", nil, nil, nil, now, now,
			))

		// 4. changed hosts: empty
		mock.ExpectQuery("DISTINCT ON").
			WillReturnRows(sqlmock.NewRows(diffHostColumns))

		// 5. total = 0 → 0 - 1 - 1 - 0 = -2, should be clamped to 0
		mock.ExpectQuery("COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

		diff, err := NewDiscoveryRepository(mockDB).GetDiscoveryDiff(ctx, jobID)

		require.NoError(t, err)
		require.NotNil(t, diff)
		assert.Equal(t, 0, diff.UnchangedCount, "unchanged count should be clamped to 0, not negative")

		require.NoError(t, mock.ExpectationsWereMet())
	})
}
