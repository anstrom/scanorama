package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/anstrom/scanorama/internal/db"
)

// portResolverTimeout caps all DB queries inside Resolve.
const portResolverTimeout = 3 * time.Second

// defaultTopPortsLimit is used when the settings key is 0 or missing.
const defaultTopPortsLimit = 256

// maxPortNumber is the highest valid TCP/UDP port number.
const maxPortNumber = 65535

// stageDefaultPorts maps each stage name to its hardcoded fallback port string.
// Values are the single-source constants defined in smartscan.go.
var stageDefaultPorts = map[string]string{
	"os_detection":        osDetectionPorts,
	"identity_enrichment": identityEnrichmentPorts,
	"refresh":             refreshPorts,
}

// portRangeRE detects nmap range notation (e.g. "1-1024") inside a port string.
var portRangeRE = regexp.MustCompile(`\b\d+-\d+\b`)

// portListResolverIface is satisfied by PortListResolver and test stubs.
// Resolve is fail-open: it always returns a non-empty port string, logging
// any DB errors internally rather than surfacing them to callers.
type portListResolverIface interface {
	Resolve(ctx context.Context, stage string, host *db.Host) string
}

// PortListResolver builds a merged, deduplicated port string for a SmartScan stage
// by combining three sources:
//  1. Operator-configured base ports from the settings table (Proposal C).
//  2. OS-matched curated ports from port_definitions (Proposal B).
//  3. Fleet top-N open ports from port_scans (Proposal A).
type PortListResolver struct {
	db     *db.DB
	logger *slog.Logger
}

// NewPortListResolver creates a PortListResolver backed by the given database.
// If logger is nil, slog.Default() is used.
func NewPortListResolver(database *db.DB, logger *slog.Logger) *PortListResolver {
	if logger == nil {
		logger = slog.Default()
	}
	return &PortListResolver{db: database, logger: logger}
}

// Resolve returns the merged port string for the given stage and host.
// If the base string contains a port range (e.g. "1-1024") it is returned
// unchanged — ranges are already broad enough that augmentation is redundant.
// Base ports are always preserved; augmentation fills remaining slots up to limit.
// All DB errors are logged and skipped (fail-open).
func (r *PortListResolver) Resolve(ctx context.Context, stage string, host *db.Host) string {
	ctx, cancel := context.WithTimeout(ctx, portResolverTimeout)
	defer cancel()

	base := r.readBasePorts(ctx, stage)
	if portRangeRE.MatchString(base) {
		return base
	}

	limit := r.readLimit(ctx)
	basePorts := parseCSVPorts(base)

	var augPorts []int
	if host.OSFamily != nil && *host.OSFamily != "" {
		augPorts = append(augPorts, r.queryOSPorts(ctx, *host.OSFamily, limit)...)
	}
	augPorts = append(augPorts, r.queryFleetTopPorts(ctx, limit)...)

	return mergeWithBase(basePorts, augPorts, limit)
}

// readBasePorts returns the base port string for the stage from settings,
// falling back to the hardcoded default if the key is absent or empty.
// Settings values are JSONB; string values arrive JSON-quoted ("\"22,80\"").
func (r *PortListResolver) readBasePorts(ctx context.Context, stage string) string {
	fallback, ok := stageDefaultPorts[stage]
	if !ok {
		r.logger.Warn("unknown stage passed to port resolver, using broad fallback", "stage", stage)
		fallback = "1-1024"
	}

	key := "smartscan." + stage + ".ports"
	var raw string
	if err := r.db.QueryRowContext(ctx,
		`SELECT value::text FROM settings WHERE key = $1`, key,
	).Scan(&raw); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			r.logger.Warn("failed to read base port setting, using default", "key", key, "error", err)
		}
		return fallback
	}

	var s string
	if err := json.Unmarshal([]byte(raw), &s); err == nil && s != "" {
		return s
	}
	if raw != "" {
		return raw
	}
	return fallback
}

