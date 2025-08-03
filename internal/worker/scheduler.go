package worker

import (
	"context"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
)

// ScheduledTarget represents a target scheduled for scanning
type ScheduledTarget struct {
	Target   *db.ScanTarget
	NextScan time.Time
	LastScan *time.Time
	Enabled  bool
}

// Scheduler manages periodic scanning of targets
type Scheduler struct {
	// Configuration
	config *config.Config
	db     *db.DB
	pool   *Pool
	logger *log.Logger

	// Target management
	targets      map[uuid.UUID]*ScheduledTarget
	targetsMutex sync.RWMutex

	// Scheduling
	scheduleTimer *time.Timer
	tickInterval  time.Duration

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Repositories
	scanTargetRepo *db.ScanTargetRepository

	// Statistics
	stats      SchedulerStats
	statsMutex sync.RWMutex
}

// SchedulerStats holds scheduler statistics
type SchedulerStats struct {
	TargetsActive     int
	TargetsInactive   int
	JobsScheduled     int64
	JobsSubmitted     int64
	LastScheduleCheck time.Time
	NextScheduledScan *time.Time
}

// NewScheduler creates a new job scheduler
func NewScheduler(ctx context.Context, config *config.Config, database *db.DB, pool *Pool) *Scheduler {
	schedulerCtx, cancel := context.WithCancel(ctx)

	return &Scheduler{
		config:         config,
		db:             database,
		pool:           pool,
		logger:         log.New(log.Writer(), "[scheduler] ", log.LstdFlags|log.Lshortfile),
		targets:        make(map[uuid.UUID]*ScheduledTarget),
		tickInterval:   30 * time.Second, // Check every 30 seconds
		ctx:            schedulerCtx,
		cancel:         cancel,
		scanTargetRepo: db.NewScanTargetRepository(database),
	}
}

// Start initializes and starts the scheduler
func (s *Scheduler) Start() error {
	s.logger.Println("Starting job scheduler...")

	// Load targets from database
	if err := s.loadTargets(); err != nil {
		return fmt.Errorf("failed to load targets: %w", err)
	}

	// Start the scheduling loop
	s.wg.Add(1)
	go s.schedulingLoop()

	s.logger.Printf("Job scheduler started with %d targets", len(s.targets))
	return nil
}

// Stop gracefully shuts down the scheduler
func (s *Scheduler) Stop() error {
	s.logger.Println("Stopping job scheduler...")

	// Signal shutdown
	s.cancel()

	// Stop timer if running
	if s.scheduleTimer != nil {
		s.scheduleTimer.Stop()
	}

	// Wait for scheduling loop to finish
	s.wg.Wait()

	s.logger.Println("Job scheduler stopped")
	return nil
}

// AddTarget adds a new target to the scheduler
func (s *Scheduler) AddTarget(target *db.ScanTarget) error {
	s.targetsMutex.Lock()
	defer s.targetsMutex.Unlock()

	scheduled := &ScheduledTarget{
		Target:   target,
		NextScan: s.calculateNextScan(target, nil),
		LastScan: nil,
		Enabled:  target.Enabled,
	}

	s.targets[target.ID] = scheduled
	s.logger.Printf("Added target %s (next scan: %v)", target.Name, scheduled.NextScan)

	return nil
}

// RemoveTarget removes a target from the scheduler
func (s *Scheduler) RemoveTarget(targetID uuid.UUID) error {
	s.targetsMutex.Lock()
	defer s.targetsMutex.Unlock()

	if target, exists := s.targets[targetID]; exists {
		delete(s.targets, targetID)
		s.logger.Printf("Removed target %s", target.Target.Name)
	}

	return nil
}

// UpdateTarget updates an existing target in the scheduler
func (s *Scheduler) UpdateTarget(target *db.ScanTarget) error {
	s.targetsMutex.Lock()
	defer s.targetsMutex.Unlock()

	if scheduled, exists := s.targets[target.ID]; exists {
		oldTarget := scheduled.Target
		scheduled.Target = target
		scheduled.Enabled = target.Enabled

		// Recalculate next scan if interval changed
		if oldTarget.ScanIntervalSeconds != target.ScanIntervalSeconds {
			scheduled.NextScan = s.calculateNextScan(target, scheduled.LastScan)
			s.logger.Printf("Updated target %s schedule (next scan: %v)", target.Name, scheduled.NextScan)
		}
	}

	return nil
}

