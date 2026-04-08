// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements discovery job management endpoints including CRUD operations,
// discovery execution control, and results retrieval.
package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/discovery"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/internal/scanning"
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

// Progress estimation constants.
// These mirror the engine's timeout formula so the handler can derive a
// realistic expected duration from the network size alone, without needing a
// separate DB column.
const (
	progressMinTimeout            = 30 * time.Second
	progressMaxTimeout            = 300 * time.Second
	progressBaseTimeout           = 60 * time.Second
	progressTimeoutMultiplierBase = 6.0
	progressTimeoutMultiplierStep = 2.0
	progressTimeoutMultiplierMax  = 50.0
	progressTimeoutDivisor        = 100.0
	// Cap displayed progress at 99% while running so it never shows 100 before
	// the status actually flips to "completed".
	progressRunningCap = 99.0
	// progressPercentScale converts a 0–1 fraction to a 0–100 percentage.
	progressPercentScale = 100.0

	// countUsableHosts constants — mirror the engine's generateTargetsFromCIDR rules.
	maxHostBits     = 24    // host-bits above this are clamped to maxUsableHosts
	maxUsableHosts  = 10000 // matches the engine's default maxHosts cap
	minPointToPoint = 31    // /31 and /32 are treated as point-to-point / single-host
)

// DiscoveryHandler handles discovery-related API endpoints.
type DiscoveryHandler struct {
	database  DiscoveryStore
	logger    *slog.Logger
	metrics   *metrics.Registry
	engine    *discovery.Engine
	scanQueue *scanning.ScanQueue
	wsHandler *WebSocketHandler

	cancelsMu sync.Mutex
	cancels   map[uuid.UUID]context.CancelFunc
}

// NewDiscoveryHandler creates a new discovery handler.
func NewDiscoveryHandler(
	database DiscoveryStore, logger *slog.Logger, metricsManager *metrics.Registry,
) *DiscoveryHandler {
	return &DiscoveryHandler{
		database: database,
		logger:   logger.With("handler", "discovery"),
		metrics:  metricsManager,
		cancels:  make(map[uuid.UUID]context.CancelFunc),
	}
}

// WithEngine sets the discovery engine used to actually execute nmap scans when
// a job is started via the API. Without an engine, StartDiscovery only flips
// the DB status row and no scan is performed.
func (h *DiscoveryHandler) WithEngine(e *discovery.Engine) *DiscoveryHandler {
	h.engine = e
	return h
}

// WithScanQueue sets the worker pool used to execute discovery jobs.
// When set, StartDiscovery submits a discoveryJob to the queue instead of
// spawning an unbounded goroutine.
func (h *DiscoveryHandler) WithScanQueue(q *scanning.ScanQueue) *DiscoveryHandler {
	h.scanQueue = q
	return h
}

// WithWebSocket attaches a WebSocketHandler so that completed discovery jobs
// broadcast a diff summary to all connected discovery clients.
func (h *DiscoveryHandler) WithWebSocket(ws *WebSocketHandler) *DiscoveryHandler {
	h.wsHandler = ws
	return h
}

// discoveryJob implements scanning.Job for nmap host-discovery operations.
// It is created by StartDiscovery and submitted to the shared ScanQueue so
// that discovery work appears alongside scan work in the admin worker view.
type discoveryJob struct {
	id        string
	jobID     uuid.UUID
	network   string
	method    string
	engine    *discovery.Engine
	database  DiscoveryStore
	logger    *slog.Logger
	wsHandler *WebSocketHandler
	cancelCtx context.Context // cancelled by StopDiscovery for per-job stop
	cleanup   func()          // removes the cancel func from the handler's map
}

// ID implements scanning.Job.
func (j *discoveryJob) ID() string { return j.id }

// Type implements scanning.Job.
func (j *discoveryJob) Type() string { return "discovery" }

// Target implements scanning.Job.
func (j *discoveryJob) Target() string { return j.network }

