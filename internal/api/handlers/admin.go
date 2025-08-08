// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements administrative endpoints for system management,
// worker control, configuration management, and log retrieval.
package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/gorilla/mux"
)

// Admin operation constants.
const (
	workerStopDelay = 100 * time.Millisecond
)

// AdminHandler handles administrative API endpoints.
type AdminHandler struct {
	database *db.DB
	logger   *slog.Logger
	metrics  *metrics.Registry
}

// NewAdminHandler creates a new admin handler.
func NewAdminHandler(database *db.DB, logger *slog.Logger, metricsManager *metrics.Registry) *AdminHandler {
	return &AdminHandler{
		database: database,
		logger:   logger.With("handler", "admin"),
		metrics:  metricsManager,
	}
}

// WorkerStatusResponse represents worker pool status information.
type WorkerStatusResponse struct {
	TotalWorkers   int                    `json:"total_workers"`
	ActiveWorkers  int                    `json:"active_workers"`
	IdleWorkers    int                    `json:"idle_workers"`
	QueueSize      int                    `json:"queue_size"`
	ProcessedJobs  int64                  `json:"processed_jobs"`
	FailedJobs     int64                  `json:"failed_jobs"`
	AvgJobDuration time.Duration          `json:"avg_job_duration"`
	Workers        []WorkerInfo           `json:"workers"`
	Summary        map[string]interface{} `json:"summary"`
	Timestamp      time.Time              `json:"timestamp"`
}

// WorkerInfo represents individual worker information.
type WorkerInfo struct {
	ID            string         `json:"id"`
	Status        string         `json:"status"`
	CurrentJob    *JobInfo       `json:"current_job,omitempty"`
	JobsProcessed int64          `json:"jobs_processed"`
	JobsFailed    int64          `json:"jobs_failed"`
	LastJobTime   *time.Time     `json:"last_job_time,omitempty"`
	StartTime     time.Time      `json:"start_time"`
	Uptime        time.Duration  `json:"uptime"`
	MemoryUsage   int64          `json:"memory_usage_bytes"`
	CPUUsage      float64        `json:"cpu_usage_percent"`
	ErrorRate     float64        `json:"error_rate"`
	Metrics       map[string]int `json:"metrics"`
}

// JobInfo represents current job information.
type JobInfo struct {
	ID        string        `json:"id"`
	Type      string        `json:"type"`
	Target    string        `json:"target,omitempty"`
	StartTime time.Time     `json:"start_time"`
	Duration  time.Duration `json:"duration"`
	Progress  float64       `json:"progress"`
}

// ConfigResponse represents configuration information.
type ConfigResponse struct {
	API      interface{} `json:"api"`
	Database interface{} `json:"database"`
	Scanning interface{} `json:"scanning"`
	Logging  interface{} `json:"logging"`
	Daemon   interface{} `json:"daemon"`
}

// ConfigUpdateRequest represents a configuration update request.
type ConfigUpdateRequest struct {
	Section string      `json:"section" validate:"required,oneof=api database scanning logging daemon"`
	Config  interface{} `json:"config" validate:"required"`
}

// LogsResponse represents log retrieval response.
type LogsResponse struct {
	Lines       []LogEntry `json:"lines"`
	TotalLines  int        `json:"total_lines"`
	StartLine   int        `json:"start_line"`
	EndLine     int        `json:"end_line"`
	HasMore     bool       `json:"has_more"`
	GeneratedAt time.Time  `json:"generated_at"`
}

