package db

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/anstrom/scanorama/internal/errors"
)

// WebhookRepository handles all webhook-related DB operations.
type WebhookRepository struct {
	db *DB
}

// NewWebhookRepository creates a new WebhookRepository.
func NewWebhookRepository(db *DB) *WebhookRepository {
	return &WebhookRepository{db: db}
}

// ListWebhooks returns all webhook endpoints ordered by creation time.
func (r *WebhookRepository) ListWebhooks(ctx context.Context) ([]*WebhookEndpoint, error) {
	q := `SELECT id, url, secret, events, enabled, created_at, updated_at
	      FROM webhook_endpoints ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, sanitizeDBError("list webhooks", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]*WebhookEndpoint, 0)
	for rows.Next() {
		w := &WebhookEndpoint{}
		if err := rows.Scan(
			&w.ID, &w.URL, &w.Secret, pq.Array(&w.Events),
			&w.Enabled, &w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan webhook endpoint: %w", err)
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

// CreateWebhook inserts a new webhook endpoint.
func (r *WebhookRepository) CreateWebhook(ctx context.Context, input CreateWebhookInput) (*WebhookEndpoint, error) {
	q := `INSERT INTO webhook_endpoints (url, secret, events)
	      VALUES ($1, $2, $3)
	      RETURNING id, url, secret, events, enabled, created_at, updated_at`

	w := &WebhookEndpoint{}
	err := r.db.QueryRowContext(ctx, q, input.URL, input.Secret, pq.Array(input.Events)).Scan(
		&w.ID, &w.URL, &w.Secret, pq.Array(&w.Events),
		&w.Enabled, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return nil, sanitizeDBError("create webhook", err)
	}
	return w, nil
}

// GetWebhook retrieves a webhook endpoint by ID.
func (r *WebhookRepository) GetWebhook(ctx context.Context, id uuid.UUID) (*WebhookEndpoint, error) {
	q := `SELECT id, url, secret, events, enabled, created_at, updated_at
	      FROM webhook_endpoints WHERE id = $1`

	w := &WebhookEndpoint{}
	err := r.db.QueryRowContext(ctx, q, id).Scan(
		&w.ID, &w.URL, &w.Secret, pq.Array(&w.Events),
		&w.Enabled, &w.CreatedAt, &w.UpdatedAt,
	)
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil, errors.NewScanError(errors.CodeNotFound, "webhook not found")
	}
	if err != nil {
		return nil, sanitizeDBError("get webhook", err)
	}
	return w, nil
}

// UpdateWebhook applies partial updates to a webhook endpoint.
func (r *WebhookRepository) UpdateWebhook(
	ctx context.Context, id uuid.UUID, input UpdateWebhookInput,
) (*WebhookEndpoint, error) {
	var eventsArg interface{}
	if len(input.Events) > 0 {
		eventsArg = pq.Array(input.Events)
	}

	q := `UPDATE webhook_endpoints SET
	          url        = COALESCE($2, url),
	          secret     = COALESCE($3, secret),
	          events     = COALESCE($4, events),
	          enabled    = COALESCE($5, enabled),
	          updated_at = NOW()
	      WHERE id = $1
	      RETURNING id, url, secret, events, enabled, created_at, updated_at`

	w := &WebhookEndpoint{}
	err := r.db.QueryRowContext(ctx, q, id, input.URL, input.Secret, eventsArg, input.Enabled).Scan(
		&w.ID, &w.URL, &w.Secret, pq.Array(&w.Events),
		&w.Enabled, &w.CreatedAt, &w.UpdatedAt,
	)
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil, errors.NewScanError(errors.CodeNotFound, "webhook not found")
	}
	if err != nil {
		return nil, sanitizeDBError("update webhook", err)
	}
	return w, nil
}

// DeleteWebhook removes a webhook endpoint by ID.
func (r *WebhookRepository) DeleteWebhook(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM webhook_endpoints WHERE id = $1`, id)
	if err != nil {
		return sanitizeDBError("delete webhook", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.NewScanError(errors.CodeNotFound, "webhook not found")
	}
	return nil
}

// ListWebhooksByEvent returns all enabled webhook endpoints subscribed to the given event type.
func (r *WebhookRepository) ListWebhooksByEvent(ctx context.Context, eventType string) ([]*WebhookEndpoint, error) {
	q := `SELECT id, url, secret, events, enabled, created_at, updated_at
	      FROM webhook_endpoints
	      WHERE enabled = TRUE AND $1 = ANY(events)
	      ORDER BY created_at`

	rows, err := r.db.QueryContext(ctx, q, eventType)
	if err != nil {
		return nil, sanitizeDBError("list webhooks by event", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]*WebhookEndpoint, 0)
	for rows.Next() {
		w := &WebhookEndpoint{}
		if err := rows.Scan(
			&w.ID, &w.URL, &w.Secret, pq.Array(&w.Events),
			&w.Enabled, &w.CreatedAt, &w.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan webhook endpoint by event: %w", err)
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

// CreateDeliveryLog inserts a delivery log record.
func (r *WebhookRepository) CreateDeliveryLog(ctx context.Context, log WebhookDeliveryLog) error {
	q := `INSERT INTO webhook_delivery_log
	          (endpoint_id, event_type, payload, status_code, attempt_count, last_error, delivered_at)
	      VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.ExecContext(ctx, q,
		log.EndpointID, log.EventType, log.Payload,
		log.StatusCode, log.AttemptCount, log.LastError, log.DeliveredAt,
	)
	return sanitizeDBError("create delivery log", err)
}

// ListDeliveryLogs returns delivery logs for a webhook endpoint, newest first.
func (r *WebhookRepository) ListDeliveryLogs(
	ctx context.Context, endpointID uuid.UUID, limit int,
) ([]*WebhookDeliveryLog, error) {
	q := `SELECT id, endpoint_id, event_type, payload, status_code,
	             attempt_count, last_error, delivered_at, created_at
	      FROM webhook_delivery_log
	      WHERE endpoint_id = $1
	      ORDER BY created_at DESC
	      LIMIT $2`

	rows, err := r.db.QueryContext(ctx, q, endpointID, limit)
	if err != nil {
		return nil, sanitizeDBError("list delivery logs", err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]*WebhookDeliveryLog, 0)
	for rows.Next() {
		l := &WebhookDeliveryLog{}
		if err := rows.Scan(
			&l.ID, &l.EndpointID, &l.EventType, &l.Payload,
			&l.StatusCode, &l.AttemptCount, &l.LastError, &l.DeliveredAt, &l.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan delivery log: %w", err)
		}
		result = append(result, l)
	}
	return result, rows.Err()
}
