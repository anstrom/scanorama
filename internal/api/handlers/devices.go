// Package handlers — device identity endpoints.
package handlers

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

// DeviceHandler handles HTTP requests for device identity management.
type DeviceHandler struct {
	svc     DeviceServicer
	logger  *slog.Logger
	metrics *metrics.Registry
}

// NewDeviceHandler creates a DeviceHandler.
func NewDeviceHandler(svc DeviceServicer, logger *slog.Logger, m *metrics.Registry) *DeviceHandler {
	return &DeviceHandler{
		svc:     svc,
		logger:  logger.With("handler", "device"),
		metrics: m,
	}
}

// ListDevices handles GET /api/v1/devices.
//
//	@Summary     List devices
//	@Tags        devices
//	@Produce     json
//	@Success     200  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /devices [get]
func (h *DeviceHandler) ListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.svc.ListDevices(r.Context())
	if err != nil {
		handleDatabaseError(w, r, err, "list", "devices", h.logger)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"devices": devices})
}

// CreateDevice handles POST /api/v1/devices.
//
//	@Summary     Create a device
//	@Tags        devices
//	@Accept      json
//	@Produce     json
//	@Param       body  body      db.CreateDeviceInput  true  "Device input"
//	@Success     201   {object}  db.Device
//	@Failure     400   {object}  map[string]any
//	@Failure     500   {object}  map[string]any
//	@Router      /devices [post]
func (h *DeviceHandler) CreateDevice(w http.ResponseWriter, r *http.Request) {
	var input db.CreateDeviceInput
	if err := parseJSON(r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	device, err := h.svc.CreateDevice(r.Context(), input)
	if err != nil {
		if errors.IsCode(err, errors.CodeValidation) {
			writeError(w, r, http.StatusBadRequest, err)
			return
		}
		handleDatabaseError(w, r, err, "create", "device", h.logger)
		return
	}
	writeJSON(w, r, http.StatusCreated, device)
}

// GetDevice handles GET /api/v1/devices/{id}.
//
//	@Summary     Get device detail
//	@Tags        devices
//	@Produce     json
//	@Param       id   path      string  true  "Device UUID"
//	@Success     200  {object}  db.DeviceDetail
//	@Failure     404  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /devices/{id} [get]
func (h *DeviceHandler) GetDevice(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	detail, err := h.svc.GetDeviceDetail(r.Context(), id)
	if err != nil {
		handleDatabaseError(w, r, err, "get", "device", h.logger)
		return
	}
	writeJSON(w, r, http.StatusOK, detail)
}

// UpdateDevice handles PUT /api/v1/devices/{id}.
//
//	@Summary     Update device name / notes
//	@Tags        devices
//	@Accept      json
//	@Produce     json
//	@Param       id    path      string               true  "Device UUID"
//	@Param       body  body      db.UpdateDeviceInput true  "Update fields"
//	@Success     200   {object}  db.Device
//	@Failure     400   {object}  map[string]any
//	@Failure     404   {object}  map[string]any
//	@Failure     500   {object}  map[string]any
//	@Router      /devices/{id} [put]
func (h *DeviceHandler) UpdateDevice(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	var input db.UpdateDeviceInput
	if err := parseJSON(r, &input); err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	device, err := h.svc.UpdateDevice(r.Context(), id, input)
	if err != nil {
		handleDatabaseError(w, r, err, "update", "device", h.logger)
		return
	}
	writeJSON(w, r, http.StatusOK, device)
}

// DeleteDevice handles DELETE /api/v1/devices/{id}.
//
//	@Summary     Delete a device
//	@Tags        devices
//	@Param       id  path  string  true  "Device UUID"
//	@Success     204
//	@Failure     404  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /devices/{id} [delete]
func (h *DeviceHandler) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.DeleteDevice(r.Context(), id); err != nil {
		handleDatabaseError(w, r, err, "delete", "device", h.logger)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AttachHost handles POST /api/v1/devices/{id}/hosts/{host_id}.
//
//	@Summary     Attach a host to a device
//	@Tags        devices
//	@Param       id       path  string  true  "Device UUID"
//	@Param       host_id  path  string  true  "Host UUID"
//	@Success     204
//	@Failure     400  {object}  map[string]any
//	@Failure     404  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /devices/{id}/hosts/{host_id} [post]
func (h *DeviceHandler) AttachHost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	hostID, err := uuid.Parse(vars["host_id"])
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.AttachHost(r.Context(), deviceID, hostID); err != nil {
		handleDatabaseError(w, r, err, "attach", "host", h.logger)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DetachHost handles DELETE /api/v1/devices/{id}/hosts/{host_id}.
//
//	@Summary     Detach a host from a device
//	@Tags        devices
//	@Param       id       path  string  true  "Device UUID"
//	@Param       host_id  path  string  true  "Host UUID"
//	@Success     204
//	@Failure     400  {object}  map[string]any
//	@Failure     404  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /devices/{id}/hosts/{host_id} [delete]
func (h *DeviceHandler) DetachHost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	deviceID, err := uuid.Parse(vars["id"])
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	hostID, err := uuid.Parse(vars["host_id"])
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.DetachHost(r.Context(), deviceID, hostID); err != nil {
		handleDatabaseError(w, r, err, "detach", "host", h.logger)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AcceptSuggestion handles POST /api/v1/devices/suggestions/{id}/accept.
//
//	@Summary     Accept a device suggestion (attaches the host)
//	@Tags        devices
//	@Param       id  path  string  true  "Suggestion UUID"
//	@Success     204
//	@Failure     404  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /devices/suggestions/{id}/accept [post]
func (h *DeviceHandler) AcceptSuggestion(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.AcceptSuggestion(r.Context(), id); err != nil {
		handleDatabaseError(w, r, err, "accept", "suggestion", h.logger)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DismissSuggestion handles POST /api/v1/devices/suggestions/{id}/dismiss.
//
//	@Summary     Dismiss a device suggestion
//	@Tags        devices
//	@Param       id  path  string  true  "Suggestion UUID"
//	@Success     204
//	@Failure     404  {object}  map[string]any
//	@Failure     500  {object}  map[string]any
//	@Router      /devices/suggestions/{id}/dismiss [post]
func (h *DeviceHandler) DismissSuggestion(w http.ResponseWriter, r *http.Request) {
	id, err := extractUUIDFromPath(r)
	if err != nil {
		writeError(w, r, http.StatusBadRequest, err)
		return
	}
	if err := h.svc.DismissSuggestion(r.Context(), id); err != nil {
		handleDatabaseError(w, r, err, "dismiss", "suggestion", h.logger)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
