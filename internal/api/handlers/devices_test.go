package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/api/handlers/mocks"
	"github.com/anstrom/scanorama/internal/db"
	apierrors "github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newDeviceHandlerWithMock(t *testing.T) (*DeviceHandler, *mocks.MockDeviceServicer, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	svc := mocks.NewMockDeviceServicer(ctrl)
	h := NewDeviceHandler(svc, createTestLogger(), metrics.NewRegistry())
	return h, svc, ctrl
}

func newDeviceRouter(h *DeviceHandler) *mux.Router {
	r := mux.NewRouter()
	// Suggestion routes before {id} wildcard — mirrors production ordering.
	r.HandleFunc("/devices/suggestions/{id}/accept", h.AcceptSuggestion).Methods(http.MethodPost)
	r.HandleFunc("/devices/suggestions/{id}/dismiss", h.DismissSuggestion).Methods(http.MethodPost)
	r.HandleFunc("/devices", h.ListDevices).Methods(http.MethodGet)
	r.HandleFunc("/devices", h.CreateDevice).Methods(http.MethodPost)
	r.HandleFunc("/devices/{id}", h.GetDevice).Methods(http.MethodGet)
	r.HandleFunc("/devices/{id}", h.UpdateDevice).Methods(http.MethodPut)
	r.HandleFunc("/devices/{id}", h.DeleteDevice).Methods(http.MethodDelete)
	r.HandleFunc("/devices/{id}/hosts/{host_id}", h.AttachHost).Methods(http.MethodPost)
	r.HandleFunc("/devices/{id}/hosts/{host_id}", h.DetachHost).Methods(http.MethodDelete)
	return r
}

func makeDeviceSummary(id uuid.UUID, name string) db.DeviceSummary {
	return db.DeviceSummary{
		ID:        id,
		Name:      name,
		MACCount:  0,
		HostCount: 0,
	}
}

