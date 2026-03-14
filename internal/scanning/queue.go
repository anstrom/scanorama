// Package scanning provides core scanning functionality and shared types for scanorama.
// This file implements a bounded scan execution queue that prevents resource
// exhaustion when many scans are requested concurrently. Scan requests are
// enqueued into a FIFO buffer and executed by a fixed-size worker pool.
package scanning

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/logging"
	"github.com/anstrom/scanorama/internal/metrics"
)

// Default queue configuration values.
const (
	DefaultMaxConcurrent = 5
	DefaultMaxQueueSize  = 50
)

// Sentinel errors for queue operations.
var (
	ErrQueueFull   = errors.New("scan queue is full")
	ErrQueueClosed = errors.New("scan queue is closed")
)

// ScanQueueRequest represents a scan job to be enqueued for execution.
type ScanQueueRequest struct {
	// ID is a unique identifier for this scan request.
	ID string
	// Config is the scan configuration to execute.
	Config *ScanConfig
	// Database is the database connection for storing results.
	Database *db.DB
	// ResultCh is an optional channel where the caller receives the result.
	// If nil, the result is discarded after logging.
	ResultCh chan<- *ScanQueueResult
}

// ScanQueueResult contains the outcome of a queued scan execution.
type ScanQueueResult struct {
	// ID is the scan request identifier.
	ID string
	// Result is the scan result, nil if an error occurred.
	Result *ScanResult
	// Error is any error that occurred during scan execution.
	Error error
	// Duration is how long the scan took to execute.
	Duration time.Duration
}

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

// ScanQueue manages a bounded worker pool for scan execution.
// It uses a buffered channel as a FIFO queue and spawns a configurable
// number of worker goroutines to process scan requests concurrently.
type ScanQueue struct {
	maxConcurrent int
	maxQueueSize  int

	queue  chan *ScanQueueRequest
	cancel context.CancelFunc

	// wg tracks active worker goroutines for graceful shutdown.
	wg sync.WaitGroup

	// closed indicates whether the queue has been shut down.
	closed atomic.Bool

	// scanFunc is the function workers call to execute a scan.
	// When nil (the default), defaultScanFunc is used.
	scanFunc func(context.Context, *ScanQueueRequest) *ScanQueueResult

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

	return &ScanQueue{
		maxConcurrent: maxConcurrent,
		maxQueueSize:  maxQueueSize,
		queue:         make(chan *ScanQueueRequest, maxQueueSize),
	}
}

// SetScanFunc overrides the function that workers use to execute scans.
// This is primarily intended for testing so that unit tests can substitute
// a lightweight implementation that does not invoke nmap.
// It must be called before Start.
func (q *ScanQueue) SetScanFunc(fn func(context.Context, *ScanQueueRequest) *ScanQueueResult) {
	q.scanFunc = fn
}

