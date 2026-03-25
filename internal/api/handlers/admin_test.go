package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/logging"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/internal/scanning"
)

// Test helper functions

func createTestAdminHandler(t *testing.T) *AdminHandler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	registry := metrics.NewRegistry()

	return NewAdminHandler(logger, registry)
}

func createTestRequest(t *testing.T, method, path string, body interface{}) *http.Request {
	t.Helper()

	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		require.NoError(t, err)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), ContextKey("request_id"), "test-request-id")

	return req.WithContext(ctx)
}

func executeRequest(t *testing.T, handler http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	handler(rr, req)
	return rr
}

func assertJSONResponse(t *testing.T, rr *httptest.ResponseRecorder, target interface{}) {
	t.Helper()

	assert.Equal(t, http.StatusOK, rr.Code, "unexpected status code")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	if target != nil {
		err := json.NewDecoder(rr.Body).Decode(target)
		require.NoError(t, err, "failed to decode response body")
	}
}

// assertNotImplementedResponse verifies that a handler responded with 501 Not Implemented
// and a valid JSON error body. Use this for stub endpoints that are not yet built.
func assertNotImplementedResponse(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, http.StatusNotImplemented, rr.Code, "expected 501 Not Implemented")
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var resp ErrorResponse
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err, "failed to decode error response body")
	assert.Equal(t, "Not Implemented", resp.Error)
	assert.NotEmpty(t, resp.Message, "error message should not be empty")
}

// TestNewAdminHandler tests admin handler initialization
func TestNewAdminHandler(t *testing.T) {
	t.Run("initializes with all dependencies", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
		registry := metrics.NewRegistry()

		handler := NewAdminHandler(logger, registry)

		assert.NotNil(t, handler)
		assert.NotNil(t, handler.logger)
		assert.Equal(t, registry, handler.metrics)
		assert.NotNil(t, handler.validator)
	})
}

// newTestScanQueue creates a started ScanQueue backed by a no-op scan
// runner. t.Cleanup registers Stop so callers never need to defer it.
func newTestScanQueue(t *testing.T, workers, queueSize int) *scanning.ScanQueue {
	t.Helper()
	q := scanning.NewScanQueue(workers, queueSize)
	q.Start(context.Background())
	t.Cleanup(q.Stop)
	return q
}

// waitForWorkerStatus polls until fn returns true or the deadline is exceeded.
func waitForWorkerStatus(t *testing.T, h *AdminHandler, fn func(WorkerStatusResponse) bool) WorkerStatusResponse {
	t.Helper()
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/workers", nil)
		rr := executeRequest(t, h.GetWorkerStatus, req)
		var resp WorkerStatusResponse
		if err := json.NewDecoder(rr.Body).Decode(&resp); err == nil && fn(resp) {
			return resp
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for expected worker status")
			return WorkerStatusResponse{}
		case <-ticker.C:
		}
	}
}

// TestGetWorkerStatus_NoQueue tests behavior when no scan queue is wired.
func TestGetWorkerStatus_NoQueue(t *testing.T) {
	t.Run("returns valid empty response", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/workers", nil)

		rr := executeRequest(t, handler.GetWorkerStatus, req)

		var response WorkerStatusResponse
		assertJSONResponse(t, rr, &response)

		assert.Equal(t, 0, response.TotalWorkers)
		assert.Equal(t, 0, response.ActiveWorkers)
		assert.Equal(t, 0, response.IdleWorkers)
		assert.NotNil(t, response.Workers, "Workers slice should be non-nil")
		assert.Empty(t, response.Workers)
		assert.False(t, response.Timestamp.IsZero())
	})

	t.Run("summary keys always present", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/workers", nil)

		rr := executeRequest(t, handler.GetWorkerStatus, req)

		var response WorkerStatusResponse
		assertJSONResponse(t, rr, &response)

		assert.Contains(t, response.Summary, "total_scans_completed")
		assert.Contains(t, response.Summary, "total_discovery_completed")
		assert.Contains(t, response.Summary, "overall_error_rate")
	})
}

