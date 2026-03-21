// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements scan management endpoints including CRUD operations,
// scan execution control, and results retrieval.
package handlers

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/internal/scanning"
)

// Scan validation constants.
const (
	maxScanNameLength = 255
	maxTargetLength   = 255
	maxTargetCount    = 100
)

// scanRunnerFunc is the signature for the function that executes a scan.
// It matches scanning.RunScanWithContext and can be replaced in tests.
type scanRunnerFunc func(
	ctx context.Context, config *scanning.ScanConfig, database *db.DB,
) (*scanning.ScanResult, error)

// ScanHandler handles scan-related API endpoints.
type ScanHandler struct {
	database   ScanStore
	logger     *slog.Logger
	metrics    *metrics.Registry
	scanQueue  *scanning.ScanQueue
	scanRunner scanRunnerFunc
	scanMode   string
}

// NewScanHandler creates a new scan handler.
func NewScanHandler(database ScanStore, logger *slog.Logger, metricsManager *metrics.Registry) *ScanHandler {
	return &ScanHandler{
		database:   database,
		logger:     logger.With("handler", "scan"),
		metrics:    metricsManager,
		scanRunner: scanning.RunScanWithContext,
	}
}

// WithScanMode sets the fallback scan mode used when a scan record does not
// carry an explicit ScanType. The sentinel default "connect" is applied last
// so callers may pass an empty string safely.
func (h *ScanHandler) WithScanMode(mode string) *ScanHandler {
	h.scanMode = mode
	return h
}

// firstNonEmpty returns the first non-empty string from the provided values,
// or an empty string if all values are empty.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// SetScanQueue configures an optional scan execution queue. When set, StartScan
// submits work to the queue instead of spawning an unbounded goroutine.
func (h *ScanHandler) SetScanQueue(q *scanning.ScanQueue) {
	h.scanQueue = q
}

// ScanRequest represents a scan creation/update request.
type ScanRequest struct {
	Name        string            `json:"name" validate:"required,min=1,max=255"`
	Description string            `json:"description,omitempty"`
	Targets     []string          `json:"targets" validate:"required,min=1"`
	ScanType    string            `json:"scan_type" validate:"required,oneof=connect syn ack udp aggressive comprehensive"` //nolint:lll
	OSDetection bool              `json:"os_detection,omitempty"`
	Ports       string            `json:"ports,omitempty"`
	ProfileID   *string           `json:"profile_id,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
	ScheduleID  *int64            `json:"schedule_id,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
}

