// Package db provides typed repository for scan profile statistics operations.
package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/anstrom/scanorama/internal/errors"
)

// ProfileStats holds effectiveness statistics for a single scan profile.
type ProfileStats struct {
	ProfileID     string     `json:"profile_id"`
	TotalScans    int        `json:"total_scans"`
	UniqueHosts   int        `json:"unique_hosts"`
	LastUsed      *time.Time `json:"last_used"`
	AvgHostsFound *float64   `json:"avg_hosts_found"`
}

// GetProfileStats returns scan effectiveness statistics for a profile.
// It queries scan_jobs for the given profile ID and aggregates:
//   - total number of scans
//   - count of distinct networks scanned (proxy for unique hosts)
//   - MAX(created_at) as last_used
//   - average of scan_stats->>'hosts_up' as avg_hosts_found
//
// If the profile does not exist in scan_profiles a 404 error is returned.
// If the profile exists but has no scans, all counters are zero / null and a
// 200 response is returned (not 404).
func (r *ProfileRepository) GetProfileStats(ctx context.Context, profileID string) (*ProfileStats, error) {
	// Verify profile exists first so we can distinguish 404 from zero-scan case.
	var exists bool
	if err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM scan_profiles WHERE id = $1)`,
		profileID,
	).Scan(&exists); err != nil {
		return nil, sanitizeDBError("check profile existence", err)
	}
	if !exists {
		return nil, errors.ErrNotFoundWithID("profile", profileID)
	}

	// Aggregate scan_jobs rows for this profile.
	// avg_hosts_found: average of scan_stats->>'hosts_up' cast to integer.
	// We skip rows where the field is absent or non-numeric (NULLIF avoids
	// division-by-zero; the FILTER clause discards NULLs before the average).
	query := `
		SELECT
			COUNT(*)                                                       AS total_scans,
			COUNT(DISTINCT network_id)                                     AS unique_hosts,
			MAX(created_at)                                                AS last_used,
			AVG(
				NULLIF((scan_stats->>'hosts_up')::TEXT, '')::INTEGER
			) FILTER (WHERE scan_stats->>'hosts_up' IS NOT NULL)          AS avg_hosts_found
		FROM scan_jobs
		WHERE profile_id = $1`

	var stats ProfileStats
	stats.ProfileID = profileID

	var lastUsed sql.NullTime
	var avgHostsFound sql.NullFloat64

	err := r.db.QueryRowContext(ctx, query, profileID).Scan(
		&stats.TotalScans,
		&stats.UniqueHosts,
		&lastUsed,
		&avgHostsFound,
	)
	if err != nil {
		return nil, sanitizeDBError("get profile stats", err)
	}

	if lastUsed.Valid {
		t := lastUsed.Time
		stats.LastUsed = &t
	}
	if avgHostsFound.Valid {
		v := avgHostsFound.Float64
		stats.AvgHostsFound = &v
	}

	return &stats, nil
}
