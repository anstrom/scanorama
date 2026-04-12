// Package handlers — unit tests for CertificateHandler using sqlmock.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/gorilla/mux"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newCertHandlerWithMock(t *testing.T) (*CertificateHandler, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	database := &db.DB{DB: sqlx.NewDb(sqlDB, "sqlmock")}
	repo := db.NewBannerRepository(database)
	return NewCertificateHandler(repo, createTestLogger(), metrics.NewRegistry()), mock
}

func routeCertExpiring(h *CertificateHandler, url string) *httptest.ResponseRecorder {
	r := mux.NewRouter()
	r.HandleFunc("/api/v1/certificates/expiring", h.GetExpiringCertificates).Methods("GET")
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

var expiringCertCols = []string{
	"host_id", "ip_address", "hostname", "port", "subject_cn", "not_after",
}

// ── GetExpiringCertificates ───────────────────────────────────────────────────

func TestCertHandler_GetExpiring_HappyPath(t *testing.T) {
	h, mock := newCertHandlerWithMock(t)

	notAfter := time.Now().Add(10 * 24 * time.Hour).UTC()
	mock.ExpectQuery("SELECT").
		WithArgs(30).
		WillReturnRows(sqlmock.NewRows(expiringCertCols).
			AddRow(
				"550e8400-e29b-41d4-a716-446655440002",
				"192.168.1.100",
				"server01.local",
				443,
				"server01.local",
				notAfter,
			))

	w := routeCertExpiring(h, "/api/v1/certificates/expiring")
	require.Equal(t, http.StatusOK, w.Code)

	var resp db.ExpiringCertificatesResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 30, resp.Days)
	require.Len(t, resp.Certificates, 1)
	cert := resp.Certificates[0]
	assert.Equal(t, "192.168.1.100", cert.HostIP)
	assert.Equal(t, "server01.local", cert.Hostname)
	assert.Equal(t, 443, cert.Port)
	assert.Equal(t, "server01.local", cert.SubjectCN)
	assert.GreaterOrEqual(t, cert.DaysLeft, 9)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCertHandler_GetExpiring_EmptyResult(t *testing.T) {
	h, mock := newCertHandlerWithMock(t)

	mock.ExpectQuery("SELECT").
		WithArgs(30).
		WillReturnRows(sqlmock.NewRows(expiringCertCols))

	w := routeCertExpiring(h, "/api/v1/certificates/expiring")
	require.Equal(t, http.StatusOK, w.Code)

	var resp db.ExpiringCertificatesResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 30, resp.Days)
	assert.Empty(t, resp.Certificates)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCertHandler_GetExpiring_CustomDays(t *testing.T) {
	h, mock := newCertHandlerWithMock(t)

	mock.ExpectQuery("SELECT").
		WithArgs(7).
		WillReturnRows(sqlmock.NewRows(expiringCertCols))

	w := routeCertExpiring(h, "/api/v1/certificates/expiring?days=7")
	require.Equal(t, http.StatusOK, w.Code)

	var resp db.ExpiringCertificatesResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 7, resp.Days)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCertHandler_GetExpiring_InvalidDays_NonNumeric(t *testing.T) {
	h, _ := newCertHandlerWithMock(t)

	w := routeCertExpiring(h, "/api/v1/certificates/expiring?days=abc")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCertHandler_GetExpiring_InvalidDays_Zero(t *testing.T) {
	h, _ := newCertHandlerWithMock(t)

	w := routeCertExpiring(h, "/api/v1/certificates/expiring?days=0")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCertHandler_GetExpiring_InvalidDays_Negative(t *testing.T) {
	h, _ := newCertHandlerWithMock(t)

	w := routeCertExpiring(h, "/api/v1/certificates/expiring?days=-5")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCertHandler_GetExpiring_DaysExceedsMax(t *testing.T) {
	h, _ := newCertHandlerWithMock(t)

	w := routeCertExpiring(h, "/api/v1/certificates/expiring?days=91")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCertHandler_GetExpiring_MaxDays_OK(t *testing.T) {
	h, mock := newCertHandlerWithMock(t)

	mock.ExpectQuery("SELECT").
		WithArgs(90).
		WillReturnRows(sqlmock.NewRows(expiringCertCols))

	w := routeCertExpiring(h, "/api/v1/certificates/expiring?days=90")
	assert.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCertHandler_GetExpiring_DBError(t *testing.T) {
	h, mock := newCertHandlerWithMock(t)

	mock.ExpectQuery("SELECT").
		WithArgs(30).
		WillReturnError(fmt.Errorf("connection refused"))

	w := routeCertExpiring(h, "/api/v1/certificates/expiring")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	require.NoError(t, mock.ExpectationsWereMet())
}
