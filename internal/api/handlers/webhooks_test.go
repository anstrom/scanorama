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

type mockWebhookServicer struct {
	listFn     func(ctx context.Context) ([]*db.WebhookEndpoint, error)
	createFn   func(ctx context.Context, input db.CreateWebhookInput) (*db.WebhookEndpoint, error)
	getFn      func(ctx context.Context, id uuid.UUID) (*db.WebhookEndpoint, error)
	updateFn   func(ctx context.Context, id uuid.UUID, input db.UpdateWebhookInput) (*db.WebhookEndpoint, error)
	deleteFn   func(ctx context.Context, id uuid.UUID) error
	testFn     func(ctx context.Context, id uuid.UUID) error
	listLogsFn func(ctx context.Context, endpointID uuid.UUID, limit int) ([]*db.WebhookDeliveryLog, error)
}

func (m *mockWebhookServicer) ListWebhooks(ctx context.Context) ([]*db.WebhookEndpoint, error) {
	return m.listFn(ctx)
}

func (m *mockWebhookServicer) CreateWebhook(
	ctx context.Context, input db.CreateWebhookInput,
) (*db.WebhookEndpoint, error) {
	return m.createFn(ctx, input)
}

func (m *mockWebhookServicer) GetWebhook(ctx context.Context, id uuid.UUID) (*db.WebhookEndpoint, error) {
	return m.getFn(ctx, id)
}

func (m *mockWebhookServicer) UpdateWebhook(
	ctx context.Context, id uuid.UUID, input db.UpdateWebhookInput,
) (*db.WebhookEndpoint, error) {
	return m.updateFn(ctx, id, input)
}

func (m *mockWebhookServicer) DeleteWebhook(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}

func (m *mockWebhookServicer) SendTestDelivery(ctx context.Context, id uuid.UUID) error {
	return m.testFn(ctx, id)
}

func (m *mockWebhookServicer) ListDeliveryLogs(
	ctx context.Context, endpointID uuid.UUID, limit int,
) ([]*db.WebhookDeliveryLog, error) {
	return m.listLogsFn(ctx, endpointID, limit)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newWebhookRouter(h *WebhookHandler) *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/webhooks", h.ListWebhooks).Methods(http.MethodGet)
	r.HandleFunc("/webhooks", h.CreateWebhook).Methods(http.MethodPost)
	r.HandleFunc("/webhooks/{id}", h.GetWebhook).Methods(http.MethodGet)
	r.HandleFunc("/webhooks/{id}", h.UpdateWebhook).Methods(http.MethodPatch)
	r.HandleFunc("/webhooks/{id}", h.DeleteWebhook).Methods(http.MethodDelete)
	r.HandleFunc("/webhooks/{id}/test", h.TestWebhook).Methods(http.MethodPost)
	r.HandleFunc("/webhooks/{id}/logs", h.ListDeliveryLogs).Methods(http.MethodGet)
	return r
}

func newWebhookHandler(svc *mockWebhookServicer) *WebhookHandler {
	return NewWebhookHandler(svc, createTestLogger(), metrics.NewRegistry())
}

