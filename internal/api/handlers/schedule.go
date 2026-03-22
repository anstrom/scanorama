// Package handlers provides HTTP request handlers for the Scanorama API.
// This file implements schedule management endpoints including CRUD operations
// and schedule activation/deactivation.
package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

// Schedule validation constants.
const (
	maxScheduleNameLength = 255
	maxScheduleDescLength = 1000
	maxScheduleTagLength  = 50
	maxScheduleRetries    = 10
	scheduleStatusActive  = "active"
)

// ScheduleHandler handles schedule-related API endpoints.
type ScheduleHandler struct {
	database ScheduleStore
	logger   *slog.Logger
	metrics  *metrics.Registry
}

// NewScheduleHandler creates a new schedule handler.
func NewScheduleHandler(
	database ScheduleStore, logger *slog.Logger, metricsManager *metrics.Registry,
) *ScheduleHandler {
	return &ScheduleHandler{
		database: database,
		logger:   logger.With("handler", "schedule"),
		metrics:  metricsManager,
	}
}

// ScheduleRequest represents a schedule creation/update request.
type ScheduleRequest struct {
	Name         string            `json:"name" validate:"required,min=1,max=255"`
	Description  string            `json:"description,omitempty"`
	CronExpr     string            `json:"cron_expr" validate:"required"`
	Type         string            `json:"type" validate:"required,oneof=scan discovery"`
	NetworkID    uuid.UUID         `json:"network_id" validate:"required"`
	Enabled      bool              `json:"enabled"`
	MaxRunTime   time.Duration     `json:"max_run_time,omitempty"`
	RetryOnError bool              `json:"retry_on_error"`
	MaxRetries   int               `json:"max_retries,omitempty"`
	RetryDelay   time.Duration     `json:"retry_delay,omitempty"`
	Options      map[string]string `json:"options,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	NotifyOnFail bool              `json:"notify_on_fail"`
	NotifyEmails []string          `json:"notify_emails,omitempty"`
}

// ScheduleResponse represents a schedule response.
type ScheduleResponse struct {
	ID           uuid.UUID         `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	CronExpr     string            `json:"cron_expr"`
	Type         string            `json:"type"`
	NetworkID    string            `json:"network_id,omitempty"`
	NetworkName  string            `json:"network_name,omitempty"`
	Enabled      bool              `json:"enabled"`
	MaxRunTime   time.Duration     `json:"max_run_time,omitempty"`
	RetryOnError bool              `json:"retry_on_error"`
	MaxRetries   int               `json:"max_retries,omitempty"`
	RetryDelay   time.Duration     `json:"retry_delay,omitempty"`
	Options      map[string]string `json:"options,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	NotifyOnFail bool              `json:"notify_on_fail"`
	NotifyEmails []string          `json:"notify_emails,omitempty"`
	Status       string            `json:"status"`
	LastRun      *time.Time        `json:"last_run,omitempty"`
	NextRun      *time.Time        `json:"next_run,omitempty"`
	RunCount     int               `json:"run_count"`
	SuccessCount int               `json:"success_count"`
	ErrorCount   int               `json:"error_count"`
	LastError    string            `json:"last_error,omitempty"`
	LastDuration *time.Duration    `json:"last_duration,omitempty"`
	AvgDuration  *time.Duration    `json:"avg_duration,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	CreatedBy    string            `json:"created_by,omitempty"`
}

// ListSchedules handles GET /api/v1/schedules - list all schedules with pagination.
func (h *ScheduleHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	listOp := &ListOperation[*db.Schedule, db.ScheduleFilters]{
		EntityType: "schedules",
		MetricName: "api_schedules_listed_total",
		Logger:     h.logger,
		Metrics:    h.metrics,
		GetFilters: h.getScheduleFilters,
		ListFromDB: h.database.ListSchedules,
		ToResponse: func(schedule *db.Schedule) interface{} {
			return h.scheduleToResponse(schedule)
		},
	}
	listOp.Execute(w, r)
}

// CreateSchedule handles POST /api/v1/schedules - create a new schedule.
func (h *ScheduleHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	CreateEntity[db.Schedule, db.CreateScheduleInput](
		w, r,
		"schedule",
		h.logger,
		h.metrics,
		func(r *http.Request) (db.CreateScheduleInput, error) {
			var req ScheduleRequest
			if err := parseJSON(r, &req); err != nil {
				return db.CreateScheduleInput{}, err
			}
			if err := h.validateScheduleRequest(&req); err != nil {
				return db.CreateScheduleInput{}, err
			}
			return h.requestToCreateSchedule(&req), nil
		},
		h.database.CreateSchedule,
		func(schedule *db.Schedule) interface{} {
			return h.scheduleToResponse(schedule)
		},
		"api_schedules_created_total")
}

