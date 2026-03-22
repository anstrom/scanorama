// Package db provides additional unit tests for host database operations.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// ── GetHost ───────────────────────────────────────────────────────────────────

func TestGetHost_Unit(t *testing.T) {
	id := uuid.New()

	// Both error cases return before fetchHostPorts fires, so no additional
	// mock expectations are needed beyond the initial SELECT.

	t.Run("not found returns CodeNotFound", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnError(sql.ErrNoRows)

		_, err := NewHostRepository(db).GetHost(context.Background(), id)

		require.Error(t, err)
		assert.True(t, errors.IsCode(err, errors.CodeNotFound),
			"expected CodeNotFound, got: %v", err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("db error is wrapped", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT").WillReturnError(fmt.Errorf("db error"))

		_, err := NewHostRepository(db).GetHost(context.Background(), id)

		require.Error(t, err)
		assert.False(t, errors.IsCode(err, errors.CodeNotFound))
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// ── ListHosts ─────────────────────────────────────────────────────────────────

func TestListHosts_Unit(t *testing.T) {
	t.Run("count error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT COUNT").
			WillReturnError(fmt.Errorf("count failed"))

		_, _, err := NewHostRepository(db).ListHosts(context.Background(), &HostFilters{}, 0, 10)

		require.Error(t, err)
	})

	t.Run("list query error is propagated", func(t *testing.T) {
		db, mock := newMockDB(t)
		mock.ExpectQuery("SELECT COUNT").
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		mock.ExpectQuery("SELECT").
			WillReturnError(fmt.Errorf("list failed"))

		_, _, err := NewHostRepository(db).ListHosts(context.Background(), &HostFilters{}, 0, 10)

		require.Error(t, err)
	})
}
