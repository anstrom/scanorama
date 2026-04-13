// Package scanning provides core scanning functionality and shared types for scanorama.
// This file implements a bounded job execution queue that prevents resource
// exhaustion when many scans or discovery jobs are requested concurrently.
// Jobs are enqueued into a FIFO buffer and executed by a fixed-size worker pool.
package scanning

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anstrom/scanorama/internal/logging"
	"github.com/anstrom/scanorama/internal/metrics"
)

// Default queue configuration values.
const (
	DefaultMaxConcurrent = 5
	DefaultMaxQueueSize  = 50
)

// Worker status values.
const (
	workerStateIdle   = "idle"
	workerStateActive = "active"
)

// Sentinel errors for queue operations.
var (
	ErrQueueFull   = errors.New("scan queue is full")
	ErrQueueClosed = errors.New("scan queue is closed")
)

// QueueStats provides a snapshot of the current queue state.
type QueueStats struct {
	// QueueDepth is the number of scan requests currently waiting in the queue.
	QueueDepth int
	// ActiveScans is the number of scans currently being executed by workers.
	ActiveScans int
	// MaxConcurrent is the maximum number of simultaneous scans.
	MaxConcurrent int
	// MaxQueueSize is the maximum number of pending scan requests.
	MaxQueueSize int
	// TotalSubmitted is the total number of scan requests submitted.
	TotalSubmitted int64
	// TotalCompleted is the total number of scans completed successfully.
	TotalCompleted int64
	// TotalRejected is the total number of scan requests rejected (queue full or closed).
	TotalRejected int64
	// TotalFailed is the total number of scans that failed with an error.
	TotalFailed int64
}

// workerState tracks the live state of a single worker goroutine.
// All fields except workerStartedAt are guarded by the embedded mutex.
type workerState struct {
	mu              sync.RWMutex
	status          string // "idle" or "active"
	jobID           string
	jobType         string
	jobTarget       string
	jobStartedAt    time.Time
	workerStartedAt time.Time
	lastJobAt       time.Time
	jobsDone        int64
	jobsFailed      int64
}

// WorkerSnapshot is a point-in-time, read-only view of a single worker's state.
type WorkerSnapshot struct {
	// ID is the worker identifier (e.g. "worker-0").
	ID string
	// Status is "idle" or "active".
	Status string
	// JobID is the ID of the job currently being processed; empty when idle.
	JobID string
	// JobType is the type of the current job (e.g. "scan"); empty when idle.
	JobType string
	// JobTarget is the scan target of the current job; empty when idle.
	JobTarget string
	// JobStartedAt is when the current job started; nil when idle.
	JobStartedAt *time.Time
	// WorkerStartedAt is when this worker goroutine was started.
	WorkerStartedAt time.Time
	// LastJobAt is when this worker last completed a job; zero if never.
	LastJobAt time.Time
	// JobsDone is the number of jobs this worker has completed successfully.
	JobsDone int64
	// JobsFailed is the number of jobs this worker has failed.
	JobsFailed int64
}

// ScanQueue manages a bounded worker pool for job execution.
// It uses a buffered channel as a FIFO queue and spawns a configurable
// number of worker goroutines to process jobs concurrently.
// Both scan and discovery jobs implement the Job interface and can be submitted.
type ScanQueue struct {
	maxConcurrent int
	maxQueueSize  int

	queue  chan Job
	cancel context.CancelFunc

	// wg tracks active worker goroutines for graceful shutdown.
	wg sync.WaitGroup

	// closed indicates whether the queue has been shut down.
	closed atomic.Bool

	// cancelMu guards the cancel field against concurrent Start/Stop calls.
	cancelMu sync.Mutex
	// workerStates holds per-worker live state for admin introspection.
	workerStates []*workerState

	// Atomic counters for thread-safe statistics.
	activeScans    atomic.Int64
	totalSubmitted atomic.Int64
	totalCompleted atomic.Int64
	totalRejected  atomic.Int64
	totalFailed    atomic.Int64
}

