// Package scheduler provides job scheduling and execution functionality for scanorama.
// It manages scheduled discovery and scanning jobs, handles job queuing,
// and coordinates the execution of network scans and host discovery operations.
package scheduler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/anstrom/scanorama/internal/profiles"
	"github.com/anstrom/scanorama/internal/scanning"
	"github.com/anstrom/scanorama/internal/services"
)

const (
	// Scan timeout constants in seconds, keyed to nmap timing templates.
	scanTimeoutParanoid   = 3600 // 1 hour  - matches nmap T0 (paranoid)
	scanTimeoutPolite     = 1800 // 30 min  - matches nmap T1 (polite)
	scanTimeoutNormal     = 900  // 15 min  - matches nmap T3 (normal)
	scanTimeoutAggressive = 600  // 10 min  - matches nmap T4 (aggressive)
	scanTimeoutInsane     = 300  // 5 min   - matches nmap T5 (insane)
)

// Scheduler manages scheduled discovery and scanning jobs.
// defaultMaxConcurrentScans is the default limit on simultaneous per-host scans
// launched by a single scan job execution. Callers can override this with
// WithMaxConcurrentScans.
const (
	defaultMaxConcurrentScans = 5
	// smartScanJobTimeout is the per-execution deadline for a smart-scan cron job.
	smartScanJobTimeout = 5 * time.Minute
)

// Scheduler manages scheduled discovery and scanning jobs.
type Scheduler struct {
	db                 *db.DB
	cron               *cron.Cron
	discovery          *discovery.Engine
	profiles           *profiles.Manager
	smartScan          smartScanBatcher // optional: enables smart_scan job type
	jobs               map[uuid.UUID]*ScheduledJob
	mu                 sync.RWMutex
	running            bool
	ctx                context.Context
	cancel             context.CancelFunc
	maxConcurrentScans int                      // bounded parallelism for per-host scans
	scanQueue          *scanning.ScanQueue      // optional queue-based scan execution
	scanRunner         scanning.ScanJobExecutor // injectable for tests; defaults to RunScanWithContext
}

// smartScanBatcher is the subset of services.SmartScanService used by the
// scheduler. Defined here to avoid an import cycle.
type smartScanBatcher interface {
	QueueBatch(ctx context.Context, filter services.BatchFilter) (*services.BatchResult, error)
}

// ScheduledJob represents a scheduled job wrapper.
type ScheduledJob struct {
	ID      uuid.UUID
	CronID  cron.EntryID
	Config  *db.ScheduledJob
	LastRun time.Time
	NextRun time.Time
	Running bool
}

// DiscoveryJobConfig represents discovery job configuration.
type DiscoveryJobConfig struct {
	NetworkID   string `json:"network_id"` // UUID string stored by the API handler
	Network     string `json:"network"`    // CIDR — populated at runtime if empty
	Method      string `json:"method"`     // populated at runtime if empty
	DetectOS    bool   `json:"detect_os"`
	Timeout     int    `json:"timeout_seconds"`
	Concurrency int    `json:"concurrency"`
}

// ScanJobConfig represents scan job configuration.
type ScanJobConfig struct {
	NetworkID     string   `json:"network_id"` // UUID string stored by the API handler
	LiveHostsOnly bool     `json:"live_hosts_only"`
	Networks      []string `json:"networks,omitempty"` // populated at runtime if nil
	ProfileID     string   `json:"profile_id,omitempty"`
	MaxAge        int      `json:"max_age_hours"`
	OSFamily      []string `json:"os_family,omitempty"`
}

// SmartScanJobConfig configures a recurring scheduled smart scan.
// On each cron fire the scheduler calls QueueBatch with a staleness filter
// targeting hosts whose knowledge score is below the threshold or whose
// last_seen timestamp is older than MaxStalenessHours.
type SmartScanJobConfig struct {
	// ScoreThreshold re-queues hosts with knowledge_score < threshold.
	// Defaults to 80 when zero.
	ScoreThreshold int `json:"score_threshold,omitempty"`
	// MaxStalenessHours re-queues hosts not seen within this many hours.
	// Ignored when zero.
	MaxStalenessHours int `json:"max_staleness_hours,omitempty"`
	// NetworkCIDR restricts the batch to hosts within the given network.
	// Empty means all networks.
	NetworkCIDR string `json:"network_cidr,omitempty"`
	// Limit caps the number of scans queued per cron fire. Defaults to the
	// SmartScanService batch limit when zero.
	Limit int `json:"limit,omitempty"`
}

// NewScheduler creates a new job scheduler.
func NewScheduler(database *db.DB, discoveryEngine *discovery.Engine, profileManager *profiles.Manager) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		db:                 database,
		cron:               cron.New(),
		discovery:          discoveryEngine,
		profiles:           profileManager,
		jobs:               make(map[uuid.UUID]*ScheduledJob),
		ctx:                ctx,
		cancel:             cancel,
		maxConcurrentScans: defaultMaxConcurrentScans,
	}
}

