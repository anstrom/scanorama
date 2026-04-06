// Package discovery provides network discovery functionality using nmap.
package discovery

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Ullaakut/nmap/v3"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

// newMockDB wraps a go-sqlmock connection in the application's *db.DB type.
func newMockDB(t *testing.T) (*db.DB, sqlmock.Sqlmock) {
	t.Helper()
	rawDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })
	return &db.DB{DB: sqlx.NewDb(rawDB, "sqlmock")}, mock
}

// newEngine returns an Engine wired to a mock DB plus the mock handle.
func newEngine(t *testing.T) (*Engine, sqlmock.Sqlmock) {
	t.Helper()
	database, mock := newMockDB(t)
	return NewEngine(database), mock
}

// mustParseCIDR parses a CIDR string and panics on error (test helper only).
func mustParseCIDR(cidr string) net.IPNet {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(fmt.Sprintf("mustParseCIDR(%q): %v", cidr, err))
	}
	return *ipnet
}

// discoverJobInsertMatcher matches any INSERT … discovery_jobs query.
const discoverJobInsert = `INSERT INTO discovery_jobs`

// ─── Discover — validation / error paths ──────────────────────────────────────

func TestDiscover_NoNetworkSpecified_ReturnsError(t *testing.T) {
	engine := NewEngine(nil)
	cfg := &Config{} // neither Network nor Networks set

	job, err := engine.Discover(context.Background(), cfg)
	require.Error(t, err)
	assert.Nil(t, job)
	assert.Contains(t, err.Error(), "no network specified")
}

func TestDiscover_UsesNetworksSlice_WhenNetworkEmpty(t *testing.T) {
	// When Config.Network is "" but Config.Networks has an entry the first
	// entry should be used. We test the validation branch that fires before
	// any DB call (oversized network).
	engine := NewEngine(nil)
	cfg := &Config{Networks: []string{"10.0.0.0/8"}} // too large (/8 < /16 limit)

	job, err := engine.Discover(context.Background(), cfg)
	require.Error(t, err)
	assert.Nil(t, job)
	assert.Contains(t, err.Error(), "network size too large")
}

func TestDiscover_InvalidCIDR_ReturnsError(t *testing.T) {
	engine := NewEngine(nil)
	cfg := &Config{Network: "not-a-cidr"}

	job, err := engine.Discover(context.Background(), cfg)
	require.Error(t, err)
	assert.Nil(t, job)
	assert.Contains(t, err.Error(), "invalid network CIDR")
}

func TestDiscover_NetworkTooLarge_ReturnsError(t *testing.T) {
	engine := NewEngine(nil)
	// /15 is larger than the allowed /16 maximum.
	cfg := &Config{Network: "10.0.0.0/15"}

	job, err := engine.Discover(context.Background(), cfg)
	require.Error(t, err)
	assert.Nil(t, job)
	assert.Contains(t, err.Error(), "network size too large")
}

func TestDiscover_ExactlyAtLimit_Accepted(t *testing.T) {
	// /16 is exactly at the limit — validation should pass (DB will be called).
	database, mock := newMockDB(t)
	engine := NewEngine(database)

	mock.ExpectExec(discoverJobInsert).
		WillReturnResult(sqlmock.NewResult(1, 1))

	cfg := &Config{Network: "10.0.0.0/16", Method: "tcp"}
	job, err := engine.Discover(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, job)
}

func TestDiscover_ValidSmallNetwork_StartsJob(t *testing.T) {
	database, mock := newMockDB(t)
	engine := NewEngine(database)

	mock.ExpectExec(discoverJobInsert).
		WillReturnResult(sqlmock.NewResult(1, 1))

	cfg := &Config{Network: "192.168.1.0/24", Method: "tcp"}
	job, err := engine.Discover(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, db.DiscoveryJobStatusRunning, job.Status)
	assert.Equal(t, "tcp", job.Method)
}