// NewScanQueue creates a new ScanQueue with the specified concurrency and queue size limits.
// If maxConcurrent <= 0, DefaultMaxConcurrent is used.
// If maxQueueSize <= 0, DefaultMaxQueueSize is used.
func NewScanQueue(maxConcurrent, maxQueueSize int) *ScanQueue {
	if maxConcurrent <= 0 {
		maxConcurrent = DefaultMaxConcurrent
	}
	if maxQueueSize <= 0 {
		maxQueueSize = DefaultMaxQueueSize
	}

	states := make([]*workerState, maxConcurrent)
	for i := range states {
		states[i] = &workerState{status: workerStateIdle}
	}

	return &ScanQueue{
		maxConcurrent: maxConcurrent,
		maxQueueSize:  maxQueueSize,
		queue:         make(chan Job, maxQueueSize),
		workerStates:  states,
	}
}

// Start launches the worker goroutines that process scan requests from the queue.
// The provided context controls the lifecycle of all workers. When the context
// is cancelled, workers will finish their current scan and exit.
// Start must only be called once.
func (q *ScanQueue) Start(ctx context.Context) {
	workerCtx, cancel := context.WithCancel(ctx)
	q.cancelMu.Lock()
	q.cancel = cancel
	q.cancelMu.Unlock()

	logging.Info("Starting scan queue",
		"max_concurrent", q.maxConcurrent,
		"max_queue_size", q.maxQueueSize)

	// Report capacity to Prometheus once at startup.
	m := metrics.GetGlobalMetrics()
	if m != nil {
		m.SetScanQueueCapacity(float64(q.maxQueueSize))
	}

	startedAt := time.Now()
	for i := 0; i < q.maxConcurrent; i++ {
		q.workerStates[i].workerStartedAt = startedAt
		q.wg.Add(1)
		go q.worker(workerCtx, i)
	}
}

// Stop performs a graceful shutdown of the scan queue.
// It marks the queue as closed (rejecting new submissions), cancels the worker
// context, and waits for all in-flight scans to complete.
// Stop is safe to call multiple times; subsequent calls are no-ops.
func (q *ScanQueue) Stop() {
	if q.closed.Swap(true) {
		// Already closed.
		return
	}

	logging.Info("Stopping scan queue, waiting for in-flight scans to complete")

	// Cancel the worker context so workers stop picking up new items
	// after finishing their current scan.
	q.cancelMu.Lock()
	cancelFn := q.cancel
	q.cancelMu.Unlock()
	if cancelFn != nil {
		cancelFn()
	}

	// Close the channel so workers drain remaining items and exit.
	close(q.queue)

	// Wait for all workers to finish.
	q.wg.Wait()

	// Update metrics to reflect zero active scans.
	q.updateMetrics()

	logging.Info("Scan queue stopped",
		"total_submitted", q.totalSubmitted.Load(),
		"total_completed", q.totalCompleted.Load(),
		"total_failed", q.totalFailed.Load(),
		"total_rejected", q.totalRejected.Load())
}

// Submit enqueues a scan request for execution by the worker pool.
// It returns ErrQueueClosed if the queue has been shut down, or ErrQueueFull
// if the queue is at capacity. Submit is non-blocking.
// Submit enqueues a job for execution by the worker pool.
// It returns ErrQueueClosed if the queue has been shut down, or ErrQueueFull
// if the queue is at capacity. Submit is non-blocking.
func (q *ScanQueue) Submit(job Job) error {
	if q.closed.Load() {
		q.totalRejected.Add(1)
		metrics.IncrementScansRejectedPrometheus("queue_closed")
		return ErrQueueClosed
	}

	if job == nil {
		return fmt.Errorf("job must not be nil")
	}

	select {
	case q.queue <- job:
		q.totalSubmitted.Add(1)
		q.updateQueueDepthMetric()
		logging.Debug("Job submitted to queue",
			"job_id", job.ID(),
			"job_type", job.Type(),
			"queue_depth", len(q.queue))
		return nil
	default:
		q.totalRejected.Add(1)
		metrics.IncrementScansRejectedPrometheus("queue_full")
		logging.Warn("Job queue is full, rejecting request",
			"job_id", job.ID(),
			"job_type", job.Type(),
			"queue_depth", len(q.queue),
			"max_queue_size", q.maxQueueSize)
		return ErrQueueFull
	}
}

// Stats returns a snapshot of the current queue statistics.
// All counters are read atomically and are safe for concurrent access.
func (q *ScanQueue) Stats() QueueStats {
	return QueueStats{
		QueueDepth:     len(q.queue),
		ActiveScans:    int(q.activeScans.Load()),
		MaxConcurrent:  q.maxConcurrent,
		MaxQueueSize:   q.maxQueueSize,
		TotalSubmitted: q.totalSubmitted.Load(),
		TotalCompleted: q.totalCompleted.Load(),
		TotalRejected:  q.totalRejected.Load(),
		TotalFailed:    q.totalFailed.Load(),
	}
}

