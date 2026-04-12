// Package services - Smart Scan orchestration service.
// Evaluates per-host knowledge gaps and queues the appropriate next scan stage.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/profiles"
	"github.com/anstrom/scanorama/internal/scanning"
)

// smartScanQueryTimeout caps suggestion-aggregation queries.
const smartScanQueryTimeout = 5 * time.Second

// staleThreshold is how old last_seen must be before a host is considered stale.
const staleThreshold = 30 * 24 * time.Hour

// hostHasServicesQueryTimeout caps per-host existence-check queries.
const hostHasServicesQueryTimeout = 3 * time.Second

// maxBatchHostsQuery is the maximum number of hosts fetched in a batch resolve.
const maxBatchHostsQuery = 500

// ScanStage describes what the Smart Scan orchestrator recommends doing next
// for a specific host.
type ScanStage struct {
	Stage       string  `json:"stage"`                // os_detection, port_expansion, service_scan, refresh, skip
	ScanType    string  `json:"scan_type"`            // nmap scan type to use
	Ports       string  `json:"ports"`                // port specification
	OSDetection bool    `json:"os_detection"`         // whether to enable OS fingerprinting
	ProfileID   *string `json:"profile_id,omitempty"` // profile to attribute the scan to (may be nil)
	Reason      string  `json:"reason"`               // human-readable explanation
}

// SuggestionGroup aggregates hosts that share the same knowledge gap.
type SuggestionGroup struct {
	Count       int    `json:"count"`
	Description string `json:"description"`
	Action      string `json:"action"` // matches ScanStage.Stage
}

