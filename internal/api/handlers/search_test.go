package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

// ── mock ──────────────────────────────────────────────────────────────────────

type mockSearchServicer struct {
	searchFn func(ctx context.Context, q string, limit int) (*db.SearchResults, error)
}

func (m *mockSearchServicer) Search(
	ctx context.Context, q string, limit int,
) (*db.SearchResults, error) {
	if m.searchFn != nil {
		return m.searchFn(ctx, q, limit)
	}
	return &db.SearchResults{
		Results: map[string][]db.SearchResult{
			"hosts":    {},
			"networks": {},
			"scans":    {},
			"profiles": {},
		},
		Total: 0,
	}, nil
}

func newTestSearchHandler(svc SearchServicer) *SearchHandler {
	return NewSearchHandler(svc, createTestLogger(), metrics.NewRegistry())
}

// ── helpers ───────────────────────────────────────────────────────────────────

func buildSearchRequest(query string) *http.Request {
	u := &url.URL{Path: "/api/v1/search", RawQuery: url.Values{"q": {query}}.Encode()}
	return httptest.NewRequest(http.MethodGet, u.String(), nil)
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestSearchHandler_Success(t *testing.T) {
	hostResult := db.SearchResult{
		ID:    "host-uuid",
		Label: "192.168.1.1 (myhost)",
		URL:   "/hosts/host-uuid",
		Type:  "host",
	}

	svc := &mockSearchServicer{
		searchFn: func(_ context.Context, q string, limit int) (*db.SearchResults, error) {
			assert.Equal(t, "myhost", q)
			assert.Equal(t, searchDefaultLimit, limit)
			return &db.SearchResults{
				Results: map[string][]db.SearchResult{
					"hosts":    {hostResult},
					"networks": {},
					"scans":    {},
					"profiles": {},
				},
				Total: 1,
			}, nil
		},
	}

	req := buildSearchRequest("myhost")
	w := httptest.NewRecorder()
	newTestSearchHandler(svc).Search(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.EqualValues(t, float64(1), raw["total"])

	results, ok := raw["results"].(map[string]any)
	require.True(t, ok)
	hosts, ok := results["hosts"].([]any)
	require.True(t, ok)
	require.Len(t, hosts, 1)

	h := hosts[0].(map[string]any)
	assert.Equal(t, "host-uuid", h["id"])
	assert.Equal(t, "192.168.1.1 (myhost)", h["label"])
	assert.Equal(t, "/hosts/host-uuid", h["url"])
	assert.Equal(t, "host", h["type"])
}

func TestSearchHandler_QueryTooShort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=x", nil)
	w := httptest.NewRecorder()
	newTestSearchHandler(&mockSearchServicer{}).Search(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.Contains(t, raw["message"], "at least 2")
}

func TestSearchHandler_QueryMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search", nil)
	w := httptest.NewRecorder()
	newTestSearchHandler(&mockSearchServicer{}).Search(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearchHandler_QueryTooLong(t *testing.T) {
	longQ := ""
	for i := 0; i < searchMaxQueryLen+1; i++ {
		longQ += "a"
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q="+longQ, nil)
	w := httptest.NewRecorder()
	newTestSearchHandler(&mockSearchServicer{}).Search(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearchHandler_InvalidLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=host&limit=abc", nil)
	w := httptest.NewRecorder()
	newTestSearchHandler(&mockSearchServicer{}).Search(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearchHandler_LimitCappedAtMax(t *testing.T) {
	var gotLimit int
	svc := &mockSearchServicer{
		searchFn: func(_ context.Context, _ string, limit int) (*db.SearchResults, error) {
			gotLimit = limit
			return &db.SearchResults{
				Results: map[string][]db.SearchResult{
					"hosts": {}, "networks": {}, "scans": {}, "profiles": {},
				},
			}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=host&limit=999", nil)
	w := httptest.NewRecorder()
	newTestSearchHandler(svc).Search(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, searchMaxLimit, gotLimit)
}

func TestSearchHandler_ServiceError(t *testing.T) {
	svc := &mockSearchServicer{
		searchFn: func(_ context.Context, _ string, _ int) (*db.SearchResults, error) {
			return nil, fmt.Errorf("database unavailable")
		},
	}

	req := buildSearchRequest("host")
	w := httptest.NewRecorder()
	newTestSearchHandler(svc).Search(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSearchHandler_CustomLimit(t *testing.T) {
	var gotLimit int
	svc := &mockSearchServicer{
		searchFn: func(_ context.Context, _ string, limit int) (*db.SearchResults, error) {
			gotLimit = limit
			return &db.SearchResults{
				Results: map[string][]db.SearchResult{
					"hosts": {}, "networks": {}, "scans": {}, "profiles": {},
				},
			}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/search?q=host&limit=5", nil)
	w := httptest.NewRecorder()
	newTestSearchHandler(svc).Search(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 5, gotLimit)
}
