package services

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/anstrom/scanorama/internal/db"
)

// Supported event type constants for webhook subscriptions.
const (
	EventHostOnline         = "host.online"
	EventHostOffline        = "host.offline"
	EventScanStarted        = "scan.started"
	EventScanCompleted      = "scan.completed"
	EventDiscoveryCompleted = "discovery.completed"
	eventWebhookTest        = "webhook.test"
)

// AllowedWebhookEvents is the set of event types that endpoints may subscribe to.
var AllowedWebhookEvents = map[string]struct{}{
	EventHostOnline:         {},
	EventHostOffline:        {},
	EventScanStarted:        {},
	EventScanCompleted:      {},
	EventDiscoveryCompleted: {},
}

const (
	webhookSecretBytes   = 32
	webhookMaxRetries    = 3
	webhookFirstAttempt  = 1
	webhookClientTimeout = 10 * time.Second
	webhookSigHeader     = "X-Scanorama-Signature"
	webhookSigPrefix     = "sha256="
)

// WebhookEvent represents a fired event to be delivered to subscribers.
type WebhookEvent struct {
	Type    string    `json:"type"`
	Payload any       `json:"payload"`
	FiredAt time.Time `json:"fired_at"`
}

// webhookRepository defines the data-access interface used by WebhookService.
//
//nolint:dupl // interface mirrors mock struct in tests by design
type webhookRepository interface {
	ListWebhooks(ctx context.Context) ([]*db.WebhookEndpoint, error)
	CreateWebhook(ctx context.Context, input db.CreateWebhookInput) (*db.WebhookEndpoint, error)
	GetWebhook(ctx context.Context, id uuid.UUID) (*db.WebhookEndpoint, error)
	UpdateWebhook(ctx context.Context, id uuid.UUID, input db.UpdateWebhookInput) (*db.WebhookEndpoint, error)
	DeleteWebhook(ctx context.Context, id uuid.UUID) error
	ListWebhooksByEvent(ctx context.Context, eventType string) ([]*db.WebhookEndpoint, error)
	CreateDeliveryLog(ctx context.Context, log db.WebhookDeliveryLog) error
	ListDeliveryLogs(ctx context.Context, endpointID uuid.UUID, limit int) ([]*db.WebhookDeliveryLog, error)
}

// WebhookService handles webhook management and event delivery.
type WebhookService struct {
	repo   webhookRepository
	client *http.Client
	logger *slog.Logger
}

// NewWebhookService creates a new WebhookService with a 10-second HTTP timeout.
func NewWebhookService(repo webhookRepository, logger *slog.Logger) *WebhookService {
	return &WebhookService{
		repo:   repo,
		client: &http.Client{Timeout: webhookClientTimeout},
		logger: logger.With("service", "webhook"),
	}
}

// ListWebhooks returns all registered webhook endpoints.
func (s *WebhookService) ListWebhooks(ctx context.Context) ([]*db.WebhookEndpoint, error) {
	return s.repo.ListWebhooks(ctx)
}

// CreateWebhook registers a new webhook endpoint. When input.Secret is empty,
// a random 32-byte hex-encoded secret is generated automatically.
func (s *WebhookService) CreateWebhook(
	ctx context.Context, input db.CreateWebhookInput,
) (*db.WebhookEndpoint, error) {
	if input.Secret == "" {
		secret, err := generateSecret()
		if err != nil {
			return nil, fmt.Errorf("generate webhook secret: %w", err)
		}
		input.Secret = secret
	}
	return s.repo.CreateWebhook(ctx, input)
}

// GetWebhook returns a single webhook endpoint by ID.
func (s *WebhookService) GetWebhook(ctx context.Context, id uuid.UUID) (*db.WebhookEndpoint, error) {
	return s.repo.GetWebhook(ctx, id)
}

// UpdateWebhook applies partial updates to a webhook endpoint.
func (s *WebhookService) UpdateWebhook(
	ctx context.Context, id uuid.UUID, input db.UpdateWebhookInput,
) (*db.WebhookEndpoint, error) {
	return s.repo.UpdateWebhook(ctx, id, input)
}

// DeleteWebhook removes a webhook endpoint.
func (s *WebhookService) DeleteWebhook(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteWebhook(ctx, id)
}

