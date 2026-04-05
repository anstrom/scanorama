// Package handlers — mock-based unit tests for DiscoveryHandler methods:
// UpdateDiscoveryJob, StartDiscovery, StopDiscovery, and requestToUpdateDiscovery.
package handlers

import (
	"bytes"
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
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newDiscoveryHandlerMock(t *testing.T) (*DiscoveryHandler, *mocks.MockDiscoveryStore, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	store := mocks.NewMockDiscoveryStore(ctrl)
	h := NewDiscoveryHandler(store, createTestLogger(), metrics.NewRegistry())
	return h, store, ctrl
}

func buildDiscoveryJob(id uuid.UUID, status string, now time.Time) *db.DiscoveryJob {
	_, ipNet, _ := net.ParseCIDR("10.0.0.0/8")
	return &db.DiscoveryJob{
		ID:        id,
		Network:   db.NetworkAddr{IPNet: *ipNet},
		Method:    "ping",
		Status:    status,
		CreatedAt: now,
	}
}

// ── TestDiscoveryHandler_UpdateDiscoveryJob ───────────────────────────────────

func TestDiscoveryHandler_UpdateDiscoveryJob(t *testing.T) {
	now := time.Now().UTC()

	route := func(h *DiscoveryHandler) *mux.Router {
		r := mux.NewRouter()
		r.HandleFunc("/discovery/{id}", h.UpdateDiscoveryJob).Methods("PUT")
		return r
	}

	t.Run("returns 400 on invalid UUID", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("PUT", "/discovery/not-a-uuid", bytes.NewBufferString(`{"method":"ping"}`))
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 on invalid JSON body", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		req := httptest.NewRequest("PUT", "/discovery/"+jobID.String(), bytes.NewBufferString(`not-json`))
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 404 when job not found", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		store.EXPECT().
			UpdateDiscoveryJob(gomock.Any(), jobID, gomock.Any()).
			Return(nil, errors.NewDatabaseError(errors.CodeNotFound, "not found"))

		req := httptest.NewRequest("PUT", "/discovery/"+jobID.String(), bytes.NewBufferString(`{"method":"ping"}`))
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 500 on database error", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		store.EXPECT().
			UpdateDiscoveryJob(gomock.Any(), jobID, gomock.Any()).
			Return(nil, errors.NewDatabaseError(errors.CodeDatabaseQuery, "db error"))

		req := httptest.NewRequest("PUT", "/discovery/"+jobID.String(), bytes.NewBufferString(`{"method":"ping"}`))
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 200 on success", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		updated := buildDiscoveryJob(jobID, db.DiscoveryJobStatusPending, now)

		store.EXPECT().
			UpdateDiscoveryJob(gomock.Any(), jobID, gomock.Any()).
			Return(updated, nil)

		body, _ := json.Marshal(map[string]string{"method": "ping"})
		req := httptest.NewRequest("PUT", "/discovery/"+jobID.String(), bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp DiscoveryResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, jobID, resp.ID)
		assert.Equal(t, "ping", resp.Method)
	})
}

// ── TestDiscoveryHandler_StartDiscovery ───────────────────────────────────────