// ScanResponse represents a scan response.
type ScanResponse struct {
	ID           uuid.UUID         `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	Targets      []string          `json:"targets"`
	ScanType     string            `json:"scan_type"`
	Ports        string            `json:"ports,omitempty"`
	ProfileID    *string           `json:"profile_id,omitempty"`
	Options      map[string]string `json:"options,omitempty"`
	ScheduleID   *int64            `json:"schedule_id,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Status       string            `json:"status"`
	Progress     float64           `json:"progress"`
	StartTime    *time.Time        `json:"started_at,omitempty"`
	EndTime      *time.Time        `json:"completed_at,omitempty"`
	Duration     *string           `json:"duration,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	CreatedBy    string            `json:"created_by,omitempty"`
	ErrorMessage *string           `json:"error_message,omitempty"`
	PortsScanned *string           `json:"ports_scanned,omitempty"`
}

// ScanResultsResponse represents scan results.
type ScanResultsResponse struct {
	ScanID      uuid.UUID       `json:"scan_id"`
	TotalHosts  int             `json:"total_hosts"`
	TotalPorts  int             `json:"total_ports"`
	OpenPorts   int             `json:"open_ports"`
	ClosedPorts int             `json:"closed_ports"`
	Results     []ScanResult    `json:"results"`
	Summary     *db.ScanSummary `json:"summary"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// ScanResult represents an individual scan result.
type ScanResult struct {
	ID           uuid.UUID `json:"id"`
	HostIP       string    `json:"host_ip"`
	Hostname     string    `json:"hostname,omitempty"`
	Port         int       `json:"port"`
	Protocol     string    `json:"protocol"`
	State        string    `json:"state"`
	Service      string    `json:"service,omitempty"`
	Version      string    `json:"version,omitempty"`
	Banner       string    `json:"banner,omitempty"`
	ScanTime     time.Time `json:"scan_time"`
	OSName       string    `json:"os_name,omitempty"`
	OSFamily     string    `json:"os_family,omitempty"`
	OSVersion    string    `json:"os_version,omitempty"`
	OSConfidence *int      `json:"os_confidence,omitempty"`
}

// ListScans handles GET /api/v1/scans - list all scans with pagination.
// ListScans handles GET /api/v1/scans - list scans with filtering and pagination.
func (h *ScanHandler) ListScans(w http.ResponseWriter, r *http.Request) {
	listOp := &ListOperation[*db.Scan, db.ScanFilters]{
		EntityType: "scans",
		MetricName: "api_scans_listed_total",
		Logger:     h.logger,
		Metrics:    h.metrics,
		GetFilters: h.getScanFilters,
		ListFromDB: h.database.ListScans,
		ToResponse: func(scan *db.Scan) interface{} {
			return h.scanToResponse(scan)
		},
	}
	listOp.Execute(w, r)
}

// CreateScan handles POST /api/v1/scans - create a new scan.
func (h *ScanHandler) CreateScan(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Creating scan", "request_id", requestID)

	// Parse request body
	var req ScanRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Validate request
	if err := h.validateScanRequest(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Verify the referenced profile exists before inserting (avoids FK 500).
	if req.ProfileID != nil && *req.ProfileID != "" {
		if _, err := h.database.GetProfile(r.Context(), *req.ProfileID); err != nil {
			if errors.IsNotFound(err) {
				writeError(w, r, http.StatusBadRequest,
					fmt.Errorf("profile %q not found", *req.ProfileID))
				return
			}
			h.logger.Error("Failed to look up profile", "request_id", requestID,
				"profile_id", *req.ProfileID, "error", err)
			writeError(w, r, http.StatusInternalServerError,
				fmt.Errorf("failed to look up profile: %w", err))
			return
		}
	}

	// Create scan in database
	scan, err := h.database.CreateScan(r.Context(), h.requestToDBScan(&req))
	if err != nil {
		h.logger.Error("Failed to create scan", "request_id", requestID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to create scan: %w", err))
		return
	}

	response := h.scanToResponse(scan)

	h.logger.Info("Scan created successfully",
		"request_id", requestID,
		"scan_id", response.ID,
		"scan_name", response.Name)

	writeJSON(w, r, http.StatusCreated, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_scans_created_total", map[string]string{
			"scan_type": req.ScanType,
		})
	}
}

// GetScan handles GET /api/v1/scans/{id} - get a specific scan.
func (h *ScanHandler) GetScan(w http.ResponseWriter, r *http.Request) {
	scanID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	crudOp := &CRUDOperation[db.Scan]{
		EntityType: "scan",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	crudOp.ExecuteGet(w, r, scanID, h.database.GetScan,
		func(scan *db.Scan) interface{} {
			return h.scanToResponse(scan)
		}, "api_scans_retrieved_total")
}

// UpdateScan handles PUT /api/v1/scans/{id} - update a scan.
func (h *ScanHandler) UpdateScan(w http.ResponseWriter, r *http.Request) {
	UpdateEntity[db.Scan, ScanRequest](
		w, r,
		"scan",
		h.logger,
		h.metrics,
		func(r *http.Request) (interface{}, error) {
			var req ScanRequest
			if err := parseJSON(r, &req); err != nil {
				return nil, err
			}
			return h.requestToDBScan(&req), nil
		},
		h.database.UpdateScan,
		func(scan *db.Scan) interface{} {
			return h.scanToResponse(scan)
		},
		"api_scans_updated_total")
}

// DeleteScan handles DELETE /api/v1/scans/{id} - delete a scan.
func (h *ScanHandler) DeleteScan(w http.ResponseWriter, r *http.Request) {
	scanID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	crudOp := &CRUDOperation[db.Scan]{
		EntityType: "scan",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	crudOp.ExecuteDelete(w, r, scanID, h.database.DeleteScan, "api_scans_deleted_total")
}

// GetScanResults handles GET /api/v1/scans/{id}/results - get scan results.
func (h *ScanHandler) GetScanResults(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())

	// Extract scan ID from URL
	scanID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	h.logger.Info("Getting scan results", "request_id", requestID, "scan_id", scanID)

	// Parse pagination parameters
	params, err := getPaginationParams(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Verify the scan exists before querying results.
	if _, err := h.database.GetScan(r.Context(), scanID); err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound, fmt.Errorf("scan not found"))
			return
		}
		h.logger.Error("Failed to look up scan", "request_id", requestID, "scan_id", scanID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to look up scan: %w", err))
		return
	}

	// Get scan results from database
	results, _, err := h.database.GetScanResults(r.Context(), scanID, params.Offset, params.PageSize)
	if err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound, fmt.Errorf("scan not found"))
			return
		}
		h.logger.Error("Failed to get scan results", "request_id", requestID, "scan_id", scanID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to retrieve scan results: %w", err))
		return
	}

	// Get scan summary
	summary, err := h.database.GetScanSummary(r.Context(), scanID)
	if err != nil {
		h.logger.Warn("Failed to get scan summary", "request_id", requestID, "scan_id", scanID, "error", err)
		summary = &db.ScanSummary{
			ScanID:      scanID,
			TotalHosts:  0,
			TotalPorts:  0,
			OpenPorts:   0,
			ClosedPorts: 0,
			Duration:    0,
		}
	}

	// Convert to response format
	scanResults := make([]ScanResult, len(results))
	for i, result := range results {
		scanResults[i] = h.resultToResponse(result)
	}

	response := ScanResultsResponse{
		ScanID:      scanID,
		TotalHosts:  summary.TotalHosts,
		TotalPorts:  len(results),
		OpenPorts:   summary.OpenPorts,
		ClosedPorts: summary.ClosedPorts,
		Results:     scanResults,
		Summary:     summary,
		GeneratedAt: time.Now().UTC(),
	}

	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_scan_results_retrieved_total", map[string]string{
			"scan_id": scanID.String(),
		})
	}
}

