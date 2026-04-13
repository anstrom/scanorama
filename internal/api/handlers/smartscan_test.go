package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apierrors "github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/scanning"
	"github.com/anstrom/scanorama/internal/services"
)

// ── mock ──────────────────────────────────────────────────────────────────────

type mockSmartScanServicer struct {
	getSuggestionsFn            func(ctx context.Context) (*services.SuggestionSummary, error)
	getProfileRecommendationsFn func(ctx context.Context) ([]services.ProfileRecommendation, error)
	evaluateHostByIDFn          func(ctx context.Context, id uuid.UUID) (*services.ScanStage, error)
	queueSmartScanFn            func(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
	queueBatchFn                func(ctx context.Context, f services.BatchFilter) (*services.BatchResult, error)
}

func (m *mockSmartScanServicer) GetSuggestions(ctx context.Context) (*services.SuggestionSummary, error) {
	return m.getSuggestionsFn(ctx)
}

func (m *mockSmartScanServicer) GetProfileRecommendations(
	ctx context.Context,
) ([]services.ProfileRecommendation, error) {
	if m.getProfileRecommendationsFn != nil {
		return m.getProfileRecommendationsFn(ctx)
	}
	return []services.ProfileRecommendation{}, nil
}

func (m *mockSmartScanServicer) EvaluateHostByID(ctx context.Context, id uuid.UUID) (*services.ScanStage, error) {
	return m.evaluateHostByIDFn(ctx, id)
}

func (m *mockSmartScanServicer) QueueSmartScan(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	return m.queueSmartScanFn(ctx, id)
}

func (m *mockSmartScanServicer) QueueBatch(
	ctx context.Context, f services.BatchFilter,
) (*services.BatchResult, error) {
	return m.queueBatchFn(ctx, f)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newSmartScanHandler(svc smartScanServicer) *SmartScanHandler {
	return &SmartScanHandler{service: svc, logger: createTestLogger()}
}

// routeSmartScan wires the handler into a mux with the same path patterns
// used in routes.go and returns a recorder for the given request.
func routeSmartScan(h *SmartScanHandler, method, path string, body []byte) *httptest.ResponseRecorder {
	r := mux.NewRouter()
	r.HandleFunc("/api/v1/smart-scan/suggestions", h.GetSuggestions).Methods("GET")
	r.HandleFunc("/api/v1/smart-scan/profile-recommendations", h.GetProfileRecommendations).Methods("GET")
	r.HandleFunc("/api/v1/smart-scan/hosts/{id}/stage", h.EvaluateHost).Methods("GET")
	r.HandleFunc("/api/v1/smart-scan/hosts/{id}/trigger", h.TriggerHost).Methods("POST")
	r.HandleFunc("/api/v1/smart-scan/trigger-batch", h.TriggerBatch).Methods("POST")

	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = int64(len(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ── GetSuggestions ────────────────────────────────────────────────────────────

func TestSmartScan_GetSuggestions_ReturnsNonNegativeCounts(t *testing.T) {
	svc := &mockSmartScanServicer{
		getSuggestionsFn: func(_ context.Context) (*services.SuggestionSummary, error) {
			return &services.SuggestionSummary{
				NoOSInfo:    services.SuggestionGroup{Count: 3, Action: "os_detection"},
				NoPorts:     services.SuggestionGroup{Count: 1, Action: "port_expansion"},
				NoServices:  services.SuggestionGroup{Count: 5, Action: "service_scan"},
				Stale:       services.SuggestionGroup{Count: 2, Action: "refresh"},
				WellKnown:   services.SuggestionGroup{Count: 10, Action: "skip"},
				TotalHosts:  21,
				GeneratedAt: time.Now(),
			}, nil
		},
	}

	w := routeSmartScan(newSmartScanHandler(svc), "GET", "/api/v1/smart-scan/suggestions", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var body services.SuggestionSummary
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.GreaterOrEqual(t, body.NoOSInfo.Count, 0)
	assert.GreaterOrEqual(t, body.NoPorts.Count, 0)
	assert.GreaterOrEqual(t, body.NoServices.Count, 0)
	assert.GreaterOrEqual(t, body.Stale.Count, 0)
	assert.GreaterOrEqual(t, body.WellKnown.Count, 0)
	assert.Equal(t, 21, body.TotalHosts)
}

func TestSmartScan_GetSuggestions_ServiceError_Returns500(t *testing.T) {
	svc := &mockSmartScanServicer{
		getSuggestionsFn: func(_ context.Context) (*services.SuggestionSummary, error) {
			return nil, apierrors.NewScanError(apierrors.CodeUnknown, "db unavailable")
		},
	}

	w := routeSmartScan(newSmartScanHandler(svc), "GET", "/api/v1/smart-scan/suggestions", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── EvaluateHost (GET /stage) ─────────────────────────────────────────────────

func TestSmartScan_EvaluateHost_ReturnsValidStage(t *testing.T) {
	hostID := uuid.New()
	svc := &mockSmartScanServicer{
		evaluateHostByIDFn: func(_ context.Context, id uuid.UUID) (*services.ScanStage, error) {
			assert.Equal(t, hostID, id)
			return &services.ScanStage{
				Stage:    "os_detection",
				ScanType: "syn",
				Ports:    "22,80,443",
				Reason:   "no OS information",
			}, nil
		},
	}

	w := routeSmartScan(newSmartScanHandler(svc), "GET",
		"/api/v1/smart-scan/hosts/"+hostID.String()+"/stage", nil)
	require.Equal(t, http.StatusOK, w.Code)

	// Decode into a raw map so we assert on the actual JSON key names, not Go
	// field names. Decoding into services.ScanStage would mask missing json tags
	// because Go's decoder maps PascalCase JSON keys to PascalCase fields.
	var raw map[string]interface{}
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.Equal(t, "os_detection", raw["stage"])
	assert.Equal(t, "syn", raw["scan_type"])
	assert.Equal(t, "22,80,443", raw["ports"])
	assert.Equal(t, "no OS information", raw["reason"])
	assert.NotContains(t, raw, "Stage", "response keys must be snake_case")
	assert.NotContains(t, raw, "ScanType", "response keys must be snake_case")
	assert.NotContains(t, raw, "OSDetection", "response keys must be snake_case")
}

func TestSmartScan_EvaluateHost_NotFound_Returns404(t *testing.T) {
	hostID := uuid.New()
	svc := &mockSmartScanServicer{
		evaluateHostByIDFn: func(_ context.Context, _ uuid.UUID) (*services.ScanStage, error) {
			return nil, apierrors.NewScanError(apierrors.CodeNotFound, "host not found")
		},
	}

	w := routeSmartScan(newSmartScanHandler(svc), "GET",
		"/api/v1/smart-scan/hosts/"+hostID.String()+"/stage", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSmartScan_EvaluateHost_InvalidID_Returns400(t *testing.T) {
	w := routeSmartScan(newSmartScanHandler(&mockSmartScanServicer{}),
		"GET", "/api/v1/smart-scan/hosts/not-a-uuid/stage", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── TriggerHost ───────────────────────────────────────────────────────────────

func TestSmartScan_TriggerHost_NoOS_Returns202WithScanID(t *testing.T) {
	hostID := uuid.New()
	scanID := uuid.New()
	svc := &mockSmartScanServicer{
		queueSmartScanFn: func(_ context.Context, id uuid.UUID) (uuid.UUID, error) {
			assert.Equal(t, hostID, id)
			return scanID, nil // non-nil scan ID → queued
		},
	}

	w := routeSmartScan(newSmartScanHandler(svc), "POST",
		"/api/v1/smart-scan/hosts/"+hostID.String()+"/trigger", nil)
	require.Equal(t, http.StatusAccepted, w.Code)

	var resp triggerHostResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.True(t, resp.Queued)
	assert.Equal(t, scanID.String(), resp.ScanID)
}

func TestSmartScan_TriggerHost_WellKnown_Returns200NotQueued(t *testing.T) {
	hostID := uuid.New()
	svc := &mockSmartScanServicer{
		queueSmartScanFn: func(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
			return uuid.Nil, nil // nil UUID → no scan needed
		},
	}

	w := routeSmartScan(newSmartScanHandler(svc), "POST",
		"/api/v1/smart-scan/hosts/"+hostID.String()+"/trigger", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var resp triggerHostResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp.Queued)
	assert.Empty(t, resp.ScanID)
}

func TestSmartScan_TriggerHost_QueueFull_Returns429(t *testing.T) {
	hostID := uuid.New()
	svc := &mockSmartScanServicer{
		queueSmartScanFn: func(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
			return uuid.Nil, scanning.ErrQueueFull
		},
	}

	w := routeSmartScan(newSmartScanHandler(svc), "POST",
		"/api/v1/smart-scan/hosts/"+hostID.String()+"/trigger", nil)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

// ── TriggerBatch ──────────────────────────────────────────────────────────────

func TestSmartScan_TriggerBatch_QueuesUpToLimit(t *testing.T) {
	svc := &mockSmartScanServicer{
		queueBatchFn: func(_ context.Context, f services.BatchFilter) (*services.BatchResult, error) {
			assert.Equal(t, 5, f.Limit)
			return &services.BatchResult{
				Queued:  5,
				Skipped: 2,
				Details: []services.BatchDetailEntry{},
			}, nil
		},
	}

	body, _ := json.Marshal(map[string]any{"limit": 5})
	w := routeSmartScan(newSmartScanHandler(svc), "POST", "/api/v1/smart-scan/trigger-batch", body)
	require.Equal(t, http.StatusAccepted, w.Code)

	var result services.BatchResult
	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	assert.Equal(t, 5, result.Queued)
	assert.LessOrEqual(t, result.Queued, 5)
}

func TestSmartScan_TriggerBatch_InvalidHostID_Returns400(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"host_ids": []string{"not-a-uuid"}})
	w := routeSmartScan(newSmartScanHandler(&mockSmartScanServicer{}),
		"POST", "/api/v1/smart-scan/trigger-batch", body)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSmartScan_TriggerBatch_EmptyBody_UsesDefaults(t *testing.T) {
	svc := &mockSmartScanServicer{
		queueBatchFn: func(_ context.Context, f services.BatchFilter) (*services.BatchResult, error) {
			assert.Equal(t, 0, f.Limit, "empty body should pass limit=0 (service applies default)")
			assert.Empty(t, f.Stage)
			return &services.BatchResult{Details: []services.BatchDetailEntry{}}, nil
		},
	}

	w := routeSmartScan(newSmartScanHandler(svc), "POST", "/api/v1/smart-scan/trigger-batch", nil)
	assert.Equal(t, http.StatusAccepted, w.Code)
}

// ── GetProfileRecommendations ─────────────────────────────────────────────────

func TestSmartScan_GetProfileRecommendations_ReturnsRecommendations(t *testing.T) {
	svc := &mockSmartScanServicer{
		getProfileRecommendationsFn: func(_ context.Context) ([]services.ProfileRecommendation, error) {
			return []services.ProfileRecommendation{
				{OSFamily: "Linux", HostCount: 5, ProfileID: "p1", ProfileName: "Linux Standard",
					Action: "port_expansion"},
				{OSFamily: "Windows", HostCount: 3, ProfileID: "p2", ProfileName: "Windows Standard",
					Action: "port_expansion"},
			}, nil
		},
	}

	w := routeSmartScan(newSmartScanHandler(svc), "GET", "/api/v1/smart-scan/profile-recommendations", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var body []services.ProfileRecommendation
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	require.Len(t, body, 2)
	assert.Equal(t, "Linux", body[0].OSFamily)
	assert.Equal(t, 5, body[0].HostCount)
	assert.Equal(t, "Linux Standard", body[0].ProfileName)
	assert.Equal(t, "Windows", body[1].OSFamily)
}

func TestSmartScan_GetProfileRecommendations_EmptyResult_ReturnsEmptyArray(t *testing.T) {
	svc := &mockSmartScanServicer{
		getProfileRecommendationsFn: func(_ context.Context) ([]services.ProfileRecommendation, error) {
			return nil, nil
		},
	}

	w := routeSmartScan(newSmartScanHandler(svc), "GET", "/api/v1/smart-scan/profile-recommendations", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var body []services.ProfileRecommendation
	require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.NotNil(t, body, "empty result must be [] not null")
	assert.Empty(t, body)
}

func TestSmartScan_GetProfileRecommendations_ServiceError_Returns500(t *testing.T) {
	svc := &mockSmartScanServicer{
		getProfileRecommendationsFn: func(_ context.Context) ([]services.ProfileRecommendation, error) {
			return nil, fmt.Errorf("db unavailable")
		},
	}

	w := routeSmartScan(newSmartScanHandler(svc), "GET", "/api/v1/smart-scan/profile-recommendations", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSmartScan_TriggerBatch_OSFamily_PassedToFilter(t *testing.T) {
	svc := &mockSmartScanServicer{
		queueBatchFn: func(_ context.Context, f services.BatchFilter) (*services.BatchResult, error) {
			assert.Equal(t, "linux", f.OSFamily)
			return &services.BatchResult{Queued: 3, Details: []services.BatchDetailEntry{}}, nil
		},
	}

	body, _ := json.Marshal(map[string]any{"os_family": "linux", "stage": "port_expansion"})
	w := routeSmartScan(newSmartScanHandler(svc), "POST", "/api/v1/smart-scan/trigger-batch", body)
	require.Equal(t, http.StatusAccepted, w.Code)

	var result services.BatchResult
	require.NoError(t, json.NewDecoder(w.Body).Decode(&result))
	assert.Equal(t, 3, result.Queued)
}