// GetSchedule handles GET /api/v1/schedules/{id} - get a specific schedule.
func (h *ScheduleHandler) GetSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	crudOp := &CRUDOperation[db.Schedule]{
		EntityType: "schedule",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	crudOp.ExecuteGet(w, r, scheduleID,
		h.database.GetSchedule,
		func(schedule *db.Schedule) interface{} {
			return h.scheduleToResponse(schedule)
		},
		"api_schedules_retrieved_total")
}

// UpdateSchedule handles PUT /api/v1/schedules/{id} - update a schedule.
func (h *ScheduleHandler) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	UpdateEntity[db.Schedule, db.UpdateScheduleInput](
		w, r,
		"schedule",
		h.logger,
		h.metrics,
		func(r *http.Request) (db.UpdateScheduleInput, error) {
			var req ScheduleRequest
			if err := parseJSON(r, &req); err != nil {
				return db.UpdateScheduleInput{}, err
			}
			if err := h.validateScheduleRequest(&req); err != nil {
				return db.UpdateScheduleInput{}, err
			}
			return h.requestToUpdateSchedule(&req), nil
		},
		h.database.UpdateSchedule,
		func(schedule *db.Schedule) interface{} {
			return h.scheduleToResponse(schedule)
		},
		"api_schedules_updated_total")
}

// DeleteSchedule handles DELETE /api/v1/schedules/{id} - delete a schedule.
func (h *ScheduleHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	crudOp := &CRUDOperation[db.Schedule]{
		EntityType: "schedule",
		Logger:     h.logger,
		Metrics:    h.metrics,
	}

	crudOp.ExecuteDelete(w, r, scheduleID, h.database.DeleteSchedule, "api_schedules_deleted_total")
}

// EnableSchedule handles POST /api/v1/schedules/{id}/enable - enable a schedule.
func (h *ScheduleHandler) EnableSchedule(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())

	// Extract schedule ID from URL
	scheduleID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	h.logger.Info("Enabling schedule", "request_id", requestID, "schedule_id", scheduleID)

	// Enable schedule in database
	err = h.database.EnableSchedule(r.Context(), scheduleID)
	if err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound,
				fmt.Errorf("schedule not found"))
			return
		}
		h.logger.Error("Failed to activate schedule", "request_id", requestID, "schedule_id", scheduleID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to activate schedule: %w", err))
		return
	}

	// Get updated schedule
	schedule, err := h.database.GetSchedule(r.Context(), scheduleID)
	if err != nil {
		h.logger.Error("Failed to get schedule after enable",
			"request_id", requestID, "schedule_id", scheduleID, "error", err)
		// Still return success since the schedule was enabled
	}

	response := map[string]interface{}{
		"schedule_id": scheduleID,
		"status":      "enabled",
		"message":     "Schedule has been enabled",
		"timestamp":   time.Now().UTC(),
		"request_id":  requestID,
	}

	if schedule != nil {
		response["schedule"] = h.scheduleToResponse(schedule)
	}

	h.logger.Info("Schedule enabled successfully",
		"request_id", requestID,
		"schedule_id", scheduleID)

	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_schedules_enabled_total", nil)
	}
}

// DisableSchedule handles POST /api/v1/schedules/{id}/disable - disable a schedule.
func (h *ScheduleHandler) DisableSchedule(w http.ResponseWriter, r *http.Request) {
	requestID := getRequestIDFromContext(r.Context())

	// Extract schedule ID from URL
	scheduleID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	h.logger.Info("Disabling schedule", "request_id", requestID, "schedule_id", scheduleID)

	// Disable schedule in database
	err = h.database.DisableSchedule(r.Context(), scheduleID)
	if err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound,
				fmt.Errorf("schedule not found"))
			return
		}
		h.logger.Error("Failed to deactivate schedule",
			"request_id", requestID, "schedule_id", scheduleID, "error", err)
		writeError(w, r, http.StatusInternalServerError,
			fmt.Errorf("failed to deactivate schedule: %w", err))
		return
	}

	response := map[string]interface{}{
		"schedule_id": scheduleID,
		"status":      "disabled",
		"message":     "Schedule has been disabled",
		"timestamp":   time.Now().UTC(),
		"request_id":  requestID,
	}

	h.logger.Info("Schedule disabled successfully",
		"request_id", requestID,
		"schedule_id", scheduleID)

	writeJSON(w, r, http.StatusOK, response)

	// Record metrics
	if h.metrics != nil {
		h.metrics.Counter("api_schedules_disabled_total", nil)
	}
}

