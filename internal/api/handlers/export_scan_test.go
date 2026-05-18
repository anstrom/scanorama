package handlers

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/db"
)

// sampleScan builds a minimal *db.Scan for export tests.
func sampleScan() *db.Scan {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	profileID := "profile-abc"
	durationStr := "5m30s"
	portsScanned := "22 open / 100 total"
	return &db.Scan{
		ID:           uuid.New(),
		Targets:      []string{"10.0.0.0/24", "10.0.1.0/24"},
		ProfileID:    &profileID,
		Status:       "completed",
		StartedAt:    &now,
		DurationStr:  &durationStr,
		PortsScanned: &portsScanned,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func TestScanHandler_ExportScans_DefaultsToCSV(t *testing.T) {
	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()

	// One scan returned — loop stops after the first page.
	store.EXPECT().
		ListScans(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return([]*db.Scan{sampleScan()}, int64(1), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scans/export", nil)
	w := httptest.NewRecorder()
	h.ExportScans(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Disposition"), "scans-")
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".csv")
	assert.Contains(t, w.Header().Get("Content-Type"), "text/csv")
}

func TestScanHandler_ExportScans_CSVFormat(t *testing.T) {
	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()

	scan := sampleScan()

	// One scan — loop stops after first page.
	store.EXPECT().
		ListScans(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return([]*db.Scan{scan}, int64(1), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scans/export?format=csv", nil)
	w := httptest.NewRecorder()
	h.ExportScans(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	r := csv.NewReader(strings.NewReader(w.Body.String()))
	rows, err := r.ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 2) // header + 1 data row

	header := rows[0]
	assert.Equal(t, "scan_id", header[0])
	assert.Equal(t, "targets", header[1])
	assert.Equal(t, "profile_id", header[2])
	assert.Equal(t, "status", header[3])

	data := rows[1]
	assert.Equal(t, scan.ID.String(), data[0])
	assert.Equal(t, "10.0.0.0/24;10.0.1.0/24", data[1])
	assert.Equal(t, "profile-abc", data[2])
	assert.Equal(t, "completed", data[3])
	assert.Equal(t, "5m30s", data[5])
	assert.Equal(t, "22 open / 100 total", data[6])
}

func TestScanHandler_ExportScans_JSONFormat(t *testing.T) {
	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()

	scan := sampleScan()

	// One scan — loop stops after first page.
	store.EXPECT().
		ListScans(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return([]*db.Scan{scan}, int64(1), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scans/export?format=json", nil)
	w := httptest.NewRecorder()
	h.ExportScans(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
	assert.Contains(t, w.Header().Get("Content-Disposition"), ".json")

	var rows []map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	require.Len(t, rows, 1)
	assert.Equal(t, scan.ID.String(), rows[0]["scan_id"])
	assert.Equal(t, "10.0.0.0/24;10.0.1.0/24", rows[0]["targets"])
	assert.Equal(t, "profile-abc", rows[0]["profile_id"])
	assert.Equal(t, "completed", rows[0]["status"])
}

func TestScanHandler_ExportScans_InvalidFormatDefaultsToCSV(t *testing.T) {
	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()

	store.EXPECT().
		ListScans(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return([]*db.Scan{}, int64(0), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scans/export?format=xml", nil)
	w := httptest.NewRecorder()
	h.ExportScans(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/csv")
}

func TestScanHandler_ExportScans_ContentDispositionFilename(t *testing.T) {
	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()

	store.EXPECT().
		ListScans(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return([]*db.Scan{}, int64(0), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scans/export", nil)
	w := httptest.NewRecorder()
	h.ExportScans(w, req)

	cd := w.Header().Get("Content-Disposition")
	assert.True(t, strings.HasPrefix(cd, `attachment; filename="`), "expected attachment; got: %s", cd)
	assert.Contains(t, cd, "scans-")
}

func TestScanHandler_ExportScans_ServiceError_EmitsNoRows(t *testing.T) {
	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()

	store.EXPECT().
		ListScans(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return(nil, int64(0), fmt.Errorf("db error"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scans/export?format=csv", nil)
	w := httptest.NewRecorder()
	h.ExportScans(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	r := csv.NewReader(strings.NewReader(w.Body.String()))
	rows, _ := r.ReadAll()
	assert.Len(t, rows, 1, "only the header row should be present on error")
}

func TestScanHandler_ExportScans_ServiceError_JSON_EmitsEmptyArray(t *testing.T) {
	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()

	store.EXPECT().
		ListScans(gomock.Any(), gomock.Any(), 0, exportPageSize).
		Return(nil, int64(0), fmt.Errorf("db error"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scans/export?format=json", nil)
	w := httptest.NewRecorder()
	h.ExportScans(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
	assert.Empty(t, rows)
}

func TestScanHandler_ExportScans_MultiPage_CSV(t *testing.T) {
	h, store, ctrl := newScanHandlerWithMock(t)
	defer ctrl.Finish()

	page1 := make([]*db.Scan, exportPageSize)
	for i := range page1 {
		page1[i] = sampleScan()
	}
	page2 := []*db.Scan{sampleScan()}

	gomock.InOrder(
		store.EXPECT().
			ListScans(gomock.Any(), gomock.Any(), 0, exportPageSize).
			Return(page1, int64(exportPageSize+1), nil),
		store.EXPECT().
			ListScans(gomock.Any(), gomock.Any(), exportPageSize, exportPageSize).
			Return(page2, int64(exportPageSize+1), nil),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/scans/export?format=csv", nil)
	w := httptest.NewRecorder()
	h.ExportScans(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	r := csv.NewReader(strings.NewReader(w.Body.String()))
	rows, err := r.ReadAll()
	require.NoError(t, err)
	assert.Len(t, rows, exportPageSize+2)
}