// GetStats returns current scheduler statistics
func (s *Scheduler) GetStats() SchedulerStats {
	s.statsMutex.RLock()
	defer s.statsMutex.RUnlock()

	stats := s.stats

	// Count active/inactive targets
	s.targetsMutex.RLock()
	stats.TargetsActive = 0
	stats.TargetsInactive = 0
	var nextScan *time.Time

	for _, target := range s.targets {
		if target.Enabled {
			stats.TargetsActive++
			if nextScan == nil || target.NextScan.Before(*nextScan) {
				nextScan = &target.NextScan
			}
		} else {
			stats.TargetsInactive++
		}
	}
	s.targetsMutex.RUnlock()

	stats.NextScheduledScan = nextScan
	return stats
}

// GetTargets returns all scheduled targets
func (s *Scheduler) GetTargets() map[uuid.UUID]*ScheduledTarget {
	s.targetsMutex.RLock()
	defer s.targetsMutex.RUnlock()

	// Return a copy to avoid race conditions
	targets := make(map[uuid.UUID]*ScheduledTarget)
	for id, target := range s.targets {
		targetCopy := *target
		targets[id] = &targetCopy
	}

	return targets
}

// loadTargets loads all scan targets from the database
func (s *Scheduler) loadTargets() error {
	targets, err := s.scanTargetRepo.GetAll(s.ctx)
	if err != nil {
		return fmt.Errorf("failed to load targets from database: %w", err)
	}

	s.targetsMutex.Lock()
	defer s.targetsMutex.Unlock()

	for _, target := range targets {
		scheduled := &ScheduledTarget{
			Target:   target,
			NextScan: s.calculateNextScan(target, nil),
			LastScan: nil,
			Enabled:  target.Enabled,
		}
		s.targets[target.ID] = scheduled
	}

	s.logger.Printf("Loaded %d targets from database", len(targets))
	return nil
}

// schedulingLoop is the main scheduling loop
func (s *Scheduler) schedulingLoop() {
	defer s.wg.Done()
	s.logger.Println("Scheduling loop started")

	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkAndSchedule()

		case <-s.ctx.Done():
			s.logger.Println("Scheduling loop stopped")
			return
		}
	}
}

// checkAndSchedule checks for due scans and schedules them
func (s *Scheduler) checkAndSchedule() {
	now := time.Now()

	s.updateStats(func(stats *SchedulerStats) {
		stats.LastScheduleCheck = now
	})

	s.targetsMutex.Lock()
	defer s.targetsMutex.Unlock()

	var dueTargets []*ScheduledTarget

	// Find targets that are due for scanning
	for _, target := range s.targets {
		if !target.Enabled {
			continue
		}

		if target.NextScan.Before(now) || target.NextScan.Equal(now) {
			dueTargets = append(dueTargets, target)
		}
	}

	// Sort by priority (earliest scheduled first)
	sort.Slice(dueTargets, func(i, j int) bool {
		return dueTargets[i].NextScan.Before(dueTargets[j].NextScan)
	})

	// Schedule due targets
	for _, target := range dueTargets {
		if err := s.scheduleTarget(target); err != nil {
			s.logger.Printf("Failed to schedule target %s: %v", target.Target.Name, err)
			continue
		}

		// Update next scan time
		target.LastScan = &now
		target.NextScan = s.calculateNextScan(target.Target, &now)

		s.updateStats(func(stats *SchedulerStats) {
			stats.JobsScheduled++
		})
	}

	if len(dueTargets) > 0 {
		s.logger.Printf("Scheduled %d targets for scanning", len(dueTargets))
	}
}

// scheduleTarget schedules a single target for scanning
func (s *Scheduler) scheduleTarget(target *ScheduledTarget) error {
	// Submit job to worker pool
	job, err := s.pool.SubmitJob(target.Target)
	if err != nil {
		return fmt.Errorf("failed to submit job to worker pool: %w", err)
	}

	s.logger.Printf("Scheduled scan job %s for target %s", job.ID, target.Target.Name)

	s.updateStats(func(stats *SchedulerStats) {
		stats.JobsSubmitted++
	})

	return nil
}

