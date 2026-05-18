// Package services contains unit tests for the search service.
package services

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ── Mocks ──────────────────────────────────────────────────────────────────────

type mockSearchHostRepo struct {
	listHostsFn func(context.Context, *db.HostFilters, int, int) ([]*db.Host, int64, error)
}

func (m *mockSearchHostRepo) ListHosts(
	ctx context.Context, filters *db.HostFilters, offset, limit int,
) ([]*db.Host, int64, error) {
	if m.listHostsFn != nil {
		return m.listHostsFn(ctx, filters, offset, limit)
	}
	return make([]*db.Host, 0), 0, nil
}

type mockSearchScanRepo struct {
	listScansFn func(context.Context, db.ScanFilters, int, int) ([]*db.Scan, int64, error)
}

func (m *mockSearchScanRepo) ListScans(
	ctx context.Context, filters db.ScanFilters, offset, limit int,
) ([]*db.Scan, int64, error) {
	if m.listScansFn != nil {
		return m.listScansFn(ctx, filters, offset, limit)
	}
	return make([]*db.Scan, 0), 0, nil
}

type mockSearchProfileRepo struct {
	listProfilesFn func(context.Context, db.ProfileFilters, int, int) ([]*db.ScanProfile, int64, error)
}

func (m *mockSearchProfileRepo) ListProfiles(
	ctx context.Context, filters db.ProfileFilters, offset, limit int,
) ([]*db.ScanProfile, int64, error) {
	if m.listProfilesFn != nil {
		return m.listProfilesFn(ctx, filters, offset, limit)
	}
	return make([]*db.ScanProfile, 0), 0, nil
}

type mockSearchNetworkRepo struct {
	searchFn func(context.Context, string, int) ([]*db.Network, error)
}

func (m *mockSearchNetworkRepo) SearchNetworks(
	ctx context.Context, query string, limit int,
) ([]*db.Network, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, query, limit)
	}
	return make([]*db.Network, 0), nil
}

// ── Helper ─────────────────────────────────────────────────────────────────────

func newTestSearchService(
	hosts searchHostRepo,
	scans searchScanRepo,
	profiles searchProfileRepo,
	networks searchNetworkRepo,
) *SearchService {
	logger := slog.Default()
	return NewSearchService(hosts, scans, profiles, networks, logger)
}

func defaultMocks() (
	*mockSearchHostRepo,
	*mockSearchScanRepo,
	*mockSearchProfileRepo,
	*mockSearchNetworkRepo,
) {
	return &mockSearchHostRepo{},
		&mockSearchScanRepo{},
		&mockSearchProfileRepo{},
		&mockSearchNetworkRepo{}
}

// ── Tests ──────────────────────────────────────────────────────────────────────

func TestSearchService_QueryTooShort(t *testing.T) {
	h, s, p, n := defaultMocks()
	svc := newTestSearchService(h, s, p, n)

	_, err := svc.Search(context.Background(), "x", 10)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least 2")
}

func TestSearchService_QueryTooLong(t *testing.T) {
	h, s, p, n := defaultMocks()
	svc := newTestSearchService(h, s, p, n)

	longQ := ""
	for i := 0; i < searchMaxQueryLen+1; i++ {
		longQ += "a"
	}
	_, err := svc.Search(context.Background(), longQ, 10)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 100")
}

func TestSearchService_HostsFound(t *testing.T) {
	hostname := "myserver"
	id := uuid.New()

	hostRepo := &mockSearchHostRepo{
		listHostsFn: func(_ context.Context, filters *db.HostFilters, _, _ int) ([]*db.Host, int64, error) {
			assert.Equal(t, "myserver", filters.Search)
			h := &db.Host{}
			h.ID = id
			h.IPAddress = db.IPAddr{}
			h.Hostname = &hostname
			return []*db.Host{h}, 1, nil
		},
	}
	_, s, p, n := defaultMocks()
	svc := newTestSearchService(hostRepo, s, p, n)

	results, err := svc.Search(context.Background(), "myserver", 10)

	require.NoError(t, err)
	require.Len(t, results.Results["host"], 1)
	assert.Equal(t, id.String(), results.Results["host"][0].ID)
	assert.Contains(t, results.Results["host"][0].Label, "myserver")
	assert.Equal(t, "host", results.Results["host"][0].Type)
	assert.Equal(t, 1, results.Total)
}

func TestSearchService_NetworksFound(t *testing.T) {
	id := uuid.New()

	netRepo := &mockSearchNetworkRepo{
		searchFn: func(_ context.Context, query string, _ int) ([]*db.Network, error) {
			assert.Equal(t, "office", query)
			n := &db.Network{Name: "Office LAN"}
			n.ID = id
			return []*db.Network{n}, nil
		},
	}
	h, s, p, _ := defaultMocks()
	svc := newTestSearchService(h, s, p, netRepo)

	results, err := svc.Search(context.Background(), "office", 10)

	require.NoError(t, err)
	require.Len(t, results.Results["network"], 1)
	assert.Equal(t, id.String(), results.Results["network"][0].ID)
	assert.Equal(t, "network", results.Results["network"][0].Type)
}

