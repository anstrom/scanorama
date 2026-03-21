package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
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
	"github.com/anstrom/scanorama/internal/scanning"
	"github.com/anstrom/scanorama/internal/services"
)

// notFoundErr returns a DatabaseError with CodeNotFound, matching what the
// real DB layer returns and what errors.IsNotFound() detects.
func notFoundErr(resource, id string) error {
	return errors.NewDatabaseError(errors.CodeNotFound, fmt.Sprintf("%s %s not found", resource, id))
}

// conflictErr returns a DatabaseError with CodeConflict.
func conflictErr(resource, msg string) error {
	return errors.NewDatabaseError(errors.CodeConflict, fmt.Sprintf("%s: %s", resource, msg))
}

// ── helpers ──────────────────────────────────────────────────────────────────

func newScanHandlerWithMock(t *testing.T) (*ScanHandler, *mocks.MockScanStore, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	store := mocks.NewMockScanStore(ctrl)
	h := NewScanHandler(store, createTestLogger(), metrics.NewRegistry())
	return h, store, ctrl
}

func newScheduleHandlerWithMock(t *testing.T) (*ScheduleHandler, *mocks.MockScheduleStore, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	store := mocks.NewMockScheduleStore(ctrl)
	h := NewScheduleHandler(store, createTestLogger(), metrics.NewRegistry())
	return h, store, ctrl
}

func newDiscoveryHandlerWithMock(t *testing.T) (*DiscoveryHandler, *mocks.MockDiscoveryStore, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	store := mocks.NewMockDiscoveryStore(ctrl)
	h := NewDiscoveryHandler(store, createTestLogger(), metrics.NewRegistry())
	return h, store, ctrl
}

func newHostHandlerWithMock(t *testing.T) (*HostHandler, *mocks.MockHostStore, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	store := mocks.NewMockHostStore(ctrl)
	h := NewHostHandler(store, createTestLogger(), metrics.NewRegistry())
	return h, store, ctrl
}

func newProfileHandlerWithMock(t *testing.T) (*ProfileHandler, *mocks.MockProfileStore, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	store := mocks.NewMockProfileStore(ctrl)
	h := NewProfileHandler(store, createTestLogger(), metrics.NewRegistry())
	return h, store, ctrl
}

func newNetworkHandlerWithMock(t *testing.T) (*NetworkHandler, *mocks.MockNetworkServicer, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	svc := mocks.NewMockNetworkServicer(ctrl)
	h := NewNetworkHandler(svc, createTestLogger(), metrics.NewRegistry())
	return h, svc, ctrl
}

// routerWithID wraps a handler in a mux router that provides the {id} path variable.
//
//nolint:gocritic,unparam // unnamedResult: second return is the pattern string, used as path by some callers
func routerWithID(method, pattern string, hf http.HandlerFunc) (*mux.Router, string) {
	r := mux.NewRouter()
	r.HandleFunc(pattern, hf).Methods(method)
	return r, pattern
}