// Execute implements scanning.Job. It runs the nmap discovery scan, updates
// the DB on completion or failure, and calls cleanup when done.
func (j *discoveryJob) Execute(workerCtx context.Context) error {
	defer j.cleanup()

	// Derive a context that honors both worker shutdown and per-job stop.
	ctx, cancel := context.WithCancel(workerCtx)
	defer cancel()
	stop := context.AfterFunc(j.cancelCtx, cancel)
	defer stop()

	cfg := &discovery.Config{
		Network:  j.network,
		Method:   j.method,
		MaxHosts: 10000,
	}

	j.logger.Info("Executing discovery",
		"job_id", j.jobID, "network", cfg.Network, "method", cfg.Method)

	hostsFound, err := j.engine.ScanNetwork(ctx, cfg)

	dbCtx := context.Background()
	if err != nil {
		j.logger.Error("Discovery execution failed", "job_id", j.jobID, "error", err)
		if stopErr := j.database.StopDiscoveryJob(dbCtx, j.jobID); stopErr != nil {
			j.logger.Error("Failed to mark discovery job as failed",
				"job_id", j.jobID, "error", stopErr)
		}
		return err
	}

	completed := db.DiscoveryJobStatusCompleted
	now := time.Now().UTC()
	if _, updateErr := j.database.UpdateDiscoveryJob(dbCtx, j.jobID, db.UpdateDiscoveryJobInput{
		Status:          &completed,
		CompletedAt:     &now,
		HostsDiscovered: &hostsFound,
		HostsResponsive: &hostsFound,
	}); updateErr != nil {
		j.logger.Error("Failed to mark discovery job as completed",
			"job_id", j.jobID, "error", updateErr)
		return updateErr
	}

	if j.wsHandler != nil {
		if diff, dErr := j.database.GetDiscoveryDiff(dbCtx, j.jobID); dErr == nil {
			_ = j.wsHandler.BroadcastDiscoveryUpdate(&DiscoveryUpdateMessage{
				JobID:        j.jobID.String(),
				Status:       "completed",
				HostsFound:   hostsFound,
				NewHosts:     len(diff.NewHosts),
				GoneHosts:    len(diff.GoneHosts),
				ChangedHosts: len(diff.ChangedHosts),
			})
		}
	}

	j.logger.Info("Discovery job finished", "job_id", j.jobID, "hosts_found", hostsFound)
	return nil
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
	NetworkID   *uuid.UUID        `json:"network_id,omitempty"`
}

// DiscoveryResponse represents a discovery job response.
type DiscoveryResponse struct {
	ID          uuid.UUID         `json:"id"`
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
	StartedAt   *time.Time        `json:"started_at,omitempty"`
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
	job, err := h.database.CreateDiscoveryJob(r.Context(), h.requestToCreateDiscovery(&req))
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

// UpdateDiscoveryJob handles PUT /api/v1/discovery/{id} - update an existing discovery job.
func (h *DiscoveryHandler) UpdateDiscoveryJob(w http.ResponseWriter, r *http.Request) {
	UpdateEntity[db.DiscoveryJob, db.UpdateDiscoveryJobInput](
		w, r,
		"discovery job",
		h.logger,
		h.metrics,
		func(r *http.Request) (db.UpdateDiscoveryJobInput, error) {
			var req DiscoveryRequest
			if err := parseJSON(r, &req); err != nil {
				return db.UpdateDiscoveryJobInput{}, err
			}
			return h.requestToUpdateDiscovery(&req), nil
		},
		h.database.UpdateDiscoveryJob,
		func(job *db.DiscoveryJob) interface{} {
			return h.discoveryToResponse(job)
		},
		"api_discovery_jobs_updated_total")
}

// GetDiscoveryDiff handles GET /api/v1/discovery/{id}/diff
func (h *DiscoveryHandler) GetDiscoveryDiff(w http.ResponseWriter, r *http.Request) {
	jobID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	diff, err := h.database.GetDiscoveryDiff(r.Context(), jobID)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "discovery diff", h.logger)
		return
	}
	writeJSON(w, r, http.StatusOK, diff)
}