// Snapshot returns a point-in-time view of all worker states.
// It is safe to call concurrently with ongoing scan execution.
func (q *ScanQueue) Snapshot() []WorkerSnapshot {
	snaps := make([]WorkerSnapshot, len(q.workerStates))
	for i, ws := range q.workerStates {
		ws.mu.RLock()
		snap := WorkerSnapshot{
			ID:              fmt.Sprintf("worker-%d", i),
			Status:          ws.status,
			JobID:           ws.jobID,
			JobType:         ws.jobType,
			JobTarget:       ws.jobTarget,
			WorkerStartedAt: ws.workerStartedAt,
			LastJobAt:       ws.lastJobAt,
			JobsDone:        ws.jobsDone,
			JobsFailed:      ws.jobsFailed,
		}
		if !ws.jobStartedAt.IsZero() {
			t := ws.jobStartedAt
			snap.JobStartedAt = &t
		}
		ws.mu.RUnlock()
		snaps[i] = snap
	}
	return snaps
}

// worker is the main loop for a single worker goroutine.
// It pulls jobs from the queue channel and executes them.
func (q *ScanQueue) worker(ctx context.Context, workerID int) {
	defer q.wg.Done()

	logging.Debug("Queue worker started", "worker_id", workerID)

	for job := range q.queue {
		q.updateQueueDepthMetric()

		// Check if context is cancelled before starting a new job.
		select {
		case <-ctx.Done():
			logging.Debug("Worker context cancelled, discarding job",
				"worker_id", workerID,
				"job_id", job.ID(),
				"job_type", job.Type())
			continue
		default:
		}

		q.executeJob(ctx, workerID, job)
	}

	logging.Debug("Queue worker stopped", "worker_id", workerID)
}

// executeJob runs a single job and handles metrics, logging, and worker state.
func (q *ScanQueue) executeJob(ctx context.Context, workerID int, job Job) {
	// Mark worker as active with current job details before touching metrics.
	jobStartedAt := time.Now()
	ws := q.workerStates[workerID]
	ws.mu.Lock()
	ws.status = workerStateActive
	ws.jobID = job.ID()
	ws.jobType = job.Type()
	ws.jobTarget = job.Target()
	ws.jobStartedAt = jobStartedAt
	ws.mu.Unlock()

	q.activeScans.Add(1)
	q.updateMetrics()

	logging.Info("Job starting",
		"worker_id", workerID,
		"job_id", job.ID(),
		"job_type", job.Type(),
		"job_target", job.Target(),
		"active_jobs", q.activeScans.Load(),
		"queue_depth", len(q.queue))

	start := time.Now()
	err := job.Execute(ctx)
	duration := time.Since(start)

	q.activeScans.Add(-1)

	// Return worker to idle and update per-worker counters.
	completedAt := time.Now()
	ws.mu.Lock()
	ws.status = workerStateIdle
	ws.jobID = ""
	ws.jobType = ""
	ws.jobTarget = ""
	ws.jobStartedAt = time.Time{}
	ws.lastJobAt = completedAt
	if err != nil {
		ws.jobsFailed++
	} else {
		ws.jobsDone++
	}
	ws.mu.Unlock()

	if err != nil {
		q.totalFailed.Add(1)
		logging.Error("Job failed",
			"worker_id", workerID,
			"job_id", job.ID(),
			"job_type", job.Type(),
			"duration", duration,
			"error", err)
	} else {
		q.totalCompleted.Add(1)
		logging.Info("Job completed",
			"worker_id", workerID,
			"job_id", job.ID(),
			"job_type", job.Type(),
			"duration", duration)
	}

	q.updateMetrics()
}

// updateMetrics pushes current active-scan count to Prometheus metrics.
func (q *ScanQueue) updateMetrics() {
	m := metrics.GetGlobalMetrics()
	if m != nil {
		m.SetActiveScans(int(q.activeScans.Load()))
	}
	q.updateQueueDepthMetric()
}

// updateQueueDepthMetric pushes the current queue depth to Prometheus.
func (q *ScanQueue) updateQueueDepthMetric() {
	m := metrics.GetGlobalMetrics()
	if m != nil {
		m.SetScanQueueDepth(float64(len(q.queue)))
	}
}
