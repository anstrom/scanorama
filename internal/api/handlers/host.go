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

	"github.com/google/uuid"

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
	service HostServicer
	logger  *slog.Logger
	metrics *metrics.Registry
	dnsRepo *db.DNSRepository // optional; nil = DNS records not included in responses
}

// NewHostHandler creates a new host handler.
func NewHostHandler(service HostServicer, logger *slog.Logger, metricsManager *metrics.Registry) *HostHandler {
	return &HostHandler{
		service: service,
		logger:  logger.With("handler", "host"),
		metrics: metricsManager,
	}
}

// WithDNSRepository attaches a DNS repository so that GetHost includes the
// host's DNS records in its response. Returns the handler for chaining.
func (h *HostHandler) WithDNSRepository(repo *db.DNSRepository) *HostHandler {
	h.dnsRepo = repo
	return h
}

// HostRequest represents a host creation/update request.
type HostRequest struct {
	IP          string            `json:"ip_address" validate:"required,ip"`
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
	ID          string `json:"id"`
	IPAddress   string `json:"ip_address"`
	Hostname    string `json:"hostname,omitempty"`
	Description string `json:"description,omitempty"`
	// Deprecated: use OSFamily instead.
	OS string `json:"os,omitempty"`
	// Deprecated: use OSName / OSVersion instead.
	OSVersionLegacy string `json:"os_version,omitempty"`
	// OSFamily is the broad OS family detected by nmap (e.g. "Linux", "Windows").
	OSFamily string `json:"os_family,omitempty"`
	// OSName is the full OS name returned by nmap (e.g. "Linux 5.15").
	OSName string `json:"os_name,omitempty"`
	// OSVersion is the OS generation / version string returned by nmap.
	OSVersion string `json:"os_version_detail,omitempty"`
	// OSConfidence is the nmap detection confidence percentage (0–100).
	OSConfidence      *int                  `json:"os_confidence,omitempty"`
	Tags              []string              `json:"tags,omitempty"`
	Groups            []db.HostGroupSummary `json:"groups,omitempty"`
	Metadata          map[string]string     `json:"metadata,omitempty"`
	Status            string                `json:"status"`
	MACAddress        string                `json:"mac_address,omitempty"`
	Ports             []db.PortInfo         `json:"ports,omitempty"`
	FirstSeen         time.Time             `json:"first_seen"`
	LastSeen          *time.Time            `json:"last_seen,omitempty"`
	LastScanID        *int64                `json:"last_scan_id,omitempty"`
	ScanCount         int                   `json:"scan_count"`
	TotalPorts        int                   `json:"total_ports"`
	CreatedAt         time.Time             `json:"created_at"`
	UpdatedAt         time.Time             `json:"updated_at"`
	DiscoveredBy      string                `json:"discovered_by,omitempty"`
	StatusChangedAt   *time.Time            `json:"status_changed_at,omitempty"`
	PreviousStatus    string                `json:"previous_status,omitempty"`
	Vendor            string                `json:"vendor,omitempty"`
	ResponseTimeMS    *int                  `json:"response_time_ms,omitempty"`
	ResponseTimeMinMS *int                  `json:"response_time_min_ms,omitempty"`
	ResponseTimeMaxMS *int                  `json:"response_time_max_ms,omitempty"`
	ResponseTimeAvgMS *int                  `json:"response_time_avg_ms,omitempty"`
	TimeoutCount      int                   `json:"timeout_count"`
	DNSRecords        []db.DNSRecord        `json:"dns_records,omitempty"`
}