// SuggestionSummary holds fleet-wide gap counts for the dashboard widget.
type SuggestionSummary struct {
	NoOSInfo    SuggestionGroup `json:"no_os_info"`
	NoPorts     SuggestionGroup `json:"no_ports"`
	NoServices  SuggestionGroup `json:"no_services"`
	Stale       SuggestionGroup `json:"stale"`
	WellKnown   SuggestionGroup `json:"well_known"`
	TotalHosts  int             `json:"total_hosts"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// BatchFilter constrains which hosts to include in a batch smart-scan trigger.
type BatchFilter struct {
	Stage       string      // empty = all eligible stages; otherwise one of ScanStage.Stage values
	HostIDs     []uuid.UUID // non-empty = only these hosts; empty = all hosts
	NetworkCIDR string      // non-empty = only hosts whose IP falls within this CIDR
	Limit       int         // max hosts to queue; 0 = use defaultBatchLimit
}

// BatchResult summarizes the outcome of a QueueBatch call.
type BatchResult struct {
	Queued  int                `json:"queued"`
	Skipped int                `json:"skipped"`
	Details []BatchDetailEntry `json:"details"`
}

// BatchDetailEntry records what happened for a single host in a batch.
type BatchDetailEntry struct {
	HostID string `json:"host_id"`
	Stage  string `json:"stage"`
	ScanID string `json:"scan_id,omitempty"`
	Reason string `json:"reason,omitempty"`
}

const defaultBatchLimit = 50

// smartHostRepository is the subset of db.HostRepository used by SmartScanService.
type smartHostRepository interface {
	GetHost(ctx context.Context, id uuid.UUID) (*db.Host, error)
	ListHosts(ctx context.Context, filters *db.HostFilters, offset, limit int) ([]*db.Host, int64, error)
	RecalculateKnowledgeScore(ctx context.Context, hostID uuid.UUID) error
}

// SmartScanService evaluates host knowledge gaps and queues targeted scans.
type SmartScanService struct {
	database       *db.DB
	hostRepo       smartHostRepository
	profileManager *profiles.Manager
	scanRepo       scanRepository
	scanQueue      *scanning.ScanQueue
	logger         *slog.Logger

	// hasOpenPortsFn and hasServicesFn are called by EvaluateHost to check
	// live host state. Replaced in tests to avoid a real database dependency.
	hasOpenPortsFn func(ctx context.Context, hostID uuid.UUID) (bool, error)
	hasServicesFn  func(ctx context.Context, hostID uuid.UUID) (bool, error)
}

// NewSmartScanService creates a new SmartScanService.
func NewSmartScanService(
	database *db.DB,
	profileManager *profiles.Manager,
	scanQueue *scanning.ScanQueue,
	logger *slog.Logger,
) *SmartScanService {
	svc := &SmartScanService{
		database:       database,
		hostRepo:       db.NewHostRepository(database),
		profileManager: profileManager,
		scanRepo:       db.NewScanRepository(database),
		scanQueue:      scanQueue,
		logger:         logger.With("service", "smart_scan"),
	}
	svc.hasOpenPortsFn = svc.queryHasOpenPorts
	svc.hasServicesFn = svc.queryHasServices
	return svc
}

// GetSuggestions aggregates host gap counts fleet-wide. Results are computed
// on-demand from the existing hosts table — no separate suggestions table needed.
func (s *SmartScanService) GetSuggestions(ctx context.Context) (*SuggestionSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, smartScanQueryTimeout)
	defer cancel()

	staleTime := time.Now().Add(-staleThreshold)

	row := s.database.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'up' AND (os_family IS NULL OR os_family = ''))           AS no_os_info,
			COUNT(*) FILTER (WHERE os_family IS NOT NULL AND os_family != ''
			                   AND NOT EXISTS (
			                       SELECT 1 FROM port_scans ps
			                       WHERE ps.host_id = h.id AND ps.state = 'open'))                    AS no_ports,
			COUNT(*) FILTER (WHERE EXISTS (
			                       SELECT 1 FROM port_scans ps
			                       WHERE ps.host_id = h.id AND ps.state = 'open')
			                   AND NOT EXISTS (
			                       SELECT 1 FROM port_banners pb
			                       WHERE pb.host_id = h.id
			                         AND pb.service IS NOT NULL
			                         AND pb.service != ''))                               AS no_services,
			COUNT(*) FILTER (WHERE last_seen < $1 AND status <> 'gone')                              AS stale,
			COUNT(*) FILTER (WHERE knowledge_score >= 80)                                            AS well_known,
			COUNT(*)                                                                                  AS total
		FROM hosts h
	`, staleTime)

	var noOS, noPorts, noServices, stale, wellKnown, total int
	if err := row.Scan(&noOS, &noPorts, &noServices, &stale, &wellKnown, &total); err != nil {
		return nil, fmt.Errorf("failed to query suggestion counts: %w", err)
	}

	return &SuggestionSummary{
		NoOSInfo: SuggestionGroup{
			Count:       noOS,
			Description: "Hosts with no OS detection",
			Action:      "os_detection",
		},
		NoPorts: SuggestionGroup{
			Count:       noPorts,
			Description: "Hosts with OS known but no open ports found",
			Action:      "port_expansion",
		},
		NoServices: SuggestionGroup{
			Count:       noServices,
			Description: "Hosts with open ports but no service identification",
			Action:      "service_scan",
		},
		Stale: SuggestionGroup{
			Count:       stale,
			Description: "Hosts not scanned in the last 30 days",
			Action:      "refresh",
		},
		WellKnown: SuggestionGroup{
			Count:       wellKnown,
			Description: "Hosts with comprehensive knowledge (score ≥ 80)",
			Action:      "skip",
		},
		TotalHosts:  total,
		GeneratedAt: time.Now(),
	}, nil
}

// EvaluateHostByID is a convenience wrapper that loads the host then calls EvaluateHost.
func (s *SmartScanService) EvaluateHostByID(ctx context.Context, hostID uuid.UUID) (*ScanStage, error) {
	host, err := s.hostRepo.GetHost(ctx, hostID)
	if err != nil {
		return nil, err
	}
	return s.EvaluateHost(ctx, host)
}

