package handlers

import (
	"bytes"
	"context"
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

	"github.com/anstrom/scanorama/internal/db"
	apierrors "github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

// ── mock ─────────────────────────────────────────────────────────────────────

type mockAlertServicer struct {
	listFn        func(ctx context.Context) ([]*db.AlertRule, error)
	listForHostFn func(ctx context.Context, hostID uuid.UUID) ([]*db.AlertRule, error)
	createFn      func(ctx context.Context, input db.CreateAlertRuleInput) (*db.AlertRule, error)
	getFn         func(ctx context.Context, id uuid.UUID) (*db.AlertRule, error)
	updateFn      func(ctx context.Context, id uuid.UUID, input db.UpdateAlertRuleInput) (*db.AlertRule, error)
	deleteFn      func(ctx context.Context, id uuid.UUID) error
}

func (m *mockAlertServicer) ListAlertRules(ctx context.Context) ([]*db.AlertRule, error) {
	return m.listFn(ctx)
}

func (m *mockAlertServicer) ListAlertRulesForHost(
	ctx context.Context, hostID uuid.UUID,
) ([]*db.AlertRule, error) {
	return m.listForHostFn(ctx, hostID)
}

func (m *mockAlertServicer) CreateAlertRule(
	ctx context.Context, input db.CreateAlertRuleInput,
) (*db.AlertRule, error) {
	return m.createFn(ctx, input)
}

func (m *mockAlertServicer) GetAlertRule(ctx context.Context, id uuid.UUID) (*db.AlertRule, error) {
	return m.getFn(ctx, id)
}

func (m *mockAlertServicer) UpdateAlertRule(
	ctx context.Context, id uuid.UUID, input db.UpdateAlertRuleInput,
) (*db.AlertRule, error) {
	return m.updateFn(ctx, id, input)
}

func (m *mockAlertServicer) DeleteAlertRule(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newAlertRouter(h *AlertHandler) *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/alerts", h.ListAlertRules).Methods(http.MethodGet)
	r.HandleFunc("/alerts", h.CreateAlertRule).Methods(http.MethodPost)
	r.HandleFunc("/alerts/{id}", h.GetAlertRule).Methods(http.MethodGet)
	r.HandleFunc("/alerts/{id}", h.UpdateAlertRule).Methods(http.MethodPatch)
	r.HandleFunc("/alerts/{id}", h.DeleteAlertRule).Methods(http.MethodDelete)
	r.HandleFunc("/hosts/{id}/alerts", h.ListAlertRulesForHost).Methods(http.MethodGet)
	return r
}

func newAlertHandler(svc *mockAlertServicer) *AlertHandler {
	return NewAlertHandler(svc, createTestLogger(), metrics.NewRegistry())
}

func makeAlertRule(id, hostID uuid.UUID) *db.AlertRule {
	hid := hostID
	return &db.AlertRule{
		ID:          id,
		HostID:      &hid,
		Trigger:     db.AlertTriggerOnline,
		ChannelType: db.AlertChannelTypeWebhook,
		ChannelURL:  "https://example.com/hook",
		Enabled:     true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// ── ListAlertRules ────────────────────────────────────────────────────────────

func TestListAlertRules_Success(t *testing.T) {
	id1 := uuid.New()
	hostID := uuid.New()
	svc := &mockAlertServicer{
		listFn: func(_ context.Context) ([]*db.AlertRule, error) {
			return []*db.AlertRule{makeAlertRule(id1, hostID)}, nil
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	rules, ok := raw["alert_rules"].([]any)
	require.True(t, ok)
	assert.Len(t, rules, 1)
}

func TestListAlertRules_Empty(t *testing.T) {
	svc := &mockAlertServicer{
		listFn: func(_ context.Context) ([]*db.AlertRule, error) {
			return make([]*db.AlertRule, 0), nil
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	rules, ok := raw["alert_rules"].([]any)
	require.True(t, ok)
	assert.Len(t, rules, 0)
}

func TestListAlertRules_ServiceError(t *testing.T) {
	svc := &mockAlertServicer{
		listFn: func(_ context.Context) ([]*db.AlertRule, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/alerts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── CreateAlertRule ───────────────────────────────────────────────────────────

func TestCreateAlertRule_Success(t *testing.T) {
	id := uuid.New()
	hostID := uuid.New()
	svc := &mockAlertServicer{
		createFn: func(_ context.Context, input db.CreateAlertRuleInput) (*db.AlertRule, error) {
			return makeAlertRule(id, hostID), nil
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	body := `{"host_id":"` + hostID.String() + `","trigger":"online","channel_url":"https://example.com/hook"}`
	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.Equal(t, "https://example.com/hook", raw["channel_url"])
	assert.Equal(t, "online", raw["trigger"])
}

func TestCreateAlertRule_InvalidTrigger(t *testing.T) {
	svc := &mockAlertServicer{}
	router := newAlertRouter(newAlertHandler(svc))

	hostID := uuid.New()
	body := `{"host_id":"` + hostID.String() + `","trigger":"invalid","channel_url":"https://example.com/hook"}`
	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateAlertRule_InvalidURL(t *testing.T) {
	svc := &mockAlertServicer{}
	router := newAlertRouter(newAlertHandler(svc))

	hostID := uuid.New()
	body := `{"host_id":"` + hostID.String() + `","trigger":"online","channel_url":"ftp://bad"}`
	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateAlertRule_MissingTarget(t *testing.T) {
	svc := &mockAlertServicer{}
	router := newAlertRouter(newAlertHandler(svc))

	body := `{"trigger":"online","channel_url":"https://example.com/hook"}`
	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateAlertRule_ServiceError(t *testing.T) {
	hostID := uuid.New()
	svc := &mockAlertServicer{
		createFn: func(_ context.Context, _ db.CreateAlertRuleInput) (*db.AlertRule, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	body := `{"host_id":"` + hostID.String() + `","trigger":"online","channel_url":"https://example.com/hook"}`
	req := httptest.NewRequest(http.MethodPost, "/alerts", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── GetAlertRule ──────────────────────────────────────────────────────────────

func TestGetAlertRule_Success(t *testing.T) {
	id := uuid.New()
	hostID := uuid.New()
	svc := &mockAlertServicer{
		getFn: func(_ context.Context, _ uuid.UUID) (*db.AlertRule, error) {
			return makeAlertRule(id, hostID), nil
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/alerts/"+id.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.Equal(t, id.String(), raw["id"])
}

func TestGetAlertRule_BadUUID(t *testing.T) {
	svc := &mockAlertServicer{}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/alerts/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetAlertRule_NotFound(t *testing.T) {
	svc := &mockAlertServicer{
		getFn: func(_ context.Context, _ uuid.UUID) (*db.AlertRule, error) {
			return nil, apierrors.NewScanError(apierrors.CodeNotFound, "alert rule not found")
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/alerts/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── UpdateAlertRule ───────────────────────────────────────────────────────────

func TestUpdateAlertRule_Success(t *testing.T) {
	id := uuid.New()
	hostID := uuid.New()
	svc := &mockAlertServicer{
		updateFn: func(_ context.Context, _ uuid.UUID, _ db.UpdateAlertRuleInput) (*db.AlertRule, error) {
			r := makeAlertRule(id, hostID)
			r.Enabled = false
			return r, nil
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	body := `{"enabled":false}`
	req := httptest.NewRequest(http.MethodPatch, "/alerts/"+id.String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.Equal(t, false, raw["enabled"])
}

func TestUpdateAlertRule_BadUUID(t *testing.T) {
	svc := &mockAlertServicer{}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodPatch, "/alerts/not-a-uuid", bytes.NewBufferString(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateAlertRule_InvalidTrigger(t *testing.T) {
	svc := &mockAlertServicer{}
	router := newAlertRouter(newAlertHandler(svc))

	body := `{"trigger":"invalid"}`
	req := httptest.NewRequest(http.MethodPatch, "/alerts/"+uuid.New().String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateAlertRule_InvalidChannelURL(t *testing.T) {
	svc := &mockAlertServicer{}
	router := newAlertRouter(newAlertHandler(svc))

	body := `{"channel_url":"ftp://bad"}`
	req := httptest.NewRequest(http.MethodPatch, "/alerts/"+uuid.New().String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateAlertRule_NotFound(t *testing.T) {
	svc := &mockAlertServicer{
		updateFn: func(_ context.Context, _ uuid.UUID, _ db.UpdateAlertRuleInput) (*db.AlertRule, error) {
			return nil, apierrors.NewScanError(apierrors.CodeNotFound, "alert rule not found")
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	body := `{"enabled":false}`
	req := httptest.NewRequest(http.MethodPatch, "/alerts/"+uuid.New().String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdateAlertRule_ServiceError(t *testing.T) {
	svc := &mockAlertServicer{
		updateFn: func(_ context.Context, _ uuid.UUID, _ db.UpdateAlertRuleInput) (*db.AlertRule, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	body := `{"enabled":false}`
	req := httptest.NewRequest(http.MethodPatch, "/alerts/"+uuid.New().String(), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── DeleteAlertRule ───────────────────────────────────────────────────────────

func TestDeleteAlertRule_Success(t *testing.T) {
	id := uuid.New()
	svc := &mockAlertServicer{
		deleteFn: func(_ context.Context, _ uuid.UUID) error { return nil },
	}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodDelete, "/alerts/"+id.String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDeleteAlertRule_BadUUID(t *testing.T) {
	svc := &mockAlertServicer{}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodDelete, "/alerts/not-a-uuid", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeleteAlertRule_NotFound(t *testing.T) {
	svc := &mockAlertServicer{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return apierrors.NewScanError(apierrors.CodeNotFound, "alert rule not found")
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodDelete, "/alerts/"+uuid.New().String(), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── ListAlertRulesForHost ────────────────────────────────────────────────────

func TestListAlertRulesForHost_Success(t *testing.T) {
	hostID := uuid.New()
	ruleID := uuid.New()
	svc := &mockAlertServicer{
		listForHostFn: func(_ context.Context, _ uuid.UUID) ([]*db.AlertRule, error) {
			return []*db.AlertRule{makeAlertRule(ruleID, hostID)}, nil
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/hosts/"+hostID.String()+"/alerts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	rules, ok := raw["alert_rules"].([]any)
	require.True(t, ok)
	assert.Len(t, rules, 1)
}

func TestListAlertRulesForHost_BadUUID(t *testing.T) {
	svc := &mockAlertServicer{}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/hosts/not-a-uuid/alerts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListAlertRulesForHost_EmptyArray(t *testing.T) {
	hostID := uuid.New()
	svc := &mockAlertServicer{
		listForHostFn: func(_ context.Context, _ uuid.UUID) ([]*db.AlertRule, error) {
			return make([]*db.AlertRule, 0), nil
		},
	}
	router := newAlertRouter(newAlertHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/hosts/"+hostID.String()+"/alerts", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	// Verify the array serializes as [] not null
	body := w.Body.String()
	assert.Contains(t, body, `"alert_rules":[]`)
}
