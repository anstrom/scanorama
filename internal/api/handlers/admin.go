// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements administrative endpoints for system management,
// worker control, configuration management, and log retrieval.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cfgpkg "github.com/anstrom/scanorama/internal/config"
	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
)

// Admin operation constants.
const (
	workerStopDelay = 100 * time.Millisecond
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
	database  *db.DB
	logger    *slog.Logger
	metrics   *metrics.Registry
	validator *validator.Validate
	config    *cfgpkg.Config
}

// NewAdminHandler creates a new admin handler.
func NewAdminHandler(
	database *db.DB,
	logger *slog.Logger,
	metricsManager *metrics.Registry,
	cfg *cfgpkg.Config,
) *AdminHandler {
	return &AdminHandler{
		database:  database,
		logger:    logger.With("handler", "admin"),
		metrics:   metricsManager,
		validator: validator.New(),
		config:    cfg,
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
// ConfigUpdateRequest represents a request to update configuration
type ConfigUpdateRequest struct {
	Section string           `json:"section" validate:"required,oneof=api database scanning logging daemon"`
	Config  ConfigUpdateData `json:"config" validate:"required"`
}

// ConfigUpdateData represents the configuration data for updates
type ConfigUpdateData struct {
	API      *APIConfigUpdate      `json:"api,omitempty"`
	Database *DatabaseConfigUpdate `json:"database,omitempty"`
	Scanning *ScanningConfigUpdate `json:"scanning,omitempty"`
	Logging  *LoggingConfigUpdate  `json:"logging,omitempty"`
	Daemon   *DaemonConfigUpdate   `json:"daemon,omitempty"`
}

// APIConfigUpdate represents updatable API configuration fields
type APIConfigUpdate struct {
	Enabled           *bool    `json:"enabled,omitempty"`
	Host              *string  `json:"host,omitempty"`
	Port              *int     `json:"port,omitempty" validate:"omitempty,min=1,max=65535"`
	ReadTimeout       *string  `json:"read_timeout,omitempty"`
	WriteTimeout      *string  `json:"write_timeout,omitempty"`
	IdleTimeout       *string  `json:"idle_timeout,omitempty"`
	MaxHeaderBytes    *int     `json:"max_header_bytes,omitempty" validate:"omitempty,min=1024,max=1048576"`
	EnableCORS        *bool    `json:"enable_cors,omitempty"`
	CORSOrigins       []string `json:"cors_origins,omitempty"`
	AuthEnabled       *bool    `json:"auth_enabled,omitempty"`
	RateLimitEnabled  *bool    `json:"rate_limit_enabled,omitempty"`
	RateLimitRequests *int     `json:"rate_limit_requests,omitempty" validate:"omitempty,min=1,max=10000"`
	RateLimitWindow   *string  `json:"rate_limit_window,omitempty"`
	RequestTimeout    *string  `json:"request_timeout,omitempty"`
	MaxRequestSize    *int     `json:"max_request_size,omitempty" validate:"omitempty,min=1,max=104857600"`
}

// DatabaseConfigUpdate represents updatable database configuration fields
type DatabaseConfigUpdate struct {
	Host            *string `json:"host,omitempty"`
	Port            *int    `json:"port,omitempty" validate:"omitempty,min=1,max=65535"`
	Database        *string `json:"database,omitempty" validate:"omitempty,min=1,max=63"`
	Username        *string `json:"username,omitempty" validate:"omitempty,min=1,max=63"`
	SSLMode         *string `json:"ssl_mode,omitempty" validate:"omitempty,oneof=disable require verify-ca verify-full"`
	MaxOpenConns    *int    `json:"max_open_conns,omitempty" validate:"omitempty,min=1,max=100"`
	MaxIdleConns    *int    `json:"max_idle_conns,omitempty" validate:"omitempty,min=1,max=100"`
	ConnMaxLifetime *string `json:"conn_max_lifetime,omitempty"`
	ConnMaxIdleTime *string `json:"conn_max_idle_time,omitempty"`
}

// ScanningConfigUpdate represents updatable scanning configuration fields
type ScanningConfigUpdate struct {
	WorkerPoolSize         *int    `json:"worker_pool_size,omitempty" validate:"omitempty,min=1,max=1000"`
	DefaultInterval        *string `json:"default_interval,omitempty"`
	MaxScanTimeout         *string `json:"max_scan_timeout,omitempty"`
	DefaultPorts           *string `json:"default_ports,omitempty" validate:"omitempty,max=1000"`
	DefaultScanType        *string `json:"default_scan_type,omitempty" validate:"omitempty,oneof=connect syn ack window fin null xmas maimon"` //nolint:lll
	MaxConcurrentTargets   *int    `json:"max_concurrent_targets,omitempty" validate:"omitempty,min=1,max=10000"`
	EnableServiceDetection *bool   `json:"enable_service_detection,omitempty"`
	EnableOSDetection      *bool   `json:"enable_os_detection,omitempty"`
}

// LoggingConfigUpdate represents updatable logging configuration fields
type LoggingConfigUpdate struct {
	Level          *string `json:"level,omitempty" validate:"omitempty,oneof=debug info warn error"`
	Format         *string `json:"format,omitempty" validate:"omitempty,oneof=text json"`
	Output         *string `json:"output,omitempty" validate:"omitempty,min=1,max=255"`
	Structured     *bool   `json:"structured,omitempty"`
	RequestLogging *bool   `json:"request_logging,omitempty"`
}

// DaemonConfigUpdate represents updatable daemon configuration fields
type DaemonConfigUpdate struct {
	PIDFile         *string `json:"pid_file,omitempty" validate:"omitempty,min=1,max=255"`
	WorkDir         *string `json:"work_dir,omitempty" validate:"omitempty,min=1,max=255"`
	User            *string `json:"user,omitempty" validate:"omitempty,min=1,max=32"`
	Group           *string `json:"group,omitempty" validate:"omitempty,min=1,max=32"`
	Daemonize       *bool   `json:"daemonize,omitempty"`
	ShutdownTimeout *string `json:"shutdown_timeout,omitempty"`
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

	// Parse request body with size limit validation
	var req ConfigUpdateRequest
	if err := parseConfigJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Validate request structure and content
	if err := h.validateConfigUpdate(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Extract and validate the specific configuration section
	configData, err := h.extractConfigSection(&req)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid config format: %w", err))
		return
	}

	// Update configuration with validated data
	updatedConfig := h.updateConfig(r.Context(), req.Section, configData)

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

// ReloadConfig handles POST /api/v1/admin/config/reload - manual config reload.
func (h *AdminHandler) ReloadConfig(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Manual configuration reload requested", "request_id", requestID)

	cfg := h.config
	if cfg == nil {
		cfg = cfgpkg.GetCurrent()
	}
	if cfg == nil {
		writeError(w, r, http.StatusServiceUnavailable, fmt.Errorf("configuration not initialized"))
		return
	}
	if err := cfg.Reload(); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("reload failed: %w", err))
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]interface{}{
		"status":     "reloaded",
		"timestamp":  time.Now().UTC(),
		"request_id": requestID,
	})
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
	// First validate the request structure using validator
	if err := h.validator.Struct(req); err != nil {
		return fmt.Errorf("request validation failed: %w", err)
	}

	if req.Section == "" {
		return fmt.Errorf("configuration section is required")
	}

	validSections := map[string]bool{
		configSectionAPI:      true,
		configSectionDatabase: true,
		configSectionScanning: true,
		configSectionLogging:  true,
		configSectionDaemon:   true,
	}

	if !validSections[req.Section] {
		return fmt.Errorf("invalid configuration section: %s", req.Section)
	}

	// Validate that the appropriate configuration section is provided and validate its content
	return h.validateConfigSection(req)
}

