// Package handlers — unit tests for PortHandler using sqlmock.
package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

// ── Helpers ────────────────────────────────────────────────────────────────────

func newPortHandlerWithMock(t *testing.T) (*PortHandler, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	database := &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}
	repo := db.NewPortRepository(database)
	return NewPortHandler(repo, createTestLogger(), metrics.NewRegistry()), mock
}

var portHandlerCols = []string{"port", "protocol", "service", "description", "category", "os_families", "is_standard"}

func withRequestID(r *http.Request) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), ContextKey("request_id"), "test-id"))
}

// ── NewPortHandler ─────────────────────────────────────────────────────────────

func TestPortHandler_New(t *testing.T) {
	h, _ := newPortHandlerWithMock(t)
	require.NotNil(t, h)
}

// ── ListPorts ─────────────────────────────────────────────────────────────────

func TestPortHandler_ListPorts_OK(t *testing.T) {
	h, mock := newPortHandlerWithMock(t)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	rows := sqlmock.NewRows(portHandlerCols).
		AddRow(80, "tcp", "http", "Hypertext Transfer Protocol", "web", nil, true).
		AddRow(443, "tcp", "https", "HTTP over TLS", "web", nil, true)
	mock.ExpectQuery("SELECT port").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ports", nil)
	req = withRequestID(req)
	rr := httptest.NewRecorder()
	h.ListPorts(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var resp struct {
		Ports      []db.PortDefinition `json:"ports"`
		Total      int64               `json:"total"`
		Page       int                 `json:"page"`
		PageSize   int                 `json:"page_size"`
		TotalPages int                 `json:"total_pages"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.Len(t, resp.Ports, 2)
	assert.Equal(t, int64(2), resp.Total)
	assert.Equal(t, 1, resp.Page)
	assert.GreaterOrEqual(t, resp.PageSize, 1)
	assert.GreaterOrEqual(t, resp.TotalPages, 1)
	assert.Equal(t, "http", resp.Ports[0].Service)
	assert.Equal(t, "https", resp.Ports[1].Service)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortHandler_ListPorts_DBError(t *testing.T) {
	h, mock := newPortHandlerWithMock(t)

	mock.ExpectQuery("SELECT COUNT").
		WillReturnError(fmt.Errorf("connection reset"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ports", nil)
	req = withRequestID(req)
	rr := httptest.NewRecorder()
	h.ListPorts(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── GetPort ───────────────────────────────────────────────────────────────────

func makeGetPortRequest(portStr, protocol string) *http.Request {
	url := "/api/v1/ports/" + portStr
	if protocol != "" {
		url += "?protocol=" + protocol
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = withRequestID(req)
	vars := map[string]string{"port": portStr}
	return mux.SetURLVars(req, vars)
}

func TestPortHandler_GetPort_OK(t *testing.T) {
	h, mock := newPortHandlerWithMock(t)

	mock.ExpectQuery("SELECT port").
		WithArgs(80, "tcp").
		WillReturnRows(sqlmock.NewRows(portHandlerCols).
			AddRow(80, "tcp", "http", "Hypertext Transfer Protocol", "web", nil, true))

	req := makeGetPortRequest("80", "tcp")
	rr := httptest.NewRecorder()
	h.GetPort(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var def db.PortDefinition
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&def))
	assert.Equal(t, 80, def.Port)
	assert.Equal(t, "tcp", def.Protocol)
	assert.Equal(t, "http", def.Service)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortHandler_GetPort_InvalidPort_Zero(t *testing.T) {
	h, _ := newPortHandlerWithMock(t)

	req := makeGetPortRequest("0", "tcp")
	rr := httptest.NewRecorder()
	h.GetPort(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPortHandler_GetPort_InvalidPort_TooLarge(t *testing.T) {
	h, _ := newPortHandlerWithMock(t)

	req := makeGetPortRequest("99999", "tcp")
	rr := httptest.NewRecorder()
	h.GetPort(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPortHandler_GetPort_InvalidPort_String(t *testing.T) {
	h, _ := newPortHandlerWithMock(t)

	req := makeGetPortRequest("abc", "tcp")
	rr := httptest.NewRecorder()
	h.GetPort(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPortHandler_GetPort_InvalidProtocol(t *testing.T) {
	h, _ := newPortHandlerWithMock(t)

	req := makeGetPortRequest("80", "ftp")
	rr := httptest.NewRecorder()
	h.GetPort(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPortHandler_GetPort_NotFound(t *testing.T) {
	h, mock := newPortHandlerWithMock(t)

	mock.ExpectQuery("SELECT port").
		WithArgs(9999, "tcp").
		WillReturnRows(sqlmock.NewRows(portHandlerCols))

	req := makeGetPortRequest("9999", "tcp")
	rr := httptest.NewRecorder()
	h.GetPort(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortHandler_GetPort_DBError(t *testing.T) {
	h, mock := newPortHandlerWithMock(t)

	mock.ExpectQuery("SELECT port").
		WithArgs(80, "tcp").
		WillReturnError(fmt.Errorf("connection refused"))

	req := makeGetPortRequest("80", "tcp")
	rr := httptest.NewRecorder()
	h.GetPort(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortHandler_GetPort_DefaultProtocol(t *testing.T) {
	h, mock := newPortHandlerWithMock(t)

	// No ?protocol= param — should default to tcp
	mock.ExpectQuery("SELECT port").
		WithArgs(80, "tcp").
		WillReturnRows(sqlmock.NewRows(portHandlerCols).
			AddRow(80, "tcp", "http", "Hypertext Transfer Protocol", "web", nil, true))

	// Build request without protocol query param
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ports/80", nil)
	req = withRequestID(req)
	req = mux.SetURLVars(req, map[string]string{"port": "80"})
	rr := httptest.NewRecorder()
	h.GetPort(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var def db.PortDefinition
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&def))
	assert.Equal(t, "tcp", def.Protocol)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListPortCategories ─────────────────────────────────────────────────────────

func TestPortHandler_ListPortCategories_OK(t *testing.T) {
	h, mock := newPortHandlerWithMock(t)

	mock.ExpectQuery("SELECT DISTINCT category").
		WillReturnRows(sqlmock.NewRows([]string{"category"}).
			AddRow("database").
			AddRow("web").
			AddRow("windows"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ports/categories", nil)
	req = withRequestID(req)
	rr := httptest.NewRecorder()
	h.ListPortCategories(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	cats, ok := resp["categories"].([]interface{})
	require.True(t, ok, "expected categories key in response")
	assert.Len(t, cats, 3)
	assert.Equal(t, "database", cats[0])
	assert.Equal(t, "web", cats[1])
	assert.Equal(t, "windows", cats[2])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortHandler_ListPortCategories_Empty(t *testing.T) {
	h, mock := newPortHandlerWithMock(t)

	mock.ExpectQuery("SELECT DISTINCT category").
		WillReturnRows(sqlmock.NewRows([]string{"category"}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ports/categories", nil)
	req = withRequestID(req)
	rr := httptest.NewRecorder()
	h.ListPortCategories(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]interface{}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	// categories key should exist; nil/null maps to nil in the Go type
	_, exists := resp["categories"]
	assert.True(t, exists, "expected categories key in response")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortHandler_ListPortCategories_DBError(t *testing.T) {
	h, mock := newPortHandlerWithMock(t)

	mock.ExpectQuery("SELECT DISTINCT category").
		WillReturnError(fmt.Errorf("connection reset"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ports/categories", nil)
	req = withRequestID(req)
	rr := httptest.NewRecorder()
	h.ListPortCategories(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── ListPortHostCounts ────────────────────────────────────────────────────────

func TestPortHandler_ListPortHostCounts_OK(t *testing.T) {
	h, mock := newPortHandlerWithMock(t)

	rows := sqlmock.NewRows([]string{"port", "protocol", "count"}).
		AddRow(80, "tcp", 42).
		AddRow(443, "tcp", 17).
		AddRow(53, "udp", 8)
	mock.ExpectQuery("SELECT port, protocol").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ports/host-counts", nil)
	req = withRequestID(req)
	rr := httptest.NewRecorder()
	h.ListPortHostCounts(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var resp []db.PortHostCount
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.Len(t, resp, 3)
	assert.Equal(t, 80, resp[0].Port)
	assert.Equal(t, "tcp", resp[0].Protocol)
	assert.Equal(t, 42, resp[0].Count)
	assert.Equal(t, 53, resp[2].Port)
	assert.Equal(t, "udp", resp[2].Protocol)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortHandler_ListPortHostCounts_Empty(t *testing.T) {
	h, mock := newPortHandlerWithMock(t)

	mock.ExpectQuery("SELECT port, protocol").
		WillReturnRows(sqlmock.NewRows([]string{"port", "protocol", "count"}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ports/host-counts", nil)
	req = withRequestID(req)
	rr := httptest.NewRecorder()
	h.ListPortHostCounts(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	// Must be [] not null.
	assert.Equal(t, "[]\n", rr.Body.String())
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPortHandler_ListPortHostCounts_DBError(t *testing.T) {
	h, mock := newPortHandlerWithMock(t)

	mock.ExpectQuery("SELECT port, protocol").
		WillReturnError(fmt.Errorf("connection reset"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ports/host-counts", nil)
	req = withRequestID(req)
	rr := httptest.NewRecorder()
	h.ListPortHostCounts(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// suppress unused import warning for bytes
var _ = bytes.NewBuffer
