// Package dns provides cached DNS resolution for scanorama.
//
// It supports both forward lookups (hostname → []IP) and reverse lookups
// (IP → hostname) backed by the dns_cache database table.  Every result —
// including negative ones — is stored so that repeated queries hit the
// database instead of the OS resolver.
//
// TTL behavior
// -------------
// Each cache row carries its own ttl_seconds value.  Two TTLs are in play:
//
//   - TTL (default 1 h)          — used for successful positive results.
//   - NegativeTTL (default 5 min) — used for NXDOMAIN / empty answers and
//     resolver errors, so transient failures do not get pinned for an hour.
//
// Both can be overridden at construction time via WithTTL / WithNegativeTTL.
//
// Cache invalidation
// ------------------
// Entries expire passively when their TTL window closes; the next lookup for
// that key then triggers a fresh network query.  Two explicit invalidation
// methods are also provided:
//
//   - Invalidate(ctx, direction, key) — removes all rows for one key.
//   - InvalidateAll(ctx)              — truncates the entire dns_cache table.
//
// RefreshStale can be called from a background goroutine or scheduled job to
// proactively re-resolve entries whose TTL has already expired.
//
// Concurrency
// -----------
// Resolver is safe for concurrent use.  A singleflight group ensures that
// simultaneous callers for the same key share one in-flight network lookup.
package dns

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
)

// Direction distinguishes the two lookup directions stored in dns_cache.
type Direction string

const (
	DirectionForward Direction = "forward" // hostname → IP
	DirectionReverse Direction = "reverse" // IP       → hostname
)

// DefaultTTL is used for successful positive cache entries when the caller
// does not supply an explicit TTL via WithTTL.
const DefaultTTL = time.Hour

// DefaultNegativeTTL is used for NXDOMAIN / empty-answer / resolver-error
// entries.  It is intentionally short so transient failures resolve quickly.
const DefaultNegativeTTL = 5 * time.Minute

// ErrNoRecords is returned by a Lookup method when the resolver returned a
// successful response that contained no usable records (NXDOMAIN / empty
// answer section).
var ErrNoRecords = errors.New("dns: lookup returned no records")

// entry is one row from the dns_cache table.
type entry struct {
	ID            uuid.UUID `db:"id"`
	Direction     Direction `db:"direction"`
	LookupKey     string    `db:"lookup_key"`
	ResolvedValue string    `db:"resolved_value"`
	ResolvedAt    time.Time `db:"resolved_at"`
	TTLSeconds    int       `db:"ttl_seconds"`
	LastError     *string   `db:"last_error"`
}

// fresh reports whether the entry is still within its TTL window.
func (e *entry) fresh() bool {
	return time.Since(e.ResolvedAt) < time.Duration(e.TTLSeconds)*time.Second
}

// Resolver performs cached DNS lookups.
type Resolver struct {
	db          *db.DB
	net         *net.Resolver // injectable for tests
	logger      *slog.Logger
	ttl         time.Duration
	negativeTTL time.Duration

	// lookupHostFn and lookupAddrFn can be replaced in tests to avoid
	// touching the network. When nil the real net.Resolver is used.
	lookupHostFn func(ctx context.Context, host string) ([]string, error)
	lookupAddrFn func(ctx context.Context, ip string) ([]string, error)

	// singleflight: sfKey → *call
	mu    sync.Mutex
	calls map[string]*call
}

// call represents an in-flight or recently completed singleflight lookup.
type call struct {
	wg  sync.WaitGroup
	val []string
	err error
}

// Option configures a Resolver.
type Option func(*Resolver)

// WithTTL overrides the default TTL for successful positive cache entries.
func WithTTL(d time.Duration) Option {
	return func(r *Resolver) { r.ttl = d }
}

// WithNegativeTTL overrides the default TTL used for NXDOMAIN / empty-answer
// / resolver-error cache entries.
func WithNegativeTTL(d time.Duration) Option {
	return func(r *Resolver) { r.negativeTTL = d }
}

// WithNetResolver replaces the underlying net.Resolver (useful in tests that
// want to verify the Option wiring without injecting lookupHostFn directly).
func WithNetResolver(nr *net.Resolver) Option {
	return func(r *Resolver) { r.net = nr }
}