// EvaluateHost determines the next recommended scan stage for a single host.
// Returns a ScanStage with Stage == "skip" when no action is recommended.
func (s *SmartScanService) EvaluateHost(ctx context.Context, host *db.Host) (*ScanStage, error) {
	// Hosts that are gone or explicitly excluded from scanning are always skipped.
	if host.Status == "gone" || host.IgnoreScanning {
		return &ScanStage{Stage: "skip", Reason: "host is gone or excluded from scanning"}, nil
	}

	hasOS := host.OSFamily != nil && *host.OSFamily != ""

	hasOpenPorts, err := s.hasOpenPortsFn(ctx, host.ID)
	if err != nil {
		s.logger.Warn("Failed to check open ports", "host_id", host.ID, "error", err)
		hasOpenPorts = false
	}

	hasServices, err := s.hasServicesFn(ctx, host.ID)
	if err != nil {
		s.logger.Warn("Failed to check service data", "host_id", host.ID, "error", err)
		hasServices = false
	}

	isStale := time.Since(host.LastSeen) > staleThreshold

	switch {
	case !hasOS && host.Status == "up":
		return s.stageOSDetection(), nil
	case hasOS && !hasOpenPorts:
		return s.stageWithProfile(ctx, host, "port_expansion"), nil
	case hasOpenPorts && !hasServices:
		return s.stageWithProfile(ctx, host, "service_scan"), nil
	case isStale:
		return &ScanStage{
			Stage:    "refresh",
			ScanType: "connect",
			Ports:    "1-1024",
			Reason:   fmt.Sprintf("last seen %s ago — refreshing scan", time.Since(host.LastSeen).Round(time.Hour)),
		}, nil
	default:
		return &ScanStage{Stage: "skip", Reason: "host knowledge is sufficient"}, nil
	}
}

// stageOSDetection returns a ScanStage configured for OS fingerprinting.
// SYN scan is required because OS detection needs raw socket access (-O flag).
func (s *SmartScanService) stageOSDetection() *ScanStage {
	return &ScanStage{
		Stage:       "os_detection",
		ScanType:    "syn",
		Ports:       "22,80,135,443,445,3389",
		OSDetection: true,
		Reason:      "no OS information — running OS fingerprint scan",
	}
}

// stageWithProfile returns a ScanStage using the best matching profile,
// falling back to a generic 1-1024 connect scan if none is found.
func (s *SmartScanService) stageWithProfile(ctx context.Context, host *db.Host, stage string) *ScanStage {
	fallbackReason := map[string]string{
		"port_expansion": "OS known but no ports found — generic port scan",
		"service_scan":   "open ports found but no service banners — generic service scan",
	}
	profileReason := map[string]string{
		"port_expansion": fmt.Sprintf("OS known (%s) but no ports found — using profile", safeStr(host.OSFamily)),
		"service_scan":   "open ports found but no service banners — service scan",
	}

	if s.profileManager != nil {
		profile, err := s.profileManager.SelectBestProfile(ctx, host)
		if err == nil && profile != nil {
			profileID := profile.ID
			return &ScanStage{
				Stage:     stage,
				ScanType:  profile.ScanType,
				Ports:     profile.Ports,
				ProfileID: &profileID,
				Reason:    fmt.Sprintf("%s %q", profileReason[stage], profile.Name),
			}
		}
	}
	return &ScanStage{
		Stage:    stage,
		ScanType: "connect",
		Ports:    "1-1024",
		Reason:   fallbackReason[stage],
	}
}

func safeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// QueueSmartScan evaluates a single host and queues the recommended scan.
// Returns the created scan UUID and an error. If the stage is "skip" a nil
// UUID is returned with no error.
func (s *SmartScanService) QueueSmartScan(ctx context.Context, hostID uuid.UUID) (uuid.UUID, error) {
	host, err := s.hostRepo.GetHost(ctx, hostID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to load host: %w", err)
	}

	stage, err := s.EvaluateHost(ctx, host)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to evaluate host: %w", err)
	}
	if stage.Stage == "skip" {
		return uuid.Nil, nil
	}

	return s.createAndQueueScan(ctx, host, stage)
}

