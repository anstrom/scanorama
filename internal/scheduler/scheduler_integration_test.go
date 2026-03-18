//go:build integration

// Package scheduler – integration tests that require a live PostgreSQL database.
//
// These tests are excluded from the normal unit-test run and are picked up only
// when the build tag "integration" is set, matching the pattern used by every
// other integration test suite in this repository (internal/db, etc.).
//
// Every test obtains a real database connection via connectIntegrationDB, seeds
// the minimum data it needs, exercises the scheduler against the live schema,
// and cleans up after itself via t.Cleanup.
package scheduler

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/anstrom/scanorama/internal/profiles"
)

// ─────────────────────────────────────────────────────────────────────────────
// Database connection helper (mirrors internal/db/hosts_repository_test.go)
// ─────────────────────────────────────────────────────────────────────────────

func connectIntegrationDB(t *testing.T) *db.DB {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	cfg := &db.Config{
		Host:            schedEnvOrDefault("TEST_DB_HOST", "localhost"),
		Port:            schedEnvIntOrDefault("TEST_DB_PORT", 5433),
		Database:        schedEnvOrDefault("TEST_DB_NAME", "scanorama_test"),
		Username:        schedEnvOrDefault("TEST_DB_USER", "test_user"),
		Password:        schedEnvOrDefault("TEST_DB_PASSWORD", "test_password"),
		SSLMode:         "disable",
		MaxOpenConns:    2,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
		ConnMaxIdleTime: time.Minute,
	}

	database, err := db.ConnectAndMigrate(context.Background(), cfg)
	if err != nil {
		database, err = db.Connect(context.Background(), cfg)
		if err != nil {
			t.Skipf("skipping — could not connect to test database (%s:%d/%s): %v",
				cfg.Host, cfg.Port, cfg.Database, err)
			return nil
		}
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func schedEnvOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func schedEnvIntOrDefault(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// ─────────────────────────────────────────────────────────────────────────────
// Scheduler factory helpers
// ─────────────────────────────────────────────────────────────────────────────

// newIntegrationScheduler builds a Scheduler wired to the live database.
// Discovery engine and profile manager are also wired so execute paths work.
func newIntegrationScheduler(t *testing.T, database *db.DB) *Scheduler {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	s := &Scheduler{
		db:                 database,
		cron:               cron.New(),
		discovery:          discovery.NewEngine(database),
		profiles:           profiles.NewManager(database),
		jobs:               make(map[uuid.UUID]*ScheduledJob),
		mu:                 sync.RWMutex{},
		ctx:                ctx,
		cancel:             cancel,
		maxConcurrentScans: defaultMaxConcurrentScans,
	}
	return s
}

// insertScheduledJob directly inserts a scheduled_jobs row and registers a
// cleanup that deletes it. Returns the inserted job.
func insertScheduledJob(t *testing.T, database *db.DB, jobType string, cfg interface{}) *db.ScheduledJob {
	t.Helper()
	ctx := context.Background()

	cfgJSON, err := json.Marshal(cfg)
	require.NoError(t, err)

	nextRun := time.Now().Add(time.Hour)
	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           "integ-" + jobType + "-" + uuid.New().String()[:8],
		Type:           jobType,
		CronExpression: "0 * * * *",
		Config:         db.JSONB(cfgJSON),
		Enabled:        true,
		CreatedAt:      time.Now(),
		NextRun:        &nextRun,
	}

	_, err = database.ExecContext(ctx,
		`INSERT INTO scheduled_jobs (id, name, type, cron_expression, config, enabled, last_run, next_run, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		job.ID, job.Name, job.Type, job.CronExpression, job.Config,
		job.Enabled, job.LastRun, job.NextRun, job.CreatedAt,
	)
	require.NoError(t, err, "insertScheduledJob: INSERT failed")

	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(),
			`DELETE FROM scheduled_jobs WHERE id = $1`, job.ID)
	})

	return job
}

// insertHost inserts a minimal host row and registers cleanup. Returns the ID.
func insertHost(t *testing.T, database *db.DB, ip string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	id := uuid.New()
	_, err := database.ExecContext(ctx,
		`INSERT INTO hosts (id, ip_address, status, discovery_method, last_seen, first_seen)
		 VALUES ($1, $2::inet, $3, $4, NOW(), NOW())
		 ON CONFLICT (ip_address) DO UPDATE SET last_seen = NOW()`,
		id, ip, db.HostStatusUp, "tcp",
	)
	require.NoError(t, err, "insertHost: INSERT failed")
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(),
			`DELETE FROM hosts WHERE id = $1`, id)
	})
	return id
}

// insertProfile inserts a minimal scan_profiles row and registers cleanup.
func insertProfile(t *testing.T, database *db.DB) *db.ScanProfile {
	t.Helper()
	ctx := context.Background()
	mgr := profiles.NewManager(database)

	p := &db.ScanProfile{
		ID:          "integ-test-profile-" + uuid.New().String()[:8],
		Name:        "Integration Test Profile",
		Description: "Created by scheduler integration tests",
		OSFamily:    pq.StringArray{"linux"},
		OSPattern:   pq.StringArray{},
		Ports:       "22",
		ScanType:    db.ScanTypeConnect,
		Timing:      db.ScanTimingNormal,
		Priority:    10,
		BuiltIn:     false,
	}

	err := mgr.Create(ctx, p)
	require.NoError(t, err, "insertProfile: Create failed")

	t.Cleanup(func() {
		_ = mgr.Delete(context.Background(), p.ID)
	})
	return p
}

// ─────────────────────────────────────────────────────────────────────────────
// AddDiscoveryJob — full success path
// Covers lines 164-166: return s.addJobToCron(…) after createScheduledJob succeeds
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_AddDiscoveryJob_Success(t *testing.T) {
	database := connectIntegrationDB(t)
	s := newIntegrationScheduler(t, database)

	ctx := context.Background()
	cfg := DiscoveryJobConfig{
		Network:     "127.0.0.1/32",
		Method:      "tcp",
		Timeout:     5,
		Concurrency: 1,
	}

	err := s.AddDiscoveryJob(ctx, "integ-disc-job", "0 * * * *", cfg)
	require.NoError(t, err, "AddDiscoveryJob must succeed against the live database")

	// The job must be in memory.
	s.mu.RLock()
	var found bool
	for _, j := range s.jobs {
		if j.Config.Name == "integ-disc-job" {
			found = true
		}
	}
	s.mu.RUnlock()
	assert.True(t, found, "job must be registered in the in-memory map")

	// Clean up the row we just wrote.
	s.mu.RLock()
	var jobID uuid.UUID
	for _, j := range s.jobs {
		if j.Config.Name == "integ-disc-job" {
			jobID = j.ID
		}
	}
	s.mu.RUnlock()
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(),
			`DELETE FROM scheduled_jobs WHERE id = $1`, jobID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// AddScanJob — full success path
// Covers lines 176-178: return s.addJobToCron(…) after createScheduledJob succeeds
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_AddScanJob_Success(t *testing.T) {
	database := connectIntegrationDB(t)
	s := newIntegrationScheduler(t, database)

	ctx := context.Background()
	cfg := &ScanJobConfig{
		LiveHostsOnly: false,
		MaxAge:        24,
	}

	err := s.AddScanJob(ctx, "integ-scan-job", "0 * * * *", cfg)
	require.NoError(t, err, "AddScanJob must succeed against the live database")

	s.mu.RLock()
	var jobID uuid.UUID
	for _, j := range s.jobs {
		if j.Config.Name == "integ-scan-job" {
			jobID = j.ID
		}
	}
	s.mu.RUnlock()
	assert.NotEqual(t, uuid.Nil, jobID, "job must be registered in memory")

	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(),
			`DELETE FROM scheduled_jobs WHERE id = $1`, jobID)
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// loadJobsFromDatabase — rows loop (scan + append path)
// Covers lines 330-338: the for rows.Next() loop that scans each row
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_LoadJobsFromDatabase_ReturnsRows(t *testing.T) {
	database := connectIntegrationDB(t)
	s := newIntegrationScheduler(t, database)

	// Seed one discovery and one scan job.
	discJob := insertScheduledJob(t, database, db.ScheduledJobTypeDiscovery,
		DiscoveryJobConfig{Network: "127.0.0.1/32", Method: "tcp", Timeout: 5, Concurrency: 1})
	scanJob := insertScheduledJob(t, database, db.ScheduledJobTypeScan,
		ScanJobConfig{LiveHostsOnly: false})

	ctx := context.Background()
	jobs, err := s.loadJobsFromDatabase(ctx)
	require.NoError(t, err)

	ids := make(map[uuid.UUID]bool)
	for _, j := range jobs {
		ids[j.ID] = true
	}
	assert.True(t, ids[discJob.ID], "discovery job must appear in loadJobsFromDatabase result")
	assert.True(t, ids[scanJob.ID], "scan job must appear in loadJobsFromDatabase result")
}

// ─────────────────────────────────────────────────────────────────────────────
// loadScheduledJobs — full rows loop including addJobToCronScheduler success
// Covers lines 859-861 (deferred rows.Close), 929-931 (addDiscoveryJobToCron
// cron.AddFunc return), 941-943 (addScanJobToCron cron.AddFunc return)
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_LoadScheduledJobs_RegistersJobsInCron(t *testing.T) {
	database := connectIntegrationDB(t)
	s := newIntegrationScheduler(t, database)

	discJob := insertScheduledJob(t, database, db.ScheduledJobTypeDiscovery,
		DiscoveryJobConfig{Network: "127.0.0.1/32", Method: "tcp", Timeout: 5, Concurrency: 1})
	scanJob := insertScheduledJob(t, database, db.ScheduledJobTypeScan,
		ScanJobConfig{LiveHostsOnly: false})

	err := s.loadScheduledJobs()
	require.NoError(t, err)

	s.mu.RLock()
	_, discFound := s.jobs[discJob.ID]
	_, scanFound := s.jobs[scanJob.ID]
	s.mu.RUnlock()

	assert.True(t, discFound, "discovery job must be registered in memory by loadScheduledJobs")
	assert.True(t, scanFound, "scan job must be registered in memory by loadScheduledJobs")
}

// ─────────────────────────────────────────────────────────────────────────────
// addDiscoveryJobToCron — cron.AddFunc success path
// Covers lines 929-931
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_AddDiscoveryJobToCron_Success(t *testing.T) {
	database := connectIntegrationDB(t)
	s := newIntegrationScheduler(t, database)

	cfgJSON, _ := json.Marshal(DiscoveryJobConfig{Network: "127.0.0.1/32", Method: "tcp"})
	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           "disc-cron-integ",
		Type:           db.ScheduledJobTypeDiscovery,
		CronExpression: "0 * * * *",
		Config:         db.JSONB(cfgJSON),
	}

	cronID, err := s.addDiscoveryJobToCron(job)
	require.NoError(t, err)
	assert.NotZero(t, cronID, "cron.AddFunc must return a non-zero entry ID on success")
}

// ─────────────────────────────────────────────────────────────────────────────
// addScanJobToCron — cron.AddFunc success path
// Covers lines 941-943
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_AddScanJobToCron_Success(t *testing.T) {
	database := connectIntegrationDB(t)
	s := newIntegrationScheduler(t, database)

	cfgJSON, _ := json.Marshal(ScanJobConfig{LiveHostsOnly: false})
	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           "scan-cron-integ",
		Type:           db.ScheduledJobTypeScan,
		CronExpression: "0 * * * *",
		Config:         db.JSONB(cfgJSON),
	}

	cronID, err := s.addScanJobToCron(job)
	require.NoError(t, err)
	assert.NotZero(t, cronID)
}

// ─────────────────────────────────────────────────────────────────────────────
// executeHostScanQuery — rows loop (scan + append)
// Covers lines 825-829: for rows.Next() → scanHostRow → append
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_ExecuteHostScanQuery_ReturnsHosts(t *testing.T) {
	database := connectIntegrationDB(t)
	insertHost(t, database, "127.10.0.1")

	s := newIntegrationScheduler(t, database)
	ctx := context.Background()

	query := `
		SELECT id, ip_address, hostname, mac_address, vendor,
		       os_family, os_name, os_version, os_confidence,
		       os_detected_at, os_method, os_details, discovery_method,
		       response_time_ms, discovery_count, ignore_scanning,
		       first_seen, last_seen, status
		FROM hosts
		WHERE ip_address = '127.10.0.1'::inet
	`

	hosts, err := s.executeHostScanQuery(ctx, query, nil)
	require.NoError(t, err)
	require.NotEmpty(t, hosts, "at least the seeded host must be returned")

	var found bool
	for _, h := range hosts {
		if h.IPAddress.IP.Equal(net.ParseIP("127.10.0.1")) {
			found = true
		}
	}
	assert.True(t, found, "seeded host 127.10.0.1 must appear in query results")
}

// ─────────────────────────────────────────────────────────────────────────────
// scanHostRow — successful scan path via executeHostScanQuery
// Covers lines 837-849: the happy path through scanHostRow
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_ScanHostRow_PopulatesHost(t *testing.T) {
	database := connectIntegrationDB(t)
	insertHost(t, database, "127.10.0.2")

	s := newIntegrationScheduler(t, database)
	ctx := context.Background()

	query := `
		SELECT id, ip_address, hostname, mac_address, vendor,
		       os_family, os_name, os_version, os_confidence,
		       os_detected_at, os_method, os_details, discovery_method,
		       response_time_ms, discovery_count, ignore_scanning,
		       first_seen, last_seen, status
		FROM hosts
		WHERE ip_address = '127.10.0.2'::inet
	`

	hosts, err := s.executeHostScanQuery(ctx, query, nil)
	require.NoError(t, err)
	require.Len(t, hosts, 1)

	h := hosts[0]
	assert.Equal(t, db.HostStatusUp, h.Status)
	assert.True(t, h.IPAddress.IP.Equal(net.ParseIP("127.10.0.2")))
}

// ─────────────────────────────────────────────────────────────────────────────
// executeDiscoveryJob — error path (Discover returns error for bad network)
// Covers lines 444-448: if err != nil { log … return }
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_ExecuteDiscoveryJob_DiscoverError_UpdatesLastRun(t *testing.T) {
	database := connectIntegrationDB(t)
	s := newIntegrationScheduler(t, database)

	// Seed a scheduled_jobs row so updateJobLastRun has a real row to UPDATE.
	dbJob := insertScheduledJob(t, database, db.ScheduledJobTypeDiscovery,
		DiscoveryJobConfig{Network: "192.0.2.0/24", Method: "tcp", Timeout: 1, Concurrency: 1})

	// Register job in memory.
	nextRun := time.Now().Add(time.Hour)
	s.jobs[dbJob.ID] = &ScheduledJob{
		ID:      dbJob.ID,
		Config:  dbJob,
		NextRun: nextRun,
		Running: false,
	}

	cfg := DiscoveryJobConfig{
		// 192.0.2.0/24 is TEST-NET-1 (RFC 5737) — unreachable, so Discover
		// will either error during validation or return quickly with no hosts.
		// Either way we get the error/updateLastRun path exercised.
		Network:     "192.0.2.0/24",
		Method:      "tcp",
		Timeout:     1,
		Concurrency: 1,
	}

	// Must not panic; will either log an error or complete quickly.
	assert.NotPanics(t, func() {
		s.executeDiscoveryJob(dbJob.ID, cfg)
	})

	// Regardless of success/failure, the job must be marked not running.
	s.mu.RLock()
	running := s.jobs[dbJob.ID].Running
	s.mu.RUnlock()
	assert.False(t, running, "job must be marked not-running after execution")

	// Verify last_run was written to the database.
	var lastRun *time.Time
	err := database.QueryRowContext(context.Background(),
		`SELECT last_run FROM scheduled_jobs WHERE id = $1`, dbJob.ID).Scan(&lastRun)
	require.NoError(t, err)
	assert.NotNil(t, lastRun, "last_run must be set in the database after execution")
}

// ─────────────────────────────────────────────────────────────────────────────
// executeDiscoveryJob — success path (Discover returns a job)
// Covers line 450-451: log.Printf("Discovery job '%s' completed…", discoveryJob.ID)
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_ExecuteDiscoveryJob_Success_LogsCompletion(t *testing.T) {
	database := connectIntegrationDB(t)
	s := newIntegrationScheduler(t, database)

	dbJob := insertScheduledJob(t, database, db.ScheduledJobTypeDiscovery,
		DiscoveryJobConfig{Network: "127.0.0.1/32", Method: "tcp", Timeout: 5, Concurrency: 1})

	nextRun := time.Now().Add(time.Hour)
	s.jobs[dbJob.ID] = &ScheduledJob{
		ID:      dbJob.ID,
		Config:  dbJob,
		NextRun: nextRun,
		Running: false,
	}

	cfg := DiscoveryJobConfig{
		Network:     "127.0.0.1/32",
		Method:      "tcp",
		Timeout:     5,
		Concurrency: 1,
	}

	// Discover on 127.0.0.1/32 always finds the loopback host quickly.
	assert.NotPanics(t, func() {
		s.executeDiscoveryJob(dbJob.ID, cfg)
	})

	s.mu.RLock()
	running := s.jobs[dbJob.ID].Running
	s.mu.RUnlock()
	assert.False(t, running)
}

// ─────────────────────────────────────────────────────────────────────────────
// executeScanJob — hosts found path
// Covers lines 490-496: log "found N hosts", processHostsForScanning call,
// and "completed in" log line
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_ExecuteScanJob_WithHosts_CallsProcessHosts(t *testing.T) {
	database := connectIntegrationDB(t)

	// Insert a profile the job can use.
	profile := insertProfile(t, database)

	// Insert a host that the job query will find.
	insertHost(t, database, "127.10.0.3")

	s := newIntegrationScheduler(t, database)

	dbJob := insertScheduledJob(t, database, db.ScheduledJobTypeScan,
		ScanJobConfig{
			LiveHostsOnly: false,
			ProfileID:     profile.ID,
		})

	nextRun := time.Now().Add(time.Hour)
	cfgJSON, _ := json.Marshal(ScanJobConfig{LiveHostsOnly: false, ProfileID: profile.ID})
	s.jobs[dbJob.ID] = &ScheduledJob{
		ID: dbJob.ID,
		Config: &db.ScheduledJob{
			ID:             dbJob.ID,
			Name:           dbJob.Name,
			Type:           db.ScheduledJobTypeScan,
			CronExpression: "0 * * * *",
			Enabled:        true,
			Config:         db.JSONB(cfgJSON),
		},
		NextRun: nextRun,
		Running: false,
	}

	jobCfg := &ScanJobConfig{
		LiveHostsOnly: false,
		ProfileID:     profile.ID,
	}

	// executeScanJob will find the seeded host, call processHostsForScanning,
	// which will launch goroutines that call RunScanWithContext. Those will
	// fail (no nmap in unit mode) or succeed; either way the function must
	// return without panicking and mark the job as not-running.
	done := make(chan struct{})
	go func() {
		s.executeScanJob(dbJob.ID, jobCfg)
		close(done)
	}()

	select {
	case <-done:
		// expected
	case <-time.After(30 * time.Second):
		t.Fatal("executeScanJob did not return within 30 s")
	}

	s.mu.RLock()
	running := s.jobs[dbJob.ID].Running
	s.mu.RUnlock()
	assert.False(t, running, "job must be marked not-running after execution")
}

// ─────────────────────────────────────────────────────────────────────────────
// Start — full lifecycle with real DB (loads jobs, starts cron)
// Covers the loadScheduledJobs path inside Start and confirms running = true
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_Start_LoadsJobsAndRunsCron(t *testing.T) {
	database := connectIntegrationDB(t)

	// Seed a scan job so loadScheduledJobs has at least one row to process.
	insertScheduledJob(t, database, db.ScheduledJobTypeScan,
		ScanJobConfig{LiveHostsOnly: false})

	s := newIntegrationScheduler(t, database)

	err := s.Start()
	require.NoError(t, err)
	t.Cleanup(func() { s.Stop() })

	s.mu.RLock()
	running := s.running
	s.mu.RUnlock()
	assert.True(t, running, "scheduler must report running = true after Start")
}

// ─────────────────────────────────────────────────────────────────────────────
// GetJobs — returns rows from the live database
// Covers loadJobsFromDatabase called inside GetJobs
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_GetJobs_ReturnsSeededJobs(t *testing.T) {
	database := connectIntegrationDB(t)
	s := newIntegrationScheduler(t, database)

	inserted := insertScheduledJob(t, database, db.ScheduledJobTypeScan,
		ScanJobConfig{LiveHostsOnly: true})

	jobs := s.GetJobs()

	var found bool
	for _, j := range jobs {
		if j.ID == inserted.ID {
			found = true
		}
	}
	assert.True(t, found, "seeded job must appear in GetJobs result")
}

// ─────────────────────────────────────────────────────────────────────────────
// RemoveJob — removes from DB and memory
// ─────────────────────────────────────────────────────────────────────────────

func TestIntegration_RemoveJob_DeletesFromDB(t *testing.T) {
	database := connectIntegrationDB(t)
	s := newIntegrationScheduler(t, database)
	ctx := context.Background()

	// Add a job properly through AddScanJob so it is in memory.
	err := s.AddScanJob(ctx, "integ-remove-test", "0 * * * *", &ScanJobConfig{})
	require.NoError(t, err)

	var jobID uuid.UUID
	s.mu.RLock()
	for _, j := range s.jobs {
		if j.Config.Name == "integ-remove-test" {
			jobID = j.ID
		}
	}
	s.mu.RUnlock()
	require.NotEqual(t, uuid.Nil, jobID)

	err = s.RemoveJob(ctx, jobID)
	require.NoError(t, err)

	// Must be gone from memory.
	s.mu.RLock()
	_, stillExists := s.jobs[jobID]
	s.mu.RUnlock()
	assert.False(t, stillExists)

	// Must be gone from the database.
	var count int
	err = database.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM scheduled_jobs WHERE id = $1`, jobID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