// WithLogger sets the structured logger used for warnings and debug output.
func WithLogger(l *slog.Logger) Option {
	return func(r *Resolver) { r.logger = l }
}

// New creates a Resolver backed by the given database connection.
func New(database *db.DB, opts ...Option) *Resolver {
	r := &Resolver{
		db:          database,
		net:         net.DefaultResolver,
		logger:      slog.Default(),
		ttl:         DefaultTTL,
		negativeTTL: DefaultNegativeTTL,
		calls:       make(map[string]*call),
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// ─── public API ───────────────────────────────────────────────────────────────

// LookupHost resolves a hostname to one or more IP address strings.
// The cache is consulted first; a live lookup is only performed when all
// cached entries are stale or absent.
//
// On success the returned slice contains at least one element.
// ErrNoRecords is returned when the resolver returned an empty answer.
func (r *Resolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return nil, fmt.Errorf("dns: empty hostname")
	}

	// Fast path: check cache.
	cached, err := r.getCachedEntries(ctx, DirectionForward, host)
	if err != nil {
		r.logger.Warn("dns cache read failed", "direction", "forward", "key", host, "error", err)
		// Non-fatal: fall through to live lookup.
	}
	if len(cached) > 0 && cached[0].fresh() {
		vals := extractValues(cached)
		if len(vals) == 0 {
			// Fresh negative-cache entry.
			return nil, ErrNoRecords
		}
		return vals, nil
	}

	// Slow path: deduplicate concurrent callers via singleflight.
	sfKey := "forward:" + host
	return r.do(sfKey, func() ([]string, error) {
		return r.resolveForward(ctx, host)
	})
}

// LookupAddr performs a reverse PTR lookup for the given IP address and
// returns the first hostname (trailing dot stripped).
// Results are cached; ErrNoRecords is returned for empty / NXDOMAIN responses.
func (r *Resolver) LookupAddr(ctx context.Context, ip string) (string, error) {
	ip = strings.TrimSpace(ip)
	if net.ParseIP(ip) == nil {
		return "", fmt.Errorf("dns: %q is not a valid IP address", ip)
	}

	// Fast path: check cache.
	cached, err := r.getCachedEntries(ctx, DirectionReverse, ip)
	if err != nil {
		r.logger.Warn("dns cache read failed", "direction", "reverse", "key", ip, "error", err)
	}
	if len(cached) > 0 && cached[0].fresh() {
		if cached[0].ResolvedValue == "" {
			return "", ErrNoRecords
		}
		return cached[0].ResolvedValue, nil
	}

	// Slow path.
	sfKey := "reverse:" + ip
	results, err := r.do(sfKey, func() ([]string, error) {
		return r.resolveReverse(ctx, ip)
	})
	if err != nil {
		return "", err
	}
	// resolveReverse always returns either (nil, err) or ([]string{primary}, nil),
	// so results is guaranteed non-empty here.
	return results[0], nil
}

// Invalidate removes all dns_cache entries for the given direction and key,
// forcing the next lookup to perform a fresh network query.
//
// This is useful when an external event (e.g. a DHCP lease change or a manual
// host update) makes the cached value known-stale before its TTL expires.
func (r *Resolver) Invalidate(ctx context.Context, direction Direction, key string) error {
	const query = `DELETE FROM dns_cache WHERE direction = $1 AND lookup_key = $2`
	_, err := r.db.ExecContext(ctx, query, string(direction), key)
	if err != nil {
		return fmt.Errorf("dns: invalidate %s %q: %w", direction, key, err)
	}
	r.logger.Debug("dns: invalidated cache entry", "direction", direction, "key", key)
	return nil
}

// InvalidateAll truncates the entire dns_cache table.  All subsequent lookups
// will hit the network until their results are re-cached.
//
// Use with care — this is a heavy operation intended for administrative
// resets, not routine use.
func (r *Resolver) InvalidateAll(ctx context.Context) error {
	const query = `DELETE FROM dns_cache`
	if _, err := r.db.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("dns: invalidate all: %w", err)
	}
	r.logger.Info("dns: entire cache invalidated")
	return nil
}