// GetDiscoveryCompare handles GET /api/v1/discovery/compare?run_a={id}&run_b={id}
// It compares two discovery runs and returns which hosts are new, gone, or changed.
func (h *DiscoveryHandler) GetDiscoveryCompare(w http.ResponseWriter, r *http.Request) {
	runAStr := r.URL.Query().Get("run_a")
	runBStr := r.URL.Query().Get("run_b")
	if runAStr == "" || runBStr == "" {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("run_a and run_b query parameters are required"))
		return
	}
	runA, err := uuid.Parse(runAStr)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid run_a: must be a valid UUID"))
		return
	}
	runB, err := uuid.Parse(runBStr)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid run_b: must be a valid UUID"))
		return
	}
	diff, err := h.database.CompareDiscoveryRuns(r.Context(), runA, runB)
	if err != nil {
		if strings.Contains(err.Error(), "cannot compare runs on different networks") {
			writeError(w, r, http.StatusUnprocessableEntity, err)
			return
		}
		handleDatabaseError(w, r, err, "compare", "discovery runs", h.logger)
		return
	}
	writeJSON(w, r, http.StatusOK, diff)
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
	requestID := getRequestIDFromContext(r.Context())

	jobID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Load the job so we can read its network/method and check its current status.
	job, err := h.database.GetDiscoveryJob(r.Context(), jobID)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "discovery job", h.logger)
		return
	}

	if job.Status == db.DiscoveryJobStatusRunning {
		writeError(w, r, http.StatusConflict, fmt.Errorf("discovery job is already running"))
		return
	}
	if job.Status == db.DiscoveryJobStatusCompleted {
		writeError(w, r, http.StatusConflict, fmt.Errorf("discovery job has already completed"))
		return
	}

	// Flip the DB row to "running" and record started_at.
	if err := h.database.StartDiscoveryJob(r.Context(), jobID); err != nil {
		handleDatabaseError(w, r, err, "start", "discovery job", h.logger)
		return
	}

	// If an engine is configured, launch the nmap scan in a goroutine.
	if h.engine != nil {
		ctx, cancel := context.WithCancel(context.Background())

		h.cancelsMu.Lock()
		h.cancels[jobID] = cancel
		h.cancelsMu.Unlock()

		djob := &discoveryJob{
			id:        jobID.String(),
			jobID:     jobID,
			network:   job.Network.String(),
			method:    job.Method,
			engine:    h.engine,
			database:  h.database,
			logger:    h.logger,
			wsHandler: h.wsHandler,
			cancelCtx: ctx,
			cleanup: func() {
				h.cancelsMu.Lock()
				delete(h.cancels, jobID)
				h.cancelsMu.Unlock()
			},
		}

		if err := h.submitDiscoveryJob(r.Context(), djob); err != nil {
			h.logger.Error("Failed to submit discovery job to queue",
				"request_id", requestID, "job_id", jobID, "error", err)
			cancel()
			writeError(w, r, http.StatusServiceUnavailable, err)
			return
		}
	} else {
		h.logger.Warn("No discovery engine configured — job status set to running but no scan will execute",
			"request_id", requestID, "job_id", jobID)
	}

	// Return the updated job.
	updated, err := h.database.GetDiscoveryJob(r.Context(), jobID)
	if err != nil {
		h.logger.Error("Failed to get discovery job after start",
			"request_id", requestID, "job_id", jobID, "error", err)
		writeError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to retrieve updated job"))
		return
	}

	writeJSON(w, r, http.StatusOK, h.discoveryToResponse(updated))

	if h.metrics != nil {
		h.metrics.Counter("api_discovery_jobs_started_total", nil)
	}

	h.logger.Info("Discovery job started", "request_id", requestID, "job_id", jobID)
}

// submitDiscoveryJob routes djob to the scan queue if one is configured, or
// falls back to a bare goroutine. On queue-submission failure it reverts the
// DB row and returns an error; the caller is responsible for canceling the
// per-job context and writing the HTTP response.
func (h *DiscoveryHandler) submitDiscoveryJob(ctx context.Context, djob *discoveryJob) error {
	if h.scanQueue == nil {
		// Fallback: no queue configured, run in an unbounded goroutine.
		go func() {
			if err := djob.Execute(djob.cancelCtx); err != nil {
				h.logger.Error("Discovery goroutine failed", "job_id", djob.jobID, "error", err)
			}
		}()
		return nil
	}
	if err := h.scanQueue.Submit(djob); err != nil {
		if stopErr := h.database.StopDiscoveryJob(ctx, djob.jobID); stopErr != nil {
			h.logger.Error("Failed to revert discovery job status", "job_id", djob.jobID, "error", stopErr)
		}
		return fmt.Errorf("job queue unavailable, please try again later")
	}
	return nil
}

// StopDiscovery handles POST /api/v1/discovery/{id}/stop - stop a running discovery job.
func (h *DiscoveryHandler) StopDiscovery(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())

	jobID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Cancel the running goroutine if one exists.
	h.cancelsMu.Lock()
	cancel, running := h.cancels[jobID]
	if running {
		cancel()
		delete(h.cancels, jobID)
	}
	h.cancelsMu.Unlock()

	// Update the DB row regardless of whether a goroutine was found — the job
	// might have been started in a previous process instance.
	if err := h.database.StopDiscoveryJob(r.Context(), jobID); err != nil {
		handleDatabaseError(w, r, err, "stop", "discovery job", h.logger)
		return
	}

	response := map[string]interface{}{
		"id":         jobID,
		"status":     "stopped",
		"message":    "discovery job has been stopped",
		"timestamp":  time.Now().UTC(),
		"request_id": requestID,
	}

	writeJSON(w, r, http.StatusOK, response)

	if h.metrics != nil {
		h.metrics.Counter("api_discovery_jobs_stopped_total", nil)
	}

	h.logger.Info("Discovery job stopped", "request_id", requestID, "job_id", jobID)
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