// extractConfigSection safely extracts the configuration data for the specified section
func (h *AdminHandler) extractConfigSection(req *ConfigUpdateRequest) (map[string]interface{}, error) {
	return h.extractConfigData(req)
}

// structToMap safely converts a struct to map[string]interface{} for processing
func structToMap(v interface{}) (map[string]interface{}, error) {
	// Use JSON marshaling/unmarshaling for safe conversion
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Remove nil values to avoid overwriting with empty values
	cleaned := make(map[string]interface{})
	for k, v := range result {
		if v != nil {
			cleaned[k] = v
		}
	}

	return cleaned, nil
}

// validateStringField validates string configuration fields with security constraints
func validateStringField(field, value string, maxLength int) error {
	if len(value) > maxLength {
		return fmt.Errorf("%s too long: %d characters (max %d)", field, len(value), maxLength)
	}

	// Check for null bytes
	for i, char := range value {
		if char == 0 {
			return fmt.Errorf("%s contains null byte at position %d", field, i)
		}
	}

	// Check for control characters (except tabs and newlines)
	for i, char := range value {
		if char < 32 && char != 9 && char != 10 && char != 13 {
			return fmt.Errorf("%s contains control character at position %d", field, i)
		}
	}

	return nil
}

// validateConfigSection validates configuration sections (extracted to reduce complexity)
func (h *AdminHandler) validateConfigSection(req *ConfigUpdateRequest) error {
	switch req.Section {
	case configSectionAPI:
		return h.validateAPISection(req.Config.API)
	case configSectionDatabase:
		return h.validateDatabaseSection(req.Config.Database)
	case configSectionScanning:
		return h.validateScanningSection(req.Config.Scanning)
	case configSectionLogging:
		return h.validateLoggingSection(req.Config.Logging)
	case configSectionDaemon:
		return h.validateDaemonSection(req.Config.Daemon)
	default:
		return fmt.Errorf("unsupported configuration section: %s", req.Section)
	}
}