// WithMaxConcurrentScans overrides the maximum number of host scans that may
// run in parallel within a single scan job execution. It returns the scheduler
// to allow call chaining. n <= 0 is treated as the default.
func (s *Scheduler) WithMaxConcurrentScans(n int) *Scheduler {
	if n > 0 {
		s.maxConcurrentScans = n
	}
	return s
}

// WithScanQueue configures the scheduler to use the provided ScanQueue for
// executing host scans instead of the default semaphore+goroutine pattern.
// When set, processHostsForScanning submits scan requests to the queue and
// waits for all results before returning. Pass nil to revert to the default.
func (s *Scheduler) WithScanQueue(q *scanning.ScanQueue) *Scheduler {
	s.scanQueue = q
	return s
}

// Start begins the scheduler.
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("scheduler is already running")
	}

	// Load existing scheduled jobs from database
	if err := s.loadScheduledJobs(); err != nil {
		return fmt.Errorf("failed to load scheduled jobs: %w", err)
	}

	// Start the cron scheduler
	s.cron.Start()
	s.running = true

	log.Printf("Scheduler started with %d jobs", len(s.jobs))
	return nil
}

// Stop stops the scheduler.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	// Stop the cron scheduler
	s.cron.Stop()
	s.cancel()
	s.running = false

	log.Println("Scheduler stopped")
}

// AddDiscoveryJob adds a new scheduled discovery job.
func (s *Scheduler) AddDiscoveryJob(ctx context.Context, name, cronExpr string, config DiscoveryJobConfig) error {
	job, err := s.createScheduledJob(ctx, name, cronExpr, db.ScheduledJobTypeDiscovery, config)
	if err != nil {
		return err
	}

	return s.addJobToCron(cronExpr, job, func() {
		s.executeDiscoveryJob(job.ID, config)
	})
}

// AddScanJob adds a new scheduled scan job.
func (s *Scheduler) AddScanJob(ctx context.Context, name, cronExpr string, config *ScanJobConfig) error {
	job, err := s.createScheduledJob(ctx, name, cronExpr, db.ScheduledJobTypeScan, config)
	if err != nil {
		return err
	}

	return s.addJobToCron(cronExpr, job, func() {
		s.executeScanJob(job.ID, config)
	})
}

// WithSmartScanService attaches a SmartScanService so the scheduler can execute
// smart_scan type jobs. Must be called before Start.
func (s *Scheduler) WithSmartScanService(svc smartScanBatcher) *Scheduler {
	s.smartScan = svc
	return s
}

// AddSmartScanJob adds a new recurring smart-scan job to the schedule.
func (s *Scheduler) AddSmartScanJob(ctx context.Context, name, cronExpr string, config SmartScanJobConfig) error {
	job, err := s.createScheduledJob(ctx, name, cronExpr, db.ScheduledJobTypeSmartScan, config)
	if err != nil {
		return err
	}

	return s.addJobToCron(cronExpr, job, func() {
		s.executeSmartScanJob(job.ID, config)
	})
}

// createScheduledJob is a helper function that creates and saves a scheduled job.
func (s *Scheduler) createScheduledJob(
	ctx context.Context, name, cronExpr, jobType string, config interface{},
) (*db.ScheduledJob, error) {
	// Validate cron expression using standard 5-field format
	if _, err := cron.ParseStandard(cronExpr); err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}

	// Marshal config
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	// Create scheduled job
	job := &db.ScheduledJob{
		ID:             uuid.New(),
		Name:           name,
		Type:           jobType,
		CronExpression: cronExpr,
		Config:         db.JSONB(configJSON),
		Enabled:        true,
		CreatedAt:      time.Now(),
	}

	// Calculate next run time using standard parser
	schedule, _ := cron.ParseStandard(cronExpr)
	nextRun := schedule.Next(time.Now())
	job.NextRun = &nextRun

	// Save to database
	if err := s.saveScheduledJob(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to save scheduled job: %w", err)
	}

	return job, nil
}

// addJobToCron adds a job to the cron scheduler.
func (s *Scheduler) addJobToCron(cronExpr string, job *db.ScheduledJob, executeFunc func()) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Wrap the execute function with panic recovery
	wrappedFunc := func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("PANIC in scheduled job '%s' (ID: %s, Type: %s): %v",
					job.Name, job.ID, job.Type, r)
				// Mark job as not running in case panic happened during execution
				s.mu.Lock()
				if scheduledJob, exists := s.jobs[job.ID]; exists {
					scheduledJob.Running = false
				}
				s.mu.Unlock()
			}
		}()
		executeFunc()
	}

	cronID, err := s.cron.AddFunc(cronExpr, wrappedFunc)
	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}

	// Store in memory
	s.jobs[job.ID] = &ScheduledJob{
		ID:      job.ID,
		CronID:  cronID,
		Config:  job,
		NextRun: *job.NextRun,
	}

	log.Printf("Added %s job '%s' with schedule '%s'", job.Type, job.Name, cronExpr)
	return nil
}

