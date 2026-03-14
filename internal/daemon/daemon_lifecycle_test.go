// Package daemon provides additional lifecycle tests for the daemon.
package daemon

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestDaemon creates a daemon wired for unit tests (no real DB or API server).
func newTestDaemon(t *testing.T) *Daemon {
	t.Helper()
	cfg := &config.Config{
		Daemon: config.DaemonConfig{
			PIDFile: filepath.Join(t.TempDir(), "test.pid"),
			WorkDir: t.TempDir(),
		},
	}
	d := New(cfg)
	// Suppress noisy log output.
	d.logger = log.New(io.Discard, "", 0)
	return d
}

// =============================================================================
// Stop
// =============================================================================

func TestDaemon_Stop_CancelsContext(t *testing.T) {
	d := newTestDaemon(t)

	// Verify context is alive before Stop.
	require.NoError(t, d.ctx.Err(), "context should be active before Stop")

	d.Stop()

	assert.ErrorIs(t, d.ctx.Err(), context.Canceled, "context should be canceled after Stop")
}

func TestDaemon_Stop_IdempotentOnAlreadyStopped(t *testing.T) {
	d := newTestDaemon(t)
	d.Stop()

	// A second Stop must not panic.
	assert.NotPanics(t, func() { d.Stop() })
}

// =============================================================================
// IsRunning
// =============================================================================

func TestDaemon_IsRunning_TrueBeforeStop(t *testing.T) {
	d := newTestDaemon(t)
	assert.True(t, d.IsRunning(), "IsRunning should be true on a fresh daemon")
}

func TestDaemon_IsRunning_FalseAfterStop(t *testing.T) {
	d := newTestDaemon(t)
	d.Stop()
	assert.False(t, d.IsRunning(), "IsRunning should be false after Stop")
}

func TestDaemon_IsRunning_FalseWhenContextCanceled(t *testing.T) {
	cfg := &config.Config{}
	ctx, cancel := context.WithCancel(context.Background())
	d := New(cfg)
	d.logger = log.New(io.Discard, "", 0)
	d.ctx = ctx
	d.cancel = cancel

	cancel()
	assert.False(t, d.IsRunning())
}

// =============================================================================
// GetPID
// =============================================================================

func TestDaemon_GetPID_ReturnsCurrentProcess(t *testing.T) {
	d := newTestDaemon(t)
	pid := d.GetPID()
	assert.Equal(t, os.Getpid(), pid, "GetPID should return the current process PID")
	assert.Positive(t, pid, "PID should be a positive integer")
}

// =============================================================================
// GetDatabase / GetConfig
// =============================================================================

func TestDaemon_GetDatabase_NilWhenNotInitialized(t *testing.T) {
	d := newTestDaemon(t)
	assert.Nil(t, d.GetDatabase(), "GetDatabase should return nil before initDatabase is called")
}

func TestDaemon_GetDatabase_ReturnsInjectedDB(t *testing.T) {
	d := newTestDaemon(t)
	// Inject a non-nil *db.DB pointer via the struct field (white-box test).
	fakeDB := &db.DB{}
	d.database = fakeDB
	assert.Same(t, fakeDB, d.GetDatabase(), "GetDatabase should return the injected *db.DB")
}

func TestDaemon_GetConfig_ReturnsOriginalConfig(t *testing.T) {
	cfg := &config.Config{
		API: config.APIConfig{Host: "127.0.0.1", Port: 9999},
	}
	d := New(cfg)
	d.logger = log.New(io.Discard, "", 0)

	got := d.GetConfig()
	assert.Same(t, cfg, got, "GetConfig should return the same pointer as the config passed to New")
	assert.Equal(t, 9999, got.API.Port)
}

// =============================================================================
// checkExistingPID
// =============================================================================

func TestDaemon_CheckExistingPID_NoPIDFileSucceeds(t *testing.T) {
	d := newTestDaemon(t)
	// Ensure the PID file does not exist.
	_ = os.Remove(d.pidFile)

	err := d.checkExistingPID()
	assert.NoError(t, err, "checkExistingPID should succeed when no PID file exists")
}

func TestDaemon_CheckExistingPID_StalePIDFileRemoved(t *testing.T) {
	d := newTestDaemon(t)

	// Write a PID that is almost certainly not running (max practical PID on Linux is 4194304).
	stalePID := 4194303
	require.NoError(t, os.WriteFile(d.pidFile, []byte(fmt.Sprintf("%d", stalePID)), 0o600))

	err := d.checkExistingPID()
	assert.NoError(t, err, "checkExistingPID should succeed after removing a stale PID file")
	_, statErr := os.Stat(d.pidFile)
	assert.True(t, os.IsNotExist(statErr), "stale PID file should have been removed")
}

