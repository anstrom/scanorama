// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements host management endpoints including CRUD operations
// and host-related scan retrieval.
package handlers

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

// Validation constants for host fields.
const (
	maxHostnameLength        = 255
	maxHostDescriptionLength = 1000
	maxOSInfoLength          = 100
	maxOSVersionLength       = 100
	maxServicesLength        = 100
	maxHostTagCount          = 100
	maxHostTagLength         = 50
	maxHostMetadataKeys      = 50
)

// HostHandler handles host-related API endpoints.
type HostHandler struct {
	database *db.DB
	logger   *slog.Logger
	metrics  *metrics.Registry
}

// NewHostHandler creates a new host handler.
func NewHostHandler(database *db.DB, logger *slog.Logger, metricsManager *metrics.Registry) *HostHandler {
	return &HostHandler{
		database: database,
		logger:   logger.With("handler", "host"),
		metrics:  metricsManager,
	}
}

// HostRequest represents a host creation/update request.
type HostRequest struct {
	IP          string            `json:"ip" validate:"required,ip"`
	Hostname    string            `json:"hostname,omitempty"`
	Description string            `json:"description,omitempty"`
	OS          string            `json:"os,omitempty"`
	OSVersion   string            `json:"os_version,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Active      bool              `json:"active"`
}

// HostResponse represents a host response.
type HostResponse struct {
	ID           int64             `json:"id"`
	IP           string            `json:"ip"`
	Hostname     string            `json:"hostname,omitempty"`
	Description  string            `json:"description,omitempty"`
	OS           string            `json:"os,omitempty"`
	OSVersion    string            `json:"os_version,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Active       bool              `json:"active"`
	LastSeen     *time.Time        `json:"last_seen,omitempty"`
	LastScanID   *int64            `json:"last_scan_id,omitempty"`
	ScanCount    int               `json:"scan_count"`
	OpenPorts    int               `json:"open_ports"`
	TotalPorts   int               `json:"total_ports"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	DiscoveredBy string            `json:"discovered_by,omitempty"`
}

// HostScanResponse represents a scan associated with a host.
type HostScanResponse struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	ScanType  string     `json:"scan_type"`
	Status    string     `json:"status"`
	Progress  float64    `json:"progress"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Duration  *string    `json:"duration,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ListHosts handles GET /api/v1/hosts - list all hosts with pagination.
func (h *HostHandler) ListHosts(w http.ResponseWriter, r *http.Request) {
	listOp := &ListOperation[*db.Host, db.HostFilters]{
		EntityType: "hosts",
		MetricName: "api_hosts_listed_total",
		Logger:     h.logger,
		Metrics:    h.metrics,
		GetFilters: h.getHostFilters,
		ListFromDB: h.database.ListHosts,
		ToResponse: func(host *db.Host) interface{} {
			return h.hostToResponse(host)
		},
	}
	listOp.Execute(w, r)
}

// CreateHost handles POST /api/v1/hosts - create a new host.
func (h *HostHandler) CreateHost(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Creating host", "request_id", requestID)

	// Parse request body
	var req HostRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Validate request
	if err := h.validateHostRequest(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Create host in database
	host, err := h.database.CreateHost(r.Context(), h.requestToDBHost(&req))
	if err != nil {
		if errors.IsConflict(err) {
			writeError(w, r, http.StatusConflict,
				fmt.Errorf("host with IP %s already exists", req.IP))
			return
		}
		h.logger.Error("Failed to create host", "request_id", requestID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to create host: %w", err))
		return
	}

	response := h.hostToResponse(host)

	h.logger.Info("Host created successfully",
		"request_id", requestID,
		"host_id", response.ID,
		"host_ip", response.IP)

	writeJSON(w, r, http.StatusCreated, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_hosts_created_total", nil)
	}
}

// GetHost handles GET /api/v1/hosts/{id} - get a specific host.
func (h *HostHandler) GetHost(w http.ResponseWriter, r *http.Request) {
	hostID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	crudOp := &CRUDOperation[db.Host]{
		EntityType: "host",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	crudOp.ExecuteGet(w, r, hostID,
		h.database.GetHost,
		func(host *db.Host) interface{} {
			return h.hostToResponse(host)
		},
		"api_hosts_retrieved_total")
}

// UpdateHost handles PUT /api/v1/hosts/{id} - update a host.
func (h *HostHandler) UpdateHost(w http.ResponseWriter, r *http.Request) {
	UpdateEntity[db.Host, HostRequest](
		w, r,
		"host",
		h.logger,
		h.metrics,
		func(r *http.Request) (interface{}, error) {
			var req HostRequest
			if err := parseJSON(r, &req); err != nil {
				return nil, err
			}
			return h.requestToDBHost(&req), nil
		},
		h.database.UpdateHost,
		func(host *db.Host) interface{} {
			return h.hostToResponse(host)
		},
		"api_hosts_updated_total")
}

// DeleteHost handles DELETE /api/v1/hosts/{id} - delete a host.
func (h *HostHandler) DeleteHost(w http.ResponseWriter, r *http.Request) {
	hostID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	crudOp := &CRUDOperation[db.Host]{
		EntityType: "host",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	crudOp.ExecuteDelete(w, r, hostID, h.database.DeleteHost, "api_hosts_deleted_total")
}

// GetHostScans handles GET /api/v1/hosts/{id}/scans - get scans for a specific host.
func (h *HostHandler) GetHostScans(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())

	// Extract host ID from URL
	hostID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	h.logger.Info("Getting host scans", "request_id", requestID, "host_id", hostID)

	// Parse pagination parameters
	params, err := getPaginationParams(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Parse filter parameters
	filters := h.getScanFilters(r)
	filters["host_id"] = hostID

	// Get host scans from database
	scans, total, err := h.database.GetHostScans(r.Context(), hostID, params.Offset, params.PageSize)
	if err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound,
				fmt.Errorf("host not found"))
			return
		}
		h.logger.Error("Failed to get host scans", "request_id", requestID, "host_id", hostID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to retrieve host scans: %w", err))
		return
	}

	// Convert to response format
	responses := make([]HostScanResponse, len(scans))
	for i, scan := range scans {
		responses[i] = h.scanToHostScanResponse(scan)
	}

	// Write paginated response
	writePaginatedResponse(w, r, responses, params, total)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_host_scans_retrieved_total", nil)
	}
}

// Helper methods

// validateHostRequest validates a host request.
func (h *HostHandler) validateHostRequest(req *HostRequest) error {
	if req.IP == "" {
		return fmt.Errorf("host IP is required")
	}

	// Basic IP format validation
	if net.ParseIP(req.IP) == nil {
		return fmt.Errorf("invalid IP address format: %s", req.IP)
	}

	if len(req.Hostname) > maxHostnameLength {
		return fmt.Errorf("hostname too long (max %d characters)", maxHostnameLength)
	}

	if len(req.Description) > maxHostDescriptionLength {
		return fmt.Errorf("description too long (max %d characters)", maxHostDescriptionLength)
	}

	if len(req.OS) > maxOSInfoLength {
		return fmt.Errorf("OS info too long (max %d characters)", maxOSInfoLength)
	}

	if len(req.OSVersion) > maxOSVersionLength {
		return fmt.Errorf("OS version field too long (max %d characters)", maxOSVersionLength)
	}

	// Validate tags
	for i, tag := range req.Tags {
		if tag == "" {
			return fmt.Errorf("tag %d is empty", i+1)
		}
		if len(tag) > maxHostTagLength {
			return fmt.Errorf("tag %d too long (max %d characters)", i+1, maxHostTagLength)
		}
	}

	return nil
}

// getHostFilters extracts filter parameters from request.
func (h *HostHandler) getHostFilters(r *http.Request) db.HostFilters {
	filters := db.HostFilters{}

	if os := r.URL.Query().Get("os"); os != "" {
		filters.OSFamily = os
	}

	if status := r.URL.Query().Get("status"); status != "" {
		filters.Status = status
	}

	if network := r.URL.Query().Get("network"); network != "" {
		filters.Network = network
	}

	return filters
}

// getScanFilters extracts scan filter parameters from request.
func (h *HostHandler) getScanFilters(r *http.Request) map[string]interface{} {
	filters := make(map[string]interface{})

	if status := r.URL.Query().Get("status"); status != "" {
		filters["status"] = status
	}

	if scanType := r.URL.Query().Get("scan_type"); scanType != "" {
		filters["scan_type"] = scanType
	}

	if createdAfter := r.URL.Query().Get("created_after"); createdAfter != "" {
		if timestamp, err := time.Parse(time.RFC3339, createdAfter); err == nil {
			filters["created_after"] = timestamp
		}
	}

	if createdBefore := r.URL.Query().Get("created_before"); createdBefore != "" {
		if timestamp, err := time.Parse(time.RFC3339, createdBefore); err == nil {
			filters["created_before"] = timestamp
		}
	}

	return filters
}

// requestToDBHost converts a host request to database host object.
func (h *HostHandler) requestToDBHost(req *HostRequest) interface{} {
	// This should return the appropriate database host type
	// The exact structure would depend on the database package implementation
	return map[string]interface{}{
		"ip":          req.IP,
		"hostname":    req.Hostname,
		"description": req.Description,
		"os":          req.OS,
		"os_version":  req.OSVersion,
		"tags":        req.Tags,
		"metadata":    req.Metadata,
		"active":      req.Active,
		"created_at":  time.Now().UTC(),
		"updated_at":  time.Now().UTC(),
	}
}

// hostToResponse converts a database host to response format.
func (h *HostHandler) hostToResponse(_ interface{}) HostResponse {
	// This would convert from the actual database host type
	// For now, return a placeholder structure
	return HostResponse{
		ID:          1,                   // host.ID
		IP:          "127.0.0.1",         // host.IP
		Hostname:    "",                  // host.Hostname
		Description: "",                  // host.Description
		OS:          "",                  // host.OS
		OSVersion:   "",                  // host.OSVersion
		Tags:        []string{},          // host.Tags
		Metadata:    map[string]string{}, // host.Metadata
		Active:      true,                // host.Active
		ScanCount:   0,                   // host.ScanCount
		OpenPorts:   0,                   // host.OpenPorts
		TotalPorts:  0,                   // host.TotalPorts
		CreatedAt:   time.Now().UTC(),    // host.CreatedAt
		UpdatedAt:   time.Now().UTC(),    // host.UpdatedAt
	}
}

// scanToHostScanResponse converts a scan to host scan response format.
func (h *HostHandler) scanToHostScanResponse(_ interface{}) HostScanResponse {
	// This would convert from the actual database scan type
	// For now, return a placeholder structure
	return HostScanResponse{
		ID:        1,                // scan.ID
		Name:      "",               // scan.Name
		ScanType:  "",               // scan.ScanType
		Status:    "pending",        // scan.Status
		Progress:  0.0,              // scan.Progress
		CreatedAt: time.Now().UTC(), // scan.CreatedAt
	}
}
