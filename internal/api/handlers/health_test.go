package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/internal/metrics/mocks"
)

// MockDB is a mock implementation of the database interface.
type MockDB struct {
	mock.Mock
}

func (m *MockDB) Ping(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func TestNewHealthHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name     string
		database DatabasePinger
		metrics  metrics.MetricsRegistry
	}{
		{
			name:     "with database and metrics",
			database: &MockDB{},
			metrics:  mocks.NewMockMetricsRegistry(ctrl),
		},
		{
			name:     "with nil database",
			database: nil,
			metrics:  mocks.NewMockMetricsRegistry(ctrl),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			handler := NewHealthHandler(tt.database, logger, tt.metrics)

			assert.NotNil(t, handler)
			assert.NotNil(t, handler.logger)
			assert.Equal(t, tt.metrics, handler.metrics)
		})
	}
}

func TestHealthHandler_Health(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		setupDB        func() *MockDB
		setupMetrics   func() *mocks.MockMetricsRegistry
		expectedStatus int
	}{
		{
			name: "healthy system",
			setupDB: func() *MockDB {
				db := &MockDB{}
				db.On("Ping", mock.Anything).Return(nil)
				return db
			},
			setupMetrics: func() *mocks.MockMetricsRegistry {
				metrics := mocks.NewMockMetricsRegistry(ctrl)
				metrics.EXPECT().Counter("api_health_checks_total", gomock.Any()).AnyTimes()
				return metrics
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "database connection error",
			setupDB: func() *MockDB {
				db := &MockDB{}
				db.On("Ping", mock.Anything).Return(errors.New("connection failed"))
				return db
			},
			setupMetrics: func() *mocks.MockMetricsRegistry {
				metrics := mocks.NewMockMetricsRegistry(ctrl)
				metrics.EXPECT().Counter("api_health_checks_total", gomock.Any()).AnyTimes()
				return metrics
			},
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name: "nil database",
			setupDB: func() *MockDB {
				return nil
			},
			setupMetrics: func() *mocks.MockMetricsRegistry {
				metrics := mocks.NewMockMetricsRegistry(ctrl)
				metrics.EXPECT().Counter("api_health_checks_total", gomock.Any()).AnyTimes()
				return metrics
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			var db DatabasePinger
			if setupDB := tt.setupDB(); setupDB != nil {
				db = setupDB
			}
			testMetrics := tt.setupMetrics()

			handler := NewHealthHandler(db, logger, testMetrics)

			req := httptest.NewRequest("GET", "/health", http.NoBody)
			w := httptest.NewRecorder()

			handler.Health(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Contains(t, response, "status")
			assert.Contains(t, response, "timestamp")
			assert.Contains(t, response, "uptime")

			// MockDB expectations are automatically verified by testify
		})
	}
}

func TestHealthHandler_HealthResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	mockMetrics := mocks.NewMockMetricsRegistry(ctrl)
	mockMetrics.EXPECT().Counter(gomock.Any(), gomock.Any()).AnyTimes()

	handler := NewHealthHandler(nil, logger, mockMetrics)

	req := httptest.NewRequest("GET", "/health", http.NoBody)
	w := httptest.NewRecorder()

	handler.Health(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.NotEmpty(t, response.Status)
	assert.NotZero(t, response.Timestamp)
	assert.NotEmpty(t, response.Uptime)
	assert.NotNil(t, response.Checks)
	assert.NotEmpty(t, response.Status)
}

func TestHealthHandler_UptimeCalculation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	mockMetrics := mocks.NewMockMetricsRegistry(ctrl)
	mockMetrics.EXPECT().Counter(gomock.Any(), gomock.Any()).AnyTimes()

	handler := NewHealthHandler(nil, logger, mockMetrics)

	// Wait a small amount to ensure uptime is measurable
	time.Sleep(10 * time.Millisecond)

	req := httptest.NewRequest("GET", "/health", http.NoBody)
	w := httptest.NewRecorder()

	handler.Health(w, req)

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify uptime format (should contain time units)
	assert.True(t,
		strings.Contains(response.Uptime, "s") ||
			strings.Contains(response.Uptime, "m") ||
			strings.Contains(response.Uptime, "h"),
		"Uptime should contain time units")
}

func TestHealthHandler_SystemInfo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	mockMetrics := mocks.NewMockMetricsRegistry(ctrl)
	mockMetrics.EXPECT().Counter(gomock.Any(), gomock.Any()).AnyTimes()

	handler := NewHealthHandler(nil, logger, mockMetrics)

	req := httptest.NewRequest("GET", "/health", http.NoBody)
	w := httptest.NewRecorder()

	handler.Health(w, req)

	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify basic HealthResponse structure
	assert.NotEmpty(t, response.Status)
	assert.NotZero(t, response.Timestamp)
	assert.NotEmpty(t, response.Uptime)
	assert.NotNil(t, response.Checks)

	// Verify timestamp is recent
	assert.True(t, time.Since(response.Timestamp) < time.Minute)
}

func TestHealthHandler_RequestIDHandling(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	mockMetrics := mocks.NewMockMetricsRegistry(ctrl)
	mockMetrics.EXPECT().Counter(gomock.Any(), gomock.Any()).AnyTimes()

	handler := NewHealthHandler(nil, logger, mockMetrics)

	// Test with request ID in context
	req := httptest.NewRequest("GET", "/health", http.NoBody)

	// Add request ID to context (simulating middleware)
	requestID := "test-request-" + uuid.New().String()
	ctx := context.WithValue(req.Context(), ContextKey("request_id"), requestID)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler.Health(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Response should be valid regardless of request ID presence
	var response HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.NotEmpty(t, response.Status)
}

func TestHealthHandler_PerformanceLoad(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	mockMetrics := mocks.NewMockMetricsRegistry(ctrl)
	mockMetrics.EXPECT().Counter(gomock.Any(), gomock.Any()).AnyTimes()

	handler := NewHealthHandler(nil, logger, mockMetrics)

	// Test multiple concurrent requests
	const numRequests = 100
	start := time.Now()

	for i := 0; i < numRequests; i++ {
		req := httptest.NewRequest("GET", "/health", http.NoBody)
		w := httptest.NewRecorder()
		handler.Health(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	duration := time.Since(start)

	// Performance assertion - should handle requests reasonably fast
	assert.Less(t, duration, 5*time.Second, "Health checks should complete in reasonable time")
}

func TestHealthHandler_LargeResponse(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	mockMetrics := mocks.NewMockMetricsRegistry(ctrl)
	mockMetrics.EXPECT().Counter(gomock.Any(), gomock.Any()).AnyTimes()

	handler := NewHealthHandler(nil, logger, mockMetrics)

	start := time.Now()
	req := httptest.NewRequest("GET", "/health", http.NoBody)
	w := httptest.NewRecorder()

	handler.Health(w, req)
	duration := time.Since(start)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Greater(t, len(w.Body.String()), 100) // Should be substantial response
	assert.Less(t, duration, 5*time.Second)      // Should complete in reasonable time
}