func TestDiscover_DBFailure_ReturnsError(t *testing.T) {
	database, mock := newMockDB(t)
	engine := NewEngine(database)

	mock.ExpectExec(discoverJobInsert).
		WillReturnError(fmt.Errorf("connection refused"))

	cfg := &Config{Network: "192.168.1.0/24", Method: "tcp"}
	job, err := engine.Discover(context.Background(), cfg)
	require.Error(t, err)
	assert.Nil(t, job)
	assert.Contains(t, err.Error(), "failed to save discovery job")
}

func TestDiscover_SingleHostNetwork_Accepted(t *testing.T) {
	// /32 (single host) is valid.
	database, mock := newMockDB(t)
	engine := NewEngine(database)

	mock.ExpectExec(discoverJobInsert).
		WillReturnResult(sqlmock.NewResult(1, 1))

	cfg := &Config{Network: "192.168.1.1/32", Method: "ping"}
	job, err := engine.Discover(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, job)
}

func TestDiscover_JobHasUUID(t *testing.T) {
	database, mock := newMockDB(t)
	engine := NewEngine(database)

	mock.ExpectExec(discoverJobInsert).
		WillReturnResult(sqlmock.NewResult(1, 1))

	cfg := &Config{Network: "10.0.0.0/24", Method: "tcp"}
	job, err := engine.Discover(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.NotEqual(t, uuid.Nil, job.ID, "returned job should have a non-zero UUID")
}

func TestDiscover_PrefersSingleNetworkField(t *testing.T) {
	// Config.Network takes precedence over Config.Networks.
	database, mock := newMockDB(t)
	engine := NewEngine(database)

	mock.ExpectExec(discoverJobInsert).
		WillReturnResult(sqlmock.NewResult(1, 1))

	cfg := &Config{
		Network:  "192.168.2.0/24",
		Networks: []string{"10.0.0.0/8"}, // would fail validation if used
		Method:   "tcp",
	}
	job, err := engine.Discover(context.Background(), cfg)
	require.NoError(t, err, "Config.Network should be chosen, not the oversized Networks[0]")
	require.NotNil(t, job)
}

func TestDiscover_PropagatesNetworkID(t *testing.T) {
	// Config.NetworkID must be copied onto the returned db.DiscoveryJob.
	database, mock := newMockDB(t)
	engine := NewEngine(database)

	networkID := uuid.New()
	mock.ExpectExec(discoverJobInsert).
		WillReturnResult(sqlmock.NewResult(1, 1))

	cfg := &Config{
		Network:   "192.168.3.0/24",
		NetworkID: &networkID,
		Method:    "tcp",
	}
	job, err := engine.Discover(context.Background(), cfg)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NotNil(t, job.NetworkID, "NetworkID should be propagated from config to the job struct")
	assert.Equal(t, networkID, *job.NetworkID)
}

// ─── saveDiscoveryJob ──────────────────────────────────────────────────────────

func TestSaveDiscoveryJob_Success(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	mock.ExpectExec(discoverJobInsert).
		WillReturnResult(sqlmock.NewResult(1, 1))

	job := &db.DiscoveryJob{
		ID:              uuid.New(),
		Network:         db.NetworkAddr{IPNet: mustParseCIDR("10.0.1.0/24")},
		Method:          "tcp",
		Status:          db.DiscoveryJobStatusRunning,
		CreatedAt:       time.Now(),
		HostsDiscovered: 0,
		HostsResponsive: 0,
	}

	err := engine.saveDiscoveryJob(ctx, job)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveDiscoveryJob_DBError(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	mock.ExpectExec(discoverJobInsert).
		WillReturnError(fmt.Errorf("disk full"))

	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		Network:   db.NetworkAddr{IPNet: mustParseCIDR("10.0.1.0/24")},
		Method:    "tcp",
		Status:    db.DiscoveryJobStatusRunning,
		CreatedAt: time.Now(),
	}

	err := engine.saveDiscoveryJob(ctx, job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disk full")
}

func TestSaveDiscoveryJob_WithCompletedAt(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	mock.ExpectExec(discoverJobInsert).
		WillReturnResult(sqlmock.NewResult(1, 1))

	now := time.Now()
	job := &db.DiscoveryJob{
		ID:              uuid.New(),
		Network:         db.NetworkAddr{IPNet: mustParseCIDR("172.16.0.0/24")},
		Method:          "ping",
		Status:          db.DiscoveryJobStatusCompleted,
		CreatedAt:       now.Add(-5 * time.Minute),
		CompletedAt:     &now,
		HostsDiscovered: 42,
		HostsResponsive: 38,
	}

	err := engine.saveDiscoveryJob(ctx, job)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveDiscoveryJob_FailedStatus(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	mock.ExpectExec(discoverJobInsert).
		WillReturnResult(sqlmock.NewResult(1, 1))

	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		Network:   db.NetworkAddr{IPNet: mustParseCIDR("10.10.0.0/24")},
		Method:    "arp",
		Status:    db.DiscoveryJobStatusFailed,
		CreatedAt: time.Now(),
	}

	err := engine.saveDiscoveryJob(ctx, job)
	assert.NoError(t, err)
}

func TestSaveDiscoveryJob_WithNetworkID(t *testing.T) {
	// When NetworkID is non-nil it must appear as the second argument ($2) of
	// the INSERT so that the networks FK column is populated.
	engine, mock := newEngine(t)
	ctx := context.Background()

	networkID := uuid.New()
	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		NetworkID: &networkID,
		Network:   db.NetworkAddr{IPNet: mustParseCIDR("10.0.2.0/24")},
		Method:    "tcp",
		Status:    db.DiscoveryJobStatusRunning,
		CreatedAt: time.Now(),
	}

	// AnyArg() for all positions except network_id so we verify both the
	// argument count (9, not the old 8) and the UUID value itself.
	mock.ExpectExec(discoverJobInsert).
		WithArgs(
			sqlmock.AnyArg(), // $1 id
			&networkID,       // $2 network_id — must match the UUID we set
			sqlmock.AnyArg(), // $3 network
			sqlmock.AnyArg(), // $4 method
			sqlmock.AnyArg(), // $5 status
			sqlmock.AnyArg(), // $6 created_at
			sqlmock.AnyArg(), // $7 completed_at
			sqlmock.AnyArg(), // $8 hosts_discovered
			sqlmock.AnyArg(), // $9 hosts_responsive
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := engine.saveDiscoveryJob(ctx, job)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveDiscoveryJob_NilNetworkID(t *testing.T) {
	// When NetworkID is nil the second argument must be nil so that the
	// database/sql driver maps it to SQL NULL (preserving existing behavior).
	engine, mock := newEngine(t)
	ctx := context.Background()

	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		NetworkID: nil,
		Network:   db.NetworkAddr{IPNet: mustParseCIDR("10.0.3.0/24")},
		Method:    "tcp",
		Status:    db.DiscoveryJobStatusRunning,
		CreatedAt: time.Now(),
	}

	mock.ExpectExec(discoverJobInsert).
		WithArgs(
			sqlmock.AnyArg(),  // $1 id
			(*uuid.UUID)(nil), // $2 network_id → SQL NULL
			sqlmock.AnyArg(),  // $3 network
			sqlmock.AnyArg(),  // $4 method
			sqlmock.AnyArg(),  // $5 status
			sqlmock.AnyArg(),  // $6 created_at
			sqlmock.AnyArg(),  // $7 completed_at
			sqlmock.AnyArg(),  // $8 hosts_discovered
			sqlmock.AnyArg(),  // $9 hosts_responsive
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := engine.saveDiscoveryJob(ctx, job)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── finalizeDiscoveryJob ──────────────────────────────────────────────────────

func TestFinalizeDiscoveryJob_RunningBecomesCompleted(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	mock.ExpectExec(discoverJobInsert).
		WillReturnResult(sqlmock.NewResult(1, 1))

	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		Network:   db.NetworkAddr{IPNet: mustParseCIDR("192.168.5.0/24")},
		Method:    "tcp",
		Status:    db.DiscoveryJobStatusRunning,
		CreatedAt: time.Now(),
	}

	engine.finalizeDiscoveryJob(ctx, job)

	assert.Equal(t, db.DiscoveryJobStatusCompleted, job.Status)
	assert.NotNil(t, job.CompletedAt, "CompletedAt should be set after finalization")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFinalizeDiscoveryJob_AlreadyFailedStatusPreserved(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	mock.ExpectExec(discoverJobInsert).
		WillReturnResult(sqlmock.NewResult(1, 1))

	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		Network:   db.NetworkAddr{IPNet: mustParseCIDR("192.168.6.0/24")},
		Method:    "tcp",
		Status:    db.DiscoveryJobStatusFailed,
		CreatedAt: time.Now(),
	}

	engine.finalizeDiscoveryJob(ctx, job)

	// A failed job's status must not be overwritten with "completed".
	assert.Equal(t, db.DiscoveryJobStatusFailed, job.Status,
		"finalizeDiscoveryJob must not overwrite a failed status")
	assert.NotNil(t, job.CompletedAt)
}

func TestFinalizeDiscoveryJob_CancelledContextSkipsSave(t *testing.T) {
	engine, mock := newEngine(t)

	// Cancel the context immediately; saveDiscoveryJob should never be called.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		Network:   db.NetworkAddr{IPNet: mustParseCIDR("10.20.0.0/24")},
		Method:    "tcp",
		Status:    db.DiscoveryJobStatusRunning,
		CreatedAt: time.Now(),
	}

	// No DB expectations — the function should return early when ctx is done.
	engine.finalizeDiscoveryJob(ctx, job)

	// The function returns without saving; no expectations means mock is satisfied.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFinalizeDiscoveryJob_DBErrorLogged(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	mock.ExpectExec(discoverJobInsert).
		WillReturnError(fmt.Errorf("network timeout"))

	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		Network:   db.NetworkAddr{IPNet: mustParseCIDR("10.30.0.0/24")},
		Method:    "tcp",
		Status:    db.DiscoveryJobStatusRunning,
		CreatedAt: time.Now(),
	}

	// Should not panic even when the DB call fails.
	assert.NotPanics(t, func() {
		engine.finalizeDiscoveryJob(ctx, job)
	})
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFinalizeDiscoveryJob_SetsCompletedAtTimestamp(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	mock.ExpectExec(discoverJobInsert).
		WillReturnResult(sqlmock.NewResult(1, 1))

	before := time.Now()
	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		Network:   db.NetworkAddr{IPNet: mustParseCIDR("10.40.0.0/24")},
		Method:    "ping",
		Status:    db.DiscoveryJobStatusRunning,
		CreatedAt: before,
	}

	engine.finalizeDiscoveryJob(ctx, job)
	after := time.Now()

	require.NotNil(t, job.CompletedAt)
	assert.True(t, !job.CompletedAt.Before(before),
		"CompletedAt should be >= the time before finalization")
	assert.True(t, !job.CompletedAt.After(after),
		"CompletedAt should be <= the time after finalization")
}

// ─── WaitForCompletion ────────────────────────────────────────────────────────

func TestWaitForCompletion_Timeout(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	// Always return "running" so the function never exits the polling loop.
	rows := sqlmock.NewRows([]string{"status", "completed_at"}).
		AddRow(db.DiscoveryJobStatusRunning, nil)
	mock.ExpectQuery(`SELECT status`).WillReturnRows(rows)

	// Use a very short timeout so the test completes quickly.
	err := engine.WaitForCompletion(ctx, uuid.New(), 10*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not complete within")
}

func TestWaitForCompletion_CompletedImmediately(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"status", "completed_at"}).
		AddRow(db.DiscoveryJobStatusCompleted, nil)
	mock.ExpectQuery(`SELECT status`).WillReturnRows(rows)

	err := engine.WaitForCompletion(ctx, uuid.New(), 5*time.Second)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWaitForCompletion_FailedJob_ReturnsError(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"status", "completed_at"}).
		AddRow(db.DiscoveryJobStatusFailed, nil)
	mock.ExpectQuery(`SELECT status`).WillReturnRows(rows)

	err := engine.WaitForCompletion(ctx, uuid.New(), 5*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "discovery job failed")
}

