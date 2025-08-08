// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements discovery job management endpoints including CRUD operations,
// discovery execution control, and results retrieval.
package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

// Validation constants.
const (
	maxDiscoveryNameLength = 255
	maxDescriptionLength   = 255
	maxPortsStringLength   = 50
	maxValidationErrors    = 100
	maxRetries             = 10
	maxTagLength           = 50
)

// DiscoveryHandler handles discovery-related API endpoints.
type DiscoveryHandler struct {
	database *db.DB
	logger   *slog.Logger
	metrics  *metrics.Registry
}

// NewDiscoveryHandler creates a new discovery handler.
func NewDiscoveryHandler(database *db.DB, logger *slog.Logger, metricsManager *metrics.Registry) *DiscoveryHandler {
	return &DiscoveryHandler{
		database: database,
		logger:   logger.With("handler", "discovery"),
		metrics:  metricsManager,
	}
}

// DiscoveryRequest represents a discovery job creation/update request.
type DiscoveryRequest struct {
	Name        string            `json:"name" validate:"required,min=1,max=255"`
	Description string            `json:"description,omitempty"`
	Networks    []string          `json:"networks" validate:"required,min=1"`
	Method      string            `json:"method" validate:"required,oneof=ping arp icmp tcp_connect"`
	Ports       string            `json:"ports,omitempty"`
	Timeout     time.Duration     `json:"timeout,omitempty"`
	Retries     int               `json:"retries,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
	ScheduleID  *int64            `json:"schedule_id,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Enabled     bool              `json:"enabled"`
}

// DiscoveryResponse represents a discovery job response.
type DiscoveryResponse struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Networks    []string          `json:"networks"`
	Method      string            `json:"method"`
	Ports       string            `json:"ports,omitempty"`
	Timeout     time.Duration     `json:"timeout,omitempty"`
	Retries     int               `json:"retries,omitempty"`
	Options     map[string]string `json:"options,omitempty"`
	ScheduleID  *int64            `json:"schedule_id,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Enabled     bool              `json:"enabled"`
	Status      string            `json:"status"`
	Progress    float64           `json:"progress"`
	HostsFound  int               `json:"hosts_found"`
	LastRun     *time.Time        `json:"last_run,omitempty"`
	NextRun     *time.Time        `json:"next_run,omitempty"`
	RunCount    int               `json:"run_count"`
	ErrorCount  int               `json:"error_count"`
	LastError   string            `json:"last_error,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	CreatedBy   string            `json:"created_by,omitempty"`
}

// DiscoveryResultsResponse represents discovery results.
type DiscoveryResultsResponse struct {
	JobID       int64                  `json:"job_id"`
	TotalHosts  int                    `json:"total_hosts"`
	NewHosts    int                    `json:"new_hosts"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Results     []DiscoveryResult      `json:"results"`
	Summary     map[string]interface{} `json:"summary"`
	GeneratedAt time.Time              `json:"generated_at"`
}

// DiscoveryResult represents an individual discovery result.
type DiscoveryResult struct {
	ID           int64     `json:"id"`
	HostIP       string    `json:"host_ip"`
	Hostname     string    `json:"hostname,omitempty"`
	MACAddress   string    `json:"mac_address,omitempty"`
	ResponseTime float64   `json:"response_time_ms"`
	Method       string    `json:"method"`
	IsNew        bool      `json:"is_new"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
}

// ListDiscoveryJobs handles GET /api/v1/discovery - list discovery jobs with filtering and pagination.
func (h *DiscoveryHandler) ListDiscoveryJobs(w http.ResponseWriter, r *http.Request) {
	listOp := &ListOperation[*db.DiscoveryJob, db.DiscoveryFilters]{
		EntityType: "discovery jobs",
		MetricName: "api_discovery_jobs_listed_total",
		Logger:     h.logger,
		Metrics:    h.metrics,
		GetFilters: h.getDiscoveryFilters,
		ListFromDB: h.database.ListDiscoveryJobs,
		ToResponse: func(job *db.DiscoveryJob) interface{} {
			return h.discoveryToResponse(job)
		},
	}
	listOp.Execute(w, r)
}

