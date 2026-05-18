// Package handlers — alert rule management endpoints.
package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

const entityTypeAlertRule = "alert rule"

// AlertHandler handles HTTP requests for alert rule management.
type AlertHandler struct {
	svc     AlertServicer
	logger  *slog.Logger
	metrics *metrics.Registry
}

// NewAlertHandler creates a new AlertHandler.
func NewAlertHandler(svc AlertServicer, logger *slog.Logger, m *metrics.Registry) *AlertHandler {
	return &AlertHandler{
		svc:     svc,
		logger:  logger.With("handler", "alert"),
		metrics: m,
	}
}

// createAlertRuleRequest is the body for POST /api/v1/alerts.
type createAlertRuleRequest struct {
	HostID     *string `json:"host_id"`
	GroupID    *string `json:"group_id"`
	Tag        *string `json:"tag"`
	Trigger    string  `json:"trigger"`
	ChannelURL string  `json:"channel_url"`
}

// updateAlertRuleRequest is the body for PATCH /api/v1/alerts/{id}.
type updateAlertRuleRequest struct {
	Trigger    *string `json:"trigger"`
	ChannelURL *string `json:"channel_url"`
	Enabled    *bool   `json:"enabled"`
}

// ListAlertRules handles GET /api/v1/alerts.
//
//	@Summary     List all alert rules
//	@Tags        alerts
//	@Produce     json
//	@Success     200  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /alerts [get]
func (h *AlertHandler) ListAlertRules(w http.ResponseWriter, r *http.Request) {
	rules, err := h.svc.ListAlertRules(r.Context())
	if err != nil {
		handleDatabaseError(w, r, err, "list", "alert rules", h.logger)
		return
	}
	if rules == nil {
		rules = make([]*db.AlertRule, 0)
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"alert_rules": rules})
}

// CreateAlertRule handles POST /api/v1/alerts.
//
//	@Summary     Create an alert rule
//	@Tags        alerts
//	@Accept      json
//	@Produce     json
//	@Param       body  body      createAlertRuleRequest  true  "Alert rule input"
//	@Success     201   {object}  db.AlertRule
//	@Failure     400   {object}  map[string]any
//	@Failure     500   {object}  map[string]any
//	@Router      /alerts [post]
func (h *AlertHandler) CreateAlertRule(w http.ResponseWriter, r *http.Request) {
	var req createAlertRuleRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if err := validateAlertRuleTrigger(req.Trigger); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	if err := validateAlertRuleChannelURL(req.ChannelURL); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	input, err := buildCreateAlertInput(req)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	if err := validateAlertRuleTarget(input); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	rule, err := h.svc.CreateAlertRule(r.Context(), input)
	if err != nil {
		handleDatabaseError(w, r, err, "create", entityTypeAlertRule, h.logger)
		return
	}
	writeJSON(w, r, http.StatusCreated, rule)
}

// GetAlertRule handles GET /api/v1/alerts/{id}.
//
//	@Summary     Get an alert rule
//	@Tags        alerts
//	@Produce     json
//	@Param       id   path      string  true  "Alert rule UUID"
//	@Success     200  {object}  db.AlertRule
//	@Failure     400  {object}  map[string]any
//	@Failure     404  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /alerts/{id} [get]
func (h *AlertHandler) GetAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid alert rule ID: %w", err))
		return
	}
	rule, err := h.svc.GetAlertRule(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", entityTypeAlertRule, h.logger)
		return
	}
	writeJSON(w, r, http.StatusOK, rule)
}

// UpdateAlertRule handles PATCH /api/v1/alerts/{id}.
//
//	@Summary     Update an alert rule
//	@Tags        alerts
//	@Accept      json
//	@Produce     json
//	@Param       id    path      string                true  "Alert rule UUID"
//	@Param       body  body      updateAlertRuleRequest true  "Update fields"
//	@Success     200   {object}  db.AlertRule
//	@Failure     400   {object}  map[string]any
//	@Failure     404   {object}  map[string]any
//	@Failure     500   {object}  map[string]any
//	@Router      /alerts/{id} [patch]
func (h *AlertHandler) UpdateAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid alert rule ID: %w", err))
		return
	}

	var req updateAlertRuleRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if req.Trigger != nil {
		if err := validateAlertRuleTrigger(*req.Trigger); err != nil {
			writeError(w, r, http.StatusBadRequest, err)
			return
		}
	}
	if req.ChannelURL != nil {
		if err := validateAlertRuleChannelURL(*req.ChannelURL); err != nil {
			writeError(w, r, http.StatusBadRequest, err)
			return
		}
	}

	rule, err := h.svc.UpdateAlertRule(r.Context(), id, db.UpdateAlertRuleInput{
		Trigger:    req.Trigger,
		ChannelURL: req.ChannelURL,
		Enabled:    req.Enabled,
	})
	if err != nil {
		handleDatabaseError(w, r, err, "update", entityTypeAlertRule, h.logger)
		return
	}
	writeJSON(w, r, http.StatusOK, rule)
}

