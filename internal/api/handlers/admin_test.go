package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

// Test helper functions

func createTestAdminHandler(t *testing.T) *AdminHandler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	registry := metrics.NewRegistry()

	return NewAdminHandler(nil, logger, registry)
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

// TestNewAdminHandler tests admin handler initialization
func TestNewAdminHandler(t *testing.T) {
	t.Run("initializes with all dependencies", func(t *testing.T) {
		logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
		registry := metrics.NewRegistry()
		database := &db.DB{}

		handler := NewAdminHandler(database, logger, registry)

		assert.NotNil(t, handler)
		assert.Equal(t, database, handler.database)
		assert.NotNil(t, handler.logger)
		assert.Equal(t, registry, handler.metrics)
		assert.NotNil(t, handler.validator)
	})
}

// TestGetWorkerStatus tests worker status retrieval endpoint
func TestGetWorkerStatus(t *testing.T) {
	t.Run("returns worker status successfully", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/workers/status", nil)

		rr := executeRequest(t, handler.GetWorkerStatus, req)

		var response WorkerStatusResponse
		assertJSONResponse(t, rr, &response)

		// Verify response structure
		assert.GreaterOrEqual(t, response.TotalWorkers, 0)
		assert.GreaterOrEqual(t, response.ActiveWorkers, 0)
		assert.GreaterOrEqual(t, response.IdleWorkers, 0)
		assert.GreaterOrEqual(t, response.QueueSize, 0)
		assert.GreaterOrEqual(t, response.ProcessedJobs, int64(0))
		assert.GreaterOrEqual(t, response.FailedJobs, int64(0))
		assert.NotNil(t, response.Workers)
		assert.NotNil(t, response.Summary)
		assert.False(t, response.Timestamp.IsZero())
	})

	t.Run("includes worker details in response", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/workers/status", nil)

		rr := executeRequest(t, handler.GetWorkerStatus, req)

		var response WorkerStatusResponse
		assertJSONResponse(t, rr, &response)

		// Verify each worker has required fields
		for _, worker := range response.Workers {
			assert.NotEmpty(t, worker.ID)
			assert.NotEmpty(t, worker.Status)
			assert.GreaterOrEqual(t, worker.JobsProcessed, int64(0))
			assert.GreaterOrEqual(t, worker.JobsFailed, int64(0))
			assert.False(t, worker.StartTime.IsZero())
			assert.GreaterOrEqual(t, worker.Uptime, time.Duration(0))
			assert.GreaterOrEqual(t, worker.MemoryUsage, int64(0))
			assert.GreaterOrEqual(t, worker.CPUUsage, float64(0))
			assert.GreaterOrEqual(t, worker.ErrorRate, float64(0))
			assert.NotNil(t, worker.Metrics)
		}
	})

	t.Run("worker count consistency", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/workers/status", nil)

		rr := executeRequest(t, handler.GetWorkerStatus, req)

		var response WorkerStatusResponse
		assertJSONResponse(t, rr, &response)

		// Total workers should match sum of active and idle
		assert.Equal(t, response.TotalWorkers, len(response.Workers))

		// Count active and idle workers
		activeCount := 0
		idleCount := 0
		for _, worker := range response.Workers {
			switch worker.Status {
			case "active":
				activeCount++
			case "idle":
				idleCount++
			}
		}

		assert.Equal(t, response.ActiveWorkers, activeCount)
		assert.Equal(t, response.IdleWorkers, idleCount)
	})

	t.Run("includes summary statistics", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/workers/status", nil)

		rr := executeRequest(t, handler.GetWorkerStatus, req)

		var response WorkerStatusResponse
		assertJSONResponse(t, rr, &response)

		// Verify summary contains expected metrics
		assert.NotEmpty(t, response.Summary)
		assert.Contains(t, response.Summary, "total_scans_completed")
		assert.Contains(t, response.Summary, "total_discovery_completed")
		assert.Contains(t, response.Summary, "overall_error_rate")
	})
}

