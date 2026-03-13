// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements the AdminHandler type and its HTTP handler methods
// for worker management, configuration, and log retrieval endpoints.
package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
)

// Configuration section constants.
const (
	configSectionAPI      = "api"
	configSectionDatabase = "database"
	configSectionScanning = "scanning"
	configSectionLogging  = "logging"
	configSectionDaemon   = "daemon"
)

// Validation limit constants.
const (
	maxDatabaseNameLength     = 63          // PostgreSQL maximum database name length
	maxUsernameLength         = 63          // PostgreSQL maximum username length
	maxAdminPortsStringLength = 1000        // Maximum length for admin ports configuration string
	maxDurationStringLength   = 50          // Maximum length for duration strings
	maxPathLength             = 4096        // Maximum file path length
	maxConfigSize             = 1024 * 1024 // Maximum configuration size (1MB)
	maxAdminHostnameLength    = 255         // Maximum hostname length for admin config
)

// AdminHandler handles administrative API endpoints.
type AdminHandler struct {
	logger    *slog.Logger
	metrics   *metrics.Registry
	validator *validator.Validate
}

// NewAdminHandler creates a new admin handler.
func NewAdminHandler(logger *slog.Logger, metricsManager *metrics.Registry) *AdminHandler {
	return &AdminHandler{
		logger:    logger.With("handler", "admin"),
		metrics:   metricsManager,
		validator: validator.New(),
	}
}

// GetWorkerStatus handles GET /api/v1/admin/workers - get worker pool status.
func (h *AdminHandler) GetWorkerStatus(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Getting worker status", "request_id", requestID)

	// Get worker status from database/worker manager
	// For now, return mock data until worker management is implemented
	workers := []WorkerInfo{
		{
			ID:            "worker-001",
			Status:        "active",
			JobsProcessed: 42,
			JobsFailed:    2,
			StartTime:     time.Now().Add(-2 * time.Hour),
			Uptime:        2 * time.Hour,
			MemoryUsage:   1024 * 1024 * 50, // 50MB
			CPUUsage:      15.5,
			ErrorRate:     0.047,
			Metrics: map[string]int{
				"scans_completed":     35,
				"discovery_completed": 7,
				"errors":              2,
			},
		},
		{
			ID:            "worker-002",
			Status:        "idle",
			JobsProcessed: 28,
			JobsFailed:    1,
			StartTime:     time.Now().Add(-1 * time.Hour),
			Uptime:        1 * time.Hour,
			MemoryUsage:   1024 * 1024 * 32, // 32MB
			CPUUsage:      5.2,
			ErrorRate:     0.036,
			Metrics: map[string]int{
				"scans_completed":     25,
				"discovery_completed": 3,
				"errors":              1,
			},
		},
	}

	response := WorkerStatusResponse{
		TotalWorkers:   len(workers),
		ActiveWorkers:  1,
		IdleWorkers:    1,
		QueueSize:      0,
		ProcessedJobs:  70,
		FailedJobs:     3,
		AvgJobDuration: 5 * time.Minute,
		Workers:        workers,
		Summary: map[string]interface{}{
			"total_scans_completed":     60,
			"total_discovery_completed": 10,
			"overall_error_rate":        0.043,
			"queue_throughput_per_hour": 35,
		},
		Timestamp: time.Now().UTC(),
	}

	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_admin_worker_status_total", nil)
	}
}

// StopWorker handles POST /api/v1/admin/workers/{id}/stop - stop a specific worker.
func (h *AdminHandler) StopWorker(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())

	// Extract and validate worker ID from URL — still returns 400 on missing ID.
	workerID, err := h.extractWorkerID(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	h.logger.Info("StopWorker called but not yet implemented",
		"request_id", requestID,
		"worker_id", workerID)

	// Worker management is not yet implemented.
	writeError(w, r, http.StatusNotImplemented,
		fmt.Errorf("stop worker is not yet implemented"))
}

// GetConfig handles GET /api/v1/admin/config - get current configuration.
func (h *AdminHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Getting configuration", "request_id", requestID)

	// Get configuration sections
	section := r.URL.Query().Get("section")

	// Get full config or specific section
	config, err := h.getCurrentConfig(r.Context(), section)
	if err != nil {
		h.logger.Error("Failed to get configuration", "request_id", requestID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to retrieve configuration: %w", err))
		return
	}

	writeJSON(w, r, http.StatusOK, config)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_admin_config_retrieved_total", map[string]string{
			"section": section,
		})
	}
}

// UpdateConfig handles PUT /api/v1/admin/config - update configuration.
func (h *AdminHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Updating configuration", "request_id", requestID)

	// Parse request body with size limit validation — still returns 400 on bad input.
	var req ConfigUpdateRequest
	if err := parseConfigJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Validate request structure and content — still returns 400 on bad input.
	if err := h.validateConfigUpdate(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Extract and validate the specific configuration section — still returns 400 on bad input.
	if _, err := h.extractConfigSection(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid config format: %w", err))
		return
	}

	h.logger.Info("UpdateConfig called but not yet implemented",
		"request_id", requestID,
		"section", req.Section)

	// Configuration persistence is not yet implemented.
	writeError(w, r, http.StatusNotImplemented,
		fmt.Errorf("update config is not yet implemented"))
}

// GetLogs handles GET /api/v1/admin/logs - retrieve system logs.
func (h *AdminHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Getting logs", "request_id", requestID)

	// Parse pagination parameters — still returns 400 on bad input.
	if _, err := getPaginationParams(r); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	h.logger.Info("GetLogs called but not yet implemented", "request_id", requestID)

	// Log retrieval is not yet implemented.
	writeError(w, r, http.StatusNotImplemented,
		fmt.Errorf("get logs is not yet implemented"))
}

// extractWorkerID extracts the worker ID from the URL path.
func (h *AdminHandler) extractWorkerID(r *http.Request) (string, error) {
	vars := mux.Vars(r)
	workerID, exists := vars["id"]
	if !exists {
		return "", fmt.Errorf("worker ID not provided")
	}

	if workerID == "" {
		return "", fmt.Errorf("worker ID cannot be empty")
	}

	return workerID, nil
}
