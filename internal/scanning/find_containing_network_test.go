// Package scanning — tests for findContainingNetwork.
package scanning

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

func newMockDB(t *testing.T) (*db.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}, mock
}

func TestFindContainingNetwork_NoMatch(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(findContainingNetworkSQL).
		WithArgs("10.0.0.5").
		WillReturnError(sql.ErrNoRows)

	id, err := findContainingNetwork(context.Background(), database, "10.0.0.5")
	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, id, "no containing network should yield uuid.Nil, not an error")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFindContainingNetwork_SingleMatch(t *testing.T) {
	database, mock := newMockDB(t)
	want := uuid.New()

	rows := sqlmock.NewRows([]string{"id"}).AddRow(want)
	mock.ExpectQuery(findContainingNetworkSQL).
		WithArgs("10.0.0.5").
		WillReturnRows(rows)

	got, err := findContainingNetwork(context.Background(), database, "10.0.0.5")
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFindContainingNetwork_LongestPrefixWins(t *testing.T) {
	database, mock := newMockDB(t)
	slash24ID := uuid.New()

	rows := sqlmock.NewRows([]string{"id"}).AddRow(slash24ID)
	mock.ExpectQuery(findContainingNetworkSQL).
		WithArgs("10.0.0.5").
		WillReturnRows(rows)

	got, err := findContainingNetwork(context.Background(), database, "10.0.0.5")
	require.NoError(t, err)
	assert.Equal(t, slash24ID, got)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFindContainingNetwork_DBError(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectQuery(findContainingNetworkSQL).
		WithArgs("10.0.0.5").
		WillReturnError(assert.AnError)

	id, err := findContainingNetwork(context.Background(), database, "10.0.0.5")
	require.Error(t, err)
	assert.Equal(t, uuid.Nil, id)
	assert.NoError(t, mock.ExpectationsWereMet())
}
