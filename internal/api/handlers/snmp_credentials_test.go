// Package handlers — unit tests for SNMPCredentialsHandler.
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

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appcrypto "github.com/anstrom/scanorama/internal/crypto"
	"github.com/anstrom/scanorama/internal/db"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newSNMPCredsHandlerWithMock(t *testing.T) (*SNMPCredentialsHandler, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	database := &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}
	repo := db.NewSNMPCredentialsRepository(database)
	return NewSNMPCredentialsHandler(repo, createTestLogger()), mock
}

var snmpCredHandlerCols = []string{
	"id", "network_id", "version", "community", "username",
	"auth_proto", "auth_pass", "priv_proto", "priv_pass",
	"created_at", "updated_at",
}

func encryptForMock(t *testing.T, s string) *string {
	t.Helper()
	if s == "" {
		return nil
	}
	enc, err := appcrypto.Encrypt(s)
	require.NoError(t, err)
	return &enc
}

func snmpJSONBody(t *testing.T, v any) *bytes.Buffer {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewBuffer(b)
}

func snmpRequest(method, target string, body *bytes.Buffer) *http.Request {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, target, body)
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req = req.WithContext(context.WithValue(req.Context(), ContextKey("request_id"), "test-id"))
	return req
}

// ── ListSNMPCredentials ───────────────────────────────────────────────────────

