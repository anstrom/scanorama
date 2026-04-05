// Package scheduler_test provides tests for the scheduler package.
package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/robfig/cron/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// TestPanicRecoveryInCronWrapper tests that the wrapper added in addJobToCron
// properly recovers from panics in job functions.
func TestPanicRecoveryInCronWrapper(t *testing.T) {
	tests := []struct {
		name            string
		jobFunc         func()
		expectPanic     bool
		expectExecution bool
	}{
		{
			name: "normal job execution - no panic",
			jobFunc: func() {
				// Normal execution, no panic
			},
			expectPanic:     false,
			expectExecution: true,
		},
		{
			name: "job panics with string - should be recovered",
			jobFunc: func() {
				panic("test panic in job")
			},
			expectPanic:     false, // Should be recovered, not propagate
			expectExecution: true,
		},
		{
			name: "job panics with struct - should be recovered",
			jobFunc: func() {
				panic(struct{ msg string }{"test error"})
			},
			expectPanic:     false,
			expectExecution: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			s := &Scheduler{
				cron:    cron.New(),
				jobs:    make(map[uuid.UUID]*ScheduledJob),
				mu:      sync.RWMutex{},
				ctx:     ctx,
				cancel:  cancel,
				running: false,
			}

			// Create a test job config
			jobID := uuid.New()
			configJSON, _ := json.Marshal(map[string]interface{}{
				"network": "192.168.1.0/24",
			})
			jobConfig := &db.ScheduledJob{
				ID:      jobID,
				Name:    "test-job",
				Type:    "discovery",
				Enabled: true,
				Config:  db.JSONB(configJSON),
			}
			nextRun := time.Now().Add(time.Hour)
			jobConfig.NextRun = &nextRun

			// Track whether job executed
			executed := false
			wrappedFunc := func() {
				executed = true
				tt.jobFunc()
			}

			// Add the job to cron - this wraps it with panic recovery
			err := s.addJobToCron("@every 1h", jobConfig, wrappedFunc)
			if err != nil {
				t.Fatalf("addJobToCron() error = %v", err)
			}

			// Verify job was added
			s.mu.RLock()
			job, exists := s.jobs[jobID]
			s.mu.RUnlock()

			if !exists {
				t.Fatal("Job was not added to scheduler")
			}

			// Start the cron scheduler
			s.cron.Start()
			defer s.cron.Stop()

			// Get the cron entry and execute it directly
			entries := s.cron.Entries()
			if len(entries) == 0 {
				t.Fatal("No cron entries found")
			}

			// Execute the job - panic should be recovered
			didPanic := false
			func() {
				defer func() {
					if r := recover(); r != nil {
						didPanic = true
						t.Errorf("Panic was not recovered by wrapper: %v", r)
					}
				}()
				entries[0].Job.Run()
			}()

			// Give it a moment to complete
			time.Sleep(50 * time.Millisecond)

			// Verify execution happened
			if tt.expectExecution && !executed {
				t.Error("Job function was not executed")
			}

			// Verify no panic propagated
			if didPanic {
				t.Error("Panic propagated outside of panic recovery wrapper")
			}

			// Verify job state was cleaned up after panic
			s.mu.RLock()
			stillRunning := job.Running
			s.mu.RUnlock()

			if stillRunning {
				t.Error("Job state not cleaned up - still marked as running after execution")
			}
		})
	}
}

// TestJobStateCleanupOnPanic verifies that job state is properly cleaned up
// when a panic occurs in the cron wrapper.
func TestJobStateCleanupOnPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		cron:   cron.New(),
		jobs:   make(map[uuid.UUID]*ScheduledJob),
		mu:     sync.RWMutex{},
		ctx:    ctx,
		cancel: cancel,
	}

	jobID := uuid.New()
	configJSON, _ := json.Marshal(map[string]interface{}{
		"network": "192.168.1.0/24",
	})
	jobConfig := &db.ScheduledJob{
		ID:      jobID,
		Name:    "panic-cleanup-test",
		Type:    "discovery",
		Enabled: true,
		Config:  db.JSONB(configJSON),
	}
	nextRun := time.Now().Add(time.Hour)
	jobConfig.NextRun = &nextRun

	// Job that will panic
	panicFunc := func() {
		panic("intentional panic for cleanup test")
	}

	err := s.addJobToCron("@every 1h", jobConfig, panicFunc)
	if err != nil {
		t.Fatalf("addJobToCron() error = %v", err)
	}

	s.cron.Start()
	defer s.cron.Stop()

	// Verify initial state
	s.mu.RLock()
	job := s.jobs[jobID]
	initialRunning := job.Running
	s.mu.RUnlock()

	if initialRunning {
		t.Error("Job should not be marked as running initially")
	}

	// Execute the job to trigger panic
	entries := s.cron.Entries()
	if len(entries) > 0 {
		entries[0].Job.Run()
	}

	// Give it time to recover
	time.Sleep(100 * time.Millisecond)

	// Verify state was cleaned up
	s.mu.RLock()
	finalRunning := job.Running
	s.mu.RUnlock()

	if finalRunning {
		t.Error("Job state not cleaned up - still marked as running after panic")
	}
}