// SendTestDelivery fires a synchronous test event to the given endpoint.
// Returns an error if the endpoint responds with a non-2xx status.
func (s *WebhookService) SendTestDelivery(ctx context.Context, id uuid.UUID) error {
	ep, err := s.repo.GetWebhook(ctx, id)
	if err != nil {
		return fmt.Errorf("get webhook for test: %w", err)
	}

	event := WebhookEvent{
		Type:    eventWebhookTest,
		Payload: map[string]string{"message": "This is a test delivery from Scanorama."},
		FiredAt: time.Now().UTC(),
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal test event: %w", err)
	}

	code, deliveryErr := s.deliver(ctx, ep.URL, ep.Secret, payload)
	now := time.Now().UTC()

	log := db.WebhookDeliveryLog{
		EndpointID:   id,
		EventType:    eventWebhookTest,
		Payload:      payload,
		AttemptCount: webhookFirstAttempt,
		CreatedAt:    now,
	}
	if deliveryErr != nil {
		errMsg := deliveryErr.Error()
		log.LastError = &errMsg
	} else {
		log.StatusCode = &code
		log.DeliveredAt = &now
	}
	_ = s.repo.CreateDeliveryLog(ctx, log)

	return deliveryErr
}

// ListDeliveryLogs returns delivery logs for a webhook endpoint.
func (s *WebhookService) ListDeliveryLogs(
	ctx context.Context, endpointID uuid.UUID, limit int,
) ([]*db.WebhookDeliveryLog, error) {
	return s.repo.ListDeliveryLogs(ctx, endpointID, limit)
}

// DeliverToURL delivers a webhook event directly to a single URL, bypassing
// the subscriber registry. Used for targeted alert rule delivery where the
// destination is specified on the rule itself rather than the endpoint list.
// The POST is signed with an empty secret (no HMAC validation on the receiver
// side). DeliverToURL is synchronous; callers are responsible for goroutine
// dispatch if needed.
func (s *WebhookService) DeliverToURL(ctx context.Context, url string, event WebhookEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal alert event: %w", err)
	}
	_, err = s.deliver(ctx, url, "", payload)
	return err
}

// Deliver looks up all enabled endpoints subscribed to event.Type and fires
// async goroutines to deliver the event to each. It returns immediately.
func (s *WebhookService) Deliver(ctx context.Context, event WebhookEvent) error {
	endpoints, err := s.repo.ListWebhooksByEvent(ctx, event.Type)
	if err != nil {
		return fmt.Errorf("list webhooks for event %s: %w", event.Type, err)
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal webhook event: %w", err)
	}

	for _, ep := range endpoints {
		go s.deliverToEndpoint(ep, payload, event.Type)
	}
	return nil
}

// deliverToEndpoint delivers a payload to one endpoint with retry logic and
// records the result in the delivery log. It is safe to call as a goroutine.
func (s *WebhookService) deliverToEndpoint(ep *db.WebhookEndpoint, payload []byte, eventType string) {
	ctx, cancel := context.WithTimeout(context.Background(), webhookClientTimeout)
	defer cancel()

	var (
		lastCode       *int
		lastErr        *string
		delivered      *time.Time
		successAttempt int
	)

	for attempt := webhookFirstAttempt; attempt <= webhookMaxRetries; attempt++ {
		code, err := s.deliver(ctx, ep.URL, ep.Secret, payload)
		if err == nil {
			now := time.Now().UTC()
			lastCode = &code
			delivered = &now
			lastErr = nil
			successAttempt = attempt
			break
		}
		msg := err.Error()
		lastErr = &msg
		s.logger.Warn("webhook delivery attempt failed",
			"endpoint_id", ep.ID,
			"url", ep.URL,
			"attempt", attempt,
			"error", err,
		)
	}

	attemptCount := webhookMaxRetries
	if delivered != nil {
		attemptCount = successAttempt
	}
	log := db.WebhookDeliveryLog{
		EndpointID:   ep.ID,
		EventType:    eventType,
		Payload:      payload,
		StatusCode:   lastCode,
		AttemptCount: attemptCount,
		LastError:    lastErr,
		DeliveredAt:  delivered,
	}

	logCtx, logCancel := context.WithTimeout(context.Background(), webhookClientTimeout)
	defer logCancel()
	if err := s.repo.CreateDeliveryLog(logCtx, log); err != nil {
		s.logger.Error("failed to record webhook delivery log",
			"endpoint_id", ep.ID,
			"error", err,
		)
	}
}

// deliver sends a single HTTP POST with HMAC-SHA256 signature to url.
// Returns the status code and an error when the status is non-2xx.
func (s *WebhookService) deliver(ctx context.Context, url, secret string, payload []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhookSigHeader, webhookSigPrefix+computeHMAC(secret, payload))

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("http post: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp.StatusCode, fmt.Errorf("non-2xx response: %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// computeHMAC returns the hex-encoded HMAC-SHA256 of payload using secret.
func computeHMAC(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// generateSecret returns a random 32-byte hex-encoded secret string.
func generateSecret() (string, error) {
	b := make([]byte, webhookSecretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
