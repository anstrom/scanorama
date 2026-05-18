package db

import (
	"time"

	"github.com/google/uuid"
)

// WebhookEndpoint represents a registered webhook delivery target.
type WebhookEndpoint struct {
	ID        uuid.UUID `db:"id"        json:"id"`
	URL       string    `db:"url"       json:"url"`
	Secret    string    `db:"secret"    json:"secret"`
	Events    []string  `db:"events"    json:"events"`
	Enabled   bool      `db:"enabled"   json:"enabled"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// WebhookDeliveryLog records a single delivery attempt for a webhook endpoint.
type WebhookDeliveryLog struct {
	ID           uuid.UUID  `db:"id"            json:"id"`
	EndpointID   uuid.UUID  `db:"endpoint_id"   json:"endpoint_id"`
	EventType    string     `db:"event_type"    json:"event_type"`
	Payload      []byte     `db:"payload"       json:"payload"`
	StatusCode   *int       `db:"status_code"   json:"status_code"`
	AttemptCount int        `db:"attempt_count" json:"attempt_count"`
	LastError    *string    `db:"last_error"    json:"last_error"`
	DeliveredAt  *time.Time `db:"delivered_at"  json:"delivered_at"`
	CreatedAt    time.Time  `db:"created_at"    json:"created_at"`
}

// CreateWebhookInput is the input for creating a new webhook endpoint.
type CreateWebhookInput struct {
	URL    string
	Secret string
	Events []string
}

// UpdateWebhookInput is the input for updating an existing webhook endpoint.
type UpdateWebhookInput struct {
	URL     *string
	Secret  *string
	Events  []string
	Enabled *bool
}
