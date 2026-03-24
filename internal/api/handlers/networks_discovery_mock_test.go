// Package handlers — unit tests for the network → discovery endpoints using
// mock stores (no real database required).
package handlers

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/api/handlers/mocks"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/internal/services"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// newNetworkHandlerWithDiscovery builds a NetworkHandler backed by both a
// MockNetworkServicer and a MockDiscoveryStore.  engine is left nil so the
// async goroutine in StartNetworkDiscovery is skipped.
func newNetworkHandlerWithDiscovery(t *testing.T) (
	*NetworkHandler,
	*mocks.MockNetworkServicer,
	*mocks.MockDiscoveryStore,
	*gomock.Controller,
) {
	t.Helper()
	ctrl := gomock.NewController(t)
	svc := mocks.NewMockNetworkServicer(ctrl)
	store := mocks.NewMockDiscoveryStore(ctrl)
	h := NewNetworkHandler(svc, createTestLogger(), metrics.NewRegistry()).
		WithDiscovery(store, nil)
	return h, svc, store, ctrl
}

// testNetwork returns a minimal *db.Network for mock returns.
func testNetwork(id uuid.UUID, name, cidr, method string) *db.Network {
	_, ipNet, _ := net.ParseCIDR(cidr)
	return &db.Network{
		ID:              id,
		Name:            name,
		CIDR:            db.NetworkAddr{IPNet: *ipNet},
		DiscoveryMethod: method,
		IsActive:        true,
		ScanEnabled:     true,
	}
}

// testDiscoveryJob returns a minimal *db.DiscoveryJob for mock returns.
func testDiscoveryJob(id uuid.UUID, cidr, method, status string, now time.Time) *db.DiscoveryJob {
	_, ipNet, _ := net.ParseCIDR(cidr)
	return &db.DiscoveryJob{
		ID:              id,
		Network:         db.NetworkAddr{IPNet: *ipNet},
		Method:          method,
		Status:          status,
		HostsDiscovered: 0,
		HostsResponsive: 0,
		CreatedAt:       now,
	}
}

// ── StartNetworkDiscovery ─────────────────────────────────────────────────────

