package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	apierrors "github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

// mockDiffScanServicer is a ScanServicer that delegates only GetScanDiff.
type mockDiffScanServicer struct {
	nilScanServicer
	getScanDiffFn func(ctx context.Context, a, b uuid.UUID) (*db.ScanDiff, error)
}

func (m *mockDiffScanServicer) GetScanDiff(ctx context.Context, a, b uuid.UUID) (*db.ScanDiff, error) {
	return m.getScanDiffFn(ctx, a, b)
}

func TestScanHandler_GetScanDiff(t *testing.T) {
	idA := uuid.MustParse("aaaaaaaa-0000-0000-0000-000000000001")
	idB := uuid.MustParse("bbbbbbbb-0000-0000-0000-000000000002")
	hostID := uuid.MustParse("cccccccc-0000-0000-0000-000000000003")

	svcName := "https"
	prevSvc := "http"

	successDiff := &db.ScanDiff{
		ScanAID: idA,
		ScanBID: idB,
		HostID:  hostID,
		Ports: []db.ScanDiffEntry{
			{Port: 443, Protocol: "tcp", State: "open", ServiceName: &svcName, Status: db.DiffStatusNew},
			{Port: 80, Protocol: "tcp", State: "open", ServiceName: &svcName, Status: db.DiffStatusChanged,
				PrevState:       func() *string { s := "filtered"; return &s }(),
				PrevServiceName: &prevSvc,
			},
		},
		OSChanged:      false,
		NewCount:       1,
		ClosedCount:    0,
		ChangedCount:   1,
		UnchangedCount: 0,
	}

	tests := []struct {
		name           string
		queryString    string
		svc            ScanServicer
		expectedStatus int
		checkBody      func(t *testing.T, body []byte)
	}{
		{
			name:        "200 success",
			queryString: "?a=" + idA.String() + "&b=" + idB.String(),
			svc: &mockDiffScanServicer{
				getScanDiffFn: func(_ context.Context, a, b uuid.UUID) (*db.ScanDiff, error) {
					assert.Equal(t, idA, a)
					assert.Equal(t, idB, b)
					return successDiff, nil
				},
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				var raw map[string]any
				require.NoError(t, json.Unmarshal(body, &raw))
				assert.Equal(t, idA.String(), raw["scan_a_id"])
				assert.Equal(t, idB.String(), raw["scan_b_id"])
				assert.Equal(t, hostID.String(), raw["host_id"])
				assert.InDelta(t, 1, raw["new_count"], 0)
				assert.InDelta(t, 1, raw["changed_count"], 0)
				assert.False(t, raw["os_changed"].(bool))
				ports, ok := raw["ports"].([]any)
				require.True(t, ok)
				assert.Len(t, ports, 2)
			},
		},
		{
			name:           "400 missing a param",
			queryString:    "?b=" + idB.String(),
			svc:            nilScanServicer{},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var raw map[string]any
				require.NoError(t, json.Unmarshal(body, &raw))
				assert.Contains(t, raw["message"], "'a'")
			},
		},
		{
			name:           "400 missing b param",
			queryString:    "?a=" + idA.String(),
			svc:            nilScanServicer{},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var raw map[string]any
				require.NoError(t, json.Unmarshal(body, &raw))
				assert.Contains(t, raw["message"], "'b'")
			},
		},
		{
			name:           "400 invalid UUID for a",
			queryString:    "?a=not-a-uuid&b=" + idB.String(),
			svc:            nilScanServicer{},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var raw map[string]any
				require.NoError(t, json.Unmarshal(body, &raw))
				assert.Contains(t, raw["message"], "invalid scan id 'a'")
			},
		},
		{
			name:           "400 invalid UUID for b",
			queryString:    "?a=" + idA.String() + "&b=not-a-uuid",
			svc:            nilScanServicer{},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var raw map[string]any
				require.NoError(t, json.Unmarshal(body, &raw))
				assert.Contains(t, raw["message"], "invalid scan id 'b'")
			},
		},
		{
			name:        "400 scans from different hosts",
			queryString: "?a=" + idA.String() + "&b=" + idB.String(),
			svc: &mockDiffScanServicer{
				getScanDiffFn: func(_ context.Context, _, _ uuid.UUID) (*db.ScanDiff, error) {
					return nil, apierrors.NewScanError(apierrors.CodeValidation,
						"scans belong to different hosts")
				},
			},
			expectedStatus: http.StatusBadRequest,
			checkBody: func(t *testing.T, body []byte) {
				var raw map[string]any
				require.NoError(t, json.Unmarshal(body, &raw))
				assert.Contains(t, raw["message"], "different hosts")
			},
		},
		{
			name:        "404 scan not found",
			queryString: "?a=" + idA.String() + "&b=" + idB.String(),
			svc: &mockDiffScanServicer{
				getScanDiffFn: func(_ context.Context, _, _ uuid.UUID) (*db.ScanDiff, error) {
					return nil, apierrors.ErrNotFoundWithID("scan", idA.String())
				},
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:        "500 service error",
			queryString: "?a=" + idA.String() + "&b=" + idB.String(),
			svc: &mockDiffScanServicer{
				getScanDiffFn: func(_ context.Context, _, _ uuid.UUID) (*db.ScanDiff, error) {
					return nil, fmt.Errorf("database connection failed")
				},
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			h := NewScanHandler(tt.svc, logger, metrics.NewRegistry())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/scans/diff"+tt.queryString, http.NoBody)
			w := httptest.NewRecorder()

			h.GetScanDiff(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.checkBody != nil {
				tt.checkBody(t, w.Body.Bytes())
			}
		})
	}
}
