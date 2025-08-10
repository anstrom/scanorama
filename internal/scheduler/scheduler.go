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
)

// Scheduler manages scheduled discovery and scanning jobs.
type Scheduler struct {
	db        *db.DB
	cron      *cron.Cron
	discovery *discovery.Engine
	profiles  *profiles.Manager
	jobs      map[uuid.UUID]*ScheduledJob
	mu        sync.RWMutex
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
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
	Network     string `json:"network"`
	Method      string `json:"method"`
	DetectOS    bool   `json:"detect_os"`
	Timeout     int    `json:"timeout_seconds"`
	Concurrency int    `json:"concurrency"`
}

// ScanJobConfig represents scan job configuration.
type ScanJobConfig struct {
	LiveHostsOnly bool     `json:"live_hosts_only"`
	Networks      []string `json:"networks,omitempty"`
	ProfileID     string   `json:"profile_id,omitempty"`
	MaxAge        int      `json:"max_age_hours"`
	OSFamily      []string `json:"os_family,omitempty"`
}

// NewScheduler creates a new job scheduler.
func NewScheduler(database *db.DB, discoveryEngine *discovery.Engine, profileManager *profiles.Manager) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		db:        database,
		cron:      cron.New(),
		discovery: discoveryEngine,
		profiles:  profileManager,
		jobs:      make(map[uuid.UUID]*ScheduledJob),
		ctx:       ctx,
		cancel:    cancel,
	}
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

	cronID, err := s.cron.AddFunc(cronExpr, executeFunc)
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

	job.Running = true
	job.LastRun = time.Now()
	s.mu.RUnlock()

	defer func() {
		s.mu.Lock()
		if job, exists := s.jobs[jobID]; exists {
			job.Running = false
		}
		s.mu.Unlock()
	}()

	log.Printf("Executing discovery job '%s' for network %s", job.Config.Name, config.Network)

	// Create discovery config
	discoveryConfig := discovery.Config{
		Network:     config.Network,
		Method:      config.Method,
		DetectOS:    config.DetectOS,
		Timeout:     time.Duration(config.Timeout) * time.Second,
		Concurrency: config.Concurrency,
	}

	// Execute discovery
	ctx := context.Background()
	discoveryJob, err := s.discovery.Discover(ctx, &discoveryConfig)
	if err != nil {
		log.Printf("Discovery job '%s' failed: %v", job.Config.Name, err)
		return
	}

	// Update last run time in database
	s.updateJobLastRun(ctx, jobID, job.LastRun)

	log.Printf("Discovery job '%s' completed, job ID: %s", job.Config.Name, discoveryJob.ID)
}

// executeScanJob executes a scan job.
func (s *Scheduler) executeScanJob(jobID uuid.UUID, config *ScanJobConfig) {
	job, shouldContinue := s.prepareJobExecution(jobID)
	if !shouldContinue {
		return
	}

	defer s.cleanupJobExecution(jobID)

	log.Printf("Executing scan job '%s'", job.Config.Name)
	ctx := context.Background()

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

	// Process hosts for scanning
	s.processHostsForScanning(ctx, hosts, config)

	// Update last run time in database
	s.updateJobLastRun(ctx, jobID, job.LastRun)

	log.Printf("Scan job '%s' completed", job.Config.Name)
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

	job.Running = true
	job.LastRun = time.Now()
	s.mu.RUnlock()

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

// processHostsForScanning processes each host for scanning with appropriate profiles.
func (s *Scheduler) processHostsForScanning(ctx context.Context, hosts []*db.Host, config *ScanJobConfig) {
	// TODO: Implement actual scanning logic here
	// This would integrate with the existing scan functionality
	// For now, just log that we would scan these hosts

	for _, host := range hosts {
		profileID := s.selectProfileForHost(ctx, host, config.ProfileID)
		if profileID == "" {
			continue
		}

		osFamily := "unknown"
		if host.OSFamily != nil {
			osFamily = *host.OSFamily
		}
		log.Printf("Would scan host %s (%s) with profile %s",
			host.IPAddress, osFamily, profileID)
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
		networkConditions := []string{}
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

// storeJobInMemory stores the job in the scheduler's memory.
func (s *Scheduler) storeJobInMemory(job *db.ScheduledJob, cronID cron.EntryID) {
	// Calculate next run time using standard parser
	schedule, _ := cron.ParseStandard(job.CronExpression)
	nextRun := schedule.Next(time.Now())

	// Store in memory
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

// updateJobLastRun updates the last run time for a scheduled job.
func (s *Scheduler) updateJobLastRun(ctx context.Context, jobID uuid.UUID, lastRun time.Time) {
	query := `UPDATE scheduled_jobs SET last_run = $1 WHERE id = $2`
	if _, err := s.db.ExecContext(ctx, query, lastRun, jobID); err != nil {
		log.Printf("Failed to update last run time for job %s: %v", jobID, err)
	}
}