// StartScan handles POST /api/v1/scans/{id}/start - start scan execution.
func (h *ScanHandler) StartScan(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	scanID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	h.logger.Info("Starting scan execution", "request_id", requestID, "scan_id", scanID)

	// Get the scan from database
	scan, err := h.database.GetScan(r.Context(), scanID)
	if err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound, fmt.Errorf("scan not found"))
			return
		}
		h.logger.Error("Failed to get scan", "request_id", requestID, "scan_id", scanID, "error", err)
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to get scan: %w", err))
		return
	}

	// Check if scan is already running
	if scan.Status == "running" {
		writeError(w, r, http.StatusConflict, fmt.Errorf("scan is already running"))
		return
	}

	// Check if scan is already completed
	if scan.Status == "completed" {
		writeError(w, r, http.StatusConflict, fmt.Errorf("scan is already completed"))
		return
	}

	// Update scan status to running
	err = h.database.StartScan(r.Context(), scanID)
	if err != nil {
		h.logger.Error("Failed to update scan status", "request_id", requestID, "scan_id", scanID, "error", err)
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to start scan: %w", err))
		return
	}

	// Convert scan to ScanConfig and execute asynchronously
	if h.scanQueue != nil {
		if code, qErr := h.submitToQueue(scanID, scan); qErr != nil {
			// Revert scan status since we couldn't queue it
			if stopErr := h.database.StopScan(r.Context(), scanID); stopErr != nil {
				h.logger.Error("Failed to revert scan status after queue rejection",
					"request_id", requestID, "scan_id", scanID, "error", stopErr)
			}
			writeError(w, r, code, qErr)
			return
		}
	} else {
		go h.executeScanAsync(scanID, scan)
	}

	// Return updated scan
	updatedScan, err := h.database.GetScan(r.Context(), scanID)
	if err != nil {
		h.logger.Error("Failed to get updated scan", "request_id", requestID, "scan_id", scanID, "error", err)
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to get updated scan: %w", err))
		return
	}

	response := h.scanToResponse(updatedScan)
	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_scans_started_total", map[string]string{
			"scan_type": scan.ScanType,
		})
	}

	h.logger.Info("Scan started successfully", "request_id", requestID, "scan_id", scanID)
}

