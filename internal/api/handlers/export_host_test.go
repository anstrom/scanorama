package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/db"
)

// sampleHost builds a minimal *db.Host for export tests.
func sampleHost(ip string) *db.Host {
	hostname := "testhost.local"
	vendor := "Acme Corp"
	osFamily := "Linux"
	return &db.Host{
		IPAddress:  db.IPAddr{IP: net.ParseIP(ip)},
		Hostname:   &hostname,
		Vendor:     &vendor,
		OSFamily:   &osFamily,
		Status:     "up",
		TotalPorts: 3,
		FirstSeen:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastSeen:   time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC),
	}
}

func TestHostHandler_ExportHosts_DefaultsToCSV(t *testing.T) {
	h, store, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	// Returning 1 host (< exportPageSize) ends the loop on the first page.
	store.EXPECT().
		ListHosts(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return([]*db.Host{sampleHost("10.0.0.1")}, int64(1), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/export", nil)
	w := httptest.NewRecorder()
	h.ExportHosts(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "hosts-")
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".csv")
	assert.Contains(t, w.Header().Get("Content-Type"), "text/csv")
}

func TestHostHandler_ExportHosts_CSVFormat(t *testing.T) {
	h, store, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	host := sampleHost("192.168.1.1")

	// One host returned on first page — loop stops without a second call.
	store.EXPECT().
		ListHosts(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return([]*db.Host{host}, int64(1), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/export?format=csv", nil)
	w := httptest.NewRecorder()
	h.ExportHosts(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	r := csv.NewReader(strings.NewReader(w.Body.String()))
	rows, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2) // header + 1 data row

	header := rows[0]
	assert.Equal(t, "ip", header[0])
	assert.Equal(t, "hostname", header[1])
	assert.Equal(t, "open_port_count", header[8])

	data := rows[1]
	assert.Equal(t, "192.168.1.1", data[0])
	assert.Equal(t, "testhost.local", data[1])
	assert.Equal(t, "up", data[5])
	assert.Equal(t, "3", data[8]) // open_port_count field (TotalPorts)
}

func TestHostHandler_ExportHosts_JSONFormat(t *testing.T) {
	h, store, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	host := sampleHost("10.1.2.3")

	// One host — loop stops after the first page.
	store.EXPECT().
		ListHosts(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return([]*db.Host{host}, int64(1), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/export?format=json", nil)
	w := httptest.NewRecorder()
	h.ExportHosts(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".json")

	body := w.Body.String()
	// Must be a JSON array.
	var rows []map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(body), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, "10.1.2.3", rows[0]["ip"])
	assert.Equal(t, "testhost.local", rows[0]["hostname"])
	assert.Equal(t, "up", rows[0]["status"])
	assert.Equal(t, float64(3), rows[0]["open_port_count"])
}

func TestHostHandler_ExportHosts_InvalidFormatDefaultsToCSV(t *testing.T) {
	h, store, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	store.EXPECT().
		ListHosts(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return([]*db.Host{}, int64(0), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/export?format=xlsx", nil)
	w := httptest.NewRecorder()
	h.ExportHosts(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/csv")
}

func TestHostHandler_ExportHosts_ContentDispositionFilename(t *testing.T) {
	h, store, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	store.EXPECT().
		ListHosts(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return([]*db.Host{}, int64(0), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/export", nil)
	w := httptest.NewRecorder()
	h.ExportHosts(w, req)

	cd := w.Header().Get("Content-Disposition")
	assert.True(t, strings.HasPrefix(cd, `attachment; filename="`), "expected attachment; got: %s", cd)
	assert.Contains(t, cd, "hosts-")
}

func TestHostHandler_ExportHosts_ServiceError_EmitsNoRows(t *testing.T) {
	h, store, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	store.EXPECT().
		ListHosts(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return(nil, int64(0), fmt.Errorf("db error"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/export?format=csv", nil)
	w := httptest.NewRecorder()
	h.ExportHosts(w, req)

	// On error the handler stops; the header row was already written.
	assert.Equal(t, http.StatusOK, w.Code)
	r := csv.NewReader(strings.NewReader(w.Body.String()))
	rows, _ := r.ReadAll()
	assert.Len(t, rows, 1, "only the header row should be present on error")
}

func TestHostHandler_ExportHosts_ServiceError_JSON_EmitsEmptyArray(t *testing.T) {
	h, store, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	store.EXPECT().
		ListHosts(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return(nil, int64(0), fmt.Errorf("db error"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/export?format=json", nil)
	w := httptest.NewRecorder()
	h.ExportHosts(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	assert.Empty(t, rows)
}

func TestHostHandler_ExportHosts_MultiPage_CSV(t *testing.T) {
	h, store, ctrl := newHostHandlerWithMock(t)
	defer ctrl.Finish()

	// Build a full first page of hosts.
	page1 := make([]*db.Host, exportPageSize)
	for i := range page1 {
		page1[i] = sampleHost(fmt.Sprintf("10.0.%d.%d", i/256, i%256))
	}
	page2 := []*db.Host{sampleHost("10.1.0.1")}

	// First call returns a full page → loop continues.
	// Second call returns a partial page → loop exits.
	gomock.InOrder(
		store.EXPECT().
			ListHosts(gomock.Any(), gomock.Any(), 0, exportPageSize).
			Return(page1, int64(exportPageSize+1), nil),
		store.EXPECT().
			ListHosts(gomock.Any(), gomock.Any(), exportPageSize, exportPageSize).
			Return(page2, int64(exportPageSize+1), nil),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/export?format=csv", nil)
	w := httptest.NewRecorder()
	h.ExportHosts(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	r := csv.NewReader(strings.NewReader(w.Body.String()))
	rows, err := r.ReadAll()
	require.NoError(t, err)
	// header + exportPageSize data rows + 1 overflow row
	assert.Len(t, rows, exportPageSize+2)
}
