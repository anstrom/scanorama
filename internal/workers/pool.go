// Package workers provides a worker pool implementation for concurrent operations
// in scanorama. It supports job queuing, rate limiting, graceful shutdown,
// and integrates with the structured logging and metrics systems.
package workers

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anstrom/scanorama/internal/logging"
	"github.com/anstrom/scanorama/internal/metrics"
)

// Job represents a unit of work to be executed by a worker.
type Job interface {
	// Execute performs the job and returns an error if it fails.
	Execute(ctx context.Context) error
	// ID returns a unique identifier for the job.
	ID() string
	// Type returns the job type for metrics and logging.
	Type() string
}

// Result represents the result of executing a job.
type Result struct {
	JobID    string
	JobType  string
	Error    error
	Duration time.Duration
	Retries  int
}

// Config holds configuration for the worker pool.
type Config struct {
	// Size is the number of worker goroutines to create.
	Size int
	// QueueSize is the maximum number of jobs that can be queued.
	QueueSize int
	// MaxRetries is the maximum number of retries for failed jobs.
	MaxRetries int
	// RetryDelay is the delay between retries.
	RetryDelay time.Duration
	// ShutdownTimeout is the maximum time to wait for workers to finish.
	ShutdownTimeout time.Duration
	// RateLimit is the maximum number of jobs per second (0 = no limit).
	RateLimit int
}

// DefaultConfig returns a default worker pool configuration.
func DefaultConfig() Config {
	return Config{
		Size:            10,
		QueueSize:       100,
		MaxRetries:      3,
		RetryDelay:      time.Second,
		ShutdownTimeout: 30 * time.Second,
		RateLimit:       0,
	}
}

// Pool manages a pool of worker goroutines for concurrent job execution.
type Pool struct {
	config          Config
	jobs            chan Job
	results         chan Result
	externalResults chan Result
	workers         []*worker
	wg              sync.WaitGroup
	ctx             context.Context
	cancel          context.CancelFunc
	shutdown        chan struct{}
	done            chan struct{}
	rateLimiter     *time.Ticker
	startOnce       sync.Once
	closeOnce       sync.Once
	shutdown32      int32 // atomic shutdown flag
}

// worker represents a single worker goroutine.
type worker struct {
	id   int
	pool *Pool
}

// New creates a new worker pool with the given configuration.
func New(config Config) *Pool {
	ctx, cancel := context.WithCancel(context.Background())

	pool := &Pool{
		config:          config,
		jobs:            make(chan Job, config.QueueSize),
		results:         make(chan Result, config.QueueSize),
		externalResults: make(chan Result, config.QueueSize),
		workers:         make([]*worker, config.Size),
		ctx:             ctx,
		cancel:          cancel,
		shutdown:        make(chan struct{}),
		done:            make(chan struct{}),
	}

	// Set up rate limiter if configured
	if config.RateLimit > 0 {
		interval := time.Second / time.Duration(config.RateLimit)
		pool.rateLimiter = time.NewTicker(interval)
	}

	// Create workers
	for i := 0; i < config.Size; i++ {
		pool.workers[i] = &worker{
			id:   i,
			pool: pool,
		}
	}

	return pool
}

// Start begins the worker pool operations.
func (p *Pool) Start() {
	p.startOnce.Do(func() {
		logging.Info("Starting worker pool",
			"worker_count", p.config.Size,
			"queue_size", p.config.QueueSize,
			"rate_limit", p.config.RateLimit)

		// Start workers
		for _, w := range p.workers {
			p.wg.Add(1)
			go w.run()
		}

		// Start result processor
		go p.processResults()

		metrics.Gauge("worker_pool_size", float64(p.config.Size), metrics.Labels{
			"component": "workers",
		})
	})
}

// Submit adds a job to the worker pool queue.
func (p *Pool) Submit(job Job) error {
	// Check if pool is shut down
	if atomic.LoadInt32(&p.shutdown32) == 1 {
		return fmt.Errorf("worker pool is shut down")
	}

	select {
	case p.jobs <- job:
		logging.Debug("Job submitted to worker pool",
			"job_id", job.ID(),
			"job_type", job.Type())
		metrics.Counter("jobs_submitted_total", metrics.Labels{
			"job_type": job.Type(),
		})
		return nil
	case <-p.ctx.Done():
		return fmt.Errorf("worker pool is shutting down")
	default:
		return fmt.Errorf("job queue is full")
	}
}

// Results returns a channel for receiving job results.
func (p *Pool) Results() <-chan Result {
	return p.externalResults
}

// Shutdown gracefully shuts down the worker pool.
func (p *Pool) Shutdown() error {
	// Set shutdown flag atomically
	if !atomic.CompareAndSwapInt32(&p.shutdown32, 0, 1) {
		// Already shut down
		return nil
	}

	logging.Info("Shutting down worker pool")

	// Cancel context first to prevent new submissions
	p.cancel()

	// Signal shutdown
	close(p.shutdown)

	// Close job queue
	close(p.jobs)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logging.Info("Worker pool shutdown completed")
	case <-time.After(p.config.ShutdownTimeout):
		logging.Warn("Worker pool shutdown timeout, forcing termination")
		<-done
	}

	// Cancel context to signal processResults to exit
	p.cancel()

	// Give processResults a moment to exit cleanly
	time.Sleep(10 * time.Millisecond)

	// Close results channels
	close(p.results)
	close(p.externalResults)

	// Stop rate limiter
	if p.rateLimiter != nil {
		p.rateLimiter.Stop()
	}

	return nil
}