// submitToQueue enqueues the scan for execution via the bounded scan queue.
// It returns the HTTP status code and error to write back, or 0/nil on success.
func (h *ScanHandler) submitToQueue(scanID uuid.UUID, scan *db.Scan) (int, error) {
	// Type-assert to *db.DB so the runner can persist hosts and ports.
	// If the store is a test double the cast yields nil, which RunScanWithContext
	// handles gracefully (it skips persistence when database is nil).
	concreteDB, _ := h.database.(*db.DB)

	scanConfig := &scanning.ScanConfig{
		Targets:     scan.Targets,
		Ports:       scan.Ports,
		ScanType:    firstNonEmpty(scan.ScanType, h.scanMode, "connect"),
		TimeoutSec:  300,
		OSDetection: getOptionBool(scan.Options, "os_detection"),
		ScanID:      &scanID,
	}

	resultCh := make(chan *scanning.ScanQueueResult, 1)
	req := &scanning.ScanQueueRequest{
		ID:       scanID.String(),
		Config:   scanConfig,
		Database: concreteDB,
		ResultCh: resultCh,
	}

	if err := h.scanQueue.Submit(req); err != nil {
		if stderrors.Is(err, scanning.ErrQueueFull) {
			h.logger.Warn("Scan queue is full, rejecting request", "scan_id", scanID)
			return http.StatusTooManyRequests, fmt.Errorf("scan queue is full, please try again later")
		}
		if stderrors.Is(err, scanning.ErrQueueClosed) {
			h.logger.Error("Scan queue is closed", "scan_id", scanID)
			return http.StatusServiceUnavailable, fmt.Errorf("scan queue is unavailable")
		}
		h.logger.Error("Failed to submit scan to queue", "scan_id", scanID, "error", err)
		return http.StatusInternalServerError, fmt.Errorf("failed to submit scan: %w", err)
	}

	// Listen for the result and update scan status when done.
	go func() {
		result := <-resultCh
		ctx := context.Background()
		if result.Error != nil {
			h.logger.Error("Queued scan execution failed",
				"scan_id", scanID, "error", result.Error)
			if stopErr := h.database.StopScan(ctx, scanID, result.Error.Error()); stopErr != nil {
				h.logger.Error("Failed to mark scan as stopped after queue execution",
					"scan_id", scanID, "error", stopErr)
			}
		} else {
			h.logger.Info("Queued scan execution completed",
				"scan_id", scanID, "duration", result.Duration)
			if completeErr := h.database.CompleteScan(ctx, scanID); completeErr != nil {
				h.logger.Error("Failed to mark scan as completed after queue execution",
					"scan_id", scanID, "error", completeErr)
			}
		}
	}()

	return 0, nil
}

// executeScanAsync executes a scan asynchronously and stores results
func (h *ScanHandler) executeScanAsync(scanID uuid.UUID, scan *db.Scan) {
	h.logger.Info("Starting async scan execution", "scan_id", scanID, "scan_name", scan.Name)

	scanType := firstNonEmpty(scan.ScanType, h.scanMode, "connect")
	scanConfig := &scanning.ScanConfig{
		Targets:     scan.Targets,
		Ports:       scan.Ports,
		ScanType:    scanType,
		TimeoutSec:  scanning.CalculateTimeout(scan.Ports, len(scan.Targets), scanType),
		OSDetection: getOptionBool(scan.Options, "os_detection"),
		ScanID:      &scanID,
	}

	// Type-assert to *db.DB so the runner can persist hosts and ports.
	// If the store is a test double the cast yields nil, which RunScanWithContext
	// handles gracefully (it skips persistence when database is nil).
	concreteDB, _ := h.database.(*db.DB)

	ctx := context.Background()
	result, err := h.scanRunner(ctx, scanConfig, concreteDB)

	if err != nil {
		h.logger.Error("Scan execution failed", "scan_id", scanID, "error", err)
		if stopErr := h.database.StopScan(ctx, scanID, err.Error()); stopErr != nil {
			h.logger.Error("Failed to mark scan as failed after execution error",
				"scan_id", scanID, "error", stopErr)
		}
	} else {
		h.logger.Info("Scan execution completed successfully", "scan_id", scanID,
			"hosts_scanned", len(result.Hosts), "duration", result.Duration)
		if completeErr := h.database.CompleteScan(ctx, scanID); completeErr != nil {
			h.logger.Error("Failed to mark scan as completed",
				"scan_id", scanID, "error", completeErr)
		}
	}
}