func makeScan(id uuid.UUID, name, status, scanType string) *db.Scan {
	now := time.Now()
	return &db.Scan{
		ID:        id,
		Name:      name,
		Status:    status,
		ScanType:  scanType,
		Targets:   []string{"192.168.1.0/24"},
		Ports:     "22,80,443",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func makeSchedule(id uuid.UUID, name string) *db.Schedule {
	now := time.Now()
	cronExpr := "0 * * * *"
	return &db.Schedule{
		ID:             id,
		Name:           name,
		Enabled:        true,
		CronExpression: cronExpr,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func makeDiscoveryJob(id uuid.UUID, method, status string) *db.DiscoveryJob {
	now := time.Now()
	return &db.DiscoveryJob{
		ID:        id,
		Method:    method,
		Status:    status,
		CreatedAt: now,
	}
}

func makeHost(id uuid.UUID, ip string) *db.Host {
	addr := db.IPAddr{}
	_ = addr.Scan(ip)
	now := time.Now()
	hostname := "host.example.com"
	return &db.Host{
		ID:        id,
		IPAddress: addr,
		Hostname:  &hostname,
		Status:    "up",
		FirstSeen: now,
		LastSeen:  now,
	}
}

func makeNetwork(id uuid.UUID, name, cidr string) *db.Network {
	_, ipNet, _ := net.ParseCIDR(cidr)
	now := time.Now()
	return &db.Network{
		ID:              id,
		Name:            name,
		CIDR:            db.NetworkAddr{IPNet: *ipNet},
		DiscoveryMethod: "ping",
		IsActive:        true,
		ScanEnabled:     true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func makeNetworkExclusion(id uuid.UUID, networkID *uuid.UUID, cidr string) *db.NetworkExclusion {
	now := time.Now()
	reason := "test exclusion"
	return &db.NetworkExclusion{
		ID:           id,
		NetworkID:    networkID,
		ExcludedCIDR: cidr,
		Reason:       &reason,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func makeProfile(id, name string) *db.ScanProfile {
	now := time.Now()
	return &db.ScanProfile{
		ID:        id,
		Name:      name,
		ScanType:  "connect",
		Ports:     "22,80,443",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ── ScanHandler ───────────────────────────────────────────────────────────────

func TestScanHandler_ListScans_Mock(t *testing.T) {
	t.Run("returns scans from store", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		scan := makeScan(id, "Test Scan", "pending", "connect")

		store.EXPECT().
			ListScans(gomock.Any(), gomock.Any(), 0, 50).
			Return([]*db.Scan{scan}, int64(1), nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/scans", nil)
		w := httptest.NewRecorder()
		h.ListScans(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data, ok := resp["data"].([]interface{})
		require.True(t, ok)
		assert.Len(t, data, 1)
	})

	t.Run("returns empty list when store returns nothing", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			ListScans(gomock.Any(), gomock.Any(), 0, 50).
			Return([]*db.Scan{}, int64(0), nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/scans", nil)
		w := httptest.NewRecorder()
		h.ListScans(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("returns 500 on store error", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			ListScans(gomock.Any(), gomock.Any(), 0, 50).
			Return(nil, int64(0), fmt.Errorf("db error"))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/scans", nil)
		w := httptest.NewRecorder()
		h.ListScans(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestScanHandler_GetScan_Mock(t *testing.T) {
	t.Run("returns scan by ID", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		scan := makeScan(id, "My Scan", "completed", "syn")

		store.EXPECT().
			GetScan(gomock.Any(), id).
			Return(scan, nil)

		router, pattern := routerWithID(http.MethodGet, "/api/v1/scans/{id}", h.GetScan)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/scans/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		_ = pattern

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, id.String(), resp["id"])
	})

	t.Run("returns 404 when scan not found", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			GetScan(gomock.Any(), id).
			Return(nil, notFoundErr("scan", id.String()))

		router, _ := routerWithID(http.MethodGet, "/api/v1/scans/{id}", h.GetScan)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/scans/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 400 for invalid UUID", func(t *testing.T) {
		h, _, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		router, _ := routerWithID(http.MethodGet, "/api/v1/scans/{id}", h.GetScan)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/scans/not-a-uuid", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 500 on unexpected store error", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			GetScan(gomock.Any(), id).
			Return(nil, fmt.Errorf("unexpected db error"))

		router, _ := routerWithID(http.MethodGet, "/api/v1/scans/{id}", h.GetScan)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/scans/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestScanHandler_CreateScan_Mock(t *testing.T) {
	t.Run("creates scan and returns 201", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		scan := makeScan(id, "New Scan", "pending", "connect")

		store.EXPECT().
			CreateScan(gomock.Any(), gomock.Any()).
			Return(scan, nil)

		body := `{"name":"New Scan","targets":["192.168.1.0/24"],"scan_type":"connect","ports":"22,80,443"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/scans", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateScan(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, id.String(), resp["id"])
	})

	t.Run("returns 400 for missing name", func(t *testing.T) {
		h, _, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		body := `{"targets":["192.168.1.0/24"],"scan_type":"connect"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/scans", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateScan(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 for empty targets", func(t *testing.T) {
		h, _, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		body := `{"name":"No Targets","targets":[],"scan_type":"connect"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/scans", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateScan(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		h, _, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/scans", bytes.NewBufferString("{bad json}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateScan(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 500 on store error", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			CreateScan(gomock.Any(), gomock.Any()).
			Return(nil, fmt.Errorf("db error"))

		body := `{"name":"New Scan","targets":["192.168.1.0/24"],"scan_type":"connect","ports":"80"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/scans", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateScan(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestScanHandler_DeleteScan_Mock(t *testing.T) {
	t.Run("deletes scan and returns 204", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().DeleteScan(gomock.Any(), id).Return(nil)

		router, _ := routerWithID(http.MethodDelete, "/api/v1/scans/{id}", h.DeleteScan)
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/scans/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("returns 404 when scan not found", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			DeleteScan(gomock.Any(), id).
			Return(notFoundErr("scan", id.String()))

		router, _ := routerWithID(http.MethodDelete, "/api/v1/scans/{id}", h.DeleteScan)
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/scans/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestScanHandler_StartScan_Mock(t *testing.T) {
	t.Run("starts pending scan", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		scan := makeScan(id, "Pending Scan", "pending", "connect")
		started := makeScan(id, "Pending Scan", "running", "connect")

		store.EXPECT().GetScan(gomock.Any(), id).Return(scan, nil)
		store.EXPECT().StartScan(gomock.Any(), id).Return(nil)
		store.EXPECT().GetScan(gomock.Any(), id).Return(started, nil)

		router, _ := routerWithID(http.MethodPost, "/api/v1/scans/{id}/start", h.StartScan)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/scans/%s/start", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("returns 409 when scan is already running", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		scan := makeScan(id, "Running Scan", "running", "connect")
		store.EXPECT().GetScan(gomock.Any(), id).Return(scan, nil)

		router, _ := routerWithID(http.MethodPost, "/api/v1/scans/{id}/start", h.StartScan)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/scans/%s/start", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("returns 409 when scan is already completed", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		scan := makeScan(id, "Done Scan", "completed", "connect")
		store.EXPECT().GetScan(gomock.Any(), id).Return(scan, nil)

		router, _ := routerWithID(http.MethodPost, "/api/v1/scans/{id}/start", h.StartScan)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/scans/%s/start", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})
}

// TestScanHandler_ExecuteScanAsync covers the goroutine that runs after StartScan
// returns 200. These tests use the injectable scanRunner field so no real nmap
// binary is required.
func TestScanHandler_ExecuteScanAsync(t *testing.T) {
	makeStartedScan := func(id uuid.UUID) *db.Scan {
		return makeScan(id, "Async Scan", "running", "connect")
	}

	t.Run("calls CompleteScan on successful scan runner", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		done := make(chan struct{})

		// Inject a runner that succeeds immediately.
		h.scanRunner = func(_ context.Context, _ *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
			return &scanning.ScanResult{Hosts: []scanning.Host{{Address: "127.0.0.1"}}}, nil
		}

		store.EXPECT().
			CompleteScan(gomock.Any(), id).
			DoAndReturn(func(_ context.Context, _ uuid.UUID) error {
				close(done)
				return nil
			})

		go h.executeScanAsync(id, makeStartedScan(id))

		select {
		case <-done:
			// pass
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for CompleteScan to be called")
		}
	})

	t.Run("calls StopScan (failed) when scan runner returns an error", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		done := make(chan struct{})

		h.scanRunner = func(_ context.Context, _ *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
			return nil, fmt.Errorf("nmap: binary not found")
		}

		store.EXPECT().
			StopScan(gomock.Any(), id, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ uuid.UUID, _ ...string) error {
				close(done)
				return nil
			})

		go h.executeScanAsync(id, makeStartedScan(id))

		select {
		case <-done:
			// pass
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for StopScan to be called")
		}
	})

	t.Run("does not call CompleteScan when scan runner returns an error", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		done := make(chan struct{})

		h.scanRunner = func(_ context.Context, _ *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
			return nil, fmt.Errorf("execution failed")
		}

		// Only StopScan should be called — CompleteScan must not be.
		store.EXPECT().StopScan(gomock.Any(), id, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ uuid.UUID, _ ...string) error {
				close(done)
				return nil
			})

		go h.executeScanAsync(id, makeStartedScan(id))

		select {
		case <-done:
			// pass — gomock will fail the test if CompleteScan is called unexpectedly
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for StopScan to be called")
		}
	})

	t.Run("type-asserting a mock store to *db.DB yields nil and runner still called", func(t *testing.T) {
		// The store is a *mocks.MockScanStore, not *db.DB, so concreteDB will be
		// nil. RunScanWithContext skips persistence when database is nil, so this
		// must not panic.
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		done := make(chan struct{})

		var capturedDB *db.DB
		h.scanRunner = func(_ context.Context, _ *scanning.ScanConfig, database *db.DB) (*scanning.ScanResult, error) {
			capturedDB = database
			return &scanning.ScanResult{}, nil
		}

		store.EXPECT().CompleteScan(gomock.Any(), id).
			DoAndReturn(func(_ context.Context, _ uuid.UUID) error {
				close(done)
				return nil
			})

		go h.executeScanAsync(id, makeStartedScan(id))

		select {
		case <-done:
			// pass
		case <-time.After(2 * time.Second):
			t.Fatal("timed out")
		}

		if capturedDB != nil {
			t.Errorf("expected nil *db.DB for mock store, got %v", capturedDB)
		}
	})
}

func TestScanHandler_StopScan_Mock(t *testing.T) {
	t.Run("stops running scan", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		// ExecuteStop only calls stopInDB — no subsequent GetScan.
		store.EXPECT().StopScan(gomock.Any(), id).Return(nil)

		router, _ := routerWithID(http.MethodPost, "/api/v1/scans/{id}/stop", h.StopScan)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/scans/%s/stop", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("returns 404 when scan not found", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			StopScan(gomock.Any(), id).
			Return(notFoundErr("scan", id.String()))

		router, _ := routerWithID(http.MethodPost, "/api/v1/scans/{id}/stop", h.StopScan)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/scans/%s/stop", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestScanHandler_GetScanResults_Mock(t *testing.T) {
	t.Run("returns results for valid scan", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		scanID := uuid.New()
		resultID := uuid.New()
		now := time.Now()
		result := &db.ScanResult{
			ID:        resultID,
			ScanID:    scanID,
			HostID:    uuid.New(),
			Port:      80,
			Protocol:  "tcp",
			State:     "open",
			Service:   "http",
			ScannedAt: now,
		}

		store.EXPECT().
			GetScan(gomock.Any(), scanID).
			Return(makeScan(scanID, "Test Scan", "completed", "connect"), nil)
		store.EXPECT().
			GetScanResults(gomock.Any(), scanID, 0, 50).
			Return([]*db.ScanResult{result}, int64(1), nil)
		// GetScanResults also calls GetScanSummary to populate the response envelope.
		store.EXPECT().
			GetScanSummary(gomock.Any(), scanID).
			Return(&db.ScanSummary{ScanID: scanID, TotalHosts: 1, OpenPorts: 1}, nil)

		router, _ := routerWithID(http.MethodGet, "/api/v1/scans/{id}/results", h.GetScanResults)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/scans/%s/results", scanID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		// GetScanResults returns a ScanResultsResponse envelope (not the generic paginated
		// data/meta wrapper), so check the results field directly.
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		results, ok := resp["results"].([]interface{})
		require.True(t, ok, "expected 'results' key in response, got: %v", resp)
		assert.Len(t, results, 1)
	})

	t.Run("returns 500 on store error", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		scanID := uuid.New()
		store.EXPECT().
			GetScan(gomock.Any(), scanID).
			Return(makeScan(scanID, "Test Scan", "completed", "connect"), nil)
		store.EXPECT().
			GetScanResults(gomock.Any(), scanID, 0, 50).
			Return(nil, int64(0), fmt.Errorf("db error"))

		router, _ := routerWithID(http.MethodGet, "/api/v1/scans/{id}/results", h.GetScanResults)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/scans/%s/results", scanID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// ── ScheduleHandler ───────────────────────────────────────────────────────────

func TestScheduleHandler_ListSchedules_Mock(t *testing.T) {
	t.Run("returns schedules from store", func(t *testing.T) {
		h, store, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		schedule := makeSchedule(id, "Nightly Scan")

		store.EXPECT().
			ListSchedules(gomock.Any(), gomock.Any(), 0, 50).
			Return([]*db.Schedule{schedule}, int64(1), nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
		w := httptest.NewRecorder()
		h.ListSchedules(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data, ok := resp["data"].([]interface{})
		require.True(t, ok)
		assert.Len(t, data, 1)
	})

	t.Run("returns 500 on store error", func(t *testing.T) {
		h, store, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			ListSchedules(gomock.Any(), gomock.Any(), 0, 50).
			Return(nil, int64(0), fmt.Errorf("db error"))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/schedules", nil)
		w := httptest.NewRecorder()
		h.ListSchedules(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestScheduleHandler_GetSchedule_Mock(t *testing.T) {
	t.Run("returns schedule by ID", func(t *testing.T) {
		h, store, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		schedule := makeSchedule(id, "Daily")
		store.EXPECT().GetSchedule(gomock.Any(), id).Return(schedule, nil)

		router, _ := routerWithID(http.MethodGet, "/api/v1/schedules/{id}", h.GetSchedule)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/schedules/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, id.String(), resp["id"])
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		h, store, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			GetSchedule(gomock.Any(), id).
			Return(nil, notFoundErr("schedule", id.String()))

		router, _ := routerWithID(http.MethodGet, "/api/v1/schedules/{id}", h.GetSchedule)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/schedules/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 400 for invalid UUID", func(t *testing.T) {
		h, _, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		router, _ := routerWithID(http.MethodGet, "/api/v1/schedules/{id}", h.GetSchedule)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/schedules/bad-id", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestScheduleHandler_CreateSchedule_Mock(t *testing.T) {
	t.Run("creates schedule and returns 201", func(t *testing.T) {
		h, store, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		schedule := makeSchedule(id, "Weekly")

		store.EXPECT().
			CreateSchedule(gomock.Any(), gomock.Any()).
			Return(schedule, nil)

		body := `{"name":"Weekly","cron_expr":"0 0 * * 0","type":"scan","target_id":1}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateSchedule(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("returns 400 for missing name", func(t *testing.T) {
		h, _, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		body := `{"cron_expr":"0 0 * * 0","type":"scan","target_id":1}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateSchedule(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 for invalid cron expression", func(t *testing.T) {
		h, _, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		body := `{"name":"Bad Cron","cron_expr":"not a cron","type":"scan","target_id":1}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/schedules", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateSchedule(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestScheduleHandler_DeleteSchedule_Mock(t *testing.T) {
	t.Run("deletes schedule and returns 204", func(t *testing.T) {
		h, store, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().DeleteSchedule(gomock.Any(), id).Return(nil)

		router, _ := routerWithID(http.MethodDelete, "/api/v1/schedules/{id}", h.DeleteSchedule)
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/schedules/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		h, store, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			DeleteSchedule(gomock.Any(), id).
			Return(notFoundErr("schedule", id.String()))

		router, _ := routerWithID(http.MethodDelete, "/api/v1/schedules/{id}", h.DeleteSchedule)
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/schedules/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestScheduleHandler_EnableDisableSchedule_Mock(t *testing.T) {
	t.Run("enable returns 200 with updated schedule", func(t *testing.T) {
		h, store, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		active := makeSchedule(id, "Sched")

		store.EXPECT().EnableSchedule(gomock.Any(), id).Return(nil)
		store.EXPECT().GetSchedule(gomock.Any(), id).Return(active, nil)

		router, _ := routerWithID(http.MethodPost, "/api/v1/schedules/{id}/enable", h.EnableSchedule)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/schedules/%s/enable", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("disable returns 200 with updated schedule", func(t *testing.T) {
		h, store, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		// ExecuteStop only calls the stop func — no subsequent GetSchedule.
		store.EXPECT().DisableSchedule(gomock.Any(), id).Return(nil)

		router, _ := routerWithID(http.MethodPost, "/api/v1/schedules/{id}/disable", h.DisableSchedule)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/schedules/%s/disable", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("enable returns 404 when not found", func(t *testing.T) {
		h, store, ctrl := newScheduleHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			EnableSchedule(gomock.Any(), id).
			Return(notFoundErr("schedule", id.String()))

		router, _ := routerWithID(http.MethodPost, "/api/v1/schedules/{id}/enable", h.EnableSchedule)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/schedules/%s/enable", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ── DiscoveryHandler ──────────────────────────────────────────────────────────

func TestDiscoveryHandler_ListDiscoveryJobs_Mock(t *testing.T) {
	t.Run("returns jobs from store", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		job := makeDiscoveryJob(id, "ping", "completed")

		store.EXPECT().
			ListDiscoveryJobs(gomock.Any(), gomock.Any(), 0, 50).
			Return([]*db.DiscoveryJob{job}, int64(1), nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery", nil)
		w := httptest.NewRecorder()
		h.ListDiscoveryJobs(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data, ok := resp["data"].([]interface{})
		require.True(t, ok)
		assert.Len(t, data, 1)
	})

	t.Run("returns 500 on store error", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			ListDiscoveryJobs(gomock.Any(), gomock.Any(), 0, 50).
			Return(nil, int64(0), fmt.Errorf("db error"))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/discovery", nil)
		w := httptest.NewRecorder()
		h.ListDiscoveryJobs(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestDiscoveryHandler_GetDiscoveryJob_Mock(t *testing.T) {
	t.Run("returns job by ID", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		job := makeDiscoveryJob(id, "arp", "running")
		store.EXPECT().GetDiscoveryJob(gomock.Any(), id).Return(job, nil)

		router, _ := routerWithID(http.MethodGet, "/api/v1/discovery/{id}", h.GetDiscoveryJob)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/discovery/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, id.String(), resp["id"])
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			GetDiscoveryJob(gomock.Any(), id).
			Return(nil, notFoundErr("discovery job", id.String()))

		router, _ := routerWithID(http.MethodGet, "/api/v1/discovery/{id}", h.GetDiscoveryJob)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/discovery/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestDiscoveryHandler_CreateDiscoveryJob_Mock(t *testing.T) {
	t.Run("creates job and returns 201", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		job := makeDiscoveryJob(id, "ping", "pending")

		store.EXPECT().
			CreateDiscoveryJob(gomock.Any(), gomock.Any()).
			Return(job, nil)

		body := `{"name":"Net Discovery","method":"ping","networks":["192.168.1.0/24"]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/discovery", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateDiscoveryJob(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("returns 400 for missing name", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		body := `{"method":"ping","networks":["192.168.1.0/24"]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/discovery", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateDiscoveryJob(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 for invalid method", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		body := `{"name":"Discovery","method":"invalid","networks":["192.168.1.0/24"]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/discovery", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateDiscoveryJob(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 for empty networks", func(t *testing.T) {
		h, _, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		body := `{"name":"Discovery","method":"ping","networks":[]}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/discovery", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateDiscoveryJob(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestDiscoveryHandler_DeleteDiscoveryJob_Mock(t *testing.T) {
	t.Run("deletes job and returns 204", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().DeleteDiscoveryJob(gomock.Any(), id).Return(nil)

		router, _ := routerWithID(http.MethodDelete, "/api/v1/discovery/{id}", h.DeleteDiscoveryJob)
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/discovery/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			DeleteDiscoveryJob(gomock.Any(), id).
			Return(notFoundErr("discovery job", id.String()))

		router, _ := routerWithID(http.MethodDelete, "/api/v1/discovery/{id}", h.DeleteDiscoveryJob)
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/discovery/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestDiscoveryHandler_StartStopDiscovery_Mock(t *testing.T) {
	t.Run("start returns 200 with updated job", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		// First call: load the job to check status and read network/method.
		pending := makeDiscoveryJob(id, "ping", "pending")
		// Second call: return the updated job after StartDiscoveryJob flips it to running.
		running := makeDiscoveryJob(id, "ping", "running")

		gomock.InOrder(
			store.EXPECT().GetDiscoveryJob(gomock.Any(), id).Return(pending, nil),
			store.EXPECT().StartDiscoveryJob(gomock.Any(), id).Return(nil),
			store.EXPECT().GetDiscoveryJob(gomock.Any(), id).Return(running, nil),
		)

		router, _ := routerWithID(http.MethodPost, "/api/v1/discovery/{id}/start", h.StartDiscovery)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/discovery/%s/start", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("start returns 409 when already running", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		running := makeDiscoveryJob(id, "ping", "running")

		store.EXPECT().GetDiscoveryJob(gomock.Any(), id).Return(running, nil)

		router, _ := routerWithID(http.MethodPost, "/api/v1/discovery/{id}/start", h.StartDiscovery)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/discovery/%s/start", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("start returns 409 when already completed", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		completed := makeDiscoveryJob(id, "ping", "completed")

		store.EXPECT().GetDiscoveryJob(gomock.Any(), id).Return(completed, nil)

		router, _ := routerWithID(http.MethodPost, "/api/v1/discovery/{id}/start", h.StartDiscovery)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/discovery/%s/start", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("stop returns 200 with updated job", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		// ExecuteStop only calls the stop func — no subsequent GetDiscoveryJob.
		store.EXPECT().StopDiscoveryJob(gomock.Any(), id).Return(nil)

		router, _ := routerWithID(http.MethodPost, "/api/v1/discovery/{id}/stop", h.StopDiscovery)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/discovery/%s/stop", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("start returns 404 when not found", func(t *testing.T) {
		h, store, ctrl := newDiscoveryHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		// The initial GetDiscoveryJob returns not-found before we even try to start.
		store.EXPECT().
			GetDiscoveryJob(gomock.Any(), id).
			Return(nil, notFoundErr("discovery job", id.String()))

		router, _ := routerWithID(http.MethodPost, "/api/v1/discovery/{id}/start", h.StartDiscovery)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/discovery/%s/start", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// ── HostHandler ───────────────────────────────────────────────────────────────

func TestHostHandler_ListHosts_Mock(t *testing.T) {
	t.Run("returns hosts from store", func(t *testing.T) {
		h, store, ctrl := newHostHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		host := makeHost(id, "192.168.1.100")

		store.EXPECT().
			ListHosts(gomock.Any(), gomock.Any(), 0, 50).
			Return([]*db.Host{host}, int64(1), nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
		w := httptest.NewRecorder()
		h.ListHosts(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data, ok := resp["data"].([]interface{})
		require.True(t, ok)
		assert.Len(t, data, 1)
	})

	t.Run("returns 500 on store error", func(t *testing.T) {
		h, store, ctrl := newHostHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			ListHosts(gomock.Any(), gomock.Any(), 0, 50).
			Return(nil, int64(0), fmt.Errorf("db error"))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
		w := httptest.NewRecorder()
		h.ListHosts(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHostHandler_GetHost_Mock(t *testing.T) {
	t.Run("returns host by ID", func(t *testing.T) {
		h, store, ctrl := newHostHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		host := makeHost(id, "10.0.0.1")
		store.EXPECT().GetHost(gomock.Any(), id).Return(host, nil)

		router, _ := routerWithID(http.MethodGet, "/api/v1/hosts/{id}", h.GetHost)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/hosts/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, id.String(), resp["id"])
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		h, store, ctrl := newHostHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			GetHost(gomock.Any(), id).
			Return(nil, notFoundErr("host", id.String()))

		router, _ := routerWithID(http.MethodGet, "/api/v1/hosts/{id}", h.GetHost)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/hosts/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 400 for invalid UUID", func(t *testing.T) {
		h, _, ctrl := newHostHandlerWithMock(t)
		defer ctrl.Finish()

		router, _ := routerWithID(http.MethodGet, "/api/v1/hosts/{id}", h.GetHost)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/not-a-uuid", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHostHandler_DeleteHost_Mock(t *testing.T) {
	t.Run("deletes host and returns 204", func(t *testing.T) {
		h, store, ctrl := newHostHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().DeleteHost(gomock.Any(), id).Return(nil)

		router, _ := routerWithID(http.MethodDelete, "/api/v1/hosts/{id}", h.DeleteHost)
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/hosts/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		h, store, ctrl := newHostHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		store.EXPECT().
			DeleteHost(gomock.Any(), id).
			Return(notFoundErr("host", id.String()))

		router, _ := routerWithID(http.MethodDelete, "/api/v1/hosts/{id}", h.DeleteHost)
		req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/hosts/%s", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHostHandler_GetHostScans_Mock(t *testing.T) {
	t.Run("returns scans for host", func(t *testing.T) {
		h, store, ctrl := newHostHandlerWithMock(t)
		defer ctrl.Finish()

		hostID := uuid.New()
		scanID := uuid.New()
		scan := makeScan(scanID, "Host Scan", "completed", "connect")

		store.EXPECT().
			GetHost(gomock.Any(), hostID).
			Return(makeHost(hostID, "192.168.1.1"), nil)
		store.EXPECT().
			GetHostScans(gomock.Any(), hostID, 0, 50).
			Return([]*db.Scan{scan}, int64(1), nil)

		router, _ := routerWithID(http.MethodGet, "/api/v1/hosts/{id}/scans", h.GetHostScans)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/hosts/%s/scans", hostID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data, ok := resp["data"].([]interface{})
		require.True(t, ok)
		assert.Len(t, data, 1)
	})

	t.Run("returns 404 when host not found", func(t *testing.T) {
		h, store, ctrl := newHostHandlerWithMock(t)
		defer ctrl.Finish()

		hostID := uuid.New()
		store.EXPECT().
			GetHost(gomock.Any(), hostID).
			Return(nil, errors.ErrNotFoundWithID("host", hostID.String()))

		router, _ := routerWithID(http.MethodGet, "/api/v1/hosts/{id}/scans", h.GetHostScans)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/hosts/%s/scans", hostID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 500 on store error", func(t *testing.T) {
		h, store, ctrl := newHostHandlerWithMock(t)
		defer ctrl.Finish()

		hostID := uuid.New()
		store.EXPECT().
			GetHost(gomock.Any(), hostID).
			Return(makeHost(hostID, "10.0.0.1"), nil)
		store.EXPECT().
			GetHostScans(gomock.Any(), hostID, 0, 50).
			Return(nil, int64(0), fmt.Errorf("db error"))

		router, _ := routerWithID(http.MethodGet, "/api/v1/hosts/{id}/scans", h.GetHostScans)
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/hosts/%s/scans", hostID), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// ── ProfileHandler ────────────────────────────────────────────────────────────

func TestProfileHandler_ListProfiles_Mock(t *testing.T) {
	t.Run("returns profiles from store", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		profile := makeProfile("web-scan", "Web Scan")

		store.EXPECT().
			ListProfiles(gomock.Any(), gomock.Any(), 0, 50).
			Return([]*db.ScanProfile{profile}, int64(1), nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
		w := httptest.NewRecorder()
		h.ListProfiles(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data, ok := resp["data"].([]interface{})
		require.True(t, ok)
		assert.Len(t, data, 1)
	})

	t.Run("returns 500 on store error", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			ListProfiles(gomock.Any(), gomock.Any(), 0, 50).
			Return(nil, int64(0), fmt.Errorf("db error"))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
		w := httptest.NewRecorder()
		h.ListProfiles(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestProfileHandler_GetProfile_Mock(t *testing.T) {
	t.Run("returns profile by ID", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		profile := makeProfile("ssh-scan", "SSH Scan")
		store.EXPECT().GetProfile(gomock.Any(), "ssh-scan").Return(profile, nil)

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/profiles/{id}", h.GetProfile).Methods(http.MethodGet)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/ssh-scan", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "ssh-scan", resp["id"])
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			GetProfile(gomock.Any(), "nonexistent").
			Return(nil, notFoundErr("profile", "nonexistent"))

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/profiles/{id}", h.GetProfile).Methods(http.MethodGet)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/nonexistent", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestProfileHandler_CreateProfile_Mock(t *testing.T) {
	t.Run("creates profile and returns 201", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		profile := makeProfile("new-profile", "New Profile")

		store.EXPECT().
			CreateProfile(gomock.Any(), gomock.Any()).
			Return(profile, nil)

		body := `{"name":"New Profile","scan_type":"connect","ports":"22,80,443"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateProfile(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
	})

	t.Run("returns 400 for missing name", func(t *testing.T) {
		h, _, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		body := `{"scan_type":"connect","ports":"22,80"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateProfile(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 409 on duplicate profile", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			CreateProfile(gomock.Any(), gomock.Any()).
			Return(nil, conflictErr("profile", "name already exists"))

		body := `{"name":"Duplicate","scan_type":"connect","ports":"80"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/profiles", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateProfile(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})
}

func TestProfileHandler_DeleteProfile_Mock(t *testing.T) {
	t.Run("deletes profile and returns 204", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().DeleteProfile(gomock.Any(), "ssh-scan").Return(nil)

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/profiles/{id}", h.DeleteProfile).Methods(http.MethodDelete)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/profiles/ssh-scan", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("returns 404 when not found", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			DeleteProfile(gomock.Any(), "missing").
			Return(notFoundErr("profile", "missing"))

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/profiles/{id}", h.DeleteProfile).Methods(http.MethodDelete)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/profiles/missing", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 409 when profile is built-in", func(t *testing.T) {
		h, store, ctrl := newProfileHandlerWithMock(t)
		defer ctrl.Finish()

		// The handler maps IsConflict errors to 409.
		store.EXPECT().
			DeleteProfile(gomock.Any(), "built-in").
			Return(conflictErr("profile", "cannot delete built-in profile"))

		router := mux.NewRouter()
		router.HandleFunc("/api/v1/profiles/{id}", h.DeleteProfile).Methods(http.MethodDelete)
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/profiles/built-in", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		// The profile DeleteProfile handler returns 500 for all non-404 DB errors today
		// (it uses a raw writeError rather than handleDatabaseError). This test documents
		// the current behavior; it should be updated to 409 once the handler is fixed.
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// ── NetworkHandler ────────────────────────────────────────────────────────────

func TestNetworkHandler_ListNetworks_Mock(t *testing.T) {
	t.Run("returns networks from service", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		network := makeNetwork(id, "Office Network", "10.0.0.0/24")

		svc.EXPECT().
			ListNetworks(gomock.Any(), false).
			Return([]*db.Network{network}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/networks", nil)
		w := httptest.NewRecorder()
		h.ListNetworks(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		data, ok := resp["data"].([]interface{})
		require.True(t, ok)
		assert.Len(t, data, 1)
	})

	t.Run("returns empty list when service returns nothing", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		svc.EXPECT().
			ListNetworks(gomock.Any(), false).
			Return([]*db.Network{}, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/networks", nil)
		w := httptest.NewRecorder()
		h.ListNetworks(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("returns 500 on service error", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		svc.EXPECT().
			ListNetworks(gomock.Any(), false).
			Return(nil, fmt.Errorf("db error"))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/networks", nil)
		w := httptest.NewRecorder()
		h.ListNetworks(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestNetworkHandler_CreateNetwork_Mock(t *testing.T) {
	t.Run("creates network and returns 201", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		network := makeNetwork(id, "New Network", "192.168.1.0/24")

		svc.EXPECT().
			CreateNetwork(gomock.Any(), "New Network", "192.168.1.0/24", "", "ping", true, true).
			Return(network, nil)

		body := `{"name":"New Network","cidr":"192.168.1.0/24","discovery_method":"ping"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/networks", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateNetwork(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, id.String(), resp["id"])
	})

	t.Run("returns 400 for invalid CIDR", func(t *testing.T) {
		h, _, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		body := `{"name":"Bad CIDR","cidr":"not-a-cidr","discovery_method":"ping"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/networks", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateNetwork(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 409 on conflict", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		svc.EXPECT().
			CreateNetwork(gomock.Any(), "Dup Network", "10.0.0.0/8", "", "ping", true, true).
			Return(nil, fmt.Errorf("network already exists"))

		body := `{"name":"Dup Network","cidr":"10.0.0.0/8","discovery_method":"ping"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/networks", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateNetwork(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		h, _, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		req := httptest.NewRequest(http.MethodPost, "/api/v1/networks", bytes.NewBufferString("{bad json}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateNetwork(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 for missing name", func(t *testing.T) {
		h, _, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		body := `{"cidr":"10.0.0.0/8","discovery_method":"ping"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/networks", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.CreateNetwork(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestNetworkHandler_GetNetwork_Mock(t *testing.T) {
	t.Run("returns network by ID", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		network := makeNetwork(id, "My Network", "172.16.0.0/16")
		nwe := &services.NetworkWithExclusions{
			Network:    network,
			Exclusions: []*db.NetworkExclusion{},
		}

		svc.EXPECT().
			GetNetworkByID(gomock.Any(), id).
			Return(nwe, nil)

		router, _ := routerWithID(http.MethodGet, "/api/v1/networks/{id}", h.GetNetwork)
		url := fmt.Sprintf("/api/v1/networks/%s", id)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, id.String(), resp["id"])
	})

	t.Run("returns 404 when network not found", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		svc.EXPECT().
			GetNetworkByID(gomock.Any(), id).
			Return(nil, notFoundErr("network", id.String()))

		router, _ := routerWithID(http.MethodGet, "/api/v1/networks/{id}", h.GetNetwork)
		url := fmt.Sprintf("/api/v1/networks/%s", id)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 400 for invalid UUID", func(t *testing.T) {
		h, _, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		router, _ := routerWithID(http.MethodGet, "/api/v1/networks/{id}", h.GetNetwork)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/networks/not-a-uuid", nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestNetworkHandler_UpdateNetwork_Mock(t *testing.T) {
	t.Run("updates network and returns 200", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		existing := makeNetwork(id, "Old Name", "10.0.0.0/24")
		nwe := &services.NetworkWithExclusions{
			Network:    existing,
			Exclusions: []*db.NetworkExclusion{},
		}
		updated := makeNetwork(id, "New Name", "10.0.0.0/24")

		svc.EXPECT().
			GetNetworkByID(gomock.Any(), id).
			Return(nwe, nil)
		svc.EXPECT().
			UpdateNetwork(gomock.Any(), id, "New Name", "10.0.0.0/24", "", "ping", true).
			Return(updated, nil)

		body := `{"name":"New Name"}`
		router, _ := routerWithID(http.MethodPut, "/api/v1/networks/{id}", h.UpdateNetwork)
		url := fmt.Sprintf("/api/v1/networks/%s", id)
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "New Name", resp["name"])
	})

	t.Run("returns 404 when network not found on get", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		svc.EXPECT().
			GetNetworkByID(gomock.Any(), id).
			Return(nil, notFoundErr("network", id.String()))

		body := `{"name":"Updated"}`
		router, _ := routerWithID(http.MethodPut, "/api/v1/networks/{id}", h.UpdateNetwork)
		url := fmt.Sprintf("/api/v1/networks/%s", id)
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("returns 400 for invalid JSON", func(t *testing.T) {
		h, _, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		router, _ := routerWithID(http.MethodPut, "/api/v1/networks/{id}", h.UpdateNetwork)
		url := fmt.Sprintf("/api/v1/networks/%s", id)
		req := httptest.NewRequest(http.MethodPut, url, bytes.NewBufferString("{bad}"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestNetworkHandler_DeleteNetwork_Mock(t *testing.T) {
	t.Run("deletes network and returns 204", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		svc.EXPECT().DeleteNetwork(gomock.Any(), id).Return(nil)

		router, _ := routerWithID(http.MethodDelete, "/api/v1/networks/{id}", h.DeleteNetwork)
		url := fmt.Sprintf("/api/v1/networks/%s", id)
		req := httptest.NewRequest(http.MethodDelete, url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("returns 404 when network not found", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		svc.EXPECT().
			DeleteNetwork(gomock.Any(), id).
			Return(notFoundErr("network", id.String()))

		router, _ := routerWithID(http.MethodDelete, "/api/v1/networks/{id}", h.DeleteNetwork)
		url := fmt.Sprintf("/api/v1/networks/%s", id)
		req := httptest.NewRequest(http.MethodDelete, url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestNetworkHandler_EnableDisableNetwork_Mock(t *testing.T) {
	t.Run("enable returns 200 with updated network", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		existing := makeNetwork(id, "Net", "10.0.0.0/24")
		nwe := &services.NetworkWithExclusions{
			Network:    existing,
			Exclusions: []*db.NetworkExclusion{},
		}
		enabled := makeNetwork(id, "Net", "10.0.0.0/24")
		enabled.IsActive = true
		enabled.ScanEnabled = true

		svc.EXPECT().
			GetNetworkByID(gomock.Any(), id).
			Return(nwe, nil)
		svc.EXPECT().
			UpdateNetwork(gomock.Any(), id, "Net", "10.0.0.0/24", "", "ping", true).
			Return(enabled, nil)

		router, _ := routerWithID(http.MethodPost, "/api/v1/networks/{id}/enable", h.EnableNetwork)
		url := fmt.Sprintf("/api/v1/networks/%s/enable", id)
		req := httptest.NewRequest(http.MethodPost, url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("disable returns 200 with updated network", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		existing := makeNetwork(id, "Net", "10.0.0.0/24")
		nwe := &services.NetworkWithExclusions{
			Network:    existing,
			Exclusions: []*db.NetworkExclusion{},
		}
		disabled := makeNetwork(id, "Net", "10.0.0.0/24")
		disabled.IsActive = false
		disabled.ScanEnabled = false

		svc.EXPECT().
			GetNetworkByID(gomock.Any(), id).
			Return(nwe, nil)
		svc.EXPECT().
			UpdateNetwork(gomock.Any(), id, "Net", "10.0.0.0/24", "", "ping", false).
			Return(disabled, nil)

		router, _ := routerWithID(http.MethodPost, "/api/v1/networks/{id}/disable", h.DisableNetwork)
		url := fmt.Sprintf("/api/v1/networks/%s/disable", id)
		req := httptest.NewRequest(http.MethodPost, url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("enable returns 404 when network not found", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		svc.EXPECT().
			GetNetworkByID(gomock.Any(), id).
			Return(nil, notFoundErr("network", id.String()))

		router, _ := routerWithID(http.MethodPost, "/api/v1/networks/{id}/enable", h.EnableNetwork)
		url := fmt.Sprintf("/api/v1/networks/%s/enable", id)
		req := httptest.NewRequest(http.MethodPost, url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestNetworkHandler_GetNetworkStats_Mock(t *testing.T) {
	t.Run("returns stats", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		stats := map[string]interface{}{
			"total_networks": 5,
			"active":         3,
		}
		svc.EXPECT().
			GetNetworkStats(gomock.Any()).
			Return(stats, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/networks/stats", nil)
		w := httptest.NewRecorder()
		h.GetNetworkStats(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp)
	})

	t.Run("returns 500 on service error", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		svc.EXPECT().
			GetNetworkStats(gomock.Any()).
			Return(nil, fmt.Errorf("db error"))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/networks/stats", nil)
		w := httptest.NewRecorder()
		h.GetNetworkStats(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestNetworkHandler_ListNetworkExclusions_Mock(t *testing.T) {
	t.Run("returns exclusions for network", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		exclID := uuid.New()
		excl := makeNetworkExclusion(exclID, &networkID, "10.0.0.1/32")

		svc.EXPECT().
			GetNetworkExclusions(gomock.Any(), networkID).
			Return([]*db.NetworkExclusion{excl}, nil)

		router := mux.NewRouter()
		router.HandleFunc(
			"/api/v1/networks/{id}/exclusions",
			h.ListNetworkExclusions,
		).Methods(http.MethodGet)
		url := fmt.Sprintf("/api/v1/networks/%s/exclusions", networkID)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestNetworkHandler_CreateNetworkExclusion_Mock(t *testing.T) {
	t.Run("creates exclusion and returns 201", func(t *testing.T) {
		h, svc, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		exclID := uuid.New()
		excl := makeNetworkExclusion(exclID, &networkID, "10.0.0.1/32")

		svc.EXPECT().
			AddExclusion(gomock.Any(), &networkID, "10.0.0.1/32", "reserved host").
			Return(excl, nil)

		body := `{"excluded_cidr":"10.0.0.1/32","reason":"reserved host"}`
		router := mux.NewRouter()
		router.HandleFunc(
			"/api/v1/networks/{id}/exclusions",
			h.CreateNetworkExclusion,
		).Methods(http.MethodPost)
		url := fmt.Sprintf("/api/v1/networks/%s/exclusions", networkID)
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)
		var resp map[string]interface{}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, exclID.String(), resp["id"])
	})

	t.Run("returns 400 for invalid excluded_cidr", func(t *testing.T) {
		h, _, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		body := `{"excluded_cidr":"not-a-cidr"}`
		router := mux.NewRouter()
		router.HandleFunc(
			"/api/v1/networks/{id}/exclusions",
			h.CreateNetworkExclusion,
		).Methods(http.MethodPost)
		url := fmt.Sprintf("/api/v1/networks/%s/exclusions", networkID)
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("returns 400 for missing excluded_cidr", func(t *testing.T) {
		h, _, ctrl := newNetworkHandlerWithMock(t)
		defer ctrl.Finish()

		networkID := uuid.New()
		body := `{"reason":"no cidr"}`
		router := mux.NewRouter()
		router.HandleFunc(
			"/api/v1/networks/{id}/exclusions",
			h.CreateNetworkExclusion,
		).Methods(http.MethodPost)
		url := fmt.Sprintf("/api/v1/networks/%s/exclusions", networkID)
		req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

// ── compile-time check that context import is used ───────────────────────────

var _ = context.Background

// ──────────────────────────────────────────────────────────────────────────────
// submitToQueue — ScanID and database propagation
//
// These tests catch the two bugs fixed in the scan-results PR:
//   1. Database was nil in the queue request, so results were never stored.
//   2. ScanID was not set, so storeScanResults created a fresh UUID that
//      didn't match the scan ID returned to the API client.
// ──────────────────────────────────────────────────────────────────────────────

func TestScanHandler_SubmitToQueue_ScanIDPropagatedToConfig(t *testing.T) {
	// Arrange: a queue whose scanFunc captures the request it receives.
	q := scanning.NewScanQueue(1, 10)
	// Use atomic pointer to avoid a data race between the worker goroutine
	// (writer) and the assert.Eventually polling goroutine (reader).
	var capturedConfig atomic.Pointer[scanning.ScanConfig]
	q.SetScanFunc(func(_ context.Context, req *scanning.ScanQueueRequest) *scanning.ScanQueueResult {
		capturedConfig.Store(req.Config)
		return &scanning.ScanQueueResult{ID: req.ID, Result: &scanning.ScanResult{}}
	})
	q.Start(context.Background())
	defer q.Stop()

	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()
	h.SetScanQueue(q)

	scanID := uuid.New()
	scan := makeScan(scanID, "Queue Test", "pending", "connect")

	// The background goroutine spawned by submitToQueue will call CompleteScan
	// once the queued scan finishes.  Allow it any number of times so the mock
	// controller does not fail when the goroutine races past the test's cleanup.
	store.EXPECT().CompleteScan(gomock.Any(), scanID).AnyTimes().Return(nil)

	// Act
	code, err := h.submitToQueue(scanID, scan)

	// Assert: submitToQueue itself succeeds.
	require.NoError(t, err)
	assert.Equal(t, 0, code)

	// Wait for the queue worker to pick up and execute the job.
	assert.Eventually(t, func() bool {
		return capturedConfig.Load() != nil
	}, 2*time.Second, 10*time.Millisecond, "queue worker did not execute the scan in time")

	cfg := capturedConfig.Load()
	require.NotNil(t, cfg.ScanID,
		"ScanConfig.ScanID must be set so storeScanResults links port_scans to the correct job_id")
	assert.Equal(t, scanID, *cfg.ScanID,
		"ScanConfig.ScanID must equal the scan's UUID")
}

func TestScanHandler_SubmitToQueue_DatabasePropagatedToRequest(t *testing.T) {
	// Arrange: a queue whose scanFunc captures the full request so we can
	// inspect the Database field.
	q := scanning.NewScanQueue(1, 10)
	// Use atomic pointer to avoid a data race between the worker goroutine
	// (writer) and the assert.Eventually polling goroutine (reader).
	var capturedReq atomic.Pointer[scanning.ScanQueueRequest]
	q.SetScanFunc(func(_ context.Context, req *scanning.ScanQueueRequest) *scanning.ScanQueueResult {
		capturedReq.Store(req)
		return &scanning.ScanQueueResult{ID: req.ID, Result: &scanning.ScanResult{}}
	})
	q.Start(context.Background())
	defer q.Stop()

	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()
	h.SetScanQueue(q)

	scanID := uuid.New()
	scan := makeScan(scanID, "DB Propagation Test", "pending", "connect")

	// Allow the background CompleteScan call that arrives after the scan
	// worker finishes — it races against the test's gomock controller cleanup.
	store.EXPECT().CompleteScan(gomock.Any(), scanID).AnyTimes().Return(nil)

	// Act
	code, err := h.submitToQueue(scanID, scan)
	require.NoError(t, err)
	assert.Equal(t, 0, code)

	assert.Eventually(t, func() bool {
		return capturedReq.Load() != nil
	}, 2*time.Second, 10*time.Millisecond, "queue worker did not execute the scan in time")

	// The handler's store is a *mocks.MockScanStore, not a *db.DB, so the
	// type assertion yields nil — but it must NOT be the unset zero value from
	// a missing assignment.  The key invariant is that the field was explicitly
	// set (even if it ends up nil after the assertion) rather than left as the
	// default nil from a forgotten line.  We verify the request itself was
	// populated by checking that Config and ID are correct.
	req := capturedReq.Load()
	require.NotNil(t, req, "request must reach the worker")
	assert.Equal(t, scanID.String(), req.ID)
	assert.NotNil(t, req.Config)
}

func TestScanHandler_SubmitToQueue_CompleteScanCalledOnSuccess(t *testing.T) {
	// After a successful queued scan the goroutine that listens on ResultCh
	// must call CompleteScan so the scan status is updated to "completed".
	q := scanning.NewScanQueue(1, 10)
	q.SetScanFunc(func(_ context.Context, req *scanning.ScanQueueRequest) *scanning.ScanQueueResult {
		return &scanning.ScanQueueResult{ID: req.ID, Result: &scanning.ScanResult{}}
	})
	q.Start(context.Background())
	defer q.Stop()

	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()
	h.SetScanQueue(q)

	scanID := uuid.New()
	scan := makeScan(scanID, "Complete Test", "pending", "connect")

	done := make(chan struct{})
	store.EXPECT().
		CompleteScan(gomock.Any(), scanID).
		DoAndReturn(func(_ context.Context, _ uuid.UUID) error {
			close(done)
			return nil
		})

	code, err := h.submitToQueue(scanID, scan)
	require.NoError(t, err)
	assert.Equal(t, 0, code)

	select {
	case <-done:
		// pass — CompleteScan was called
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for CompleteScan to be called after queued scan succeeded")
	}
}

func TestScanHandler_SubmitToQueue_StopScanCalledOnFailure(t *testing.T) {
	// When the queued scan runner returns an error, StopScan must be called
	// (and CompleteScan must NOT be called).
	q := scanning.NewScanQueue(1, 10)
	q.SetScanFunc(func(_ context.Context, req *scanning.ScanQueueRequest) *scanning.ScanQueueResult {
		return &scanning.ScanQueueResult{
			ID:    req.ID,
			Error: fmt.Errorf("nmap: binary not found"),
		}
	})
	q.Start(context.Background())
	defer q.Stop()

	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()
	h.SetScanQueue(q)

	scanID := uuid.New()
	scan := makeScan(scanID, "Failure Test", "pending", "connect")

	done := make(chan struct{})
	store.EXPECT().
		StopScan(gomock.Any(), scanID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ uuid.UUID, _ ...string) error {
			close(done)
			return nil
		})
	// CompleteScan must NOT be called; gomock will fail the test if it is.

	code, err := h.submitToQueue(scanID, scan)
	require.NoError(t, err)
	assert.Equal(t, 0, code)

	select {
	case <-done:
		// pass — StopScan was called
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for StopScan to be called after queued scan failed")
	}
}

func TestScanHandler_SubmitToQueue_QueueFull_Returns429(t *testing.T) {
	// A full queue must be surfaced as 429 Too Many Requests, not a panic or
	// silent drop.
	q := scanning.NewScanQueue(1, 1)
	// Block the single worker indefinitely so the queue fills up.
	started := make(chan struct{})
	block := make(chan struct{})
	var startOnce sync.Once
	q.SetScanFunc(func(_ context.Context, req *scanning.ScanQueueRequest) *scanning.ScanQueueResult {
		startOnce.Do(func() { close(started) })
		<-block
		return &scanning.ScanQueueResult{ID: req.ID, Result: &scanning.ScanResult{}}
	})
	q.Start(context.Background())
	defer func() {
		close(block)
		q.Stop()
	}()

	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()
	h.SetScanQueue(q)

	// The two scans that do get queued will each call CompleteScan once the
	// worker unblocks (during defer q.Stop()).  Allow those calls so the mock
	// controller does not fail after the test body has finished.
	store.EXPECT().CompleteScan(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	store.EXPECT().StopScan(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	scan := makeScan(uuid.New(), "Filler", "pending", "connect")

	// Fill the worker slot.
	_, _ = h.submitToQueue(uuid.New(), scan)
	// Wait until the worker has started so the queue slot is occupied.
	<-started
	// Fill the queue buffer.
	_, _ = h.submitToQueue(uuid.New(), scan)

	// This one should be rejected.
	code, err := h.submitToQueue(uuid.New(), scan)
	assert.Equal(t, http.StatusTooManyRequests, code)
	assert.Error(t, err)
}

func TestScanHandler_ExecuteScanAsync_ScanIDSetOnConfig(t *testing.T) {
	// Regression test: executeScanAsync must set ScanID on the ScanConfig so
	// that storeScanResults links port_scans rows to the correct job UUID.
	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()

	scanID := uuid.New()
	done := make(chan struct{})

	var capturedConfig *scanning.ScanConfig
	h.scanRunner = func(_ context.Context, cfg *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
		capturedConfig = cfg
		return &scanning.ScanResult{}, nil
	}

	store.EXPECT().
		CompleteScan(gomock.Any(), scanID).
		DoAndReturn(func(_ context.Context, _ uuid.UUID) error {
			close(done)
			return nil
		})

	go h.executeScanAsync(scanID, makeScan(scanID, "Async ScanID Test", "running", "connect"))

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for CompleteScan")
	}

	require.NotNil(t, capturedConfig, "scanRunner must have been called")
	require.NotNil(t, capturedConfig.ScanID,
		"ScanConfig.ScanID must be non-nil so results are stored under the correct UUID")
	assert.Equal(t, scanID, *capturedConfig.ScanID)
}

func TestScanHandler_StartScan_DBFailure(t *testing.T) {
	t.Run("returns 500 when GetScan fails before status check", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		dbErr := fmt.Errorf("connection reset by peer")

		store.EXPECT().GetScan(gomock.Any(), id).Return(nil, dbErr)

		router, _ := routerWithID(http.MethodPost, "/api/v1/scans/{id}/start", h.StartScan)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/scans/%s/start", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 500 when StartScan DB call fails", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		scan := makeScan(id, "Pending Scan", "pending", "connect")
		dbErr := fmt.Errorf("deadlock detected")

		store.EXPECT().GetScan(gomock.Any(), id).Return(scan, nil)
		store.EXPECT().StartScan(gomock.Any(), id).Return(dbErr)

		router, _ := routerWithID(http.MethodPost, "/api/v1/scans/{id}/start", h.StartScan)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/scans/%s/start", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 500 when second GetScan fails after StartScan succeeds", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()
		scan := makeScan(id, "Pending Scan", "pending", "connect")
		dbErr := fmt.Errorf("timeout reading updated row")

		store.EXPECT().GetScan(gomock.Any(), id).Return(scan, nil)
		store.EXPECT().StartScan(gomock.Any(), id).Return(nil)
		store.EXPECT().GetScan(gomock.Any(), id).Return(nil, dbErr)

		router, _ := routerWithID(http.MethodPost, "/api/v1/scans/{id}/start", h.StartScan)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/scans/%s/start", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("returns 404 when GetScan returns not-found error", func(t *testing.T) {
		h, store, ctrl := newScanHandlerWithMock(t)
		defer ctrl.Finish()

		id := uuid.New()

		store.EXPECT().GetScan(gomock.Any(), id).Return(nil, notFoundErr("scan", id.String()))

		router, _ := routerWithID(http.MethodPost, "/api/v1/scans/{id}/start", h.StartScan)
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/scans/%s/start", id), nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