// TestGetWorkerStatus tests behavior when a real scan queue is wired.
func TestGetWorkerStatus(t *testing.T) {
	t.Run("worker count matches queue size", func(t *testing.T) {
		q := newTestScanQueue(t, 3, 20)
		handler := createTestAdminHandler(t).WithScanQueue(q)

		resp := waitForWorkerStatus(t, handler, func(r WorkerStatusResponse) bool {
			return r.TotalWorkers == 3
		})

		assert.Equal(t, 3, resp.TotalWorkers)
		assert.Equal(t, len(resp.Workers), resp.TotalWorkers)
	})

	t.Run("all workers idle at startup", func(t *testing.T) {
		q := newTestScanQueue(t, 2, 10)
		handler := createTestAdminHandler(t).WithScanQueue(q)

		resp := waitForWorkerStatus(t, handler, func(r WorkerStatusResponse) bool {
			return r.TotalWorkers == 2
		})

		assert.Equal(t, 2, resp.IdleWorkers)
		assert.Equal(t, 0, resp.ActiveWorkers)
		for _, w := range resp.Workers {
			assert.Equal(t, "idle", w.Status)
			assert.Nil(t, w.CurrentJob)
		}
	})

	t.Run("worker shows active with current job while scan runs", func(t *testing.T) {
		started := make(chan struct{})
		unblock := make(chan struct{})

		q := scanning.NewScanQueue(1, 10)
		q.Start(context.Background())
		t.Cleanup(q.Stop)

		handler := createTestAdminHandler(t).WithScanQueue(q)

		done := make(chan struct{})
		job := scanning.NewScanJob(
			"admin-test-active-1",
			&scanning.ScanConfig{Targets: []string{"10.0.0.1"}, Ports: "80", ScanType: "connect"},
			nil,
			func(_ context.Context, _ *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
				close(started)
				<-unblock
				return &scanning.ScanResult{}, nil
			},
			func(_ *scanning.ScanResult, _ error) { close(done) },
		)
		require.NoError(t, q.Submit(job))

		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("worker did not start the job in time")
		}

		resp := waitForWorkerStatus(t, handler, func(r WorkerStatusResponse) bool {
			return r.ActiveWorkers == 1
		})

		assert.Equal(t, 1, resp.ActiveWorkers)
		assert.Equal(t, 0, resp.IdleWorkers)
		require.Len(t, resp.Workers, 1)

		w := resp.Workers[0]
		assert.Equal(t, "active", w.Status)
		require.NotNil(t, w.CurrentJob)
		assert.Equal(t, "scan", w.CurrentJob.Type)
		assert.Equal(t, "10.0.0.1", w.CurrentJob.Target)

		close(unblock)
		<-done
	})

	t.Run("worker returns to idle after job completes", func(t *testing.T) {
		q := newTestScanQueue(t, 1, 10)
		handler := createTestAdminHandler(t).WithScanQueue(q)

		done := make(chan struct{})
		job := scanning.NewScanJob(
			"admin-test-idle-after-1",
			&scanning.ScanConfig{Targets: []string{"10.0.0.2"}, Ports: "80", ScanType: "connect"},
			nil,
			func(_ context.Context, _ *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
				return &scanning.ScanResult{}, nil
			},
			func(_ *scanning.ScanResult, _ error) { close(done) },
		)
		require.NoError(t, q.Submit(job))
		<-done

		resp := waitForWorkerStatus(t, handler, func(r WorkerStatusResponse) bool {
			return r.IdleWorkers == 1
		})

		assert.Equal(t, 1, resp.IdleWorkers)
		assert.Equal(t, 0, resp.ActiveWorkers)
		assert.Nil(t, resp.Workers[0].CurrentJob)
	})

	t.Run("processed jobs counter increments", func(t *testing.T) {
		q := newTestScanQueue(t, 1, 10)
		handler := createTestAdminHandler(t).WithScanQueue(q)

		const jobs = 3
		var wg sync.WaitGroup
		for i := range jobs {
			wg.Add(1)
			job := scanning.NewScanJob(
				fmt.Sprintf("admin-test-count-%d", i),
				&scanning.ScanConfig{Targets: []string{"127.0.0.1"}, Ports: "80", ScanType: "connect"},
				nil,
				func(_ context.Context, _ *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
					return &scanning.ScanResult{}, nil
				},
				func(_ *scanning.ScanResult, _ error) { wg.Done() },
			)
			require.NoError(t, q.Submit(job))
		}
		doneCh := make(chan struct{})
		go func() { wg.Wait(); close(doneCh) }()
		select {
		case <-doneCh:
		case <-time.After(5 * time.Second):
			t.Fatal("jobs did not complete in time")
		}

		resp := waitForWorkerStatus(t, handler, func(r WorkerStatusResponse) bool {
			return r.ProcessedJobs == jobs
		})

		assert.Equal(t, int64(jobs), resp.ProcessedJobs)
		assert.Equal(t, int64(0), resp.FailedJobs)
	})

	t.Run("worker count consistency", func(t *testing.T) {
		q := newTestScanQueue(t, 4, 20)
		handler := createTestAdminHandler(t).WithScanQueue(q)

		resp := waitForWorkerStatus(t, handler, func(r WorkerStatusResponse) bool {
			return r.TotalWorkers == 4
		})

		assert.Equal(t, resp.TotalWorkers, len(resp.Workers))

		active, idle := 0, 0
		for _, w := range resp.Workers {
			switch w.Status {
			case "active":
				active++
			case "idle":
				idle++
			}
		}
		assert.Equal(t, resp.ActiveWorkers, active)
		assert.Equal(t, resp.IdleWorkers, idle)
	})

	t.Run("summary keys always present", func(t *testing.T) {
		q := newTestScanQueue(t, 2, 10)
		handler := createTestAdminHandler(t).WithScanQueue(q)

		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/workers", nil)
		rr := executeRequest(t, handler.GetWorkerStatus, req)

		var resp WorkerStatusResponse
		assertJSONResponse(t, rr, &resp)

		assert.Contains(t, resp.Summary, "total_scans_completed")
		assert.Contains(t, resp.Summary, "total_discovery_completed")
		assert.Contains(t, resp.Summary, "overall_error_rate")
	})

	t.Run("worker start times are set", func(t *testing.T) {
		q := newTestScanQueue(t, 2, 10)
		handler := createTestAdminHandler(t).WithScanQueue(q)

		resp := waitForWorkerStatus(t, handler, func(r WorkerStatusResponse) bool {
			return r.TotalWorkers == 2
		})

		for _, w := range resp.Workers {
			assert.False(t, w.StartTime.IsZero(), "StartTime should be set for worker %s", w.ID)
		}
	})

	t.Run("concurrent jobs across multiple workers", func(t *testing.T) {
		const numWorkers = 3
		var mu sync.Mutex
		started := make([]bool, numWorkers)
		unblock := make(chan struct{})

		q := scanning.NewScanQueue(numWorkers, 20)
		q.Start(context.Background())
		t.Cleanup(q.Stop)

		handler := createTestAdminHandler(t).WithScanQueue(q)

		var wg sync.WaitGroup
		for i := range numWorkers {
			wg.Add(1)
			job := scanning.NewScanJob(
				fmt.Sprintf("admin-concurrent-%d", i),
				&scanning.ScanConfig{Targets: []string{"10.0.0.1"}, Ports: "80", ScanType: "connect"},
				nil,
				func(_ context.Context, _ *scanning.ScanConfig, _ *db.DB) (*scanning.ScanResult, error) {
					mu.Lock()
					for i := range started {
						if !started[i] {
							started[i] = true
							break
						}
					}
					mu.Unlock()
					<-unblock
					return &scanning.ScanResult{}, nil
				},
				func(_ *scanning.ScanResult, _ error) { wg.Done() },
			)
			require.NoError(t, q.Submit(job))
		}

		resp := waitForWorkerStatus(t, handler, func(r WorkerStatusResponse) bool {
			return r.ActiveWorkers == numWorkers
		})

		assert.Equal(t, numWorkers, resp.ActiveWorkers)
		assert.Equal(t, 0, resp.IdleWorkers)

		close(unblock)
		doneCh := make(chan struct{})
		go func() { wg.Wait(); close(doneCh) }()
		select {
		case <-doneCh:
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent jobs did not complete in time")
		}
	})
}

