// Package db provides typed repository for database operations.
package db

import (
	"context"
	"fmt"
	"log/slog"
)

// staleJobMessage is written to scan_jobs.error_message for every job that
// was found in the 'running' state when the server started — indicating the
// job was interrupted by a prior crash or restart rather than failing on its
// own terms.
const staleJobMessage = "interrupted by server restart"

// RecoveryResult reports how many jobs were transitioned out of the 'running'
// state during startup recovery.
type RecoveryResult struct {
	ScanJobsRecovered      int
	DiscoveryJobsRecovered int
}

// Total returns the combined number of recovered jobs across all tables.
func (r RecoveryResult) Total() int {
	return r.ScanJobsRecovered + r.DiscoveryJobsRecovered
}

// RecoverStaleJobs transitions every job that is stuck in the 'running' state
// back to 'failed', stamping completed_at = NOW().  It must be called once
// after the database connection is established and before the HTTP listener
// opens so that:
//
//   - Clients never see a job that is permanently 'running' but has no live
//     goroutine behind it.
//   - The job can be retried or re-queued by the operator / scheduler.
//
// The function is intentionally idempotent: if there are no stale jobs both
// result counts are zero and no error is returned.
//
// Errors from either UPDATE are returned immediately; the second UPDATE is
// skipped if the first fails, so the caller can decide whether to abort
// startup or continue with a warning.
func RecoverStaleJobs(ctx context.Context, database *DB) (RecoveryResult, error) {
	if database == nil {
		return RecoveryResult{}, nil
	}

	var result RecoveryResult

	// ── scan_jobs ──────────────────────────────────────────────────────────
	scanRes, err := database.ExecContext(ctx, `
		UPDATE scan_jobs
		SET    status        = 'failed',
		       completed_at  = NOW(),
		       error_message = $1
		WHERE  status = 'running'
	`, staleJobMessage)
	if err != nil {
		return result, fmt.Errorf("recover stale scan jobs: %w", err)
	}
	if n, err := scanRes.RowsAffected(); err == nil {
		result.ScanJobsRecovered = int(n)
	}

	// ── discovery_jobs ─────────────────────────────────────────────────────
	discRes, err := database.ExecContext(ctx, `
		UPDATE discovery_jobs
		SET    status       = 'failed',
		       completed_at = NOW()
		WHERE  status = 'running'
	`)
	if err != nil {
		return result, fmt.Errorf("recover stale discovery jobs: %w", err)
	}
	if n, err := discRes.RowsAffected(); err == nil {
		result.DiscoveryJobsRecovered = int(n)
	}

	// Only emit a log line when there was actually something to recover; a
	// clean startup should stay silent.
	if result.Total() > 0 {
		slog.Warn("Recovered stale jobs interrupted by previous restart",
			"scan_jobs", result.ScanJobsRecovered,
			"discovery_jobs", result.DiscoveryJobsRecovered,
		)
	}

	return result, nil
}