// DeleteAlertRule handles DELETE /api/v1/alerts/{id}.
//
//	@Summary     Delete an alert rule
//	@Tags        alerts
//	@Param       id  path  string  true  "Alert rule UUID"
//	@Success     204
//	@Failure     400  {object}  map[string]any
//	@Failure     404  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /alerts/{id} [delete]
func (h *AlertHandler) DeleteAlertRule(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid alert rule ID: %w", err))
		return
	}
	if err := h.svc.DeleteAlertRule(r.Context(), id); err != nil {
		handleDatabaseError(w, r, err, "delete", entityTypeAlertRule, h.logger)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListAlertRulesForHost handles GET /api/v1/hosts/{id}/alerts.
//
//	@Summary     List alert rules for a host
//	@Tags        alerts
//	@Produce     json
//	@Param       id   path      string  true  "Host UUID"
//	@Success     200  {object}  map[string]any
//	@Failure     400  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /hosts/{id}/alerts [get]
func (h *AlertHandler) ListAlertRulesForHost(w http.ResponseWriter, r *http.Request) {
	hostID, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid host ID: %w", err))
		return
	}
	rules, err := h.svc.ListAlertRulesForHost(r.Context(), hostID)
	if err != nil {
		handleDatabaseError(w, r, err, "list", "host alert rules", h.logger)
		return
	}
	if rules == nil {
		rules = make([]*db.AlertRule, 0)
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"alert_rules": rules})
}

// validateAlertRuleTrigger checks that the trigger value is allowed.
func validateAlertRuleTrigger(trigger string) error {
	switch trigger {
	case db.AlertTriggerOnline, db.AlertTriggerOffline, db.AlertTriggerBoth:
		return nil
	default:
		return fmt.Errorf("trigger must be one of: online, offline, both")
	}
}

// validateAlertRuleChannelURL checks the channel URL is non-empty and http/https.
func validateAlertRuleChannelURL(url string) error {
	if url == "" {
		return fmt.Errorf("channel_url is required")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("channel_url must start with http:// or https://")
	}
	return nil
}

// validateAlertRuleTarget checks that exactly one of HostID, GroupID, or Tag is set.
func validateAlertRuleTarget(input db.CreateAlertRuleInput) error {
	count := 0
	if input.HostID != nil {
		count++
	}
	if input.GroupID != nil {
		count++
	}
	if input.Tag != nil && *input.Tag != "" {
		count++
	}
	if count != 1 {
		return fmt.Errorf("exactly one of host_id, group_id, or tag must be set")
	}
	return nil
}

// buildCreateAlertInput converts the HTTP request into a CreateAlertRuleInput,
// parsing UUID strings and validating the target constraint.
func buildCreateAlertInput(req createAlertRuleRequest) (db.CreateAlertRuleInput, error) {
	input := db.CreateAlertRuleInput{
		Trigger:    req.Trigger,
		ChannelURL: req.ChannelURL,
		Tag:        req.Tag,
	}
	if req.HostID != nil {
		id, err := parseUUIDField("host_id", *req.HostID)
		if err != nil {
			return db.CreateAlertRuleInput{}, err
		}
		input.HostID = &id
	}
	if req.GroupID != nil {
		id, err := parseUUIDField("group_id", *req.GroupID)
		if err != nil {
			return db.CreateAlertRuleInput{}, err
		}
		input.GroupID = &id
	}
	return input, nil
}

// parseUUIDField parses a UUID string and returns a descriptive error on failure.
func parseUUIDField(field, value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.UUID{}, fmt.Errorf("invalid %s: %w", field, err)
	}
	return id, nil
}
