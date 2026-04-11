// Package scanning - post-scan hook for Smart Scan re-evaluation.
package scanning

import (
	"sync"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/google/uuid"
)

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