func TestWaitForCompletion_UnknownStatus_ReturnsError(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	rows := sqlmock.NewRows([]string{"status", "completed_at"}).
		AddRow("bogus_status", nil)
	mock.ExpectQuery(`SELECT status`).WillReturnRows(rows)

	err := engine.WaitForCompletion(ctx, uuid.New(), 5*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown job status")
}

func TestWaitForCompletion_DBError_ReturnsError(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	// Return a real DB error (not the sql.ErrNoRows sentinel).
	mock.ExpectQuery(`SELECT status`).
		WillReturnError(fmt.Errorf("connection lost"))

	err := engine.WaitForCompletion(ctx, uuid.New(), 5*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to check job status")
}

func TestWaitForCompletion_ZeroTimeout_ReturnsTimeoutError(t *testing.T) {
	engine, _ := newEngine(t)
	ctx := context.Background()

	// A zero timeout means the deadline is already in the past before the
	// first iteration; the loop body is never entered.
	err := engine.WaitForCompletion(ctx, uuid.New(), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not complete within")
}

// ─── saveDiscoveredHosts ───────────────────────────────────────────────────────

func TestSaveDiscoveredHosts_Empty_NoError(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	err := engine.saveDiscoveredHosts(ctx, []Result{})
	assert.NoError(t, err)
	// No DB calls should have been made.
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveDiscoveredHosts_NewHost_Inserted(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	ip := net.ParseIP("192.168.1.50")

	// SELECT returns no rows → host does not exist yet → INSERT path.
	mock.ExpectQuery(`SELECT id FROM hosts`).
		WithArgs("192.168.1.50").
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // empty = not found

	mock.ExpectExec(`INSERT INTO hosts`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	results := []Result{
		{
			IPAddress: ip,
			Status:    "up",
			Method:    "tcp",
		},
	}

	err := engine.saveDiscoveredHosts(ctx, results)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveDiscoveredHosts_ExistingHost_Updated(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	ip := net.ParseIP("10.0.0.5")
	existingID := uuid.New().String()

	// SELECT returns an existing row → UPDATE path.
	mock.ExpectQuery(`SELECT id FROM hosts`).
		WithArgs("10.0.0.5").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingID))

	mock.ExpectExec(`UPDATE hosts`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	results := []Result{
		{
			IPAddress: ip,
			Status:    "up",
			Method:    "ping",
		},
	}

	err := engine.saveDiscoveredHosts(ctx, results)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveDiscoveredHosts_InsertError_ReturnsError(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	ip := net.ParseIP("10.0.0.10")

	mock.ExpectQuery(`SELECT id FROM hosts`).
		WithArgs("10.0.0.10").
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // not found

	mock.ExpectExec(`INSERT INTO hosts`).
		WillReturnError(fmt.Errorf("unique constraint violation"))

	results := []Result{{IPAddress: ip, Status: "up", Method: "tcp"}}

	err := engine.saveDiscoveredHosts(ctx, results)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "errors saving hosts")
}

func TestSaveDiscoveredHosts_UpdateError_ReturnsError(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	ip := net.ParseIP("10.0.0.20")
	existingID := uuid.New().String()

	mock.ExpectQuery(`SELECT id FROM hosts`).
		WithArgs("10.0.0.20").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingID))

	mock.ExpectExec(`UPDATE hosts`).
		WillReturnError(fmt.Errorf("deadlock detected"))

	results := []Result{{IPAddress: ip, Status: "down", Method: "arp"}}

	err := engine.saveDiscoveredHosts(ctx, results)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "errors saving hosts")
}

func TestSaveDiscoveredHosts_SelectError_CollectsError(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	ip := net.ParseIP("10.0.0.30")

	// Returning a real (non-ErrNoRows) error from the SELECT.
	mock.ExpectQuery(`SELECT id FROM hosts`).
		WithArgs("10.0.0.30").
		WillReturnError(fmt.Errorf("table hosts does not exist"))

	results := []Result{{IPAddress: ip, Status: "up", Method: "tcp"}}

	err := engine.saveDiscoveredHosts(ctx, results)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "errors saving hosts")
}

func TestSaveDiscoveredHosts_MultipleHosts_PartialFailure(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	ip1 := net.ParseIP("10.1.0.1")
	ip2 := net.ParseIP("10.1.0.2")

	// First host: success (insert path).
	mock.ExpectQuery(`SELECT id FROM hosts`).
		WithArgs("10.1.0.1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`INSERT INTO hosts`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Second host: SELECT succeeds but UPDATE fails.
	mock.ExpectQuery(`SELECT id FROM hosts`).
		WithArgs("10.1.0.2").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New().String()))
	mock.ExpectExec(`UPDATE hosts`).
		WillReturnError(fmt.Errorf("timeout"))

	results := []Result{
		{IPAddress: ip1, Status: "up", Method: "tcp"},
		{IPAddress: ip2, Status: "up", Method: "tcp"},
	}

	err := engine.saveDiscoveredHosts(ctx, results)
	require.Error(t, err, "a partial failure should be surfaced")
	assert.Contains(t, err.Error(), "errors saving hosts")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveDiscoveredHosts_MultipleHosts_AllSucceed(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	hosts := []Result{
		{IPAddress: net.ParseIP("172.16.0.1"), Status: "up", Method: "ping"},
		{IPAddress: net.ParseIP("172.16.0.2"), Status: "up", Method: "ping"},
		{IPAddress: net.ParseIP("172.16.0.3"), Status: "down", Method: "ping"},
	}

	for _, h := range hosts {
		mock.ExpectQuery(`SELECT id FROM hosts`).
			WithArgs(h.IPAddress.String()).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))
		mock.ExpectExec(`INSERT INTO hosts`).
			WillReturnResult(sqlmock.NewResult(1, 1))
	}

	err := engine.saveDiscoveredHosts(ctx, hosts)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ─── Engine configuration helpers ────────────────────────────────────────────