func TestSearchService_ScansFound(t *testing.T) {
	id := uuid.New()

	scanRepo := &mockSearchScanRepo{
		listScansFn: func(_ context.Context, _ db.ScanFilters, _, _ int) ([]*db.Scan, int64, error) {
			sc := &db.Scan{
				Targets: []string{"192.168.1.0/24"},
				Status:  "completed",
			}
			sc.ID = id
			return []*db.Scan{sc}, 1, nil
		},
	}
	h, _, p, n := defaultMocks()
	svc := newTestSearchService(h, scanRepo, p, n)

	results, err := svc.Search(context.Background(), "192.168", 10)

	require.NoError(t, err)
	require.Len(t, results.Results["scan"], 1)
	assert.Equal(t, id.String(), results.Results["scan"][0].ID)
	assert.Contains(t, results.Results["scan"][0].Label, "completed")
}

func TestSearchService_ProfilesFound(t *testing.T) {
	profileRepo := &mockSearchProfileRepo{
		listProfilesFn: func(_ context.Context, _ db.ProfileFilters, _, _ int) ([]*db.ScanProfile, int64, error) {
			return []*db.ScanProfile{
				{ID: "quick-scan", Name: "Quick Scan"},
				{ID: "full-scan", Name: "Full Scan"},
			}, 2, nil
		},
	}
	h, s, _, n := defaultMocks()
	svc := newTestSearchService(h, s, profileRepo, n)

	results, err := svc.Search(context.Background(), "quick", 10)

	require.NoError(t, err)
	require.Len(t, results.Results["profile"], 1)
	assert.Equal(t, "quick-scan", results.Results["profile"][0].ID)
	assert.Equal(t, "Quick Scan", results.Results["profile"][0].Label)
}

func TestSearchService_EmptyResults(t *testing.T) {
	h, s, p, n := defaultMocks()
	svc := newTestSearchService(h, s, p, n)

	results, err := svc.Search(context.Background(), "xyznotfound", 10)

	require.NoError(t, err)
	assert.Equal(t, 0, results.Total)
	assert.Empty(t, results.Results["host"])
	assert.Empty(t, results.Results["network"])
	assert.Empty(t, results.Results["scan"])
	assert.Empty(t, results.Results["profile"])
}

func TestSearchService_LimitDefault(t *testing.T) {
	var gotLimit int
	hostRepo := &mockSearchHostRepo{
		listHostsFn: func(_ context.Context, _ *db.HostFilters, _, limit int) ([]*db.Host, int64, error) {
			gotLimit = limit
			return make([]*db.Host, 0), 0, nil
		},
	}
	_, s, p, n := defaultMocks()
	svc := newTestSearchService(hostRepo, s, p, n)

	_, err := svc.Search(context.Background(), "test", 0)

	require.NoError(t, err)
	assert.Equal(t, searchDefaultLmt, gotLimit)
}

func TestSearchService_LimitCapped(t *testing.T) {
	var gotLimit int
	hostRepo := &mockSearchHostRepo{
		listHostsFn: func(_ context.Context, _ *db.HostFilters, _, limit int) ([]*db.Host, int64, error) {
			gotLimit = limit
			return make([]*db.Host, 0), 0, nil
		},
	}
	_, s, p, n := defaultMocks()
	svc := newTestSearchService(hostRepo, s, p, n)

	_, err := svc.Search(context.Background(), "test", 999)

	require.NoError(t, err)
	assert.Equal(t, searchMaxLimit, gotLimit)
}

func TestSearchService_HostRepoError_DoesNotFailOtherSections(t *testing.T) {
	hostRepo := &mockSearchHostRepo{
		listHostsFn: func(_ context.Context, _ *db.HostFilters, _, _ int) ([]*db.Host, int64, error) {
			return nil, 0, fmt.Errorf("db error")
		},
	}
	_, s, p, n := defaultMocks()
	svc := newTestSearchService(hostRepo, s, p, n)

	results, err := svc.Search(context.Background(), "test", 10)

	// Search should succeed overall — individual failures are logged, not surfaced.
	require.NoError(t, err)
	assert.NotNil(t, results)
}

func TestSearchService_ResultsHaveCorrectURLs(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")

	hostRepo := &mockSearchHostRepo{
		listHostsFn: func(_ context.Context, _ *db.HostFilters, _, _ int) ([]*db.Host, int64, error) {
			h := &db.Host{}
			h.ID = id
			return []*db.Host{h}, 1, nil
		},
	}
	_, s, p, n := defaultMocks()
	svc := newTestSearchService(hostRepo, s, p, n)

	results, err := svc.Search(context.Background(), "00", 10)

	require.NoError(t, err)
	require.Len(t, results.Results["host"], 1)
	assert.Equal(t, "/hosts/"+id.String(), results.Results["host"][0].URL)
}
