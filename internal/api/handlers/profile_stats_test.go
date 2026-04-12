package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/api/handlers/mocks"
	"github.com/anstrom/scanorama/internal/db"
	apierrors "github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

func TestProfileHandler_GetProfileStats(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	avg := 12.5

	tests := []struct {
		name           string
		profileID      string
		setupMock      func(m *mocks.MockProfileServicer)
		expectedStatus int
		checkBody      func(t *testing.T, body []byte)
	}{
		{
			name:      "happy path - profile with scans",
			profileID: "web-full",
			setupMock: func(m *mocks.MockProfileServicer) {
				m.EXPECT().GetProfileStats(gomock.Any(), "web-full").Return(&db.ProfileStats{
					ProfileID:     "web-full",
					TotalScans:    42,
					UniqueHosts:   7,
					LastUsed:      &now,
					AvgHostsFound: &avg,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				t.Helper()
				var resp ProfileStatsResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "web-full", resp.ProfileID)
				assert.Equal(t, 42, resp.TotalScans)
				assert.Equal(t, 7, resp.UniqueHosts)
				require.NotNil(t, resp.LastUsed)
				assert.WithinDuration(t, now, *resp.LastUsed, time.Second)
				require.NotNil(t, resp.AvgHostsFound)
				assert.InDelta(t, 12.5, *resp.AvgHostsFound, 0.001)
			},
		},
		{
			name:      "profile with no scans - zero values, 200 OK",
			profileID: "empty-profile",
			setupMock: func(m *mocks.MockProfileServicer) {
				m.EXPECT().GetProfileStats(gomock.Any(), "empty-profile").Return(&db.ProfileStats{
					ProfileID:     "empty-profile",
					TotalScans:    0,
					UniqueHosts:   0,
					LastUsed:      nil,
					AvgHostsFound: nil,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkBody: func(t *testing.T, body []byte) {
				t.Helper()
				var resp ProfileStatsResponse
				require.NoError(t, json.Unmarshal(body, &resp))
				assert.Equal(t, "empty-profile", resp.ProfileID)
				assert.Equal(t, 0, resp.TotalScans)
				assert.Equal(t, 0, resp.UniqueHosts)
				assert.Nil(t, resp.LastUsed)
				assert.Nil(t, resp.AvgHostsFound)
			},
		},
		{
			name:      "unknown profile ID returns 404",
			profileID: "no-such-profile",
			setupMock: func(m *mocks.MockProfileServicer) {
				m.EXPECT().GetProfileStats(gomock.Any(), "no-such-profile").Return(
					nil, apierrors.ErrNotFoundWithID("profile", "no-such-profile"),
				)
			},
			expectedStatus: http.StatusNotFound,
			checkBody:      nil,
		},
		{
			name:      "internal error returns 500",
			profileID: "web-full",
			setupMock: func(m *mocks.MockProfileServicer) {
				m.EXPECT().GetProfileStats(gomock.Any(), "web-full").Return(
					nil, fmt.Errorf("database unavailable"),
				)
			},
			expectedStatus: http.StatusInternalServerError,
			checkBody:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSvc := mocks.NewMockProfileServicer(ctrl)
			tt.setupMock(mockSvc)

			logger := createTestLogger()
			handler := NewProfileHandler(mockSvc, logger, metrics.NewRegistry())

			req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/"+tt.profileID+"/stats", http.NoBody)
			req = mux.SetURLVars(req, map[string]string{"id": tt.profileID})
			w := httptest.NewRecorder()

			handler.GetProfileStats(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.checkBody != nil {
				tt.checkBody(t, w.Body.Bytes())
			}
		})
	}
}
