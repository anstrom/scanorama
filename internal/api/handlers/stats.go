// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements the StatsHandler for summary statistics endpoints.
package handlers

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/anstrom/scanorama/internal/db"
)

const statsQueryTimeout = 5 * time.Second

// StatsHandler handles statistics API endpoints.
type StatsHandler struct {
	db     *db.DB
	logger *slog.Logger
}

// NewStatsHandler creates a new StatsHandler.
func NewStatsHandler(database *db.DB, logger *slog.Logger) *StatsHandler {
	return &StatsHandler{
		db:     database,
		logger: logger.With("handler", "stats"),
	}
}

// KnowledgeScoreDistribution holds host counts across four score bands.
type KnowledgeScoreDistribution struct {
	Band0to25  int `json:"0_25"`
	Band25to50 int `json:"25_50"`
	Band50to75 int `json:"50_75"`
	Band75to100 int `json:"75_100"`
}

// StatsSummaryResponse holds the aggregated statistics summary.
type StatsSummaryResponse struct {
	HostsByStatus               map[string]int             `json:"hosts_by_status"`
	HostsByOSFamily             []OSFamilyCount            `json:"hosts_by_os_family"`
	TopPorts                    []PortCount                `json:"top_ports"`
	StaleHostCount              int                        `json:"stale_host_count"`
	AvgScanDurationS            float64                    `json:"avg_scan_duration_s"`
	AvgKnowledgeScore           float64                    `json:"avg_knowledge_score"`
	KnowledgeScoreDistribution  KnowledgeScoreDistribution `json:"knowledge_score_distribution"`
}

// OSFamilyCount holds a count for a given OS family.
type OSFamilyCount struct {
	Family string `json:"family"`
	Count  int    `json:"count"`
}

// PortCount holds a count for a given port.
type PortCount struct {
	Port  int `json:"port"`
	Count int `json:"count"`
}

// GetStatsSummary handles GET /api/v1/stats/summary.
func (h *StatsHandler) GetStatsSummary(w http.ResponseWriter, r *http.Request) {
	response := StatsSummaryResponse{
		HostsByStatus:   make(map[string]int),
		HostsByOSFamily: []OSFamilyCount{},
		TopPorts:        []PortCount{},
	}

	if err := h.fillHostsByStatus(r.Context(), response.HostsByStatus); err != nil {
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}
	osFamilies, err := h.queryOSFamilies(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}
	response.HostsByOSFamily = osFamilies

	topPorts, err := h.queryTopPorts(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}
	response.TopPorts = topPorts

	staleCount, err := h.queryStaleHostCount(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}
	response.StaleHostCount = staleCount

	avgDuration, err := h.queryAvgScanDuration(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}
	response.AvgScanDurationS = avgDuration

	avgScore, dist, err := h.queryKnowledgeScoreStats(r.Context())
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}
	response.AvgKnowledgeScore = avgScore
	response.KnowledgeScoreDistribution = dist

	writeJSON(w, r, http.StatusOK, response)
}

func (h *StatsHandler) fillHostsByStatus(ctx context.Context, out map[string]int) error {
	ctx, cancel := context.WithTimeout(ctx, statsQueryTimeout)
	defer cancel()

	rows, err := h.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM hosts GROUP BY status`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return err
		}
		out[status] = count
	}
	return rows.Err()
}

func (h *StatsHandler) queryOSFamilies(ctx context.Context) ([]OSFamilyCount, error) {
	ctx, cancel := context.WithTimeout(ctx, statsQueryTimeout)
	defer cancel()

	rows, err := h.db.QueryContext(ctx,
		`SELECT os_family, COUNT(*) as cnt FROM hosts
		WHERE os_family IS NOT NULL AND os_family != ''
		GROUP BY os_family ORDER BY cnt DESC LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []OSFamilyCount
	for rows.Next() {
		var entry OSFamilyCount
		if err := rows.Scan(&entry.Family, &entry.Count); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []OSFamilyCount{}
	}
	return result, nil
}

func (h *StatsHandler) queryTopPorts(ctx context.Context) ([]PortCount, error) {
	ctx, cancel := context.WithTimeout(ctx, statsQueryTimeout)
	defer cancel()

	rows, err := h.db.QueryContext(ctx,
		`SELECT port, COUNT(*) as cnt FROM port_scans
		WHERE state = 'open' GROUP BY port ORDER BY cnt DESC LIMIT 5`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var result []PortCount
	for rows.Next() {
		var entry PortCount
		if err := rows.Scan(&entry.Port, &entry.Count); err != nil {
			return nil, err
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if result == nil {
		result = []PortCount{}
	}
	return result, nil
}

func (h *StatsHandler) queryStaleHostCount(ctx context.Context) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, statsQueryTimeout)
	defer cancel()

	var count int
	row := h.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM hosts
		WHERE last_seen < NOW() - INTERVAL '7 days' AND status != 'gone'`)
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (h *StatsHandler) queryAvgScanDuration(ctx context.Context) (float64, error) {
	ctx, cancel := context.WithTimeout(ctx, statsQueryTimeout)
	defer cancel()

	var avg sql.NullFloat64
	row := h.db.QueryRowContext(ctx,
		`SELECT AVG(EXTRACT(EPOCH FROM (completed_at - started_at)))
		FROM scan_jobs
		WHERE status = 'completed'
		AND completed_at IS NOT NULL AND started_at IS NOT NULL
		AND completed_at > NOW() - INTERVAL '30 days'`)
	if err := row.Scan(&avg); err != nil {
		return 0, err
	}
	if avg.Valid {
		return avg.Float64, nil
	}
	return 0, nil
}

func (h *StatsHandler) queryKnowledgeScoreStats(
	ctx context.Context,
) (float64, KnowledgeScoreDistribution, error) {
	ctx, cancel := context.WithTimeout(ctx, statsQueryTimeout)
	defer cancel()

	var avg sql.NullFloat64
	var d0, d25, d50, d75 int
	row := h.db.QueryRowContext(ctx, `
		SELECT
			AVG(knowledge_score),
			COUNT(*) FILTER (WHERE knowledge_score < 25),
			COUNT(*) FILTER (WHERE knowledge_score >= 25 AND knowledge_score < 50),
			COUNT(*) FILTER (WHERE knowledge_score >= 50 AND knowledge_score < 75),
			COUNT(*) FILTER (WHERE knowledge_score >= 75)
		FROM hosts
	`)
	if err := row.Scan(&avg, &d0, &d25, &d50, &d75); err != nil {
		return 0, KnowledgeScoreDistribution{}, err
	}
	var avgScore float64
	if avg.Valid {
		avgScore = avg.Float64
	}
	return avgScore, KnowledgeScoreDistribution{
		Band0to25:   d0,
		Band25to50:  d25,
		Band50to75:  d50,
		Band75to100: d75,
	}, nil
}