// StopScan handles POST /api/v1/scans/{id}/stop - stop scan execution.
func (h *ScanHandler) StopScan(w http.ResponseWriter, r *http.Request) {
	scanID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	jobOp := &JobControlOperation{
		EntityType: "scan",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	jobOp.ExecuteStop(w, r, scanID, func(ctx context.Context, id uuid.UUID) error {
		return h.database.StopScan(ctx, id)
	}, "api_scans_stopped_total")
}

// Helper methods

// validateScanRequest validates a scan request.
func (h *ScanHandler) validateScanRequest(req *ScanRequest) error {
	if req.Name == "" {
		return fmt.Errorf("scan name is required")
	}

	if len(req.Name) > maxScanNameLength {
		return fmt.Errorf("scan name too long (max %d characters)", maxScanNameLength)
	}

	if len(req.Targets) == 0 {
		return fmt.Errorf("at least one target is required")
	}

	if len(req.Targets) > maxTargetCount {
		return fmt.Errorf("too many targets (max %d)", maxTargetCount)
	}

	// Validate scan type — must match the cases handled by buildScanOptions.
	validScanTypes := map[string]bool{
		"connect":       true,
		"syn":           true,
		"ack":           true,
		"udp":           true,
		"aggressive":    true,
		"comprehensive": true,
	}

	if !validScanTypes[req.ScanType] {
		return fmt.Errorf("invalid scan type: %s", req.ScanType)
	}

	// Validate targets format (basic validation)
	for i, target := range req.Targets {
		if target == "" {
			return fmt.Errorf("target %d is empty", i+1)
		}
		if len(target) > maxTargetLength {
			return fmt.Errorf("target %d too long (max %d characters)", i+1, maxTargetLength)
		}
		if _, _, err := net.ParseCIDR(target); err != nil {
			if net.ParseIP(target) == nil {
				return fmt.Errorf("target %d: %q is not a valid IP address or CIDR range", i+1, target)
			}
		}
	}

	if req.Ports == "" {
		return fmt.Errorf("ports is required")
	}
	if err := parsePortSpec(req.Ports); err != nil {
		return err
	}

	return nil
}

// parsePortSpec validates a port specification string.
// The spec is comma-separated with optional T:/U: protocol prefixes and
// optional hyphenated ranges (e.g. "T:80,U:53,1024-9999").
// Every individual port value must be in the range 1–65535.
func parsePortSpec(ports string) error {
	for _, token := range strings.Split(ports, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if err := parsePortToken(token); err != nil {
			return err
		}
	}
	return nil
}

// parsePortToken validates a single port token (after comma-splitting).
// It strips an optional T:/U: prefix, rejects whitespace, and validates the
// port number or range.
func parsePortToken(token string) error {
	// Strip optional protocol prefix (T: or U:)
	if len(token) >= 2 && (token[0] == 'T' || token[0] == 'U') && token[1] == ':' {
		token = token[2:]
	}
	// Reject tokens containing whitespace (e.g. "80 - 443").
	if strings.ContainsAny(token, " \t") {
		return fmt.Errorf("invalid port spec %q: whitespace not allowed", token)
	}
	parts := strings.SplitN(token, "-", 2)
	if len(parts) == 2 {
		return parsePortRange(parts[0], parts[1])
	}
	return validatePortNumber(parts[0])
}

// parsePortRange validates that start and end are valid ports and start <= end.
func parsePortRange(startStr, endStr string) error {
	startNum, err := strconv.Atoi(startStr)
	if err != nil {
		return fmt.Errorf("invalid port %q: must be a number", startStr)
	}
	if startNum < 1 || startNum > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", startNum)
	}
	endNum, err := strconv.Atoi(endStr)
	if err != nil {
		return fmt.Errorf("invalid port %q: must be a number", endStr)
	}
	if endNum < 1 || endNum > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", endNum)
	}
	if startNum > endNum {
		return fmt.Errorf("invalid port range %d-%d: start must be <= end", startNum, endNum)
	}
	return nil
}