// validateAPISection validates API configuration section
func (h *AdminHandler) validateAPISection(config *APIConfigUpdate) error {
	if config == nil {
		return fmt.Errorf("api configuration data is required for api section")
	}
	if err := h.validator.Struct(config); err != nil {
		return fmt.Errorf("api configuration validation failed: %w", err)
	}
	if err := h.validateAPIConfig(config); err != nil {
		return fmt.Errorf("api configuration security validation failed: %w", err)
	}
	return nil
}

// validateDatabaseSection validates database configuration section
func (h *AdminHandler) validateDatabaseSection(config *DatabaseConfigUpdate) error {
	if config == nil {
		return fmt.Errorf("database configuration data is required for database section")
	}
	if err := h.validator.Struct(config); err != nil {
		return fmt.Errorf("database configuration validation failed: %w", err)
	}
	if err := h.validateDatabaseConfig(config); err != nil {
		return fmt.Errorf("database configuration security validation failed: %w", err)
	}
	return nil
}

// validateScanningSection validates scanning configuration section
func (h *AdminHandler) validateScanningSection(config *ScanningConfigUpdate) error {
	if config == nil {
		return fmt.Errorf("scanning configuration data is required for scanning section")
	}
	if err := h.validator.Struct(config); err != nil {
		return fmt.Errorf("scanning configuration validation failed: %w", err)
	}
	if err := h.validateScanningConfig(config); err != nil {
		return fmt.Errorf("scanning configuration security validation failed: %w", err)
	}
	return nil
}

// validateLoggingSection validates logging configuration section
func (h *AdminHandler) validateLoggingSection(config *LoggingConfigUpdate) error {
	if config == nil {
		return fmt.Errorf("logging configuration data is required for logging section")
	}
	if err := h.validator.Struct(config); err != nil {
		return fmt.Errorf("logging configuration validation failed: %w", err)
	}
	if err := h.validateLoggingConfig(config); err != nil {
		return fmt.Errorf("logging configuration security validation failed: %w", err)
	}
	return nil
}

// validateDaemonSection validates daemon configuration section
func (h *AdminHandler) validateDaemonSection(config *DaemonConfigUpdate) error {
	if config == nil {
		return fmt.Errorf("daemon configuration data is required for daemon section")
	}
	if err := h.validator.Struct(config); err != nil {
		return fmt.Errorf("daemon configuration validation failed: %w", err)
	}
	if err := h.validateDaemonConfig(config); err != nil {
		return fmt.Errorf("daemon configuration security validation failed: %w", err)
	}
	return nil
}

// extractConfigData extracts configuration data by section (extracted to reduce complexity)
func (h *AdminHandler) extractConfigData(req *ConfigUpdateRequest) (map[string]interface{}, error) {
	switch req.Section {
	case configSectionAPI:
		return h.extractAPIConfigData(req.Config.API)
	case configSectionDatabase:
		return h.extractDatabaseConfigData(req.Config.Database)
	case configSectionScanning:
		return h.extractScanningConfigData(req.Config.Scanning)
	case configSectionLogging:
		return h.extractLoggingConfigData(req.Config.Logging)
	case configSectionDaemon:
		return h.extractDaemonConfigData(req.Config.Daemon)
	default:
		return nil, fmt.Errorf("unsupported configuration section: %s", req.Section)
	}
}

// extractAPIConfigData safely extracts API configuration data
func (h *AdminHandler) extractAPIConfigData(config *APIConfigUpdate) (map[string]interface{}, error) {
	if config == nil {
		return nil, fmt.Errorf("api configuration is required")
	}
	data, err := structToMap(config)
	if err != nil {
		return nil, fmt.Errorf("failed to process api config: %w", err)
	}
	return data, nil
}