// Wait waits for all workers to complete and the pool to shut down.
func (p *Pool) Wait() {
	<-p.done
}

// worker.run executes the worker loop.
func (w *worker) run() {
	defer w.pool.wg.Done()

	logging.Debug("Worker started", "worker_id", w.id)
	defer logging.Debug("Worker stopped", "worker_id", w.id)

	for {
		select {
		case job, ok := <-w.pool.jobs:
			if !ok {
				// Job channel closed, worker should exit
				return
			}
			w.executeJob(job)

		case <-w.pool.shutdown:
			return

		case <-w.pool.ctx.Done():
			return
		}
	}
}

// executeJob executes a single job with retry logic.
func (w *worker) executeJob(job Job) {
	jobTimer := metrics.NewTimer("job_duration_seconds", metrics.Labels{
		"job_type":  job.Type(),
		"worker_id": fmt.Sprintf("worker-%d", w.id),
	})
	defer jobTimer.Stop()

	// Apply rate limiting if configured
	if w.pool.rateLimiter != nil {
		select {
		case <-w.pool.rateLimiter.C:
			// Rate limit satisfied, proceed
		case <-w.pool.ctx.Done():
			return
		}
	}

	var lastErr error
	var retries int

	for attempt := 0; attempt <= w.pool.config.MaxRetries; attempt++ {
		start := time.Now()

		// Create job context with timeout
		jobCtx, cancel := context.WithCancel(w.pool.ctx)

		// Execute the job
		err := job.Execute(jobCtx)
		cancel()

		duration := time.Since(start)

		if err == nil {
			// Job succeeded
			w.pool.results <- Result{
				JobID:    job.ID(),
				JobType:  job.Type(),
				Duration: duration,
				Retries:  retries,
			}

			metrics.Counter("jobs_completed_total", metrics.Labels{
				"job_type": job.Type(),
				"status":   "success",
			})

			logging.Debug("Job completed successfully",
				"job_id", job.ID(),
				"job_type", job.Type(),
				"duration", duration,
				"worker_id", w.id,
				"retries", retries)
			return
		}

		lastErr = err
		retries = attempt

		// Check if we should retry
		if attempt < w.pool.config.MaxRetries {
			logging.Debug("Job failed, retrying",
				"job_id", job.ID(),
				"job_type", job.Type(),
				"attempt", attempt+1,
				"max_retries", w.pool.config.MaxRetries,
				"error", err)

			// Wait before retry
			select {
			case <-time.After(w.pool.config.RetryDelay):
			case <-w.pool.ctx.Done():
				return
			}
		}
	}

	// Job failed after all retries
	w.pool.results <- Result{
		JobID:   job.ID(),
		JobType: job.Type(),
		Error:   lastErr,
		Retries: retries,
	}

	metrics.Counter("jobs_completed_total", metrics.Labels{
		"job_type": job.Type(),
		"status":   "error",
	})

	logging.Error("Job failed after retries",
		"job_id", job.ID(),
		"job_type", job.Type(),
		"retries", retries,
		"error", lastErr,
		"worker_id", w.id)
}

// processResults processes job results from workers.
func (p *Pool) processResults() {
	defer p.closeOnce.Do(func() {
		close(p.done)
	})

	for {
		select {
		case result, ok := <-p.results:
			if !ok {
				return
			}

			// Fan out result to external consumers
			select {
			case p.externalResults <- result:
			case <-p.ctx.Done():
				return
			default:
				// External consumer not reading, continue with metrics
			}

			// Update metrics based on result
			if result.Error != nil {
				metrics.Counter("job_errors_total", metrics.Labels{
					"job_type": result.JobType,
				})
			}

			metrics.Histogram("job_retry_count", float64(result.Retries), metrics.Labels{
				"job_type": result.JobType,
			})

		case <-p.ctx.Done():
			return
		}
	}
}

// ScanJob implements Job interface for scan operations.
type ScanJob struct {
	id       string
	target   string
	ports    string
	scanType string
	executor func(ctx context.Context, target, ports, scanType string) error
}

// NewScanJob creates a new scan job.
func NewScanJob(id, target, ports, scanType string,
	executor func(ctx context.Context, target, ports, scanType string) error) *ScanJob {
	return &ScanJob{
		id:       id,
		target:   target,
		ports:    ports,
		scanType: scanType,
		executor: executor,
	}
}

// Execute implements the Job interface.
func (j *ScanJob) Execute(ctx context.Context) error {
	return j.executor(ctx, j.target, j.ports, j.scanType)
}

// ID implements the Job interface.
func (j *ScanJob) ID() string {
	return j.id
}

// Type implements the Job interface.
func (j *ScanJob) Type() string {
	return "scan"
}

// DiscoveryJob implements Job interface for discovery operations.
type DiscoveryJob struct {
	id       string
	network  string
	method   string
	executor func(ctx context.Context, network, method string) error
}

// NewDiscoveryJob creates a new discovery job.
func NewDiscoveryJob(id, network, method string,
	executor func(ctx context.Context, network, method string) error) *DiscoveryJob {
	return &DiscoveryJob{
		id:       id,
		network:  network,
		method:   method,
		executor: executor,
	}
}

// Execute implements the Job interface.
func (j *DiscoveryJob) Execute(ctx context.Context) error {
	return j.executor(ctx, j.network, j.method)
}

// ID implements the Job interface.
func (j *DiscoveryJob) ID() string {
	return j.id
}

// Type implements the Job interface.
func (j *DiscoveryJob) Type() string {
	return "discovery"
}
