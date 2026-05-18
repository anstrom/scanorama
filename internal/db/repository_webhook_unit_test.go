// Package db — unit tests for WebhookRepository using sqlmock.
// These run without a live database.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// webhookCols mirrors the column list returned by webhook_endpoints SELECT queries.
var webhookCols = []string{"id", "url", "secret", "events", "enabled", "created_at", "updated_at"}

// deliveryLogCols mirrors the column list returned by webhook_delivery_log SELECT queries.
var deliveryLogCols = []string{
	"id", "endpoint_id", "event_type", "payload",
	"status_code", "attempt_count", "last_error", "delivered_at", "created_at",
}

// ── NewWebhookRepository ────────────────────────────────────────────────────

func TestWebhookRepository_New(t *testing.T) {
	db, _ := newMockDB(t)
	repo := NewWebhookRepository(db)
	require.NotNil(t, repo)
}

// ── ListWebhooks ────────────────────────────────────────────────────────────

func TestWebhookRepository_ListWebhooks_Empty(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewWebhookRepository(database)

	mock.ExpectQuery("SELECT .* FROM webhook_endpoints").
		WillReturnRows(sqlmock.NewRows(webhookCols))

	result, err := repo.ListWebhooks(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, result, "should return [] not nil")
	assert.Empty(t, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWebhookRepository_ListWebhooks_WithRows(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewWebhookRepository(database)

	now := time.Now().UTC()
	id1 := uuid.New()
	id2 := uuid.New()

	rows := sqlmock.NewRows(webhookCols).
		AddRow(id1, "https://a.example.com", "sec1",
			pq.Array([]string{"host.online"}), true, now, now).
		AddRow(id2, "https://b.example.com", "sec2",
			pq.Array([]string{"scan.completed", "host.offline"}), false, now, now)

	mock.ExpectQuery("SELECT .* FROM webhook_endpoints").
		WillReturnRows(rows)

	result, err := repo.ListWebhooks(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, id1, result[0].ID)
	assert.Equal(t, "https://a.example.com", result[0].URL)
	assert.Equal(t, id2, result[1].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── CreateWebhook ───────────────────────────────────────────────────────────

func TestWebhookRepository_CreateWebhook(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewWebhookRepository(database)

	now := time.Now().UTC()
	id := uuid.New()
	events := []string{"host.online", "host.offline"}

	rows := sqlmock.NewRows(webhookCols).
		AddRow(id, "https://example.com/hook", "mysecret",
			pq.Array(events), true, now, now)

	mock.ExpectQuery("INSERT INTO webhook_endpoints").
		WithArgs("https://example.com/hook", "mysecret", sqlmock.AnyArg()).
		WillReturnRows(rows)

	ep, err := repo.CreateWebhook(context.Background(), CreateWebhookInput{
		URL:    "https://example.com/hook",
		Secret: "mysecret",
		Events: events,
	})
	require.NoError(t, err)
	require.NotNil(t, ep)
	assert.Equal(t, id, ep.ID)
	assert.Equal(t, "https://example.com/hook", ep.URL)
	assert.Equal(t, "mysecret", ep.Secret)
	assert.True(t, ep.Enabled)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetWebhook ──────────────────────────────────────────────────────────────

func TestWebhookRepository_GetWebhook_Found(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewWebhookRepository(database)

	now := time.Now().UTC()
	id := uuid.New()

	rows := sqlmock.NewRows(webhookCols).
		AddRow(id, "https://example.com", "secret",
			pq.Array([]string{"host.online"}), true, now, now)

	mock.ExpectQuery("SELECT .* FROM webhook_endpoints").
		WithArgs(id).
		WillReturnRows(rows)

	ep, err := repo.GetWebhook(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, ep)
	assert.Equal(t, id, ep.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWebhookRepository_GetWebhook_NotFound(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewWebhookRepository(database)

	id := uuid.New()

	mock.ExpectQuery("SELECT .* FROM webhook_endpoints").
		WithArgs(id).
		WillReturnError(sql.ErrNoRows)

	_, err := repo.GetWebhook(context.Background(), id)
	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound),
		"expected CodeNotFound, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── DeleteWebhook ───────────────────────────────────────────────────────────

func TestWebhookRepository_DeleteWebhook_Found(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewWebhookRepository(database)

	id := uuid.New()

	mock.ExpectExec("DELETE FROM webhook_endpoints").
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.DeleteWebhook(context.Background(), id)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWebhookRepository_DeleteWebhook_NotFound(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewWebhookRepository(database)

	id := uuid.New()

	mock.ExpectExec("DELETE FROM webhook_endpoints").
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.DeleteWebhook(context.Background(), id)
	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound),
		"expected CodeNotFound, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListWebhooksByEvent ─────────────────────────────────────────────────────

func TestWebhookRepository_ListWebhooksByEvent(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewWebhookRepository(database)

	now := time.Now().UTC()
	id := uuid.New()

	rows := sqlmock.NewRows(webhookCols).
		AddRow(id, "https://example.com", "sec",
			pq.Array([]string{"host.online"}), true, now, now)

	mock.ExpectQuery("SELECT .* FROM webhook_endpoints").
		WithArgs("host.online").
		WillReturnRows(rows)

	result, err := repo.ListWebhooksByEvent(context.Background(), "host.online")
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, id, result[0].ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── CreateDeliveryLog ───────────────────────────────────────────────────────

func TestWebhookRepository_CreateDeliveryLog(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewWebhookRepository(database)

	endpointID := uuid.New()
	statusCode := 200
	now := time.Now().UTC()
	log := WebhookDeliveryLog{
		EndpointID:   endpointID,
		EventType:    "host.online",
		Payload:      []byte(`{"type":"host.online"}`),
		StatusCode:   &statusCode,
		AttemptCount: 1,
		DeliveredAt:  &now,
	}

	mock.ExpectExec("INSERT INTO webhook_delivery_log").
		WithArgs(
			log.EndpointID, log.EventType, log.Payload,
			log.StatusCode, log.AttemptCount, log.LastError, log.DeliveredAt,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.CreateDeliveryLog(context.Background(), log)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWebhookRepository_CreateDeliveryLog_DBError(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewWebhookRepository(database)

	mock.ExpectExec("INSERT INTO webhook_delivery_log").
		WillReturnError(fmt.Errorf("connection refused"))

	err := repo.CreateDeliveryLog(context.Background(), WebhookDeliveryLog{
		EndpointID:   uuid.New(),
		EventType:    "host.online",
		Payload:      []byte(`{}`),
		AttemptCount: 1,
	})
	require.Error(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListDeliveryLogs ────────────────────────────────────────────────────────

func TestWebhookRepository_ListDeliveryLogs(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewWebhookRepository(database)

	endpointID := uuid.New()
	logID := uuid.New()
	statusCode := 200
	now := time.Now().UTC()
	const deliveryLimit = 10

	rows := sqlmock.NewRows(deliveryLogCols).
		AddRow(
			logID, endpointID, "host.online", []byte(`{}`),
			&statusCode, 1, nil, &now, now,
		)

	mock.ExpectQuery("SELECT .* FROM webhook_delivery_log").
		WithArgs(endpointID, deliveryLimit).
		WillReturnRows(rows)

	logs, err := repo.ListDeliveryLogs(context.Background(), endpointID, deliveryLimit)
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, logID, logs[0].ID)
	assert.Equal(t, endpointID, logs[0].EndpointID)
	assert.Equal(t, "host.online", logs[0].EventType)
	require.NoError(t, mock.ExpectationsWereMet())
}
