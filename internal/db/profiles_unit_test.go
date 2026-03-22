// Package db provides unit tests for scan profile database operations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// ── GetProfile ────────────────────────────────────────────────────────────────

func TestGetProfile_Unit(t *testing.T) {
	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnError(sql.ErrNoRows)

		_, err := NewProfileRepository(db).GetProfile(context.Background(), "test-profile")

		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound),
			"expected CodeNotFound, got: %v", err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("db error"))

		_, err := NewProfileRepository(db).GetProfile(context.Background(), "test-profile")

		require.Error(t, err)
		assert.False(t, errors.IsCode(err, errors.CodeNotFound))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ── DeleteProfile ─────────────────────────────────────────────────────────────

func TestDeleteProfile_Unit(t *testing.T) {
	const profileID = "test-profile"

	t.Run("success", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		// Existence check: exists=true, builtIn=false.
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"exists", "built_in"}).
				AddRow(true, false))
		// In-use check: profile is not referenced by any scan jobs.
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
		mock.ExpectExec("DELETE").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := NewProfileRepository(db).DeleteProfile(context.Background(), profileID)

		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		// Existence check: exists=false → CodeNotFound returned immediately.
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"exists", "built_in"}).
				AddRow(false, false))

		err := NewProfileRepository(db).DeleteProfile(context.Background(), profileID)

		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound),
			"expected CodeNotFound, got: %v", err)
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectBegin()
		mock.ExpectQuery("SELECT COUNT").
			WillReturnError(fmt.Errorf("db error"))

		err := NewProfileRepository(db).DeleteProfile(context.Background(), profileID)

		require.Error(t, err)
	})
}

// ── ListProfiles ──────────────────────────────────────────────────────────────

func TestListProfiles_Unit(t *testing.T) {
	t.Run("count error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT COUNT").
			WillReturnError(fmt.Errorf("count failed"))

		_, _, err := NewProfileRepository(db).ListProfiles(context.Background(), ProfileFilters{}, 0, 10)

		require.Error(t, err)
	})

	t.Run("list query error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
		mock.ExpectQuery("SELECT").
			WillReturnError(fmt.Errorf("list failed"))

		_, _, err := NewProfileRepository(db).ListProfiles(context.Background(), ProfileFilters{}, 0, 10)

		require.Error(t, err)
	})
}