func TestSNMPCredentialsHandler_List_Empty(t *testing.T) {
	h, mock := newSNMPCredsHandlerWithMock(t)
	mock.ExpectQuery("SELECT .* FROM snmp_credentials").
		WillReturnRows(sqlmock.NewRows(snmpCredHandlerCols))

	rr := httptest.NewRecorder()
	h.ListSNMPCredentials(rr, snmpRequest(http.MethodGet, "/admin/snmp/credentials", nil))

	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]any
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, []any{}, resp["credentials"], "empty list must return [] not null")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsHandler_List_SecretsAreRedacted(t *testing.T) {
	h, mock := newSNMPCredsHandlerWithMock(t)
	id := uuid.New()
	now := time.Now().UTC()
	encCommunity := encryptForMock(t, "plaintext-secret")

	rows := sqlmock.NewRows(snmpCredHandlerCols).
		AddRow(id, nil, "v2c", encCommunity, nil, nil, nil, nil, nil, now, now)
	mock.ExpectQuery("SELECT .* FROM snmp_credentials").WillReturnRows(rows)

	rr := httptest.NewRecorder()
	h.ListSNMPCredentials(rr, snmpRequest(http.MethodGet, "/admin/snmp/credentials", nil))

	assert.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	assert.NotContains(t, body, "plaintext-secret", "plaintext must never appear in response")
	assert.Contains(t, body, "***")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsHandler_List_DBError(t *testing.T) {
	h, mock := newSNMPCredsHandlerWithMock(t)
	mock.ExpectQuery("SELECT .* FROM snmp_credentials").
		WillReturnError(fmt.Errorf("db down"))

	rr := httptest.NewRecorder()
	h.ListSNMPCredentials(rr, snmpRequest(http.MethodGet, "/admin/snmp/credentials", nil))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── UpsertSNMPCredential ──────────────────────────────────────────────────────

func TestSNMPCredentialsHandler_Upsert_V2c_OK(t *testing.T) {
	h, mock := newSNMPCredsHandlerWithMock(t)
	id := uuid.New()
	now := time.Now().UTC()
	encCommunity := encryptForMock(t, "my-community")

	mock.ExpectQuery("INSERT INTO snmp_credentials").
		WillReturnRows(sqlmock.NewRows(snmpCredHandlerCols).
			AddRow(id, nil, "v2c", encCommunity, nil, nil, nil, nil, nil, now, now))

	body := snmpJSONBody(t, map[string]any{"version": "v2c", "community": "my-community"})
	rr := httptest.NewRecorder()
	h.UpsertSNMPCredential(rr, snmpRequest(http.MethodPut, "/admin/snmp/credentials", body))

	assert.Equal(t, http.StatusOK, rr.Code)
	// Response must redact the community.
	respBody := rr.Body.String()
	assert.NotContains(t, respBody, "my-community")
	assert.Contains(t, respBody, "***")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsHandler_Upsert_V2c_MissingCommunity_400(t *testing.T) {
	h, _ := newSNMPCredsHandlerWithMock(t)
	body := snmpJSONBody(t, map[string]any{"version": "v2c"})
	rr := httptest.NewRecorder()
	h.UpsertSNMPCredential(rr, snmpRequest(http.MethodPut, "/admin/snmp/credentials", body))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSNMPCredentialsHandler_Upsert_V3_MissingUsername_400(t *testing.T) {
	h, _ := newSNMPCredsHandlerWithMock(t)
	body := snmpJSONBody(t, map[string]any{"version": "v3"})
	rr := httptest.NewRecorder()
	h.UpsertSNMPCredential(rr, snmpRequest(http.MethodPut, "/admin/snmp/credentials", body))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSNMPCredentialsHandler_Upsert_InvalidVersion_400(t *testing.T) {
	h, _ := newSNMPCredsHandlerWithMock(t)
	body := snmpJSONBody(t, map[string]any{"version": "v1", "community": "x"})
	rr := httptest.NewRecorder()
	h.UpsertSNMPCredential(rr, snmpRequest(http.MethodPut, "/admin/snmp/credentials", body))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSNMPCredentialsHandler_Upsert_DefaultsToV2c(t *testing.T) {
	h, mock := newSNMPCredsHandlerWithMock(t)
	id := uuid.New()
	now := time.Now().UTC()
	encCommunity := encryptForMock(t, "comm")

	mock.ExpectQuery("INSERT INTO snmp_credentials").
		WillReturnRows(sqlmock.NewRows(snmpCredHandlerCols).
			AddRow(id, nil, "v2c", encCommunity, nil, nil, nil, nil, nil, now, now))

	// No version field in the request.
	body := snmpJSONBody(t, map[string]any{"community": "comm"})
	rr := httptest.NewRecorder()
	h.UpsertSNMPCredential(rr, snmpRequest(http.MethodPut, "/admin/snmp/credentials", body))

	assert.Equal(t, http.StatusOK, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsHandler_Upsert_MalformedJSON_400(t *testing.T) {
	h, _ := newSNMPCredsHandlerWithMock(t)
	rr := httptest.NewRecorder()
	h.UpsertSNMPCredential(rr, snmpRequest(http.MethodPut, "/admin/snmp/credentials",
		bytes.NewBufferString("{not json")))

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSNMPCredentialsHandler_Upsert_DBError_500(t *testing.T) {
	h, mock := newSNMPCredsHandlerWithMock(t)
	mock.ExpectQuery("INSERT INTO snmp_credentials").
		WillReturnError(fmt.Errorf("constraint error"))

	body := snmpJSONBody(t, map[string]any{"version": "v2c", "community": "x"})
	rr := httptest.NewRecorder()
	h.UpsertSNMPCredential(rr, snmpRequest(http.MethodPut, "/admin/snmp/credentials", body))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

// ── DeleteSNMPCredential ──────────────────────────────────────────────────────

func snmpDeleteRequest(id uuid.UUID) *http.Request {
	req := snmpRequest(http.MethodDelete, "/admin/snmp/credentials/"+id.String(), nil)
	req = mux.SetURLVars(req, map[string]string{"id": id.String()})
	return req
}

func TestSNMPCredentialsHandler_Delete_OK(t *testing.T) {
	h, mock := newSNMPCredsHandlerWithMock(t)
	id := uuid.New()
	mock.ExpectExec("DELETE FROM snmp_credentials").
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(0, 1))

	rr := httptest.NewRecorder()
	h.DeleteSNMPCredential(rr, snmpDeleteRequest(id))

	assert.Equal(t, http.StatusNoContent, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsHandler_Delete_NotFound_404(t *testing.T) {
	h, mock := newSNMPCredsHandlerWithMock(t)
	id := uuid.New()
	mock.ExpectExec("DELETE FROM snmp_credentials").
		WithArgs(id).
		WillReturnResult(sqlmock.NewResult(0, 0))

	rr := httptest.NewRecorder()
	h.DeleteSNMPCredential(rr, snmpDeleteRequest(id))

	assert.Equal(t, http.StatusNotFound, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSNMPCredentialsHandler_Delete_InvalidUUID_400(t *testing.T) {
	h, _ := newSNMPCredsHandlerWithMock(t)
	req := snmpRequest(http.MethodDelete, "/admin/snmp/credentials/not-a-uuid", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "not-a-uuid"})

	rr := httptest.NewRecorder()
	h.DeleteSNMPCredential(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestSNMPCredentialsHandler_Delete_DBError_500(t *testing.T) {
	h, mock := newSNMPCredsHandlerWithMock(t)
	id := uuid.New()
	mock.ExpectExec("DELETE FROM snmp_credentials").
		WithArgs(id).
		WillReturnError(fmt.Errorf("db error"))

	rr := httptest.NewRecorder()
	h.DeleteSNMPCredential(rr, snmpDeleteRequest(id))

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
