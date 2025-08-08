// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements scan management endpoints including CRUD operations,
// scan execution control, and results retrieval.
package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

// Scan validation constants.
const (
	maxScanNameLength = 255
	maxTargetLength   = 255
)

// ScanHandler handles scan-related API endpoints.
type ScanHandler struct {
	database *db.DB
	logger   *slog.Logger
	metrics  *metrics.Registry
}

// NewScanHandler creates a new scan handler.
func NewScanHandler(database *db.DB, logger *slog.Logger, metricsManager *metrics.Registry) *ScanHandler {
	return &ScanHandler{
		database: database,
		logger:   logger.With("handler", "scan"),
		metrics:  metricsManager,
	}
}

// ScanRequest represents a scan creation/update request.
type ScanRequest struct {
	Name        string            `json:"name" validate:"required,min=1,max=255"`
	Description string            `json:"description,omitempty"`
	Targets     []string          `json:"targets" validate:"required,min=1"`
	ScanType    string            `json:"scan_type" validate:"required,oneof=connect syn ack aggressive comprehensive"`
	Ports       string            `json:"ports,omitempty"`
	ProfileID   *int64            `json:"profile_id,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
	ScheduleID  *int64            `json:"schedule_id,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
}

// ScanResponse represents a scan response.
type ScanResponse struct {
	ID          uuid.UUID         `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Targets     []string          `json:"targets"`
	ScanType    string            `json:"scan_type"`
	Ports       string            `json:"ports,omitempty"`
	ProfileID   *int64            `json:"profile_id,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
	ScheduleID  *int64            `json:"schedule_id,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Status      string            `json:"status"`
	Progress    float64           `json:"progress"`
	StartTime   *time.Time        `json:"start_time,omitempty"`
	EndTime     *time.Time        `json:"end_time,omitempty"`
	Duration    *string           `json:"duration,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	CreatedBy   string            `json:"created_by,omitempty"`
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
	ID       uuid.UUID `json:"id"`
	HostIP   string    `json:"host_ip"`
	Hostname string    `json:"hostname,omitempty"`
	Port     int       `json:"port"`
	Protocol string    `json:"protocol"`
	State    string    `json:"state"`
	Service  string    `json:"service,omitempty"`
	Version  string    `json:"version,omitempty"`
	Banner   string    `json:"banner,omitempty"`
	ScanTime time.Time `json:"scan_time"`
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

	jobOp.ExecuteStart(w, r, scanID, h.database.StartScan,
		func(ctx context.Context, id uuid.UUID) (interface{}, error) {
			return h.database.GetScan(ctx, id)
		},
		func(item interface{}) interface{} {
			if scan, ok := item.(*db.Scan); ok {
				return h.scanToResponse(scan)
			}
			return nil
		}, "api_scans_started_total")
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

	jobOp.ExecuteStop(w, r, scanID, h.database.StopScan, "api_scans_stopped_total")
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

	// Validate scan type
	validScanTypes := map[string]bool{
		"connect":       true,
		"syn":           true,
		"ack":           true,
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
		if id, err := strconv.ParseInt(profileID, 10, 64); err == nil {
			filters.ProfileID = &id
		}
	}

	return filters
}

// requestToDBScan converts a scan request to database scan object.
func (h *ScanHandler) requestToDBScan(req *ScanRequest) interface{} {
	// This should return the appropriate database scan type
	// The exact structure would depend on the database package implementation
	return map[string]interface{}{
		"name":        req.Name,
		"description": req.Description,
		"targets":     req.Targets,
		"scan_type":   req.ScanType,
		"ports":       req.Ports,
		"profile_id":  req.ProfileID,
		"options":     req.Options,
		"schedule_id": req.ScheduleID,
		"tags":        req.Tags,
		"status":      "pending",
		"created_at":  time.Now().UTC(),
	}
}

// scanToResponse converts a database scan to response format.
func (h *ScanHandler) scanToResponse(scan *db.Scan) ScanResponse {
	// This would convert from the actual database scan type
	// For now, return a placeholder structure
	return ScanResponse{
		ID:          scan.ID,
		Name:        scan.Name,
		Description: scan.Description,
		Targets:     []string{}, // scan.Targets
		ScanType:    scan.ScanType,
		Status:      scan.Status,
		Progress:    0.0,
		CreatedAt:   scan.CreatedAt,
		UpdatedAt:   scan.UpdatedAt,
	}
}

// resultToResponse converts a database scan result to response format.
func (h *ScanHandler) resultToResponse(result *db.ScanResult) ScanResult {
	return ScanResult{
		ID:       result.ID,
		HostIP:   "", // Would need to lookup host IP from HostID
		Hostname: "", // Would need to lookup hostname from HostID
		Port:     result.Port,
		Protocol: result.Protocol,
		State:    result.State,
		Service:  result.Service,
		Version:  "",               // Not available in current db.ScanResult
		Banner:   "",               // Not available in current db.ScanResult
		ScanTime: time.Now().UTC(), // Would use actual scan time from database
	}
}

// Helper functions for response utilities
