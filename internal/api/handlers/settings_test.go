package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ── Helpers ────────────────────────────────────────────────────────────────────

func newSettingsHandlerWithMock(t *testing.T) (*SettingsHandler, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	database := &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}
	repo := db.NewSettingsRepository(database)
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	return NewSettingsHandler(repo, logger), mock
}

var settingHandlerCols = []string{"key", "value", "description", "type", "updated_at"}

// ── GetSettings ────────────────────────────────────────────────────────────────

func TestSettingsHandler_GetSettings_Empty(t *testing.T) {
	h, mock := newSettingsHandlerWithMock(t)
	mock.ExpectQuery("SELECT key").WillReturnRows(sqlmock.NewRows(settingHandlerCols))

	req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	req = req.WithContext(context.WithValue(req.Context(), ContextKey("request_id"), "test-id"))
	rr := httptest.NewRecorder()
	h.GetSettings(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	settings := resp["settings"].([]interface{})
	assert.Empty(t, settings)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsHandler_GetSettings_ReturnsSettings(t *testing.T) {
	h, mock := newSettingsHandlerWithMock(t)
	rows := sqlmock.NewRows(settingHandlerCols).
		AddRow("scan.max_concurrent", "5", "Max concurrent scans", "int", time.Now().UTC()).
		AddRow("notifications.scan_complete", "true", "Notify on scan completion", "bool", time.Now().UTC())
	mock.ExpectQuery("SELECT key").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	req = req.WithContext(context.WithValue(req.Context(), ContextKey("request_id"), "test-id"))
	rr := httptest.NewRecorder()
	h.GetSettings(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var resp struct {
		Settings []db.Setting `json:"settings"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.Len(t, resp.Settings, 2)
	assert.Equal(t, "scan.max_concurrent", resp.Settings[0].Key)
	assert.Equal(t, "5", resp.Settings[0].Value)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsHandler_GetSettings_DBError(t *testing.T) {
	h, mock := newSettingsHandlerWithMock(t)
	mock.ExpectQuery("SELECT key").WillReturnError(fmt.Errorf("connection reset"))

	req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	req = req.WithContext(context.WithValue(req.Context(), ContextKey("request_id"), "test-id"))
	rr := httptest.NewRecorder()
	h.GetSettings(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── UpdateSettings ─────────────────────────────────────────────────────────────

func TestSettingsHandler_UpdateSettings_Success(t *testing.T) {
	h, mock := newSettingsHandlerWithMock(t)
	mock.ExpectExec("UPDATE settings").
		WithArgs("scan.max_concurrent", "10").
		WillReturnResult(sqlmock.NewResult(0, 1))

	body, _ := json.Marshal(map[string]string{"key": "scan.max_concurrent", "value": "10"})
	req := httptest.NewRequest(http.MethodPut, "/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ContextKey("request_id"), "test-id"))
	rr := httptest.NewRecorder()
	h.UpdateSettings(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "scan.max_concurrent", resp["key"])
	assert.Equal(t, true, resp["updated"])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsHandler_UpdateSettings_BoolValue(t *testing.T) {
	h, mock := newSettingsHandlerWithMock(t)
	mock.ExpectExec("UPDATE settings").
		WithArgs("notifications.scan_complete", "false").
		WillReturnResult(sqlmock.NewResult(0, 1))

	body, _ := json.Marshal(map[string]string{"key": "notifications.scan_complete", "value": "false"})
	req := httptest.NewRequest(http.MethodPut, "/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ContextKey("request_id"), "test-id"))
	rr := httptest.NewRecorder()
	h.UpdateSettings(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsHandler_UpdateSettings_MissingKey(t *testing.T) {
	h, _ := newSettingsHandlerWithMock(t)

	body, _ := json.Marshal(map[string]string{"value": "5"})
	req := httptest.NewRequest(http.MethodPut, "/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ContextKey("request_id"), "test-id"))
	rr := httptest.NewRecorder()
	h.UpdateSettings(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSettingsHandler_UpdateSettings_InvalidJSON(t *testing.T) {
	h, _ := newSettingsHandlerWithMock(t)

	body, _ := json.Marshal(map[string]string{"key": "scan.max_concurrent", "value": "not-json-bool"})
	req := httptest.NewRequest(http.MethodPut, "/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ContextKey("request_id"), "test-id"))
	rr := httptest.NewRecorder()
	h.UpdateSettings(rr, req)

	// "not-json-bool" is not valid JSON — it's not a number, bool, null, or quoted string
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSettingsHandler_UpdateSettings_NotFound(t *testing.T) {
	h, mock := newSettingsHandlerWithMock(t)
	mock.ExpectExec("UPDATE settings").
		WithArgs("unknown.key", "5").
		WillReturnResult(sqlmock.NewResult(0, 0)) // 0 rows = key not found

	body, _ := json.Marshal(map[string]string{"key": "unknown.key", "value": "5"})
	req := httptest.NewRequest(http.MethodPut, "/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ContextKey("request_id"), "test-id"))
	rr := httptest.NewRecorder()
	h.UpdateSettings(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsHandler_UpdateSettings_DBError(t *testing.T) {
	h, mock := newSettingsHandlerWithMock(t)
	mock.ExpectExec("UPDATE settings").
		WillReturnError(fmt.Errorf("connection reset"))

	body, _ := json.Marshal(map[string]string{"key": "scan.max_concurrent", "value": "5"})
	req := httptest.NewRequest(http.MethodPut, "/admin/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ContextKey("request_id"), "test-id"))
	rr := httptest.NewRecorder()
	h.UpdateSettings(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSettingsHandler_UpdateSettings_MalformedBody(t *testing.T) {
	h, _ := newSettingsHandlerWithMock(t)

	req := httptest.NewRequest(http.MethodPut, "/admin/settings", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(context.WithValue(req.Context(), ContextKey("request_id"), "test-id"))
	rr := httptest.NewRecorder()
	h.UpdateSettings(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