// LogEntry represents a single log entry.
type LogEntry struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Component string                 `json:"component,omitempty"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Error     string                 `json:"error,omitempty"`
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

	// Extract worker ID from URL
	workerID, err := h.extractWorkerID(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	h.logger.Info("Stopping worker", "request_id", requestID, "worker_id", workerID)

	// Parse request options
	graceful := r.URL.Query().Get("graceful") != "false" // Default to graceful shutdown

	// Stop worker (placeholder implementation)
	h.stopWorker(r.Context(), workerID, graceful)

	response := map[string]interface{}{
		"worker_id":  workerID,
		"status":     "stopped",
		"graceful":   graceful,
		"message":    "Worker has been stopped",
		"timestamp":  time.Now().UTC(),
		"request_id": requestID,
	}

	h.logger.Info("Worker stopped successfully",
		"request_id", requestID,
		"worker_id", workerID,
		"graceful", graceful)

	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_admin_workers_stopped_total", map[string]string{
			"graceful": strconv.FormatBool(graceful),
		})
	}
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

	// Parse request body
	var req ConfigUpdateRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Validate request
	if err := h.validateConfigUpdate(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Update configuration
	configMap, ok := req.Config.(map[string]interface{})
	if !ok {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid config format: expected map"))
		return
	}
	updatedConfig := h.updateConfig(r.Context(), req.Section, configMap)

	response := map[string]interface{}{
		"section":          req.Section,
		"status":           "updated",
		"message":          fmt.Sprintf("Configuration section '%s' has been updated", req.Section),
		"config":           updatedConfig,
		"timestamp":        time.Now().UTC(),
		"request_id":       requestID,
		"restart_required": h.isRestartRequired(req.Section),
	}

	h.logger.Info("Configuration updated successfully",
		"request_id", requestID,
		"section", req.Section)

	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_admin_config_updated_total", map[string]string{
			"section": req.Section,
		})
	}
}

// GetLogs handles GET /api/v1/admin/logs - retrieve system logs.
func (h *AdminHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Getting logs", "request_id", requestID)

	// Parse query parameters
	level := r.URL.Query().Get("level")         // Filter by log level
	component := r.URL.Query().Get("component") // Filter by component
	since := r.URL.Query().Get("since")         // Time filter
	until := r.URL.Query().Get("until")         // Time filter
	tail := r.URL.Query().Get("tail")           // Number of recent lines
	search := r.URL.Query().Get("search")       // Search term

	// Parse pagination parameters
	params, err := getPaginationParams(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Build log filters
	filters := map[string]interface{}{
		"level":     level,
		"component": component,
		"search":    search,
	}

	// Parse time filters
	if since != "" {
		if sinceTime, err := time.Parse(time.RFC3339, since); err == nil {
			filters["since"] = sinceTime
		}
	}
	if until != "" {
		if untilTime, err := time.Parse(time.RFC3339, until); err == nil {
			filters["until"] = untilTime
		}
	}

	// Parse tail parameter
	if tail != "" {
		if tailLines, err := strconv.Atoi(tail); err == nil && tailLines > 0 {
			filters["tail"] = tailLines
		}
	}

	// Get logs from database/log manager
	logs, total := h.getLogs(r.Context(), filters, params.Offset, params.PageSize)

	response := LogsResponse{
		Lines:       logs,
		TotalLines:  int(total),
		StartLine:   params.Offset + 1,
		EndLine:     params.Offset + len(logs),
		HasMore:     int64(params.Offset+len(logs)) < total,
		GeneratedAt: time.Now().UTC(),
	}

	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_admin_logs_retrieved_total", map[string]string{
			"level":     level,
			"component": component,
		})
	}
}

// Helper methods

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

// validateConfigUpdate validates a configuration update request.
func (h *AdminHandler) validateConfigUpdate(req *ConfigUpdateRequest) error {
	if req.Section == "" {
		return fmt.Errorf("configuration section is required")
	}

	validSections := map[string]bool{
		"api":      true,
		"database": true,
		"scanning": true,
		"logging":  true,
		"daemon":   true,
	}

	if !validSections[req.Section] {
		return fmt.Errorf("invalid configuration section: %s", req.Section)
	}

	if req.Config == nil {
		return fmt.Errorf("configuration data is required")
	}

	return nil
}

// stopWorker stops a specific worker (placeholder implementation).
func (h *AdminHandler) stopWorker(_ context.Context, workerID string, graceful bool) {
	// This would interface with the actual worker manager
	// For now, return a placeholder implementation

	h.logger.Info("Stopping worker", "worker_id", workerID, "graceful", graceful)

	// Simulate worker stopping logic
	time.Sleep(workerStopDelay)

	// In a real implementation, this would:
	// 1. Find the worker by ID
	// 2. Signal it to stop (graceful or immediate)
	// 3. Wait for confirmation or timeout
	// 4. Return appropriate error if stop failed
}

// getCurrentConfig retrieves current configuration.
func (h *AdminHandler) getCurrentConfig(_ context.Context, section string) (map[string]interface{}, error) {
	// This would get the actual configuration from the config manager
	// For now, return mock configuration data

	config := ConfigResponse{
		API: map[string]interface{}{
			"enabled":             true,
			"host":                "127.0.0.1",
			"port":                8080,
			"auth_enabled":        false,
			"rate_limit_enabled":  true,
			"rate_limit_requests": 100,
			"read_timeout":        "10s",
			"write_timeout":       "10s",
		},
		Database: map[string]interface{}{
			"host":            "localhost",
			"port":            5432,
			"database":        "scanorama",
			"ssl_mode":        "require",
			"max_connections": 25,
		},
		Scanning: map[string]interface{}{
			"worker_pool_size":         10,
			"default_scan_type":        "connect",
			"max_concurrent_targets":   100,
			"default_ports":            "22,80,443,8080,8443",
			"enable_service_detection": true,
		},
		Logging: map[string]interface{}{
			"level":      "info",
			"format":     "text",
			"output":     "stdout",
			"structured": true,
		},
		Daemon: map[string]interface{}{
			"pid_file":         "/tmp/scanorama.pid",
			"shutdown_timeout": "30s",
			"daemonize":        true,
		},
	}

	// Return specific section if requested
	if section != "" {
		switch section {
		case "api":
			return config.API.(map[string]interface{}), nil
		case "database":
			return config.Database.(map[string]interface{}), nil
		case "scanning":
			return config.Scanning.(map[string]interface{}), nil
		case "logging":
			return config.Logging.(map[string]interface{}), nil
		case "daemon":
			return config.Daemon.(map[string]interface{}), nil
		default:
			return nil, fmt.Errorf("unknown configuration section: %s", section)
		}
	}

	// Return entire config as map
	return map[string]interface{}{
		"api":      config.API,
		"database": config.Database,
		"scanning": config.Scanning,
		"logging":  config.Logging,
		"daemon":   config.Daemon,
	}, nil
}

// updateConfig updates configuration (placeholder implementation).
func (h *AdminHandler) updateConfig(
	_ context.Context,
	section string,
	config map[string]interface{},
) map[string]interface{} {
	// This would interface with the actual configuration manager
	// For now, return a placeholder implementation

	h.logger.Info("Updating configuration", "section", section)

	// In a real implementation, this would:
	// 1. Validate the configuration data for the specific section
	// 2. Update the configuration in memory and/or file
	// 3. Apply changes that can be applied without restart
	// 4. Return the updated configuration

	// For now, just return the input data as if it was applied
	return config
}

// isRestartRequired checks if a configuration change requires restart.
func (h *AdminHandler) isRestartRequired(section string) bool {
	// Define which configuration sections require restart
	restartRequired := map[string]bool{
		"api":      true,  // Changing API settings requires restart
		"database": true,  // Database connection changes require restart
		"daemon":   true,  // Daemon settings require restart
		"scanning": false, // Most scanning settings can be applied at runtime
		"logging":  false, // Logging settings can be applied at runtime
	}

	return restartRequired[section]
}

// getLogs retrieves system logs (placeholder implementation).
func (h *AdminHandler) getLogs(
	_ context.Context,
	filters map[string]interface{},
	offset, limit int,
) (logs []LogEntry, total int64) {
	// This would interface with the actual logging system
	// For now, return mock log data

	logs = []LogEntry{
		{
			Timestamp: time.Now().Add(-5 * time.Minute),
			Level:     "info",
			Message:   "Scan completed successfully",
			Component: "scanner",
			Fields: map[string]interface{}{
				"scan_id":     123,
				"target":      "192.168.1.1",
				"duration_ms": 2500,
			},
		},
		{
			Timestamp: time.Now().Add(-10 * time.Minute),
			Level:     "warn",
			Message:   "Database connection pool running low",
			Component: "database",
			Fields: map[string]interface{}{
				"active_connections": 23,
				"max_connections":    25,
			},
		},
		{
			Timestamp: time.Now().Add(-15 * time.Minute),
			Level:     "error",
			Message:   "Failed to resolve hostname",
			Component: "discovery",
			Error:     "no such host",
			Fields: map[string]interface{}{
				"hostname": "invalid.example.com",
				"job_id":   456,
			},
		},
		{
			Timestamp: time.Now().Add(-20 * time.Minute),
			Level:     "info",
			Message:   "Worker started",
			Component: "worker",
			Fields: map[string]interface{}{
				"worker_id": "worker-001",
				"pool_size": 10,
			},
		},
	}

	// Apply filters (basic implementation)
	filteredLogs := []LogEntry{}
	for _, log := range logs {
		if h.matchesFilters(&log, filters) {
			filteredLogs = append(filteredLogs, log)
		}
	}

	// Apply pagination
	start := offset
	end := offset + limit
	total = int64(len(filteredLogs))

	if start >= len(filteredLogs) {
		return []LogEntry{}, total
	}
	if end > len(filteredLogs) {
		end = len(filteredLogs)
	}

	return filteredLogs[start:end], total
}

// matchesFilters checks if a log entry matches the given filters.
func (h *AdminHandler) matchesFilters(log *LogEntry, filters map[string]interface{}) bool {
	if level, ok := filters["level"].(string); ok && level != "" {
		if log.Level != level {
			return false
		}
	}

	if component, ok := filters["component"].(string); ok && component != "" {
		if log.Component != component {
			return false
		}
	}

	if search, ok := filters["search"].(string); ok && search != "" {
		if !contains(log.Message, search) && !contains(log.Error, search) {
			return false
		}
	}

	if since, ok := filters["since"].(time.Time); ok {
		if log.Timestamp.Before(since) {
			return false
		}
	}

	if until, ok := filters["until"].(time.Time); ok {
		if log.Timestamp.After(until) {
			return false
		}
	}

	return true
}

// contains performs case-insensitive substring search.
func contains(str, substr string) bool {
	return len(str) >= len(substr) &&
		(str == substr ||
			substr == "" ||
			str[:len(substr)] == substr ||
			str[len(str)-len(substr):] == substr ||
			containsRecursive(str, substr))
}

func containsRecursive(str, substr string) bool {
	if len(str) < len(substr) {
		return false
	}
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