// requestToCreateDiscovery converts a DiscoveryRequest to a typed CreateDiscoveryJobInput for the DB layer.
func (h *DiscoveryHandler) requestToCreateDiscovery(req *DiscoveryRequest) db.CreateDiscoveryJobInput {
	return db.CreateDiscoveryJobInput{
		Networks:  req.Networks,
		Method:    req.Method,
		NetworkID: req.NetworkID,
	}
}

// requestToUpdateDiscovery converts a DiscoveryRequest to a typed UpdateDiscoveryJobInput for the DB layer.
// Only the Method field is propagated since that is the only field the DB update function honors
// for API-initiated updates.
func (h *DiscoveryHandler) requestToUpdateDiscovery(req *DiscoveryRequest) db.UpdateDiscoveryJobInput {
	input := db.UpdateDiscoveryJobInput{}
	if req.Method != "" {
		input.Method = &req.Method
	}
	return input
}

// discoveryToResponse converts a database discovery job to response format.
func (h *DiscoveryHandler) discoveryToResponse(job *db.DiscoveryJob) DiscoveryResponse {
	resp := DiscoveryResponse{
		ID:         job.ID,
		Networks:   []string{job.Network.String()},
		Method:     job.Method,
		Enabled:    job.Status != db.DiscoveryJobStatusFailed,
		Status:     job.Status,
		HostsFound: job.HostsDiscovered,
		CreatedAt:  job.CreatedAt,
		UpdatedAt:  job.CreatedAt, // No separate UpdatedAt in DB model, use CreatedAt
	}

	// Compute progress from status
	switch job.Status {
	case db.DiscoveryJobStatusCompleted:
		resp.Progress = 100.0
	case db.DiscoveryJobStatusRunning:
		resp.Progress = estimateDiscoveryProgress(job)
	default:
		resp.Progress = 0.0
	}

	if job.StartedAt != nil {
		resp.StartedAt = job.StartedAt
	}

	return resp
}

// estimateDiscoveryProgress returns a 0–99 progress percentage for a running
// job based on elapsed time vs the expected duration.
//
// Since nmap runs as a single blocking call with no intermediate progress
// events, and we have no progress column in the DB, we derive a synthetic
// estimate:
//
//  1. Count the usable host addresses in the stored CIDR (same logic the
//     engine uses to build its target list).
//  2. Apply the engine's timeout formula to get an expected duration.
//  3. Divide elapsed time by that expected duration, capped at 99 % so the
//     bar never reaches 100 % before the status flips to "completed".
func estimateDiscoveryProgress(job *db.DiscoveryJob) float64 {
	if job.StartedAt == nil {
		return 0.0
	}

	elapsed := time.Since(*job.StartedAt)
	if elapsed <= 0 {
		return 0.0
	}

	expected := estimateExpectedDuration(job.Network.IPNet)

	pct := (elapsed.Seconds() / expected.Seconds()) * progressPercentScale
	if pct > progressRunningCap {
		pct = progressRunningCap
	}
	// Round to one decimal place so the frontend gets a smooth-ish stream of
	// distinct values on every poll rather than long runs of the same integer.
	return math.Round(pct*10) / 10
}

// estimateExpectedDuration mirrors the engine's calculateDynamicTimeout logic
// to produce the expected wall-clock duration for a given network.
func estimateExpectedDuration(ipnet net.IPNet) time.Duration {
	targetCount := countUsableHosts(ipnet)

	multiplier := progressTimeoutMultiplierBase +
		(float64(targetCount)/progressTimeoutDivisor)*progressTimeoutMultiplierStep
	if multiplier > progressTimeoutMultiplierMax {
		multiplier = progressTimeoutMultiplierMax
	}

	d := time.Duration(float64(progressBaseTimeout) * multiplier)
	if d < progressMinTimeout {
		d = progressMinTimeout
	}
	if d > progressMaxTimeout {
		d = progressMaxTimeout
	}
	return d
}

// countUsableHosts returns the number of target IP addresses the engine would
// generate for the given network, using the same rules as generateTargetsFromCIDR.
func countUsableHosts(ipnet net.IPNet) int {
	ones, bits := ipnet.Mask.Size()
	if bits == 0 {
		return 1
	}

	// /32 — single host
	if ones == 32 {
		return 1
	}

	// /31 — RFC 3021 point-to-point, both addresses usable
	if ones == 31 && bits == 32 {
		return 2
	}

	hostBits := bits - ones
	if hostBits > maxHostBits {
		// Clamp to the engine's maxHosts default to avoid huge numbers.
		return maxUsableHosts
	}

	total := 1 << hostBits
	// Subtract network and broadcast for standard subnets.
	if ones < minPointToPoint {
		total -= 2
	}
	if total < 0 {
		total = 0
	}
	// Respect the engine's default maxHosts cap.
	if total > maxUsableHosts {
		total = maxUsableHosts
	}
	return total
}