// RefreshStale finds all dns_cache rows whose TTL has expired and
// re-resolves them.  It is intended to be called from a background goroutine
// or scheduled job; it respects the supplied context for cancellation.
func (r *Resolver) RefreshStale(ctx context.Context) error {
	const query = `
		SELECT id, direction, lookup_key, resolved_value, resolved_at,
		       ttl_seconds, last_error
		FROM dns_cache
		WHERE resolved_at + (ttl_seconds * interval '1 second') < NOW()
		ORDER BY resolved_at ASC
		LIMIT 500`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return fmt.Errorf("dns: failed to query stale entries: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			r.logger.Warn("dns: failed to close stale query rows", "error", closeErr)
		}
	}()

	// Collect unique (direction, lookup_key) pairs — a single hostname can
	// have multiple resolved_value rows (one per A record) and we only want
	// to re-resolve each key once.
	type workItem struct {
		direction Direction
		key       string
	}
	seen := make(map[string]struct{})
	var work []workItem

	for rows.Next() {
		var e entry
		if scanErr := rows.Scan(
			&e.ID, &e.Direction, &e.LookupKey, &e.ResolvedValue,
			&e.ResolvedAt, &e.TTLSeconds, &e.LastError,
		); scanErr != nil {
			r.logger.Warn("dns: failed to scan stale row", "error", scanErr)
			continue
		}
		dedupeKey := string(e.Direction) + ":" + e.LookupKey
		if _, exists := seen[dedupeKey]; exists {
			continue
		}
		seen[dedupeKey] = struct{}{}
		work = append(work, workItem{e.Direction, e.LookupKey})
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("dns: row iteration error: %w", err)
	}

	for _, item := range work {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var lookupErr error
		switch item.direction {
		case DirectionForward:
			_, lookupErr = r.LookupHost(ctx, item.key)
		case DirectionReverse:
			_, lookupErr = r.LookupAddr(ctx, item.key)
		}
		if lookupErr != nil && !errors.Is(lookupErr, ErrNoRecords) {
			r.logger.Warn("dns: stale refresh failed",
				"direction", item.direction, "key", item.key, "error", lookupErr)
		}
	}
	return nil
}

// ─── internal helpers ─────────────────────────────────────────────────────────

// do deduplicates concurrent callers for the same sfKey via a lightweight
// singleflight: the first caller performs the work while subsequent callers
// block and share the result.
func (r *Resolver) do(sfKey string, fn func() ([]string, error)) ([]string, error) {
	r.mu.Lock()
	if c, ok := r.calls[sfKey]; ok {
		r.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err
	}
	c := &call{}
	c.wg.Add(1)
	r.calls[sfKey] = c
	r.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	r.mu.Lock()
	delete(r.calls, sfKey)
	r.mu.Unlock()

	return c.val, c.err
}

// resolveForward performs a live A/AAAA lookup and caches every returned IP.
func (r *Resolver) resolveForward(ctx context.Context, host string) ([]string, error) {
	lookupFn := r.lookupHostFn
	if lookupFn == nil {
		lookupFn = r.net.LookupHost
	}
	addrs, lookupErr := lookupFn(ctx, host)

	if lookupErr != nil {
		errStr := lookupErr.Error()
		_ = r.upsertEntry(ctx, &entry{
			Direction:     DirectionForward,
			LookupKey:     host,
			ResolvedValue: "",
			TTLSeconds:    int(r.negativeTTL.Seconds()),
			LastError:     &errStr,
		})
		return nil, fmt.Errorf("dns: forward lookup for %q: %w", host, lookupErr)
	}

	if len(addrs) == 0 {
		emptyMsg := "no records returned"
		_ = r.upsertEntry(ctx, &entry{
			Direction:     DirectionForward,
			LookupKey:     host,
			ResolvedValue: "",
			TTLSeconds:    int(r.negativeTTL.Seconds()),
			LastError:     &emptyMsg,
		})
		return nil, ErrNoRecords
	}

	ttlSec := int(r.ttl.Seconds())
	for _, addr := range addrs {
		_ = r.upsertEntry(ctx, &entry{
			Direction:     DirectionForward,
			LookupKey:     host,
			ResolvedValue: addr,
			TTLSeconds:    ttlSec,
			LastError:     nil,
		})
	}

	r.logger.Debug("dns: forward lookup complete", "host", host, "addrs", addrs)
	return addrs, nil
}

