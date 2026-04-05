package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/internal/scanning"
	"github.com/anstrom/scanorama/internal/services"
)

// NetworkHandler handles network-related API requests.
type NetworkHandler struct {
	*BaseHandler
	service        NetworkServicer
	discoveryStore DiscoveryStore
	engine         *discovery.Engine
	scanQueue      *scanning.ScanQueue
	hostService    HostServicer
	scanService    ScanServicer
}

// NewNetworkHandler creates a new network handler.
func NewNetworkHandler(
	service NetworkServicer, logger *slog.Logger, metricsRegistry metrics.MetricsRegistry,
) *NetworkHandler {
	return &NetworkHandler{
		BaseHandler: NewBaseHandler(logger, metricsRegistry),
		service:     service,
	}
}

// WithDiscovery injects the discovery store and engine so that
// POST /networks/{id}/discover can create and immediately run a discovery job.
func (h *NetworkHandler) WithDiscovery(store DiscoveryStore, engine *discovery.Engine) *NetworkHandler {
	h.discoveryStore = store
	h.engine = engine
	return h
}

// WithScanQueue sets the worker pool used to execute network discovery jobs.
// When set, StartNetworkDiscovery submits a discoveryJob to the queue instead
// of spawning an unbounded goroutine.
func (h *NetworkHandler) WithScanQueue(q *scanning.ScanQueue) *NetworkHandler {
	h.scanQueue = q
	return h
}

// WithHostService wires the host service into the handler.
func (h *NetworkHandler) WithHostService(svc HostServicer) *NetworkHandler {
	h.hostService = svc
	return h
}

// WithScanService wires the scan service into the handler.
func (h *NetworkHandler) WithScanService(svc ScanServicer) *NetworkHandler {
	h.scanService = svc
	return h
}

// CreateNetworkRequest represents the request body for creating a network.
type CreateNetworkRequest struct {
	Name            string  `json:"name"             validate:"required,min=1,max=100"`
	CIDR            string  `json:"cidr"             validate:"required,cidr"`
	Description     *string `json:"description,omitempty"`
	DiscoveryMethod string  `json:"discovery_method" validate:"required,oneof=ping tcp arp"`
	IsActive        *bool   `json:"is_active,omitempty"`
	ScanEnabled     *bool   `json:"scan_enabled,omitempty"`
}

// UpdateNetworkRequest represents the request body for updating a network.
type UpdateNetworkRequest struct {
	Name            *string `json:"name,omitempty"             validate:"omitempty,min=1,max=100"`
	CIDR            *string `json:"cidr,omitempty"             validate:"omitempty,cidr"`
	Description     *string `json:"description,omitempty"`
	DiscoveryMethod *string `json:"discovery_method,omitempty" validate:"omitempty,oneof=ping tcp arp"`
	IsActive        *bool   `json:"is_active,omitempty"`
	ScanEnabled     *bool   `json:"scan_enabled,omitempty"`
}

// RenameNetworkRequest represents the request body for renaming a network.
type RenameNetworkRequest struct {
	NewName string `json:"new_name" validate:"required,min=1,max=100"`
}

// CreateExclusionRequest represents the request body for creating a network exclusion.
type CreateExclusionRequest struct {
	ExcludedCIDR string  `json:"excluded_cidr" validate:"required,cidr"`
	Reason       *string `json:"reason,omitempty"`
}