// TestMultipleJobsPanicIsolation verifies that a panic in one job
// doesn't prevent other jobs from executing.
func TestMultipleJobsPanicIsolation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		cron:   cron.New(),
		jobs:   make(map[uuid.UUID]*ScheduledJob),
		mu:     sync.RWMutex{},
		ctx:    ctx,
		cancel: cancel,
	}

	s.cron.Start()
	defer s.cron.Stop()

	// Job 1: Will panic
	jobID1 := uuid.New()
	configJSON1, _ := json.Marshal(map[string]interface{}{
		"network": "192.168.1.0/24",
	})
	jobConfig1 := &db.ScheduledJob{
		ID:      jobID1,
		Name:    "panicking-job",
		Type:    "discovery",
		Enabled: true,
		Config:  db.JSONB(configJSON1),
	}
	nextRun1 := time.Now().Add(time.Hour)
	jobConfig1.NextRun = &nextRun1

	job1Executed := false
	panicFunc := func() {
		job1Executed = true
		panic("panic in job 1")
	}

	err := s.addJobToCron("@every 1h", jobConfig1, panicFunc)
	if err != nil {
		t.Fatalf("addJobToCron() for job 1 error = %v", err)
	}

	// Job 2: Normal execution
	jobID2 := uuid.New()
	configJSON2, _ := json.Marshal(map[string]interface{}{
		"network": "192.168.2.0/24",
	})
	jobConfig2 := &db.ScheduledJob{
		ID:      jobID2,
		Name:    "normal-job",
		Type:    "discovery",
		Enabled: true,
		Config:  db.JSONB(configJSON2),
	}
	nextRun2 := time.Now().Add(time.Hour)
	jobConfig2.NextRun = &nextRun2

	job2Executed := false
	normalFunc := func() {
		job2Executed = true
	}

	err = s.addJobToCron("@every 1h", jobConfig2, normalFunc)
	if err != nil {
		t.Fatalf("addJobToCron() for job 2 error = %v", err)
	}

	// Execute both jobs
	entries := s.cron.Entries()
	if len(entries) != 2 {
		t.Fatalf("Expected 2 cron entries, got %d", len(entries))
	}

	// Run both jobs
	entries[0].Job.Run()
	time.Sleep(50 * time.Millisecond)
	entries[1].Job.Run()
	time.Sleep(50 * time.Millisecond)

	// Verify both executed
	if !job1Executed {
		t.Error("Panicking job did not execute")
	}
	if !job2Executed {
		t.Error("Normal job did not execute - panic isolation failed")
	}

	// Verify state of both jobs
	s.mu.RLock()
	job1 := s.jobs[jobID1]
	job2 := s.jobs[jobID2]
	s.mu.RUnlock()

	if job1.Running {
		t.Error("Panicking job still marked as running")
	}
	if job2.Running {
		t.Error("Normal job incorrectly marked as running")
	}
}

// TestNewScheduler verifies that NewScheduler properly initializes
// all fields.
func TestNewScheduler(t *testing.T) {
	s := NewScheduler(nil, nil, nil)

	if s == nil {
		t.Fatal("NewScheduler returned nil")
	}

	if s.cron == nil {
		t.Error("Scheduler cron is nil")
	}

	if s.jobs == nil {
		t.Error("Scheduler jobs map is nil")
	}

	if s.ctx == nil {
		t.Error("Scheduler context is nil")
	}

	if s.cancel == nil {
		t.Error("Scheduler cancel function is nil")
	}
}

// ==============================================================================
// Phase 1: Lifecycle and State Management Tests
// ==============================================================================

// TestScheduler_Stop tests stopping the scheduler
func TestScheduler_Stop(t *testing.T) {
	tests := []struct {
		name           string
		startScheduler bool
	}{
		{
			name:           "stop_running_scheduler",
			startScheduler: true,
		},
		{
			name:           "stop_already_stopped_scheduler_noop",
			startScheduler: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScheduler(nil, nil, nil)

			if tt.startScheduler {
				// Manually set running state without calling Start (which requires DB)
				s.mu.Lock()
				s.running = true
				s.cron.Start()
				s.mu.Unlock()
			}

			initialRunning := s.running

			// Execute
			s.Stop()

			// Assert
			assert.False(t, s.running, "scheduler should not be running after Stop")

			// Verify context was cancelled if it was running
			if initialRunning {
				select {
				case <-s.ctx.Done():
					// Context cancelled as expected
				case <-time.After(100 * time.Millisecond):
					t.Error("Context was not cancelled after Stop")
				}
			}
		})
	}
}

// TestScheduler_StopConcurrency tests concurrent stop operations
func TestScheduler_StopConcurrency(t *testing.T) {
	s := NewScheduler(nil, nil, nil)

	// Mark as running
	s.mu.Lock()
	s.running = true
	s.cron.Start()
	s.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Stop()
		}()
	}

	wg.Wait()

	// Should be stopped and no panic
	assert.False(t, s.running)
}

// TestScheduler_PrepareJobExecution tests the prepareJobExecution helper
func TestScheduler_PrepareJobExecution(t *testing.T) {
	tests := []struct {
		name        string
		setupJob    func(*Scheduler, uuid.UUID)
		wantExecute bool
	}{
		{
			name: "job_enabled_and_not_running",
			setupJob: func(s *Scheduler, id uuid.UUID) {
				s.jobs[id] = &ScheduledJob{
					ID: id,
					Config: &db.ScheduledJob{
						ID:      id,
						Name:    "test-job",
						Enabled: true,
					},
					Running: false,
				}
			},
			wantExecute: true,
		},
		{
			name: "job_disabled_should_skip",
			setupJob: func(s *Scheduler, id uuid.UUID) {
				s.jobs[id] = &ScheduledJob{
					ID: id,
					Config: &db.ScheduledJob{
						ID:      id,
						Name:    "test-job",
						Enabled: false,
					},
					Running: false,
				}
			},
			wantExecute: false,
		},
		{
			name: "job_already_running_should_skip",
			setupJob: func(s *Scheduler, id uuid.UUID) {
				s.jobs[id] = &ScheduledJob{
					ID: id,
					Config: &db.ScheduledJob{
						ID:      id,
						Name:    "test-job",
						Enabled: true,
					},
					Running: true,
				}
			},
			wantExecute: false,
		},
		{
			name: "job_not_found_should_skip",
			setupJob: func(s *Scheduler, id uuid.UUID) {
				// Don't add job
			},
			wantExecute: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScheduler(nil, nil, nil)
			jobID := uuid.New()

			if tt.setupJob != nil {
				tt.setupJob(s, jobID)
			}

			// Execute
			job, shouldExecute := s.prepareJobExecution(jobID)

			// Assert
			assert.Equal(t, tt.wantExecute, shouldExecute)

			if shouldExecute {
				// Verify job was marked as running
				assert.NotNil(t, job, "job should be returned")
				assert.True(t, job.Running, "job should be marked as running")
			} else {
				assert.Nil(t, job, "job should be nil when not executing")
			}
		})
	}
}

// TestScheduler_CleanupJobExecution tests the cleanupJobExecution helper
func TestScheduler_CleanupJobExecution(t *testing.T) {
	s := NewScheduler(nil, nil, nil)
	jobID := uuid.New()

	// Setup job
	s.jobs[jobID] = &ScheduledJob{
		ID: jobID,
		Config: &db.ScheduledJob{
			ID:      jobID,
			Name:    "test-job",
			Enabled: true,
		},
		Running: true,
	}

	// Execute
	s.cleanupJobExecution(jobID)

	// Assert
	s.mu.RLock()
	job := s.jobs[jobID]
	s.mu.RUnlock()

	assert.False(t, job.Running, "job should not be marked as running after cleanup")
}