// resolveReverse performs a live PTR lookup and caches the primary result.
func (r *Resolver) resolveReverse(ctx context.Context, ip string) ([]string, error) {
	lookupFn := r.lookupAddrFn
	if lookupFn == nil {
		lookupFn = r.net.LookupAddr
	}
	names, lookupErr := lookupFn(ctx, ip)

	if lookupErr != nil {
		errStr := lookupErr.Error()
		_ = r.upsertEntry(ctx, &entry{
			Direction:     DirectionReverse,
			LookupKey:     ip,
			ResolvedValue: "",
			TTLSeconds:    int(r.negativeTTL.Seconds()),
			LastError:     &errStr,
		})
		return nil, fmt.Errorf("dns: reverse lookup for %q: %w", ip, lookupErr)
	}

	// Strip trailing dots that some resolvers leave on PTR records.
	cleaned := make([]string, 0, len(names))
	for _, n := range names {
		cleaned = append(cleaned, strings.TrimSuffix(n, "."))
	}

	primary := ""
	if len(cleaned) > 0 {
		primary = cleaned[0]
	}

	ttlSec := int(r.ttl.Seconds())
	if primary == "" {
		ttlSec = int(r.negativeTTL.Seconds())
	}

	_ = r.upsertEntry(ctx, &entry{
		Direction:     DirectionReverse,
		LookupKey:     ip,
		ResolvedValue: primary,
		TTLSeconds:    ttlSec,
		LastError:     nil,
	})

	r.logger.Debug("dns: reverse lookup complete", "ip", ip, "names", cleaned)

	if primary == "" {
		return nil, ErrNoRecords
	}
	return []string{primary}, nil
}

// getCachedEntries returns all dns_cache rows matching direction + key,
// sorted by resolved_at descending so the freshest entry is first.
func (r *Resolver) getCachedEntries(ctx context.Context, dir Direction, key string) ([]*entry, error) {
	const query = `
		SELECT id, direction, lookup_key, resolved_value, resolved_at,
		       ttl_seconds, last_error
		FROM dns_cache
		WHERE direction = $1 AND lookup_key = $2
		ORDER BY resolved_at DESC`

	rows, err := r.db.QueryContext(ctx, query, string(dir), key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			r.logger.Warn("dns: failed to close cache query rows", "error", closeErr)
		}
	}()

	var entries []*entry
	for rows.Next() {
		e := &entry{}
		if scanErr := rows.Scan(
			&e.ID, &e.Direction, &e.LookupKey, &e.ResolvedValue,
			&e.ResolvedAt, &e.TTLSeconds, &e.LastError,
		); scanErr != nil {
			return nil, scanErr
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// upsertEntry inserts or updates a single dns_cache row.
// On conflict (direction, lookup_key, resolved_value) it refreshes
// resolved_at, ttl_seconds, and last_error in-place.
func (r *Resolver) upsertEntry(ctx context.Context, e *entry) error {
	const query = `
		INSERT INTO dns_cache
		    (id, direction, lookup_key, resolved_value, resolved_at, ttl_seconds, last_error)
		VALUES
		    ($1, $2, $3, $4, NOW(), $5, $6)
		ON CONFLICT ON CONSTRAINT uq_dns_cache_entry DO UPDATE SET
		    resolved_at  = EXCLUDED.resolved_at,
		    ttl_seconds  = EXCLUDED.ttl_seconds,
		    last_error   = EXCLUDED.last_error`

	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}

	_, err := r.db.ExecContext(ctx, query,
		e.ID,
		string(e.Direction),
		e.LookupKey,
		e.ResolvedValue,
		e.TTLSeconds,
		e.LastError,
	)
	if err != nil {
		r.logger.Warn("dns: failed to upsert cache entry",
			"direction", e.Direction,
			"key", e.LookupKey,
			"value", e.ResolvedValue,
			"error", err)
	}
	return err
}

// extractValues pulls the ResolvedValue field from a slice of entries,
// filtering out empty strings (negative-cache sentinels).
func extractValues(entries []*entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.ResolvedValue != "" {
			out = append(out, e.ResolvedValue)
		}
	}
	return out
}