// Start launches the worker goroutines that process scan requests from the queue.
// The provided context controls the lifecycle of all workers. When the context
// is cancelled, workers will finish their current scan and exit.
// Start must only be called once.
func (q *ScanQueue) Start(ctx context.Context) {
	workerCtx, cancel := context.WithCancel(ctx)
	q.cancel = cancel

	logging.Info("Starting scan queue",
		"max_concurrent", q.maxConcurrent,
		"max_queue_size", q.maxQueueSize)

	// Report capacity to Prometheus once at startup.
	m := metrics.GetGlobalMetrics()
	if m != nil {
		m.SetScanQueueCapacity(float64(q.maxQueueSize))
	}

	for i := 0; i < q.maxConcurrent; i++ {
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
	if q.cancel != nil {
		q.cancel()
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
func (q *ScanQueue) Submit(req *ScanQueueRequest) error {
	if q.closed.Load() {
		q.totalRejected.Add(1)
		metrics.IncrementScansRejectedPrometheus("queue_closed")
		return ErrQueueClosed
	}

	if req == nil {
		return fmt.Errorf("scan request must not be nil")
	}

	select {
	case q.queue <- req:
		q.totalSubmitted.Add(1)
		q.updateQueueDepthMetric()
		logging.Debug("Scan request submitted to queue",
			"request_id", req.ID,
			"queue_depth", len(q.queue))
		return nil
	default:
		q.totalRejected.Add(1)
		metrics.IncrementScansRejectedPrometheus("queue_full")
		logging.Warn("Scan queue is full, rejecting request",
			"request_id", req.ID,
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

// worker is the main loop for a single worker goroutine.
// It pulls scan requests from the queue channel and executes them.
func (q *ScanQueue) worker(ctx context.Context, workerID int) {
	defer q.wg.Done()

	logging.Debug("Scan queue worker started", "worker_id", workerID)

	for req := range q.queue {
		q.updateQueueDepthMetric()

		// Check if context is cancelled before starting a new scan.
		select {
		case <-ctx.Done():
			// Context cancelled but we still have a request we pulled from the channel.
			// Send back an error result so the caller isn't left hanging.
			q.sendResult(req, &ScanQueueResult{
				ID:    req.ID,
				Error: fmt.Errorf("scan queue shutting down: %w", ctx.Err()),
			})
			logging.Debug("Worker context cancelled, discarding request",
				"worker_id", workerID,
				"request_id", req.ID)
			continue
		default:
		}

		q.executeScan(ctx, workerID, req)
	}

	logging.Debug("Scan queue worker stopped", "worker_id", workerID)
}

// executeScan runs a single scan request and handles metrics, logging, and result delivery.
func (q *ScanQueue) executeScan(ctx context.Context, workerID int, req *ScanQueueRequest) {
	q.activeScans.Add(1)
	q.updateMetrics()

	logging.Info("Scan starting",
		"worker_id", workerID,
		"request_id", req.ID,
		"active_scans", q.activeScans.Load(),
		"queue_depth", len(q.queue))

	start := time.Now()

	// Use the injected scan function if available; otherwise fall back to
	// the real RunScanWithContext implementation.
	var queueResult *ScanQueueResult
	if q.scanFunc != nil {
		queueResult = q.scanFunc(ctx, req)
		if queueResult.Duration == 0 {
			queueResult.Duration = time.Since(start)
		}
	} else {
		queueResult = defaultScanFunc(ctx, req)
	}

	duration := time.Since(start)
	if queueResult.Duration == 0 {
		queueResult.Duration = duration
	}

	q.activeScans.Add(-1)

	if queueResult.Error != nil {
		q.totalFailed.Add(1)
		logging.Error("Scan failed",
			"worker_id", workerID,
			"request_id", req.ID,
			"duration", queueResult.Duration,
			"error", queueResult.Error)
	} else {
		q.totalCompleted.Add(1)
		hostCount := 0
		if queueResult.Result != nil {
			hostCount = len(queueResult.Result.Hosts)
		}
		logging.Info("Scan completed",
			"worker_id", workerID,
			"request_id", req.ID,
			"duration", queueResult.Duration,
			"hosts_found", hostCount)
	}

	q.updateMetrics()
	q.sendResult(req, queueResult)
}

// defaultScanFunc is the production scan function that delegates to RunScanWithContext.
func defaultScanFunc(ctx context.Context, req *ScanQueueRequest) *ScanQueueResult {
	start := time.Now()
	result, err := RunScanWithContext(ctx, req.Config, req.Database)
	return &ScanQueueResult{
		ID:       req.ID,
		Result:   result,
		Error:    err,
		Duration: time.Since(start),
	}
}

// sendResult delivers a scan result back to the caller via the ResultCh channel.
// If ResultCh is nil, the result is silently discarded.
func (q *ScanQueue) sendResult(req *ScanQueueRequest, result *ScanQueueResult) {
	if req.ResultCh == nil {
		return
	}

	// Non-blocking send to avoid worker deadlock if the caller isn't reading.
	select {
	case req.ResultCh <- result:
	default:
		logging.Warn("Failed to deliver scan result, channel full or not ready",
			"request_id", req.ID)
	}
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
