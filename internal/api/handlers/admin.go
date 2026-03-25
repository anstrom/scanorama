// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements the AdminHandler type and its HTTP handler methods
// for configuration endpoints.
package handlers

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/anstrom/scanorama/internal/logging"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/internal/scanning"
	"github.com/go-playground/validator/v10"
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
	logger     *slog.Logger
	metrics    *metrics.Registry
	validator  *validator.Validate
	ringBuffer *logging.RingBuffer
	scanQueue  *scanning.ScanQueue
}

// NewAdminHandler creates a new admin handler.
func NewAdminHandler(logger *slog.Logger, metricsManager *metrics.Registry) *AdminHandler {
	return &AdminHandler{
		logger:    logger.With("handler", "admin"),
		metrics:   metricsManager,
		validator: validator.New(),
	}
}

// WithRingBuffer sets the ring buffer used by log-related endpoints and
// returns the handler for method chaining.
func (h *AdminHandler) WithRingBuffer(rb *logging.RingBuffer) *AdminHandler {
	h.ringBuffer = rb
	return h
}

// WithScanQueue sets the scan queue used by worker-status endpoints and
// returns the handler for method chaining.
func (h *AdminHandler) WithScanQueue(q *scanning.ScanQueue) *AdminHandler {
	h.scanQueue = q
	return h
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