// Helper methods

// validateScheduleRequest validates a schedule request.
func (h *ScheduleHandler) validateScheduleRequest(req *ScheduleRequest) error {
	if err := h.validateBasicScheduleFields(req); err != nil {
		return err
	}
	if err := h.validateScheduleCron(req.CronExpr); err != nil {
		return err
	}
	if err := h.validateScheduleType(req.Type); err != nil {
		return err
	}
	if err := h.validateScheduleOptions(req); err != nil {
		return err
	}
	return h.validateScheduleTags(req.Tags)
}

func (h *ScheduleHandler) validateBasicScheduleFields(req *ScheduleRequest) error {
	if req.Name == "" {
		return fmt.Errorf("schedule name is required")
	}
	if len(req.Name) > maxScheduleNameLength {
		return fmt.Errorf("schedule name too long (max %d characters)", maxScheduleNameLength)
	}
	if len(req.Description) > maxScheduleDescLength {
		return fmt.Errorf("description too long (max %d characters)", maxScheduleDescLength)
	}
	return nil
}

func (h *ScheduleHandler) validateScheduleCron(cronExpr string) error {
	if cronExpr == "" {
		return fmt.Errorf("cron expression is required")
	}
	if err := h.validateCronExpression(cronExpr); err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}
	return nil
}

func (h *ScheduleHandler) validateScheduleType(scheduleType string) error {
	validTypes := map[string]bool{
		"scan":      true,
		"discovery": true,
	}
	if !validTypes[scheduleType] {
		return fmt.Errorf("invalid schedule type: %s", scheduleType)
	}
	return nil
}

func (h *ScheduleHandler) validateScheduleOptions(req *ScheduleRequest) error {
	if req.NetworkID == uuid.Nil {
		return fmt.Errorf("network_id is required")
	}

	// Validate timeouts
	if req.MaxRunTime < 0 {
		return fmt.Errorf("max run time cannot be negative")
	}
	if req.MaxRunTime > 24*time.Hour {
		return fmt.Errorf("max run time too long (max 24 hours)")
	}

	// Validate retries
	if req.MaxRetries < 0 {
		return fmt.Errorf("max retries cannot be negative")
	}
	if req.MaxRetries > maxScheduleRetries {
		return fmt.Errorf("max retries too high (max %d)", maxScheduleRetries)
	}

	if req.RetryDelay < 0 {
		return fmt.Errorf("retry delay cannot be negative")
	}
	if req.RetryDelay > time.Hour {
		return fmt.Errorf("retry delay too long (max 1 hour)")
	}

	// Validate notification emails
	for i, email := range req.NotifyEmails {
		if email == "" {
			return fmt.Errorf("notification email %d is empty", i+1)
		}
		if len(email) > maxScheduleNameLength {
			return fmt.Errorf("notification email %d too long (max %d characters)", i+1, maxScheduleNameLength)
		}
		// Basic email format validation
		if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
			return fmt.Errorf("notification email %d has invalid format: %s", i+1, email)
		}
	}
	return nil
}

func (h *ScheduleHandler) validateScheduleTags(tags []string) error {
	for i, tag := range tags {
		if tag == "" {
			return fmt.Errorf("tag %d is empty", i+1)
		}
		if len(tag) > maxScheduleTagLength {
			return fmt.Errorf("tag %d too long (max %d characters)", i+1, maxScheduleTagLength)
		}
	}
	return nil
}

// validateCronExpression performs basic cron expression validation.
func (h *ScheduleHandler) validateCronExpression(cronExpr string) error {
	// Empty expression is invalid
	if strings.TrimSpace(cronExpr) == "" {
		return fmt.Errorf("cron expression cannot be empty")
	}

	// Use the robfig/cron parser to validate the expression
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(cronExpr)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	return nil
}

// getScheduleFilters extracts filter parameters from request.
func (h *ScheduleHandler) getScheduleFilters(r *http.Request) db.ScheduleFilters {
	filters := db.ScheduleFilters{}

	if scheduleType := r.URL.Query().Get("type"); scheduleType != "" {
		filters.JobType = scheduleType
	}

	if enabled := r.URL.Query().Get("enabled"); enabled != "" {
		if enabledVal, err := strconv.ParseBool(enabled); err == nil {
			filters.Enabled = enabledVal
		}
	}

	return filters
}

