// Package services — unit tests for WebhookService.
// These run without a live database or external HTTP server.
package services

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ---------------------------------------------------------------------------
// Hand-rolled mock for webhookRepository
// ---------------------------------------------------------------------------

type mockWebhookRepo struct {
	listWebhooksFn  func(ctx context.Context) ([]*db.WebhookEndpoint, error)
	createWebhookFn func(ctx context.Context, input db.CreateWebhookInput) (*db.WebhookEndpoint, error)
	getWebhookFn    func(ctx context.Context, id uuid.UUID) (*db.WebhookEndpoint, error)
	updateWebhookFn func(
		ctx context.Context, id uuid.UUID, input db.UpdateWebhookInput,
	) (*db.WebhookEndpoint, error)
	deleteWebhookFn       func(ctx context.Context, id uuid.UUID) error
	listWebhooksByEventFn func(ctx context.Context, eventType string) ([]*db.WebhookEndpoint, error)
	createDeliveryLogFn   func(ctx context.Context, log db.WebhookDeliveryLog) error
	listDeliveryLogsFn    func(ctx context.Context, endpointID uuid.UUID, limit int) ([]*db.WebhookDeliveryLog, error)
}

func (m *mockWebhookRepo) ListWebhooks(ctx context.Context) ([]*db.WebhookEndpoint, error) {
	if m.listWebhooksFn != nil {
		return m.listWebhooksFn(ctx)
	}
	return make([]*db.WebhookEndpoint, 0), nil
}

func (m *mockWebhookRepo) CreateWebhook(
	ctx context.Context, input db.CreateWebhookInput,
) (*db.WebhookEndpoint, error) {
	if m.createWebhookFn != nil {
		return m.createWebhookFn(ctx, input)
	}
	return nil, nil
}

func (m *mockWebhookRepo) GetWebhook(ctx context.Context, id uuid.UUID) (*db.WebhookEndpoint, error) {
	if m.getWebhookFn != nil {
		return m.getWebhookFn(ctx, id)
	}
	return nil, nil
}

func (m *mockWebhookRepo) UpdateWebhook(
	ctx context.Context, id uuid.UUID, input db.UpdateWebhookInput,
) (*db.WebhookEndpoint, error) {
	if m.updateWebhookFn != nil {
		return m.updateWebhookFn(ctx, id, input)
	}
	return nil, nil
}

func (m *mockWebhookRepo) DeleteWebhook(ctx context.Context, id uuid.UUID) error {
	if m.deleteWebhookFn != nil {
		return m.deleteWebhookFn(ctx, id)
	}
	return nil
}

func (m *mockWebhookRepo) ListWebhooksByEvent(
	ctx context.Context, eventType string,
) ([]*db.WebhookEndpoint, error) {
	if m.listWebhooksByEventFn != nil {
		return m.listWebhooksByEventFn(ctx, eventType)
	}
	return make([]*db.WebhookEndpoint, 0), nil
}

func (m *mockWebhookRepo) CreateDeliveryLog(ctx context.Context, log db.WebhookDeliveryLog) error {
	if m.createDeliveryLogFn != nil {
		return m.createDeliveryLogFn(ctx, log)
	}
	return nil
}