// TestScheduler_CleanupJobExecutionConcurrency tests concurrent cleanup
func TestScheduler_CleanupJobExecutionConcurrency(t *testing.T) {
	s := NewScheduler(nil, nil, nil)
	jobID := uuid.New()

	s.jobs[jobID] = &ScheduledJob{
		ID:      jobID,
		Running: true,
		Config:  &db.ScheduledJob{ID: jobID},
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.cleanupJobExecution(jobID)
		}()
	}

	wg.Wait()

	s.mu.RLock()
	job := s.jobs[jobID]
	s.mu.RUnlock()

	assert.False(t, job.Running)
}

// TestScheduler_StoreJobInMemory tests storing a job in memory
func TestScheduler_StoreJobInMemory(t *testing.T) {
	s := NewScheduler(nil, nil, nil)
	jobID := uuid.New()
	cronID := cron.EntryID(123)

	dbJob := &db.ScheduledJob{
		ID:             jobID,
		Name:           "test-job",
		Type:           db.ScheduledJobTypeDiscovery,
		CronExpression: "0 0 * * *",
		Enabled:        true,
	}

	// Execute
	s.storeJobInMemory(dbJob, cronID)

	// Assert
	s.mu.RLock()
	job, exists := s.jobs[jobID]
	s.mu.RUnlock()

	require.True(t, exists, "job should be stored in memory")
	assert.Equal(t, jobID, job.ID)
	assert.Equal(t, cronID, job.CronID)
	assert.Equal(t, dbJob, job.Config)
	assert.False(t, job.NextRun.IsZero(), "NextRun should be calculated from cron expression")

	// Verify NextRun is correctly calculated from cron expression "0 0 * * *" (midnight daily)
	schedule, err := cron.ParseStandard("0 0 * * *")
	require.NoError(t, err)
	expectedNextRun := schedule.Next(time.Now())
	assert.WithinDuration(t, expectedNextRun, job.NextRun, 2*time.Second,
		"NextRun should match cron expression '0 0 * * *' (midnight daily)")

	assert.False(t, job.Running)
}

// TestScheduler_StoreJobInMemoryConcurrency tests that storeJobInMemory properly
// works when called with the mutex held, preventing race conditions (issue #266).
// This simulates the actual usage pattern where Start() holds the lock during job loading.
func TestScheduler_StoreJobInMemoryConcurrency(t *testing.T) {
	s := NewScheduler(nil, nil, nil)

	var wg sync.WaitGroup

	// Simulate concurrent Start() calls that hold the lock
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Acquire lock as Start() does
			s.mu.Lock()
			defer s.mu.Unlock()

			jobID := uuid.New()
			dbJob := &db.ScheduledJob{
				ID:             jobID,
				Name:           fmt.Sprintf("test-job-%d", idx),
				Type:           db.ScheduledJobTypeDiscovery,
				CronExpression: "0 0 * * *",
				Enabled:        true,
			}
			// Call storeJobInMemory while holding lock (as Start() does)
			s.storeJobInMemory(dbJob, cron.EntryID(idx))
		}(i)
	}

	// Concurrent reads should use proper locking
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.mu.RLock()
			_ = len(s.jobs)
			s.mu.RUnlock()
		}()
	}

	wg.Wait()

	// Should have 10 jobs
	s.mu.RLock()
	count := len(s.jobs)
	s.mu.RUnlock()

	assert.Equal(t, 10, count, "all 10 jobs should be stored without data loss")
}

// ==============================================================================
// Phase 2: Job Configuration Tests
// ==============================================================================

// TestDiscoveryJobConfig_JSONMarshalUnmarshal tests marshaling/unmarshaling
func TestDiscoveryJobConfig_JSONMarshalUnmarshal(t *testing.T) {
	config := DiscoveryJobConfig{
		NetworkID:   "550e8400-e29b-41d4-a716-446655440000",
		Network:     "192.168.1.0/24",
		Method:      "ping",
		DetectOS:    true,
		Timeout:     30,
		Concurrency: 50,
	}

	// Marshal
	data, err := json.Marshal(config)
	require.NoError(t, err)

	// Unmarshal
	var decoded DiscoveryJobConfig
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, config.NetworkID, decoded.NetworkID)
	assert.Equal(t, config.Network, decoded.Network)
	assert.Equal(t, config.Method, decoded.Method)
	assert.Equal(t, config.DetectOS, decoded.DetectOS)
	assert.Equal(t, config.Timeout, decoded.Timeout)
	assert.Equal(t, config.Concurrency, decoded.Concurrency)

	// Verify the JSON key name matches what buildScheduleJobConfig writes ("network_id").
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", raw["network_id"],
		"DiscoveryJobConfig must serialize NetworkID under the key \"network_id\"")

	// Verify that a payload produced by buildScheduleJobConfig (only "network_id",
	// no "network" / "method") is correctly decoded: NetworkID is populated and
	// Network/Method fall back to their zero values.
	apiPayload := []byte(`{"network_id":"550e8400-e29b-41d4-a716-446655440000"}`)
	var fromAPI DiscoveryJobConfig
	require.NoError(t, json.Unmarshal(apiPayload, &fromAPI))
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", fromAPI.NetworkID)
	assert.Empty(t, fromAPI.Network, "Network must be empty when only network_id is stored")
	assert.Empty(t, fromAPI.Method, "Method must be empty when only network_id is stored")
}

// TestScanJobConfig_JSONMarshalUnmarshal tests marshaling/unmarshaling
func TestScanJobConfig_JSONMarshalUnmarshal(t *testing.T) {
	config := ScanJobConfig{
		NetworkID:     "550e8400-e29b-41d4-a716-446655440001",
		LiveHostsOnly: true,
		Networks:      []string{"192.168.1.0/24", "10.0.0.0/8"},
		ProfileID:     "profile-123",
		MaxAge:        24,
		OSFamily:      []string{"linux", "windows"},
	}

	// Marshal
	data, err := json.Marshal(config)
	require.NoError(t, err)

	// Unmarshal
	var decoded ScanJobConfig
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	// Assert
	assert.Equal(t, config.NetworkID, decoded.NetworkID)
	assert.Equal(t, config.LiveHostsOnly, decoded.LiveHostsOnly)
	assert.Equal(t, config.Networks, decoded.Networks)
	assert.Equal(t, config.ProfileID, decoded.ProfileID)
	assert.Equal(t, config.MaxAge, decoded.MaxAge)
	assert.Equal(t, config.OSFamily, decoded.OSFamily)

	// Verify the JSON key name matches what buildScheduleJobConfig writes ("network_id").
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440001", raw["network_id"],
		"ScanJobConfig must serialize NetworkID under the key \"network_id\"")

	// Verify that a payload produced by buildScheduleJobConfig (only "network_id",
	// no "networks") is correctly decoded: NetworkID is populated and Networks is nil.
	apiPayload := []byte(`{"network_id":"550e8400-e29b-41d4-a716-446655440001"}`)
	var fromAPI ScanJobConfig
	require.NoError(t, json.Unmarshal(apiPayload, &fromAPI))
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440001", fromAPI.NetworkID)
	assert.Empty(t, fromAPI.Networks, "Networks must be nil/empty when only network_id is stored")
}

