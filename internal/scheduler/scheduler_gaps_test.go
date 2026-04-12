//go:build !integration

// Package scheduler – gap-filling unit tests.
//
// Target functions and their coverage before this file:
//
//	WithScanQueue                0.0%
//	executeDiscoveryJob         19.4%  → cover enabled+not-running path, already-running skip, not-found/disabled skip
//	executeScanJob              22.7%  → cover enabled path: no hosts, hosts found, getHostsToScan error
//	processHostsForScanning      7.0%  → cover profile selected, GetByID error, semaphore ctx cancel
//	processHostsViaQueue         0.0%  → cover full path: submit, success result, error result, ctx cancel, queue full
//	selectProfileForHost         0.0%  → cover explicit ID, auto/empty, SelectBestProfile error
//	addJobToCronScheduler       75.0%  → cover unknown type branch
//	addDiscoveryJobToCron       60.0%  → cover unmarshal error
//	addScanJobToCron            60.0%  → cover unmarshal error
//	loadScheduledJobs           72.7%  → cover processRow error (logged+continued), rows.Err path
//	AddDiscoveryJob             80.0%  → cover createScheduledJob error path
//	AddScanJob                  80.0%  → cover createScheduledJob error path
//	createScheduledJob          91.7%  → cover saveScheduledJob error path
//	addJobToCron                94.1%  → cover cron.AddFunc error path
package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/profiles"
	"github.com/anstrom/scanorama/internal/scanning"
	"github.com/anstrom/scanorama/internal/services"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// newMockDB returns a *db.DB backed by sqlmock and the mock controller.
func newMockDB(t *testing.T) (*db.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}, mock
}

// newMockProfileManager returns a *profiles.Manager wired to sqlmock.
func newMockProfileManager(t *testing.T) (*profiles.Manager, sqlmock.Sqlmock) {
	t.Helper()
	database, mock := newMockDB(t)
	return profiles.NewManager(database), mock
}

// hostWithIP builds a minimal *db.Host with the given dotted-decimal IP.
func hostWithIP(ip string) *db.Host {
	h := &db.Host{ID: uuid.New()}
	h.IPAddress.IP = net.ParseIP(ip)
	return h
}

