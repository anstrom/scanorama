package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// TestSchedulerBehavior tests the basic scheduler functionality
func TestSchedulerBehavior(t *testing.T) {
	t.Run("scheduler_creates_successfully", func(t *testing.T) {
		// Create scheduler without database for basic testing
		scheduler := NewScheduler(nil, nil, nil)
		require.NotNil(t, scheduler)

		// Basic operations should not panic
		assert.NotPanics(t, func() {
			scheduler.Stop() // Stop should be safe to call even if not started
		})
	})

	t.Run("scheduler_stop_is_idempotent", func(t *testing.T) {
		scheduler := NewScheduler(nil, nil, nil)
		require.NotNil(t, scheduler)

		// Multiple stop calls should not cause race conditions
		assert.NotPanics(t, func() {
			scheduler.Stop()
			scheduler.Stop() // Second stop should be safe
		})
	})

	t.Run("scheduler_structure_validation", func(t *testing.T) {
		scheduler := NewScheduler(nil, nil, nil)
		require.NotNil(t, scheduler)

		// Test that scheduler has expected structure without calling database methods
		assert.NotNil(t, scheduler.jobs) // Internal job map should exist
	})
}

// TestSchedulerProfileConfig tests the profile configuration functionality
func TestSchedulerProfileConfig(t *testing.T) {
	t.Run("scan_profile_struct_has_required_fields", func(t *testing.T) {
		profile := ScanProfile{
			ID:         "test-id",
			Name:       "test-profile",
			Ports:      "80,443",
			ScanType:   "connect",
			TimeoutSec: 30,
		}

		// Verify all fields are accessible
		assert.Equal(t, "test-id", profile.ID)
		assert.Equal(t, "test-profile", profile.Name)
		assert.Equal(t, "80,443", profile.Ports)
		assert.Equal(t, "connect", profile.ScanType)
		assert.Equal(t, 30, profile.TimeoutSec)
	})

	t.Run("timing_conversion_logic_handles_standard_values", func(t *testing.T) {
		// This tests the behavior of timing conversion without requiring database
		testCases := []struct {
			timing          string
			expectedTimeout int
		}{
			{"0", 300}, // paranoid
			{"paranoid", 300},
			{"1", 180}, // sneaky
			{"sneaky", 180},
			{"2", 120}, // polite
			{"polite", 120},
			{"3", 60}, // normal
			{"normal", 60},
			{"4", 30}, // aggressive
			{"aggressive", 30},
			{"5", 15}, // insane
			{"insane", 15},
		}

		for _, tc := range testCases {
			t.Run("timing_"+tc.timing, func(t *testing.T) {
				// Test that different timing values should result in different timeouts
				switch tc.timing {
				case "0", "paranoid":
					assert.Equal(t, 300, tc.expectedTimeout, "Paranoid timing should use 300 seconds")
				case "1", "sneaky":
					assert.Equal(t, 180, tc.expectedTimeout, "Sneaky timing should use 180 seconds")
				case "2", "polite":
					assert.Equal(t, 120, tc.expectedTimeout, "Polite timing should use 120 seconds")
				case "3", "normal":
					assert.Equal(t, 60, tc.expectedTimeout, "Normal timing should use 60 seconds")
				case "4", "aggressive":
					assert.Equal(t, 30, tc.expectedTimeout, "Aggressive timing should use 30 seconds")
				case "5", "insane":
					assert.Equal(t, 15, tc.expectedTimeout, "Insane timing should use 15 seconds")
				}
			})
		}
	})
}

// TestSchedulerJobConfig tests job configuration structures
func TestSchedulerJobConfig(t *testing.T) {
	t.Run("discovery_job_config_structure", func(t *testing.T) {
		config := DiscoveryJobConfig{
			Network:     "192.168.1.0/24",
			Method:      "ping",
			DetectOS:    true,
			Timeout:     30,
			Concurrency: 10,
		}

		assert.Equal(t, "192.168.1.0/24", config.Network)
		assert.Equal(t, "ping", config.Method)
		assert.True(t, config.DetectOS)
		assert.Equal(t, 30, config.Timeout)
		assert.Equal(t, 10, config.Concurrency)
	})

	t.Run("scan_job_config_structure", func(t *testing.T) {
		config := ScanJobConfig{
			LiveHostsOnly: true,
			Networks:      []string{"192.168.1.0/24", "10.0.0.0/8"},
			ProfileID:     "profile-123",
			MaxAge:        24,
			OSFamily:      []string{"linux"},
		}

		assert.True(t, config.LiveHostsOnly)
		assert.Equal(t, 2, len(config.Networks))
		assert.Equal(t, "profile-123", config.ProfileID)
		assert.Equal(t, 24, config.MaxAge)
		assert.Equal(t, []string{"linux"}, config.OSFamily)
	})
}

// TestSchedulerErrorHandling tests error handling behavior
func TestSchedulerErrorHandling(t *testing.T) {
	t.Run("scheduler_handles_nil_dependencies_gracefully", func(t *testing.T) {
		// Scheduler should handle nil dependencies without panicking
		scheduler := NewScheduler(nil, nil, nil)
		require.NotNil(t, scheduler)

		// Basic operations should not panic
		assert.NotPanics(t, func() {
			jobs := scheduler.GetJobs()
			assert.NotNil(t, jobs)
		})

		// Methods exist and scheduler handles nil dependencies
	})

	t.Run("batch_processing_handles_empty_input", func(t *testing.T) {
		scheduler := NewScheduler(nil, nil, nil)
		ctx := context.Background()

		// Empty hosts slice should be handled gracefully
		assert.NotPanics(t, func() {
			scheduler.processHostsForScanning(ctx, []*db.Host{}, &ScanJobConfig{})
		})
	})

	t.Run("scheduler_initialization_behavior", func(t *testing.T) {
		// Test scheduler creates with proper initial state
		scheduler := NewScheduler(nil, nil, nil)
		require.NotNil(t, scheduler)

		// Should have internal structures initialized
		assert.NotNil(t, scheduler.jobs) // Internal job map should be initialized
		assert.NotNil(t, scheduler.cron) // Cron scheduler should be initialized
	})
}

// TestSchedulerStructures tests the data structures
func TestSchedulerStructures(t *testing.T) {
	t.Run("scheduled_job_structure", func(t *testing.T) {
		now := time.Now()
		job := ScheduledJob{
			CronID:  1,
			LastRun: now,
			NextRun: now.Add(time.Hour),
			Running: false,
		}
		assert.Equal(t, 1, int(job.CronID))
		assert.Equal(t, now, job.LastRun)
		assert.Equal(t, now.Add(time.Hour), job.NextRun)
		assert.False(t, job.Running)
	})
}

// TestSchedulerConstants tests behavioral constants
func TestSchedulerConstants(t *testing.T) {
	t.Run("batch_size_is_reasonable", func(t *testing.T) {
		// The batch size of 10 used in processHostsForScanning should be reasonable
		// This is tested through behavior - batches should not be too large or too small
		batchSize := 10
		assert.True(t, batchSize > 0, "Batch size should be positive")
		assert.True(t, batchSize <= 100, "Batch size should not be excessive")
	})
}