// TestScheduledJob_StateTransitions tests job state transitions
func TestScheduledJob_StateTransitions(t *testing.T) {
	jobID := uuid.New()
	job := &ScheduledJob{
		ID: jobID,
		Config: &db.ScheduledJob{
			ID:      jobID,
			Name:    "test-job",
			Enabled: true,
		},
		Running: false,
	}

	// Initial state
	assert.False(t, job.Running)

	// Transition to running
	job.Running = true
	assert.True(t, job.Running)

	// Transition back to not running
	job.Running = false
	assert.False(t, job.Running)
}

// TestScheduler_AddDiscoveryJob tests adding a discovery job.
func TestScheduler_AddDiscoveryJob(t *testing.T) {
	tests := []struct {
		name        string
		jobName     string
		cronExpr    string
		config      DiscoveryJobConfig
		mockSetup   func(sqlmock.Sqlmock)
		expectError bool
		errorMsg    string
	}{
		{
			name:     "successful discovery job creation",
			jobName:  "Test Discovery",
			cronExpr: "0 0 * * *",
			config: DiscoveryJobConfig{
				Network:     "192.168.1.0/24",
				Method:      "ping",
				DetectOS:    true,
				Timeout:     30,
				Concurrency: 100,
			},
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO scheduled_jobs").
					WithArgs(sqlmock.AnyArg(), "Test Discovery", db.ScheduledJobTypeDiscovery,
						"0 0 * * *", sqlmock.AnyArg(), true, nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			expectError: false,
		},
		{
			name:     "invalid cron expression",
			jobName:  "Invalid Cron",
			cronExpr: "invalid cron",
			config: DiscoveryJobConfig{
				Network: "192.168.1.0/24",
			},
			mockSetup:   func(mock sqlmock.Sqlmock) {},
			expectError: true,
			errorMsg:    "invalid cron expression",
		},
		{
			name:     "database error on insert",
			jobName:  "DB Error Test",
			cronExpr: "0 0 * * *",
			config: DiscoveryJobConfig{
				Network: "192.168.1.0/24",
			},
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO scheduled_jobs").
					WillReturnError(sql.ErrConnDone)
			},
			expectError: true,
			errorMsg:    "failed to save scheduled job",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock database
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			tt.mockSetup(mock)

			// Create scheduler - wrap mock DB in db.DB
			wrappedDB := &db.DB{DB: sqlx.NewDb(mockDB, "sqlmock")}
			s := NewScheduler(wrappedDB, nil, nil)

			// Execute
			ctx := context.Background()
			err = s.AddDiscoveryJob(ctx, tt.jobName, tt.cronExpr, tt.config)

			// Assert
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				// Verify job was added to in-memory map
				assert.Len(t, s.jobs, 1)
			}

			// Verify all expectations were met
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestScheduler_AddScanJob tests adding a scan job.
func TestScheduler_AddScanJob(t *testing.T) {
	tests := []struct {
		name        string
		jobName     string
		cronExpr    string
		config      *ScanJobConfig
		mockSetup   func(sqlmock.Sqlmock)
		expectError bool
		errorMsg    string
	}{
		{
			name:     "successful scan job creation",
			jobName:  "Test Scan",
			cronExpr: "0 */6 * * *",
			config: &ScanJobConfig{
				LiveHostsOnly: true,
				Networks:      []string{"192.168.1.0/24"},
				ProfileID:     uuid.New().String(),
				MaxAge:        24,
				OSFamily:      []string{"linux"},
			},
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO scheduled_jobs").
					WithArgs(sqlmock.AnyArg(), "Test Scan", db.ScheduledJobTypeScan,
						"0 */6 * * *", sqlmock.AnyArg(), true, nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			expectError: false,
		},
		{
			name:     "scan job with nil config",
			jobName:  "Nil Config Test",
			cronExpr: "0 0 * * *",
			config:   nil,
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec("INSERT INTO scheduled_jobs").
					WithArgs(sqlmock.AnyArg(), "Nil Config Test", db.ScheduledJobTypeScan,
						"0 0 * * *", sqlmock.AnyArg(), true, nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
			expectError: false,
		},
		{
			name:     "invalid cron expression",
			jobName:  "Invalid Cron",
			cronExpr: "bad cron",
			config: &ScanJobConfig{
				Networks: []string{"192.168.1.0/24"},
			},
			mockSetup:   func(mock sqlmock.Sqlmock) {},
			expectError: true,
			errorMsg:    "invalid cron expression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock database
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			tt.mockSetup(mock)

			// Create scheduler - wrap mock DB in db.DB
			wrappedDB := &db.DB{DB: sqlx.NewDb(mockDB, "sqlmock")}
			s := NewScheduler(wrappedDB, nil, nil)

			// Execute
			ctx := context.Background()
			err = s.AddScanJob(ctx, tt.jobName, tt.cronExpr, tt.config)

			// Assert
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				// Verify job was added to in-memory map
				assert.Len(t, s.jobs, 1)
			}

			// Verify all expectations were met
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestScheduler_RemoveJob tests removing a scheduled job.
func TestScheduler_RemoveJob(t *testing.T) {
	tests := []struct {
		name        string
		setupJobs   func(*Scheduler) uuid.UUID
		jobID       uuid.UUID
		mockSetup   func(sqlmock.Sqlmock, uuid.UUID)
		expectError bool
		errorMsg    string
	}{
		{
			name: "successful job removal",
			setupJobs: func(s *Scheduler) uuid.UUID {
				jobID := uuid.New()
				cronID := s.cron.Schedule(cron.Every(time.Hour), cron.FuncJob(func() {}))
				s.jobs[jobID] = &ScheduledJob{
					ID:     jobID,
					CronID: cronID,
					Config: &db.ScheduledJob{
						ID:   jobID,
						Name: "Test Job",
					},
				}
				return jobID
			},
			mockSetup: func(mock sqlmock.Sqlmock, jobID uuid.UUID) {
				mock.ExpectExec("DELETE FROM scheduled_jobs WHERE id").
					WithArgs(jobID).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectError: false,
		},
		{
			name: "job not found",
			setupJobs: func(s *Scheduler) uuid.UUID {
				return uuid.New() // Return a non-existent job ID
			},
			mockSetup:   func(mock sqlmock.Sqlmock, jobID uuid.UUID) {},
			expectError: true,
			errorMsg:    "job not found",
		},
		{
			name: "database error on delete",
			setupJobs: func(s *Scheduler) uuid.UUID {
				jobID := uuid.New()
				cronID := s.cron.Schedule(cron.Every(time.Hour), cron.FuncJob(func() {}))
				s.jobs[jobID] = &ScheduledJob{
					ID:     jobID,
					CronID: cronID,
					Config: &db.ScheduledJob{
						ID:   jobID,
						Name: "Test Job",
					},
				}
				return jobID
			},
			mockSetup: func(mock sqlmock.Sqlmock, jobID uuid.UUID) {
				mock.ExpectExec("DELETE FROM scheduled_jobs WHERE id").
					WithArgs(jobID).
					WillReturnError(sql.ErrConnDone)
			},
			expectError: true,
			errorMsg:    "failed to delete from database",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock database
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			// Create scheduler - wrap mock DB in db.DB
			wrappedDB := &db.DB{DB: sqlx.NewDb(mockDB, "sqlmock")}
			s := NewScheduler(wrappedDB, nil, nil)

			// Setup jobs and get the job ID to use
			jobID := tt.setupJobs(s)
			tt.mockSetup(mock, jobID)

			// Execute
			ctx := context.Background()
			err = s.RemoveJob(ctx, jobID)

			// Assert
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				// Verify job was removed from in-memory map
				_, exists := s.jobs[jobID]
				assert.False(t, exists, "job should be removed from memory")
			}

			// Verify all expectations were met
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestScheduler_GetJobs tests retrieving all scheduled jobs.
func TestScheduler_GetJobs(t *testing.T) {
	tests := []struct {
		name          string
		mockSetup     func(sqlmock.Sqlmock)
		expectedCount int
	}{
		{
			name: "retrieve multiple jobs",
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "name", "type", "cron_expression", "config",
					"enabled", "last_run", "next_run", "created_at",
				}).
					AddRow(
						uuid.New(), "Job 1", db.ScheduledJobTypeDiscovery,
						"0 0 * * *", []byte(`{"network":"192.168.1.0/24"}`),
						true, nil, time.Now(), time.Now(),
					).
					AddRow(
						uuid.New(), "Job 2", db.ScheduledJobTypeScan,
						"0 */6 * * *", []byte(`{"networks":["10.0.0.0/8"]}`),
						true, nil, time.Now(), time.Now(),
					)

				mock.ExpectQuery("SELECT (.+) FROM scheduled_jobs").
					WillReturnRows(rows)
			},
			expectedCount: 2,
		},
		{
			name: "no jobs found",
			mockSetup: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "name", "type", "cron_expression", "config",
					"enabled", "last_run", "next_run", "created_at",
				})

				mock.ExpectQuery("SELECT (.+) FROM scheduled_jobs").
					WillReturnRows(rows)
			},
			expectedCount: 0,
		},
		{
			name: "database error returns empty list",
			mockSetup: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT (.+) FROM scheduled_jobs").
					WillReturnError(sql.ErrConnDone)
			},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock database
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			tt.mockSetup(mock)

			// Create scheduler - wrap mock DB in db.DB
			wrappedDB := &db.DB{DB: sqlx.NewDb(mockDB, "sqlmock")}
			s := NewScheduler(wrappedDB, nil, nil)

			// Execute
			jobs := s.GetJobs()

			// Assert
			assert.Len(t, jobs, tt.expectedCount)

			// Verify NextRun is set for each job
			for _, job := range jobs {
				assert.False(t, job.NextRun.IsZero(), "NextRun should be calculated")
			}

			// Verify all expectations were met
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestScheduler_EnableJob tests enabling a scheduled job.
func TestScheduler_EnableJob(t *testing.T) {
	tests := []struct {
		name        string
		setupJob    func(*Scheduler) uuid.UUID
		mockSetup   func(sqlmock.Sqlmock, uuid.UUID)
		expectError bool
		errorMsg    string
	}{
		{
			name: "successfully enable job",
			setupJob: func(s *Scheduler) uuid.UUID {
				jobID := uuid.New()
				s.jobs[jobID] = &ScheduledJob{
					ID: jobID,
					Config: &db.ScheduledJob{
						ID:      jobID,
						Name:    "Test Job",
						Enabled: false,
					},
				}
				return jobID
			},
			mockSetup: func(mock sqlmock.Sqlmock, jobID uuid.UUID) {
				mock.ExpectExec("UPDATE scheduled_jobs SET enabled").
					WithArgs(true, jobID).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectError: false,
		},
		{
			name: "job not found",
			setupJob: func(s *Scheduler) uuid.UUID {
				return uuid.New() // Return non-existent ID
			},
			mockSetup:   func(mock sqlmock.Sqlmock, jobID uuid.UUID) {},
			expectError: true,
			errorMsg:    "job not found",
		},
		{
			name: "database error on update",
			setupJob: func(s *Scheduler) uuid.UUID {
				jobID := uuid.New()
				s.jobs[jobID] = &ScheduledJob{
					ID: jobID,
					Config: &db.ScheduledJob{
						ID:      jobID,
						Name:    "Test Job",
						Enabled: false,
					},
				}
				return jobID
			},
			mockSetup: func(mock sqlmock.Sqlmock, jobID uuid.UUID) {
				mock.ExpectExec("UPDATE scheduled_jobs SET enabled").
					WithArgs(true, jobID).
					WillReturnError(sql.ErrConnDone)
			},
			expectError: true,
			errorMsg:    "failed to update job status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock database
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			// Create scheduler - wrap mock DB in db.DB
			wrappedDB := &db.DB{DB: sqlx.NewDb(mockDB, "sqlmock")}
			s := NewScheduler(wrappedDB, nil, nil)

			// Setup job
			jobID := tt.setupJob(s)
			tt.mockSetup(mock, jobID)

			// Execute
			ctx := context.Background()
			err = s.EnableJob(ctx, jobID)

			// Assert
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				// Verify job is enabled in memory
				job, exists := s.jobs[jobID]
				require.True(t, exists)
				assert.True(t, job.Config.Enabled, "job should be enabled in memory")
			}

			// Verify all expectations were met
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestScheduler_DisableJob tests disabling a scheduled job.
func TestScheduler_DisableJob(t *testing.T) {
	tests := []struct {
		name        string
		setupJob    func(*Scheduler) uuid.UUID
		mockSetup   func(sqlmock.Sqlmock, uuid.UUID)
		expectError bool
		errorMsg    string
	}{
		{
			name: "successfully disable job",
			setupJob: func(s *Scheduler) uuid.UUID {
				jobID := uuid.New()
				s.jobs[jobID] = &ScheduledJob{
					ID: jobID,
					Config: &db.ScheduledJob{
						ID:      jobID,
						Name:    "Test Job",
						Enabled: true,
					},
				}
				return jobID
			},
			mockSetup: func(mock sqlmock.Sqlmock, jobID uuid.UUID) {
				mock.ExpectExec("UPDATE scheduled_jobs SET enabled").
					WithArgs(false, jobID).
					WillReturnResult(sqlmock.NewResult(0, 1))
			},
			expectError: false,
		},
		{
			name: "job not found",
			setupJob: func(s *Scheduler) uuid.UUID {
				return uuid.New() // Return non-existent ID
			},
			mockSetup:   func(mock sqlmock.Sqlmock, jobID uuid.UUID) {},
			expectError: true,
			errorMsg:    "job not found",
		},
		{
			name: "database error on update",
			setupJob: func(s *Scheduler) uuid.UUID {
				jobID := uuid.New()
				s.jobs[jobID] = &ScheduledJob{
					ID: jobID,
					Config: &db.ScheduledJob{
						ID:      jobID,
						Name:    "Test Job",
						Enabled: true,
					},
				}
				return jobID
			},
			mockSetup: func(mock sqlmock.Sqlmock, jobID uuid.UUID) {
				mock.ExpectExec("UPDATE scheduled_jobs SET enabled").
					WithArgs(false, jobID).
					WillReturnError(sql.ErrConnDone)
			},
			expectError: true,
			errorMsg:    "failed to update job status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock database
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			// Create scheduler - wrap mock DB in db.DB
			wrappedDB := &db.DB{DB: sqlx.NewDb(mockDB, "sqlmock")}
			s := NewScheduler(wrappedDB, nil, nil)

			// Setup job
			jobID := tt.setupJob(s)
			tt.mockSetup(mock, jobID)

			// Execute
			ctx := context.Background()
			err = s.DisableJob(ctx, jobID)

			// Assert
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				// Verify job is disabled in memory
				job, exists := s.jobs[jobID]
				require.True(t, exists)
				assert.False(t, job.Config.Enabled, "job should be disabled in memory")
			}

			// Verify all expectations were met
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestScheduler_SetJobEnabled tests the internal setJobEnabled method.
func TestScheduler_SetJobEnabled(t *testing.T) {
	t.Run("concurrent enable/disable operations", func(t *testing.T) {
		// Create mock database
		mockDB, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer mockDB.Close()

		// Create scheduler with a job - wrap mock DB in db.DB
		wrappedDB := &db.DB{DB: sqlx.NewDb(mockDB, "sqlmock")}
		s := NewScheduler(wrappedDB, nil, nil)
		jobID := uuid.New()
		s.jobs[jobID] = &ScheduledJob{
			ID: jobID,
			Config: &db.ScheduledJob{
				ID:      jobID,
				Name:    "Concurrent Test Job",
				Enabled: true,
			},
		}

		// Set up expectations for multiple updates
		mock.ExpectExec("UPDATE scheduled_jobs SET enabled").
			WithArgs(false, jobID).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec("UPDATE scheduled_jobs SET enabled").
			WithArgs(true, jobID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		// Execute concurrent operations
		var wg sync.WaitGroup
		wg.Add(2)

		ctx := context.Background()

		go func() {
			defer wg.Done()
			_ = s.DisableJob(ctx, jobID)
		}()

		go func() {
			defer wg.Done()
			time.Sleep(10 * time.Millisecond)
			_ = s.EnableJob(ctx, jobID)
		}()

		wg.Wait()

		// Verify job state is consistent
		job, exists := s.jobs[jobID]
		require.True(t, exists)
		assert.NotNil(t, job.Config)
	})
}

// TestScheduler_Start tests the scheduler start behavior.
func TestScheduler_Start(t *testing.T) {
	tests := []struct {
		name        string
		setupJobs   func(sqlmock.Sqlmock)
		expectError bool
		errorMsg    string
		jobCount    int
	}{
		{
			name: "start with existing jobs loaded",
			setupJobs: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "name", "type", "cron_expression", "config",
					"enabled", "last_run", "next_run", "created_at",
				}).
					AddRow(
						uuid.New(), "Test Job 1", db.ScheduledJobTypeDiscovery,
						"0 0 * * *", []byte(`{"network":"192.168.1.0/24"}`),
						true, nil, time.Now(), time.Now(),
					).
					AddRow(
						uuid.New(), "Test Job 2", db.ScheduledJobTypeScan,
						"0 */6 * * *", []byte(`{"networks":["10.0.0.0/8"]}`),
						true, nil, time.Now(), time.Now(),
					)

				mock.ExpectQuery("SELECT (.+) FROM scheduled_jobs").
					WillReturnRows(rows)
			},
			expectError: false,
			jobCount:    2,
		},
		{
			name: "start with no existing jobs",
			setupJobs: func(mock sqlmock.Sqlmock) {
				rows := sqlmock.NewRows([]string{
					"id", "name", "type", "cron_expression", "config",
					"enabled", "last_run", "next_run", "created_at",
				})

				mock.ExpectQuery("SELECT (.+) FROM scheduled_jobs").
					WillReturnRows(rows)
			},
			expectError: false,
			jobCount:    0,
		},
		{
			name: "start with database error",
			setupJobs: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT (.+) FROM scheduled_jobs").
					WillReturnError(sql.ErrConnDone)
			},
			expectError: true,
			errorMsg:    "failed to load scheduled jobs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock database
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			tt.setupJobs(mock)

			// Create scheduler
			wrappedDB := &db.DB{DB: sqlx.NewDb(mockDB, "sqlmock")}
			s := NewScheduler(wrappedDB, nil, nil)

			// Execute
			err = s.Start()

			// Assert
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
				assert.Len(t, s.jobs, tt.jobCount, "should have loaded correct number of jobs")
				assert.True(t, s.running, "scheduler should be marked as running")
			}

			// Verify all expectations were met
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

// TestScheduler_StartAlreadyRunning tests that starting an already running scheduler returns an error.
func TestScheduler_StartAlreadyRunning(t *testing.T) {
	// Create mock database
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer mockDB.Close()

	// Setup mock to return no jobs
	rows := sqlmock.NewRows([]string{
		"id", "name", "type", "cron_expression", "config",
		"enabled", "last_run", "next_run", "created_at",
	})
	mock.ExpectQuery("SELECT (.+) FROM scheduled_jobs").
		WillReturnRows(rows)

	// Create and start scheduler
	wrappedDB := &db.DB{DB: sqlx.NewDb(mockDB, "sqlmock")}
	s := NewScheduler(wrappedDB, nil, nil)

	err = s.Start()
	require.NoError(t, err)

	// Try to start again
	err = s.Start()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

// ==============================================================================
// Phase 1c: Concurrency Control, NextRun Persistence, and Scheduler Options
// ==============================================================================

// TestWithMaxConcurrentScans verifies that WithMaxConcurrentScans correctly
// sets the concurrency limit and that the setter returns the scheduler for
// call chaining.
func TestWithMaxConcurrentScans(t *testing.T) {
	t.Run("sets positive value", func(t *testing.T) {
		s := NewScheduler(nil, nil, nil)
		assert.Equal(t, defaultMaxConcurrentScans, s.maxConcurrentScans)

		result := s.WithMaxConcurrentScans(10)

		assert.Equal(t, 10, s.maxConcurrentScans)
		assert.Same(t, s, result, "should return the same scheduler for chaining")
	})

	t.Run("ignores zero value (keeps default)", func(t *testing.T) {
		s := NewScheduler(nil, nil, nil)
		s.WithMaxConcurrentScans(0)

		assert.Equal(t, defaultMaxConcurrentScans, s.maxConcurrentScans,
			"zero should be ignored, keeping the default")
	})

	t.Run("ignores negative value (keeps default)", func(t *testing.T) {
		s := NewScheduler(nil, nil, nil)
		s.WithMaxConcurrentScans(-5)

		assert.Equal(t, defaultMaxConcurrentScans, s.maxConcurrentScans,
			"negative value should be ignored, keeping the default")
	})

	t.Run("allows value of 1 (serial execution)", func(t *testing.T) {
		s := NewScheduler(nil, nil, nil)
		s.WithMaxConcurrentScans(1)

		assert.Equal(t, 1, s.maxConcurrentScans)
	})

	t.Run("allows large value", func(t *testing.T) {
		s := NewScheduler(nil, nil, nil)
		s.WithMaxConcurrentScans(100)

		assert.Equal(t, 100, s.maxConcurrentScans)
	})
}

// TestNewScheduler_DefaultConcurrency verifies that the default max concurrent
// scans is set correctly on construction.
func TestNewScheduler_DefaultConcurrency(t *testing.T) {
	s := NewScheduler(nil, nil, nil)
	assert.Equal(t, defaultMaxConcurrentScans, s.maxConcurrentScans,
		"scheduler should use default concurrency on construction")
}

// TestCalculateNextRun verifies that calculateNextRun returns the correct next
// run time for a given cron expression and reference time.
func TestCalculateNextRun(t *testing.T) {
	t.Run("returns correct next run for valid cron expression", func(t *testing.T) {
		s := NewScheduler(nil, nil, nil)

		jobID := uuid.New()
		cronExpr := "0 * * * *" // every hour at minute 0

		// Register a fake job in memory with the cron expression.
		s.jobs[jobID] = &ScheduledJob{
			ID: jobID,
			Config: &db.ScheduledJob{
				ID:             jobID,
				CronExpression: cronExpr,
			},
		}

		// Use a fixed reference time: 2024-01-15 14:30:00 UTC
		ref := time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC)
		next := s.calculateNextRun(jobID, ref)

		// Next run should be 15:00:00 UTC
		expected := time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC)
		assert.Equal(t, expected, next)
	})

	t.Run("returns zero time when job not found", func(t *testing.T) {
		s := NewScheduler(nil, nil, nil)

		next := s.calculateNextRun(uuid.New(), time.Now())

		assert.True(t, next.IsZero(), "should return zero time for unknown job")
	})

	t.Run("returns zero time for invalid cron expression", func(t *testing.T) {
		s := NewScheduler(nil, nil, nil)

		jobID := uuid.New()
		s.jobs[jobID] = &ScheduledJob{
			ID: jobID,
			Config: &db.ScheduledJob{
				ID:             jobID,
				CronExpression: "not-a-valid-cron",
			},
		}

		next := s.calculateNextRun(jobID, time.Now())

		assert.True(t, next.IsZero(), "should return zero time for invalid cron expression")
	})

	t.Run("daily cron returns next midnight", func(t *testing.T) {
		s := NewScheduler(nil, nil, nil)

		jobID := uuid.New()
		s.jobs[jobID] = &ScheduledJob{
			ID: jobID,
			Config: &db.ScheduledJob{
				ID:             jobID,
				CronExpression: "0 0 * * *", // midnight every day
			},
		}

		ref := time.Date(2024, 3, 10, 8, 0, 0, 0, time.UTC)
		next := s.calculateNextRun(jobID, ref)

		expected := time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC)
		assert.Equal(t, expected, next)
	})
}

// TestUpdateJobNextRunInMemory verifies that the in-memory NextRun field is
// updated correctly and that concurrent access is safe.
func TestUpdateJobNextRunInMemory(t *testing.T) {
	t.Run("updates NextRun for existing job", func(t *testing.T) {
		s := NewScheduler(nil, nil, nil)

		jobID := uuid.New()
		s.jobs[jobID] = &ScheduledJob{
			ID:      jobID,
			NextRun: time.Time{},
			Config:  &db.ScheduledJob{ID: jobID},
		}

		nextRun := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
		s.updateJobNextRunInMemory(jobID, nextRun)

		s.mu.RLock()
		got := s.jobs[jobID].NextRun
		s.mu.RUnlock()

		assert.Equal(t, nextRun, got)
	})

	t.Run("is a no-op for unknown job", func(t *testing.T) {
		s := NewScheduler(nil, nil, nil)

		// Should not panic even when job is not in the map.
		assert.NotPanics(t, func() {
			s.updateJobNextRunInMemory(uuid.New(), time.Now())
		})
	})

	t.Run("concurrent updates do not race", func(t *testing.T) {
		s := NewScheduler(nil, nil, nil)

		jobID := uuid.New()
		s.jobs[jobID] = &ScheduledJob{
			ID:     jobID,
			Config: &db.ScheduledJob{ID: jobID},
		}

		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				s.updateJobNextRunInMemory(jobID, time.Now().Add(time.Duration(n)*time.Minute))
			}(i)
		}
		wg.Wait()

		// The final value is non-deterministic but the scheduler must not crash.
		s.mu.RLock()
		_, exists := s.jobs[jobID]
		s.mu.RUnlock()
		assert.True(t, exists)
	})
}

// TestExecuteDiscoveryJob_NetworkIDResolution tests the network-ID resolution
// path inside executeDiscoveryJob: when config.Network is empty but
// config.NetworkID is set, the function must look up the CIDR and discovery
// method from the database before running the discovery engine.
func TestExecuteDiscoveryJob_NetworkIDResolution(t *testing.T) {
	tests := []struct {
		name      string
		mockSetup func(mock sqlmock.Sqlmock, networkID uuid.UUID)
	}{
		{
			// The SELECT fails: the function should log and return early without
			// calling s.discovery.Discover (and therefore without panicking on
			// the nil discovery engine).  No UPDATE is issued because
			// updateJobLastRun is only called after a successful Discover call,
			// not in a defer.
			name: "db_lookup_fails_returns_early",
			mockSetup: func(mock sqlmock.Sqlmock, networkID uuid.UUID) {
				mock.ExpectQuery("SELECT cidr").
					WithArgs(networkID.String()).
					WillReturnError(sql.ErrNoRows)
			},
		},
		{
			// The SELECT succeeds: the function proceeds to call
			// s.discovery.Discover.  s.discovery is a nil *discovery.Engine,
			// but Go allows method calls on nil receivers; the Engine's Discover
			// method validates the network before touching any receiver fields,
			// so for "10.0.0.0/8" it returns an error ("network size too large")
			// rather than panicking.  executeDiscoveryJob calls updateJobLastRun
			// unconditionally after Discover returns (success or failure), so an
			// UPDATE expectation is required after the SELECT.
			name: "db_lookup_succeeds_proceeds",
			mockSetup: func(mock sqlmock.Sqlmock, networkID uuid.UUID) {
				rows := sqlmock.NewRows([]string{"cidr", "discovery_method"}).
					AddRow("10.0.0.0/8", "tcp")
				mock.ExpectQuery("SELECT cidr").
					WithArgs(networkID.String()).
					WillReturnRows(rows)
				// updateJobLastRun fires after Discover returns with an error.
				mock.ExpectExec("UPDATE scheduled_jobs").
					WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
					WillReturnResult(sqlmock.NewResult(1, 1))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer mockDB.Close()

			jobID := uuid.New()
			networkID := uuid.New()

			tt.mockSetup(mock, networkID)

			wrappedDB := &db.DB{DB: sqlx.NewDb(mockDB, "sqlmock")}
			// nil discovery engine: Discover() will panic if reached; the
			// function's own recover() handles that case.
			s := NewScheduler(wrappedDB, nil, nil)

			s.jobs[jobID] = &ScheduledJob{
				ID: jobID,
				Config: &db.ScheduledJob{
					ID:      jobID,
					Name:    "net-test",
					Enabled: true,
				},
				Running: false,
			}

			// Must not propagate any panic to the test.
			s.executeDiscoveryJob(jobID, DiscoveryJobConfig{NetworkID: networkID.String()})

			assert.NoError(t, mock.ExpectationsWereMet())

			s.mu.RLock()
			running := s.jobs[jobID].Running
			s.mu.RUnlock()
			assert.False(t, running, "job should not be marked as running after executeDiscoveryJob returns")
		})
	}
}

// TestExecuteScanJob_NetworkIDResolution tests the network-ID resolution path
// inside executeScanJob: when config.Networks is empty but config.NetworkID is
// set, the function must look up the CIDR from the database before querying
// hosts.
func TestExecuteScanJob_NetworkIDResolution(t *testing.T) {
	t.Run("db_lookup_fails_returns_early", func(t *testing.T) {
		mockDB, mock, err := sqlmock.New()
		require.NoError(t, err)
		defer mockDB.Close()

		jobID := uuid.New()
		networkID := uuid.New()

		// Execution order inside executeScanJob when the SELECT fails:
		//   1. prepareJobExecution succeeds → Running = true
		//   2. defer cleanupJobExecution registered  (no DB call)
		//   3. defer updateJobLastRun registered     (will issue UPDATE)
		//   4. SELECT cidr fails → early return
		//   5. Defers fire LIFO: updateJobLastRun → issues UPDATE
		//                        cleanupJobExecution → no DB call
		//                        panic-recovery defer → r == nil, no-op
		// Register expectations in the order the queries actually hit the driver.
		mock.ExpectQuery("SELECT cidr").
			WithArgs(networkID.String()).
			WillReturnError(sql.ErrNoRows)

		mock.ExpectExec("UPDATE scheduled_jobs").
			WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), jobID).
			WillReturnResult(sqlmock.NewResult(1, 1))

		wrappedDB := &db.DB{DB: sqlx.NewDb(mockDB, "sqlmock")}
		s := NewScheduler(wrappedDB, nil, nil)

		s.jobs[jobID] = &ScheduledJob{
			ID: jobID,
			Config: &db.ScheduledJob{
				ID:      jobID,
				Name:    "net-test",
				Enabled: true,
			},
			Running: false,
		}

		s.executeScanJob(jobID, &ScanJobConfig{NetworkID: networkID.String()})

		assert.NoError(t, mock.ExpectationsWereMet())

		s.mu.RLock()
		running := s.jobs[jobID].Running
		s.mu.RUnlock()
		assert.False(t, running, "job should not be marked as running after executeScanJob returns")
	})
}