func makeDevice(id uuid.UUID, name string) *db.Device {
	return &db.Device{
		ID:        id,
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func makeDeviceDetail(id uuid.UUID, name string) *db.DeviceDetail {
	return &db.DeviceDetail{
		Device:     *makeDevice(id, name),
		KnownMACs:  make([]db.DeviceKnownMAC, 0),
		KnownNames: make([]db.DeviceKnownName, 0),
		Hosts:      make([]db.AttachedHostSummary, 0),
	}
}

// ── ListDevices ───────────────────────────────────────────────────────────────

func TestListDevices_ReturnsDevices(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id1 := uuid.New()
	id2 := uuid.New()
	summaries := []db.DeviceSummary{
		makeDeviceSummary(id1, "Router"),
		makeDeviceSummary(id2, "NAS"),
	}

	svc.EXPECT().
		ListDevices(gomock.Any()).
		Return(summaries, nil)

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/devices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	devices, ok := resp["devices"].([]any)
	require.True(t, ok, "expected 'devices' key with a JSON array value")
	assert.Len(t, devices, 2)
}

func TestListDevices_EmptyList(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	svc.EXPECT().
		ListDevices(gomock.Any()).
		Return([]db.DeviceSummary{}, nil)

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/devices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	devices, ok := resp["devices"].([]any)
	require.True(t, ok, "empty device list must serialize as [] not null")
	assert.Empty(t, devices)
}

func TestListDevices_ServiceError(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	svc.EXPECT().
		ListDevices(gomock.Any()).
		Return(nil, fmt.Errorf("database unavailable"))

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/devices", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── CreateDevice ──────────────────────────────────────────────────────────────

func TestCreateDevice_Valid(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	device := makeDevice(id, "Router")

	svc.EXPECT().
		CreateDevice(gomock.Any(), gomock.Any()).
		Return(device, nil)

	r := newDeviceRouter(h)
	body := `{"name":"Router","notes":""}`
	req := httptest.NewRequest(http.MethodPost, "/devices", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Equal(t, id.String(), raw["id"])
	assert.Equal(t, "Router", raw["name"])
}

func TestCreateDevice_ValidationError(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	svc.EXPECT().
		CreateDevice(gomock.Any(), gomock.Any()).
		Return(nil, apierrors.NewScanError(apierrors.CodeValidation, "device name is required"))

	r := newDeviceRouter(h)
	body := `{"name":""}`
	req := httptest.NewRequest(http.MethodPost, "/devices", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateDevice_InvalidBody(t *testing.T) {
	h, _, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodPost, "/devices",
		bytes.NewBufferString(`{not valid json}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateDevice_ServiceError(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	svc.EXPECT().
		CreateDevice(gomock.Any(), gomock.Any()).
		Return(nil, fmt.Errorf("database error"))

	r := newDeviceRouter(h)
	body := `{"name":"Router"}`
	req := httptest.NewRequest(http.MethodPost, "/devices", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── GetDevice ─────────────────────────────────────────────────────────────────

func TestGetDevice_Success(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	detail := makeDeviceDetail(id, "Router")

	svc.EXPECT().
		GetDeviceDetail(gomock.Any(), id).
		Return(detail, nil)

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/devices/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Equal(t, id.String(), raw["id"])
	assert.Equal(t, "Router", raw["name"])
}

func TestGetDevice_NotFound(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		GetDeviceDetail(gomock.Any(), id).
		Return(nil, notFoundErr("device", id.String()))

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/devices/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetDevice_InvalidID(t *testing.T) {
	h, _, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/devices/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── UpdateDevice ──────────────────────────────────────────────────────────────

func TestUpdateDevice_Success(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	notes := "updated notes"
	updated := makeDevice(id, "Main Router")
	updated.Notes = &notes

	svc.EXPECT().
		UpdateDevice(gomock.Any(), id, gomock.Any()).
		Return(updated, nil)

	r := newDeviceRouter(h)
	body := `{"name":"Main Router","notes":"updated notes"}`
	req := httptest.NewRequest(http.MethodPut, "/devices/"+id.String(),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Equal(t, "Main Router", raw["name"])
}

func TestUpdateDevice_NotFound(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		UpdateDevice(gomock.Any(), id, gomock.Any()).
		Return(nil, notFoundErr("device", id.String()))

	r := newDeviceRouter(h)
	body := `{"name":"Renamed"}`
	req := httptest.NewRequest(http.MethodPut, "/devices/"+id.String(),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateDevice_InvalidBody(t *testing.T) {
	h, _, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()
	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodPut, "/devices/"+id.String(),
		bytes.NewBufferString(`{bad json}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// ── DeleteDevice ──────────────────────────────────────────────────────────────

func TestDeleteDevice_Success(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		DeleteDevice(gomock.Any(), id).
		Return(nil)

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodDelete, "/devices/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDeleteDevice_NotFound(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		DeleteDevice(gomock.Any(), id).
		Return(notFoundErr("device", id.String()))

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodDelete, "/devices/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── AttachHost / DetachHost ───────────────────────────────────────────────────

func TestAttachHost_Success(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	deviceID := uuid.New()
	hostID := uuid.New()

	svc.EXPECT().
		AttachHost(gomock.Any(), deviceID, hostID).
		Return(nil)

	r := newDeviceRouter(h)
	path := fmt.Sprintf("/devices/%s/hosts/%s", deviceID, hostID)
	req := httptest.NewRequest(http.MethodPost, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestAttachHost_InvalidDeviceID(t *testing.T) {
	h, _, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	hostID := uuid.New()
	r := newDeviceRouter(h)
	path := fmt.Sprintf("/devices/bad-id/hosts/%s", hostID)
	req := httptest.NewRequest(http.MethodPost, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAttachHost_InvalidHostID(t *testing.T) {
	h, _, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	deviceID := uuid.New()
	r := newDeviceRouter(h)
	path := fmt.Sprintf("/devices/%s/hosts/bad-id", deviceID)
	req := httptest.NewRequest(http.MethodPost, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAttachHost_NotFound(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	deviceID := uuid.New()
	hostID := uuid.New()

	svc.EXPECT().
		AttachHost(gomock.Any(), deviceID, hostID).
		Return(notFoundErr("device", deviceID.String()))

	r := newDeviceRouter(h)
	path := fmt.Sprintf("/devices/%s/hosts/%s", deviceID, hostID)
	req := httptest.NewRequest(http.MethodPost, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDetachHost_Success(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	deviceID := uuid.New()
	hostID := uuid.New()

	svc.EXPECT().
		DetachHost(gomock.Any(), deviceID, hostID).
		Return(nil)

	r := newDeviceRouter(h)
	path := fmt.Sprintf("/devices/%s/hosts/%s", deviceID, hostID)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDetachHost_NotFound(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	deviceID := uuid.New()
	hostID := uuid.New()

	svc.EXPECT().
		DetachHost(gomock.Any(), deviceID, hostID).
		Return(notFoundErr("host", hostID.String()))

	r := newDeviceRouter(h)
	path := fmt.Sprintf("/devices/%s/hosts/%s", deviceID, hostID)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── AcceptSuggestion ─────────────────────────────────────────────────────────

func TestAcceptSuggestion_Success(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		AcceptSuggestion(gomock.Any(), id).
		Return(nil)

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodPost,
		"/devices/suggestions/"+id.String()+"/accept", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestAcceptSuggestion_NotFound(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		AcceptSuggestion(gomock.Any(), id).
		Return(notFoundErr("suggestion", id.String()))

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodPost,
		"/devices/suggestions/"+id.String()+"/accept", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── DismissSuggestion ─────────────────────────────────────────────────────────

func TestDismissSuggestion_Success(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		DismissSuggestion(gomock.Any(), id).
		Return(nil)

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodPost,
		"/devices/suggestions/"+id.String()+"/dismiss", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDismissSuggestion_NotFound(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		DismissSuggestion(gomock.Any(), id).
		Return(notFoundErr("suggestion", id.String()))

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodPost,
		"/devices/suggestions/"+id.String()+"/dismiss", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── Additional error-path coverage ───────────────────────────────────────────

func TestUpdateDevice_InvalidID(t *testing.T) {
	h, _, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodPut, "/devices/not-a-uuid",
		bytes.NewBufferString(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteDevice_InvalidID(t *testing.T) {
	h, _, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodDelete, "/devices/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteDevice_ServiceError(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		DeleteDevice(gomock.Any(), id).
		Return(fmt.Errorf("database unavailable"))

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodDelete, "/devices/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAttachHost_ServiceError(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	deviceID := uuid.New()
	hostID := uuid.New()

	svc.EXPECT().
		AttachHost(gomock.Any(), deviceID, hostID).
		Return(fmt.Errorf("database unavailable"))

	r := newDeviceRouter(h)
	path := fmt.Sprintf("/devices/%s/hosts/%s", deviceID, hostID)
	req := httptest.NewRequest(http.MethodPost, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestDetachHost_ServiceError(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	deviceID := uuid.New()
	hostID := uuid.New()

	svc.EXPECT().
		DetachHost(gomock.Any(), deviceID, hostID).
		Return(fmt.Errorf("database unavailable"))

	r := newDeviceRouter(h)
	path := fmt.Sprintf("/devices/%s/hosts/%s", deviceID, hostID)
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestAcceptSuggestion_InvalidID(t *testing.T) {
	h, _, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodPost,
		"/devices/suggestions/not-a-uuid/accept", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAcceptSuggestion_ServiceError(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		AcceptSuggestion(gomock.Any(), id).
		Return(fmt.Errorf("database unavailable"))

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodPost,
		"/devices/suggestions/"+id.String()+"/accept", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestDismissSuggestion_InvalidID(t *testing.T) {
	h, _, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodPost,
		"/devices/suggestions/not-a-uuid/dismiss", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDismissSuggestion_ServiceError(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		DismissSuggestion(gomock.Any(), id).
		Return(fmt.Errorf("database unavailable"))

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodPost,
		"/devices/suggestions/"+id.String()+"/dismiss", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetDevice_ServiceError(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		GetDeviceDetail(gomock.Any(), id).
		Return(nil, fmt.Errorf("database unavailable"))

	r := newDeviceRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/devices/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateDevice_ServiceError(t *testing.T) {
	h, svc, ctrl := newDeviceHandlerWithMock(t)
	defer ctrl.Finish()

	id := uuid.New()

	svc.EXPECT().
		UpdateDevice(gomock.Any(), id, gomock.Any()).
		Return(nil, fmt.Errorf("database unavailable"))

	r := newDeviceRouter(h)
	body := `{"name":"Router"}`
	req := httptest.NewRequest(http.MethodPut, "/devices/"+id.String(),
		bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