// calculateNextScan calculates the next scan time for a target
func (s *Scheduler) calculateNextScan(target *db.ScanTarget, lastScan *time.Time) time.Time {
	interval := time.Duration(target.ScanIntervalSeconds) * time.Second

	if lastScan == nil {
		// For new targets, schedule immediately but with some jitter to avoid thundering herd
		jitter := time.Duration(target.ID.ID()%60) * time.Second
		return time.Now().Add(jitter)
	}

	return lastScan.Add(interval)
}

// RefreshTargets reloads targets from the database
func (s *Scheduler) RefreshTargets() error {
	s.logger.Println("Refreshing targets from database...")

	targets, err := s.scanTargetRepo.GetAll(s.ctx)
	if err != nil {
		return fmt.Errorf("failed to refresh targets: %w", err)
	}

	s.targetsMutex.Lock()
	defer s.targetsMutex.Unlock()

	// Track existing targets
	existingTargets := make(map[uuid.UUID]bool)
	for id := range s.targets {
		existingTargets[id] = true
	}

	// Update or add targets
	for _, target := range targets {
		if existing, exists := s.targets[target.ID]; exists {
			// Update existing target
			existing.Target = target
			existing.Enabled = target.Enabled
			delete(existingTargets, target.ID)
		} else {
			// Add new target
			scheduled := &ScheduledTarget{
				Target:   target,
				NextScan: s.calculateNextScan(target, nil),
				LastScan: nil,
				Enabled:  target.Enabled,
			}
			s.targets[target.ID] = scheduled
		}
	}

	// Remove targets that no longer exist
	for id := range existingTargets {
		if target, exists := s.targets[id]; exists {
			s.logger.Printf("Removing deleted target %s", target.Target.Name)
			delete(s.targets, id)
		}
	}

	s.logger.Printf("Refreshed targets: %d active", len(s.targets))
	return nil
}

// GetNextScheduledTarget returns the target with the earliest next scan time
func (s *Scheduler) GetNextScheduledTarget() *ScheduledTarget {
	s.targetsMutex.RLock()
	defer s.targetsMutex.RUnlock()

	var earliest *ScheduledTarget
	for _, target := range s.targets {
		if !target.Enabled {
			continue
		}

		if earliest == nil || target.NextScan.Before(earliest.NextScan) {
			earliest = target
		}
	}

	return earliest
}

// GetTargetByID returns a specific target by ID
func (s *Scheduler) GetTargetByID(id uuid.UUID) *ScheduledTarget {
	s.targetsMutex.RLock()
	defer s.targetsMutex.RUnlock()

	if target, exists := s.targets[id]; exists {
		targetCopy := *target
		return &targetCopy
	}

	return nil
}

// ForceSchedule forces an immediate scan of a specific target
func (s *Scheduler) ForceSchedule(targetID uuid.UUID) error {
	s.targetsMutex.Lock()
	defer s.targetsMutex.Unlock()

	target, exists := s.targets[targetID]
	if !exists {
		return fmt.Errorf("target not found: %s", targetID)
	}

	if !target.Enabled {
		return fmt.Errorf("target is disabled: %s", target.Target.Name)
	}

	if err := s.scheduleTarget(target); err != nil {
		return fmt.Errorf("failed to force schedule target: %w", err)
	}

	// Update schedule
	now := time.Now()
	target.LastScan = &now
	target.NextScan = s.calculateNextScan(target.Target, &now)

	s.logger.Printf("Force scheduled target %s", target.Target.Name)
	return nil
}

// updateStats safely updates scheduler statistics
func (s *Scheduler) updateStats(updater func(*SchedulerStats)) {
	s.statsMutex.Lock()
	defer s.statsMutex.Unlock()
	updater(&s.stats)
}

// GetOverdueTargets returns targets that are overdue for scanning
func (s *Scheduler) GetOverdueTargets() []*ScheduledTarget {
	s.targetsMutex.RLock()
	defer s.targetsMutex.RUnlock()

	now := time.Now()
	var overdue []*ScheduledTarget

	for _, target := range s.targets {
		if !target.Enabled {
			continue
		}

		if target.NextScan.Before(now) {
			targetCopy := *target
			overdue = append(overdue, &targetCopy)
		}
	}

	// Sort by how overdue they are (most overdue first)
	sort.Slice(overdue, func(i, j int) bool {
		return overdue[i].NextScan.Before(overdue[j].NextScan)
	})

	return overdue
}
