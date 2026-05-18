// Package services provides business logic services for Scanorama.
// This file implements the unified cross-entity search service.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/anstrom/scanorama/internal/db"
	apierrors "github.com/anstrom/scanorama/internal/errors"
)

const (
	searchTypeHost    = "host"
	searchTypeNetwork = "network"
	searchTypeScan    = "scan"
	searchTypeProfile = "profile"

	searchMinQueryLen = 2
	searchMaxQueryLen = 100
	searchMaxLimit    = 50
	searchDefaultLmt  = 10

	// scanSortCreatedAt is the ScanFilters.SortBy value for chronological order.
	scanSortCreatedAt = "created_at"
)

// searchHostRepo is the subset of HostRepository used by SearchService.
type searchHostRepo interface {
	ListHosts(ctx context.Context, filters *db.HostFilters, offset, limit int) ([]*db.Host, int64, error)
}

// searchScanRepo is the subset of ScanRepository used by SearchService.
type searchScanRepo interface {
	ListScans(ctx context.Context, filters db.ScanFilters, offset, limit int) ([]*db.Scan, int64, error)
}

// searchProfileRepo is the subset of ProfileRepository used by SearchService.
type searchProfileRepo interface {
	ListProfiles(ctx context.Context, filters db.ProfileFilters, offset, limit int) ([]*db.ScanProfile, int64, error)
}

// searchNetworkRepo runs network-specific search queries.
type searchNetworkRepo interface {
	SearchNetworks(ctx context.Context, query string, limit int) ([]*db.Network, error)
}

// SearchService provides unified search across all entity types.
type SearchService struct {
	hosts    searchHostRepo
	scans    searchScanRepo
	profiles searchProfileRepo
	networks searchNetworkRepo
	logger   *slog.Logger
}

// NewSearchService constructs a SearchService wired to the given repositories.
func NewSearchService(
	hosts searchHostRepo,
	scans searchScanRepo,
	profiles searchProfileRepo,
	networks searchNetworkRepo,
	logger *slog.Logger,
) *SearchService {
	return &SearchService{
		hosts:    hosts,
		scans:    scans,
		profiles: profiles,
		networks: networks,
		logger:   logger.With("service", "search"),
	}
}

// Search executes a cross-entity search for q, returning at most limit results
// per entity type. q must be between 2 and 100 characters.
func (s *SearchService) Search(ctx context.Context, q string, limit int) (*db.SearchResults, error) {
	if len(q) < searchMinQueryLen {
		return nil, apierrors.NewScanError(
			apierrors.CodeValidation,
			fmt.Sprintf("query must be at least %d characters", searchMinQueryLen),
		)
	}
	if len(q) > searchMaxQueryLen {
		return nil, apierrors.NewScanError(
			apierrors.CodeValidation,
			fmt.Sprintf("query must be at most %d characters", searchMaxQueryLen),
		)
	}
	if limit <= 0 {
		limit = searchDefaultLmt
	}
	if limit > searchMaxLimit {
		limit = searchMaxLimit
	}

	out := &db.SearchResults{
		Results: map[string][]db.SearchResult{
			searchTypeHost:    make([]db.SearchResult, 0),
			searchTypeNetwork: make([]db.SearchResult, 0),
			searchTypeScan:    make([]db.SearchResult, 0),
			searchTypeProfile: make([]db.SearchResult, 0),
		},
	}

	hosts, err := s.searchHosts(ctx, q, limit)
	if err != nil {
		s.logger.Warn("host search failed", "error", err)
	} else {
		out.Results[searchTypeHost] = hosts
	}

	networks, err := s.searchNetworks(ctx, q, limit)
	if err != nil {
		s.logger.Warn("network search failed", "error", err)
	} else {
		out.Results[searchTypeNetwork] = networks
	}

	scans, err := s.searchScans(ctx, q, limit)
	if err != nil {
		s.logger.Warn("scan search failed", "error", err)
	} else {
		out.Results[searchTypeScan] = scans
	}

	profiles, err := s.searchProfiles(ctx, q, limit)
	if err != nil {
		s.logger.Warn("profile search failed", "error", err)
	} else {
		out.Results[searchTypeProfile] = profiles
	}

	for _, results := range out.Results {
		out.Total += len(results)
	}

	return out, nil
}

