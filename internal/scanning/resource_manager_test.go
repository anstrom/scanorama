package scanning

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestFixedResourceManager_Acquire(t *testing.T) {
	t.Run("successful acquisition", func(t *testing.T) {
		rm := NewFixedResourceManager(5)
		ctx := context.Background()

		err := rm.Acquire(ctx, "test-scan-1")
		if err != nil {
			t.Fatalf("Expected successful acquisition, got error: %v", err)
		}

		if rm.GetActiveScans() != 1 {
			t.Errorf("Expected 1 active scan, got %d", rm.GetActiveScans())
		}

		rm.Release("test-scan-1")
	})

	t.Run("resource exhaustion", func(t *testing.T) {
		rm := NewFixedResourceManager(2)
		ctx := context.Background()

		// Acquire all available slots
		err1 := rm.Acquire(ctx, "scan-1")
		err2 := rm.Acquire(ctx, "scan-2")

		if err1 != nil || err2 != nil {
			t.Fatalf("Expected successful acquisition, got errors: %v, %v", err1, err2)
		}

		// Third acquisition should timeout
		ctx3, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		err3 := rm.Acquire(ctx3, "scan-3")
		if err3 == nil {
			t.Error("Expected timeout error, got success")
		}

		// Clean up
		rm.Release("scan-1")
		rm.Release("scan-2")
	})

	t.Run("context cancellation", func(t *testing.T) {
		rm := NewFixedResourceManager(1)

		// Fill the semaphore
		ctx1 := context.Background()
		err := rm.Acquire(ctx1, "blocking-scan")
		if err != nil {
			t.Fatalf("Expected successful acquisition, got error: %v", err)
		}

		// Cancel context while waiting
		ctx2, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err2 := rm.Acquire(ctx2, "cancelled-scan")
		if err2 == nil {
			t.Error("Expected cancellation error, got success")
		}

		rm.Release("blocking-scan")
	})
}

func TestFixedResourceManager_Release(t *testing.T) {
	t.Run("proper release", func(t *testing.T) {
		rm := NewFixedResourceManager(3)
		ctx := context.Background()

		// Acquire and release multiple scans
		scanIDs := []string{"scan-1", "scan-2", "scan-3"}

		for _, id := range scanIDs {
			err := rm.Acquire(ctx, id)
			if err != nil {
				t.Fatalf("Failed to acquire resource for %s: %v", id, err)
			}
		}

		if rm.GetActiveScans() != 3 {
			t.Errorf("Expected 3 active scans, got %d", rm.GetActiveScans())
		}

		// Release all scans
		for _, id := range scanIDs {
			rm.Release(id)
		}

		if rm.GetActiveScans() != 0 {
			t.Errorf("Expected 0 active scans after release, got %d", rm.GetActiveScans())
		}

		if rm.GetAvailableSlots() != 3 {
			t.Errorf("Expected 3 available slots, got %d", rm.GetAvailableSlots())
		}
	})

	t.Run("release non-existent scan", func(t *testing.T) {
		rm := NewFixedResourceManager(2)

		// This should not panic or cause issues
		rm.Release("non-existent-scan")

		if rm.GetActiveScans() != 0 {
			t.Errorf("Expected 0 active scans, got %d", rm.GetActiveScans())
		}
	})
}

func TestFixedResourceManager_ConcurrentAccess(t *testing.T) {
	t.Run("concurrent acquire and release", func(t *testing.T) {
		rm := NewFixedResourceManager(10)
		ctx := context.Background()

		const numGoroutines = 50
		const scansPerGoroutine = 5

		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines*scansPerGoroutine)

		// Start multiple goroutines doing concurrent operations
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()

				for j := 0; j < scansPerGoroutine; j++ {
					scanID := fmt.Sprintf("worker-%d-scan-%d", workerID, j)

					// Acquire
					if err := rm.Acquire(ctx, scanID); err != nil {
						errors <- err
						return
					}

					// Simulate some work
					time.Sleep(time.Millisecond)

					// Release
					rm.Release(scanID)
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		// Check for errors
		for err := range errors {
			t.Errorf("Concurrent operation failed: %v", err)
		}

		// Verify final state
		if rm.GetActiveScans() != 0 {
			t.Errorf("Expected 0 active scans after completion, got %d", rm.GetActiveScans())
		}

		if rm.GetAvailableSlots() != 10 {
			t.Errorf("Expected 10 available slots, got %d", rm.GetAvailableSlots())
		}
	})
}

func TestFixedResourceManager_IsHealthy(t *testing.T) {
	t.Run("healthy state", func(t *testing.T) {
		rm := NewFixedResourceManager(5)

		if !rm.IsHealthy() {
			t.Error("Expected healthy state with no active scans")
		}

		// Add some recent scans
		ctx := context.Background()
		rm.Acquire(ctx, "recent-scan-1")
		rm.Acquire(ctx, "recent-scan-2")

		if !rm.IsHealthy() {
			t.Error("Expected healthy state with recent scans")
		}

		rm.Release("recent-scan-1")
		rm.Release("recent-scan-2")
	})

	t.Run("unhealthy state with hung scans", func(t *testing.T) {
		rm := NewFixedResourceManager(4)

		// Manually simulate hung scans by acquiring resources without releasing them
		ctx := context.Background()
		for i := 0; i < 4; i++ {
			rm.Acquire(ctx, fmt.Sprintf("hung-scan-%d", i))
		}

		// All resources are now consumed, system should still be healthy initially
		if !rm.IsHealthy() {
			t.Error("Expected healthy state even with all resources consumed")
		}

		// Try to acquire one more (should fail due to no available resources)
		done := make(chan bool)
		go func() {
			rm.Acquire(ctx, "blocked-scan")
			done <- true
		}()

		select {
		case <-done:
			t.Error("Expected acquisition to block when no resources available")
		case <-time.After(100 * time.Millisecond):
			// Expected behavior - acquisition should block
		}

		// Clean up
		for i := 0; i < 4; i++ {
			rm.Release(fmt.Sprintf("hung-scan-%d", i))
		}
	})
}
