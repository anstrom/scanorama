// Package handlers — mock-based unit tests for DiscoveryHandler.GetDiscoveryDiff.
package handlers

import (
	"encoding/json"
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
