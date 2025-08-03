package worker

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal"
	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
)

// Job represents a scanning job to be executed
type Job struct {
	ID       uuid.UUID
	TargetID uuid.UUID
	Target   *db.ScanTarget
	Priority int
	Retries  int
	Created  time.Time
}

// Result represents the result of a completed job
type Result struct {
	Job    *Job
	Result *internal.ScanResult
	Error  error
	Worker int
}

// Worker represents a single worker in the pool
type Worker struct {
	ID      int
	pool    *Pool
	jobChan chan *Job
	quit    chan bool
	logger  *log.Logger
}

// Pool manages a pool of workers for concurrent scanning
type Pool struct {
	// Configuration
	config *config.Config
	db     *db.DB
	logger *log.Logger

	// Worker management
	workers    []*Worker
	workerWg   sync.WaitGroup
	jobQueue   chan *Job
	resultChan chan *Result

	// Job management
	pendingJobs map[uuid.UUID]*Job
	jobMutex    sync.RWMutex

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	quit   chan bool

	// Statistics
	stats      Stats
	statsMutex sync.RWMutex

	// Repositories
	scanJobRepo    *db.ScanJobRepository
	hostRepo       *db.HostRepository
	portScanRepo   *db.PortScanRepository
	scanTargetRepo *db.ScanTargetRepository
}

// Stats holds pool statistics
type Stats struct {
	JobsQueued    int64
	JobsCompleted int64
	JobsFailed    int64
	JobsRetried   int64
	WorkersActive int
	WorkersIdle   int
	LastJobTime   time.Time
}

// NewPool creates a new worker pool
func NewPool(ctx context.Context, config *config.Config, database *db.DB) *Pool {
	poolCtx, cancel := context.WithCancel(ctx)

	pool := &Pool{
		config:         config,
		db:             database,
		logger:         log.New(log.Writer(), "[worker-pool] ", log.LstdFlags|log.Lshortfile),
		workers:        make([]*Worker, 0, config.Scanning.WorkerPoolSize),
		jobQueue:       make(chan *Job, config.Scanning.WorkerPoolSize*2),
		resultChan:     make(chan *Result, config.Scanning.WorkerPoolSize),
		pendingJobs:    make(map[uuid.UUID]*Job),
		ctx:            poolCtx,
		cancel:         cancel,
		quit:           make(chan bool),
		scanJobRepo:    db.NewScanJobRepository(database),
		hostRepo:       db.NewHostRepository(database),
		portScanRepo:   db.NewPortScanRepository(database),
		scanTargetRepo: db.NewScanTargetRepository(database),
	}

	return pool
}

// Start initializes and starts the worker pool
func (p *Pool) Start() error {
	p.logger.Printf("Starting worker pool with %d workers", p.config.Scanning.WorkerPoolSize)

	// Create and start workers
	for i := 0; i < p.config.Scanning.WorkerPoolSize; i++ {
		worker := &Worker{
			ID:      i,
			pool:    p,
			jobChan: make(chan *Job),
			quit:    make(chan bool),
			logger:  log.New(log.Writer(), fmt.Sprintf("[worker-%d] ", i), log.LstdFlags|log.Lshortfile),
		}

		p.workers = append(p.workers, worker)
		p.workerWg.Add(1)
		go worker.start()
	}

	// Start job dispatcher
	go p.dispatcher()

	// Start result processor
	go p.resultProcessor()

	p.logger.Println("Worker pool started successfully")
	return nil
}

// Stop gracefully shuts down the worker pool
func (p *Pool) Stop() error {
	p.logger.Println("Stopping worker pool...")

	// Signal shutdown
	close(p.quit)
	p.cancel()

	// Stop all workers
	for _, worker := range p.workers {
		worker.stop()
	}

	// Wait for all workers to finish
	p.workerWg.Wait()

	// Close channels
	close(p.jobQueue)
	close(p.resultChan)

	p.logger.Println("Worker pool stopped")
	return nil
}