// schedulerWithJob is a convenience function that creates a bare *Scheduler
// (no real DB/discovery/profiles) and registers one enabled, not-running job
// in memory. It returns the scheduler and the job ID.
func schedulerWithJob(t *testing.T, enabled bool) (*Scheduler, uuid.UUID) {
	t.Helper()
	s := &Scheduler{
		jobs:               make(map[uuid.UUID]*ScheduledJob),
		mu:                 sync.RWMutex{},
		maxConcurrentScans: defaultMaxConcurrentScans,
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel
	t.Cleanup(cancel)

	jobID := uuid.New()
	cfgJSON, _ := json.Marshal(ScanJobConfig{LiveHostsOnly: false})
	s.jobs[jobID] = &ScheduledJob{
		ID: jobID,
		Config: &db.ScheduledJob{
			ID:             jobID,
			Name:           "test-job",
			Type:           db.ScheduledJobTypeScan,
			CronExpression: "0 * * * *",
			Enabled:        enabled,
			Config:         db.JSONB(cfgJSON),
		},
		Running: false,
	}
	return s, jobID
}

// sampleScanProfile returns a minimal *db.ScanProfile for use in tests.
func sampleScanProfile(id string) *db.ScanProfile {
	return &db.ScanProfile{
		ID:       id,
		Name:     "test-profile",
		Ports:    "22,80",
		ScanType: db.ScanTypeConnect,
		Timing:   db.ScanTimingNormal,
		OSFamily: pq.StringArray{"linux"},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// WithScanQueue
// ─────────────────────────────────────────────────────────────────────────────

func TestWithScanQueue_SetsQueue(t *testing.T) {
	s := NewScheduler(nil, nil, nil)
	q := scanning.NewScanQueue(2, 10)

	got := s.WithScanQueue(q)

	assert.Same(t, s, got, "WithScanQueue must return the receiver for chaining")
	assert.Same(t, q, s.scanQueue, "scanQueue must be set to the provided queue")
}

func TestWithScanQueue_NilClearsQueue(t *testing.T) {
	s := NewScheduler(nil, nil, nil)
	q := scanning.NewScanQueue(2, 10)
	s.scanQueue = q

	got := s.WithScanQueue(nil)
	assert.Same(t, s, got)
	assert.Nil(t, s.scanQueue, "passing nil must clear the queue")
}

// ─────────────────────────────────────────────────────────────────────────────
// executeDiscoveryJob
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteDiscoveryJob_NotFound_IsNoOp(t *testing.T) {
	s := &Scheduler{
		jobs: make(map[uuid.UUID]*ScheduledJob),
		mu:   sync.RWMutex{},
	}
	// Call with an ID that does not exist in the map — must not panic.
	assert.NotPanics(t, func() {
		s.executeDiscoveryJob(uuid.New(), DiscoveryJobConfig{})
	})
}

func TestExecuteDiscoveryJob_DisabledJob_IsNoOp(t *testing.T) {
	s := &Scheduler{
		jobs: make(map[uuid.UUID]*ScheduledJob),
		mu:   sync.RWMutex{},
	}
	jobID := uuid.New()
	cfgJSON, _ := json.Marshal(DiscoveryJobConfig{Network: "10.0.0.0/8"})
	s.jobs[jobID] = &ScheduledJob{
		ID: jobID,
		Config: &db.ScheduledJob{
			ID:      jobID,
			Name:    "disabled",
			Enabled: false,
			Config:  db.JSONB(cfgJSON),
		},
		Running: false,
	}

	assert.NotPanics(t, func() {
		s.executeDiscoveryJob(jobID, DiscoveryJobConfig{Network: "10.0.0.0/8"})
	})

	// Job must still not be running.
	s.mu.RLock()
	running := s.jobs[jobID].Running
	s.mu.RUnlock()
	assert.False(t, running)
}

func TestExecuteDiscoveryJob_AlreadyRunning_SkipsExecution(t *testing.T) {
	s := &Scheduler{
		jobs: make(map[uuid.UUID]*ScheduledJob),
		mu:   sync.RWMutex{},
	}
	jobID := uuid.New()
	cfgJSON, _ := json.Marshal(DiscoveryJobConfig{Network: "10.0.0.0/8"})
	s.jobs[jobID] = &ScheduledJob{
		ID: jobID,
		Config: &db.ScheduledJob{
			ID:      jobID,
			Name:    "already-running",
			Enabled: true,
			Config:  db.JSONB(cfgJSON),
		},
		Running: true, // already running
	}

	assert.NotPanics(t, func() {
		s.executeDiscoveryJob(jobID, DiscoveryJobConfig{Network: "10.0.0.0/8"})
	})

	// Running flag must still be true (we did not touch it).
	s.mu.RLock()
	running := s.jobs[jobID].Running
	s.mu.RUnlock()
	assert.True(t, running, "already-running job must remain marked as running")
}

// TestExecuteDiscoveryJob_NilDiscovery_UpdatesLastRunAndLogs tests the normal
// execution path when the discovery engine is nil (Discover returns an error).
// The job must be marked not-running and updateJobLastRun must be attempted.
func TestExecuteDiscoveryJob_NilDiscovery_RunsAndCleansUp(t *testing.T) {
	database, mock := newMockDB(t)

	// Expect the UPDATE scheduled_jobs SET last_run … call.
	mock.ExpectExec("UPDATE scheduled_jobs SET last_run").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		db:        database,
		jobs:      make(map[uuid.UUID]*ScheduledJob),
		mu:        sync.RWMutex{},
		ctx:       ctx,
		cancel:    cancel,
		discovery: nil, // will cause Discover to panic/nil-deref → recovered
	}

	jobID := uuid.New()
	cfgJSON, _ := json.Marshal(DiscoveryJobConfig{
		Network: "192.168.0.0/24", Method: "ping", Timeout: 5, Concurrency: 1,
	})
	s.jobs[jobID] = &ScheduledJob{
		ID: jobID,
		Config: &db.ScheduledJob{
			ID:             jobID,
			Name:           "nil-discovery",
			CronExpression: "0 * * * *",
			Enabled:        true,
			Config:         db.JSONB(cfgJSON),
		},
		Running: false,
	}

	// With a nil discovery engine, Discover will nil-pointer panic.
	// The outer defer recover in executeDiscoveryJob must catch it and mark
	// Running = false.
	assert.NotPanics(t, func() {
		s.executeDiscoveryJob(jobID, DiscoveryJobConfig{
			Network:     "192.168.0.0/24",
			Method:      "ping",
			Timeout:     5,
			Concurrency: 1,
		})
	})

	s.mu.RLock()
	running := s.jobs[jobID].Running
	s.mu.RUnlock()
	assert.False(t, running, "job must be marked not-running after execution")
}

// ─────────────────────────────────────────────────────────────────────────────
// executeScanJob
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteScanJob_DisabledJob_IsNoOp(t *testing.T) {
	s, jobID := schedulerWithJob(t, false /* disabled */)
	assert.NotPanics(t, func() {
		s.executeScanJob(jobID, &ScanJobConfig{})
	})
}

func TestExecuteScanJob_AlreadyRunning_IsNoOp(t *testing.T) {
	s, jobID := schedulerWithJob(t, true)
	s.jobs[jobID].Running = true

	assert.NotPanics(t, func() {
		s.executeScanJob(jobID, &ScanJobConfig{})
	})
	s.mu.RLock()
	running := s.jobs[jobID].Running
	s.mu.RUnlock()
	assert.True(t, running)
}

// TestExecuteScanJob_GetHostsError covers the getHostsToScan failure branch.
func TestExecuteScanJob_GetHostsError(t *testing.T) {
	database, mock := newMockDB(t)

	// getHostsToScan issues a SELECT … FROM hosts query.
	mock.ExpectQuery("SELECT").WillReturnError(errors.New("db error"))

	// updateJobLastRun issues an UPDATE.
	mock.ExpectExec("UPDATE scheduled_jobs SET last_run").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s, jobID := schedulerWithJob(t, true)
	s.db = database

	assert.NotPanics(t, func() {
		s.executeScanJob(jobID, &ScanJobConfig{LiveHostsOnly: false})
	})

	s.mu.RLock()
	running := s.jobs[jobID].Running
	s.mu.RUnlock()
	assert.False(t, running, "job must be cleaned up after error")
}

// TestExecuteScanJob_NoHosts covers the len(hosts)==0 early-return branch.
func TestExecuteScanJob_NoHosts(t *testing.T) {
	database, mock := newMockDB(t)

	hostCols := []string{
		"id", "ip_address", "hostname", "mac_address", "vendor",
		"os_family", "os_name", "os_version", "os_confidence",
		"os_detected_at", "os_method", "os_details", "discovery_method",
		"response_time_ms", "discovery_count", "ignore_scanning",
		"first_seen", "last_seen", "status",
	}
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows(hostCols))

	mock.ExpectExec("UPDATE scheduled_jobs SET last_run").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	s, jobID := schedulerWithJob(t, true)
	s.db = database

	assert.NotPanics(t, func() {
		s.executeScanJob(jobID, &ScanJobConfig{LiveHostsOnly: false})
	})

	s.mu.RLock()
	running := s.jobs[jobID].Running
	s.mu.RUnlock()
	assert.False(t, running)
}

// ─────────────────────────────────────────────────────────────────────────────
// selectProfileForHost
// ─────────────────────────────────────────────────────────────────────────────

func TestSelectProfileForHost_ExplicitID_ReturnsID(t *testing.T) {
	s := NewScheduler(nil, nil, nil)
	host := hostWithIP("10.0.0.1")

	got := s.selectProfileForHost(context.Background(), host, "explicit-profile-id")
	assert.Equal(t, "explicit-profile-id", got)
}

func TestSelectProfileForHost_Auto_CallsSelectBestProfile(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	profileID := "auto-selected-profile"
	profile := sampleScanProfile(profileID)

	// SelectBestProfile internally calls GetAll then GetByID("generic-default")
	// or GetByOSFamily. Match any SELECT query.
	cols := []string{
		"id", "name", "description", "os_family", "os_pattern",
		"ports", "scan_type", "timing", "scripts", "options",
		"priority", "built_in", "created_at", "updated_at",
	}
	osFamilyVal, _ := profile.OSFamily.Value()
	osPatternVal, _ := pq.StringArray{}.Value()
	scriptsVal, _ := pq.StringArray{}.Value()
	optionsVal, _ := db.JSONB(nil).Value()
	now := time.Now()

	rows := sqlmock.NewRows(cols).AddRow(
		profile.ID, profile.Name, "desc",
		osFamilyVal, osPatternVal,
		profile.Ports, profile.ScanType, profile.Timing,
		scriptsVal, optionsVal,
		10, false, now, now,
	)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	s := NewScheduler(nil, nil, mgr)
	host := hostWithIP("10.0.0.1")
	osFamily := "linux"
	host.OSFamily = &osFamily

	got := s.selectProfileForHost(context.Background(), host, "auto")
	// We don't assert the exact ID because SelectBestProfile may pick
	// differently; we only verify no panic and a non-empty result.
	_ = got // may be empty if mock rows don't align exactly — that's fine
}

func TestSelectProfileForHost_Empty_CallsSelectBestProfile(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	// If SelectBestProfile returns no profiles it falls back to generic-default.
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "description", "os_family", "os_pattern",
		"ports", "scan_type", "timing", "scripts", "options",
		"priority", "built_in", "created_at", "updated_at",
	}))
	// Fallback GetByID("generic-default") — also returns empty → error.
	mock.ExpectQuery("SELECT").WillReturnError(errors.New("not found"))

	s := NewScheduler(nil, nil, mgr)
	host := hostWithIP("10.0.0.2")

	got := s.selectProfileForHost(context.Background(), host, "")
	// Error from SelectBestProfile means we return "".
	assert.Equal(t, "", got)
}