// RemoveJob removes a scheduled job.
func (s *Scheduler) RemoveJob(ctx context.Context, jobID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the job
	job, exists := s.jobs[jobID]
	if !exists {
		return fmt.Errorf("job not found")
	}

	// Remove from cron scheduler
	s.cron.Remove(job.CronID)

	// Remove from database
	if err := s.deleteScheduledJob(ctx, jobID); err != nil {
		return fmt.Errorf("failed to delete from database: %w", err)
	}

	// Remove from memory
	delete(s.jobs, jobID)

	log.Printf("Removed scheduled job '%s'", job.Config.Name)
	return nil
}

// GetJobs returns all scheduled jobs from the database.
func (s *Scheduler) GetJobs() []*ScheduledJob {
	ctx := context.Background()
	dbJobs, err := s.loadJobsFromDatabase(ctx)
	if err != nil {
		log.Printf("Failed to load jobs from database: %v", err)
		return []*ScheduledJob{}
	}

	jobs := make([]*ScheduledJob, 0, len(dbJobs))
	for _, dbJob := range dbJobs {
		// Calculate next run time
		schedule, _ := cron.ParseStandard(dbJob.CronExpression)
		nextRun := schedule.Next(time.Now())

		job := &ScheduledJob{
			ID:      dbJob.ID,
			Config:  dbJob,
			NextRun: nextRun,
		}
		jobs = append(jobs, job)
	}

	return jobs
}