// QueueBatch queues smart scans for all eligible hosts matching the filter.
func (s *SmartScanService) QueueBatch(ctx context.Context, filter BatchFilter) (*BatchResult, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = defaultBatchLimit
	}

	hosts, err := s.resolveHosts(ctx, filter)
	if err != nil {
		return nil, err
	}

	result := &BatchResult{Details: []BatchDetailEntry{}}

	for _, host := range hosts {
		if result.Queued >= limit {
			break
		}

		stage, err := s.EvaluateHost(ctx, host)
		if err != nil {
			s.logger.Warn("Failed to evaluate host for batch smart scan",
				"host_id", host.ID, "error", err)
			result.Skipped++
			continue
		}
		if stage.Stage == "skip" {
			result.Skipped++
			continue
		}
		if filter.Stage != "" && stage.Stage != filter.Stage {
			result.Skipped++
			continue
		}

		scanID, err := s.createAndQueueScan(ctx, host, stage)
		if err != nil {
			s.logger.Warn("Failed to queue smart scan for host",
				"host_id", host.ID, "error", err)
			result.Skipped++
			result.Details = append(result.Details, BatchDetailEntry{
				HostID: host.ID.String(),
				Stage:  stage.Stage,
				Reason: err.Error(),
			})
			continue
		}

		result.Queued++
		result.Details = append(result.Details, BatchDetailEntry{
			HostID: host.ID.String(),
			Stage:  stage.Stage,
			ScanID: scanID.String(),
		})
	}

	return result, nil
}

// ReEvaluateHosts is the PostScanHook callback. It recalculates knowledge
// scores for the given hosts, then evaluates their next scan stage and logs
// the recommendation. It does not auto-queue further scans.
func (s *SmartScanService) ReEvaluateHosts(_ *db.DB, hostIDs []uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, id := range hostIDs {
		if err := s.hostRepo.RecalculateKnowledgeScore(ctx, id); err != nil {
			s.logger.Warn("Post-scan knowledge score update failed", "host_id", id, "error", err)
		}
		host, err := s.hostRepo.GetHost(ctx, id)
		if err != nil {
			continue
		}
		stage, err := s.EvaluateHost(ctx, host)
		if err != nil {
			continue
		}
		s.logger.Debug("Post-scan stage evaluation", "host_id", id, "stage", stage.Stage, "reason", stage.Reason)
	}
}

// ── internal helpers ──────────────────────────────────────────────────────────

// queryHasOpenPorts returns true if the host has at least one open port in port_scans.
func (s *SmartScanService) queryHasOpenPorts(ctx context.Context, hostID uuid.UUID) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, hostHasServicesQueryTimeout)
	defer cancel()

	var exists bool
	err := s.database.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM port_scans
			WHERE host_id = $1 AND state = 'open'
		)`, hostID,
	).Scan(&exists)
	return exists, err
}

// queryHasServices returns true if the host has at least one port_banners row
// with a non-empty service name.
func (s *SmartScanService) queryHasServices(ctx context.Context, hostID uuid.UUID) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, hostHasServicesQueryTimeout)
	defer cancel()

	var exists bool
	err := s.database.QueryRowContext(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM port_banners
			WHERE host_id = $1 AND service IS NOT NULL AND service != ''
		)`, hostID,
	).Scan(&exists)
	return exists, err
}