// validatePortNumber checks that s is a valid port number string (1–65535).
func validatePortNumber(s string) error {
	portNum, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("invalid port %q: must be a number", s)
	}
	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("invalid port %d: must be between 1 and 65535", portNum)
	}
	return nil
}

// getScanFilters extracts filter parameters from request.
func (h *ScanHandler) getScanFilters(r *http.Request) db.ScanFilters {
	filters := db.ScanFilters{}

	if status := r.URL.Query().Get("status"); status != "" {
		filters.Status = status
	}

	if scanType := r.URL.Query().Get("scan_type"); scanType != "" {
		filters.ScanType = scanType
	}

	if tag := r.URL.Query().Get("tag"); tag != "" {
		filters.Tags = []string{tag}
	}

	if profileID := r.URL.Query().Get("profile_id"); profileID != "" {
		filters.ProfileID = &profileID
	}

	return filters
}

// requestToDBScan converts a scan request to database scan object.
func (h *ScanHandler) requestToDBScan(req *ScanRequest) interface{} {
	// This should return the appropriate database scan type
	// The exact structure would depend on the database package implementation
	return map[string]interface{}{
		"name":         req.Name,
		"description":  req.Description,
		"targets":      req.Targets,
		"scan_type":    req.ScanType,
		"ports":        req.Ports,
		"profile_id":   req.ProfileID,
		"options":      req.Options,
		"schedule_id":  req.ScheduleID,
		"tags":         req.Tags,
		"status":       "pending",
		"created_at":   time.Now().UTC(),
		"os_detection": req.OSDetection,
	}
}

// scanToResponse converts a database scan to response format.
func (h *ScanHandler) scanToResponse(scan *db.Scan) ScanResponse {
	resp := ScanResponse{
		ID:          scan.ID,
		Name:        scan.Name,
		Description: scan.Description,
		Targets:     scan.Targets,
		ScanType:    scan.ScanType,
		Ports:       scan.Ports,
		ProfileID:   scan.ProfileID,
		ScheduleID:  scan.ScheduleID,
		Tags:        scan.Tags,
		Status:      scan.Status,
		StartTime:   scan.StartedAt,
		EndTime:     scan.CompletedAt,
		CreatedAt:   scan.CreatedAt,
		UpdatedAt:   scan.UpdatedAt,
	}

	// Ensure Targets is never nil for JSON serialization
	if resp.Targets == nil {
		resp.Targets = []string{}
	}

	// Convert options from map[string]interface{} to map[string]string
	if scan.Options != nil {
		resp.Options = make(map[string]string, len(scan.Options))
		for k, v := range scan.Options {
			resp.Options[k] = fmt.Sprintf("%v", v)
		}
	}

	// Compute progress from status
	switch scan.Status {
	case "completed":
		resp.Progress = 100.0
	case "failed":
		resp.Progress = 0.0
	case "running":
		resp.Progress = 50.0 // Approximation without a dedicated progress field
	default:
		resp.Progress = 0.0
	}

	// Compute duration if start and end times are available
	if scan.StartedAt != nil && scan.CompletedAt != nil {
		d := scan.CompletedAt.Sub(*scan.StartedAt).String()
		resp.Duration = &d
	}

	resp.ErrorMessage = scan.ErrorMessage
	resp.PortsScanned = scan.PortsScanned

	return resp
}

// resultToResponse converts a database scan result to response format.
func getOptionBool(options map[string]interface{}, key string) bool {
	if options == nil {
		return false
	}
	v, ok := options[key]
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func (h *ScanHandler) resultToResponse(result *db.ScanResult) ScanResult {
	return ScanResult{
		ID:           result.ID,
		HostIP:       result.HostIP,
		Port:         result.Port,
		Protocol:     result.Protocol,
		State:        result.State,
		Service:      result.Service,
		ScanTime:     result.ScannedAt,
		OSName:       result.OSName,
		OSFamily:     result.OSFamily,
		OSVersion:    result.OSVersion,
		OSConfidence: result.OSConfidence,
	}
}

// Helper functions for response utilities
