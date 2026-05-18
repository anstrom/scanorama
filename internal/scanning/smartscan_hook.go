// Package scanning - post-scan hook for Smart Scan re-evaluation.
package scanning

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/google/uuid"
)

// alertEvalWindow is the look-back window used on the very first evaluation
// (before a cursor has been established).
const alertEvalWindow = 5 * time.Minute

// alertEvaluatorFn is the type for the alert evaluation hook callback.
type alertEvaluatorFn func(ctx context.Context, since time.Time) error

var (
	alertEvaluatorMu   sync.RWMutex
	alertEvaluatorFunc alertEvaluatorFn
	// lastAlertEvalAt is the timestamp of the last successful evaluation.
	// Zero value means never evaluated; the initial window falls back to
	// alertEvalWindow. Protected by alertEvaluatorMu.
	lastAlertEvalAt time.Time
)

// SetAlertEvaluator registers a callback that evaluates alert rules after
// each scan completes. Call once at application startup. Pass nil to clear.
func SetAlertEvaluator(fn alertEvaluatorFn) {
	alertEvaluatorMu.Lock()
	alertEvaluatorFunc = fn
	alertEvaluatorMu.Unlock()
}

// callAlertEvaluator invokes the registered alert evaluator, if any.
// Called as a goroutine after scan results are stored. Uses a persistent
// cursor so each transition is evaluated at most once across multiple scans.
func callAlertEvaluator() {
	alertEvaluatorMu.RLock()
	fn := alertEvaluatorFunc
	last := lastAlertEvalAt
	alertEvaluatorMu.RUnlock()
	if fn == nil {
		return
	}

	var since time.Time
	if last.IsZero() {
		since = time.Now().Add(-alertEvalWindow)
	} else {
		since = last
	}

	// Snapshot call time before invoking fn so any transition recorded
	// during this evaluation is included in the next window.
	callTime := time.Now()
	if err := fn(context.Background(), since); err != nil {
		slog.Error("alert evaluation failed", "error", err)
		return
	}

	alertEvaluatorMu.Lock()
	lastAlertEvalAt = callTime
	alertEvaluatorMu.Unlock()
}

// postScanHookFn is the type for post-scan hook callbacks.
type postScanHookFn func(database *db.DB, hostIDs []uuid.UUID)

var (
	postScanHookMu   sync.RWMutex
	postScanHookFunc postScanHookFn
)

// SetPostScanHook registers a callback to be called after scan results are
// stored and knowledge scores are queued for recalculation. Call once at
// application startup. Pass nil to clear.
func SetPostScanHook(fn postScanHookFn) {
	postScanHookMu.Lock()
	postScanHookFunc = fn
	postScanHookMu.Unlock()
}

// callPostScanHook invokes the registered hook if one is set.
// Safe for concurrent use from multiple goroutines.
func callPostScanHook(database *db.DB, hostIDs []uuid.UUID) {
	postScanHookMu.RLock()
	fn := postScanHookFunc
	postScanHookMu.RUnlock()
	if fn != nil {
		fn(database, hostIDs)
	}
}

// hasPostScanHook reports whether a hook is currently registered.
// Used by scan.go to decide whether to fall back to the built-in
// knowledge-score update goroutine.
func hasPostScanHook() bool {
	postScanHookMu.RLock()
	has := postScanHookFunc != nil
	postScanHookMu.RUnlock()
	return has
}