// TestStopWorker tests worker stop endpoint
func TestStopWorker(t *testing.T) {
	t.Run("returns 501 for valid worker ID (not yet implemented)", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers/worker-001/stop", nil)

		// Add worker ID to mux vars
		req = mux.SetURLVars(req, map[string]string{"id": "worker-001"})

		rr := executeRequest(t, handler.StopWorker, req)

		// Worker management is not yet implemented; expect 501.
		assertNotImplementedResponse(t, rr)
	})

	t.Run("returns 501 regardless of graceful default", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers/worker-001/stop", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "worker-001"})

		rr := executeRequest(t, handler.StopWorker, req)

		assertNotImplementedResponse(t, rr)
	})

	t.Run("returns 501 with graceful=true", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers/worker-001/stop?graceful=true", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "worker-001"})

		rr := executeRequest(t, handler.StopWorker, req)

		assertNotImplementedResponse(t, rr)
	})

	t.Run("returns 501 with graceful=false", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers/worker-001/stop?graceful=false", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "worker-001"})

		rr := executeRequest(t, handler.StopWorker, req)

		assertNotImplementedResponse(t, rr)
	})

	t.Run("rejects request without worker ID", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers/stop", nil)
		// No mux vars set - missing worker ID; validation still returns 400.

		rr := executeRequest(t, handler.StopWorker, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("returns 501 for all valid worker IDs", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		workerIDs := []string{"worker-001", "worker-002", "worker-999"}

		for _, workerID := range workerIDs {
			t.Run(fmt.Sprintf("worker_%s", workerID), func(t *testing.T) {
				path := fmt.Sprintf("/api/v1/admin/workers/%s/stop", workerID)
				req := createTestRequest(t, http.MethodPost, path, nil)
				req = mux.SetURLVars(req, map[string]string{"id": workerID})

				rr := executeRequest(t, handler.StopWorker, req)

				assertNotImplementedResponse(t, rr)
			})
		}
	})
}