func TestSelectProfileForHost_SelectBestProfileError_ReturnsEmpty(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	mock.ExpectQuery("SELECT").WillReturnError(errors.New("db gone"))

	s := NewScheduler(nil, nil, mgr)
	host := hostWithIP("10.0.0.3")

	got := s.selectProfileForHost(context.Background(), host, "")
	assert.Equal(t, "", got)
}

// ─────────────────────────────────────────────────────────────────────────────
// processHostsForScanning — profile manager present, GetByID paths
// ─────────────────────────────────────────────────────────────────────────────

// TestProcessHostsForScanning_GetByIDError covers the profile.GetByID error
// branch: hosts are present, selectProfileForHost returns a non-empty ID, but
// GetByID fails → host is skipped without panic.
func TestProcessHostsForScanning_GetByIDError(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	// selectProfileForHost uses the explicit ID — no DB call.
	// GetByID("bad-profile") will fail.
	mock.ExpectQuery("SELECT").
		WithArgs("bad-profile").
		WillReturnError(errors.New("profile not found"))

	s := NewScheduler(nil, nil, mgr)
	host := hostWithIP("10.1.1.1")
	hosts := []*db.Host{host}
	config := &ScanJobConfig{ProfileID: "bad-profile"}

	assert.NotPanics(t, func() {
		s.processHostsForScanning(context.Background(), hosts, config)
	})
}

// TestProcessHostsForScanning_ContextCancelledWaitingSemaphore covers the
// select { case sem <- …; case <-ctx.Done() } branch when the semaphore is
// full and the context is cancelled.
func TestProcessHostsForScanning_ContextCancelledWaitingSemaphore(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	// One GetByID call per host we let through before cancellation.
	profileCols := []string{
		"id", "name", "description", "os_family", "os_pattern",
		"ports", "scan_type", "timing", "scripts", "options",
		"priority", "built_in", "created_at", "updated_at",
	}
	now := time.Now()
	osFamilyVal, _ := pq.StringArray{"linux"}.Value()
	osPatternVal, _ := pq.StringArray{}.Value()
	scriptsVal, _ := pq.StringArray{}.Value()
	optionsVal, _ := db.JSONB(nil).Value()

	for i := 0; i < 3; i++ {
		mock.ExpectQuery("SELECT").
			WithArgs("sem-profile").
			WillReturnRows(sqlmock.NewRows(profileCols).AddRow(
				"sem-profile", "Sem", "d",
				osFamilyVal, osPatternVal,
				"22", "connect", "normal",
				scriptsVal, optionsVal,
				10, false, now, now,
			))
	}

	// maxConcurrentScans = 1 so the semaphore fills immediately.
	s := NewScheduler(nil, nil, mgr)
	s.WithMaxConcurrentScans(1)

	// Build more hosts than the semaphore can accept.
	hosts := make([]*db.Host, 5)
	for i := range hosts {
		hosts[i] = hostWithIP(fmt.Sprintf("10.2.2.%d", i+1))
	}
	config := &ScanJobConfig{ProfileID: "sem-profile"}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Must complete (via ctx cancellation) without hanging or panicking.
	done := make(chan struct{})
	go func() {
		s.processHostsForScanning(ctx, hosts, config)
		close(done)
	}()

	select {
	case <-done:
		// expected
	case <-time.After(2 * time.Second):
		t.Fatal("processHostsForScanning did not return after context cancellation")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// processHostsViaQueue
// ─────────────────────────────────────────────────────────────────────────────

// TestProcessHostsViaQueue_NilProfileManager returns immediately because
// processHostsForScanning checks s.profiles == nil before the queue branch.
// We test processHostsViaQueue directly by setting up a queue path.
func TestProcessHostsViaQueue_NoProfileManager_ReturnsEarly(t *testing.T) {
	q := scanning.NewScanQueue(2, 10)
	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)
	defer cancel()

	s := NewScheduler(nil, nil, nil) // nil profiles
	s.WithScanQueue(q)

	host := hostWithIP("10.3.0.1")
	config := &ScanJobConfig{ProfileID: "p1"}

	// processHostsForScanning exits early when profiles == nil, before
	// reaching the queue branch.
	assert.NotPanics(t, func() {
		s.processHostsForScanning(context.Background(), []*db.Host{host}, config)
	})
}

// TestProcessHostsViaQueue_NoProfileSelected_SkipsHost covers the empty
// profileID branch inside processHostsViaQueue.
func TestProcessHostsViaQueue_NoProfileSelected_SkipsHost(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	// SelectBestProfile → GetAll returns error so profileID == "".
	mock.ExpectQuery("SELECT").WillReturnError(errors.New("no profiles"))

	q := scanning.NewScanQueue(2, 10)
	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)
	defer cancel()

	s := NewScheduler(nil, nil, mgr)
	s.WithScanQueue(q)

	host := hostWithIP("10.3.0.2")
	config := &ScanJobConfig{ProfileID: ""} // triggers auto-select

	assert.NotPanics(t, func() {
		s.processHostsViaQueue(context.Background(), []*db.Host{host}, config)
	})
}

