// Package scanning provides mock-based unit tests for storeScanResults that
// exercise paths requiring a database without needing a live PostgreSQL instance.
// sqlmock simulates the database driver, so these tests run in -short mode.
//
// Primary motivation: the nil-scanID else branch (line 535, scanJob struct
// literal) is unreachable in unit tests because all DB-backed tests call
// setupTestDB which skips under testing.Short().  These mock tests fill that gap.
package scanning

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

// newMockScanDB creates a *db.DB backed by a sqlmock driver.
// Returns the wrapped DB, the mock controller, and a cleanup function.
func newMockScanDB(t *testing.T) (*db.DB, sqlmock.Sqlmock, func()) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	wrappedDB := &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}
	cleanup := func() { _ = sqlDB.Close() }
	return wrappedDB, mock, cleanup
}

// ---------------------------------------------------------------------------
// storeScanResults — non-nil scanID path (UPDATE existing job)
// ---------------------------------------------------------------------------

// TestStoreScanResults_WithScanID_Mock_Success exercises the `if scanID != nil`
// branch: an ExecContext UPDATE is issued and storeScanResults returns nil.
func TestStoreScanResults_WithScanID_Mock_Success(t *testing.T) {
	database, mock, cleanup := newMockScanDB(t)
	defer cleanup()

	ctx := context.Background()
	scanID := uuid.New()

	config := &ScanConfig{
		Targets:  []string{"10.0.0.1"},
		Ports:    "80",
		ScanType: "connect",
		ScanID:   &scanID,
	}
	result := &ScanResult{
		StartTime: time.Now(),
		Duration:  1 * time.Second,
		Stats:     HostStats{Up: 0, Down: 0, Total: 0},
		Hosts:     []Host{},
	}

	// UPDATE scan_jobs SET scan_stats = $1 WHERE id = $2
	mock.ExpectExec(`UPDATE scan_jobs`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := storeScanResults(ctx, database, config, result, &scanID)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestStoreScanResults_WithScanID_Mock_UpdateFails verifies that a failed UPDATE
// is non-fatal: storeScanResults logs a warning and still returns nil.
func TestStoreScanResults_WithScanID_Mock_UpdateFails(t *testing.T) {
	database, mock, cleanup := newMockScanDB(t)
	defer cleanup()

	ctx := context.Background()
	scanID := uuid.New()

	config := &ScanConfig{
		Targets:  []string{"10.0.0.1"},
		Ports:    "80",
		ScanType: "connect",
		ScanID:   &scanID,
	}
	result := &ScanResult{
		StartTime: time.Now(),
		Duration:  1 * time.Second,
		Stats:     HostStats{Up: 0, Down: 0, Total: 0},
		Hosts:     []Host{},
	}

	// UPDATE fails — must be non-fatal.
	mock.ExpectExec(`UPDATE scan_jobs`).
		WillReturnError(errors.New("connection reset"))

	err := storeScanResults(ctx, database, config, result, &scanID)
	require.NoError(t, err, "UPDATE failure is non-fatal; storeScanResults must still return nil")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---------------------------------------------------------------------------
// storeScanResults — nil scanID path (CREATE new job — covers line 535)
// ---------------------------------------------------------------------------

// TestStoreScanResults_NilScanID_Mock_Success is the primary test covering
// line 535 (the db.ScanJob struct literal in the else branch).
// With scanID == nil, storeScanResults must:
//  1. Call findOrCreateNetwork — mock SELECT returns an existing network row.
//  2. Build the db.ScanJob struct literal (line 535).
//  3. Call jobRepo.Create via NamedQueryContext (converts named params →
//     positional, then calls QueryxContext; no Prepare step).
//  4. Return nil when Hosts is empty.
func TestStoreScanResults_NilScanID_Mock_Success(t *testing.T) {
	database, mock, cleanup := newMockScanDB(t)
	defer cleanup()

	ctx := context.Background()
	networkID := uuid.New()
	now := time.Now()

	config := &ScanConfig{
		Targets:  []string{"10.0.0.1"},
		Ports:    "80",
		ScanType: "connect",
	}
	result := &ScanResult{
		StartTime: now,
		Duration:  1 * time.Second,
		Stats:     HostStats{Up: 0, Down: 0, Total: 0},
		Hosts:     []Host{}, // empty — avoids host-processing DB calls
	}

	// findOrCreateNetwork: SELECT id FROM networks WHERE cidr = $1
	// Returns an existing row so no INSERT into networks is required.
	mock.ExpectQuery(`SELECT id FROM networks`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(networkID.String()))

	// jobRepo.Create: NamedQueryContext resolves named params in order:
	// id, network_id, status, started_at, completed_at, scan_stats → 6 args.
	mock.ExpectQuery(`INSERT INTO scan_jobs`).
		WithArgs(
			sqlmock.AnyArg(), // id
			sqlmock.AnyArg(), // network_id
			sqlmock.AnyArg(), // status
			sqlmock.AnyArg(), // started_at
			sqlmock.AnyArg(), // completed_at
			sqlmock.AnyArg(), // scan_stats
		).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(now))

	err := storeScanResults(ctx, database, config, result, nil)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestStoreScanResults_NilScanID_Mock_NetworkNotFound exercises the path where
// the target CIDR is absent from the DB, so findOrCreateNetwork falls through
// to INSERT a new ad-hoc network row before creating the scan_job.
func TestStoreScanResults_NilScanID_Mock_NetworkNotFound(t *testing.T) {
	database, mock, cleanup := newMockScanDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	config := &ScanConfig{
		Targets:  []string{"10.0.0.2"},
		Ports:    "443",
		ScanType: "connect",
	}
	result := &ScanResult{
		StartTime: now,
		Duration:  1 * time.Second,
		Stats:     HostStats{Up: 0, Down: 0, Total: 0},
		Hosts:     []Host{},
	}

	// findOrCreateNetwork: SELECT returns an empty result set.
	// sql.Row.Scan will return sql.ErrNoRows, triggering the INSERT path.
	mock.ExpectQuery(`SELECT id FROM networks`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // no rows

	// findOrCreateNetwork: INSERT INTO networks (ad-hoc entry).
	// Args: id, name, cidr, scan_ports, scan_type (5 positional).
	mock.ExpectExec(`INSERT INTO networks`).
		WithArgs(
			sqlmock.AnyArg(), // id
			sqlmock.AnyArg(), // name
			sqlmock.AnyArg(), // cidr
			sqlmock.AnyArg(), // scan_ports
			sqlmock.AnyArg(), // scan_type
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// jobRepo.Create: INSERT INTO scan_jobs.
	mock.ExpectQuery(`INSERT INTO scan_jobs`).
		WithArgs(
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"created_at"}).AddRow(now))

	err := storeScanResults(ctx, database, config, result, nil)
	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestStoreScanResults_NilScanID_Mock_NetworkQueryFails exercises the early-exit
// when findOrCreateNetwork encounters a fatal (non-ErrNoRows) SELECT error.
func TestStoreScanResults_NilScanID_Mock_NetworkQueryFails(t *testing.T) {
	database, mock, cleanup := newMockScanDB(t)
	defer cleanup()

	ctx := context.Background()

	config := &ScanConfig{
		Targets:  []string{"10.0.0.1"},
		Ports:    "80",
		ScanType: "connect",
	}
	result := &ScanResult{
		StartTime: time.Now(),
		Duration:  1 * time.Second,
		Hosts:     []Host{},
	}

	// SELECT returns a connection-level error (not sql.ErrNoRows).
	mock.ExpectQuery(`SELECT id FROM networks`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnError(errors.New("connection refused"))

	err := storeScanResults(ctx, database, config, result, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find or create network")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestStoreScanResults_NilScanID_Mock_CreateJobFails covers the error branch
// immediately after the scanJob struct is built at line 535: jobRepo.Create
// returns an error and storeScanResults must propagate it.
func TestStoreScanResults_NilScanID_Mock_CreateJobFails(t *testing.T) {
	database, mock, cleanup := newMockScanDB(t)
	defer cleanup()

	ctx := context.Background()
	networkID := uuid.New()

	config := &ScanConfig{
		Targets:  []string{"10.0.0.1"},
		Ports:    "80",
		ScanType: "connect",
	}
	result := &ScanResult{
		StartTime: time.Now(),
		Duration:  1 * time.Second,
		Stats:     HostStats{Up: 0, Down: 0, Total: 0},
		Hosts:     []Host{},
	}

	// findOrCreateNetwork succeeds (existing network).
	mock.ExpectQuery(`SELECT id FROM networks`).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(networkID.String()))

	// jobRepo.Create fails — e.g. unique constraint on id.
	mock.ExpectQuery(`INSERT INTO scan_jobs`).
		WillReturnError(errors.New("unique constraint violation"))

	err := storeScanResults(ctx, database, config, result, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create scan job")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestStoreScanResults_NilScanID_Mock_NoTargets verifies that storeScanResults
// propagates the "no targets" error produced by findOrCreateNetwork when
// ScanConfig.Targets is empty — no DB calls are made in this case.
func TestStoreScanResults_NilScanID_Mock_NoTargets(t *testing.T) {
	database, _, cleanup := newMockScanDB(t)
	defer cleanup()

	ctx := context.Background()

	config := &ScanConfig{
		Targets:  []string{},
		Ports:    "80",
		ScanType: "connect",
	}
	result := &ScanResult{
		StartTime: time.Now(),
		Hosts:     []Host{},
	}

	err := storeScanResults(ctx, database, config, result, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find or create network")
}