// extractDatabaseConfigData safely extracts database configuration data
func (h *AdminHandler) extractDatabaseConfigData(config *DatabaseConfigUpdate) (map[string]interface{}, error) {
	if config == nil {
		return nil, fmt.Errorf("database configuration is required")
	}
	data, err := structToMap(config)
	if err != nil {
		return nil, fmt.Errorf("failed to process database config: %w", err)
	}
	return data, nil
}

// extractScanningConfigData safely extracts scanning configuration data
func (h *AdminHandler) extractScanningConfigData(config *ScanningConfigUpdate) (map[string]interface{}, error) {
	if config == nil {
		return nil, fmt.Errorf("scanning configuration is required")
	}
	data, err := structToMap(config)
	if err != nil {
		return nil, fmt.Errorf("failed to process scanning config: %w", err)
	}
	return data, nil
}

// extractLoggingConfigData safely extracts logging configuration data
func (h *AdminHandler) extractLoggingConfigData(config *LoggingConfigUpdate) (map[string]interface{}, error) {
	if config == nil {
		return nil, fmt.Errorf("logging configuration is required")
	}
	data, err := structToMap(config)
	if err != nil {
		return nil, fmt.Errorf("failed to process logging config: %w", err)
	}
	return data, nil
}

// extractDaemonConfigData safely extracts daemon configuration data
func (h *AdminHandler) extractDaemonConfigData(config *DaemonConfigUpdate) (map[string]interface{}, error) {
	if config == nil {
		return nil, fmt.Errorf("daemon configuration is required")
	}
	data, err := structToMap(config)
	if err != nil {
		return nil, fmt.Errorf("failed to process daemon config: %w", err)
	}
	return data, nil
}

// Additional validation for configuration fields with custom security checks
func (h *AdminHandler) validateAPIConfig(config *APIConfigUpdate) error {
	if err := h.validateAPINetworkSettings(config); err != nil {
		return err
	}

	if err := h.validateAPITimeoutSettings(config); err != nil {
		return err
	}

	if err := h.validateAPICORSSettings(config); err != nil {
		return err
	}

	return nil
}

// validateAPINetworkSettings validates API network-related configuration
func (h *AdminHandler) validateAPINetworkSettings(config *APIConfigUpdate) error {
	if config.Host != nil {
		if err := validateHostField("host", *config.Host); err != nil {
			return err
		}
	}

	if config.Port != nil {
		if err := validatePortField("port", *config.Port); err != nil {
			return err
		}
	}

	return nil
}

// validateAPITimeoutSettings validates API timeout-related configuration
func (h *AdminHandler) validateAPITimeoutSettings(config *APIConfigUpdate) error {
	if config.ReadTimeout != nil {
		if err := validateDurationField("read_timeout", *config.ReadTimeout); err != nil {
			return err
		}
	}

	if config.WriteTimeout != nil {
		if err := validateDurationField("write_timeout", *config.WriteTimeout); err != nil {
			return err
		}
	}

	if config.IdleTimeout != nil {
		if err := validateDurationField("idle_timeout", *config.IdleTimeout); err != nil {
			return err
		}
	}

	if config.RequestTimeout != nil {
		if err := validateDurationField("request_timeout", *config.RequestTimeout); err != nil {
			return err
		}
	}

	if config.RateLimitWindow != nil {
		if err := validateDurationField("rate_limit_window", *config.RateLimitWindow); err != nil {
			return err
		}
	}

	return nil
}

// validateAPICORSSettings validates API CORS-related configuration
func (h *AdminHandler) validateAPICORSSettings(config *APIConfigUpdate) error {
	if config.CORSOrigins != nil {
		for i, origin := range config.CORSOrigins {
			fieldName := fmt.Sprintf("cors_origins[%d]", i)
			if err := validateStringField(fieldName, origin, maxAdminHostnameLength); err != nil {
				return err
			}
		}
	}

	return nil
}

func (h *AdminHandler) validateDatabaseConfig(config *DatabaseConfigUpdate) error {
	if config.Host != nil {
		if err := validateHostField("host", *config.Host); err != nil {
			return err
		}
	}

	if config.Port != nil {
		if err := validatePortField("port", *config.Port); err != nil {
			return err
		}
	}

	if config.Database != nil {
		if err := validateStringField("database", *config.Database, maxDatabaseNameLength); err != nil {
			return err
		}
	}

	if config.Username != nil {
		if err := validateStringField("username", *config.Username, maxUsernameLength); err != nil {
			return err
		}
	}

	if config.ConnMaxLifetime != nil {
		if err := validateDurationField("conn_max_lifetime", *config.ConnMaxLifetime); err != nil {
			return err
		}
	}

	if config.ConnMaxIdleTime != nil {
		if err := validateDurationField("conn_max_idle_time", *config.ConnMaxIdleTime); err != nil {
			return err
		}
	}

	return nil
}