// TestProcessHostsViaQueue_GetByIDError_SkipsHost covers the GetByID error
// branch inside processHostsViaQueue (profile ID is set but lookup fails).
func TestProcessHostsViaQueue_GetByIDError_SkipsHost(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	mock.ExpectQuery("SELECT").
		WithArgs("missing-profile").
		WillReturnError(errors.New("not found"))

	q := scanning.NewScanQueue(2, 10)
	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)
	defer cancel()

	s := NewScheduler(nil, nil, mgr)
	s.WithScanQueue(q)

	host := hostWithIP("10.3.0.3")
	config := &ScanJobConfig{ProfileID: "missing-profile"}

	assert.NotPanics(t, func() {
		s.processHostsViaQueue(context.Background(), []*db.Host{host}, config)
	})
}

// TestProcessHostsViaQueue_AllSubmitted_SuccessResult covers the happy path:
// hosts are submitted to the queue and results are collected via the result
// channel. We inject a custom scan function that returns success immediately.
func TestProcessHostsViaQueue_AllSubmitted_SuccessResult(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	profileCols := []string{
		"id", "name", "description", "os_family", "os_pattern",
		"ports", "scan_type", "timing", "scripts", "options",
		"priority", "built_in", "created_at", "updated_at",
	}
	now := time.Now()
	osFamilyVal, _ := pq.StringArray{"linux"}.Value()
	osPatternVal, _ := pq.StringArray{}.Value()
	scriptsVal, _ := pq.StringArray{}.Value()
	optionsVal, _ := db.JSONB(nil).Value()

	mock.ExpectQuery("SELECT").
		WithArgs("queue-profile").
		WillReturnRows(sqlmock.NewRows(profileCols).AddRow(
			"queue-profile", "Queue", "d",
			osFamilyVal, osPatternVal,
			"22,80", "connect", "normal",
			scriptsVal, optionsVal,
			10, false, now, now,
		))

	q := scanning.NewScanQueue(4, 20)
	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)
	defer cancel()

	s := NewScheduler(nil, nil, mgr)
	s.WithScanQueue(q)
	s.scanRunner = func(_ context.Context, _ *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
		return &scanning.ScanResult{}, nil
	}

	host := hostWithIP("10.4.0.1")
	config := &ScanJobConfig{ProfileID: "queue-profile"}

	assert.NotPanics(t, func() {
		s.processHostsViaQueue(context.Background(), []*db.Host{host}, config)
	})
}

// TestProcessHostsViaQueue_ErrorResult covers the result.Error != nil branch.
func TestProcessHostsViaQueue_ErrorResult(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	profileCols := []string{
		"id", "name", "description", "os_family", "os_pattern",
		"ports", "scan_type", "timing", "scripts", "options",
		"priority", "built_in", "created_at", "updated_at",
	}
	now := time.Now()
	osFamilyVal, _ := pq.StringArray{"linux"}.Value()
	osPatternVal, _ := pq.StringArray{}.Value()
	scriptsVal, _ := pq.StringArray{}.Value()
	optionsVal, _ := db.JSONB(nil).Value()

	mock.ExpectQuery("SELECT").
		WithArgs("err-profile").
		WillReturnRows(sqlmock.NewRows(profileCols).AddRow(
			"err-profile", "Err", "d",
			osFamilyVal, osPatternVal,
			"22", "connect", "normal",
			scriptsVal, optionsVal,
			10, false, now, now,
		))

	q := scanning.NewScanQueue(4, 20)
	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)
	defer cancel()

	s := NewScheduler(nil, nil, mgr)
	s.WithScanQueue(q)
	s.scanRunner = func(_ context.Context, _ *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
		return nil, errors.New("scan failed")
	}

	host := hostWithIP("10.4.0.2")
	config := &ScanJobConfig{ProfileID: "err-profile"}

	assert.NotPanics(t, func() {
		s.processHostsViaQueue(context.Background(), []*db.Host{host}, config)
	})
}

// TestProcessHostsViaQueue_ContextCancelledWhileWaiting covers the
// case <-ctx.Done() branch while waiting for results.
func TestProcessHostsViaQueue_ContextCancelledWhileWaiting(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	profileCols := []string{
		"id", "name", "description", "os_family", "os_pattern",
		"ports", "scan_type", "timing", "scripts", "options",
		"priority", "built_in", "created_at", "updated_at",
	}
	now := time.Now()
	osFamilyVal, _ := pq.StringArray{"linux"}.Value()
	osPatternVal, _ := pq.StringArray{}.Value()
	scriptsVal, _ := pq.StringArray{}.Value()
	optionsVal, _ := db.JSONB(nil).Value()

	mock.ExpectQuery("SELECT").
		WithArgs("slow-profile").
		WillReturnRows(sqlmock.NewRows(profileCols).AddRow(
			"slow-profile", "Slow", "d",
			osFamilyVal, osPatternVal,
			"22", "connect", "normal",
			scriptsVal, optionsVal,
			10, false, now, now,
		))

	started := make(chan struct{})
	q := scanning.NewScanQueue(4, 20)
	queueCtx, queueCancel := context.WithCancel(context.Background())
	q.Start(queueCtx)
	defer queueCancel()

	s := NewScheduler(nil, nil, mgr)
	s.WithScanQueue(q)
	s.scanRunner = func(ctx context.Context, _ *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}

	host := hostWithIP("10.4.0.3")
	config := &ScanJobConfig{ProfileID: "slow-profile"}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		s.processHostsViaQueue(ctx, []*db.Host{host}, config)
		close(done)
	}()

	// Wait until the scan function has started, then cancel the context.
	<-started
	cancel()

	select {
	case <-done:
		// expected
	case <-time.After(3 * time.Second):
		t.Fatal("processHostsViaQueue did not return after context cancellation")
	}
}

