// Package db provides unit tests for the local PostgreSQL bootstrap helpers.
package db

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// existsRows returns a single-column boolean result set as the EXISTS probes do.
func existsRows(exists bool) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"exists"}).AddRow(exists)
}

func TestEnsureRoleAndDatabase(t *testing.T) {
	ctx := context.Background()

	t.Run("creates role and database when neither exists", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectQuery("pg_roles").WithArgs("scanorama").WillReturnRows(existsRows(false))
		mock.ExpectExec("CREATE ROLE \"scanorama\" LOGIN").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectQuery("pg_database").WithArgs("scanorama").WillReturnRows(existsRows(false))
		mock.ExpectExec("CREATE DATABASE \"scanorama\" OWNER \"scanorama\"").
			WillReturnResult(sqlmock.NewResult(0, 0))

		res, err := EnsureRoleAndDatabase(ctx, db, "scanorama", "scanorama")

		require.NoError(t, err)
		assert.True(t, res.RoleCreated)
		assert.True(t, res.DatabaseCreated)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no-op when role and database already exist", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectQuery("pg_roles").WithArgs("scanorama").WillReturnRows(existsRows(true))
		mock.ExpectQuery("pg_database").WithArgs("scanorama").WillReturnRows(existsRows(true))

		res, err := EnsureRoleAndDatabase(ctx, db, "scanorama", "scanorama")

		require.NoError(t, err)
		assert.False(t, res.RoleCreated)
		assert.False(t, res.DatabaseCreated)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("creates only the database when the role already exists", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectQuery("pg_roles").WithArgs("scanorama").WillReturnRows(existsRows(true))
		mock.ExpectQuery("pg_database").WithArgs("scanorama").WillReturnRows(existsRows(false))
		mock.ExpectExec("CREATE DATABASE \"scanorama\" OWNER \"scanorama\"").
			WillReturnResult(sqlmock.NewResult(0, 0))

		res, err := EnsureRoleAndDatabase(ctx, db, "scanorama", "scanorama")

		require.NoError(t, err)
		assert.False(t, res.RoleCreated)
		assert.True(t, res.DatabaseCreated)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("wraps a query error", func(t *testing.T) {
		db, mock := newMockDB(t)

		mock.ExpectQuery("pg_roles").WithArgs("scanorama").WillReturnError(errors.New("connection refused"))

		_, err := EnsureRoleAndDatabase(ctx, db, "scanorama", "scanorama")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "scanorama")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("rejects an invalid role identifier before touching the database", func(t *testing.T) {
		db, _ := newMockDB(t)

		_, err := EnsureRoleAndDatabase(ctx, db, "scan; DROP DATABASE x", "scanorama")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "role")
	})

	t.Run("rejects an invalid database identifier", func(t *testing.T) {
		db, mock := newMockDB(t)
		// Both identifiers are validated before any query, so no queries run.
		_, err := EnsureRoleAndDatabase(ctx, db, "scanorama", "bad name")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "database")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestValidateIdentifier(t *testing.T) {
	valid := []string{"scanorama", "app_db", "_x", "Role1"}
	for _, id := range valid {
		assert.NoError(t, validateIdentifier(id), "expected %q to be valid", id)
	}

	invalid := []string{"", "1abc", "has space", "drop;table", "a-b", "töet"}
	for _, id := range invalid {
		assert.Error(t, validateIdentifier(id), "expected %q to be rejected", id)
	}
}
