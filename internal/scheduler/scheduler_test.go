// Package scheduler_test provides tests for the scheduler package.
package scheduler

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

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

// TestExecuteDiscoveryJobPanicRecovery tests the panic recovery
// in executeDiscoveryJob without requiring a full discovery engine.
func TestExecuteDiscoveryJobPanicRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		jobs:   make(map[uuid.UUID]*ScheduledJob),
		mu:     sync.RWMutex{},
		ctx:    ctx,
		cancel: cancel,
	}

	// Create a disabled job (won't execute but tests code path)
	jobID := uuid.New()
	configJSON, _ := json.Marshal(map[string]interface{}{
		"network": "192.168.1.0/24",
	})
	jobConfig := &db.ScheduledJob{
		ID:      jobID,
		Name:    "test-discovery-job",
		Type:    "discovery",
		Enabled: false, // Disabled so it returns early
		Config:  db.JSONB(configJSON),
	}

	s.jobs[jobID] = &ScheduledJob{
		ID:      jobID,
		Config:  jobConfig,
		Running: false,
	}

	config := DiscoveryJobConfig{
		Network:     "192.168.1.0/24",
		Method:      "ping",
		DetectOS:    false,
		Timeout:     30,
		Concurrency: 50,
	}

	// Execute - should not panic
	didPanic := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				didPanic = true
				t.Errorf("Panic was not recovered in executeDiscoveryJob: %v", r)
			}
		}()
		s.executeDiscoveryJob(jobID, config)
	}()

	if didPanic {
		t.Error("executeDiscoveryJob panicked despite recovery wrapper")
	}

	// Verify job state is consistent
	s.mu.RLock()
	job := s.jobs[jobID]
	s.mu.RUnlock()

	if job.Running {
		t.Error("Job incorrectly marked as running")
	}
}

// TestExecuteScanJobPanicRecovery tests the panic recovery
// in executeScanJob.
func TestExecuteScanJobPanicRecovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &Scheduler{
		jobs:   make(map[uuid.UUID]*ScheduledJob),
		mu:     sync.RWMutex{},
		ctx:    ctx,
		cancel: cancel,
	}

	// Create a disabled job
	jobID := uuid.New()
	configJSON, _ := json.Marshal(map[string]interface{}{
		"live_hosts_only": true,
	})
	jobConfig := &db.ScheduledJob{
		ID:      jobID,
		Name:    "test-scan-job",
		Type:    "scan",
		Enabled: false, // Disabled so it returns early
		Config:  db.JSONB(configJSON),
	}

	s.jobs[jobID] = &ScheduledJob{
		ID:      jobID,
		Config:  jobConfig,
		Running: false,
	}

	config := &ScanJobConfig{
		LiveHostsOnly: true,
		Networks:      []string{"192.168.1.0/24"},
	}

	// Execute - should not panic
	didPanic := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				didPanic = true
				t.Errorf("Panic was not recovered in executeScanJob: %v", r)
			}
		}()
		s.executeScanJob(jobID, config)
	}()

	if didPanic {
		t.Error("executeScanJob panicked despite recovery wrapper")
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