func makeWebhookEndpoint(id uuid.UUID, url string) *db.WebhookEndpoint {
	return &db.WebhookEndpoint{
		ID:        id,
		URL:       url,
		Secret:    "s3cr3t",
		Events:    []string{"host.online"},
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func makeDeliveryLog(id, endpointID uuid.UUID) *db.WebhookDeliveryLog {
	code := http.StatusOK
	now := time.Now()
	return &db.WebhookDeliveryLog{
		ID:           id,
		EndpointID:   endpointID,
		EventType:    "host.online",
		Payload:      []byte(`{}`),
		StatusCode:   &code,
		AttemptCount: 1,
		DeliveredAt:  &now,
		CreatedAt:    time.Now(),
	}
}

func jsonBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewBuffer(b)
}

// ── ListWebhooks ──────────────────────────────────────────────────────────────

func TestListWebhooks_ReturnsArray(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		listFn: func(_ context.Context) ([]*db.WebhookEndpoint, error) {
			return []*db.WebhookEndpoint{makeWebhookEndpoint(id, "https://example.com/hook")}, nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	webhooks, ok := resp["webhooks"].([]any)
	require.True(t, ok, "expected webhooks key with JSON array")
	assert.Len(t, webhooks, 1)
	first := webhooks[0].(map[string]any)
	assert.Equal(t, "https://example.com/hook", first["url"])
}

func TestListWebhooks_EmptyIsArray(t *testing.T) {
	svc := &mockWebhookServicer{
		listFn: func(_ context.Context) ([]*db.WebhookEndpoint, error) {
			return make([]*db.WebhookEndpoint, 0), nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	webhooks, ok := resp["webhooks"].([]any)
	require.True(t, ok, "empty list must be [] not null")
	assert.Empty(t, webhooks)
}

func TestListWebhooks_ServiceError(t *testing.T) {
	svc := &mockWebhookServicer{
		listFn: func(_ context.Context) ([]*db.WebhookEndpoint, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── CreateWebhook ─────────────────────────────────────────────────────────────

func TestCreateWebhook_Success(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		createFn: func(_ context.Context, input db.CreateWebhookInput) (*db.WebhookEndpoint, error) {
			return makeWebhookEndpoint(id, input.URL), nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	body := jsonBody(t, map[string]any{
		"url":    "https://example.com/hook",
		"events": []string{"host.online"},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhooks", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "https://example.com/hook", resp["url"])
}

func TestCreateWebhook_BadURL(t *testing.T) {
	svc := &mockWebhookServicer{}
	r := newWebhookRouter(newWebhookHandler(svc))
	body := jsonBody(t, map[string]any{
		"url":    "ftp://bad-scheme.com",
		"events": []string{"host.online"},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhooks", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateWebhook_MissingURL(t *testing.T) {
	svc := &mockWebhookServicer{}
	r := newWebhookRouter(newWebhookHandler(svc))
	body := jsonBody(t, map[string]any{
		"url":    "",
		"events": []string{"host.online"},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhooks", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateWebhook_InvalidEvent(t *testing.T) {
	svc := &mockWebhookServicer{}
	r := newWebhookRouter(newWebhookHandler(svc))
	body := jsonBody(t, map[string]any{
		"url":    "https://example.com/hook",
		"events": []string{"not.a.real.event"},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhooks", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateWebhook_ServiceError(t *testing.T) {
	svc := &mockWebhookServicer{
		createFn: func(_ context.Context, _ db.CreateWebhookInput) (*db.WebhookEndpoint, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	body := jsonBody(t, map[string]any{
		"url":    "https://example.com/hook",
		"events": []string{"host.online"},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhooks", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

// ── GetWebhook ────────────────────────────────────────────────────────────────

func TestGetWebhook_Success(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		getFn: func(_ context.Context, gotID uuid.UUID) (*db.WebhookEndpoint, error) {
			assert.Equal(t, id, gotID)
			return makeWebhookEndpoint(id, "https://example.com/hook"), nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, id.String(), resp["id"])
}

func TestGetWebhook_BadUUID(t *testing.T) {
	svc := &mockWebhookServicer{}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks/not-a-uuid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetWebhook_NotFound(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		getFn: func(_ context.Context, _ uuid.UUID) (*db.WebhookEndpoint, error) {
			return nil, apierrors.NewScanError(apierrors.CodeNotFound, "webhook not found")
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── UpdateWebhook ─────────────────────────────────────────────────────────────

func TestUpdateWebhook_Success(t *testing.T) {
	id := uuid.New()
	newURL := "https://new.example.com/hook"
	enabled := true
	svc := &mockWebhookServicer{
		updateFn: func(_ context.Context, gotID uuid.UUID, input db.UpdateWebhookInput) (*db.WebhookEndpoint, error) {
			assert.Equal(t, id, gotID)
			ep := makeWebhookEndpoint(id, *input.URL)
			ep.Enabled = *input.Enabled
			return ep, nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	body := jsonBody(t, map[string]any{"url": newURL, "enabled": enabled})
	req := httptest.NewRequest(http.MethodPatch, "/webhooks/"+id.String(), body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, newURL, resp["url"])
}

func TestUpdateWebhook_BadUUID(t *testing.T) {
	svc := &mockWebhookServicer{}
	r := newWebhookRouter(newWebhookHandler(svc))
	body := jsonBody(t, map[string]any{"enabled": false})
	req := httptest.NewRequest(http.MethodPatch, "/webhooks/not-a-uuid", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateWebhook_ServiceError(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		updateFn: func(_ context.Context, _ uuid.UUID, _ db.UpdateWebhookInput) (*db.WebhookEndpoint, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	body := jsonBody(t, map[string]any{"enabled": false})
	req := httptest.NewRequest(http.MethodPatch, "/webhooks/"+id.String(), body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUpdateWebhook_NotFound(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		updateFn: func(_ context.Context, _ uuid.UUID, _ db.UpdateWebhookInput) (*db.WebhookEndpoint, error) {
			return nil, apierrors.NewScanError(apierrors.CodeNotFound, "webhook not found")
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	body := jsonBody(t, map[string]any{"enabled": false})
	req := httptest.NewRequest(http.MethodPatch, "/webhooks/"+id.String(), body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── DeleteWebhook ─────────────────────────────────────────────────────────────

func TestDeleteWebhook_Success(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		deleteFn: func(_ context.Context, gotID uuid.UUID) error {
			assert.Equal(t, id, gotID)
			return nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodDelete, "/webhooks/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDeleteWebhook_NotFound(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return apierrors.NewScanError(apierrors.CodeNotFound, "webhook not found")
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodDelete, "/webhooks/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── TestWebhook ───────────────────────────────────────────────────────────────

func TestTestWebhook_Success(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		testFn: func(_ context.Context, gotID uuid.UUID) error {
			assert.Equal(t, id, gotID)
			return nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodPost, "/webhooks/"+id.String()+"/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "delivered", resp["status"])
}

func TestTestWebhook_DeliveryFailure(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		testFn: func(_ context.Context, _ uuid.UUID) error {
			return fmt.Errorf("endpoint returned 500")
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodPost, "/webhooks/"+id.String()+"/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestTestWebhook_NotFound(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		testFn: func(_ context.Context, _ uuid.UUID) error {
			return apierrors.NewScanError(apierrors.CodeNotFound, "webhook not found")
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodPost, "/webhooks/"+id.String()+"/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// ── ListDeliveryLogs ──────────────────────────────────────────────────────────

func TestListDeliveryLogs_ReturnsArray(t *testing.T) {
	endpointID := uuid.New()
	logID := uuid.New()
	svc := &mockWebhookServicer{
		listLogsFn: func(_ context.Context, gotID uuid.UUID, limit int) ([]*db.WebhookDeliveryLog, error) {
			assert.Equal(t, endpointID, gotID)
			assert.Equal(t, defaultLogLimit, limit)
			return []*db.WebhookDeliveryLog{makeDeliveryLog(logID, endpointID)}, nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks/"+endpointID.String()+"/logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	logs, ok := resp["logs"].([]any)
	require.True(t, ok, "expected logs key with JSON array")
	assert.Len(t, logs, 1)
	first := logs[0].(map[string]any)
	assert.Equal(t, "host.online", first["event_type"])
}

func TestListDeliveryLogs_BadUUID(t *testing.T) {
	svc := &mockWebhookServicer{}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks/not-a-uuid/logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListDeliveryLogs_EmptyIsArray(t *testing.T) {
	endpointID := uuid.New()
	svc := &mockWebhookServicer{
		listLogsFn: func(_ context.Context, _ uuid.UUID, _ int) ([]*db.WebhookDeliveryLog, error) {
			return make([]*db.WebhookDeliveryLog, 0), nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks/"+endpointID.String()+"/logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	logs, ok := resp["logs"].([]any)
	require.True(t, ok, "empty log list must be [] not null")
	assert.Empty(t, logs)
}

func TestListDeliveryLogs_LimitFromQuery(t *testing.T) {
	endpointID := uuid.New()
	svc := &mockWebhookServicer{
		listLogsFn: func(_ context.Context, _ uuid.UUID, limit int) ([]*db.WebhookDeliveryLog, error) {
			assert.Equal(t, 100, limit)
			return make([]*db.WebhookDeliveryLog, 0), nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks/"+endpointID.String()+"/logs?limit=100", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestListDeliveryLogs_LimitCappedAtMax(t *testing.T) {
	endpointID := uuid.New()
	svc := &mockWebhookServicer{
		listLogsFn: func(_ context.Context, _ uuid.UUID, limit int) ([]*db.WebhookDeliveryLog, error) {
			assert.Equal(t, maxLogLimit, limit)
			return make([]*db.WebhookDeliveryLog, 0), nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks/"+endpointID.String()+"/logs?limit=9999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ── JSON key assertions ───────────────────────────────────────────────────────

func TestCreateWebhook_ResponseIncludesSecret(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		createFn: func(_ context.Context, input db.CreateWebhookInput) (*db.WebhookEndpoint, error) {
			ep := makeWebhookEndpoint(id, input.URL)
			ep.Secret = "supersecret"
			return ep, nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	body := jsonBody(t, map[string]any{
		"url":    "https://example.com/hook",
		"events": []string{"host.online"},
	})
	req := httptest.NewRequest(http.MethodPost, "/webhooks", body)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Contains(t, raw, "secret") // secret included only on creation
	assert.Equal(t, "supersecret", raw["secret"])
}

func TestWebhookEndpoint_JSONKeys(t *testing.T) {
	id := uuid.New()
	svc := &mockWebhookServicer{
		getFn: func(_ context.Context, _ uuid.UUID) (*db.WebhookEndpoint, error) {
			return makeWebhookEndpoint(id, "https://example.com/hook"), nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks/"+id.String(), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var raw map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	assert.Contains(t, raw, "id")
	assert.Contains(t, raw, "url")
	assert.NotContains(t, raw, "secret") // secret is omitted from GET responses
	assert.Contains(t, raw, "events")
	assert.Contains(t, raw, "enabled")
	assert.Contains(t, raw, "created_at")
	assert.Contains(t, raw, "updated_at")
}

func TestWebhookDeliveryLog_JSONKeys(t *testing.T) {
	endpointID := uuid.New()
	logID := uuid.New()
	svc := &mockWebhookServicer{
		listLogsFn: func(_ context.Context, _ uuid.UUID, _ int) ([]*db.WebhookDeliveryLog, error) {
			return []*db.WebhookDeliveryLog{makeDeliveryLog(logID, endpointID)}, nil
		},
	}
	r := newWebhookRouter(newWebhookHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/webhooks/"+endpointID.String()+"/logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	logs, ok := resp["logs"].([]any)
	require.True(t, ok)
	require.Len(t, logs, 1)
	raw := logs[0].(map[string]any)
	assert.Contains(t, raw, "id")
	assert.Contains(t, raw, "endpoint_id")
	assert.Contains(t, raw, "event_type")
	assert.Contains(t, raw, "status_code")
	assert.Contains(t, raw, "attempt_count")
	assert.Contains(t, raw, "last_error")
	assert.Contains(t, raw, "delivered_at")
	assert.Contains(t, raw, "created_at")
}