func TestNewEngine_DefaultsApplied(t *testing.T) {
	e := NewEngine(nil)
	assert.Equal(t, defaultConcurrency, e.concurrency)
	assert.Equal(t, time.Duration(defaultTimeoutSeconds)*time.Second, e.timeout)
}

func TestSetConcurrency_UpdatesField(t *testing.T) {
	e := NewEngine(nil)
	e.SetConcurrency(25)
	assert.Equal(t, 25, e.concurrency)
}

func TestSetTimeout_UpdatesField(t *testing.T) {
	e := NewEngine(nil)
	e.SetTimeout(2 * time.Minute)
	assert.Equal(t, 2*time.Minute, e.timeout)
}

// ─── convertNmapResultsToDiscovery — RTT and vendor/MAC ───────────────────────

func TestConvertNmapResults_RTT_Populated(t *testing.T) {
	engine := &Engine{}

	// SRTT = "4000" means 4000 µs = 4 ms.
	nmapResult := &nmap.Run{
		Hosts: []nmap.Host{
			{
				Addresses: []nmap.Address{
					{Addr: "192.168.1.1", AddrType: "ipv4"},
				},
				Status: nmap.Status{State: "up"},
				Times:  nmap.Times{SRTT: "4000"},
			},
		},
	}

	results := engine.convertNmapResultsToDiscovery(nmapResult, "ping")
	require.Len(t, results, 1)
	assert.Equal(t, 4*time.Millisecond, results[0].ResponseTime)
}