// CreateDiscoveryJob handles POST /api/v1/discovery - create a new discovery job.
func (h *DiscoveryHandler) CreateDiscoveryJob(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())
	h.logger.Info("Creating discovery job", "request_id", requestID)

	// Parse request body
	var req DiscoveryRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Validate request
	if err := h.validateDiscoveryRequest(&req); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Create discovery job in database
	job, err := h.database.CreateDiscoveryJob(r.Context(), h.requestToDBDiscovery(&req))
	if err != nil {
		h.logger.Error("Failed to create discovery job", "request_id", requestID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to create discovery job: %w", err))
		return
	}

	response := h.discoveryToResponse(job)

	h.logger.Info("Discovery job created successfully",
		"request_id", requestID,
		"job_id", response.ID,
		"job_name", response.Name)

	writeJSON(w, r, http.StatusCreated, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_discovery_jobs_created_total", nil)
	}
}

// GetDiscoveryJob handles GET /api/v1/discovery/{id} - get a discovery job.
func (h *DiscoveryHandler) GetDiscoveryJob(w http.ResponseWriter, r *http.Request) {
	jobID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	crudOp := &CRUDOperation[db.DiscoveryJob]{
		EntityType: "discovery job",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	crudOp.ExecuteGet(w, r, jobID,
		h.database.GetDiscoveryJob,
		func(job *db.DiscoveryJob) interface{} {
			return h.discoveryToResponse(job)
		},
		"api_discovery_jobs_retrieved_total")
}

func (h *DiscoveryHandler) UpdateDiscoveryJob(w http.ResponseWriter, r *http.Request) {
	UpdateEntity[db.DiscoveryJob, DiscoveryRequest](
		w, r,
		"discovery job",
		h.logger,
		h.metrics,
		func(r *http.Request) (interface{}, error) {
			var req DiscoveryRequest
			if err := parseJSON(r, &req); err != nil {
				return nil, err
			}
			return h.requestToDBDiscovery(&req), nil
		},
		h.database.UpdateDiscoveryJob,
		func(job *db.DiscoveryJob) interface{} {
			return h.discoveryToResponse(job)
		},
		"api_discovery_jobs_updated_total")
}

// DeleteDiscoveryJob handles DELETE /api/v1/discovery/{id} - delete a discovery job.
func (h *DiscoveryHandler) DeleteDiscoveryJob(w http.ResponseWriter, r *http.Request) {
	jobID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	crudOp := &CRUDOperation[db.DiscoveryJob]{
		EntityType: "discovery job",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	crudOp.ExecuteDelete(w, r, jobID, h.database.DeleteDiscoveryJob, "api_discovery_jobs_deleted_total")
}

// StartDiscovery handles POST /api/v1/discovery/{id}/start - start a discovery job.
func (h *DiscoveryHandler) StartDiscovery(w http.ResponseWriter, r *http.Request) {
	jobID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	jobOp := &JobControlOperation{
		EntityType: "discovery job",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	jobOp.ExecuteStart(w, r, jobID, h.database.StartDiscoveryJob,
		func(ctx context.Context, id uuid.UUID) (interface{}, error) {
			return h.database.GetDiscoveryJob(ctx, id)
		},
		func(item interface{}) interface{} {
			if job, ok := item.(*db.DiscoveryJob); ok {
				return h.discoveryToResponse(job)
			}
			return nil
		}, "api_discovery_jobs_started_total")
}

// StopDiscovery handles POST /api/v1/discovery/{id}/stop - stop a running discovery job.
func (h *DiscoveryHandler) StopDiscovery(w http.ResponseWriter, r *http.Request) {
	jobID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	jobOp := &JobControlOperation{
		EntityType: "discovery job",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	jobOp.ExecuteStop(w, r, jobID, h.database.StopDiscoveryJob, "api_discovery_jobs_stopped_total")
}

// Helper methods

// Helper methods

// validateDiscoveryRequest validates a discovery request.
func (h *DiscoveryHandler) validateDiscoveryRequest(req *DiscoveryRequest) error {
	if err := h.validateBasicFields(req); err != nil {
		return err
	}
	if err := h.validateMethod(req.Method); err != nil {
		return err
	}
	if err := h.validateNetworks(req.Networks); err != nil {
		return err
	}
	if err := h.validateLimits(req.Timeout, req.Retries); err != nil {
		return err
	}
	return h.validateTags(req.Tags)
}

func (h *DiscoveryHandler) validateBasicFields(req *DiscoveryRequest) error {
	if req.Name == "" {
		return fmt.Errorf("discovery job name is required")
	}
	if len(req.Name) > maxDiscoveryNameLength {
		return fmt.Errorf("discovery job name too long (max %d characters)", maxDiscoveryNameLength)
	}
	if len(req.Networks) == 0 {
		return fmt.Errorf("at least one network is required")
	}
	return nil
}

func (h *DiscoveryHandler) validateMethod(method string) error {
	validMethods := map[string]bool{
		"ping":        true,
		"arp":         true,
		"icmp":        true,
		"tcp_connect": true,
	}
	if !validMethods[method] {
		return fmt.Errorf("invalid discovery method: %s", method)
	}
	return nil
}

func (h *DiscoveryHandler) validateNetworks(networks []string) error {
	for i, network := range networks {
		if network == "" {
			return fmt.Errorf("network %d is empty", i+1)
		}
		// Try to parse as CIDR first
		if _, _, err := net.ParseCIDR(network); err != nil {
			// Try to parse as single IP
			if net.ParseIP(network) == nil {
				return fmt.Errorf("network %d has invalid format: %s", i+1, network)
			}
		}
		if len(network) > maxDescriptionLength {
			return fmt.Errorf("network %d too long (max %d characters)", i+1, maxDescriptionLength)
		}
	}
	return nil
}

func (h *DiscoveryHandler) validateLimits(timeout time.Duration, retries int) error {
	if timeout < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}
	if retries < 0 {
		return fmt.Errorf("retries cannot be negative")
	}
	if retries > maxRetries {
		return fmt.Errorf("too many retries (max %d)", maxRetries)
	}
	return nil
}

func (h *DiscoveryHandler) validateTags(tags []string) error {
	for i, tag := range tags {
		if tag == "" {
			return fmt.Errorf("tag %d is empty", i+1)
		}
		if len(tag) > maxTagLength {
			return fmt.Errorf("tag %d too long (max %d characters)", i+1, maxTagLength)
		}
	}
	return nil
}

// getDiscoveryFilters extracts filter parameters from request.
func (h *DiscoveryHandler) getDiscoveryFilters(r *http.Request) db.DiscoveryFilters {
	filters := db.DiscoveryFilters{}

	if status := r.URL.Query().Get("status"); status != "" {
		filters.Status = status
	}

	if method := r.URL.Query().Get("method"); method != "" {
		filters.Method = method
	}

	return filters
}

// requestToDBDiscovery converts a discovery request to database discovery object.
func (h *DiscoveryHandler) requestToDBDiscovery(req *DiscoveryRequest) interface{} {
	// This should return the appropriate database discovery type
	// The exact structure would depend on the database package implementation
	return map[string]interface{}{
		"name":        req.Name,
		"description": req.Description,
		"networks":    req.Networks,
		"method":      req.Method,
		"ports":       req.Ports,
		"timeout":     req.Timeout,
		"retries":     req.Retries,
		"options":     req.Options,
		"schedule_id": req.ScheduleID,
		"tags":        req.Tags,
		"enabled":     req.Enabled,
		"status":      "pending",
		"created_at":  time.Now().UTC(),
		"updated_at":  time.Now().UTC(),
	}
}

// discoveryToResponse converts a database discovery job to response format.
func (h *DiscoveryHandler) discoveryToResponse(_ interface{}) DiscoveryResponse {
	// This would convert from the actual database discovery type
	// For now, return a placeholder structure
	return DiscoveryResponse{
		ID:          1,                // job.ID
		Name:        "",               // job.Name
		Description: "",               // job.Description
		Networks:    []string{},       // job.Networks
		Method:      "ping",           // job.Method
		Enabled:     true,             // job.Enabled
		Status:      "pending",        // job.Status
		Progress:    0.0,              // job.Progress
		HostsFound:  0,                // job.HostsFound
		RunCount:    0,                // job.RunCount
		ErrorCount:  0,                // job.ErrorCount
		CreatedAt:   time.Now().UTC(), // job.CreatedAt
		UpdatedAt:   time.Now().UTC(), // job.UpdatedAt
	}
}