// resolveHosts returns the list of active hosts for a batch operation.
// When filter.HostIDs is non-empty only those hosts are returned; otherwise
// all up hosts are returned, capped at maxBatchHostsQuery.
func (s *SmartScanService) resolveHosts(ctx context.Context, filter BatchFilter) ([]*db.Host, error) {
	if len(filter.HostIDs) > 0 {
		hosts := make([]*db.Host, 0, len(filter.HostIDs))
		for _, id := range filter.HostIDs {
			h, err := s.hostRepo.GetHost(ctx, id)
			if err != nil {
				s.logger.Warn("Host not found in batch", "host_id", id)
				continue
			}
			hosts = append(hosts, h)
		}
		return hosts, nil
	}

	// Fetch only up hosts to avoid wasting the query budget on gone/ignored entries.
	hosts, _, err := s.hostRepo.ListHosts(ctx, &db.HostFilters{
		Status:  "up",
		Network: filter.NetworkCIDR,
	}, 0, maxBatchHostsQuery)
	return hosts, err
}

// createAndQueueScan builds a scan record from the stage recommendation and
// submits it to the queue. Returns the new scan UUID.
func (s *SmartScanService) createAndQueueScan(ctx context.Context, host *db.Host, stage *ScanStage) (uuid.UUID, error) {
	ip := host.IPAddress.String()
	scanName := fmt.Sprintf("Smart Scan: %s [%s]", ip, stage.Stage)

	input := db.CreateScanInput{
		Name:        scanName,
		Targets:     []string{ip},
		ScanType:    stage.ScanType,
		Ports:       stage.Ports,
		OSDetection: stage.OSDetection,
		ProfileID:   stage.ProfileID,
	}

	scan, err := s.scanRepo.CreateScan(ctx, input)
	if err != nil {
		return uuid.Nil, fmt.Errorf("failed to create smart scan record: %w", err)
	}

	if err := s.scanRepo.StartScan(ctx, scan.ID); err != nil {
		return uuid.Nil, fmt.Errorf("failed to start smart scan: %w", err)
	}

	scanID := scan.ID
	scanConfig := &scanning.ScanConfig{
		Targets:     scan.Targets,
		Ports:       scan.Ports,
		ScanType:    scan.ScanType,
		TimeoutSec:  scanning.CalculateTimeout(scan.Ports, len(scan.Targets), scan.ScanType),
		OSDetection: stage.OSDetection,
		ScanID:      &scanID,
	}

	repo := s.scanRepo
	logger := s.logger
	database := s.database

	job := scanning.NewScanJob(
		scan.ID.String(),
		scanConfig,
		database,
		scanning.ScanJobExecutor(scanning.RunScanWithContext),
		func(_ *scanning.ScanResult, err error) {
			bgCtx := context.Background()
			if err != nil {
				logger.Error("Smart scan execution failed", "scan_id", scanID, "error", err)
				if stopErr := repo.StopScan(bgCtx, scanID, err.Error()); stopErr != nil {
					logger.Error("Failed to mark smart scan as stopped", "scan_id", scanID, "error", stopErr)
				}
			} else {
				logger.Info("Smart scan completed", "scan_id", scanID)
				if completeErr := repo.CompleteScan(bgCtx, scanID); completeErr != nil {
					logger.Error("Failed to mark smart scan as completed", "scan_id", scanID, "error", completeErr)
				}
			}
		},
	)

	if s.scanQueue != nil {
		if err := s.scanQueue.Submit(job); err != nil {
			// Revert scan to stopped state — use a background context because the
			// caller's context may already be cancelled at this point.
			bgCtx := context.Background()
			_ = s.scanRepo.StopScan(bgCtx, scan.ID, err.Error())
			return uuid.Nil, fmt.Errorf("queue rejected smart scan: %w", err)
		}
	} else {
		go func() {
			bgCtx := context.Background()
			if _, execErr := scanning.RunScanWithContext(bgCtx, scanConfig, database); execErr != nil {
				logger.Error("Async smart scan failed", "scan_id", scanID, "error", execErr)
				_ = repo.StopScan(bgCtx, scanID, execErr.Error())
			} else {
				_ = repo.CompleteScan(bgCtx, scanID)
			}
		}()
	}

	return scan.ID, nil
}