func (s *SearchService) searchHosts(ctx context.Context, q string, limit int) ([]db.SearchResult, error) {
	hosts, _, err := s.hosts.ListHosts(ctx, &db.HostFilters{Search: q}, 0, limit)
	if err != nil {
		return nil, fmt.Errorf("search hosts: %w", err)
	}

	results := make([]db.SearchResult, 0, len(hosts))
	for _, h := range hosts {
		results = append(results, db.SearchResult{
			ID:    h.ID.String(),
			Label: hostLabel(h),
			URL:   "/hosts/" + h.ID.String(),
			Type:  searchTypeHost,
		})
	}
	return results, nil
}

// hostLabel returns "ip (hostname)" when a hostname is known, otherwise "ip".
func hostLabel(h *db.Host) string {
	ip := h.IPAddress.String()
	if h.Hostname != nil && *h.Hostname != "" {
		return ip + " (" + *h.Hostname + ")"
	}
	return ip
}

func (s *SearchService) searchNetworks(ctx context.Context, q string, limit int) ([]db.SearchResult, error) {
	networks, err := s.networks.SearchNetworks(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("search networks: %w", err)
	}

	results := make([]db.SearchResult, 0, len(networks))
	for _, n := range networks {
		results = append(results, db.SearchResult{
			ID:    n.ID.String(),
			Label: n.Name + " (" + n.CIDR.String() + ")",
			URL:   "/networks/" + n.ID.String(),
			Type:  searchTypeNetwork,
		})
	}
	return results, nil
}

func (s *SearchService) searchScans(ctx context.Context, q string, limit int) ([]db.SearchResult, error) {
	scans, _, err := s.scans.ListScans(
		ctx,
		db.ScanFilters{SortBy: scanSortCreatedAt, SortOrder: "desc"},
		0,
		// Fetch more so we can client-side filter by target match.
		// Profiles are filtered client-side; for scans we need a wider net.
		limit*scanFetchMultiplier,
	)
	if err != nil {
		return nil, fmt.Errorf("search scans: %w", err)
	}

	lower := strings.ToLower(q)
	results := make([]db.SearchResult, 0)
	for _, sc := range scans {
		if len(results) >= limit {
			break
		}
		matched := false
		for _, t := range sc.Targets {
			if strings.Contains(strings.ToLower(t), lower) {
				matched = true
				break
			}
		}
		if sc.Name != "" && strings.Contains(strings.ToLower(sc.Name), lower) {
			matched = true
		}
		if !matched {
			continue
		}
		results = append(results, db.SearchResult{
			ID:    sc.ID.String(),
			Label: scanLabel(sc),
			URL:   "/scans/" + sc.ID.String(),
			Type:  searchTypeScan,
		})
	}
	return results, nil
}

// scanFetchMultiplier is how many more scans to fetch from the DB than limit,
// because we filter client-side by target name match.
const scanFetchMultiplier = 5

// scanLabel builds "target1,target2 — status".
func scanLabel(sc *db.Scan) string {
	label := strings.Join(sc.Targets, ",")
	if label == "" {
		label = sc.Name
	}
	return label + " — " + sc.Status
}

func (s *SearchService) searchProfiles(ctx context.Context, q string, limit int) ([]db.SearchResult, error) {
	// Profiles are few (<100 typically). Fetch all and filter client-side.
	profiles, _, err := s.profiles.ListProfiles(ctx, db.ProfileFilters{}, 0, searchMaxLimit)
	if err != nil {
		return nil, fmt.Errorf("search profiles: %w", err)
	}

	lower := strings.ToLower(q)
	results := make([]db.SearchResult, 0)
	for _, p := range profiles {
		if len(results) >= limit {
			break
		}
		if strings.Contains(strings.ToLower(p.Name), lower) {
			results = append(results, db.SearchResult{
				ID:    p.ID,
				Label: p.Name,
				URL:   "/profiles/" + p.ID,
				Type:  searchTypeProfile,
			})
		}
	}
	return results, nil
}
