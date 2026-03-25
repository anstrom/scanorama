// Package scanning provides core scanning functionality and shared types for scanorama.
// This file defines the Job interface and its concrete scan implementation so that
// the ScanQueue can execute any type of work — scans, discovery, or future job types —
// through a single unified worker pool.
package scanning

import (
	"context"
	"strings"

	"github.com/anstrom/scanorama/internal/db"
)

// Job is implemented by any unit of work the ScanQueue can execute.
// Both scan and discovery jobs implement this interface, allowing them to share
// the same worker pool and appear uniformly in the admin worker view.
type Job interface {
	// ID returns a stable identifier used for logging and deduplication.
	ID() string
	// Type returns the job category shown in the admin worker view (e.g. "scan", "discovery").
	Type() string
	// Target returns a human-readable description of the work target,
	// shown in the admin worker view (e.g. "192.168.1.0/24").
	Target() string
	// Execute runs the job to completion, respecting ctx for cancellation.
	Execute(ctx context.Context) error
}

// ScanJobExecutor is the function signature for the underlying nmap scan runner.
// RunScanWithContext is used in production; tests inject a lightweight stub.
type ScanJobExecutor func(ctx context.Context, cfg *ScanConfig, database *db.DB) (*ScanResult, error)

// ScanJob implements Job for nmap port-scan operations.
type ScanJob struct {
	id       string
	cfg      *ScanConfig
	database *db.DB
	executor ScanJobExecutor
	onDone   func(result *ScanResult, err error)
}

// NewScanJob constructs a ScanJob ready for submission to a ScanQueue.
// executor is called to perform the actual nmap scan; onDone is invoked with
// the outcome (result may be nil on error) when execution finishes.
func NewScanJob(
	id string,
	cfg *ScanConfig,
	database *db.DB,
	executor ScanJobExecutor,
	onDone func(*ScanResult, error),
) *ScanJob {
	return &ScanJob{
		id:       id,
		cfg:      cfg,
		database: database,
		executor: executor,
		onDone:   onDone,
	}
}

// ID implements Job.
func (j *ScanJob) ID() string { return j.id }

// Type implements Job.
func (j *ScanJob) Type() string { return "scan" }

// Target implements Job.
func (j *ScanJob) Target() string {
	if j.cfg == nil || len(j.cfg.Targets) == 0 {
		return ""
	}
	return strings.Join(j.cfg.Targets, ", ")
}

// Execute implements Job. It runs the nmap scan via the injected executor and
// then invokes onDone with the outcome. The error is also returned so the
// queue can update per-worker failure counters independently of onDone.
func (j *ScanJob) Execute(ctx context.Context) error {
	result, err := j.executor(ctx, j.cfg, j.database)
	if j.onDone != nil {
		j.onDone(result, err)
	}
	return err
}