// SubmitJob adds a new job to the queue
func (p *Pool) SubmitJob(target *db.ScanTarget) (*Job, error) {
	job := &Job{
		ID:       uuid.New(),
		TargetID: target.ID,
		Target:   target,
		Priority: 1,
		Retries:  0,
		Created:  time.Now(),
	}

	// Create database record
	dbJob := &db.ScanJob{
		ID:       job.ID,
		TargetID: job.TargetID,
		Status:   db.ScanJobStatusPending,
	}

	if err := p.scanJobRepo.Create(p.ctx, dbJob); err != nil {
		return nil, fmt.Errorf("failed to create scan job record: %w", err)
	}

	// Add to pending jobs
	p.jobMutex.Lock()
	p.pendingJobs[job.ID] = job
	p.jobMutex.Unlock()

	// Queue the job
	select {
	case p.jobQueue <- job:
		p.updateStats(func(s *Stats) { s.JobsQueued++ })
		p.logger.Printf("Job %s queued for target %s", job.ID, job.Target.Name)
		return job, nil
	case <-p.ctx.Done():
		return nil, fmt.Errorf("worker pool is shutting down")
	}
}

// GetStats returns current pool statistics
func (p *Pool) GetStats() Stats {
	p.statsMutex.RLock()
	defer p.statsMutex.RUnlock()

	// Count active/idle workers
	stats := p.stats
	stats.WorkersActive = 0
	stats.WorkersIdle = len(p.workers)

	// This is a simplified count - in reality you'd track worker states
	if len(p.pendingJobs) > 0 {
		stats.WorkersActive = min(len(p.pendingJobs), len(p.workers))
		stats.WorkersIdle = len(p.workers) - stats.WorkersActive
	}

	return stats
}

// dispatcher distributes jobs to available workers
func (p *Pool) dispatcher() {
	p.logger.Println("Job dispatcher started")
	defer p.logger.Println("Job dispatcher stopped")

	for {
		select {
		case job := <-p.jobQueue:
			// Find an available worker
			select {
			case worker := <-p.getAvailableWorker():
				worker.jobChan <- job
			case <-p.ctx.Done():
				return
			}

		case <-p.quit:
			return
		}
	}
}

// getAvailableWorker returns a channel that will receive an available worker
func (p *Pool) getAvailableWorker() <-chan *Worker {
	workerChan := make(chan *Worker, 1)

	go func() {
		for _, worker := range p.workers {
			select {
			case workerChan <- worker:
				return
			default:
				continue
			}
		}
	}()

	return workerChan
}

// resultProcessor handles job results
func (p *Pool) resultProcessor() {
	p.logger.Println("Result processor started")
	defer p.logger.Println("Result processor stopped")

	for {
		select {
		case result := <-p.resultChan:
			p.processResult(result)

		case <-p.quit:
			return
		}
	}
}

// processResult processes a completed job result
func (p *Pool) processResult(result *Result) {
	p.jobMutex.Lock()
	delete(p.pendingJobs, result.Job.ID)
	p.jobMutex.Unlock()

	if result.Error != nil {
		p.handleJobError(result)
		return
	}

	p.handleJobSuccess(result)
}

// handleJobSuccess processes a successful job
func (p *Pool) handleJobSuccess(result *Result) {
	p.logger.Printf("Job %s completed successfully by worker %d", result.Job.ID, result.Worker)

	// Update job status in database
	if err := p.scanJobRepo.UpdateStatus(p.ctx, result.Job.ID, db.ScanJobStatusCompleted, nil); err != nil {
		p.logger.Printf("Failed to update job status: %v", err)
	}

	// Store scan results
	if err := p.storeResults(result); err != nil {
		p.logger.Printf("Failed to store scan results: %v", err)
	}

	p.updateStats(func(s *Stats) {
		s.JobsCompleted++
		s.LastJobTime = time.Now()
	})
}

// handleJobError processes a failed job
func (p *Pool) handleJobError(result *Result) {
	p.logger.Printf("Job %s failed on worker %d: %v", result.Job.ID, result.Worker, result.Error)

	// Check if we should retry
	maxRetries := p.config.Scanning.Retry.MaxRetries
	if result.Job.Retries < maxRetries {
		p.retryJob(result.Job)
		return
	}

	// Update job status as failed
	errorMsg := result.Error.Error()
	if err := p.scanJobRepo.UpdateStatus(p.ctx, result.Job.ID, db.ScanJobStatusFailed, &errorMsg); err != nil {
		p.logger.Printf("Failed to update job status: %v", err)
	}

	p.updateStats(func(s *Stats) { s.JobsFailed++ })
}

