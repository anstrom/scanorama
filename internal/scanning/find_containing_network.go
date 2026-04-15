// Package scanning — helper that resolves a scan target to the registered
// network (if any) that should be stored as scan_jobs.network_id.
package scanning

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
)

// findContainingNetwork returns the UUID of the most-specific registered
// network whose CIDR contains target (passed as any valid PostgreSQL inet
// literal — a bare IP, /32, /128, or broader CIDR).  Returns uuid.Nil when
// no registered network contains the target; that is not an error, it is
// the common case for ad-hoc scans.  Returns a non-nil error only for
// genuine DB failures.
//
// Longest-prefix-match semantics: if both 10.0.0.0/16 and 10.0.0.0/24 are
// registered and target is 10.0.0.5, the /24 wins.
//
// findContainingNetworkSQL is exported to the package so tests can reference
// the exact same string under sqlmock.QueryMatcherEqual; drift becomes a
// compile error instead of a cryptic runtime mismatch.
const findContainingNetworkSQL = `SELECT id FROM networks WHERE $1::inet <<= cidr ORDER BY masklen(cidr) DESC LIMIT 1`

func findContainingNetwork(ctx context.Context, database *db.DB, target string) (uuid.UUID, error) {
	var id uuid.UUID
	err := database.QueryRowContext(ctx, findContainingNetworkSQL, target).Scan(&id)
	if err == nil {
		return id, nil
	}
	if stderrors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, nil
	}
	return uuid.Nil, fmt.Errorf("failed to look up containing network for %q: %w", target, err)
}
