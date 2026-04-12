// Package db — unit tests for ProfileRepository.GetProfileStats using sqlmock.
package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// ── GetProfileStats ───────────────────────────────────────────────────────────

func TestProfileRepository_GetProfileStats_OK(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewProfileRepository(db)

	profileID := "quick-scan"
	lastUsed := time.Now().UTC()
	avg := 12.5

	// EXISTS check
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(profileID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Aggregate query
	mock.ExpectQuery(`SELECT`).
		WithArgs(profileID).
		WillReturnRows(sqlmock.NewRows([]string{"total_scans", "unique_hosts", "last_used", "avg_hosts_found"}).
			AddRow(5, 3, lastUsed, avg))

	stats, err := repo.GetProfileStats(context.Background(), profileID)
	require.NoError(t, err)
	assert.Equal(t, profileID, stats.ProfileID)
	assert.Equal(t, 5, stats.TotalScans)
	assert.Equal(t, 3, stats.UniqueHosts)
	require.NotNil(t, stats.LastUsed)
	assert.WithinDuration(t, lastUsed, *stats.LastUsed, time.Second)
	require.NotNil(t, stats.AvgHostsFound)
	assert.InDelta(t, 12.5, *stats.AvgHostsFound, 0.001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProfileRepository_GetProfileStats_NoScans(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewProfileRepository(db)

	profileID := "empty-profile"

	// EXISTS check — profile exists
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(profileID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	// Aggregate query returns zero/null values (no scan_jobs rows for this profile)
	mock.ExpectQuery(`SELECT`).
		WithArgs(profileID).
		WillReturnRows(sqlmock.NewRows([]string{"total_scans", "unique_hosts", "last_used", "avg_hosts_found"}).
			AddRow(0, 0, nil, nil))

	stats, err := repo.GetProfileStats(context.Background(), profileID)
	require.NoError(t, err)
	assert.Equal(t, profileID, stats.ProfileID)
	assert.Equal(t, 0, stats.TotalScans)
	assert.Equal(t, 0, stats.UniqueHosts)
	assert.Nil(t, stats.LastUsed)
	assert.Nil(t, stats.AvgHostsFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProfileRepository_GetProfileStats_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewProfileRepository(db)

	profileID := "does-not-exist"

	// EXISTS check — profile not found
	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(profileID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	_, err := repo.GetProfileStats(context.Background(), profileID)
	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err), "expected not-found error, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProfileRepository_GetProfileStats_ExistsQueryError(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewProfileRepository(db)

	profileID := "quick-scan"

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(profileID).
		WillReturnError(fmt.Errorf("connection lost"))

	_, err := repo.GetProfileStats(context.Background(), profileID)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestProfileRepository_GetProfileStats_AggregateQueryError(t *testing.T) {
	db, mock := newMockDB(t)
	repo := NewProfileRepository(db)

	profileID := "quick-scan"

	mock.ExpectQuery(`SELECT EXISTS`).
		WithArgs(profileID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	mock.ExpectQuery(`SELECT`).
		WithArgs(profileID).
		WillReturnError(fmt.Errorf("aggregate query failed"))

	_, err := repo.GetProfileStats(context.Background(), profileID)
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