func TestConvertNmapResults_RTT_ZeroWhenMissing(t *testing.T) {
	engine := &Engine{}

	// Empty SRTT string → ResponseTime stays zero.
	nmapResult := &nmap.Run{
		Hosts: []nmap.Host{
			{
				Addresses: []nmap.Address{
					{Addr: "10.0.0.1", AddrType: "ipv4"},
				},
				Status: nmap.Status{State: "up"},
				Times:  nmap.Times{SRTT: ""},
			},
		},
	}

	results := engine.convertNmapResultsToDiscovery(nmapResult, "ping")
	require.Len(t, results, 1)
	assert.Equal(t, time.Duration(0), results[0].ResponseTime)
}

func TestConvertNmapResults_VendorAndMAC_Populated(t *testing.T) {
	engine := &Engine{}

	nmapResult := &nmap.Run{
		Hosts: []nmap.Host{
			{
				Addresses: []nmap.Address{
					{Addr: "192.168.1.2", AddrType: "ipv4"},
					{Addr: "aa:bb:cc:dd:ee:ff", AddrType: "mac", Vendor: "Cisco"},
				},
				Status: nmap.Status{State: "up"},
			},
		},
	}

	results := engine.convertNmapResultsToDiscovery(nmapResult, "arp")
	require.Len(t, results, 1)
	assert.Equal(t, "Cisco", results[0].Vendor)
	assert.Equal(t, "aa:bb:cc:dd:ee:ff", results[0].MACAddress)
}