// TestGetConfig tests configuration retrieval endpoint
func TestGetConfig(t *testing.T) {
	t.Run("retrieves full configuration", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/config", nil)

		rr := executeRequest(t, handler.GetConfig, req)

		var response map[string]interface{}
		assertJSONResponse(t, rr, &response)

		// Should return configuration data
		assert.NotNil(t, response)
	})

	t.Run("retrieves specific configuration section", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		sections := []string{"api", "database", "scanning", "logging", "daemon"}

		for _, section := range sections {
			t.Run(fmt.Sprintf("section_%s", section), func(t *testing.T) {
				path := fmt.Sprintf("/api/v1/admin/config?section=%s", section)
				req := createTestRequest(t, http.MethodGet, path, nil)

				rr := executeRequest(t, handler.GetConfig, req)

				// Should return 200 for valid sections
				assert.Equal(t, http.StatusOK, rr.Code)
			})
		}
	})

	t.Run("handles empty section parameter", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/config?section=", nil)

		rr := executeRequest(t, handler.GetConfig, req)

		// Empty section should return full config
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

// TestUpdateConfig tests configuration update endpoint
func TestUpdateConfig(t *testing.T) {
	t.Run("returns 501 for valid API config update (not yet implemented)", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		host := "0.0.0.0"
		port := 8080
		reqBody := ConfigUpdateRequest{
			Section: "api",
			Config: ConfigUpdateData{
				API: &APIConfigUpdate{
					Host: &host,
					Port: &port,
				},
			},
		}

		req := createTestRequest(t, http.MethodPut, "/api/v1/admin/config", reqBody)

		rr := executeRequest(t, handler.UpdateConfig, req)

		// Config persistence is not yet implemented; validation still runs, then 501.
		assertNotImplementedResponse(t, rr)
	})

	t.Run("rejects invalid JSON", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/config", bytes.NewBufferString("{invalid json}"))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), ContextKey("request_id"), "test-request-id")
		req = req.WithContext(ctx)

		rr := executeRequest(t, handler.UpdateConfig, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("rejects empty section", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		host := "0.0.0.0"
		reqBody := ConfigUpdateRequest{
			Section: "",
			Config: ConfigUpdateData{
				API: &APIConfigUpdate{
					Host: &host,
				},
			},
		}

		req := createTestRequest(t, http.MethodPut, "/api/v1/admin/config", reqBody)

		rr := executeRequest(t, handler.UpdateConfig, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("validates API section with invalid port", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		invalidPort := -1
		reqBody := ConfigUpdateRequest{
			Section: "api",
			Config: ConfigUpdateData{
				API: &APIConfigUpdate{
					Port: &invalidPort,
				},
			},
		}

		req := createTestRequest(t, http.MethodPut, "/api/v1/admin/config", reqBody)

		rr := executeRequest(t, handler.UpdateConfig, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("validates database section with invalid port", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		invalidPort := 99999
		reqBody := ConfigUpdateRequest{
			Section: "database",
			Config: ConfigUpdateData{
				Database: &DatabaseConfigUpdate{
					Port: &invalidPort,
				},
			},
		}

		req := createTestRequest(t, http.MethodPut, "/api/v1/admin/config", reqBody)

		rr := executeRequest(t, handler.UpdateConfig, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("returns 501 for critical sections (not yet implemented)", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		criticalSections := []string{"database", "api"}

		for _, section := range criticalSections {
			t.Run(section, func(t *testing.T) {
				host := "localhost"
				var reqBody ConfigUpdateRequest

				switch section {
				case "api":
					reqBody = ConfigUpdateRequest{
						Section: section,
						Config: ConfigUpdateData{
							API: &APIConfigUpdate{
								Host: &host,
							},
						},
					}
				case "database":
					reqBody = ConfigUpdateRequest{
						Section: section,
						Config: ConfigUpdateData{
							Database: &DatabaseConfigUpdate{
								Host: &host,
							},
						},
					}
				}

				req := createTestRequest(t, http.MethodPut, "/api/v1/admin/config", reqBody)

				rr := executeRequest(t, handler.UpdateConfig, req)

				// Validation passes but persistence is not yet implemented.
				assertNotImplementedResponse(t, rr)
			})
		}
	})

	t.Run("validates scanning section then returns 501", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		workerPoolSize := 10
		reqBody := ConfigUpdateRequest{
			Section: "scanning",
			Config: ConfigUpdateData{
				Scanning: &ScanningConfigUpdate{
					WorkerPoolSize: &workerPoolSize,
				},
			},
		}

		req := createTestRequest(t, http.MethodPut, "/api/v1/admin/config", reqBody)

		rr := executeRequest(t, handler.UpdateConfig, req)

		// Validation passes; persistence not yet implemented.
		assertNotImplementedResponse(t, rr)
	})

	t.Run("validates logging section then returns 501", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		level := "info"
		reqBody := ConfigUpdateRequest{
			Section: "logging",
			Config: ConfigUpdateData{
				Logging: &LoggingConfigUpdate{
					Level: &level,
				},
			},
		}

		req := createTestRequest(t, http.MethodPut, "/api/v1/admin/config", reqBody)

		rr := executeRequest(t, handler.UpdateConfig, req)

		// Validation passes; persistence not yet implemented.
		assertNotImplementedResponse(t, rr)
	})

	t.Run("validates daemon section then returns 501", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		pidFile := "/var/run/scanorama.pid"
		reqBody := ConfigUpdateRequest{
			Section: "daemon",
			Config: ConfigUpdateData{
				Daemon: &DaemonConfigUpdate{
					PIDFile: &pidFile,
				},
			},
		}

		req := createTestRequest(t, http.MethodPut, "/api/v1/admin/config", reqBody)

		rr := executeRequest(t, handler.UpdateConfig, req)

		// Validation passes; persistence not yet implemented.
		assertNotImplementedResponse(t, rr)
	})

	t.Run("validates scanning section with invalid worker pool size", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		invalidSize := 0
		reqBody := ConfigUpdateRequest{
			Section: "scanning",
			Config: ConfigUpdateData{
				Scanning: &ScanningConfigUpdate{
					WorkerPoolSize: &invalidSize,
				},
			},
		}

		req := createTestRequest(t, http.MethodPut, "/api/v1/admin/config", reqBody)

		rr := executeRequest(t, handler.UpdateConfig, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("validates logging section with invalid level", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		invalidLevel := "invalid"
		reqBody := ConfigUpdateRequest{
			Section: "logging",
			Config: ConfigUpdateData{
				Logging: &LoggingConfigUpdate{
					Level: &invalidLevel,
				},
			},
		}

		req := createTestRequest(t, http.MethodPut, "/api/v1/admin/config", reqBody)

		rr := executeRequest(t, handler.UpdateConfig, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

// TestGetLogs tests log retrieval endpoint.
func TestGetLogs(t *testing.T) {
	t.Run("returns 200 with empty data when ring buffer not wired", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs", nil)

		rr := executeRequest(t, handler.GetLogs, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("returns 200 with all filter combinations", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		paths := []string{
			"/api/v1/admin/logs?level=error",
			"/api/v1/admin/logs?component=scanner",
			"/api/v1/admin/logs?search=timeout",
			"/api/v1/admin/logs?page=1&page_size=50",
			"/api/v1/admin/logs?level=error&component=scanner&search=timeout",
		}
		for _, path := range paths {
			req := createTestRequest(t, http.MethodGet, path, nil)
			rr := executeRequest(t, handler.GetLogs, req)
			assert.Equal(t, http.StatusOK, rr.Code, "path: %s", path)
		}
	})

	t.Run("returns 400 on bad pagination", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs?page=invalid", nil)

		rr := executeRequest(t, handler.GetLogs, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("ignores invalid time format in since/until", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs?since=invalid-time", nil)

		rr := executeRequest(t, handler.GetLogs, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("returns 200 with time range filters", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		since := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		until := time.Now().Format(time.RFC3339)

		path := fmt.Sprintf("/api/v1/admin/logs?since=%s&until=%s", since, until)
		req := createTestRequest(t, http.MethodGet, path, nil)

		rr := executeRequest(t, handler.GetLogs, req)

		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("returns log entries from ring buffer", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		rb := logging.NewRingBuffer(10)
		rb.Append(logging.LogEntry{
			Time:    time.Now(),
			Level:   "info",
			Message: "test message",
		})
		handler = handler.WithRingBuffer(rb)

		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs", nil)
		rr := executeRequest(t, handler.GetLogs, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var resp struct {
			Data []logging.LogEntry `json:"data"`
		}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Len(t, resp.Data, 1)
		assert.Equal(t, "test message", resp.Data[0].Message)
	})

	t.Run("level filter hides entries below minimum level", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		rb := logging.NewRingBuffer(10)
		rb.Append(logging.LogEntry{Time: time.Now(), Level: "debug", Message: "debug msg"})
		rb.Append(logging.LogEntry{Time: time.Now(), Level: "info", Message: "info msg"})
		rb.Append(logging.LogEntry{Time: time.Now(), Level: "error", Message: "error msg"})
		handler = handler.WithRingBuffer(rb)

		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs?level=error", nil)
		rr := executeRequest(t, handler.GetLogs, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var resp struct {
			Data []logging.LogEntry `json:"data"`
		}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Len(t, resp.Data, 1)
		assert.Equal(t, "error msg", resp.Data[0].Message)
	})

	t.Run("pagination metadata is correct", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		rb := logging.NewRingBuffer(10)
		for i := 0; i < 5; i++ {
			rb.Append(logging.LogEntry{Time: time.Now(), Level: "info", Message: "msg"})
		}
		handler = handler.WithRingBuffer(rb)

		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs?page=1&page_size=2", nil)
		rr := executeRequest(t, handler.GetLogs, req)

		assert.Equal(t, http.StatusOK, rr.Code)

		var resp LogsResponse
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Len(t, resp.Data, 2)
		assert.Equal(t, 5, resp.Pagination.TotalItems)
		assert.Equal(t, 3, resp.Pagination.TotalPages)
	})
}

// TestExtractWorkerID tests worker ID extraction from URL
func TestExtractWorkerID(t *testing.T) {
	t.Run("extracts valid worker ID", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers/worker-123/stop", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "worker-123"})

		workerID, err := handler.extractWorkerID(req)

		assert.NoError(t, err)
		assert.Equal(t, "worker-123", workerID)
	})

	t.Run("returns error when ID not in path", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers/stop", nil)
		// No mux vars set

		workerID, err := handler.extractWorkerID(req)

		assert.Error(t, err)
		assert.Empty(t, workerID)
		assert.Contains(t, err.Error(), "worker ID not provided")
	})

	t.Run("handles empty worker ID", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers//stop", nil)
		req = mux.SetURLVars(req, map[string]string{"id": ""})

		workerID, err := handler.extractWorkerID(req)

		assert.Error(t, err)
		assert.Empty(t, workerID)
		assert.Contains(t, err.Error(), "worker ID cannot be empty")
	})
}

// TestValidateConfigUpdate tests configuration validation
func TestValidateConfigUpdate(t *testing.T) {
	handler := createTestAdminHandler(t)

	t.Run("accepts valid API config", func(t *testing.T) {
		host := "0.0.0.0"
		port := 8080
		req := &ConfigUpdateRequest{
			Section: "api",
			Config: ConfigUpdateData{
				API: &APIConfigUpdate{
					Host: &host,
					Port: &port,
				},
			},
		}

		err := handler.validateConfigUpdate(req)
		assert.NoError(t, err)
	})

	t.Run("rejects empty section", func(t *testing.T) {
		host := "localhost"
		req := &ConfigUpdateRequest{
			Section: "",
			Config: ConfigUpdateData{
				API: &APIConfigUpdate{
					Host: &host,
				},
			},
		}

		err := handler.validateConfigUpdate(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Section")
	})

	t.Run("rejects empty config", func(t *testing.T) {
		req := &ConfigUpdateRequest{
			Section: "api",
			Config:  ConfigUpdateData{},
		}

		err := handler.validateConfigUpdate(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "config")
	})

	t.Run("rejects invalid section name", func(t *testing.T) {
		host := "localhost"
		req := &ConfigUpdateRequest{
			Section: "invalid_section",
			Config: ConfigUpdateData{
				API: &APIConfigUpdate{
					Host: &host,
				},
			},
		}

		err := handler.validateConfigUpdate(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Section")
	})

	t.Run("accepts valid scanning config", func(t *testing.T) {
		workerPoolSize := 10
		req := &ConfigUpdateRequest{
			Section: "scanning",
			Config: ConfigUpdateData{
				Scanning: &ScanningConfigUpdate{
					WorkerPoolSize: &workerPoolSize,
				},
			},
		}

		err := handler.validateConfigUpdate(req)
		assert.NoError(t, err)
	})

	t.Run("accepts valid logging config", func(t *testing.T) {
		level := "info"
		req := &ConfigUpdateRequest{
			Section: "logging",
			Config: ConfigUpdateData{
				Logging: &LoggingConfigUpdate{
					Level: &level,
				},
			},
		}

		err := handler.validateConfigUpdate(req)
		assert.NoError(t, err)
	})

	t.Run("accepts valid daemon config", func(t *testing.T) {
		pidFile := "/var/run/scanorama.pid"
		req := &ConfigUpdateRequest{
			Section: "daemon",
			Config: ConfigUpdateData{
				Daemon: &DaemonConfigUpdate{
					PIDFile: &pidFile,
				},
			},
		}

		err := handler.validateConfigUpdate(req)
		assert.NoError(t, err)
	})
}

// TestValidateDatabaseConfig tests database configuration validation
func TestValidateDatabaseConfig(t *testing.T) {
	handler := createTestAdminHandler(t)

	t.Run("validates database host", func(t *testing.T) {
		host := "localhost"
		config := &DatabaseConfigUpdate{
			Host: &host,
		}

		err := handler.validateDatabaseConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates database port", func(t *testing.T) {
		port := 5432
		config := &DatabaseConfigUpdate{
			Port: &port,
		}

		err := handler.validateDatabaseConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates database name", func(t *testing.T) {
		dbName := "scanorama"
		config := &DatabaseConfigUpdate{
			Database: &dbName,
		}

		err := handler.validateDatabaseConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates username", func(t *testing.T) {
		username := "dbuser"
		config := &DatabaseConfigUpdate{
			Username: &username,
		}

		err := handler.validateDatabaseConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates connection max lifetime", func(t *testing.T) {
		lifetime := "1h"
		config := &DatabaseConfigUpdate{
			ConnMaxLifetime: &lifetime,
		}

		err := handler.validateDatabaseConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates connection max idle time", func(t *testing.T) {
		idleTime := "30m"
		config := &DatabaseConfigUpdate{
			ConnMaxIdleTime: &idleTime,
		}

		err := handler.validateDatabaseConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates all database fields together", func(t *testing.T) {
		host := "localhost"
		port := 5432
		dbName := "scanorama"
		username := "dbuser"
		lifetime := "1h"
		idleTime := "30m"

		config := &DatabaseConfigUpdate{
			Host:            &host,
			Port:            &port,
			Database:        &dbName,
			Username:        &username,
			ConnMaxLifetime: &lifetime,
			ConnMaxIdleTime: &idleTime,
		}

		err := handler.validateDatabaseConfig(config)
		assert.NoError(t, err)
	})

	t.Run("rejects invalid duration for conn max lifetime", func(t *testing.T) {
		invalidDuration := "invalid"
		config := &DatabaseConfigUpdate{
			ConnMaxLifetime: &invalidDuration,
		}

		err := handler.validateDatabaseConfig(config)
		assert.Error(t, err)
	})

	t.Run("rejects invalid duration for conn max idle time", func(t *testing.T) {
		invalidDuration := "not-a-duration"
		config := &DatabaseConfigUpdate{
			ConnMaxIdleTime: &invalidDuration,
		}

		err := handler.validateDatabaseConfig(config)
		assert.Error(t, err)
	})
}

// TestValidateScanningConfig tests scanning configuration validation
func TestValidateScanningConfig(t *testing.T) {
	handler := createTestAdminHandler(t)

	t.Run("validates default interval", func(t *testing.T) {
		interval := "5m"
		config := &ScanningConfigUpdate{
			DefaultInterval: &interval,
		}

		err := handler.validateScanningConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates max scan timeout", func(t *testing.T) {
		timeout := "30m"
		config := &ScanningConfigUpdate{
			MaxScanTimeout: &timeout,
		}

		err := handler.validateScanningConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates default ports", func(t *testing.T) {
		ports := "22,80,443,8080"
		config := &ScanningConfigUpdate{
			DefaultPorts: &ports,
		}

		err := handler.validateScanningConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates all scanning fields together", func(t *testing.T) {
		interval := "5m"
		timeout := "30m"
		ports := "22,80,443"
		workerPoolSize := 10
		enableServiceDetection := true

		config := &ScanningConfigUpdate{
			DefaultInterval:        &interval,
			MaxScanTimeout:         &timeout,
			DefaultPorts:           &ports,
			WorkerPoolSize:         &workerPoolSize,
			EnableServiceDetection: &enableServiceDetection,
		}

		err := handler.validateScanningConfig(config)
		assert.NoError(t, err)
	})

	t.Run("rejects invalid default interval", func(t *testing.T) {
		invalidInterval := "not-a-duration"
		config := &ScanningConfigUpdate{
			DefaultInterval: &invalidInterval,
		}

		err := handler.validateScanningConfig(config)
		assert.Error(t, err)
	})

	t.Run("rejects invalid max scan timeout", func(t *testing.T) {
		invalidTimeout := "invalid-timeout"
		config := &ScanningConfigUpdate{
			MaxScanTimeout: &invalidTimeout,
		}

		err := handler.validateScanningConfig(config)
		assert.Error(t, err)
	})
}

// TestValidateLoggingConfig tests logging configuration validation
func TestValidateLoggingConfig(t *testing.T) {
	handler := createTestAdminHandler(t)

	t.Run("validates output path", func(t *testing.T) {
		output := "/var/log/scanorama.log"
		config := &LoggingConfigUpdate{
			Output: &output,
		}

		err := handler.validateLoggingConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates stdout output", func(t *testing.T) {
		output := "stdout"
		config := &LoggingConfigUpdate{
			Output: &output,
		}

		err := handler.validateLoggingConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates all logging fields together", func(t *testing.T) {
		output := "/var/log/scanorama.log"
		level := "info"
		format := "json"
		structured := true

		config := &LoggingConfigUpdate{
			Output:     &output,
			Level:      &level,
			Format:     &format,
			Structured: &structured,
		}

		err := handler.validateLoggingConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates empty config", func(t *testing.T) {
		config := &LoggingConfigUpdate{}

		err := handler.validateLoggingConfig(config)
		assert.NoError(t, err)
	})
}

// TestValidateDaemonConfig tests daemon configuration validation
func TestValidateDaemonConfig(t *testing.T) {
	handler := createTestAdminHandler(t)

	t.Run("validates PID file path", func(t *testing.T) {
		pidFile := "/var/run/scanorama.pid"
		config := &DaemonConfigUpdate{
			PIDFile: &pidFile,
		}

		err := handler.validateDaemonConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates work directory", func(t *testing.T) {
		workDir := "/var/lib/scanorama"
		config := &DaemonConfigUpdate{
			WorkDir: &workDir,
		}

		err := handler.validateDaemonConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates user", func(t *testing.T) {
		user := "scanorama"
		config := &DaemonConfigUpdate{
			User: &user,
		}

		err := handler.validateDaemonConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates group", func(t *testing.T) {
		group := "scanorama"
		config := &DaemonConfigUpdate{
			Group: &group,
		}

		err := handler.validateDaemonConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates shutdown timeout", func(t *testing.T) {
		timeout := "30s"
		config := &DaemonConfigUpdate{
			ShutdownTimeout: &timeout,
		}

		err := handler.validateDaemonConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates all daemon fields together", func(t *testing.T) {
		pidFile := "/var/run/scanorama.pid"
		workDir := "/var/lib/scanorama"
		user := "scanorama"
		group := "scanorama"
		timeout := "30s"
		daemonize := true

		config := &DaemonConfigUpdate{
			PIDFile:         &pidFile,
			WorkDir:         &workDir,
			User:            &user,
			Group:           &group,
			ShutdownTimeout: &timeout,
			Daemonize:       &daemonize,
		}

		err := handler.validateDaemonConfig(config)
		assert.NoError(t, err)
	})

	t.Run("rejects invalid shutdown timeout", func(t *testing.T) {
		invalidTimeout := "not-a-duration"
		config := &DaemonConfigUpdate{
			ShutdownTimeout: &invalidTimeout,
		}

		err := handler.validateDaemonConfig(config)
		assert.Error(t, err)
	})
}

// TestValidateAPITimeoutSettings tests API timeout validation
func TestValidateAPITimeoutSettings(t *testing.T) {
	handler := createTestAdminHandler(t)

	t.Run("validates read timeout", func(t *testing.T) {
		readTimeout := "30s"
		config := &APIConfigUpdate{
			ReadTimeout: &readTimeout,
		}

		err := handler.validateAPIConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates write timeout", func(t *testing.T) {
		writeTimeout := "30s"
		config := &APIConfigUpdate{
			WriteTimeout: &writeTimeout,
		}

		err := handler.validateAPIConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates idle timeout", func(t *testing.T) {
		idleTimeout := "2m"
		config := &APIConfigUpdate{
			IdleTimeout: &idleTimeout,
		}

		err := handler.validateAPIConfig(config)
		assert.NoError(t, err)
	})

	t.Run("validates request timeout", func(t *testing.T) {
		requestTimeout := "60s"
		config := &APIConfigUpdate{
			RequestTimeout: &requestTimeout,
		}

		err := handler.validateAPIConfig(config)
		assert.NoError(t, err)
	})

	t.Run("rejects invalid read timeout", func(t *testing.T) {
		invalidTimeout := "not-valid"
		config := &APIConfigUpdate{
			ReadTimeout: &invalidTimeout,
		}

		err := handler.validateAPIConfig(config)
		assert.Error(t, err)
	})

	t.Run("rejects invalid write timeout", func(t *testing.T) {
		invalidTimeout := "invalid"
		config := &APIConfigUpdate{
			WriteTimeout: &invalidTimeout,
		}

		err := handler.validateAPIConfig(config)
		assert.Error(t, err)
	})

	t.Run("rejects invalid idle timeout", func(t *testing.T) {
		invalidTimeout := "bad-duration"
		config := &APIConfigUpdate{
			IdleTimeout: &invalidTimeout,
		}

		err := handler.validateAPIConfig(config)
		assert.Error(t, err)
	})

	t.Run("rejects invalid request timeout", func(t *testing.T) {
		invalidTimeout := "nope"
		config := &APIConfigUpdate{
			RequestTimeout: &invalidTimeout,
		}

		err := handler.validateAPIConfig(config)
		assert.Error(t, err)
	})
}

// TestValidateConfigSections tests section validation functions
func TestValidateConfigSections(t *testing.T) {
	handler := createTestAdminHandler(t)

	t.Run("database section rejects nil config", func(t *testing.T) {
		err := handler.validateDatabaseSection(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database configuration data is required")
	})

	t.Run("scanning section rejects nil config", func(t *testing.T) {
		err := handler.validateScanningSection(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "scanning configuration data is required")
	})

	t.Run("logging section rejects nil config", func(t *testing.T) {
		err := handler.validateLoggingSection(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "logging configuration data is required")
	})

	t.Run("daemon section rejects nil config", func(t *testing.T) {
		err := handler.validateDaemonSection(nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "daemon configuration data is required")
	})

	t.Run("database section validates valid config", func(t *testing.T) {
		host := "localhost"
		config := &DatabaseConfigUpdate{
			Host: &host,
		}

		err := handler.validateDatabaseSection(config)
		assert.NoError(t, err)
	})

	t.Run("scanning section validates valid config", func(t *testing.T) {
		workerPoolSize := 10
		config := &ScanningConfigUpdate{
			WorkerPoolSize: &workerPoolSize,
		}

		err := handler.validateScanningSection(config)
		assert.NoError(t, err)
	})

	t.Run("logging section validates valid config", func(t *testing.T) {
		level := "info"
		config := &LoggingConfigUpdate{
			Level: &level,
		}

		err := handler.validateLoggingSection(config)
		assert.NoError(t, err)
	})

	t.Run("daemon section validates valid config", func(t *testing.T) {
		pidFile := "/var/run/scanorama.pid"
		config := &DaemonConfigUpdate{
			PIDFile: &pidFile,
		}

		err := handler.validateDaemonSection(config)
		assert.NoError(t, err)
	})
}