func TestDaemon_CheckExistingPID_InvalidPIDFileRemoved(t *testing.T) {
	d := newTestDaemon(t)
	require.NoError(t, os.WriteFile(d.pidFile, []byte("not-a-pid"), 0o600))

	err := d.checkExistingPID()
	assert.NoError(t, err, "checkExistingPID should succeed after removing an invalid PID file")
	_, statErr := os.Stat(d.pidFile)
	assert.True(t, os.IsNotExist(statErr), "invalid PID file should have been removed")
}

func TestDaemon_CheckExistingPID_LivePIDReturnsError(t *testing.T) {
	d := newTestDaemon(t)

	// Write the current process PID — it is definitely still running.
	currentPID := os.Getpid()
	require.NoError(t, os.WriteFile(d.pidFile, []byte(fmt.Sprintf("%d", currentPID)), 0o600))

	err := d.checkExistingPID()
	assert.Error(t, err, "checkExistingPID should return an error when the PID is still alive")
	assert.Contains(t, err.Error(), "already running")
}

// =============================================================================
// isProcessRunning
// =============================================================================

func TestDaemon_IsProcessRunning_CurrentProcess(t *testing.T) {
	d := newTestDaemon(t)
	assert.True(t, d.isProcessRunning(os.Getpid()), "current process should be detected as running")
}

func TestDaemon_IsProcessRunning_InvalidPID(t *testing.T) {
	d := newTestDaemon(t)
	// Use a very high PID that is almost certainly not in use.
	assert.False(t, d.isProcessRunning(4194303), "non-existent PID should not be running")
}

func TestDaemon_IsProcessRunning_ZombieSafety(t *testing.T) {
	d := newTestDaemon(t)
	// Spawn a child that exits immediately, then reap it.
	cmd := os.Args[0]
	proc, err := os.StartProcess(cmd, []string{cmd, "-test.run=_NoSuchTest_"}, &os.ProcAttr{
		Files: []*os.File{nil, nil, nil},
	})
	if err != nil {
		t.Skip("cannot start child process")
	}
	state, waitErr := proc.Wait()
	if waitErr != nil || state == nil {
		t.Skip("could not wait on child process")
	}
	// After Wait() the child is reaped; its PID should no longer be live.
	assert.False(t, d.isProcessRunning(proc.Pid),
		"reaped child process should not be detected as running")
}

// =============================================================================
// performHealthCheck
// =============================================================================

func TestDaemon_PerformHealthCheck_NilDatabaseNoPanic(t *testing.T) {
	d := newTestDaemon(t)
	d.database = nil
	assert.NotPanics(t, func() { d.performHealthCheck() })
}

func TestDaemon_PerformHealthCheck_CompletesWithoutDB(t *testing.T) {
	d := newTestDaemon(t)
	d.database = nil
	// Should return immediately without doing anything harmful.
	d.performHealthCheck()
}

// =============================================================================
// GetContext
// =============================================================================

func TestDaemon_GetContext_NotNil(t *testing.T) {
	d := newTestDaemon(t)
	ctx := d.GetContext()
	assert.NotNil(t, ctx)
}

func TestDaemon_GetContext_CanceledAfterStop(t *testing.T) {
	d := newTestDaemon(t)
	ctx := d.GetContext()
	d.Stop()
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}

// =============================================================================
// cleanup
// =============================================================================

func TestDaemon_Cleanup_RemovesPIDFile(t *testing.T) {
	d := newTestDaemon(t)
	require.NoError(t, d.createPIDFile())
	_, err := os.Stat(d.pidFile)
	require.NoError(t, err, "PID file should exist before cleanup")

	d.cleanup()

	_, err = os.Stat(d.pidFile)
	assert.True(t, os.IsNotExist(err), "PID file should be removed by cleanup")
}

func TestDaemon_Cleanup_NilDatabaseNoPanic(t *testing.T) {
	d := newTestDaemon(t)
	d.database = nil
	d.apiServer = nil
	assert.NotPanics(t, func() { d.cleanup() })
}

// =============================================================================
// setupSignalHandlers — verify SIGTERM cancels the context
// =============================================================================

func TestDaemon_SetupSignalHandlers_SIGTERMCancelsContext(t *testing.T) {
	d := newTestDaemon(t)
	d.setupSignalHandlers()

	done := make(chan struct{})
	go func() {
		<-d.ctx.Done()
		close(done)
	}()

	_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)

	select {
	case <-done:
		// Context was canceled as expected.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("context was not canceled within 500 ms after SIGTERM")
	}
}