// HostScanResponse represents a scan associated with a host.
type HostScanResponse struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	ScanType  string     `json:"scan_type"`
	Ports     string     `json:"ports,omitempty"`
	Targets   []string   `json:"targets,omitempty"`
	Status    string     `json:"status"`
	Progress  float64    `json:"progress"`
	StartTime *time.Time `json:"start_time,omitempty"`
	EndTime   *time.Time `json:"end_time,omitempty"`
	Duration  *string    `json:"duration,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ListHosts handles GET /api/v1/hosts - list all hosts with pagination.
func (h *HostHandler) ListHosts(w http.ResponseWriter, r *http.Request) {
	listOp := &ListOperation[*db.Host, *db.HostFilters]{
		EntityType: "hosts",
		MetricName: "api_hosts_listed_total",
		Logger:     h.logger,
		Metrics:    h.metrics,
		GetFilters: h.getHostFilters,
		ListFromDB: h.service.ListHosts,
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
	host, err := h.service.CreateHost(r.Context(), h.requestToCreateHost(&req))
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
		"host_ip", response.IPAddress)

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

	host, err := h.service.GetHost(r.Context(), hostID)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "host", h.logger)
		return
	}

	resp := h.hostToResponse(host)

	// Attach DNS records when the repository is wired in.
	if h.dnsRepo != nil {
		if dnsRecords, dnsErr := h.dnsRepo.ListDNSRecords(r.Context(), hostID); dnsErr == nil {
			resp.DNSRecords = dnsRecords
		} else {
			h.logger.Warn("failed to fetch DNS records", "host_id", hostID, "error", dnsErr)
		}
	}

	writeJSON(w, r, http.StatusOK, resp)
	recordCRUDMetric(h.metrics, "api_hosts_retrieved_total", nil)
}

// UpdateHost handles PUT /api/v1/hosts/{id} - update a host.
func (h *HostHandler) UpdateHost(w http.ResponseWriter, r *http.Request) {
	UpdateEntity[db.Host, db.UpdateHostInput](
		w, r,
		"host",
		h.logger,
		h.metrics,
		func(r *http.Request) (db.UpdateHostInput, error) {
			var req HostRequest
			if err := parseJSON(r, &req); err != nil {
				return db.UpdateHostInput{}, err
			}
			return h.requestToUpdateHost(&req), nil
		},
		h.service.UpdateHost,
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

	crudOp.ExecuteDelete(w, r, hostID, h.service.DeleteHost, "api_hosts_deleted_total")
}

// BulkDeleteHostsRequest is the body for the bulk-delete endpoint.
type BulkDeleteHostsRequest struct {
	IDs []string `json:"ids"`
}

// BulkDeleteHostsResponse reports how many hosts were removed.
type BulkDeleteHostsResponse struct {
	Deleted int64 `json:"deleted"`
}

// BulkDeleteHosts handles DELETE /api/v1/hosts - delete multiple hosts in one call.
// The request body must contain a JSON object with an "ids" array of UUID strings.
// IDs that do not exist are silently skipped; only genuine DB errors are returned.
func (h *HostHandler) BulkDeleteHosts(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())

	var req BulkDeleteHostsRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	if len(req.IDs) == 0 {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("ids must not be empty"))
		return
	}

	const maxBulkDelete = 500
	if len(req.IDs) > maxBulkDelete {
		writeError(w, r, http.StatusBadRequest,
			fmt.Errorf("too many ids: maximum %d per request", maxBulkDelete))
		return
	}

	uuids := make([]uuid.UUID, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeError(w, r, http.StatusBadRequest,
				fmt.Errorf("invalid host id %q: %w", raw, err))
			return
		}
		uuids = append(uuids, id)
	}

	deleted, err := h.service.BulkDeleteHosts(r.Context(), uuids)
	if err != nil {
		h.logger.Error("Failed to bulk-delete hosts", "request_id", requestID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to delete hosts: %w", err))
		return
	}

	h.logger.Info("Hosts bulk-deleted", "request_id", requestID, "count", deleted)

	writeJSON(w, r, http.StatusOK, BulkDeleteHostsResponse{Deleted: deleted})

	if h.metrics != nil {
		h.metrics.Counter("api_hosts_bulk_deleted_total", nil)
	}
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

	// Verify the host exists before fetching its scans.
	if _, err := h.service.GetHost(r.Context(), hostID); err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound, fmt.Errorf("host not found"))
			return
		}
		h.logger.Error("Failed to verify host existence", "request_id", requestID, "host_id", hostID, "error", err)
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to retrieve host: %w", err))
		return
	}

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
	scans, total, err := h.service.GetHostScans(r.Context(), hostID, params.Offset, params.PageSize)
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
func (h *HostHandler) getHostFilters(r *http.Request) *db.HostFilters {
	filters := &db.HostFilters{}

	if os := r.URL.Query().Get("os"); os != "" {
		filters.OSFamily = os
	}

	if status := r.URL.Query().Get("status"); status != "" {
		filters.Status = status
	}

	if network := r.URL.Query().Get("network"); network != "" {
		filters.Network = network
	}

	if search := r.URL.Query().Get("search"); search != "" {
		filters.Search = search
	}

	if sortBy := r.URL.Query().Get("sort_by"); sortBy != "" {
		filters.SortBy = sortBy
	}

	if sortOrder := r.URL.Query().Get("sort_order"); sortOrder != "" {
		filters.SortOrder = sortOrder
	}

	if vendor := r.URL.Query().Get("vendor"); vendor != "" {
		filters.Vendor = vendor
	}

	if filterJSON := r.URL.Query().Get("filter"); filterJSON != "" {
		if expr, err := db.ParseFilterExpr([]byte(filterJSON)); err == nil {
			filters.Expr = expr
		}
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

// requestToCreateHost converts a HostRequest to a typed CreateHostInput for the DB layer.
func (h *HostHandler) requestToCreateHost(req *HostRequest) db.CreateHostInput {
	input := db.CreateHostInput{
		IPAddress:      req.IP,
		Status:         "up",
		IgnoreScanning: !req.Active,
		Tags:           req.Tags,
	}
	if req.Hostname != "" {
		input.Hostname = req.Hostname
	}
	if req.OS != "" {
		input.OSFamily = req.OS
	}
	if req.OSVersion != "" {
		input.OSName = req.OSVersion
	}
	return input
}

// requestToUpdateHost converts a HostRequest to a typed UpdateHostInput for the DB layer.
func (h *HostHandler) requestToUpdateHost(req *HostRequest) db.UpdateHostInput {
	input := db.UpdateHostInput{}
	if req.Hostname != "" {
		input.Hostname = &req.Hostname
	}
	if req.OS != "" {
		input.OSFamily = &req.OS
	}
	if req.OSVersion != "" {
		input.OSName = &req.OSVersion
	}
	// Always propagate the active/ignore_scanning flag.
	ignoreScanning := !req.Active
	input.IgnoreScanning = &ignoreScanning
	if req.Tags != nil {
		input.Tags = &req.Tags
	}
	return input
}

// hostToResponse converts a database host to response format.
// populateOSFields fills OS-related fields on the response from the host model.
func populateOSFields(r *HostResponse, host *db.Host) {
	if host.OSFamily != nil {
		r.OSFamily = *host.OSFamily
		r.OS = *host.OSFamily
	}
	if host.OSName != nil {
		r.OSName = *host.OSName
	}
	if host.OSVersion != nil {
		r.OSVersion = *host.OSVersion
	}
	if host.OSConfidence != nil {
		r.OSConfidence = host.OSConfidence
	}
	switch {
	case host.OSName != nil && host.OSVersion != nil:
		r.OSVersionLegacy = fmt.Sprintf("%s %s", *host.OSName, *host.OSVersion)
	case host.OSName != nil:
		r.OSVersionLegacy = *host.OSName
	case host.OSVersion != nil:
		r.OSVersionLegacy = *host.OSVersion
	}
}

// populateResponseTimeFields fills response-time statistics on the response.
func populateResponseTimeFields(r *HostResponse, host *db.Host) {
	if host.ResponseTimeMS != nil {
		r.ResponseTimeMS = host.ResponseTimeMS
	}
	if host.ResponseTimeMinMS != nil {
		r.ResponseTimeMinMS = host.ResponseTimeMinMS
	}
	if host.ResponseTimeMaxMS != nil {
		r.ResponseTimeMaxMS = host.ResponseTimeMaxMS
	}
	if host.ResponseTimeAvgMS != nil {
		r.ResponseTimeAvgMS = host.ResponseTimeAvgMS
	}
	r.TimeoutCount = host.TimeoutCount
}

func (h *HostHandler) hostToResponse(host *db.Host) HostResponse {
	response := HostResponse{
		ID:        host.ID.String(),
		IPAddress: host.IPAddress.String(),
		Status:    host.Status,
		FirstSeen: host.FirstSeen,
		CreatedAt: host.FirstSeen,
		UpdatedAt: host.LastSeen,
	}

	// Handle optional fields
	if host.Hostname != nil {
		response.Hostname = *host.Hostname
	}

	// Populate all OS fields individually so the frontend can display each one.
	populateOSFields(&response, host)

	// Set last seen time
	response.LastSeen = &host.LastSeen

	if host.MACAddress != nil {
		response.MACAddress = host.MACAddress.String()
	}

	response.TotalPorts = host.TotalPorts
	response.ScanCount = host.ScanCount
	if host.Ports != nil {
		response.Ports = host.Ports
	} else {
		response.Ports = []db.PortInfo{}
	}
	if len(host.Tags) > 0 {
		response.Tags = host.Tags
	} else {
		response.Tags = []string{}
	}
	response.Groups = host.Groups
	response.Metadata = map[string]string{}

	if host.StatusChangedAt != nil {
		response.StatusChangedAt = host.StatusChangedAt
	}
	if host.PreviousStatus != nil {
		response.PreviousStatus = *host.PreviousStatus
	}
	if host.Vendor != nil {
		response.Vendor = *host.Vendor
	}
	populateResponseTimeFields(&response, host)

	return response
}

// scanToHostScanResponse converts a scan to host scan response format.
func (h *HostHandler) scanToHostScanResponse(scan *db.Scan) HostScanResponse {
	response := HostScanResponse{
		ID:        scan.ID.String(),
		Name:      scan.Name,
		ScanType:  scan.ScanType,
		Ports:     scan.Ports,
		Targets:   scan.Targets,
		Status:    scan.Status,
		Progress:  0.0, // would need live progress tracking to populate
		CreatedAt: scan.CreatedAt,
	}

	if scan.StartedAt != nil {
		response.StartTime = scan.StartedAt
	}

	if scan.CompletedAt != nil {
		response.EndTime = scan.CompletedAt
		if scan.StartedAt != nil {
			duration := scan.CompletedAt.Sub(*scan.StartedAt)
			durationStr := duration.String()
			response.Duration = &durationStr
		}
	}

	return response
}
