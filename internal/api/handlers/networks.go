package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/internal/services"
)

// NetworkHandler handles network-related API requests
type NetworkHandler struct {
	*BaseHandler
	service *services.NetworkService
}

// NewNetworkHandler creates a new network handler
func NewNetworkHandler(database *db.DB, logger *slog.Logger, metricsRegistry metrics.MetricsRegistry) *NetworkHandler {
	return &NetworkHandler{
		BaseHandler: NewBaseHandler(logger, metricsRegistry),
		service:     services.NewNetworkService(database),
	}
}

// CreateNetworkRequest represents the request body for creating a network
type CreateNetworkRequest struct {
	Name            string  `json:"name" validate:"required,min=1,max=100"`
	CIDR            string  `json:"cidr" validate:"required,cidr"`
	Description     *string `json:"description,omitempty"`
	DiscoveryMethod string  `json:"discovery_method" validate:"required,oneof=ping tcp arp"`
	IsActive        *bool   `json:"is_active,omitempty"`
	ScanEnabled     *bool   `json:"scan_enabled,omitempty"`
}

type UpdateNetworkRequest struct {
	Name            *string `json:"name,omitempty" validate:"omitempty,min=1,max=100"`
	CIDR            *string `json:"cidr,omitempty" validate:"omitempty,cidr"`
	Description     *string `json:"description,omitempty"`
	DiscoveryMethod *string `json:"discovery_method,omitempty" validate:"omitempty,oneof=ping tcp arp"`
	IsActive        *bool   `json:"is_active,omitempty"`
	ScanEnabled     *bool   `json:"scan_enabled,omitempty"`
}

type RenameNetworkRequest struct {
	NewName string `json:"new_name" validate:"required,min=1,max=100"`
}

type CreateExclusionRequest struct {
	ExcludedCIDR string  `json:"excluded_cidr" validate:"required,cidr"`
	Reason       *string `json:"reason,omitempty"`
}

type NetworkStatsResponse struct {
	Networks   map[string]interface{} `json:"networks"`
	Hosts      map[string]interface{} `json:"hosts"`
	Exclusions map[string]interface{} `json:"exclusions"`
}

// Helper functions

func (h *NetworkHandler) parseNetworkFilters(r *http.Request) (showInactive bool, nameFilter string) {
	showInactive = r.URL.Query().Get("show_inactive") == "true"
	nameFilter = r.URL.Query().Get("name")
	return showInactive, nameFilter
}