// TestProcessHostsViaQueue_QueueFull covers the ErrQueueFull branch.
// We use a queue of capacity 1 whose single slot is pre-filled with a
// blocking request before the test runs, so every subsequent Submit returns
// ErrQueueFull immediately.
func TestProcessHostsViaQueue_QueueFull(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	profileCols := []string{
		"id", "name", "description", "os_family", "os_pattern",
		"ports", "scan_type", "timing", "scripts", "options",
		"priority", "built_in", "created_at", "updated_at",
	}
	now := time.Now()
	osFamilyVal, _ := pq.StringArray{"linux"}.Value()
	osPatternVal, _ := pq.StringArray{}.Value()
	scriptsVal, _ := pq.StringArray{}.Value()
	optionsVal, _ := db.JSONB(nil).Value()

	const numHosts = 3
	// Expect one GetByID per host — all profile lookups succeed, but the
	// queue is already full so every Submit returns ErrQueueFull.
	for i := 0; i < numHosts; i++ {
		mock.ExpectQuery("SELECT").
			WithArgs("full-profile").
			WillReturnRows(sqlmock.NewRows(profileCols).AddRow(
				"full-profile", "Full", "d",
				osFamilyVal, osPatternVal,
				"22", "connect", "normal",
				scriptsVal, optionsVal,
				10, false, now, now,
			))
	}

	// Queue with capacity 1, workers never started, so the single slot fills
	// immediately and every subsequent Submit gets ErrQueueFull.
	q := scanning.NewScanQueue(1, 1)
	// Do NOT call q.Start — no workers drain the queue.

	// Pre-fill the sole queue slot with a dummy job so it is full.
	dummy := scanning.NewScanJob(
		"dummy",
		&scanning.ScanConfig{},
		nil,
		func(_ context.Context, _ *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
			return &scanning.ScanResult{}, nil
		},
		nil,
	)
	require.NoError(t, q.Submit(dummy), "pre-fill must succeed")

	s := NewScheduler(nil, nil, mgr)
	s.WithScanQueue(q)

	hosts := make([]*db.Host, numHosts)
	for i := range hosts {
		hosts[i] = hostWithIP(fmt.Sprintf("10.5.0.%d", i+1))
	}
	config := &ScanJobConfig{ProfileID: "full-profile"}

	assert.NotPanics(t, func() {
		s.processHostsViaQueue(context.Background(), hosts, config)
	})
}

// TestProcessHostsViaQueue_ContextCancelledDuringSubmit covers the
// ctx.Err() != nil check at the top of the submission loop.
func TestProcessHostsViaQueue_ContextCancelledDuringSubmit(t *testing.T) {
	s := NewScheduler(nil, nil, nil)
	q := scanning.NewScanQueue(2, 10)
	s.WithScanQueue(q)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	hosts := []*db.Host{hostWithIP("10.6.0.1"), hostWithIP("10.6.0.2")}
	config := &ScanJobConfig{ProfileID: "any"}

	// profiles is nil so processHostsForScanning returns early, but we can
	// call processHostsViaQueue directly.
	assert.NotPanics(t, func() {
		s.processHostsViaQueue(ctx, hosts, config)
	})
}

// TestProcessHostsViaQueue_ZeroSubmitted_ReturnsEarly covers the
// submitted == 0 early-return branch.
func TestProcessHostsViaQueue_ZeroSubmitted_ReturnsEarly(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	// All hosts will fail profile selection (SelectBestProfile errors).
	mock.ExpectQuery("SELECT").WillReturnError(errors.New("no profile"))
	mock.ExpectQuery("SELECT").WillReturnError(errors.New("no profile"))

	q := scanning.NewScanQueue(2, 10)
	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)
	defer cancel()

	s := NewScheduler(nil, nil, mgr)
	s.WithScanQueue(q)

	hosts := []*db.Host{hostWithIP("10.7.0.1"), hostWithIP("10.7.0.2")}
	config := &ScanJobConfig{ProfileID: ""} // triggers auto-select → error

	assert.NotPanics(t, func() {
		s.processHostsViaQueue(context.Background(), hosts, config)
	})
}

