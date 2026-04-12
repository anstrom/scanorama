// Package handlers - Smart Scan HTTP adapter.
// Thin HTTP layer over services.SmartScanService.
package handlers

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/scanning"
	"github.com/anstrom/scanorama/internal/services"
)

// smartScanServicer is the service interface consumed by SmartScanHandler.
type smartScanServicer interface {
	GetSuggestions(ctx context.Context) (*services.SuggestionSummary, error)
	EvaluateHostByID(ctx context.Context, hostID uuid.UUID) (*services.ScanStage, error)
	QueueSmartScan(ctx context.Context, hostID uuid.UUID) (uuid.UUID, error)
	QueueBatch(ctx context.Context, filter services.BatchFilter) (*services.BatchResult, error)
}

// SmartScanHandler handles Smart Scan API endpoints.
type SmartScanHandler struct {
	service smartScanServicer
	logger  *slog.Logger
}

// NewSmartScanHandler creates a new SmartScanHandler.
func NewSmartScanHandler(svc *services.SmartScanService, logger *slog.Logger) *SmartScanHandler {
	return &SmartScanHandler{
		service: svc,
		logger:  logger.With("handler", "smart_scan"),
	}
}

// GetSuggestions handles GET /api/v1/smart-scan/suggestions.
//
//	@Summary		Get Smart Scan suggestions
//	@Description	Returns fleet-wide counts of hosts in each knowledge-gap category.
//	@Tags			smart-scan
//	@Produce		json
//	@Success		200	{object}	services.SuggestionSummary
//	@Failure		500	{object}	ErrorResponse
//	@Router			/smart-scan/suggestions [get]
func (h *SmartScanHandler) GetSuggestions(w http.ResponseWriter, r *http.Request) {
	summary, err := h.service.GetSuggestions(r.Context())
	if err != nil {
		h.logger.Error("Failed to get smart scan suggestions", "error", err)
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, r, http.StatusOK, summary)
}

// EvaluateHost handles GET /api/v1/smart-scan/hosts/{id}/stage.
//
//	@Summary		Evaluate next scan stage for a host
//	@Description	Returns the recommended next scan stage for the specified host.
//	@Tags			smart-scan
//	@Produce		json
//	@Param			id	path		string	true	"Host UUID"
//	@Success		200	{object}	services.ScanStage
//	@Failure		400	{object}	ErrorResponse
//	@Failure		404	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/smart-scan/hosts/{id}/stage [get]
func (h *SmartScanHandler) EvaluateHost(w http.ResponseWriter, r *http.Request) {
	hostID, err := parseHostID(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	// Load host so EvaluateHost can inspect its fields.
	stage, err := h.service.EvaluateHostByID(r.Context(), hostID)
	if err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound, err)
			return
		}
		h.logger.Error("Failed to evaluate host stage", "host_id", hostID, "error", err)
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, r, http.StatusOK, stage)
}

// TriggerHost handles POST /api/v1/smart-scan/hosts/{id}/trigger.
//
//	@Summary		Trigger Smart Scan for a host
//	@Description	Evaluates the host's knowledge gaps and queues the appropriate scan.
//	@Tags			smart-scan
//	@Produce		json
//	@Param			id	path		string	true	"Host UUID"
//	@Success		202	{object}	triggerHostResponse
//	@Success		200	{object}	triggerHostResponse	"No action needed"
//	@Failure		400	{object}	ErrorResponse
//	@Failure		429	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/smart-scan/hosts/{id}/trigger [post]
func (h *SmartScanHandler) TriggerHost(w http.ResponseWriter, r *http.Request) {
	hostID, err := parseHostID(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}

	scanID, err := h.service.QueueSmartScan(r.Context(), hostID)
	if err != nil {
		if errors.IsNotFound(err) {
			writeError(w, r, http.StatusNotFound, err)
			return
		}
		if stderrors.Is(err, scanning.ErrQueueFull) {
			writeError(w, r, http.StatusTooManyRequests, err)
			return
		}
		h.logger.Error("Failed to trigger smart scan", "host_id", hostID, "error", err)
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}

	resp := triggerHostResponse{HostID: hostID.String()}
	if scanID == uuid.Nil {
		resp.Queued = false
		resp.Message = "no scan needed — host knowledge is sufficient"
		writeJSON(w, r, http.StatusOK, resp)
		return
	}
	resp.Queued = true
	resp.ScanID = scanID.String()
	writeJSON(w, r, http.StatusAccepted, resp)
}

type triggerHostResponse struct {
	HostID  string `json:"host_id"`
	Queued  bool   `json:"queued"`
	ScanID  string `json:"scan_id,omitempty"`
	Message string `json:"message,omitempty"`
}

// TriggerBatch handles POST /api/v1/smart-scan/trigger-batch.
//
//	@Summary		Batch trigger Smart Scan
//	@Description	Queues smart scans for all eligible hosts matching the filter.
//	@Tags			smart-scan
//	@Accept			json
//	@Produce		json
//	@Param			body	body		triggerBatchRequest	false	"Batch filter"
//	@Success		202	{object}	services.BatchResult
//	@Failure		400	{object}	ErrorResponse
//	@Failure		500	{object}	ErrorResponse
//	@Router			/smart-scan/trigger-batch [post]
func (h *SmartScanHandler) TriggerBatch(w http.ResponseWriter, r *http.Request) {
	var req triggerBatchRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, r, http.StatusBadRequest, err)
			return
		}
	}

	hostIDs := make([]uuid.UUID, 0, len(req.HostIDs))
	for _, s := range req.HostIDs {
		id, err := uuid.Parse(s)
		if err != nil {
			writeError(w, r, http.StatusBadRequest, err)
			return
		}
		hostIDs = append(hostIDs, id)
	}

	result, err := h.service.QueueBatch(r.Context(), services.BatchFilter{
		Stage:       req.Stage,
		HostIDs:     hostIDs,
		NetworkCIDR: req.NetworkCIDR,
		Limit:       req.Limit,
	})
	if err != nil {
		h.logger.Error("Failed to queue batch smart scan", "error", err)
		writeError(w, r, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, r, http.StatusAccepted, result)
}

type triggerBatchRequest struct {
	Stage       string   `json:"stage"`
	HostIDs     []string `json:"host_ids"`
	NetworkCIDR string   `json:"network_cidr"`
	Limit       int      `json:"limit"`
}

// parseHostID extracts the host UUID from the path variable {id}.
func parseHostID(r *http.Request) (uuid.UUID, error) {
	vars := mux.Vars(r)
	raw, ok := vars["id"]
	if !ok || raw == "" {
		return uuid.Nil, errors.NewScanError(errors.CodeValidation, "missing host id")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, errors.NewScanError(errors.CodeValidation, "invalid host id")
	}
	return id, nil
}