func (h *NetworkHandler) applyNetworkFilters(networks []*db.Network,
	showInactive bool, nameFilter string) []*db.Network {
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

func (h *NetworkHandler) parseCreateNetworkRequest(r *http.Request) (*CreateNetworkRequest, error) {
	var req CreateNetworkRequest
	if err := parseJSON(r, &req); err != nil {
		return nil, fmt.Errorf("invalid request body: %w", err)
	}
	return &req, nil
}

func (h *NetworkHandler) setNetworkDefaults(req *CreateNetworkRequest) (isActive bool, scanEnabled bool) {
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

// API Handlers

// ListNetworks handles GET /api/v1/networks
func (h *NetworkHandler) ListNetworks(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Listing networks", "request_id", requestID)

	params, err := getPaginationParams(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	showInactive, nameFilter := h.parseNetworkFilters(r)

	networks, err := h.service.ListNetworks(r.Context(), false) // Get all networks
	if err != nil {
		handleDatabaseError(w, r, err, "list", "networks", h.logger)
		return
	}

	filteredNetworks := h.applyNetworkFilters(networks, showInactive, nameFilter)

	// Apply pagination
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

// CreateNetwork handles POST /api/v1/networks
func (h *NetworkHandler) CreateNetwork(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Creating network", "request_id", requestID)

	req, err := h.parseCreateNetworkRequest(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	isActive, _ := h.setNetworkDefaults(req)

	description := ""
	if req.Description != nil {
		description = *req.Description
	}

	network, err := h.service.CreateNetwork(r.Context(), req.Name, req.CIDR, description, req.DiscoveryMethod, isActive)
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

// GetNetwork handles GET /api/v1/networks/{id}
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

	network := h.convertNetworkWithExclusionsToNetwork(networkWithExclusions)
	writeJSON(w, r, http.StatusOK, network)
	recordCRUDMetric(h.metrics, "networks_retrieved", nil)
}

// UpdateNetwork handles PUT /api/v1/networks/{id}
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

	// Get existing network
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

// DeleteNetwork handles DELETE /api/v1/networks/{id}
func (h *NetworkHandler) DeleteNetwork(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	err = h.service.DeleteNetwork(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "delete", "network", h.logger)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	recordCRUDMetric(h.metrics, "networks_deleted", map[string]string{"status": "success"})
}

// EnableNetwork handles POST /api/v1/networks/{id}/enable
func (h *NetworkHandler) EnableNetwork(w http.ResponseWriter, r *http.Request) {
	h.updateNetworkStatus(w, r, true, true, "enable")
}

// DisableNetwork handles POST /api/v1/networks/{id}/disable
func (h *NetworkHandler) DisableNetwork(w http.ResponseWriter, r *http.Request) {
	h.updateNetworkStatus(w, r, false, false, "disable")
}

// RenameNetwork handles PUT /api/v1/networks/{id}/rename
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
			writeError(w, r, http.StatusConflict, fmt.Errorf("network name '%s' already exists", req.NewName))
			return
		}
		handleDatabaseError(w, r, err, "rename", "network", h.logger)
		return
	}

	writeJSON(w, r, http.StatusOK, updatedNetwork)
	recordCRUDMetric(h.metrics, "networks_renamed", nil)
}

// GetNetworkStats handles GET /api/v1/networks/stats
func (h *NetworkHandler) GetNetworkStats(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Getting network statistics", "request_id", requestID)

	stats, err := h.service.GetNetworkStats(r.Context())
	if err != nil {
		handleDatabaseError(w, r, err, "get", "network stats", h.logger)
		return
	}

	response := NetworkStatsResponse{
		Networks:   stats["networks"].(map[string]interface{}),
		Hosts:      stats["hosts"].(map[string]interface{}),
		Exclusions: stats["exclusions"].(map[string]interface{}),
	}

	writeJSON(w, r, http.StatusOK, response)
	recordCRUDMetric(h.metrics, "network_stats_retrieved", nil)
}

// ListNetworkExclusions handles GET /api/v1/networks/{id}/exclusions
func (h *NetworkHandler) ListNetworkExclusions(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Verify network exists
	_, err = h.service.GetNetworkByID(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "network", h.logger)
		return
	}

	// Note: This would need a GetNetworkExclusions method in the service
	exclusions := []*db.NetworkExclusion{}
	writeJSON(w, r, http.StatusOK, exclusions)
	recordCRUDMetric(h.metrics, "network_exclusions_listed", nil)
}

// CreateNetworkExclusion handles POST /api/v1/networks/{id}/exclusions
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

	// Verify network exists
	_, err = h.service.GetNetworkByID(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "network", h.logger)
		return
	}

	reason := h.parseReasonFromRequest(&req)
	exclusion, err := h.service.AddExclusion(r.Context(), &id, req.ExcludedCIDR, reason)
	if err != nil {
		handleDatabaseError(w, r, err, "create", "exclusion", h.logger)
		return
	}

	writeJSON(w, r, http.StatusCreated, exclusion)
	recordCRUDMetric(h.metrics, "network_exclusions_created", nil)
}

// ListGlobalExclusions handles GET /api/v1/exclusions
func (h *NetworkHandler) ListGlobalExclusions(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Listing global exclusions", "request_id", requestID)

	exclusions, err := h.service.GetGlobalExclusions(r.Context())
	if err != nil {
		handleDatabaseError(w, r, err, "list", "global exclusions", h.logger)
		return
	}

	writeJSON(w, r, http.StatusOK, exclusions)
	recordCRUDMetric(h.metrics, "global_exclusions_listed", nil)
}

// CreateGlobalExclusion handles POST /api/v1/exclusions
func (h *NetworkHandler) CreateGlobalExclusion(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Creating global exclusion", "request_id", requestID)

	var req CreateExclusionRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
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

// DeleteExclusion handles DELETE /api/v1/exclusions/{id}
func (h *NetworkHandler) DeleteExclusion(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	err = h.service.RemoveExclusion(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "delete", "exclusion", h.logger)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	recordCRUDMetric(h.metrics, "exclusions_deleted", map[string]string{"status": "success"})
}

// Helper method for enable/disable operations
func (h *NetworkHandler) updateNetworkStatus(w http.ResponseWriter, r *http.Request,
	isActive, scanEnabled bool, operation string) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info(fmt.Sprintf("Network %s operation", operation), "request_id", requestID, "network_id", id)

	networkWithExclusions, err := h.service.GetNetworkByID(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "network", h.logger)
		return
	}

	network := h.convertNetworkWithExclusionsToNetwork(networkWithExclusions)
	h.updateNetworkStatusFields(network, isActive, scanEnabled)

	description := ""
	if network.Description != nil {
		description = *network.Description
	}

	updatedNetwork, err := h.service.UpdateNetwork(r.Context(), id, network.Name,
		network.CIDR.String(), description, network.DiscoveryMethod, isActive)
	if err != nil {
		handleDatabaseError(w, r, err, operation, "network", h.logger)
		return
	}

	h.logger.Info(fmt.Sprintf("Network %s successfully", operation), "request_id", requestID, "network_id", id)
	writeJSON(w, r, http.StatusOK, updatedNetwork)
	recordCRUDMetric(h.metrics, fmt.Sprintf("networks_%sd", operation), nil)
}