// NetworkStatsResponse is the response body for network statistics.
type NetworkStatsResponse struct {
	Networks   map[string]interface{} `json:"networks"`
	Hosts      map[string]interface{} `json:"hosts"`
	Exclusions map[string]interface{} `json:"exclusions"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (h *NetworkHandler) parseNetworkFilters(r *http.Request) (showInactive bool, nameFilter string) {
	showInactive = r.URL.Query().Get("show_inactive") == "true"
	nameFilter = r.URL.Query().Get("name")
	return showInactive, nameFilter
}

func (h *NetworkHandler) applyNetworkFilters(
	networks []*db.Network, showInactive bool, nameFilter string,
) []*db.Network {
	filtered := make([]*db.Network, 0, len(networks))
	for _, network := range networks {
		if !h.shouldIncludeNetwork(network, showInactive, nameFilter) {
			continue
		}
		filtered = append(filtered, network)
	}
	return filtered
}

func (h *NetworkHandler) shouldIncludeNetwork(network *db.Network, showInactive bool, nameFilter string) bool {
	if !showInactive && !network.IsActive {
		return false
	}
	if nameFilter != "" && !strings.Contains(strings.ToLower(network.Name), strings.ToLower(nameFilter)) {
		return false
	}
	return true
}

func (h *NetworkHandler) setNetworkDefaults(req *CreateNetworkRequest) (isActive, scanEnabled bool) {
	isActive = true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}
	scanEnabled = true
	if req.ScanEnabled != nil {
		scanEnabled = *req.ScanEnabled
	}
	return isActive, scanEnabled
}

func (h *NetworkHandler) convertNetworkWithExclusionsToNetwork(nwe *services.NetworkWithExclusions) *db.Network {
	return nwe.Network
}

func (h *NetworkHandler) updateNetworkFields(network *db.Network, req *UpdateNetworkRequest) {
	if req.Name != nil {
		network.Name = *req.Name
	}
	if req.Description != nil {
		network.Description = req.Description
	}
	if req.DiscoveryMethod != nil {
		network.DiscoveryMethod = *req.DiscoveryMethod
	}
	if req.IsActive != nil {
		network.IsActive = *req.IsActive
	}
	if req.ScanEnabled != nil {
		network.ScanEnabled = *req.ScanEnabled
	}
}

func (h *NetworkHandler) updateNetworkStatusFields(network *db.Network, isActive, scanEnabled bool) {
	network.IsActive = isActive
	network.ScanEnabled = scanEnabled
}

func (h *NetworkHandler) parseReasonFromRequest(req *CreateExclusionRequest) string {
	if req.Reason != nil {
		return *req.Reason
	}
	return ""
}

// ---------------------------------------------------------------------------
// Validation helpers
// ---------------------------------------------------------------------------

const maxNetworkNameLen = 100

var validDiscoveryMethods = map[string]bool{
	"ping": true,
	"tcp":  true,
	"arp":  true,
	"icmp": true,
}

func (h *NetworkHandler) validateCreateNetworkRequest(req *CreateNetworkRequest) error {
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(req.Name) > maxNetworkNameLen {
		return fmt.Errorf("name too long (max 100 characters)")
	}
	if req.CIDR == "" {
		return fmt.Errorf("cidr is required")
	}
	if _, _, err := net.ParseCIDR(req.CIDR); err != nil {
		return fmt.Errorf("invalid cidr %q: %w", req.CIDR, err)
	}
	if !validDiscoveryMethods[req.DiscoveryMethod] {
		return fmt.Errorf("invalid discovery_method %q: must be one of ping, tcp, arp, icmp", req.DiscoveryMethod)
	}
	return nil
}

func (h *NetworkHandler) validateUpdateNetworkRequest(req *UpdateNetworkRequest) error {
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return fmt.Errorf("name cannot be empty")
		}
		if len(*req.Name) > maxNetworkNameLen {
			return fmt.Errorf("name too long (max 100 characters)")
		}
	}
	if req.CIDR != nil {
		if _, _, err := net.ParseCIDR(*req.CIDR); err != nil {
			return fmt.Errorf("invalid cidr %q: %w", *req.CIDR, err)
		}
	}
	if req.DiscoveryMethod != nil && !validDiscoveryMethods[*req.DiscoveryMethod] {
		return fmt.Errorf("invalid discovery_method %q: must be one of ping, tcp, arp, icmp", *req.DiscoveryMethod)
	}
	return nil
}

func (h *NetworkHandler) validateRenameNetworkRequest(req *RenameNetworkRequest) error {
	if strings.TrimSpace(req.NewName) == "" {
		return fmt.Errorf("new_name is required")
	}
	if len(req.NewName) > maxNetworkNameLen {
		return fmt.Errorf("new_name too long (max 100 characters)")
	}
	return nil
}

func (h *NetworkHandler) validateCreateExclusionRequest(req *CreateExclusionRequest) error {
	if req.ExcludedCIDR == "" {
		return fmt.Errorf("excluded_cidr is required")
	}
	if _, _, err := net.ParseCIDR(req.ExcludedCIDR); err != nil {
		return fmt.Errorf("invalid excluded_cidr %q: %w", req.ExcludedCIDR, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// API Handlers
// ---------------------------------------------------------------------------

// ListNetworks handles GET /api/v1/networks.
func (h *NetworkHandler) ListNetworks(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Listing networks", "request_id", requestID)

	params, err := getPaginationParams(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	showInactive, nameFilter := h.parseNetworkFilters(r)

	networks, err := h.service.ListNetworks(r.Context(), false)
	if err != nil {
		handleDatabaseError(w, r, err, "list", "networks", h.logger)
		return
	}

	filteredNetworks := h.applyNetworkFilters(networks, showInactive, nameFilter)

	totalItems := int64(len(filteredNetworks))
	startIdx := params.Offset
	endIdx := startIdx + params.PageSize

	if startIdx >= len(filteredNetworks) {
		filteredNetworks = []*db.Network{}
	} else {
		if endIdx > len(filteredNetworks) {
			endIdx = len(filteredNetworks)
		}
		filteredNetworks = filteredNetworks[startIdx:endIdx]
	}

	writePaginatedResponse(w, r, filteredNetworks, params, totalItems)
	recordCRUDMetric(h.metrics, "networks_listed", map[string]string{"status": "success"})
}

// CreateNetwork handles POST /api/v1/networks.
func (h *NetworkHandler) CreateNetwork(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Creating network", "request_id", requestID)

	var req CreateNetworkRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if err := h.validateCreateNetworkRequest(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	isActive, scanEnabled := h.setNetworkDefaults(&req)

	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	network, err := h.service.CreateNetwork(
		r.Context(), req.Name, req.CIDR, description, req.DiscoveryMethod, isActive, scanEnabled,
	)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, r, http.StatusConflict, err)
			return
		}
		handleDatabaseError(w, r, err, "create", "network", h.logger)
		return
	}

	h.logger.Info("Network created successfully", "request_id", requestID, "network_id", network.ID)
	writeJSON(w, r, http.StatusCreated, network)
	recordCRUDMetric(h.metrics, "networks_created", nil)
}

// GetNetwork handles GET /api/v1/networks/{id}.
func (h *NetworkHandler) GetNetwork(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	networkWithExclusions, err := h.service.GetNetworkByID(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "network", h.logger)
		return
	}

	writeJSON(w, r, http.StatusOK, h.convertNetworkWithExclusionsToNetwork(networkWithExclusions))
	recordCRUDMetric(h.metrics, "networks_retrieved", nil)
}

// UpdateNetwork handles PUT /api/v1/networks/{id}.
func (h *NetworkHandler) UpdateNetwork(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	var req UpdateNetworkRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if err := h.validateUpdateNetworkRequest(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	networkWithExclusions, err := h.service.GetNetworkByID(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "network", h.logger)
		return
	}

	network := h.convertNetworkWithExclusionsToNetwork(networkWithExclusions)
	h.updateNetworkFields(network, &req)

	description := ""
	if network.Description != nil {
		description = *network.Description
	}

	updatedNetwork, err := h.service.UpdateNetwork(r.Context(), id, network.Name,
		network.CIDR.String(), description, network.DiscoveryMethod, network.IsActive)
	if err != nil {
		handleDatabaseError(w, r, err, "update", "network", h.logger)
		return
	}

	writeJSON(w, r, http.StatusOK, updatedNetwork)
	recordCRUDMetric(h.metrics, "networks_updated", nil)
}

// DeleteNetwork handles DELETE /api/v1/networks/{id}.
func (h *NetworkHandler) DeleteNetwork(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	if err := h.service.DeleteNetwork(r.Context(), id); err != nil {
		handleDatabaseError(w, r, err, "delete", "network", h.logger)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	recordCRUDMetric(h.metrics, "networks_deleted", map[string]string{"status": "success"})
}

// EnableNetwork handles POST /api/v1/networks/{id}/enable.
func (h *NetworkHandler) EnableNetwork(w http.ResponseWriter, r *http.Request) {
	h.updateNetworkStatus(w, r, true, true, "enable")
}

// DisableNetwork handles POST /api/v1/networks/{id}/disable.
func (h *NetworkHandler) DisableNetwork(w http.ResponseWriter, r *http.Request) {
	h.updateNetworkStatus(w, r, false, false, "disable")
}

// RenameNetwork handles PUT /api/v1/networks/{id}/rename.
func (h *NetworkHandler) RenameNetwork(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	var req RenameNetworkRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if err := h.validateRenameNetworkRequest(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	networkWithExclusions, err := h.service.GetNetworkByID(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "network", h.logger)
		return
	}

	network := h.convertNetworkWithExclusionsToNetwork(networkWithExclusions)
	description := ""
	if network.Description != nil {
		description = *network.Description
	}

	updatedNetwork, err := h.service.UpdateNetwork(r.Context(), id, req.NewName,
		network.CIDR.String(), description, network.DiscoveryMethod, network.IsActive)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, r, http.StatusConflict, fmt.Errorf("network name %q already exists", req.NewName))
			return
		}
		handleDatabaseError(w, r, err, "rename", "network", h.logger)
		return
	}

	writeJSON(w, r, http.StatusOK, updatedNetwork)
	recordCRUDMetric(h.metrics, "networks_renamed", nil)
}

// GetNetworkStats handles GET /api/v1/networks/stats.
func (h *NetworkHandler) GetNetworkStats(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Getting network statistics", "request_id", requestID)

	stats, err := h.service.GetNetworkStats(r.Context())
	if err != nil {
		handleDatabaseError(w, r, err, "get", "network stats", h.logger)
		return
	}

	writeJSON(w, r, http.StatusOK, stats)
	recordCRUDMetric(h.metrics, "network_stats_retrieved", nil)
}

// updateNetworkStatus is a helper for enabling/disabling a network.
func (h *NetworkHandler) updateNetworkStatus(
	w http.ResponseWriter, r *http.Request, isActive, scanEnabled bool, action string,
) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	networkWithExclusions, err := h.service.GetNetworkByID(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, action, "network", h.logger)
		return
	}

	network := h.convertNetworkWithExclusionsToNetwork(networkWithExclusions)
	h.updateNetworkStatusFields(network, isActive, scanEnabled)

	description := ""
	if network.Description != nil {
		description = *network.Description
	}

	updatedNetwork, err := h.service.UpdateNetwork(r.Context(), id, network.Name,
		network.CIDR.String(), description, network.DiscoveryMethod, network.IsActive)
	if err != nil {
		handleDatabaseError(w, r, err, action, "network", h.logger)
		return
	}

	writeJSON(w, r, http.StatusOK, updatedNetwork)
	recordCRUDMetric(h.metrics, "networks_"+action+"d", nil)
}

// ListNetworkExclusions handles GET /api/v1/networks/{id}/exclusions.
func (h *NetworkHandler) ListNetworkExclusions(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	exclusions, err := h.service.GetNetworkExclusions(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "list", "network exclusions", h.logger)
		return
	}

	writeJSON(w, r, http.StatusOK, exclusions)
	recordCRUDMetric(h.metrics, "network_exclusions_listed", nil)
}

// CreateNetworkExclusion handles POST /api/v1/networks/{id}/exclusions.
func (h *NetworkHandler) CreateNetworkExclusion(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	var req CreateExclusionRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if err := h.validateCreateExclusionRequest(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	reason := h.parseReasonFromRequest(&req)
	exclusion, err := h.service.AddExclusion(r.Context(), &id, req.ExcludedCIDR, reason)
	if err != nil {
		handleDatabaseError(w, r, err, "create", "network exclusion", h.logger)
		return
	}

	writeJSON(w, r, http.StatusCreated, exclusion)
	recordCRUDMetric(h.metrics, "network_exclusions_created", nil)
}

// ListGlobalExclusions handles GET /api/v1/exclusions.
func (h *NetworkHandler) ListGlobalExclusions(w http.ResponseWriter, r *http.Request) {
	exclusions, err := h.service.GetGlobalExclusions(r.Context())
	if err != nil {
		handleDatabaseError(w, r, err, "list", "global exclusions", h.logger)
		return
	}

	writeJSON(w, r, http.StatusOK, exclusions)
	recordCRUDMetric(h.metrics, "global_exclusions_listed", nil)
}

// CreateGlobalExclusion handles POST /api/v1/exclusions.
func (h *NetworkHandler) CreateGlobalExclusion(w http.ResponseWriter, r *http.Request) {
	var req CreateExclusionRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if err := h.validateCreateExclusionRequest(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	reason := h.parseReasonFromRequest(&req)
	exclusion, err := h.service.AddExclusion(r.Context(), nil, req.ExcludedCIDR, reason)
	if err != nil {
		handleDatabaseError(w, r, err, "create", "global exclusion", h.logger)
		return
	}

	writeJSON(w, r, http.StatusCreated, exclusion)
	recordCRUDMetric(h.metrics, "global_exclusions_created", nil)
}

// StartNetworkDiscovery handles POST /api/v1/networks/{id}/discover.
// It creates a discovery job from the network's own CIDR and discovery_method,
// immediately starts it, and returns the running job.
//
//	@Summary		Start a discovery run for a network
//	@Description	Creates a discovery job linked to the network, immediately transitions it to running,
//	@Description	and returns the job. If a discovery engine is configured the scan executes asynchronously.
//	@Tags			networks
//	@Produce		json
//	@Param			id	path		string	true	"Network UUID"
//	@Success		202	{object}	DiscoveryResponse
//	@Failure		400	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Failure		503	{object}	ErrorResponse
//	@Router			/networks/{id}/discover [post]
func (h *NetworkHandler) StartNetworkDiscovery(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())

	if h.discoveryStore == nil {
		writeError(w, r, http.StatusServiceUnavailable,
			fmt.Errorf("discovery is not configured on this server"))
		return
	}

	networkID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Look up the registered network to get its CIDR and preferred method.
	nwe, err := h.service.GetNetworkByID(r.Context(), networkID)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "network", h.logger)
		return
	}
	network := nwe.Network

	cidr := network.CIDR.String()
	method := network.DiscoveryMethod
	if method == "" {
		method = "tcp"
	}

	// Create the discovery job.
	job, err := h.discoveryStore.CreateDiscoveryJob(r.Context(), db.CreateDiscoveryJobInput{
		Networks:  []string{cidr},
		Method:    method,
		NetworkID: &networkID,
	})
	if err != nil {
		h.logger.Error("Failed to create discovery job for network",
			"request_id", requestID, "network_id", networkID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to create discovery job: %w", err))
		return
	}

	// Transition the job to running.
	if err := h.discoveryStore.StartDiscoveryJob(r.Context(), job.ID); err != nil {
		h.logger.Error("Failed to start discovery job",
			"request_id", requestID, "job_id", job.ID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to start discovery job: %w", err))
		return
	}

	// If an engine is wired up, run the actual nmap scan in the background.
	if h.engine != nil {
		djob := &discoveryJob{
			id:        job.ID.String(),
			jobID:     job.ID,
			network:   cidr,
			method:    method,
			engine:    h.engine,
			database:  h.discoveryStore,
			logger:    h.logger,
			cancelCtx: context.Background(),
			cleanup:   func() {},
		}

		if err := h.submitDiscoveryJob(r.Context(), djob); err != nil {
			h.logger.Error("Failed to submit network discovery job to queue",
				"request_id", requestID, "job_id", job.ID, "error", err)
			writeError(w, r, http.StatusServiceUnavailable, err)
			return
		}
	} else {
		h.logger.Warn("No discovery engine configured — job marked running but no scan will execute",
			"request_id", requestID, "job_id", job.ID)
	}

	// Return the freshly-started job.
	updated, err := h.discoveryStore.GetDiscoveryJob(r.Context(), job.ID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to retrieve started discovery job: %w", err))
		return
	}

	h.logger.Info("Network discovery started",
		"request_id", requestID,
		"network_id", networkID,
		"job_id", job.ID,
		"cidr", cidr,
		"method", method)

	writeJSON(w, r, http.StatusAccepted, discoveryJobToResponse(updated))

	if h.metrics != nil {
		h.metrics.Counter("api_network_discovery_started_total", nil)
	}
}

// submitDiscoveryJob routes djob to the scan queue if one is configured, or
// falls back to a bare goroutine. On queue-submission failure it reverts the
// DB row and returns an error; the caller writes the HTTP response.
func (h *NetworkHandler) submitDiscoveryJob(ctx context.Context, djob *discoveryJob) error {
	if h.scanQueue == nil {
		go func() {
			if err := djob.Execute(context.Background()); err != nil {
				h.logger.Error("Network discovery goroutine failed", "job_id", djob.jobID, "error", err)
			}
		}()
		return nil
	}
	if err := h.scanQueue.Submit(djob); err != nil {
		_ = h.discoveryStore.StopDiscoveryJob(ctx, djob.jobID)
		return fmt.Errorf("job queue unavailable, please try again later")
	}
	return nil
}

// discoveryJobToResponse converts a db.DiscoveryJob to a DiscoveryResponse.
// This is a package-level helper shared between DiscoveryHandler and NetworkHandler.
func discoveryJobToResponse(job *db.DiscoveryJob) DiscoveryResponse {
	resp := DiscoveryResponse{
		ID:         job.ID,
		Networks:   []string{job.Network.String()},
		Method:     job.Method,
		Enabled:    job.Status != db.DiscoveryJobStatusFailed,
		Status:     job.Status,
		HostsFound: job.HostsDiscovered,
		CreatedAt:  job.CreatedAt,
		UpdatedAt:  job.CreatedAt,
	}
	switch job.Status {
	case db.DiscoveryJobStatusCompleted:
		resp.Progress = 100.0
	case db.DiscoveryJobStatusRunning:
		resp.Progress = 5.0 // minimal progress indicator; real progress is in DiscoveryHandler
	}
	if job.StartedAt != nil {
		resp.StartedAt = job.StartedAt
	}
	return resp
}

// ListNetworkDiscoveryJobs handles GET /api/v1/networks/{id}/discovery.
// Returns a paginated list of discovery jobs linked to the specified network,
// ordered most-recent first.
//
//	@Summary		List discovery jobs for a network
//	@Description	Returns paginated history of discovery runs linked to the network.
//	@Tags			networks
//	@Produce		json
//	@Param			id		path		string	true	"Network UUID"
//	@Param			page		query		int		false	"Page number (default 1)"
//	@Param			page_size	query		int		false	"Page size (default 50, max 100)"
//	@Success		200		{object}	PaginatedResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		503		{object}	ErrorResponse
//	@Router			/networks/{id}/discovery [get]
func (h *NetworkHandler) ListNetworkDiscoveryJobs(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())

	if h.discoveryStore == nil {
		writeError(w, r, http.StatusServiceUnavailable,
			fmt.Errorf("discovery is not configured on this server"))
		return
	}

	networkID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	params, err := getPaginationParams(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	jobs, total, err := h.discoveryStore.ListDiscoveryJobsByNetwork(
		r.Context(), networkID, params.Offset, params.PageSize)
	if err != nil {
		h.logger.Error("Failed to list discovery jobs for network",
			"request_id", requestID, "network_id", networkID, "error", err)
		handleDatabaseError(w, r, err, "list", "discovery jobs", h.logger)
		return
	}

	items := make([]interface{}, len(jobs))
	for i, job := range jobs {
		resp := discoveryJobToResponse(job)
		items[i] = resp
	}

	h.logger.Info("Listed network discovery jobs",
		"request_id", requestID,
		"network_id", networkID,
		"count", len(jobs),
		"total", total)

	writePaginatedResponse(w, r, items, params, total)
}

// networkScanRequest is the optional JSON body accepted by StartNetworkScan.
type networkScanRequest struct {
	// OSDetection enables nmap OS fingerprinting (-O) on every target.
	// Requires the scanner to run with elevated privileges.
	OSDetection bool `json:"os_detection"`
}

// StartNetworkScan handles POST /api/v1/networks/{id}/scan.
// Lists all active (status=up) hosts in the network's CIDR, creates a pending
// scan job targeting those hosts, and returns it ready to be started.
//
//	@Summary		Scan all active hosts in a network
//	@Description	Creates a pending scan targeting every active host discovered in the network's CIDR.
//	@Tags			networks
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"Network UUID"
//	@Param			body	body		networkScanRequest	false	"Scan options"
//	@Success		201		{object}	ScanResponse
//	@Failure		400		{object}	ErrorResponse
//	@Failure		404		{object}	ErrorResponse
//	@Failure		503		{object}	ErrorResponse
//	@Router			/networks/{id}/scan [post]
func (h *NetworkHandler) StartNetworkScan(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())

	if h.hostService == nil || h.scanService == nil {
		writeError(w, r, http.StatusServiceUnavailable,
			fmt.Errorf("scan service is not configured on this server"))
		return
	}

	networkID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Parse optional request body — ignore silently if the body is empty or
	// not JSON so that callers can POST without a body for the default behavior.
	var req networkScanRequest
	if r.ContentLength > 0 {
		if err := parseJSON(r, &req); err != nil {
			writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
			return
		}
	}

	nwe, err := h.service.GetNetworkByID(r.Context(), networkID)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "network", h.logger)
		return
	}
	network := nwe.Network
	cidr := network.CIDR.String()

	const maxScanTargets = 500
	hosts, _, err := h.hostService.ListHosts(r.Context(), &db.HostFilters{
		Status:  "up",
		Network: cidr,
	}, 0, maxScanTargets)
	if err != nil {
		h.logger.Error("Failed to list hosts for network scan",
			"request_id", requestID, "network_id", networkID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to list hosts: %w", err))
		return
	}

	if len(hosts) == 0 {
		writeError(w, r, http.StatusBadRequest,
			fmt.Errorf("no active hosts found in network %s", cidr))
		return
	}

	targets := make([]string, len(hosts))
	for i, host := range hosts {
		targets[i] = host.IPAddress.String()
	}

	scanName := fmt.Sprintf("Network scan: %s (%d hosts)", network.Name, len(targets))
	scan, err := h.scanService.CreateScan(r.Context(), db.CreateScanInput{
		Name:        scanName,
		Targets:     targets,
		ScanType:    "connect",
		Ports:       "1-1024",
		OSDetection: req.OSDetection,
	})
	if err != nil {
		h.logger.Error("Failed to create network scan",
			"request_id", requestID, "network_id", networkID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to create scan: %w", err))
		return
	}

	h.logger.Info("Network scan created",
		"request_id", requestID,
		"network_id", networkID,
		"scan_id", scan.ID,
		"target_count", len(targets),
		"os_detection", req.OSDetection)

	type networkScanResponse struct {
		ID      string   `json:"id"`
		Name    string   `json:"name"`
		Targets []string `json:"targets"`
		Status  string   `json:"status"`
	}
	writeJSON(w, r, http.StatusCreated, networkScanResponse{
		ID:      scan.ID.String(),
		Name:    scan.Name,
		Targets: scan.Targets,
		Status:  scan.Status,
	})

	recordCRUDMetric(h.metrics, "api_network_scan_created_total", nil)
}

func (h *NetworkHandler) DeleteExclusion(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	if err := h.service.RemoveExclusion(r.Context(), id); err != nil {
		handleDatabaseError(w, r, err, "delete", "exclusion", h.logger)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	recordCRUDMetric(h.metrics, "exclusions_deleted", nil)
}