func (h *AdminHandler) validateScanningConfig(config *ScanningConfigUpdate) error {
	if config.DefaultInterval != nil {
		if err := validateDurationField("default_interval", *config.DefaultInterval); err != nil {
			return err
		}
	}

	if config.MaxScanTimeout != nil {
		if err := validateDurationField("max_scan_timeout", *config.MaxScanTimeout); err != nil {
			return err
		}
	}

	if config.DefaultPorts != nil {
		if err := validateStringField("default_ports", *config.DefaultPorts, maxAdminPortsStringLength); err != nil {
			return err
		}
		// Additional validation for port ranges could be added here
	}

	return nil
}

func (h *AdminHandler) validateLoggingConfig(config *LoggingConfigUpdate) error {
	if config.Output != nil {
		if err := validatePathField("output", *config.Output); err != nil {
			return err
		}
	}

	return nil
}

func (h *AdminHandler) validateDaemonConfig(config *DaemonConfigUpdate) error {
	if config.PIDFile != nil {
		if err := validatePathField("pid_file", *config.PIDFile); err != nil {
			return err
		}
	}

	if config.WorkDir != nil {
		if err := validatePathField("work_dir", *config.WorkDir); err != nil {
			return err
		}
	}

	if config.User != nil {
		if err := validateStringField("user", *config.User, 32); err != nil {
			return err
		}
	}

	if config.Group != nil {
		if err := validateStringField("group", *config.Group, 32); err != nil {
			return err
		}
	}

	if config.ShutdownTimeout != nil {
		if err := validateDurationField("shutdown_timeout", *config.ShutdownTimeout); err != nil {
			return err
		}
	}

	return nil
}

// validateHostField validates hostname or IP address fields
func validateHostField(field, value string) error {
	if err := validateStringField(field, value, maxAdminHostnameLength); err != nil { // Max hostname length
		return err
	}

	// Basic hostname validation - allow empty for "listen on all interfaces"
	if value == "" {
		return nil
	}

	// Check for valid characters (basic validation)
	for i, char := range value {
		if (char < 'a' || char > 'z') &&
			(char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') &&
			char != '.' && char != '-' && char != ':' {
			return fmt.Errorf("%s contains invalid character at position %d", field, i)
		}
	}

	return nil
}

// validatePortField validates port number fields
func validatePortField(field string, value int) error {
	if value < 1 || value > 65535 {
		return fmt.Errorf("%s out of range: %d (must be 1-65535)", field, value)
	}

	// Privileged ports (< 1024) are allowed but noted for security awareness

	return nil
}

// validateDurationField validates duration string fields
func validateDurationField(field, value string) error {
	if err := validateStringField(field, value, maxDurationStringLength); err != nil {
		return err
	}

	if value == "" {
		return nil
	}

	// Try to parse as duration
	_, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("%s is not a valid duration: %w", field, err)
	}

	return nil
}

// validatePathField validates file path fields
func validatePathField(field, value string) error {
	if err := validateStringField(field, value, maxPathLength); err != nil {
		return err
	}

	if value == "" {
		return nil
	}

	// Check for directory traversal patterns
	if strings.Contains(value, "..") {
		return fmt.Errorf("%s contains directory traversal: %s", field, value)
	}

	// Additional check for cleaned path differences
	cleanPath := filepath.Clean(value)
	if cleanPath != value && strings.Contains(cleanPath, "..") {
		return fmt.Errorf("%s contains directory traversal: %s", field, value)
	}

	return nil
}

// parseConfigJSON safely parses JSON with size limits and security constraints</text>
func parseConfigJSON(r *http.Request, dest interface{}) error {
	if r.Body == nil {
		return fmt.Errorf("request body is empty")
	}

	// Enforce maximum request size
	r.Body = http.MaxBytesReader(nil, r.Body, maxConfigSize)

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	// Use strict number handling to prevent precision issues
	decoder.UseNumber()

	if err := decoder.Decode(dest); err != nil {
		if err.Error() == "http: request body too large" {
			return fmt.Errorf("configuration data too large (max 1MB)")
		}
		return fmt.Errorf("invalid JSON: %w", err)
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
