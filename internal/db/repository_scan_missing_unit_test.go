package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/lib/pq/pqerror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// ── ScanRepository.GetProfile ─────────────────────────────────────────────
//
// GetProfile is a thin delegation to ProfileRepository.GetProfile. These tests
// verify the delegation passes through results and errors correctly; the
// underlying query logic is covered by TestGetProfile_Unit in profiles_unit_test.go.

// scanRepoProfileColumns and scanRepoProfileRow are local to this file to avoid
// redeclaring the package-level profileColumns/profileRow from profiles_unit_test.go.
var scanRepoProfileColumns = []string{
	"id", "name", "description", "os_family", "ports", "scan_type",
	"timing", "scripts", "options", "priority", "built_in",
	"created_at", "updated_at",
}

func scanRepoProfileRow(id, name string) []driver.Value {
	now := time.Now().UTC()
	return []driver.Value{
		id, name, nil, nil, "80,443", "connect",
		nil, nil, nil, 0, false,
		now, now,
	}
}

func TestScanRepository_GetProfile_Success(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery(`SELECT.*FROM scan_profiles`).
		WillReturnRows(sqlmock.NewRows(scanRepoProfileColumns).
			AddRow(scanRepoProfileRow("quick", "Quick Scan")...))

	profile, err := NewScanRepository(db).GetProfile(context.Background(), "quick")

	require.NoError(t, err)
	assert.Equal(t, "quick", profile.ID)
	assert.Equal(t, "Quick Scan", profile.Name)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScanRepository_GetProfile_NotFound(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery(`SELECT.*FROM scan_profiles`).WillReturnError(sql.ErrNoRows)

	_, err := NewScanRepository(db).GetProfile(context.Background(), "missing")

	require.Error(t, err)
	assert.True(t, errors.IsNotFound(err), "expected not-found error, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestScanRepository_GetProfile_DBError(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectQuery(`SELECT.*FROM scan_profiles`).
		WillReturnError(fmt.Errorf("connection reset"))

	_, err := NewScanRepository(db).GetProfile(context.Background(), "any")

	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── findOrCreateNetwork ───────────────────────────────────────────────────
//
// findOrCreateNetwork takes a *sql.Tx directly, so tests create a raw
// database/sql mock and begin a transaction rather than going through sqlx.

func newRawMockTx(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db, mock
}

func TestFindOrCreateNetwork_ExistingCIDR(t *testing.T) {
	rawDB, mock := newRawMockTx(t)
	networkID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM networks WHERE cidr`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(networkID))

	tx, err := rawDB.Begin()
	require.NoError(t, err)

	got, err := findOrCreateNetwork(context.Background(), tx, "corp", "10.0.0.0/8", "", "", "")

	require.NoError(t, err)
	assert.Equal(t, networkID, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFindOrCreateNetwork_NewCIDR_CreatesNetwork(t *testing.T) {
	rawDB, mock := newRawMockTx(t)
	networkID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM networks WHERE cidr`).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`SAVEPOINT`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`INSERT INTO networks`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(networkID))
	mock.ExpectExec(`RELEASE SAVEPOINT`).WillReturnResult(sqlmock.NewResult(0, 0))

	tx, err := rawDB.Begin()
	require.NoError(t, err)

	got, err := findOrCreateNetwork(context.Background(), tx, "new-net", "192.168.0.0/16", "", "", "")

	require.NoError(t, err)
	assert.Equal(t, networkID, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFindOrCreateNetwork_NameCollisionFallsBackToCIDR(t *testing.T) {
	rawDB, mock := newRawMockTx(t)
	networkID := uuid.New()

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM networks WHERE cidr`).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`SAVEPOINT`).WillReturnResult(sqlmock.NewResult(0, 0))
	// First insert: unique violation on name — should trigger CIDR fallback.
	mock.ExpectQuery(`INSERT INTO networks`).
		WillReturnError(&pq.Error{
			Code:       pqerror.UniqueViolation,
			Constraint: "networks_name_key",
			Message:    "duplicate key value",
		})
	mock.ExpectExec(`ROLLBACK TO SAVEPOINT`).WillReturnResult(sqlmock.NewResult(0, 0))
	// Retry insert using CIDR as the network name.
	mock.ExpectQuery(`INSERT INTO networks`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(networkID))
	mock.ExpectExec(`RELEASE SAVEPOINT`).WillReturnResult(sqlmock.NewResult(0, 0))

	tx, err := rawDB.Begin()
	require.NoError(t, err)

	got, err := findOrCreateNetwork(context.Background(), tx, "taken-name", "172.16.0.0/12", "", "", "")

	require.NoError(t, err)
	assert.Equal(t, networkID, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFindOrCreateNetwork_SelectErrorNotErrNoRows(t *testing.T) {
	rawDB, mock := newRawMockTx(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM networks WHERE cidr`).
		WillReturnError(fmt.Errorf("connection reset"))

	tx, err := rawDB.Begin()
	require.NoError(t, err)

	_, err = findOrCreateNetwork(context.Background(), tx, "net", "10.0.0.0/8", "", "", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "look up network by CIDR")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestFindOrCreateNetwork_SavepointError(t *testing.T) {
	rawDB, mock := newRawMockTx(t)

	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT id FROM networks WHERE cidr`).WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`SAVEPOINT`).WillReturnError(fmt.Errorf("savepoint failed"))

	tx, err := rawDB.Begin()
	require.NoError(t, err)

	_, err = findOrCreateNetwork(context.Background(), tx, "net", "10.0.0.0/8", "", "", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "savepoint before create network")
	require.NoError(t, mock.ExpectationsWereMet())
}
