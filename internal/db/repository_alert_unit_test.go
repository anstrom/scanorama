// Package db — unit tests for AlertRepository using sqlmock.
// These run without a live database.
package db

import (
	"context"
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/errors"
)

// alertRuleCols mirrors the column list used by alert_rules SELECT queries.
var alertRuleCols = []string{
	"id", "host_id", "group_id", "tag",
	"trigger", "channel_type", "channel_url",
	"enabled", "created_at", "updated_at",
}

// ── NewAlertRepository ──────────────────────────────────────────────────────

func TestAlertRepository_New(t *testing.T) {
	db, _ := newMockDB(t)
	repo := NewAlertRepository(db)
	require.NotNil(t, repo)
}

// ── ListAlertRules ──────────────────────────────────────────────────────────

func TestAlertRepository_ListAlertRules_Empty(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewAlertRepository(database)

	mock.ExpectQuery("SELECT .* FROM alert_rules").
		WillReturnRows(sqlmock.NewRows(alertRuleCols))

	result, err := repo.ListAlertRules(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, result, "should return [] not nil")
	assert.Empty(t, result)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAlertRepository_ListAlertRules_WithRows(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewAlertRepository(database)

	now := time.Now().UTC()
	id1 := uuid.New()
	id2 := uuid.New()
	hostID := uuid.New()

	rows := sqlmock.NewRows(alertRuleCols).
		AddRow(id1, &hostID, nil, nil, "online", "webhook", "https://a.example.com", true, now, now).
		AddRow(id2, nil, nil, nil, "offline", "webhook", "https://b.example.com", false, now, now)

	mock.ExpectQuery("SELECT .* FROM alert_rules").
		WillReturnRows(rows)

	result, err := repo.ListAlertRules(context.Background())
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, id1, result[0].ID)
	assert.Equal(t, "online", result[0].Trigger)
	assert.Equal(t, id2, result[1].ID)
	assert.Equal(t, "offline", result[1].Trigger)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── CreateAlertRule ─────────────────────────────────────────────────────────

func TestAlertRepository_CreateAlertRule(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewAlertRepository(database)

	now := time.Now().UTC()
	id := uuid.New()
	hostID := uuid.New()

	rows := sqlmock.NewRows(alertRuleCols).
		AddRow(id, &hostID, nil, nil, "both", "webhook", "https://example.com/hook", true, now, now)

	mock.ExpectQuery("INSERT INTO alert_rules").
		WithArgs(&hostID, nil, nil, "both", "https://example.com/hook").
		WillReturnRows(rows)

	ar, err := repo.CreateAlertRule(context.Background(), CreateAlertRuleInput{
		HostID:     &hostID,
		Trigger:    "both",
		ChannelURL: "https://example.com/hook",
	})
	require.NoError(t, err)
	require.NotNil(t, ar)
	assert.Equal(t, id, ar.ID)
	assert.Equal(t, "both", ar.Trigger)
	assert.Equal(t, "https://example.com/hook", ar.ChannelURL)
	assert.True(t, ar.Enabled)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetAlertRule ────────────────────────────────────────────────────────────

func TestAlertRepository_GetAlertRule_Found(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewAlertRepository(database)

	now := time.Now().UTC()
	id := uuid.New()

	rows := sqlmock.NewRows(alertRuleCols).
		AddRow(id, nil, nil, nil, "online", "webhook", "https://example.com", true, now, now)

	mock.ExpectQuery("SELECT .* FROM alert_rules").
		WithArgs(id).
		WillReturnRows(rows)

	ar, err := repo.GetAlertRule(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, ar)
	assert.Equal(t, id, ar.ID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAlertRepository_GetAlertRule_NotFound(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewAlertRepository(database)

	id := uuid.New()

	mock.ExpectQuery("SELECT .* FROM alert_rules").
		WithArgs(id).
		WillReturnError(sql.ErrNoRows)

	_, err := repo.GetAlertRule(context.Background(), id)
	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound),
		"expected CodeNotFound, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── UpdateAlertRule ─────────────────────────────────────────────────────────

func TestAlertRepository_UpdateAlertRule_Found(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewAlertRepository(database)

	now := time.Now().UTC()
	id := uuid.New()
	newTrigger := "offline"
	newURL := "https://updated.example.com"
	enabled := false

	rows := sqlmock.NewRows(alertRuleCols).
		AddRow(id, nil, nil, nil, newTrigger, "webhook", newURL, enabled, now, now)

	mock.ExpectQuery("UPDATE alert_rules").
		WithArgs(id, &newTrigger, &newURL, &enabled).
		WillReturnRows(rows)

	ar, err := repo.UpdateAlertRule(context.Background(), id, UpdateAlertRuleInput{
		Trigger:    &newTrigger,
		ChannelURL: &newURL,
		Enabled:    &enabled,
	})
	require.NoError(t, err)
	require.NotNil(t, ar)
	assert.Equal(t, id, ar.ID)
	assert.Equal(t, "offline", ar.Trigger)
	assert.Equal(t, newURL, ar.ChannelURL)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAlertRepository_UpdateAlertRule_NotFound(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewAlertRepository(database)

	id := uuid.New()

	mock.ExpectQuery("UPDATE alert_rules").
		WithArgs(id, nil, nil, nil).
		WillReturnError(sql.ErrNoRows)

	_, err := repo.UpdateAlertRule(context.Background(), id, UpdateAlertRuleInput{})
	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound),
		"expected CodeNotFound, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── DeleteAlertRule ─────────────────────────────────────────────────────────

func TestAlertRepository_DeleteAlertRule_Found(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewAlertRepository(database)

	id := uuid.New()

	mock.ExpectExec("DELETE FROM alert_rules").
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.DeleteAlertRule(context.Background(), id)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAlertRepository_DeleteAlertRule_NotFound(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewAlertRepository(database)

	id := uuid.New()

	mock.ExpectExec("DELETE FROM alert_rules").
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.DeleteAlertRule(context.Background(), id)
	require.Error(t, err)
	assert.True(t, errors.IsCode(err, errors.CodeNotFound),
		"expected CodeNotFound, got: %v", err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetStatusTransitionsSince ───────────────────────────────────────────────

func TestAlertRepository_GetStatusTransitionsSince(t *testing.T) {
	database, mock := newMockDB(t)
	repo := NewAlertRepository(database)

	hostID := uuid.New()
	changedAt := time.Now().UTC()
	since := changedAt.Add(-time.Hour)

	// statusTransitionCols matches the SELECT columns in GetStatusTransitionsSince.
	statusTransitionCols := []string{"host_id", "from_status", "to_status", "changed_at", "tags"}

	rows := sqlmock.NewRows(statusTransitionCols).
		AddRow(hostID, "down", "up", changedAt, pq.Array([]string{"web", "prod"}))

	mock.ExpectQuery("SELECT .* FROM host_status_events").
		WithArgs(since).
		WillReturnRows(rows)

	result, err := repo.GetStatusTransitionsSince(context.Background(), since)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, hostID, result[0].HostID)
	assert.Equal(t, "down", result[0].FromStatus)
	assert.Equal(t, "up", result[0].ToStatus)
	assert.ElementsMatch(t, []string{"web", "prod"}, result[0].Tags)
	require.NoError(t, mock.ExpectationsWereMet())
}