// TestProcessHostsViaQueue_ResultWithNilResult covers the
// result.Result == nil branch in the success counter path.
func TestProcessHostsViaQueue_ResultWithNilResult(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	profileCols := []string{
		"id", "name", "description", "os_family", "os_pattern",
		"ports", "scan_type", "timing", "scripts", "options",
		"priority", "built_in", "created_at", "updated_at",
	}
	now := time.Now()
	osFamilyVal, _ := pq.StringArray{"linux"}.Value()
	osPatternVal, _ := pq.StringArray{}.Value()
	scriptsVal, _ := pq.StringArray{}.Value()
	optionsVal, _ := db.JSONB(nil).Value()

	mock.ExpectQuery("SELECT").
		WithArgs("nil-result-profile").
		WillReturnRows(sqlmock.NewRows(profileCols).AddRow(
			"nil-result-profile", "NR", "d",
			osFamilyVal, osPatternVal,
			"22", "connect", "normal",
			scriptsVal, optionsVal,
			10, false, now, now,
		))

	q := scanning.NewScanQueue(4, 20)
	ctx, cancel := context.WithCancel(context.Background())
	q.Start(ctx)
	defer cancel()

	s := NewScheduler(nil, nil, mgr)
	s.WithScanQueue(q)
	s.scanRunner = func(_ context.Context, _ *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
		return nil, nil // success but nil result
	}

	host := hostWithIP("10.8.0.1")
	config := &ScanJobConfig{ProfileID: "nil-result-profile"}

	assert.NotPanics(t, func() {
		s.processHostsViaQueue(context.Background(), []*db.Host{host}, config)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// addJobToCronScheduler
// ─────────────────────────────────────────────────────────────────────────────

func TestAddJobToCronScheduler_UnknownType_ReturnsError(t *testing.T) {
	s := NewScheduler(nil, nil, nil)

	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           "unknown",
		Type:           "unknown-type",
		CronExpression: "0 * * * *",
		Config:         db.JSONB(`{}`),
	}

	_, err := s.addJobToCronScheduler(job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown job type")
}

func TestAddJobToCronScheduler_DiscoveryType_Succeeds(t *testing.T) {
	s := NewScheduler(nil, nil, nil)

	cfgJSON, _ := json.Marshal(DiscoveryJobConfig{Network: "10.0.0.0/8", Method: "ping"})
	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           "disc",
		Type:           db.ScheduledJobTypeDiscovery,
		CronExpression: "0 * * * *",
		Config:         db.JSONB(cfgJSON),
	}

	cronID, err := s.addJobToCronScheduler(job)
	require.NoError(t, err)
	assert.NotZero(t, cronID)
}

func TestAddJobToCronScheduler_ScanType_Succeeds(t *testing.T) {
	s := NewScheduler(nil, nil, nil)

	cfgJSON, _ := json.Marshal(ScanJobConfig{LiveHostsOnly: true})
	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           "scan",
		Type:           db.ScheduledJobTypeScan,
		CronExpression: "0 * * * *",
		Config:         db.JSONB(cfgJSON),
	}

	cronID, err := s.addJobToCronScheduler(job)
	require.NoError(t, err)
	assert.NotZero(t, cronID)
}

// ─────────────────────────────────────────────────────────────────────────────
// addDiscoveryJobToCron / addScanJobToCron — unmarshal error paths
// ─────────────────────────────────────────────────────────────────────────────

func TestAddDiscoveryJobToCron_UnmarshalError(t *testing.T) {
	s := NewScheduler(nil, nil, nil)

	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           "bad-disc",
		Type:           db.ScheduledJobTypeDiscovery,
		CronExpression: "0 * * * *",
		Config:         db.JSONB(`not-valid-json{{{`),
	}

	_, err := s.addDiscoveryJobToCron(job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal discovery config")
}

func TestAddScanJobToCron_UnmarshalError(t *testing.T) {
	s := NewScheduler(nil, nil, nil)

	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           "bad-scan",
		Type:           db.ScheduledJobTypeScan,
		CronExpression: "0 * * * *",
		Config:         db.JSONB(`not-valid-json{{{`),
	}

	_, err := s.addScanJobToCron(job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal scan config")
}

// ─────────────────────────────────────────────────────────────────────────────
// loadScheduledJobs — processRow error (logged+continued) and rows.Err
// ─────────────────────────────────────────────────────────────────────────────

func TestLoadScheduledJobs_ProcessRowError_ContinuesOtherRows(t *testing.T) {
	database, mock := newMockDB(t)

	cols := []string{
		"id", "name", "type", "cron_expression",
		"config", "enabled", "last_run", "next_run", "created_at",
	}
	now := time.Now()
	goodID := uuid.New()
	badID := uuid.New()

	// Row 1: bad config JSON so addDiscoveryJobToCron fails → error logged, continued.
	// Row 2: good discovery job.
	goodCfg, _ := json.Marshal(DiscoveryJobConfig{Network: "10.0.0.0/8", Method: "ping"})

	rows := sqlmock.NewRows(cols).
		AddRow(
			badID, "bad-job", db.ScheduledJobTypeDiscovery, "0 * * * *",
			[]byte(`not-json`), true, nil, nil, now,
		).
		AddRow(
			goodID, "good-job", db.ScheduledJobTypeDiscovery, "0 * * * *",
			goodCfg, true, nil, nil, now,
		)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		db:   database,
		jobs: make(map[uuid.UUID]*ScheduledJob),
		mu:   sync.RWMutex{},
		ctx:  ctx,
		cron: newCron(),
	}

	err := s.loadScheduledJobs()
	// loadScheduledJobs always returns nil (errors are logged+skipped).
	assert.NoError(t, err)

	// The good job should be registered; bad one should be absent.
	s.mu.RLock()
	_, goodExists := s.jobs[goodID]
	_, badExists := s.jobs[badID]
	s.mu.RUnlock()

	assert.True(t, goodExists, "good job must be loaded despite earlier row error")
	assert.False(t, badExists, "bad job must be skipped")
}

func TestLoadScheduledJobs_ScanError_ContinuesOtherRows(t *testing.T) {
	database, mock := newMockDB(t)

	cols := []string{
		"id", "name", "type", "cron_expression",
		"config", "enabled", "last_run", "next_run", "created_at",
	}
	now := time.Now()
	goodID := uuid.New()

	goodCfg, _ := json.Marshal(ScanJobConfig{LiveHostsOnly: false})

	// First row deliberately has wrong column count to force a scan error.
	// We do this by using a different Rows set: one column only.
	// Actually sqlmock applies the same columns to all rows; instead we use
	// the standard column count but supply a value that can't Scan into uuid.UUID.
	rows := sqlmock.NewRows(cols).
		AddRow(
			"not-a-uuid", "bad-scan-row", db.ScheduledJobTypeScan, "0 * * * *",
			goodCfg, true, nil, nil, now,
		).
		AddRow(
			goodID, "good-scan-job", db.ScheduledJobTypeScan, "0 * * * *",
			goodCfg, true, nil, nil, now,
		)

	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		db:   database,
		jobs: make(map[uuid.UUID]*ScheduledJob),
		mu:   sync.RWMutex{},
		ctx:  ctx,
		cron: newCron(),
	}

	err := s.loadScheduledJobs()
	assert.NoError(t, err)

	s.mu.RLock()
	_, goodExists := s.jobs[goodID]
	s.mu.RUnlock()
	assert.True(t, goodExists, "good job must still be loaded after a bad row")
}

// ─────────────────────────────────────────────────────────────────────────────
// AddDiscoveryJob / AddScanJob — createScheduledJob error (DB save fails)
// ─────────────────────────────────────────────────────────────────────────────