// retryJob retries a failed job
func (p *Pool) retryJob(job *Job) {
	job.Retries++

	// Calculate retry delay with exponential backoff
	delay := time.Duration(float64(p.config.Scanning.Retry.RetryDelay) *
		pow(p.config.Scanning.Retry.BackoffMultiplier, float64(job.Retries-1)))

	p.logger.Printf("Retrying job %s (attempt %d/%d) after %v",
		job.ID, job.Retries+1, p.config.Scanning.Retry.MaxRetries+1, delay)

	// Schedule retry
	go func() {
		select {
		case <-time.After(delay):
			p.jobQueue <- job
			p.updateStats(func(s *Stats) { s.JobsRetried++ })
		case <-p.ctx.Done():
			return
		}
	}()
}

// storeResults stores scan results in the database
func (p *Pool) storeResults(result *Result) error {
	if result.Result == nil {
		return fmt.Errorf("no scan result to store")
	}

	// Store hosts and port scans
	for _, host := range result.Result.Hosts {
		// Convert to database host
		dbHost := &db.Host{
			IPAddress: db.IPAddr{IP: net.ParseIP(host.Address)},
			Status:    host.Status,
		}

		// Create or update host
		if err := p.hostRepo.CreateOrUpdate(p.ctx, dbHost); err != nil {
			return fmt.Errorf("failed to store host: %w", err)
		}

		// Store port scans
		var portScans []*db.PortScan
		for _, port := range host.Ports {
			portScan := &db.PortScan{
				JobID:          result.Job.ID,
				HostID:         dbHost.ID,
				Port:           int(port.Number),
				Protocol:       port.Protocol,
				State:          port.State,
				ServiceName:    &port.Service,
				ServiceVersion: &port.Version,
				ServiceProduct: &port.ServiceInfo,
			}
			portScans = append(portScans, portScan)
		}

		if len(portScans) > 0 {
			if err := p.portScanRepo.CreateBatch(p.ctx, portScans); err != nil {
				return fmt.Errorf("failed to store port scans: %w", err)
			}
		}
	}

	return nil
}

// updateStats safely updates pool statistics
func (p *Pool) updateStats(updater func(*Stats)) {
	p.statsMutex.Lock()
	defer p.statsMutex.Unlock()
	updater(&p.stats)
}

// start starts a worker
func (w *Worker) start() {
	defer w.pool.workerWg.Done()
	w.logger.Printf("Worker %d started", w.ID)

	for {
		select {
		case job := <-w.jobChan:
			w.executeJob(job)

		case <-w.quit:
			w.logger.Printf("Worker %d stopping", w.ID)
			return

		case <-w.pool.ctx.Done():
			w.logger.Printf("Worker %d shutting down", w.ID)
			return
		}
	}
}

// stop stops a worker
func (w *Worker) stop() {
	close(w.quit)
}

// executeJob executes a scanning job
func (w *Worker) executeJob(job *Job) {
	w.logger.Printf("Worker %d executing job %s for target %s", w.ID, job.ID, job.Target.Name)

	// Update job status to running
	if err := w.pool.scanJobRepo.UpdateStatus(w.pool.ctx, job.ID, db.ScanJobStatusRunning, nil); err != nil {
		w.logger.Printf("Failed to update job status: %v", err)
	}

	// Create scan configuration
	scanConfig := internal.ScanConfig{
		Targets:    []string{job.Target.Network.String()},
		Ports:      job.Target.ScanPorts,
		ScanType:   job.Target.ScanType,
		TimeoutSec: int(w.pool.config.Scanning.MaxScanTimeout.Seconds()),
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(w.pool.ctx, w.pool.config.Scanning.MaxScanTimeout)
	defer cancel()

	// Execute the scan
	result, err := internal.RunScanWithContext(ctx, scanConfig)

	// Send result back to pool
	w.pool.resultChan <- &Result{
		Job:    job,
		Result: result,
		Error:  err,
		Worker: w.ID,
	}
}

// Helper function for exponential backoff calculation
func pow(base, exp float64) float64 {
	if exp == 0 {
		return 1
	}
	result := base
	for i := 1; i < int(exp); i++ {
		result *= base
	}
	return result
}

// Helper function to find minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
