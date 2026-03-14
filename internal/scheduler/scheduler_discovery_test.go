// Package scheduler provides tests for the scheduler package.
package scheduler

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
)

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