func TestConvertNmapResults_MAC_NoVendor(t *testing.T) {
	engine := &Engine{}

	// MAC address present but Vendor is empty — MACAddress should be set, Vendor stays "".
	nmapResult := &nmap.Run{
		Hosts: []nmap.Host{
			{
				Addresses: []nmap.Address{
					{Addr: "192.168.1.3", AddrType: "ipv4"},
					{Addr: "11:22:33:44:55:66", AddrType: "mac", Vendor: ""},
				},
				Status: nmap.Status{State: "up"},
			},
		},
	}

	results := engine.convertNmapResultsToDiscovery(nmapResult, "arp")
	require.Len(t, results, 1)
	assert.Equal(t, "11:22:33:44:55:66", results[0].MACAddress)
	assert.Equal(t, "", results[0].Vendor)
}

// ─── saveDiscoveredHosts — vendor / RTT / nil arg paths ───────────────────────

func TestSaveDiscoveredHosts_WithVendorAndRTT_Inserted(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	ip := net.ParseIP("10.10.10.1")

	// SELECT returns no rows → INSERT path with non-nil $4/$5/$6.
	mock.ExpectQuery(`SELECT id FROM hosts`).
		WithArgs("10.10.10.1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	mock.ExpectExec(`INSERT INTO hosts`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	results := []Result{
		{
			IPAddress:    ip,
			Status:       "up",
			Method:       "arp",
			Vendor:       "Apple",
			MACAddress:   "aa:bb:cc:dd:ee:ff",
			ResponseTime: 5 * time.Millisecond,
		},
	}

	err := engine.saveDiscoveredHosts(ctx, results)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveDiscoveredHosts_WithVendorAndRTT_Updated(t *testing.T) {
	engine, mock := newEngine(t)
	ctx := context.Background()

	ip := net.ParseIP("10.10.10.2")
	existingID := uuid.New().String()

	// SELECT returns existing ID → UPDATE path with non-nil $4/$6.
	mock.ExpectQuery(`SELECT id FROM hosts`).
		WithArgs("10.10.10.2").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingID))

	mock.ExpectExec(`UPDATE hosts`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	results := []Result{
		{
			IPAddress:    ip,
			Status:       "up",
			Method:       "arp",
			Vendor:       "Dell",
			ResponseTime: 10 * time.Millisecond,
		},
	}

	err := engine.saveDiscoveredHosts(ctx, results)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSaveDiscoveredHosts_EmptyVendorAndZeroRTT(t *testing.T) {
	// Vendor="" and ResponseTime=0 → $4/$5/$6 are nil (SQL NULL).
	// Both INSERT and UPDATE paths still succeed.
	engine, mock := newEngine(t)
	ctx := context.Background()

	ip1 := net.ParseIP("10.10.10.3")
	ip2 := net.ParseIP("10.10.10.4")
	existingID := uuid.New().String()

	// First host: no existing row → INSERT with all nil nullable args.
	mock.ExpectQuery(`SELECT id FROM hosts`).
		WithArgs("10.10.10.3").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`INSERT INTO hosts`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Second host: existing row → UPDATE with all nil nullable args.
	mock.ExpectQuery(`SELECT id FROM hosts`).
		WithArgs("10.10.10.4").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(existingID))
	mock.ExpectExec(`UPDATE hosts`).
		WillReturnResult(sqlmock.NewResult(0, 1))

	results := []Result{
		// Vendor="", MACAddress="", ResponseTime=0 → all nullable args become nil.
		{IPAddress: ip1, Status: "up", Method: "ping"},
		{IPAddress: ip2, Status: "up", Method: "ping"},
	}

	err := engine.saveDiscoveredHosts(ctx, results)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