// readLimit returns the top_ports_limit setting, defaulting to 256.
func (r *PortListResolver) readLimit(ctx context.Context) int {
	const key = "smartscan.top_ports_limit"
	var raw string
	if err := r.db.QueryRowContext(ctx,
		`SELECT value::text FROM settings WHERE key = $1`, key,
	).Scan(&raw); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			r.logger.Warn("failed to read top_ports_limit setting, using default", "error", err)
		}
		return defaultTopPortsLimit
	}
	n, err := strconv.Atoi(strings.Trim(raw, `"`))
	if err != nil {
		r.logger.Warn("smartscan.top_ports_limit is not a valid integer, using default", "raw", raw)
		return defaultTopPortsLimit
	}
	if n <= 0 {
		r.logger.Warn("smartscan.top_ports_limit must be positive, using default", "value", n)
		return defaultTopPortsLimit
	}
	return n
}

// queryOSPorts returns TCP ports from port_definitions associated with the
// given OS family, ordered by is_standard DESC then port ASC.
func (r *PortListResolver) queryOSPorts(ctx context.Context, osFamily string, limit int) []int {
	rows, err := r.db.QueryContext(ctx, `
		SELECT port
		FROM port_definitions
		WHERE os_families @> ARRAY[$1]::text[]
		  AND protocol = 'tcp'
		ORDER BY is_standard DESC, port ASC
		LIMIT $2`, osFamily, limit)
	if err != nil {
		r.logger.Warn("failed to query OS port definitions, skipping source", "os_family", osFamily, "error", err)
		return nil
	}
	defer func() { _ = rows.Close() }()

	var ports []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			r.logger.Warn("failed to scan OS port row, skipping", "error", err)
			continue
		}
		ports = append(ports, p)
	}
	if err := rows.Err(); err != nil {
		// Discard ports accumulated so far — partial lists are less safe than
		// skipping the source entirely (fail-open: caller still has other sources).
		r.logger.Warn("error iterating OS port rows, source skipped", "os_family", osFamily, "error", err)
		return nil
	}
	return ports
}

// queryFleetTopPorts returns the top-N TCP ports most commonly seen open across
// the fleet, ordered by distinct host count descending.
func (r *PortListResolver) queryFleetTopPorts(ctx context.Context, limit int) []int {
	rows, err := r.db.QueryContext(ctx, `
		SELECT port
		FROM port_scans
		WHERE state = 'open'
		  AND protocol = 'tcp'
		GROUP BY port
		ORDER BY COUNT(DISTINCT host_id) DESC
		LIMIT $1`, limit)
	if err != nil {
		r.logger.Warn("failed to query fleet top ports, skipping source", "error", err)
		return nil
	}
	defer func() { _ = rows.Close() }()

	var ports []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			r.logger.Warn("failed to scan fleet port row, skipping", "error", err)
			continue
		}
		ports = append(ports, p)
	}
	if err := rows.Err(); err != nil {
		// Discard ports accumulated so far — partial lists are less safe than
		// skipping the source entirely (fail-open: caller still has other sources).
		r.logger.Warn("error iterating fleet port rows, source skipped", "error", err)
		return nil
	}
	return ports
}

// parseCSVPorts splits a comma-separated port string into integers.
// Non-numeric tokens (ranges, labels) are silently skipped.
func parseCSVPorts(s string) []int {
	parts := strings.Split(s, ",")
	ports := make([]int, 0, len(parts))
	for _, tok := range parts {
		tok = strings.TrimSpace(tok)
		if n, err := strconv.Atoi(tok); err == nil && n > 0 && n <= maxPortNumber {
			ports = append(ports, n)
		}
	}
	return ports
}

// mergeWithBase deduplicates and sorts the merged result.
// Base ports are always preserved. Augmentation ports fill remaining slots up to
// limit (if > 0). This ensures operator-configured base ports survive the cap
// even when the augmented fleet/OS pool is large.
func mergeWithBase(base, aug []int, limit int) string {
	seen := make(map[int]struct{}, len(base)+len(aug))
	result := make([]int, 0, len(base)+len(aug))

	for _, p := range base {
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			result = append(result, p)
		}
	}
	for _, p := range aug {
		if limit > 0 && len(result) >= limit {
			break
		}
		if _, ok := seen[p]; !ok {
			seen[p] = struct{}{}
			result = append(result, p)
		}
	}

	sort.Ints(result)
	parts := make([]string, len(result))
	for i, p := range result {
		parts[i] = strconv.Itoa(p)
	}
	return strings.Join(parts, ",")
}

// Compile-time assertion that PortListResolver satisfies the interface.
var _ portListResolverIface = (*PortListResolver)(nil)
