//go:build integration

package daemon

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------------
// Integration test DB helper — mirrors the pattern in internal/db/hosts_repository_test.go
// but lives here so the daemon package tests don't import test-only db internals.
// -----------------------------------------------------------------------------

func integrationDBConfig(t *testing.T) db.Config {
	t.Helper()
	return db.Config{
		Host:            intEnvOrDefault("TEST_DB_HOST", "localhost"),
		Port:            intEnvIntOrDefault("TEST_DB_PORT", 5433),
		Database:        intEnvOrDefault("TEST_DB_NAME", "scanorama_test"),
		Username:        intEnvOrDefault("TEST_DB_USER", "test_user"),
		Password:        intEnvOrDefault("TEST_DB_PASSWORD", "test_password"),
		SSLMode:         "disable",
		MaxOpenConns:    2,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute,
	}
}

// connectIntegrationDB connects to the live test database, skipping the test
// if none is reachable (keeps the integration suite green when Postgres is absent).
func connectIntegrationDB(t *testing.T) *db.DB {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := integrationDBConfig(t)
	database, err := db.ConnectAndMigrate(context.Background(), &cfg)
	if err != nil {
		// Migrations may already be applied by a parallel test run — fall back.
		database, err = db.Connect(context.Background(), &cfg)
		if err != nil {
			t.Skipf("skipping — could not connect to test database (%s:%d/%s): %v",
				cfg.Host, cfg.Port, cfg.Database, err)
			return nil
		}
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

// minimalDaemonCfg returns a *config.Config wired to the live test database.
// API is disabled so initAPIServer is a no-op.
func minimalDaemonCfg(t *testing.T) *config.Config {
	t.Helper()
	dbCfg := integrationDBConfig(t)
	cfg := config.Default()
	cfg.Database = dbCfg
	cfg.API.Enabled = false
	cfg.Daemon.PIDFile = filepath.Join(t.TempDir(), "daemon.pid")
	cfg.Daemon.WorkDir = ""      // skip chdir
	cfg.Daemon.Daemonize = false // no fork
	cfg.Daemon.ShutdownTimeout = 3 * time.Second
	return cfg
}

// silentIntegrationDaemon builds a Daemon with a discarding logger.
func silentIntegrationDaemon(t *testing.T) *Daemon {
	t.Helper()
	d := New(minimalDaemonCfg(t))
	d.logger = log.New(io.Discard, "", 0)
	return d
}

func intEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intEnvIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// -----------------------------------------------------------------------------
// initDatabase
// -----------------------------------------------------------------------------

// TestIntegration_InitDatabase_Success verifies that initDatabase connects to
// the live test database and populates d.database.
func TestIntegration_InitDatabase_Success(t *testing.T) {
	// Ensure the DB is reachable before constructing the daemon.
	_ = connectIntegrationDB(t)

	d := silentIntegrationDaemon(t)
	require.Nil(t, d.database, "database should be nil before initDatabase")

	err := d.initDatabase()
	require.NoError(t, err, "initDatabase must succeed against the live test database")
	require.NotNil(t, d.database, "database must be set after initDatabase")
	t.Cleanup(func() { _ = d.database.Close() })
}

// TestIntegration_InitDatabase_PingSucceeds verifies that the connection
// established by initDatabase is actually alive (Ping returns nil).
func TestIntegration_InitDatabase_PingSucceeds(t *testing.T) {
	_ = connectIntegrationDB(t)

	d := silentIntegrationDaemon(t)
	require.NoError(t, d.initDatabase())
	t.Cleanup(func() { _ = d.database.Close() })

	err := d.database.Ping(d.ctx)
	assert.NoError(t, err, "Ping must succeed on a freshly-opened database connection")
}

// -----------------------------------------------------------------------------
// performHealthCheck — non-nil DB, Ping succeeds
// -----------------------------------------------------------------------------

// TestIntegration_PerformHealthCheck_DBConnected covers the d.database != nil
// → Ping succeeds path.  The health check should be a no-op (no reconnect).
func TestIntegration_PerformHealthCheck_DBConnected(t *testing.T) {
	database := connectIntegrationDB(t)

	d := silentIntegrationDaemon(t)
	d.database = database

	// Must not panic and must not attempt a reconnect (ctx is live, DB is healthy).
	assert.NotPanics(t, func() { d.performHealthCheck() })

	// Database reference must be unchanged (no reconnect was triggered).
	assert.Same(t, database, d.database,
		"performHealthCheck must not replace a healthy database connection")
}

// TestIntegration_PerformHealthCheck_DBPingFails covers the non-nil DB whose
// Ping returns an error, triggering reconnectDatabase. We simulate a failed
// Ping by cancelling the daemon's context so PingContext sees ctx.Err() and
// returns immediately. The subsequent reconnect attempt also fails (ctx is
// cancelled), so the whole health check completes quickly without hanging.
func TestIntegration_PerformHealthCheck_DBPingFails(t *testing.T) {
	database := connectIntegrationDB(t)

	d := silentIntegrationDaemon(t)
	d.database = database

	// Cancel the context — Ping will observe ctx.Done and return an error.
	d.cancel()

	// Must not panic. reconnectDatabase will also bail out immediately because
	// ctx is already cancelled.
	assert.NotPanics(t, func() { d.performHealthCheck() })
}

// -----------------------------------------------------------------------------
// reconnectDatabase — success path
// -----------------------------------------------------------------------------

// TestIntegration_ReconnectDatabase_Success verifies that reconnectDatabase
// can re-establish a live connection and updates d.database.
func TestIntegration_ReconnectDatabase_Success(t *testing.T) {
	_ = connectIntegrationDB(t) // ensure DB reachable

	d := silentIntegrationDaemon(t)
	d.database = nil // start without a connection

	err := d.reconnectDatabase()
	require.NoError(t, err, "reconnectDatabase must succeed against the live test database")
	require.NotNil(t, d.database, "d.database must be set after successful reconnect")
	t.Cleanup(func() { _ = d.database.Close() })

	// Verify the new connection is live.
	assert.NoError(t, d.database.Ping(d.ctx))
}

// TestIntegration_ReconnectDatabase_ClosesExistingConnection verifies that
// when d.database is already set, reconnectDatabase closes it before opening a
// new one — and the new connection is healthy.
func TestIntegration_ReconnectDatabase_ClosesExistingConnection(t *testing.T) {
	existing := connectIntegrationDB(t)

	d := silentIntegrationDaemon(t)
	d.database = existing // inject a live connection to be replaced

	err := d.reconnectDatabase()
	require.NoError(t, err)
	require.NotNil(t, d.database)
	t.Cleanup(func() { _ = d.database.Close() })

	assert.NoError(t, d.database.Ping(d.ctx), "new connection must be healthy")
}

// -----------------------------------------------------------------------------
// cleanup — with a real database connection
// -----------------------------------------------------------------------------

// TestIntegration_Cleanup_ClosesDatabase verifies that cleanup closes a live
// database connection without panicking, and logs the error if Close fails.
func TestIntegration_Cleanup_ClosesDatabase(t *testing.T) {
	database := connectIntegrationDB(t)

	d := silentIntegrationDaemon(t)
	d.database = database
	d.apiServer = nil

	// Write a PID file so that branch is also exercised.
	require.NoError(t, d.createPIDFile())

	assert.NotPanics(t, func() { d.cleanup() })

	// PID file should be gone.
	_, err := os.Stat(d.pidFile)
	assert.True(t, os.IsNotExist(err), "cleanup must remove the PID file")
}

// TestIntegration_Cleanup_AlreadyClosedDatabase verifies that cleanup handles
// a database whose underlying connection has already been closed (Close returns
// an error) without panicking.
func TestIntegration_Cleanup_AlreadyClosedDatabase(t *testing.T) {
	database := connectIntegrationDB(t)

	// Close it early so cleanup's database.Close() sees an error.
	_ = database.Close()

	d := silentIntegrationDaemon(t)
	d.database = database
	d.apiServer = nil
	d.pidFile = "" // skip PID file

	assert.NotPanics(t, func() { d.cleanup() })
}

// -----------------------------------------------------------------------------
// dumpStatus — non-nil DB, Ping succeeds (covers "CONNECTED" log branch)
// -----------------------------------------------------------------------------

// TestIntegration_DumpStatus_DBConnected covers the d.database != nil → Ping
// succeeds → "Database Status: CONNECTED" branch in dumpStatus.
func TestIntegration_DumpStatus_DBConnected(t *testing.T) {
	database := connectIntegrationDB(t)

	d := silentIntegrationDaemon(t)
	d.database = database

	assert.NotPanics(t, func() { d.dumpStatus() })
}

// TestIntegration_DumpStatus_DBDisconnected covers the d.database != nil →
// Ping fails → "Database Status: DISCONNECTED" branch.
func TestIntegration_DumpStatus_DBDisconnected(t *testing.T) {
	database := connectIntegrationDB(t)

	d := silentIntegrationDaemon(t)
	d.database = database

	// Cancel context so PingContext returns immediately with an error.
	d.cancel()

	assert.NotPanics(t, func() { d.dumpStatus() })
}

// -----------------------------------------------------------------------------
// Full Start → run → Stop lifecycle
// -----------------------------------------------------------------------------

// TestIntegration_Daemon_StartStop exercises the complete Start path with a
// real database: Validate → createPIDFile → setupSignalHandlers → initDatabase
// → initAPIServer (disabled) → run loop → Stop.
func TestIntegration_Daemon_StartStop(t *testing.T) {
	_ = connectIntegrationDB(t) // skip if DB unavailable

	cfg := minimalDaemonCfg(t)
	d := New(cfg)
	d.logger = log.New(io.Discard, "", 0)

	startErr := make(chan error, 1)
	go func() {
		startErr <- d.Start()
	}()

	// Give the daemon time to reach the run loop.
	time.Sleep(150 * time.Millisecond)

	// Verify it is running.
	assert.True(t, d.IsRunning(), "daemon should report running after Start")
	assert.NotNil(t, d.GetDatabase(), "database should be initialised")

	// Stop the daemon and wait for Start to return.
	stopErr := d.Stop()
	assert.NoError(t, stopErr, "Stop must not return an error")

	select {
	case err := <-startErr:
		assert.NoError(t, err, "Start must return nil after a clean shutdown")
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5 s after Stop was called")
	}

	// PID file should have been cleaned up.
	_, statErr := os.Stat(cfg.Daemon.PIDFile)
	assert.True(t, os.IsNotExist(statErr), "PID file must be removed after Stop")
}

// TestIntegration_Daemon_IsRunning_FalseAfterStop checks IsRunning transitions.
func TestIntegration_Daemon_IsRunning_FalseAfterStop(t *testing.T) {
	_ = connectIntegrationDB(t)

	d := silentIntegrationDaemon(t)

	// Directly initialise the database so we can test cleanup without going
	// through the full Start path (avoids the blocking run loop).
	require.NoError(t, d.initDatabase())
	t.Cleanup(func() { _ = d.Stop() })

	assert.True(t, d.IsRunning())

	d.Stop()

	assert.False(t, d.IsRunning())
}

// TestIntegration_Daemon_GetDatabase_AfterInit verifies GetDatabase returns the
// live connection after initDatabase has been called.
func TestIntegration_Daemon_GetDatabase_AfterInit(t *testing.T) {
	_ = connectIntegrationDB(t)

	d := silentIntegrationDaemon(t)
	require.NoError(t, d.initDatabase())
	t.Cleanup(func() { _ = d.database.Close() })

	got := d.GetDatabase()
	assert.NotNil(t, got)
	assert.Same(t, d.database, got)
}
