// Package handlers — webhook management endpoints.
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

const (
	entityTypeWebhook = "webhook"
	defaultLogLimit   = 50
	maxLogLimit       = 200
	queryParamLimit   = "limit"
)

// WebhookHandler handles HTTP requests for webhook management.
type WebhookHandler struct {
	svc     WebhookServicer
	logger  *slog.Logger
	metrics *metrics.Registry
}

// NewWebhookHandler creates a new WebhookHandler.
func NewWebhookHandler(svc WebhookServicer, logger *slog.Logger, m *metrics.Registry) *WebhookHandler {
	return &WebhookHandler{
		svc:     svc,
		logger:  logger.With("handler", "webhook"),
		metrics: m,
	}
}

// createWebhookRequest is the body for POST /api/v1/webhooks.
type createWebhookRequest struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret"`
	Events []string `json:"events"`
}

// updateWebhookRequest is the body for PATCH /api/v1/webhooks/{id}.
type updateWebhookRequest struct {
	URL     *string  `json:"url"`
	Secret  *string  `json:"secret"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
}

// ListWebhooks handles GET /api/v1/webhooks.
//
//	@Summary     List webhook endpoints
//	@Tags        webhooks
//	@Produce     json
//	@Success     200  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /webhooks [get]
func (h *WebhookHandler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	endpoints, err := h.svc.ListWebhooks(r.Context())
	if err != nil {
		handleDatabaseError(w, r, err, "list", "webhooks", h.logger)
		return
	}
	resp := make([]*db.WebhookResponse, 0, len(endpoints))
	for _, ep := range endpoints {
		resp = append(resp, ep.ToResponse())
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"webhooks": resp})
}

// CreateWebhook handles POST /api/v1/webhooks.
//
//	@Summary     Create a webhook endpoint
//	@Tags        webhooks
//	@Accept      json
//	@Produce     json
//	@Param       body  body      createWebhookRequest  true  "Webhook input"
//	@Success     201   {object}  db.WebhookCreateResponse
//	@Failure     400   {object}  map[string]any
//	@Failure     500   {object}  map[string]any
//	@Router      /webhooks [post]
func (h *WebhookHandler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	var req createWebhookRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if err := validateWebhookURL(req.URL); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	if err := validateWebhookEvents(req.Events); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	endpoint, err := h.svc.CreateWebhook(r.Context(), db.CreateWebhookInput{
		URL:    req.URL,
		Secret: req.Secret,
		Events: req.Events,
	})
	if err != nil {
		handleDatabaseError(w, r, err, "create", entityTypeWebhook, h.logger)
		return
	}
	writeJSON(w, r, http.StatusCreated, endpoint.ToCreateResponse())
}

// GetWebhook handles GET /api/v1/webhooks/{id}.
//
//	@Summary     Get a webhook endpoint
//	@Tags        webhooks
//	@Produce     json
//	@Param       id   path      string  true  "Webhook UUID"
//	@Success     200  {object}  db.WebhookResponse
//	@Failure     400  {object}  map[string]any
//	@Failure     404  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /webhooks/{id} [get]
func (h *WebhookHandler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid webhook ID: %w", err))
		return
	}
	endpoint, err := h.svc.GetWebhook(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", entityTypeWebhook, h.logger)
		return
	}
	writeJSON(w, r, http.StatusOK, endpoint.ToResponse())
}

// UpdateWebhook handles PATCH /api/v1/webhooks/{id}.
//
//	@Summary     Update a webhook endpoint
//	@Tags        webhooks
//	@Accept      json
//	@Produce     json
//	@Param       id    path      string               true  "Webhook UUID"
//	@Param       body  body      updateWebhookRequest true  "Update fields"
//	@Success     200   {object}  db.WebhookResponse
//	@Failure     400   {object}  map[string]any
//	@Failure     404   {object}  map[string]any
//	@Failure     500   {object}  map[string]any
//	@Router      /webhooks/{id} [patch]
func (h *WebhookHandler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid webhook ID: %w", err))
		return
	}

	var req updateWebhookRequest
	if err := parseJSON(r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}

	if req.URL != nil {
		if err := validateWebhookURL(*req.URL); err != nil {
			writeError(w, r, http.StatusBadRequest, err)
			return
		}
	}
	if len(req.Events) > 0 {
		if err := validateWebhookEvents(req.Events); err != nil {
			writeError(w, r, http.StatusBadRequest, err)
			return
		}
	}

	endpoint, err := h.svc.UpdateWebhook(r.Context(), id, db.UpdateWebhookInput{
		URL:     req.URL,
		Secret:  req.Secret,
		Events:  req.Events,
		Enabled: req.Enabled,
	})
	if err != nil {
		handleDatabaseError(w, r, err, "update", entityTypeWebhook, h.logger)
		return
	}
	writeJSON(w, r, http.StatusOK, endpoint.ToResponse())
}

// DeleteWebhook handles DELETE /api/v1/webhooks/{id}.
//
//	@Summary     Delete a webhook endpoint
//	@Tags        webhooks
//	@Param       id  path  string  true  "Webhook UUID"
//	@Success     204
//	@Failure     400  {object}  map[string]any
//	@Failure     404  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /webhooks/{id} [delete]
func (h *WebhookHandler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid webhook ID: %w", err))
		return
	}
	if err := h.svc.DeleteWebhook(r.Context(), id); err != nil {
		handleDatabaseError(w, r, err, "delete", entityTypeWebhook, h.logger)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TestWebhook handles POST /api/v1/webhooks/{id}/test.
//
//	@Summary     Send a test delivery to a webhook endpoint
//	@Tags        webhooks
//	@Param       id  path  string  true  "Webhook UUID"
//	@Success     200  {object}  map[string]any
//	@Failure     400  {object}  map[string]any
//	@Failure     404  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /webhooks/{id}/test [post]
func (h *WebhookHandler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid webhook ID: %w", err))
		return
	}
	if err := h.svc.SendTestDelivery(r.Context(), id); err != nil {
		handleDatabaseError(w, r, err, "test", entityTypeWebhook, h.logger)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{responseKeyStatus: statusDelivered})
}

// ListDeliveryLogs handles GET /api/v1/webhooks/{id}/logs.
//
//	@Summary     List delivery logs for a webhook endpoint
//	@Tags        webhooks
//	@Produce     json
//	@Param       id     path      string  true  "Webhook UUID"
//	@Param       limit  query     int     false "Maximum log entries (default 50, max 200)"
//	@Success     200    {object}  map[string]any
//	@Failure     400    {object}  map[string]any
//	@Failure     500    {object}  map[string]any
//	@Router      /webhooks/{id}/logs [get]
func (h *WebhookHandler) ListDeliveryLogs(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, fmt.Errorf("invalid webhook ID: %w", err))
		return
	}

	limit, err := getQueryParamInt(r, queryParamLimit, defaultLogLimit)
	if err != nil || limit < 1 {
		limit = defaultLogLimit
	}
	if limit > maxLogLimit {
		limit = maxLogLimit
	}

	logs, err := h.svc.ListDeliveryLogs(r.Context(), id, limit)
	if err != nil {
		handleDatabaseError(w, r, err, "list", "delivery logs", h.logger)
		return
	}
	if logs == nil {
		logs = make([]*db.WebhookDeliveryLog, 0)
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"logs": logs})
}

// validateWebhookURL checks that the URL is non-empty and starts with http/https.
func validateWebhookURL(url string) error {
	if url == "" {
		return fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("url must start with http:// or https://")
	}
	return nil
}

// validateWebhookEvents checks that all event types are from the allowed set
// and that there are no duplicates.
func validateWebhookEvents(events []string) error {
	if len(events) == 0 {
		return fmt.Errorf("at least one event type is required")
	}
	seen := make(map[string]struct{}, len(events))
	for _, e := range events {
		if _, ok := services.AllowedWebhookEvents[e]; !ok {
			return fmt.Errorf("unknown event type %q", e)
		}
		if _, dup := seen[e]; dup {
			return fmt.Errorf("duplicate event type %q", e)
		}
		seen[e] = struct{}{}
	}
	return nil
}