func (m *mockWebhookRepo) ListDeliveryLogs(
	ctx context.Context, endpointID uuid.UUID, limit int,
) ([]*db.WebhookDeliveryLog, error) {
	if m.listDeliveryLogsFn != nil {
		return m.listDeliveryLogsFn(ctx, endpointID, limit)
	}
	return make([]*db.WebhookDeliveryLog, 0), nil
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func webhookDiscardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ---------------------------------------------------------------------------
// CreateWebhook
// ---------------------------------------------------------------------------

func TestWebhookService_CreateWebhook_GeneratesSecret(t *testing.T) {
	var captured db.CreateWebhookInput

	repo := &mockWebhookRepo{
		createWebhookFn: func(_ context.Context, input db.CreateWebhookInput) (*db.WebhookEndpoint, error) {
			captured = input
			return &db.WebhookEndpoint{
				ID:     uuid.New(),
				URL:    input.URL,
				Secret: input.Secret,
				Events: input.Events,
			}, nil
		},
	}
	svc := NewWebhookService(repo, webhookDiscardLogger())

	_, err := svc.CreateWebhook(context.Background(), db.CreateWebhookInput{
		URL:    "https://example.com/hook",
		Events: []string{EventHostOnline},
	})
	require.NoError(t, err)

	// Secret must be auto-generated: 32 bytes = 64 hex chars.
	assert.NotEmpty(t, captured.Secret)
	assert.Len(t, captured.Secret, 64)
}

func TestWebhookService_CreateWebhook_UsesProvidedSecret(t *testing.T) {
	var captured db.CreateWebhookInput

	repo := &mockWebhookRepo{
		createWebhookFn: func(_ context.Context, input db.CreateWebhookInput) (*db.WebhookEndpoint, error) {
			captured = input
			return &db.WebhookEndpoint{
				ID:     uuid.New(),
				URL:    input.URL,
				Secret: input.Secret,
				Events: input.Events,
			}, nil
		},
	}
	svc := NewWebhookService(repo, webhookDiscardLogger())

	_, err := svc.CreateWebhook(context.Background(), db.CreateWebhookInput{
		URL:    "https://example.com/hook",
		Secret: "mysecret",
		Events: []string{EventHostOnline},
	})
	require.NoError(t, err)
	assert.Equal(t, "mysecret", captured.Secret)
}

// ---------------------------------------------------------------------------
// ListWebhooks
// ---------------------------------------------------------------------------

func TestWebhookService_ListWebhooks(t *testing.T) {
	ep1 := &db.WebhookEndpoint{ID: uuid.New(), URL: "https://a.example.com"}
	ep2 := &db.WebhookEndpoint{ID: uuid.New(), URL: "https://b.example.com"}

	repo := &mockWebhookRepo{
		listWebhooksFn: func(_ context.Context) ([]*db.WebhookEndpoint, error) {
			return []*db.WebhookEndpoint{ep1, ep2}, nil
		},
	}
	svc := NewWebhookService(repo, webhookDiscardLogger())

	result, err := svc.ListWebhooks(context.Background())
	require.NoError(t, err)
	assert.Len(t, result, 2)
	assert.Equal(t, ep1.ID, result[0].ID)
	assert.Equal(t, ep2.ID, result[1].ID)
}

// ---------------------------------------------------------------------------
// GetWebhook
// ---------------------------------------------------------------------------

func TestWebhookService_GetWebhook(t *testing.T) {
	id := uuid.New()
	ep := &db.WebhookEndpoint{ID: id, URL: "https://example.com"}

	repo := &mockWebhookRepo{
		getWebhookFn: func(_ context.Context, got uuid.UUID) (*db.WebhookEndpoint, error) {
			assert.Equal(t, id, got)
			return ep, nil
		},
	}
	svc := NewWebhookService(repo, webhookDiscardLogger())

	result, err := svc.GetWebhook(context.Background(), id)
	require.NoError(t, err)
	assert.Equal(t, ep, result)
}

// ---------------------------------------------------------------------------
// DeleteWebhook
// ---------------------------------------------------------------------------

func TestWebhookService_DeleteWebhook(t *testing.T) {
	id := uuid.New()
	var deletedID uuid.UUID

	repo := &mockWebhookRepo{
		deleteWebhookFn: func(_ context.Context, got uuid.UUID) error {
			deletedID = got
			return nil
		},
	}
	svc := NewWebhookService(repo, webhookDiscardLogger())

	require.NoError(t, svc.DeleteWebhook(context.Background(), id))
	assert.Equal(t, id, deletedID)
}

// ---------------------------------------------------------------------------
// Deliver
// ---------------------------------------------------------------------------

func TestWebhookService_Deliver_FansOutToSubscribers(t *testing.T) {
	// Track how many POST requests arrive at each test server.
	var hits atomic.Int32

	makeServer := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.NotEmpty(t, r.Header.Get(webhookSigHeader))
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
	}

	ts1 := makeServer()
	defer ts1.Close()
	ts2 := makeServer()
	defer ts2.Close()

	ep1 := &db.WebhookEndpoint{ID: uuid.New(), URL: ts1.URL, Secret: "sec1", Enabled: true}
	ep2 := &db.WebhookEndpoint{ID: uuid.New(), URL: ts2.URL, Secret: "sec2", Enabled: true}

	// Channel to detect when both delivery logs have been written.
	logsDone := make(chan struct{}, 2)

	repo := &mockWebhookRepo{
		listWebhooksByEventFn: func(_ context.Context, _ string) ([]*db.WebhookEndpoint, error) {
			return []*db.WebhookEndpoint{ep1, ep2}, nil
		},
		createDeliveryLogFn: func(_ context.Context, _ db.WebhookDeliveryLog) error {
			logsDone <- struct{}{}
			return nil
		},
	}
	svc := NewWebhookService(repo, webhookDiscardLogger())

	err := svc.Deliver(context.Background(), WebhookEvent{
		Type:    EventHostOnline,
		Payload: map[string]string{"host": "10.0.0.1"},
		FiredAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	// Wait for both goroutines to finish (via delivery log writes).
	for i := 0; i < 2; i++ {
		select {
		case <-logsDone:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for webhook delivery goroutines")
		}
	}

	assert.Equal(t, int32(2), hits.Load())
}

func TestWebhookService_Deliver_EmptySubscriberList(t *testing.T) {
	var httpCalls atomic.Int32

	repo := &mockWebhookRepo{
		listWebhooksByEventFn: func(_ context.Context, _ string) ([]*db.WebhookEndpoint, error) {
			return make([]*db.WebhookEndpoint, 0), nil
		},
		createDeliveryLogFn: func(_ context.Context, _ db.WebhookDeliveryLog) error {
			httpCalls.Add(1)
			return nil
		},
	}
	svc := NewWebhookService(repo, webhookDiscardLogger())

	err := svc.Deliver(context.Background(), WebhookEvent{
		Type:    EventScanCompleted,
		Payload: nil,
		FiredAt: time.Now().UTC(),
	})
	require.NoError(t, err)

	// Give goroutines time to run (there should be none).
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, int32(0), httpCalls.Load())
}

func TestWebhookService_Deliver_RepoError(t *testing.T) {
	repo := &mockWebhookRepo{
		listWebhooksByEventFn: func(_ context.Context, _ string) ([]*db.WebhookEndpoint, error) {
			return nil, errList
		},
	}
	svc := NewWebhookService(repo, webhookDiscardLogger())

	err := svc.Deliver(context.Background(), WebhookEvent{
		Type:    EventHostOnline,
		Payload: nil,
		FiredAt: time.Now().UTC(),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list webhooks for event")
}

// errList is a sentinel error used in tests.
var errList = newSentinelError("list error")

type sentinelError struct{ msg string }

func (e sentinelError) Error() string { return e.msg }

func newSentinelError(msg string) sentinelError { return sentinelError{msg: msg} }

// ---------------------------------------------------------------------------
// SendTestDelivery
// ---------------------------------------------------------------------------

func TestWebhookService_SendTestDelivery_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	endpointID := uuid.New()
	var logCreated bool
	var loggedCode *int

	repo := &mockWebhookRepo{
		getWebhookFn: func(_ context.Context, _ uuid.UUID) (*db.WebhookEndpoint, error) {
			return &db.WebhookEndpoint{ID: endpointID, URL: ts.URL, Secret: "secret"}, nil
		},
		createDeliveryLogFn: func(_ context.Context, log db.WebhookDeliveryLog) error {
			logCreated = true
			loggedCode = log.StatusCode
			return nil
		},
	}
	svc := NewWebhookService(repo, webhookDiscardLogger())

	err := svc.SendTestDelivery(context.Background(), endpointID)
	require.NoError(t, err)
	assert.True(t, logCreated)
	require.NotNil(t, loggedCode)
	assert.Equal(t, http.StatusOK, *loggedCode)
}

func TestWebhookService_SendTestDelivery_Non2xx(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	endpointID := uuid.New()

	repo := &mockWebhookRepo{
		getWebhookFn: func(_ context.Context, _ uuid.UUID) (*db.WebhookEndpoint, error) {
			return &db.WebhookEndpoint{ID: endpointID, URL: ts.URL, Secret: "secret"}, nil
		},
		createDeliveryLogFn: func(_ context.Context, _ db.WebhookDeliveryLog) error {
			return nil
		},
	}
	svc := NewWebhookService(repo, webhookDiscardLogger())

	err := svc.SendTestDelivery(context.Background(), endpointID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-2xx")
}

// ---------------------------------------------------------------------------
// computeHMAC
// ---------------------------------------------------------------------------

func TestComputeHMAC(t *testing.T) {
	secret := "my-secret"
	payload := []byte(`{"type":"host.online"}`)

	sig1 := computeHMAC(secret, payload)
	sig2 := computeHMAC(secret, payload)

	assert.Equal(t, sig1, sig2, "HMAC must be deterministic")
	assert.NotEmpty(t, sig1)

	// Different secret → different signature.
	sigOther := computeHMAC("other-secret", payload)
	assert.NotEqual(t, sig1, sigOther)
}