// loadJobsFromDatabase loads all scheduled jobs from the database.
func (s *Scheduler) loadJobsFromDatabase(ctx context.Context) ([]*db.ScheduledJob, error) {
	query := `
		SELECT id, name, type, cron_expression, config, enabled, last_run, next_run, created_at
		FROM scheduled_jobs
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query scheduled jobs: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	var jobs []*db.ScheduledJob
	for rows.Next() {
		job := &db.ScheduledJob{}
		err := rows.Scan(
			&job.ID, &job.Name, &job.Type, &job.CronExpression,
			&job.Config, &job.Enabled, &job.LastRun, &job.NextRun, &job.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan scheduled job: %w", err)
		}
		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// EnableJob enables a scheduled job.
func (s *Scheduler) EnableJob(ctx context.Context, jobID uuid.UUID) error {
	return s.setJobEnabled(ctx, jobID, true)
}

// DisableJob disables a scheduled job.
func (s *Scheduler) DisableJob(ctx context.Context, jobID uuid.UUID) error {
	return s.setJobEnabled(ctx, jobID, false)
}

// setJobEnabled enables or disables a scheduled job.
func (s *Scheduler) setJobEnabled(ctx context.Context, jobID uuid.UUID, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	job, exists := s.jobs[jobID]
	if !exists {
		return fmt.Errorf("job not found")
	}

	// Update database
	query := `UPDATE scheduled_jobs SET enabled = $1 WHERE id = $2`
	if _, err := s.db.ExecContext(ctx, query, enabled, jobID); err != nil {
		return fmt.Errorf("failed to update job status: %w", err)
	}

	// Update in-memory config
	job.Config.Enabled = enabled

	action := "disabled"
	if enabled {
		action = "enabled"
	}

	log.Printf("Job '%s' %s", job.Config.Name, action)
	return nil
}

// executeDiscoveryJob executes a discovery job.
func (s *Scheduler) executeDiscoveryJob(jobID uuid.UUID, config DiscoveryJobConfig) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in executeDiscoveryJob (jobID: %s): %v", jobID, r)
			// Ensure job is marked as not running
			s.mu.Lock()
			if job, exists := s.jobs[jobID]; exists {
				job.Running = false
			}
			s.mu.Unlock()
		}
	}()

	s.mu.RLock()
	job, exists := s.jobs[jobID]
	if !exists || !job.Config.Enabled {
		s.mu.RUnlock()
		return
	}

	if job.Running {
		log.Printf("Discovery job '%s' is already running, skipping", job.Config.Name)
		s.mu.RUnlock()
		return
	}
	s.mu.RUnlock()

	s.mu.Lock()
	job.Running = true
	job.LastRun = time.Now()
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if job, exists := s.jobs[jobID]; exists {
			job.Running = false
		}
		s.mu.Unlock()
	}()

	startTime := job.LastRun
	log.Printf("Executing discovery job '%s' for network %s", job.Config.Name, config.Network)

	// Resolve network CIDR and discovery method from the database when the
	// stored config only carries a network UUID (as written by buildScheduleJobConfig).
	network := config.Network
	method := config.Method
	if network == "" && config.NetworkID != "" {
		var cidr, discoveryMethod string
		err := s.db.QueryRowContext(
			context.Background(),
			`SELECT cidr::text, discovery_method FROM networks WHERE id = $1`,
			config.NetworkID,
		).Scan(&cidr, &discoveryMethod)
		if err != nil {
			log.Printf("executeDiscoveryJob: failed to resolve network %s: %v", config.NetworkID, err)
			return
		}
		network = cidr
		method = discoveryMethod
	}

	// Create discovery config
	discoveryConfig := discovery.Config{
		Network:     network,
		Method:      method,
		DetectOS:    config.DetectOS,
		Timeout:     time.Duration(config.Timeout) * time.Second,
		Concurrency: config.Concurrency,
	}
	// Bug 3: forward the registered network UUID so the engine can persist
	// network_id on the discovery_jobs row it creates internally.
	if config.NetworkID != "" {
		if uid, err := uuid.Parse(config.NetworkID); err == nil {
			discoveryConfig.NetworkID = &uid
		}
	}

	// Execute discovery
	ctx := context.Background()
	discoveryJob, err := s.discovery.Discover(ctx, &discoveryConfig)

	// Always update last_run and next_run in the database, even on failure,
	// so the scheduler's persistent state stays current.
	s.updateJobLastRun(ctx, jobID, startTime)

	if err != nil {
		log.Printf("Discovery job '%s' failed after %s: %v",
			job.Config.Name, time.Since(startTime).Round(time.Millisecond), err)
		return
	}

	log.Printf("Discovery job '%s' completed in %s, discovery job ID: %s",
		job.Config.Name, time.Since(startTime).Round(time.Millisecond), discoveryJob.ID)
}

// executeScanJob executes a scan job.
func (s *Scheduler) executeScanJob(jobID uuid.UUID, config *ScanJobConfig) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in executeScanJob (jobID: %s): %v", jobID, r)
			// Ensure job is marked as not running
			s.cleanupJobExecution(jobID)
		}
	}()

	job, shouldContinue := s.prepareJobExecution(jobID)
	if !shouldContinue {
		return
	}

	defer s.cleanupJobExecution(jobID)

	startTime := job.LastRun
	log.Printf("Executing scan job '%s'", job.Config.Name)
	ctx := context.Background()

	// Always persist last_run and next_run when the job finishes (success or failure).
	defer s.updateJobLastRun(ctx, jobID, startTime)

	// Resolve network CIDR from the database when the stored config only carries a
	// network UUID (as written by buildScheduleJobConfig).  Without this, Networks
	// stays nil and getHostsToScan scans every host in the database.
	if len(config.Networks) == 0 && config.NetworkID != "" {
		var cidr string
		err := s.db.QueryRowContext(
			context.Background(),
			`SELECT cidr::text FROM networks WHERE id = $1`,
			config.NetworkID,
		).Scan(&cidr)
		if err != nil {
			log.Printf("executeScanJob: failed to resolve network %s: %v", config.NetworkID, err)
			return
		}
		config.Networks = []string{cidr}
	}

	// Get hosts to scan based on configuration
	hosts, err := s.getHostsToScan(ctx, config)
	if err != nil {
		log.Printf("Scan job '%s' failed to get hosts: %v", job.Config.Name, err)
		return
	}

	if len(hosts) == 0 {
		log.Printf("Scan job '%s' found no hosts to scan", job.Config.Name)
		return
	}

	log.Printf("Scan job '%s' found %d hosts to scan", job.Config.Name, len(hosts))

	// Process hosts for scanning (bounded concurrency, results persisted by RunScanWithContext).
	s.processHostsForScanning(ctx, hosts, config)

	log.Printf("Scan job '%s' completed in %s",
		job.Config.Name, time.Since(startTime).Round(time.Millisecond))
}

// prepareJobExecution validates and prepares a job for execution.
func (s *Scheduler) prepareJobExecution(jobID uuid.UUID) (*ScheduledJob, bool) {
	s.mu.RLock()
	job, exists := s.jobs[jobID]
	if !exists || !job.Config.Enabled {
		s.mu.RUnlock()
		return nil, false
	}

	if job.Running {
		log.Printf("Scan job '%s' is already running, skipping", job.Config.Name)
		s.mu.RUnlock()
		return nil, false
	}
	s.mu.RUnlock()

	s.mu.Lock()
	job.Running = true
	job.LastRun = time.Now()
	s.mu.Unlock()

	return job, true
}

// cleanupJobExecution marks the job as no longer running.
func (s *Scheduler) cleanupJobExecution(jobID uuid.UUID) {
	s.mu.Lock()
	if job, exists := s.jobs[jobID]; exists {
		job.Running = false
	}
	s.mu.Unlock()
}

// processHostsForScanning runs scans against each host using the appropriate profile.
// Scans are dispatched concurrently, bounded by s.maxConcurrentScans, so that a
// large host list does not open an unbounded number of nmap processes at once.
func (s *Scheduler) processHostsForScanning(ctx context.Context, hosts []*db.Host, config *ScanJobConfig) {
	if s.profiles == nil {
		log.Printf("Scan job skipped: no profile manager configured")
		return
	}

	if len(hosts) == 0 {
		return
	}

	if s.scanQueue != nil {
		s.processHostsViaQueue(ctx, hosts, config)
		return
	}

	// sem is a counting semaphore that limits concurrent scans.
	sem := make(chan struct{}, s.maxConcurrentScans)
	var wg sync.WaitGroup

hostLoop:
	for i, host := range hosts {
		// Check for context cancellation before starting the next goroutine.
		if ctx.Err() != nil {
			log.Printf("Scan job context canceled after dispatching %d/%d hosts", i, len(hosts))
			break
		}

		profileID := s.selectProfileForHost(ctx, host, config.ProfileID)
		if profileID == "" {
			log.Printf("No profile available for host %s, skipping", host.IPAddress)
			continue
		}

		profile, err := s.profiles.GetByID(ctx, profileID)
		if err != nil {
			log.Printf("Failed to get profile %s for host %s: %v", profileID, host.IPAddress, err)
			continue
		}

		// Acquire semaphore slot (blocks when maxConcurrentScans are already running).
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			log.Printf("Scan job context canceled while waiting for semaphore slot")
			break hostLoop
		}

		wg.Add(1)
		go func(h *db.Host, pID string, ports, scanType, timing string, timeoutSec int) {
			defer func() {
				<-sem // release semaphore slot
				wg.Done()
				if r := recover(); r != nil {
					log.Printf("PANIC while scanning host %s: %v", h.IPAddress, r)
				}
			}()

			osFamily := "unknown"
			if h.OSFamily != nil {
				osFamily = *h.OSFamily
			}

			scanConfig := &scanning.ScanConfig{
				Targets:     []string{h.IPAddress.String()},
				Ports:       ports,
				ScanType:    scanType,
				Timing:      timing,
				TimeoutSec:  timeoutSec,
				Concurrency: 1,
			}

			log.Printf("Scanning host %s (%s) with profile %s (ports: %s, type: %s)",
				h.IPAddress, osFamily, pID, ports, scanType)

			result, err := scanning.RunScanWithContext(ctx, scanConfig, s.db)
			if err != nil {
				log.Printf("Failed to scan host %s with profile %s: %v", h.IPAddress, pID, err)
				return
			}

			log.Printf("Completed scan of host %s: %d hosts responded, %d up",
				h.IPAddress, result.Stats.Total, result.Stats.Up)
		}(host, profileID, profile.Ports, profile.ScanType, profile.Timing, timingToScanTimeout(profile.Timing))
	}

	// Wait for all in-flight scans to finish before returning.
	wg.Wait()
}

// processHostsViaQueue submits scan jobs to the ScanQueue and waits for all
// results before returning. This allows the scheduler to leverage the queue's
// bounded worker pool instead of managing its own goroutines.
func (s *Scheduler) processHostsViaQueue(ctx context.Context, hosts []*db.Host, config *ScanJobConfig) {
	runner := s.scanRunner
	if runner == nil {
		runner = scanning.RunScanWithContext
	}

	var (
		wg           sync.WaitGroup
		mu           sync.Mutex
		successCount int
		failCount    int
		submitted    int
	)

	for i, host := range hosts {
		// Check for context cancellation before submitting the next job.
		if ctx.Err() != nil {
			log.Printf("Scan job context canceled after submitting %d/%d hosts to queue", i, len(hosts))
			break
		}

		profileID := s.selectProfileForHost(ctx, host, config.ProfileID)
		if profileID == "" {
			log.Printf("No profile available for host %s, skipping", host.IPAddress)
			continue
		}

		profile, err := s.profiles.GetByID(ctx, profileID)
		if err != nil {
			log.Printf("Failed to get profile %s for host %s: %v", profileID, host.IPAddress, err)
			continue
		}

		scanConfig := &scanning.ScanConfig{
			Targets:     []string{host.IPAddress.String()},
			Ports:       profile.Ports,
			ScanType:    profile.ScanType,
			Timing:      profile.Timing,
			TimeoutSec:  timingToScanTimeout(profile.Timing),
			Concurrency: 1,
		}

		jobID := fmt.Sprintf("sched-%s-%s", host.ID, host.IPAddress)
		hostIP := host.IPAddress.String()

		wg.Add(1)
		job := scanning.NewScanJob(
			jobID,
			scanConfig,
			s.db,
			runner,
			func(result *scanning.ScanResult, err error) {
				defer wg.Done()
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					failCount++
					log.Printf("Queued scan %s failed: %v", jobID, err)
				} else {
					successCount++
					hostCount := 0
					if result != nil {
						hostCount = len(result.Hosts)
					}
					log.Printf("Queued scan %s completed: %d hosts found", jobID, hostCount)
				}
			},
		)

		if err := s.scanQueue.Submit(job); err != nil {
			wg.Done() // onDone won't be called since Submit failed
			if err == scanning.ErrQueueFull {
				log.Printf("Scan queue full, skipping host %s", hostIP)
			} else {
				log.Printf("Failed to submit scan for host %s to queue: %v", hostIP, err)
			}
			continue
		}

		submitted++
		log.Printf("Submitted scan for host %s to queue (profile %s)", hostIP, profileID)
	}

	if submitted == 0 {
		log.Printf("No scans were submitted to the queue")
		return
	}

	// Wait for all submitted scans to complete, respecting context cancellation.
	log.Printf("Waiting for %d queued scan(s) to complete", submitted)
	waitWithCancel(ctx, &wg, func() {
		mu.Lock()
		log.Printf("All queued scans finished: %d succeeded, %d failed out of %d submitted",
			successCount, failCount, submitted)
		mu.Unlock()
	})
}

// waitWithCancel runs wg.Wait() in a background goroutine and calls onDone
// when all work finishes, or returns early if ctx is cancelled first.
func waitWithCancel(ctx context.Context, wg *sync.WaitGroup, onDone func()) {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		onDone()
	case <-ctx.Done():
		log.Printf("Scan job context canceled while waiting for queued results")
	}
}

// timingToScanTimeout converts a profile timing string to a scan timeout in seconds.
// Longer timeouts are used for polite/paranoid scans to match their slower pace.
func timingToScanTimeout(timing string) int {
	switch timing {
	case db.ScanTimingParanoid:
		return scanTimeoutParanoid
	case db.ScanTimingPolite:
		return scanTimeoutPolite
	case db.ScanTimingNormal:
		return scanTimeoutNormal
	case db.ScanTimingAggressive:
		return scanTimeoutAggressive
	case db.ScanTimingInsane:
		return scanTimeoutInsane
	default:
		return scanTimeoutNormal
	}
}

// selectProfileForHost selects the appropriate profile for a host.
func (s *Scheduler) selectProfileForHost(ctx context.Context, host *db.Host, configProfileID string) string {
	profileID := configProfileID
	if profileID == "" || profileID == "auto" {
		// Auto-select profile based on OS
		profile, err := s.profiles.SelectBestProfile(ctx, host)
		if err != nil {
			log.Printf("Failed to select profile for host %s: %v", host.IPAddress, err)
			return ""
		}
		profileID = profile.ID
	}
	return profileID
}

// getHostsToScan returns hosts that should be scanned based on configuration.
func (s *Scheduler) getHostsToScan(ctx context.Context, config *ScanJobConfig) ([]*db.Host, error) {
	query, args := s.buildHostScanQuery(config)
	return s.executeHostScanQuery(ctx, query, args)
}

// buildHostScanQuery constructs the SQL query and arguments for host scanning.
func (s *Scheduler) buildHostScanQuery(config *ScanJobConfig) (query string, args []interface{}) {
	query = `
		SELECT id, ip_address, hostname, mac_address, vendor, os_family, os_name, os_version,
		       os_confidence, os_detected_at, os_method, os_details, discovery_method,
		       response_time_ms, discovery_count, ignore_scanning, first_seen, last_seen, status
		FROM hosts
		WHERE ignore_scanning = false
	`
	args = []interface{}{}
	argCount := 0

	// Apply various filters based on configuration
	query, args, _ = s.addHostScanFilters(query, args, argCount, config)
	query += " ORDER BY last_seen DESC"

	return query, args
}

// addHostScanFilters adds WHERE clauses based on scan job configuration.
func (s *Scheduler) addHostScanFilters(
	query string, args []interface{}, argCount int, config *ScanJobConfig,
) (updatedQuery string, updatedArgs []interface{}, updatedArgCount int) {
	// Filter by status if live hosts only
	if config.LiveHostsOnly {
		query += " AND status = $1"
		argCount++
		args = append(args, db.HostStatusUp)
	}

	// Filter by max age
	if config.MaxAge > 0 {
		query += fmt.Sprintf(" AND last_seen >= NOW() - INTERVAL '%d hours'", config.MaxAge)
	}

	// Filter by OS family if specified
	if len(config.OSFamily) > 0 {
		argCount++
		query += fmt.Sprintf(" AND os_family = ANY($%d)", argCount)
		args = append(args, config.OSFamily)
	}

	// Filter by networks if specified
	if len(config.Networks) > 0 {
		networkConditions := make([]string, 0, len(config.Networks))
		for _, network := range config.Networks {
			argCount++
			networkConditions = append(networkConditions, fmt.Sprintf("ip_address << $%d", argCount))
			args = append(args, network)
		}
		query += " AND (" + strings.Join(networkConditions, " OR ") + ")"
	}

	updatedQuery = query
	updatedArgs = args
	updatedArgCount = argCount
	return
}

// executeHostScanQuery executes the query and scans the results into Host objects.
func (s *Scheduler) executeHostScanQuery(ctx context.Context, query string, args []interface{}) ([]*db.Host, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query hosts to scan: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	var hosts []*db.Host
	for rows.Next() {
		host, err := s.scanHostRow(rows)
		if err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}

	return hosts, rows.Err()
}

// scanHostRow scans a single row into a Host object.
func (s *Scheduler) scanHostRow(rows *sql.Rows) (*db.Host, error) {
	host := &db.Host{}
	err := rows.Scan(
		&host.ID, &host.IPAddress, &host.Hostname, &host.MACAddress, &host.Vendor,
		&host.OSFamily, &host.OSName, &host.OSVersion, &host.OSConfidence,
		&host.OSDetectedAt, &host.OSMethod, &host.OSDetails, &host.DiscoveryMethod,
		&host.ResponseTimeMS, &host.DiscoveryCount, &host.IgnoreScanning,
		&host.FirstSeen, &host.LastSeen, &host.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan host: %w", err)
	}
	return host, nil
}

// loadScheduledJobs loads scheduled jobs from the database.
func (s *Scheduler) loadScheduledJobs() error {
	rows, err := s.queryScheduledJobs()
	if err != nil {
		return err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Failed to close rows: %v", err)
		}
	}()

	for rows.Next() {
		if err := s.processScheduledJobRow(rows); err != nil {
			log.Printf("Failed to process scheduled job: %v", err)
			continue
		}
	}

	return rows.Err()
}

// queryScheduledJobs executes the query for scheduled jobs.
func (s *Scheduler) queryScheduledJobs() (*sql.Rows, error) {
	query := `
		SELECT id, name, type, cron_expression, config, enabled, last_run, next_run, created_at
		FROM scheduled_jobs
		WHERE enabled = true
		ORDER BY created_at ASC
	`

	rows, err := s.db.QueryContext(s.ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query scheduled jobs: %w", err)
	}
	return rows, nil
}

// processScheduledJobRow processes a single scheduled job row.
func (s *Scheduler) processScheduledJobRow(rows *sql.Rows) error {
	job := &db.ScheduledJob{}
	err := rows.Scan(
		&job.ID, &job.Name, &job.Type, &job.CronExpression,
		&job.Config, &job.Enabled, &job.LastRun, &job.NextRun, &job.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to scan scheduled job: %w", err)
	}

	cronID, err := s.addJobToCronScheduler(job)
	if err != nil {
		return fmt.Errorf("failed to add job %s to cron: %w", job.Name, err)
	}

	s.storeJobInMemory(job, cronID)
	return nil
}

// addJobToCronScheduler adds a job to the cron scheduler based on its type.
func (s *Scheduler) addJobToCronScheduler(job *db.ScheduledJob) (cron.EntryID, error) {
	switch job.Type {
	case db.ScheduledJobTypeDiscovery:
		return s.addDiscoveryJobToCron(job)
	case db.ScheduledJobTypeScan:
		return s.addScanJobToCron(job)
	case db.ScheduledJobTypeSmartScan:
		return s.addSmartScanJobToCron(job)
	default:
		return 0, fmt.Errorf("unknown job type: %s", job.Type)
	}
}

// addDiscoveryJobToCron adds a discovery job to the cron scheduler.
func (s *Scheduler) addDiscoveryJobToCron(job *db.ScheduledJob) (cron.EntryID, error) {
	var config DiscoveryJobConfig
	if err := json.Unmarshal([]byte(job.Config), &config); err != nil {
		return 0, fmt.Errorf("failed to unmarshal discovery config: %w", err)
	}

	return s.cron.AddFunc(job.CronExpression, func() {
		s.executeDiscoveryJob(job.ID, config)
	})
}

// addScanJobToCron adds a scan job to the cron scheduler.
func (s *Scheduler) addScanJobToCron(job *db.ScheduledJob) (cron.EntryID, error) {
	var config ScanJobConfig
	if err := json.Unmarshal([]byte(job.Config), &config); err != nil {
		return 0, fmt.Errorf("failed to unmarshal scan config: %w", err)
	}

	return s.cron.AddFunc(job.CronExpression, func() {
		s.executeScanJob(job.ID, &config)
	})
}

// addSmartScanJobToCron adds a smart_scan job to the cron scheduler.
func (s *Scheduler) addSmartScanJobToCron(job *db.ScheduledJob) (cron.EntryID, error) {
	var config SmartScanJobConfig
	if err := json.Unmarshal([]byte(job.Config), &config); err != nil {
		return 0, fmt.Errorf("failed to unmarshal smart scan config: %w", err)
	}

	return s.cron.AddFunc(job.CronExpression, func() {
		s.executeSmartScanJob(job.ID, config)
	})
}

// executeSmartScanJob runs a scheduled smart scan by calling QueueBatch.
func (s *Scheduler) executeSmartScanJob(jobID uuid.UUID, config SmartScanJobConfig) {
	if s.smartScan == nil {
		log.Printf("Smart scan job %s skipped: no SmartScanService configured", jobID)
		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, smartScanJobTimeout)
	defer cancel()

	filter := services.BatchFilter{
		NetworkCIDR:       config.NetworkCIDR,
		Limit:             config.Limit,
		Source:            db.ScanSourceScheduled,
		ScoreThreshold:    config.ScoreThreshold,
		MaxStalenessHours: config.MaxStalenessHours,
	}

	result, err := s.smartScan.QueueBatch(ctx, filter)
	if err != nil {
		log.Printf("Smart scan job %s failed: %v", jobID, err)
		return
	}
	log.Printf("Smart scan job %s queued %d scans, skipped %d", jobID, result.Queued, result.Skipped)
}

// storeJobInMemory stores the job in the scheduler's memory.
// storeJobInMemory stores a job in the in-memory map.
// IMPORTANT: This function must be called with s.mu held by the caller.
func (s *Scheduler) storeJobInMemory(job *db.ScheduledJob, cronID cron.EntryID) {
	// Calculate next run time using standard parser
	schedule, _ := cron.ParseStandard(job.CronExpression)
	nextRun := schedule.Next(time.Now())

	// Store in memory (caller must hold s.mu)
	s.jobs[job.ID] = &ScheduledJob{
		ID:      job.ID,
		CronID:  cronID,
		Config:  job,
		NextRun: nextRun,
	}
}

// saveScheduledJob saves a scheduled job to the database.
func (s *Scheduler) saveScheduledJob(ctx context.Context, job *db.ScheduledJob) error {
	query := `
		INSERT INTO scheduled_jobs (id, name, type, cron_expression, config, enabled, last_run, next_run, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := s.db.ExecContext(ctx, query,
		job.ID, job.Name, job.Type, job.CronExpression, job.Config,
		job.Enabled, job.LastRun, job.NextRun, job.CreatedAt)

	return err
}

// deleteScheduledJob deletes a scheduled job from the database.
func (s *Scheduler) deleteScheduledJob(ctx context.Context, jobID uuid.UUID) error {
	query := `DELETE FROM scheduled_jobs WHERE id = $1`
	_, err := s.db.ExecContext(ctx, query, jobID)
	return err
}

// updateJobLastRun updates the last_run timestamp and recalculates next_run for
// a scheduled job, persisting both values to the database. It also refreshes
// the in-memory NextRun field so the scheduler's state stays consistent.
func (s *Scheduler) updateJobLastRun(ctx context.Context, jobID uuid.UUID, lastRun time.Time) {
	// Derive the next run time from the cron expression stored in the job config.
	nextRun := s.calculateNextRun(jobID, lastRun)

	query := `UPDATE scheduled_jobs SET last_run = $1, next_run = $2 WHERE id = $3`
	if _, err := s.db.ExecContext(ctx, query, lastRun, nextRun, jobID); err != nil {
		log.Printf("Failed to update last/next run time for job %s: %v", jobID, err)
		return
	}

	// Keep the in-memory entry consistent.
	s.updateJobNextRunInMemory(jobID, nextRun)
}

// calculateNextRun computes the next scheduled run time for a job by parsing
// its cron expression from the in-memory registry. Falls back to 0 time when
// the job is not found or the expression cannot be parsed.
func (s *Scheduler) calculateNextRun(jobID uuid.UUID, after time.Time) time.Time {
	s.mu.RLock()
	job, exists := s.jobs[jobID]
	s.mu.RUnlock()

	if !exists {
		return time.Time{}
	}

	schedule, err := cron.ParseStandard(job.Config.CronExpression)
	if err != nil {
		log.Printf("Failed to parse cron expression for job %s: %v", jobID, err)
		return time.Time{}
	}

	return schedule.Next(after)
}

// updateJobNextRunInMemory updates only the in-memory NextRun field of a job.
// It is safe to call from any goroutine.
func (s *Scheduler) updateJobNextRunInMemory(jobID uuid.UUID, nextRun time.Time) {
	s.mu.Lock()
	if job, exists := s.jobs[jobID]; exists {
		job.NextRun = nextRun
	}
	s.mu.Unlock()
}