func TestNetworkHandler_StartNetworkDiscovery(t *testing.T) {
	now := time.Now().UTC()

	t.Run("returns 503 when discovery store is not configured", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		svc := mocks.NewMockNetworkServicer(ctrl)
		// No WithDiscovery — discoveryStore stays nil.
		h := NewNetworkHandler(svc, createTestLogger(), metrics.NewRegistry())

		networkID := uuid.New()
		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discover", h.StartNetworkDiscovery).Methods("POST")

		req := httptest.NewRequest("POST", "/networks/"+networkID.String()+"/discover", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("returns 400 on invalid UUID", func(t *testing.T) {
		h, _, _, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discover", h.StartNetworkDiscovery).Methods("POST")
		req := httptest.NewRequest("POST", "/networks/not-a-uuid/discover", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 404 when network is not found", func(t *testing.T) {
		h, svc, _, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		svc.EXPECT().
			GetNetworkByID(gomock.Any(), networkID).
			Return(nil, errors.NewDatabaseError(errors.CodeNotFound, "network not found"))

		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discover", h.StartNetworkDiscovery).Methods("POST")
		req := httptest.NewRequest("POST", "/networks/"+networkID.String()+"/discover", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 500 when CreateDiscoveryJob fails", func(t *testing.T) {
		h, svc, store, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		network := testNetwork(networkID, "TestNet", "10.0.0.0/8", "tcp")

		svc.EXPECT().
			GetNetworkByID(gomock.Any(), networkID).
			Return(&services.NetworkWithExclusions{Network: network}, nil)
		store.EXPECT().
			CreateDiscoveryJob(gomock.Any(), gomock.Any()).
			Return(nil, errors.NewDatabaseError(errors.CodeDatabaseQuery, "db error"))

		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discover", h.StartNetworkDiscovery).Methods("POST")
		req := httptest.NewRequest("POST", "/networks/"+networkID.String()+"/discover", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 500 when StartDiscoveryJob fails", func(t *testing.T) {
		h, svc, store, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		jobID := uuid.New()
		network := testNetwork(networkID, "TestNet", "10.0.0.0/8", "tcp")
		job := testDiscoveryJob(jobID, "10.0.0.0/8", "tcp", "pending", now)

		svc.EXPECT().
			GetNetworkByID(gomock.Any(), networkID).
			Return(&services.NetworkWithExclusions{Network: network}, nil)
		store.EXPECT().
			CreateDiscoveryJob(gomock.Any(), gomock.Any()).
			Return(job, nil)
		store.EXPECT().
			StartDiscoveryJob(gomock.Any(), jobID).
			Return(errors.NewDatabaseError(errors.CodeDatabaseQuery, "start failed"))

		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discover", h.StartNetworkDiscovery).Methods("POST")
		req := httptest.NewRequest("POST", "/networks/"+networkID.String()+"/discover", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 500 when GetDiscoveryJob fails after start", func(t *testing.T) {
		h, svc, store, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		jobID := uuid.New()
		network := testNetwork(networkID, "TestNet", "10.0.0.0/8", "tcp")
		job := testDiscoveryJob(jobID, "10.0.0.0/8", "tcp", "pending", now)

		svc.EXPECT().
			GetNetworkByID(gomock.Any(), networkID).
			Return(&services.NetworkWithExclusions{Network: network}, nil)
		store.EXPECT().
			CreateDiscoveryJob(gomock.Any(), gomock.Any()).
			Return(job, nil)
		store.EXPECT().
			StartDiscoveryJob(gomock.Any(), jobID).
			Return(nil)
		store.EXPECT().
			GetDiscoveryJob(gomock.Any(), jobID).
			Return(nil, errors.NewDatabaseError(errors.CodeDatabaseQuery, "get failed"))

		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discover", h.StartNetworkDiscovery).Methods("POST")
		req := httptest.NewRequest("POST", "/networks/"+networkID.String()+"/discover", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success — returns 202 with running job", func(t *testing.T) {
		h, svc, store, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		jobID := uuid.New()
		network := testNetwork(networkID, "Production", "192.168.0.0/16", "tcp")
		pendingJob := testDiscoveryJob(jobID, "192.168.0.0/16", "tcp", "pending", now)
		runningJob := testDiscoveryJob(jobID, "192.168.0.0/16", "tcp", "running", now)

		svc.EXPECT().
			GetNetworkByID(gomock.Any(), networkID).
			Return(&services.NetworkWithExclusions{Network: network}, nil)
		store.EXPECT().
			CreateDiscoveryJob(gomock.Any(), db.CreateDiscoveryJobInput{
				Networks:  []string{"192.168.0.0/16"},
				Method:    "tcp",
				NetworkID: nil,
			}).
			Return(pendingJob, nil)
		store.EXPECT().
			StartDiscoveryJob(gomock.Any(), jobID).
			Return(nil)
		store.EXPECT().
			GetDiscoveryJob(gomock.Any(), jobID).
			Return(runningJob, nil)

		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discover", h.StartNetworkDiscovery).Methods("POST")
		req := httptest.NewRequest("POST", "/networks/"+networkID.String()+"/discover", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusAccepted, w.Code)

		var resp DiscoveryResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, jobID, resp.ID)
		assert.Equal(t, "running", resp.Status)
		assert.Equal(t, "tcp", resp.Method)
	})

	t.Run("uses 'tcp' when network discovery method is empty", func(t *testing.T) {
		h, svc, store, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		jobID := uuid.New()
		// Method is empty — handler should default to "tcp".
		network := testNetwork(networkID, "NoMethod", "10.0.0.0/8", "")
		pendingJob := testDiscoveryJob(jobID, "10.0.0.0/8", "tcp", "pending", now)
		runningJob := testDiscoveryJob(jobID, "10.0.0.0/8", "tcp", "running", now)

		svc.EXPECT().GetNetworkByID(gomock.Any(), networkID).
			Return(&services.NetworkWithExclusions{Network: network}, nil)
		store.EXPECT().
			CreateDiscoveryJob(gomock.Any(), db.CreateDiscoveryJobInput{
				Networks:  []string{"10.0.0.0/8"},
				Method:    "tcp",
				NetworkID: nil,
			}).
			Return(pendingJob, nil)
		store.EXPECT().StartDiscoveryJob(gomock.Any(), jobID).Return(nil)
		store.EXPECT().GetDiscoveryJob(gomock.Any(), jobID).Return(runningJob, nil)

		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discover", h.StartNetworkDiscovery).Methods("POST")
		req := httptest.NewRequest("POST", "/networks/"+networkID.String()+"/discover", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusAccepted, w.Code)
		var resp DiscoveryResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "tcp", resp.Method)
	})
}

// ── ListNetworkDiscoveryJobs ──────────────────────────────────────────────────

func TestNetworkHandler_ListNetworkDiscoveryJobs(t *testing.T) {
	now := time.Now().UTC()

	t.Run("returns 503 when discovery store is not configured", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		svc := mocks.NewMockNetworkServicer(ctrl)
		h := NewNetworkHandler(svc, createTestLogger(), metrics.NewRegistry())
		// No WithDiscovery — discoveryStore stays nil.

		networkID := uuid.New()
		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discovery", h.ListNetworkDiscoveryJobs).Methods("GET")
		req := httptest.NewRequest("GET", "/networks/"+networkID.String()+"/discovery", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	})

	t.Run("returns 400 on invalid UUID", func(t *testing.T) {
		h, _, _, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discovery", h.ListNetworkDiscoveryJobs).Methods("GET")
		req := httptest.NewRequest("GET", "/networks/not-a-uuid/discovery", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 on invalid pagination params", func(t *testing.T) {
		h, _, _, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discovery", h.ListNetworkDiscoveryJobs).Methods("GET")
		req := httptest.NewRequest("GET", "/networks/"+networkID.String()+"/discovery?page=abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("propagates db error", func(t *testing.T) {
		h, _, store, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		store.EXPECT().
			ListDiscoveryJobsByNetwork(gomock.Any(), networkID, 0, 50).
			Return(nil, int64(0), errors.NewDatabaseError(errors.CodeDatabaseQuery, "db error"))

		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discovery", h.ListNetworkDiscoveryJobs).Methods("GET")
		req := httptest.NewRequest("GET", "/networks/"+networkID.String()+"/discovery", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns empty list when no jobs exist", func(t *testing.T) {
		h, _, store, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		store.EXPECT().
			ListDiscoveryJobsByNetwork(gomock.Any(), networkID, 0, 50).
			Return([]*db.DiscoveryJob{}, int64(0), nil)

		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discovery", h.ListNetworkDiscoveryJobs).Methods("GET")
		req := httptest.NewRequest("GET", "/networks/"+networkID.String()+"/discovery", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		pagination := resp["pagination"].(map[string]interface{})
		assert.Equal(t, float64(0), pagination["total_items"])
	})

	t.Run("returns paginated list of discovery jobs", func(t *testing.T) {
		h, _, store, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		job1 := testDiscoveryJob(uuid.New(), "10.0.0.0/8", "tcp", "completed", now.Add(-time.Hour))
		job2 := testDiscoveryJob(uuid.New(), "10.0.0.0/8", "ping", "running", now)

		store.EXPECT().
			ListDiscoveryJobsByNetwork(gomock.Any(), networkID, 0, 50).
			Return([]*db.DiscoveryJob{job2, job1}, int64(2), nil)

		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discovery", h.ListNetworkDiscoveryJobs).Methods("GET")
		req := httptest.NewRequest("GET", "/networks/"+networkID.String()+"/discovery", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		pagination := resp["pagination"].(map[string]interface{})
		assert.Equal(t, float64(2), pagination["total_items"])

		items, ok := resp["data"].([]interface{})
		require.True(t, ok, "response should have a 'data' array")
		assert.Len(t, items, 2)

		// Most-recent job first.
		first := items[0].(map[string]interface{})
		assert.Equal(t, "running", first["status"])
		second := items[1].(map[string]interface{})
		assert.Equal(t, "completed", second["status"])
	})

	t.Run("respects pagination query params", func(t *testing.T) {
		h, _, store, ctrl := newNetworkHandlerWithDiscovery(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		// page=2, page_size=5 → offset=5, limit=5
		store.EXPECT().
			ListDiscoveryJobsByNetwork(gomock.Any(), networkID, 5, 5).
			Return([]*db.DiscoveryJob{}, int64(12), nil)

		r := mux.NewRouter()
		r.HandleFunc("/networks/{id}/discovery", h.ListNetworkDiscoveryJobs).Methods("GET")
		req := httptest.NewRequest("GET",
			"/networks/"+networkID.String()+"/discovery?page=2&page_size=5", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		pagination := resp["pagination"].(map[string]interface{})
		assert.Equal(t, float64(12), pagination["total_items"])
	})
}