// buildScheduleJobConfig assembles the job_config map from a ScheduleRequest,
// packing fields that don't have dedicated DB columns.
func buildScheduleJobConfig(req *ScheduleRequest) map[string]interface{} {
	jobConfig := map[string]interface{}{
		"network_id":     req.NetworkID.String(),
		"max_run_time":   req.MaxRunTime.String(),
		"retry_on_error": req.RetryOnError,
		"max_retries":    req.MaxRetries,
		"retry_delay":    req.RetryDelay.String(),
		"notify_on_fail": req.NotifyOnFail,
		"notify_emails":  req.NotifyEmails,
		"tags":           req.Tags,
	}
	if req.Options != nil {
		jobConfig["options"] = req.Options
	}
	return jobConfig
}

// requestToCreateSchedule converts a ScheduleRequest to a typed CreateScheduleInput for the DB layer.
func (h *ScheduleHandler) requestToCreateSchedule(req *ScheduleRequest) db.CreateScheduleInput {
	return db.CreateScheduleInput{
		Name:           req.Name,
		JobType:        req.Type,
		CronExpression: req.CronExpr,
		JobConfig:      buildScheduleJobConfig(req),
		Enabled:        req.Enabled,
	}
}

// requestToUpdateSchedule converts a ScheduleRequest to a typed UpdateScheduleInput for the DB layer.
// Only non-empty fields are set so that absent values don't overwrite existing data.
func (h *ScheduleHandler) requestToUpdateSchedule(req *ScheduleRequest) db.UpdateScheduleInput {
	input := db.UpdateScheduleInput{
		JobConfig: buildScheduleJobConfig(req),
	}
	if req.Name != "" {
		input.Name = &req.Name
	}
	if req.Type != "" {
		input.JobType = &req.Type
	}
	if req.CronExpr != "" {
		input.CronExpression = &req.CronExpr
	}
	input.Enabled = &req.Enabled
	return input
}

// scheduleToResponse converts a database schedule to response format.
func (h *ScheduleHandler) scheduleToResponse(schedule *db.Schedule) ScheduleResponse {
	resp := ScheduleResponse{
		ID:          schedule.ID,
		Name:        schedule.Name,
		Description: schedule.Description,
		CronExpr:    schedule.CronExpression,
		Type:        schedule.JobType,
		Enabled:     schedule.Enabled,
		LastRun:     schedule.LastRun,
		NextRun:     schedule.NextRun,
		CreatedAt:   schedule.CreatedAt,
		UpdatedAt:   schedule.UpdatedAt,
	}

	// Derive status from enabled + last run info
	switch {
	case !schedule.Enabled:
		resp.Status = "disabled"
	case schedule.LastRun != nil:
		resp.Status = scheduleStatusActive
	default:
		resp.Status = "pending"
	}

	applyJobConfigToScheduleResponse(schedule.JobConfig, &resp)

	return resp
}

// applyJobConfigToScheduleResponse extracts optional fields stored in JobConfig
// and populates the corresponding response fields.
func applyJobConfigToScheduleResponse(cfg map[string]interface{}, resp *ScheduleResponse) {
	if cfg == nil {
		return
	}

	if networkID, ok := cfg["network_id"]; ok {
		resp.NetworkID = fmt.Sprintf("%v", networkID)
	}
	if v, ok := cfg["retry_on_error"].(bool); ok {
		resp.RetryOnError = v
	}
	if v, ok := cfg["max_retries"].(float64); ok {
		resp.MaxRetries = int(v)
	}
	if v, ok := cfg["notify_on_fail"].(bool); ok {
		resp.NotifyOnFail = v
	}

	if emails, ok := cfg["notify_emails"].([]interface{}); ok {
		for _, e := range emails {
			if s, ok := e.(string); ok {
				resp.NotifyEmails = append(resp.NotifyEmails, s)
			}
		}
	}
	if tags, ok := cfg["tags"].([]interface{}); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				resp.Tags = append(resp.Tags, s)
			}
		}
	}
	if opts, ok := cfg["options"].(map[string]interface{}); ok {
		resp.Options = make(map[string]string, len(opts))
		for k, v := range opts {
			resp.Options[k] = fmt.Sprintf("%v", v)
		}
	}
}