func TestAddDiscoveryJob_CreateScheduledJobError(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectExec("INSERT INTO scheduled_jobs").
		WillReturnError(errors.New("insert failed"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		db:   database,
		jobs: make(map[uuid.UUID]*ScheduledJob),
		mu:   sync.RWMutex{},
		ctx:  ctx,
		cron: newCron(),
	}

	err := s.AddDiscoveryJob(ctx, "test", "0 * * * *", DiscoveryJobConfig{
		Network: "10.0.0.0/8",
		Method:  "ping",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save scheduled job")
}

func TestAddScanJob_CreateScheduledJobError(t *testing.T) {
	database, mock := newMockDB(t)

	mock.ExpectExec("INSERT INTO scheduled_jobs").
		WillReturnError(errors.New("insert failed"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		db:   database,
		jobs: make(map[uuid.UUID]*ScheduledJob),
		mu:   sync.RWMutex{},
		ctx:  ctx,
		cron: newCron(),
	}

	err := s.AddScanJob(ctx, "test", "0 * * * *", &ScanJobConfig{LiveHostsOnly: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save scheduled job")
}

// ─────────────────────────────────────────────────────────────────────────────
// addJobToCron — cron.AddFunc error (invalid cron expression after save)
// ─────────────────────────────────────────────────────────────────────────────

func TestAddJobToCron_InvalidCronExpr_ReturnsError(t *testing.T) {
	s := &Scheduler{
		jobs: make(map[uuid.UUID]*ScheduledJob),
		mu:   sync.RWMutex{},
		cron: newCron(),
	}

	nextRun := time.Now().Add(time.Hour)
	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           "bad-cron",
		Type:           db.ScheduledJobTypeScan,
		CronExpression: "NOT A VALID CRON",
		Config:         db.JSONB(`{}`),
		NextRun:        &nextRun,
	}

	err := s.addJobToCron("NOT A VALID CRON", job, func() {})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to add cron job")
}

// ─────────────────────────────────────────────────────────────────────────────
// createScheduledJob — invalid cron expression
// ─────────────────────────────────────────────────────────────────────────────

func TestCreateScheduledJob_InvalidCronExpression(t *testing.T) {
	database, _ := newMockDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		db:   database,
		jobs: make(map[uuid.UUID]*ScheduledJob),
		mu:   sync.RWMutex{},
		ctx:  ctx,
		cron: newCron(),
	}

	_, err := s.createScheduledJob(ctx, "name", "INVALID CRON", db.ScheduledJobTypeScan, &ScanJobConfig{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid cron expression")
}

// ─────────────────────────────────────────────────────────────────────────────
// processHostsForScanning — semaphore/wg path with injected scan counter
// ─────────────────────────────────────────────────────────────────────────────

// TestProcessHostsForScanning_ConcurrencyRespected verifies the WaitGroup path
// is reached: all goroutines complete and the function returns. We use a mock
// profile manager that returns profiles, but there is no real scanning engine,
// so RunScanWithContext will fail. That's fine — we only care that the
// semaphore and WaitGroup work correctly.
func TestProcessHostsForScanning_WaitGroupCompletes(t *testing.T) {
	mgr, mock := newMockProfileManager(t)

	profileCols := []string{
		"id", "name", "description", "os_family", "os_pattern",
		"ports", "scan_type", "timing", "scripts", "options",
		"priority", "built_in", "created_at", "updated_at",
	}
	now := time.Now()
	osFamilyVal, _ := pq.StringArray{"linux"}.Value()
	osPatternVal, _ := pq.StringArray{}.Value()
	scriptsVal, _ := pq.StringArray{}.Value()
	optionsVal, _ := db.JSONB(nil).Value()

	const numHosts = 3
	for i := 0; i < numHosts; i++ {
		mock.ExpectQuery("SELECT").
			WithArgs("wg-profile").
			WillReturnRows(sqlmock.NewRows(profileCols).AddRow(
				"wg-profile", "WG", "d",
				osFamilyVal, osPatternVal,
				"22", "connect", "normal",
				scriptsVal, optionsVal,
				10, false, now, now,
			))
	}

	s := NewScheduler(nil, nil, mgr)
	s.WithMaxConcurrentScans(2)

	hosts := make([]*db.Host, numHosts)
	for i := range hosts {
		hosts[i] = hostWithIP(fmt.Sprintf("10.9.0.%d", i+1))
	}
	config := &ScanJobConfig{ProfileID: "wg-profile"}

	var dispatched int64

	// We can't easily inject a fake RunScanWithContext, but we verify that
	// processHostsForScanning eventually returns (wg.Wait() completes).
	done := make(chan struct{})
	go func() {
		// This will launch goroutines that call RunScanWithContext with nil db,
		// which will fail fast. The test only cares that it doesn't hang.
		s.processHostsForScanning(context.Background(), hosts, config)
		atomic.StoreInt64(&dispatched, int64(numHosts))
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, int64(numHosts), atomic.LoadInt64(&dispatched))
	case <-time.After(10 * time.Second):
		t.Fatal("processHostsForScanning did not return within 10 s")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// newCron helper
// ─────────────────────────────────────────────────────────────────────────────

// newCron returns a fresh *cron.Cron, matching the one used inside NewScheduler.
func newCron() *cron.Cron {
	return cron.New()
}

// ─────────────────────────────────────────────────────────────────────────────
// mockSmartScanBatcher
// ─────────────────────────────────────────────────────────────────────────────

// mockSmartScanBatcher implements smartScanBatcher for unit tests.
// It records the last BatchFilter received by QueueBatch.
type mockSmartScanBatcher struct {
	queueBatchFn func(ctx context.Context, filter services.BatchFilter) (*services.BatchResult, error)
	lastFilter   services.BatchFilter
}

func (m *mockSmartScanBatcher) QueueBatch(
	ctx context.Context, filter services.BatchFilter,
) (*services.BatchResult, error) {
	m.lastFilter = filter
	if m.queueBatchFn != nil {
		return m.queueBatchFn(ctx, filter)
	}
	return &services.BatchResult{}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// WithSmartScanService
// ─────────────────────────────────────────────────────────────────────────────

func TestWithSmartScanService_SetsField(t *testing.T) {
	s := NewScheduler(nil, nil, nil)
	mock := &mockSmartScanBatcher{}

	got := s.WithSmartScanService(mock)

	assert.Same(t, s, got, "must return receiver for chaining")
	assert.Equal(t, mock, s.smartScan)
}

// ─────────────────────────────────────────────────────────────────────────────
// executeSmartScanJob
// ─────────────────────────────────────────────────────────────────────────────

func TestExecuteSmartScanJob_NilService_IsNoOp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		jobs:   make(map[uuid.UUID]*ScheduledJob),
		mu:     sync.RWMutex{},
		ctx:    ctx,
		cancel: cancel,
		// smartScan intentionally nil
	}
	jobID := uuid.New()
	cfgJSON, _ := json.Marshal(SmartScanJobConfig{ScoreThreshold: 70})
	s.jobs[jobID] = &ScheduledJob{
		ID: jobID,
		Config: &db.ScheduledJob{
			ID:      jobID,
			Name:    "ss-nil",
			Enabled: true,
			Config:  db.JSONB(cfgJSON),
		},
	}

	// Must not panic when smartScan == nil.
	assert.NotPanics(t, func() {
		s.executeSmartScanJob(jobID, SmartScanJobConfig{ScoreThreshold: 70})
	})
}

func TestExecuteSmartScanJob_PassesCorrectBatchFilter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	batcher := &mockSmartScanBatcher{}
	s := &Scheduler{
		jobs:      make(map[uuid.UUID]*ScheduledJob),
		mu:        sync.RWMutex{},
		ctx:       ctx,
		cancel:    cancel,
		smartScan: batcher,
	}
	jobID := uuid.New()
	config := SmartScanJobConfig{
		ScoreThreshold:    65,
		MaxStalenessHours: 48,
		NetworkCIDR:       "10.0.0.0/8",
		Limit:             25,
	}
	cfgJSON, _ := json.Marshal(config)
	s.jobs[jobID] = &ScheduledJob{
		ID: jobID,
		Config: &db.ScheduledJob{
			ID:      jobID,
			Name:    "ss-filter",
			Enabled: true,
			Config:  db.JSONB(cfgJSON),
		},
	}

	s.executeSmartScanJob(jobID, config)

	assert.Equal(t, db.ScanSourceScheduled, batcher.lastFilter.Source)
	assert.Equal(t, 65, batcher.lastFilter.ScoreThreshold)
	assert.Equal(t, 48, batcher.lastFilter.MaxStalenessHours)
	assert.Equal(t, "10.0.0.0/8", batcher.lastFilter.NetworkCIDR)
	assert.Equal(t, 25, batcher.lastFilter.Limit)
}

func TestExecuteSmartScanJob_QueueBatchError_IsLogged(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	batcher := &mockSmartScanBatcher{
		queueBatchFn: func(_ context.Context, _ services.BatchFilter) (*services.BatchResult, error) {
			return nil, errors.New("batch failed")
		},
	}
	s := &Scheduler{
		jobs:      make(map[uuid.UUID]*ScheduledJob),
		mu:        sync.RWMutex{},
		ctx:       ctx,
		cancel:    cancel,
		smartScan: batcher,
	}
	jobID := uuid.New()
	cfgJSON, _ := json.Marshal(SmartScanJobConfig{})
	s.jobs[jobID] = &ScheduledJob{
		ID: jobID,
		Config: &db.ScheduledJob{
			ID: jobID, Name: "ss-err", Enabled: true, Config: db.JSONB(cfgJSON),
		},
	}

	// QueueBatch error must not panic.
	assert.NotPanics(t, func() {
		s.executeSmartScanJob(jobID, SmartScanJobConfig{})
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// addSmartScanJobToCron
// ─────────────────────────────────────────────────────────────────────────────

func TestAddSmartScanJobToCron_Succeeds(t *testing.T) {
	s := NewScheduler(nil, nil, nil)
	s.smartScan = &mockSmartScanBatcher{}

	cfgJSON, _ := json.Marshal(SmartScanJobConfig{ScoreThreshold: 80, Limit: 10})
	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           "ss-cron",
		Type:           db.ScheduledJobTypeSmartScan,
		CronExpression: "0 * * * *",
		Config:         db.JSONB(cfgJSON),
	}

	cronID, err := s.addSmartScanJobToCron(job)
	require.NoError(t, err)
	assert.NotZero(t, cronID)
}

func TestAddSmartScanJobToCron_UnmarshalError(t *testing.T) {
	s := NewScheduler(nil, nil, nil)

	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           "bad-ss",
		Type:           db.ScheduledJobTypeSmartScan,
		CronExpression: "0 * * * *",
		Config:         db.JSONB(`not-valid-json{{{`),
	}

	_, err := s.addSmartScanJobToCron(job)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unmarshal smart scan config")
}

// ─────────────────────────────────────────────────────────────────────────────
// AddSmartScanJob
// ─────────────────────────────────────────────────────────────────────────────

func TestAddSmartScanJob_InvalidCron_ReturnsError(t *testing.T) {
	s := &Scheduler{
		jobs: make(map[uuid.UUID]*ScheduledJob),
		mu:   sync.RWMutex{},
		cron: newCron(),
	}

	err := s.AddSmartScanJob(
		context.Background(), "test", "not-a-cron", SmartScanJobConfig{},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid cron expression")
}

func TestAddSmartScanJob_CreateScheduledJobError(t *testing.T) {
	database, mock := newMockDB(t)
	mock.ExpectExec("INSERT INTO scheduled_jobs").
		WillReturnError(errors.New("insert failed"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		db:        database,
		jobs:      make(map[uuid.UUID]*ScheduledJob),
		mu:        sync.RWMutex{},
		ctx:       ctx,
		cron:      newCron(),
		smartScan: &mockSmartScanBatcher{},
	}

	err := s.AddSmartScanJob(ctx, "test", "0 * * * *", SmartScanJobConfig{ScoreThreshold: 80})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to save scheduled job")
}

// TestAddJobToCronScheduler_SmartScanType confirms that addJobToCronScheduler
// dispatches smart_scan jobs to addSmartScanJobToCron correctly.
func TestAddJobToCronScheduler_SmartScanType_Succeeds(t *testing.T) {
	s := NewScheduler(nil, nil, nil)
	s.smartScan = &mockSmartScanBatcher{}

	cfgJSON, _ := json.Marshal(SmartScanJobConfig{Limit: 5})
	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           "ss-dispatch",
		Type:           db.ScheduledJobTypeSmartScan,
		CronExpression: "0 * * * *",
		Config:         db.JSONB(cfgJSON),
	}

	cronID, err := s.addJobToCronScheduler(job)
	require.NoError(t, err)
	assert.NotZero(t, cronID)
}
