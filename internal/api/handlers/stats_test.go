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

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
)

// ── Helpers ────────────────────────────────────────────────────────────────────

func newStatsHandlerWithMock(t *testing.T) (*StatsHandler, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	database := &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	return NewStatsHandler(database, logger), mock
}

func statsRequest(t *testing.T) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/stats/summary", nil)
	return req.WithContext(context.WithValue(req.Context(), ContextKey("request_id"), "test-id"))
}

// expectAllStatsQueries registers all five stats queries in order with happy-path defaults.
func expectAllStatsQueries(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("SELECT status, COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"status", "count"}).
			AddRow("up", 10).
			AddRow("down", 3))

	mock.ExpectQuery("SELECT os_family").
		WillReturnRows(sqlmock.NewRows([]string{"os_family", "cnt"}).
			AddRow("Linux", 8).
			AddRow("Windows", 5))

	mock.ExpectQuery("SELECT port").
		WillReturnRows(sqlmock.NewRows([]string{"port", "cnt"}).
			AddRow(80, 12).
			AddRow(443, 9).
			AddRow(22, 6))

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	mock.ExpectQuery("SELECT AVG").
		WillReturnRows(sqlmock.NewRows([]string{"avg"}).AddRow(45.5))
}

// ── GetStatsSummary ────────────────────────────────────────────────────────────

func TestStatsHandler_GetStatsSummary_Success(t *testing.T) {
	h, mock := newStatsHandlerWithMock(t)
	expectAllStatsQueries(mock)

	rr := httptest.NewRecorder()
	h.GetStatsSummary(rr, statsRequest(t))

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var resp StatsSummaryResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))

	assert.Equal(t, 10, resp.HostsByStatus["up"])
	assert.Equal(t, 3, resp.HostsByStatus["down"])
	require.Len(t, resp.HostsByOSFamily, 2)
	assert.Equal(t, "Linux", resp.HostsByOSFamily[0].Family)
	assert.Equal(t, 8, resp.HostsByOSFamily[0].Count)
	require.Len(t, resp.TopPorts, 3)
	assert.Equal(t, 80, resp.TopPorts[0].Port)
	assert.Equal(t, 12, resp.TopPorts[0].Count)
	assert.Equal(t, 2, resp.StaleHostCount)
	assert.InDelta(t, 45.5, resp.AvgScanDurationS, 0.01)

	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStatsHandler_GetStatsSummary_EmptyDB(t *testing.T) {
	h, mock := newStatsHandlerWithMock(t)

	mock.ExpectQuery("SELECT status, COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))
	mock.ExpectQuery("SELECT os_family").
		WillReturnRows(sqlmock.NewRows([]string{"os_family", "cnt"}))
	mock.ExpectQuery("SELECT port").
		WillReturnRows(sqlmock.NewRows([]string{"port", "cnt"}))
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT AVG").
		WillReturnRows(sqlmock.NewRows([]string{"avg"}).AddRow(nil)) // NULL avg

	rr := httptest.NewRecorder()
	h.GetStatsSummary(rr, statsRequest(t))

	require.Equal(t, http.StatusOK, rr.Code)
	var resp StatsSummaryResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))

	assert.Empty(t, resp.HostsByStatus)
	assert.Empty(t, resp.HostsByOSFamily)
	assert.Empty(t, resp.TopPorts)
	assert.Equal(t, 0, resp.StaleHostCount)
	assert.Equal(t, 0.0, resp.AvgScanDurationS)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStatsHandler_GetStatsSummary_StatusQueryError(t *testing.T) {
	h, mock := newStatsHandlerWithMock(t)
	mock.ExpectQuery("SELECT status, COUNT").WillReturnError(fmt.Errorf("db error"))

	rr := httptest.NewRecorder()
	h.GetStatsSummary(rr, statsRequest(t))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStatsHandler_GetStatsSummary_OSFamilyQueryError(t *testing.T) {
	h, mock := newStatsHandlerWithMock(t)
	mock.ExpectQuery("SELECT status, COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))
	mock.ExpectQuery("SELECT os_family").WillReturnError(fmt.Errorf("db error"))

	rr := httptest.NewRecorder()
	h.GetStatsSummary(rr, statsRequest(t))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStatsHandler_GetStatsSummary_TopPortsQueryError(t *testing.T) {
	h, mock := newStatsHandlerWithMock(t)
	mock.ExpectQuery("SELECT status, COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))
	mock.ExpectQuery("SELECT os_family").
		WillReturnRows(sqlmock.NewRows([]string{"os_family", "cnt"}))
	mock.ExpectQuery("SELECT port").WillReturnError(fmt.Errorf("db error"))

	rr := httptest.NewRecorder()
	h.GetStatsSummary(rr, statsRequest(t))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStatsHandler_GetStatsSummary_StaleHostQueryError(t *testing.T) {
	h, mock := newStatsHandlerWithMock(t)
	mock.ExpectQuery("SELECT status, COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))
	mock.ExpectQuery("SELECT os_family").
		WillReturnRows(sqlmock.NewRows([]string{"os_family", "cnt"}))
	mock.ExpectQuery("SELECT port").
		WillReturnRows(sqlmock.NewRows([]string{"port", "cnt"}))
	mock.ExpectQuery("SELECT COUNT").WillReturnError(fmt.Errorf("db error"))

	rr := httptest.NewRecorder()
	h.GetStatsSummary(rr, statsRequest(t))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStatsHandler_GetStatsSummary_AvgDurationQueryError(t *testing.T) {
	h, mock := newStatsHandlerWithMock(t)
	mock.ExpectQuery("SELECT status, COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"status", "count"}))
	mock.ExpectQuery("SELECT os_family").
		WillReturnRows(sqlmock.NewRows([]string{"os_family", "cnt"}))
	mock.ExpectQuery("SELECT port").
		WillReturnRows(sqlmock.NewRows([]string{"port", "cnt"}))
	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT AVG").WillReturnError(fmt.Errorf("db error"))

	rr := httptest.NewRecorder()
	h.GetStatsSummary(rr, statsRequest(t))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestStatsHandler_GetStatsSummary_ResponseShape(t *testing.T) {
	h, mock := newStatsHandlerWithMock(t)
	expectAllStatsQueries(mock)

	rr := httptest.NewRecorder()
	h.GetStatsSummary(rr, statsRequest(t))

	require.Equal(t, http.StatusOK, rr.Code)

	// Verify JSON field names match the documented API shape.
	var raw map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&raw))
	assert.Contains(t, raw, "hosts_by_status")
	assert.Contains(t, raw, "hosts_by_os_family")
	assert.Contains(t, raw, "top_ports")
	assert.Contains(t, raw, "stale_host_count")
	assert.Contains(t, raw, "avg_scan_duration_s")
}

// ── NewStatsHandler ────────────────────────────────────────────────────────────

func TestNewStatsHandler(t *testing.T) {
	sqlDB, _, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	database := &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	h := NewStatsHandler(database, logger)
	require.NotNil(t, h)
}