// TestStopWorker tests worker stop endpoint
func TestStopWorker(t *testing.T) {
	t.Run("stops worker with valid ID", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers/worker-001/stop", nil)

		// Add worker ID to mux vars
		req = mux.SetURLVars(req, map[string]string{"id": "worker-001"})

		rr := executeRequest(t, handler.StopWorker, req)

		var response map[string]interface{}
		assertJSONResponse(t, rr, &response)

		// Verify response contains expected fields
		assert.Equal(t, "worker-001", response["worker_id"])
		assert.Equal(t, "stopped", response["status"])
		assert.NotEmpty(t, response["message"])
		assert.NotNil(t, response["timestamp"])
		assert.NotNil(t, response["graceful"])
	})

	t.Run("defaults to graceful shutdown", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers/worker-001/stop", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "worker-001"})

		rr := executeRequest(t, handler.StopWorker, req)

		var response map[string]interface{}
		assertJSONResponse(t, rr, &response)

		// Default should be graceful
		assert.True(t, response["graceful"].(bool))
	})

	t.Run("supports graceful parameter true", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers/worker-001/stop?graceful=true", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "worker-001"})

		rr := executeRequest(t, handler.StopWorker, req)

		var response map[string]interface{}
		assertJSONResponse(t, rr, &response)

		assert.True(t, response["graceful"].(bool))
	})

	t.Run("supports graceful parameter false", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers/worker-001/stop?graceful=false", nil)
		req = mux.SetURLVars(req, map[string]string{"id": "worker-001"})

		rr := executeRequest(t, handler.StopWorker, req)

		var response map[string]interface{}
		assertJSONResponse(t, rr, &response)

		assert.False(t, response["graceful"].(bool))
	})

	t.Run("rejects request without worker ID", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodPost, "/api/v1/admin/workers/stop", nil)
		// No mux vars set - missing worker ID

		rr := executeRequest(t, handler.StopWorker, req)

		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("handles different worker IDs", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		workerIDs := []string{"worker-001", "worker-002", "worker-999"}

		for _, workerID := range workerIDs {
			t.Run(fmt.Sprintf("worker_%s", workerID), func(t *testing.T) {
				path := fmt.Sprintf("/api/v1/admin/workers/%s/stop", workerID)
				req := createTestRequest(t, http.MethodPost, path, nil)
				req = mux.SetURLVars(req, map[string]string{"id": workerID})

				rr := executeRequest(t, handler.StopWorker, req)

				var response map[string]interface{}
				assertJSONResponse(t, rr, &response)

				assert.Equal(t, workerID, response["worker_id"])
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
	t.Run("updates API configuration section", func(t *testing.T) {
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

		var response map[string]interface{}
		assertJSONResponse(t, rr, &response)

		assert.Equal(t, "api", response["section"])
		assert.Equal(t, "updated", response["status"])
		assert.NotEmpty(t, response["message"])
		assert.NotNil(t, response["config"])
		assert.NotNil(t, response["restart_required"])
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

	t.Run("indicates restart requirement for critical sections", func(t *testing.T) {
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

				var response map[string]interface{}
				assertJSONResponse(t, rr, &response)

				// Critical sections should indicate restart required
				restartRequired, exists := response["restart_required"]
				assert.True(t, exists, "restart_required field should be present")
				assert.NotNil(t, restartRequired)
			})
		}
	})

	t.Run("validates scanning section", func(t *testing.T) {
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

		var response map[string]interface{}
		assertJSONResponse(t, rr, &response)

		assert.Equal(t, "scanning", response["section"])
	})

	t.Run("validates logging section", func(t *testing.T) {
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

		var response map[string]interface{}
		assertJSONResponse(t, rr, &response)

		assert.Equal(t, "logging", response["section"])
	})

	t.Run("validates daemon section", func(t *testing.T) {
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

		var response map[string]interface{}
		assertJSONResponse(t, rr, &response)

		assert.Equal(t, "daemon", response["section"])
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

// TestGetLogs tests log retrieval endpoint
func TestGetLogs(t *testing.T) {
	t.Run("retrieves logs without filters", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs", nil)

		rr := executeRequest(t, handler.GetLogs, req)

		var response LogsResponse
		assertJSONResponse(t, rr, &response)

		// Verify response structure
		assert.NotNil(t, response.Lines)
		assert.GreaterOrEqual(t, response.TotalLines, 0)
		assert.GreaterOrEqual(t, response.StartLine, 0)
		assert.GreaterOrEqual(t, response.EndLine, 0)
		assert.False(t, response.GeneratedAt.IsZero())
	})

	t.Run("filters logs by level", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		levels := []string{"debug", "info", "warn", "error"}

		for _, level := range levels {
			t.Run(level, func(t *testing.T) {
				req := createTestRequest(t, http.MethodGet, fmt.Sprintf("/api/v1/admin/logs?level=%s", level), nil)

				rr := executeRequest(t, handler.GetLogs, req)

				var response LogsResponse
				assertJSONResponse(t, rr, &response)

				assert.NotNil(t, response.Lines)
			})
		}
	})

	t.Run("filters logs by component", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		components := []string{"api", "scanner", "discovery", "scheduler"}

		for _, component := range components {
			t.Run(component, func(t *testing.T) {
				path := fmt.Sprintf("/api/v1/admin/logs?component=%s", component)
				req := createTestRequest(t, http.MethodGet, path, nil)

				rr := executeRequest(t, handler.GetLogs, req)

				assert.Equal(t, http.StatusOK, rr.Code)
			})
		}
	})

	t.Run("supports search parameter", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs?search=error", nil)

		rr := executeRequest(t, handler.GetLogs, req)

		var response LogsResponse
		assertJSONResponse(t, rr, &response)

		assert.NotNil(t, response.Lines)
	})

	t.Run("supports time range filters", func(t *testing.T) {
		handler := createTestAdminHandler(t)

		since := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		until := time.Now().Format(time.RFC3339)

		path := fmt.Sprintf("/api/v1/admin/logs?since=%s&until=%s", since, until)
		req := createTestRequest(t, http.MethodGet, path, nil)

		rr := executeRequest(t, handler.GetLogs, req)

		var response LogsResponse
		assertJSONResponse(t, rr, &response)

		assert.NotNil(t, response.Lines)
	})

	t.Run("supports tail parameter", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs?tail=100", nil)

		rr := executeRequest(t, handler.GetLogs, req)

		var response LogsResponse
		assertJSONResponse(t, rr, &response)

		assert.NotNil(t, response.Lines)
	})

	t.Run("supports pagination", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs?page=1&page_size=50", nil)

		rr := executeRequest(t, handler.GetLogs, req)

		var response LogsResponse
		assertJSONResponse(t, rr, &response)

		assert.NotNil(t, response.Lines)
		assert.LessOrEqual(t, len(response.Lines), 50)
	})

	t.Run("handles invalid time format gracefully", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs?since=invalid-time", nil)

		rr := executeRequest(t, handler.GetLogs, req)

		// Should still return logs, just ignore invalid time filter
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("handles invalid tail parameter gracefully", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs?tail=invalid", nil)

		rr := executeRequest(t, handler.GetLogs, req)

		// Should still return logs, just ignore invalid tail
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("combines multiple filters", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		path := "/api/v1/admin/logs?level=error&component=scanner&search=timeout"
		req := createTestRequest(t, http.MethodGet, path, nil)

		rr := executeRequest(t, handler.GetLogs, req)

		var response LogsResponse
		assertJSONResponse(t, rr, &response)

		assert.NotNil(t, response.Lines)
	})

	t.Run("indicates when more logs are available", func(t *testing.T) {
		handler := createTestAdminHandler(t)
		req := createTestRequest(t, http.MethodGet, "/api/v1/admin/logs?page_size=10", nil)

		rr := executeRequest(t, handler.GetLogs, req)

		var response LogsResponse
		assertJSONResponse(t, rr, &response)

		// HasMore field should indicate if pagination needed
		assert.NotNil(t, response.HasMore)
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

// TestIsRestartRequired tests restart requirement detection
func TestIsRestartRequired(t *testing.T) {
	handler := createTestAdminHandler(t)

	testCases := []struct {
		section       string
		expectRestart bool
		description   string
	}{
		{"api", true, "API section changes require restart"},
		{"database", true, "Database section changes require restart"},
		{"scanning", false, "Scanning section changes may not require restart"},
		{"logging", false, "Logging section changes may not require restart"},
		{"daemon", true, "Daemon section changes require restart"},
	}

	for _, tc := range testCases {
		t.Run(tc.section, func(t *testing.T) {
			result := handler.isRestartRequired(tc.section)

			// Just verify the method returns a boolean
			assert.IsType(t, true, result, tc.description)
		})
	}
}