func TestDiscoveryHandler_StartDiscovery(t *testing.T) {
	now := time.Now().UTC()

	route := func(h *DiscoveryHandler) *mux.Router {
		r := mux.NewRouter()
		r.HandleFunc("/discovery/{id}/start", h.StartDiscovery).Methods("POST")
		return r
	}

	t.Run("returns 400 on invalid UUID", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("POST", "/discovery/not-a-uuid/start", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 404 when job not found", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		store.EXPECT().
			GetDiscoveryJob(gomock.Any(), jobID).
			Return(nil, errors.NewDatabaseError(errors.CodeNotFound, "not found"))

		req := httptest.NewRequest("POST", "/discovery/"+jobID.String()+"/start", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 409 when already running", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		job := buildDiscoveryJob(jobID, db.DiscoveryJobStatusRunning, now)
		store.EXPECT().GetDiscoveryJob(gomock.Any(), jobID).Return(job, nil)

		req := httptest.NewRequest("POST", "/discovery/"+jobID.String()+"/start", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("returns 409 when already completed", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		job := buildDiscoveryJob(jobID, db.DiscoveryJobStatusCompleted, now)
		store.EXPECT().GetDiscoveryJob(gomock.Any(), jobID).Return(job, nil)

		req := httptest.NewRequest("POST", "/discovery/"+jobID.String()+"/start", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("returns 500 when StartDiscoveryJob fails", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		job := buildDiscoveryJob(jobID, db.DiscoveryJobStatusPending, now)

		gomock.InOrder(
			store.EXPECT().GetDiscoveryJob(gomock.Any(), jobID).Return(job, nil),
			store.EXPECT().StartDiscoveryJob(gomock.Any(), jobID).Return(
				errors.NewDatabaseError(errors.CodeDatabaseQuery, "db error"),
			),
		)

		req := httptest.NewRequest("POST", "/discovery/"+jobID.String()+"/start", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 500 when second GetDiscoveryJob fails", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		job := buildDiscoveryJob(jobID, db.DiscoveryJobStatusPending, now)

		gomock.InOrder(
			store.EXPECT().GetDiscoveryJob(gomock.Any(), jobID).Return(job, nil),
			store.EXPECT().StartDiscoveryJob(gomock.Any(), jobID).Return(nil),
			store.EXPECT().GetDiscoveryJob(gomock.Any(), jobID).Return(
				nil, errors.NewDatabaseError(errors.CodeDatabaseQuery, "db error"),
			),
		)

		req := httptest.NewRequest("POST", "/discovery/"+jobID.String()+"/start", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 200 on success — no engine", func(t *testing.T) {
		// engine is nil (not set via WithEngine), so the goroutine path is skipped.
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		pending := buildDiscoveryJob(jobID, db.DiscoveryJobStatusPending, now)
		running := buildDiscoveryJob(jobID, db.DiscoveryJobStatusRunning, now)

		gomock.InOrder(
			store.EXPECT().GetDiscoveryJob(gomock.Any(), jobID).Return(pending, nil),
			store.EXPECT().StartDiscoveryJob(gomock.Any(), jobID).Return(nil),
			store.EXPECT().GetDiscoveryJob(gomock.Any(), jobID).Return(running, nil),
		)

		req := httptest.NewRequest("POST", "/discovery/"+jobID.String()+"/start", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp DiscoveryResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, jobID, resp.ID)
		assert.Equal(t, db.DiscoveryJobStatusRunning, resp.Status)
	})
}

// ── TestDiscoveryHandler_StopDiscovery ───────────────────────────────────────

func TestDiscoveryHandler_StopDiscovery(t *testing.T) {
	route := func(h *DiscoveryHandler) *mux.Router {
		r := mux.NewRouter()
		r.HandleFunc("/discovery/{id}/stop", h.StopDiscovery).Methods("POST")
		return r
	}

	t.Run("returns 400 on invalid UUID", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("POST", "/discovery/not-a-uuid/stop", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 404 when job not found", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		store.EXPECT().
			StopDiscoveryJob(gomock.Any(), jobID).
			Return(errors.NewDatabaseError(errors.CodeNotFound, "not found"))

		req := httptest.NewRequest("POST", "/discovery/"+jobID.String()+"/stop", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 500 on database error", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		store.EXPECT().
			StopDiscoveryJob(gomock.Any(), jobID).
			Return(errors.NewDatabaseError(errors.CodeDatabaseQuery, "db error"))

		req := httptest.NewRequest("POST", "/discovery/"+jobID.String()+"/stop", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 200 on success", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		store.EXPECT().StopDiscoveryJob(gomock.Any(), jobID).Return(nil)

		req := httptest.NewRequest("POST", "/discovery/"+jobID.String()+"/stop", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]interface{}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "stopped", resp["status"])
	})
}

// ── TestDiscoveryHandler_requestToUpdateDiscovery ─────────────────────────────

func TestDiscoveryHandler_requestToUpdateDiscovery(t *testing.T) {
	h := NewDiscoveryHandler(nil, createTestLogger(), metrics.NewRegistry())

	t.Run("method set — returns pointer", func(t *testing.T) {
		req := &DiscoveryRequest{Method: "ping"}
		input := h.requestToUpdateDiscovery(req)
		require.NotNil(t, input.Method)
		assert.Equal(t, "ping", *input.Method)
	})

	t.Run("method empty — returns nil pointer", func(t *testing.T) {
		req := &DiscoveryRequest{}
		input := h.requestToUpdateDiscovery(req)
		assert.Nil(t, input.Method)
	})
}
