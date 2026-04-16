// Package handlers — mock-based unit tests for DiscoveryHandler.GetDiscoveryDiff.
package handlers

import (
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
	"go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
)

// ── TestDiscoveryHandler_GetDiscoveryDiff ─────────────────────────────────────

func TestDiscoveryHandler_GetDiscoveryDiff(t *testing.T) {
	route := func(h *DiscoveryHandler) *mux.Router {
		r := mux.NewRouter()
		r.HandleFunc("/discovery/{id}/diff", h.GetDiscoveryDiff).Methods("GET")
		return r
	}

	t.Run("returns 400 on invalid UUID", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("GET", "/discovery/not-a-uuid/diff", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 500 on store error", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		store.EXPECT().
			GetDiscoveryDiff(gomock.Any(), jobID).
			Return(nil, errors.NewDatabaseError(errors.CodeDatabaseQuery, "connection lost"))

		req := httptest.NewRequest("GET", "/discovery/"+jobID.String()+"/diff", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 404 when job not found", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		store.EXPECT().
			GetDiscoveryDiff(gomock.Any(), jobID).
			Return(nil, errors.NewDatabaseError(errors.CodeNotFound, "not found"))

		req := httptest.NewRequest("GET", "/discovery/"+jobID.String()+"/diff", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 200 with diff body on success", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		jobID := uuid.New()
		hostID := uuid.New()
		now := time.Now().UTC().Truncate(time.Millisecond)
		hostname := "server.local"

		diff := &db.DiscoveryDiff{
			JobID: jobID,
			NewHosts: []db.DiffHost{
				{
					ID:        hostID,
					IPAddress: "10.0.0.1",
					Hostname:  &hostname,
					Status:    "up",
					LastSeen:  now,
					FirstSeen: now,
				},
			},
			GoneHosts:      []db.DiffHost{},
			ChangedHosts:   []db.DiffHost{},
			UnchangedCount: 3,
			Suggestions:    make([]db.DeviceSuggestion, 0),
		}

		store.EXPECT().
			GetDiscoveryDiff(gomock.Any(), jobID).
			Return(diff, nil)

		req := httptest.NewRequest("GET", "/discovery/"+jobID.String()+"/diff", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp db.DiscoveryDiff
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, jobID, resp.JobID)
		require.Len(t, resp.NewHosts, 1)
		assert.Equal(t, hostID, resp.NewHosts[0].ID)
		assert.Equal(t, "10.0.0.1", resp.NewHosts[0].IPAddress)
		assert.Equal(t, 3, resp.UnchangedCount)
		assert.Empty(t, resp.GoneHosts)
		assert.Empty(t, resp.ChangedHosts)
	})
}

// ── TestDiscoveryHandler_GetDiscoveryCompare ──────────────────────────────────

func TestDiscoveryHandler_GetDiscoveryCompare(t *testing.T) {
	route := func(h *DiscoveryHandler) *mux.Router {
		r := mux.NewRouter()
		r.HandleFunc("/discovery/compare", h.GetDiscoveryCompare).Methods("GET")
		return r
	}

	t.Run("returns 400 when run_a is missing", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("GET", "/discovery/compare?run_b="+uuid.New().String(), nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 when run_b is missing", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("GET", "/discovery/compare?run_a="+uuid.New().String(), nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 when both params are missing", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("GET", "/discovery/compare", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 on invalid UUID for run_a", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("GET", "/discovery/compare?run_a=not-a-uuid&run_b="+uuid.New().String(), nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 on invalid UUID for run_b", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		req := httptest.NewRequest("GET", "/discovery/compare?run_a="+uuid.New().String()+"&run_b=not-a-uuid", nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 422 when networks differ", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		runA := uuid.New()
		runB := uuid.New()
		store.EXPECT().
			CompareDiscoveryRuns(gomock.Any(), runA, runB).
			Return(nil, fmt.Errorf("cannot compare runs on different networks: 10.0.0.0/24 vs 192.168.1.0/24"))

		req := httptest.NewRequest("GET", "/discovery/compare?run_a="+runA.String()+"&run_b="+runB.String(), nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("returns 404 when a run is not found", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		runA := uuid.New()
		runB := uuid.New()
		store.EXPECT().
			CompareDiscoveryRuns(gomock.Any(), runA, runB).
			Return(nil, errors.NewDatabaseError(errors.CodeNotFound, "discovery job not found"))

		req := httptest.NewRequest("GET", "/discovery/compare?run_a="+runA.String()+"&run_b="+runB.String(), nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 500 on store error", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		runA := uuid.New()
		runB := uuid.New()
		store.EXPECT().
			CompareDiscoveryRuns(gomock.Any(), runA, runB).
			Return(nil, errors.NewDatabaseError(errors.CodeDatabaseQuery, "connection lost"))

		req := httptest.NewRequest("GET", "/discovery/compare?run_a="+runA.String()+"&run_b="+runB.String(), nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 200 with compare body on success", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerMock(t)
		defer ctrl.Finish()

		runA := uuid.New()
		runB := uuid.New()
		hostID := uuid.New()
		now := time.Now().UTC().Truncate(time.Millisecond)
		hostname := "new-server.local"

		diff := &db.DiscoveryCompareDiff{
			RunAID: runA,
			RunBID: runB,
			NewHosts: []db.DiffHost{
				{
					ID:        hostID,
					IPAddress: "10.0.0.42",
					Hostname:  &hostname,
					Status:    "up",
					LastSeen:  now,
					FirstSeen: now,
				},
			},
			GoneHosts:      []db.DiffHost{},
			ChangedHosts:   []db.DiffHost{},
			UnchangedCount: 5,
		}

		store.EXPECT().
			CompareDiscoveryRuns(gomock.Any(), runA, runB).
			Return(diff, nil)

		req := httptest.NewRequest("GET", "/discovery/compare?run_a="+runA.String()+"&run_b="+runB.String(), nil)
		w := httptest.NewRecorder()
		route(h).ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp db.DiscoveryCompareDiff
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, runA, resp.RunAID)
		assert.Equal(t, runB, resp.RunBID)
		require.Len(t, resp.NewHosts, 1)
		assert.Equal(t, hostID, resp.NewHosts[0].ID)
		assert.Equal(t, "10.0.0.42", resp.NewHosts[0].IPAddress)
		assert.Equal(t, 5, resp.UnchangedCount)
		assert.Empty(t, resp.GoneHosts)
		assert.Empty(t, resp.ChangedHosts)
	})
}
