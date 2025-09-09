package scanning

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	// maxScanDurationMinutes defines the maximum allowed scan duration before considering it potentially hung.
	maxScanDurationMinutes = 30
)

// ResourceManager manages scan resources and concurrency limits.
type ResourceManager interface {
	// Acquire attempts to acquire a resource slot for the given scan ID.
	// It blocks until a slot is available or the context is cancelled.
	Acquire(ctx context.Context, scanID string) error

	// Release releases the resource slot for the given scan ID.
	Release(scanID string)

	// GetActiveScans returns the current number of active scans.
	GetActiveScans() int

	// GetAvailableSlots returns the number of available resource slots.
	GetAvailableSlots() int

	// IsHealthy returns true if the resource manager is operating normally.
	IsHealthy() bool

	// Close gracefully shuts down the resource manager.
	Close() error
}

// FixedResourceManager implements ResourceManager with a fixed number of resource slots.
type FixedResourceManager struct {
	capacity    int
	semaphore   chan struct{}
	activeScans map[string]time.Time
	mutex       sync.RWMutex
	closed      bool
}

// NewFixedResourceManager creates a new resource manager with the specified capacity.
func NewFixedResourceManager(capacity int) *FixedResourceManager {
	if capacity <= 0 {
		capacity = 1
	}

	return &FixedResourceManager{
		capacity:    capacity,
		semaphore:   make(chan struct{}, capacity),
		activeScans: make(map[string]time.Time),
	}
}

// Acquire attempts to acquire a resource slot for the given scan ID.
func (rm *FixedResourceManager) Acquire(ctx context.Context, scanID string) error {
	rm.mutex.Lock()
	if rm.closed {
		rm.mutex.Unlock()
		return fmt.Errorf("resource manager is closed")
	}
	rm.mutex.Unlock()

	select {
	case rm.semaphore <- struct{}{}:
		rm.mutex.Lock()
		rm.activeScans[scanID] = time.Now()
		rm.mutex.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release releases the resource slot for the given scan ID.
func (rm *FixedResourceManager) Release(scanID string) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if _, exists := rm.activeScans[scanID]; exists {
		delete(rm.activeScans, scanID)

		// Try to release from semaphore (non-blocking)
		select {
		case <-rm.semaphore:
		default:
			// Semaphore was already empty, this shouldn't happen but we handle it gracefully
		}
	}
}

// GetActiveScans returns the current number of active scans.
func (rm *FixedResourceManager) GetActiveScans() int {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	return len(rm.activeScans)
}

// GetAvailableSlots returns the number of available resource slots.
func (rm *FixedResourceManager) GetAvailableSlots() int {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	return rm.capacity - len(rm.activeScans)
}

// IsHealthy returns true if the resource manager is operating normally.
func (rm *FixedResourceManager) IsHealthy() bool {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	if rm.closed {
		return false
	}

	// Check if there are any scans that have been running for too long
	now := time.Now()
	maxScanDuration := maxScanDurationMinutes * time.Minute // Configurable threshold

	for _, startTime := range rm.activeScans {
		if now.Sub(startTime) > maxScanDuration {
			// There's a potentially hung scan, but we still consider the manager healthy
			// as this might be a legitimate long-running scan
			continue
		}
	}

	return true
}

// Close gracefully shuts down the resource manager.
func (rm *FixedResourceManager) Close() error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	if rm.closed {
		return nil
	}

	rm.closed = true

	// Release all active scans
	rm.activeScans = make(map[string]time.Time)

	// Drain the semaphore
	for {
		select {
		case <-rm.semaphore:
		default:
			return nil
		}
	}
}

// GetStats returns statistics about the resource manager.
func (rm *FixedResourceManager) GetStats() map[string]interface{} {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	return map[string]interface{}{
		"capacity":        rm.capacity,
		"active_scans":    len(rm.activeScans),
		"available_slots": rm.capacity - len(rm.activeScans),
		"is_healthy":      rm.IsHealthy(),
		"closed":          rm.closed,
	}
}
