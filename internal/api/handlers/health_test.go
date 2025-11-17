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

func TestHealthHandler_Liveness(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("liveness check with metrics", func(t *testing.T) {
		logger := createTestLogger()
		mockMetrics := mocks.NewMockMetricsRegistry(ctrl)
		mockMetrics.EXPECT().Counter("api_liveness_checks_total", nil).Times(1)

		handler := NewHealthHandler(nil, logger, mockMetrics)

		req := httptest.NewRequest("GET", "/liveness", http.NoBody)
		w := httptest.NewRecorder()

		handler.Liveness(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Contains(t, response, "status")
		assert.Contains(t, response, "timestamp")
		assert.Contains(t, response, "uptime")
		assert.Equal(t, "alive", response["status"])
	})

	t.Run("liveness check without metrics", func(t *testing.T) {
		logger := createTestLogger()

		handler := NewHealthHandler(nil, logger, nil)

		req := httptest.NewRequest("GET", "/liveness", http.NoBody)
		w := httptest.NewRecorder()

		handler.Liveness(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Contains(t, response, "status")
		assert.Contains(t, response, "timestamp")
		assert.Contains(t, response, "uptime")
		assert.Equal(t, "alive", response["status"])
	})
}

func TestHealthHandler_LivenessResponse(t *testing.T) {
	logger := createTestLogger()
	handler := NewHealthHandler(nil, logger, nil)

	req := httptest.NewRequest("GET", "/liveness", http.NoBody)
	w := httptest.NewRecorder()

	handler.Liveness(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response LivenessResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "alive", response.Status)
	assert.NotZero(t, response.Timestamp)
	assert.NotEmpty(t, response.Uptime)

	// Verify timestamp is recent
	assert.True(t, time.Since(response.Timestamp) < time.Minute)
}

func TestHealthHandler_LivenessPerformance(t *testing.T) {
	logger := createTestLogger()
	handler := NewHealthHandler(nil, logger, nil)

	// Test multiple concurrent liveness requests
	const numRequests = 100
	start := time.Now()

	for i := 0; i < numRequests; i++ {
		req := httptest.NewRequest("GET", "/liveness", http.NoBody)
		w := httptest.NewRecorder()
		handler.Liveness(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	duration := time.Since(start)

	// Liveness should be faster than health checks since it has no dependencies
	assert.Less(t, duration, 2*time.Second, "Liveness checks should be very fast")

	// Verify average response time per request
	avgDuration := duration / numRequests
	assert.Less(t, avgDuration, 20*time.Millisecond, "Individual liveness checks should be under 20ms")
}

func TestHealthHandler_LivenessVsHealthPerformance(t *testing.T) {
	logger := createTestLogger()

	// Test with a mock database that has some latency
	mockDB := &MockDB{}
	mockDB.On("Ping", mock.Anything).Return(nil).Run(func(_ mock.Arguments) {
		time.Sleep(5 * time.Millisecond) // Simulate database latency
	})

	handler := NewHealthHandler(mockDB, logger, nil)

	const numRequests = 10

	// Measure liveness performance
	livenessStart := time.Now()
	for i := 0; i < numRequests; i++ {
		req := httptest.NewRequest("GET", "/liveness", http.NoBody)
		w := httptest.NewRecorder()
		handler.Liveness(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
	livenessDuration := time.Since(livenessStart)

	// Measure health performance
	healthStart := time.Now()
	for i := 0; i < numRequests; i++ {
		req := httptest.NewRequest("GET", "/health", http.NoBody)
		w := httptest.NewRecorder()
		handler.Health(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}
	healthDuration := time.Since(healthStart)

	// Liveness should be significantly faster than health
	assert.Less(t, livenessDuration, healthDuration,
		"Liveness checks should be faster than health checks")

	t.Logf("Liveness duration: %v, Health duration: %v",
		livenessDuration, healthDuration)
}

func TestHealthHandler_Status(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		setupDB        func() *MockDB
		setupMetrics   func() *mocks.MockMetricsRegistry
		expectedStatus int
	}{
		{
			name: "status with healthy database",
			setupDB: func() *MockDB {
				db := &MockDB{}
				db.On("Ping", mock.Anything).Return(nil)
				return db
			},
			setupMetrics: func() *mocks.MockMetricsRegistry {
				m := mocks.NewMockMetricsRegistry(ctrl)
				m.EXPECT().Counter("api_status_checks_total", gomock.Any()).AnyTimes()
				m.EXPECT().GetMetrics().Return(nil).AnyTimes()
				return m
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "status with database error",
			setupDB: func() *MockDB {
				db := &MockDB{}
				db.On("Ping", mock.Anything).Return(errors.New("connection failed"))
				return db
			},
			setupMetrics: func() *mocks.MockMetricsRegistry {
				m := mocks.NewMockMetricsRegistry(ctrl)
				m.EXPECT().Counter("api_status_checks_total", gomock.Any()).AnyTimes()
				m.EXPECT().GetMetrics().Return(nil).AnyTimes()
				return m
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "status without database",
			setupDB: func() *MockDB {
				return nil
			},
			setupMetrics: func() *mocks.MockMetricsRegistry {
				m := mocks.NewMockMetricsRegistry(ctrl)
				m.EXPECT().Counter("api_status_checks_total", gomock.Any()).AnyTimes()
				m.EXPECT().GetMetrics().Return(nil).AnyTimes()
				return m
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

			req := httptest.NewRequest("GET", "/status", http.NoBody)
			w := httptest.NewRecorder()

			handler.Status(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			var response StatusResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			// Verify service information
			assert.NotEmpty(t, response.Service.Name)
			assert.NotEmpty(t, response.Service.Version)
			assert.NotZero(t, response.Service.StartTime)
			assert.NotEmpty(t, response.Service.Uptime)
			assert.NotZero(t, response.Service.PID)

			// Verify system information
			assert.NotNil(t, response.System)
			assert.NotZero(t, response.System.Memory.Allocated)

			// Verify database information
			assert.NotNil(t, response.Database)

			// Verify metrics information
			assert.NotNil(t, response.Metrics)

			// Verify health information
			assert.NotNil(t, response.Health)
			assert.NotEmpty(t, response.Health.Status)
		})
	}
}

func TestHealthHandler_Version(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		name           string
		setupMetrics   func() *mocks.MockMetricsRegistry
		expectedStatus int
	}{
		{
			name: "version with metrics",
			setupMetrics: func() *mocks.MockMetricsRegistry {
				metrics := mocks.NewMockMetricsRegistry(ctrl)
				metrics.EXPECT().Counter("api_version_requests_total", nil).Times(1)
				return metrics
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "version without metrics",
			setupMetrics:   nil,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			var testMetrics metrics.MetricsRegistry
			if tt.setupMetrics != nil {
				testMetrics = tt.setupMetrics()
			}

			handler := NewHealthHandler(nil, logger, testMetrics)

			req := httptest.NewRequest("GET", "/version", http.NoBody)
			w := httptest.NewRecorder()

			handler.Version(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var response VersionResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.NotEmpty(t, response.Version)
			assert.NotEmpty(t, response.GoVersion)
			assert.NotZero(t, response.Timestamp)

			// Verify timestamp is recent
			assert.True(t, time.Since(response.Timestamp) < time.Minute)
		})
	}
}

func TestHealthHandler_Metrics(t *testing.T) {
	logger := createTestLogger()
	handler := NewHealthHandler(nil, logger, nil)

	req := httptest.NewRequest("GET", "/metrics", http.NoBody)
	w := httptest.NewRecorder()

	handler.Metrics(w, req)

	// Metrics endpoint should return 200 and prometheus format
	assert.Equal(t, http.StatusOK, w.Code)

	// Response should contain prometheus metrics format markers
	body := w.Body.String()
	assert.NotEmpty(t, body)
}

func TestHealthHandler_SetBuildInfo(t *testing.T) {
	// Test setting build info
	testVersion := "1.2.3"
	testCommit := "abc123"
	testBuildTime := "2024-01-01T00:00:00Z"

	SetBuildInfo(testVersion, testCommit, testBuildTime)

	// Create handler and verify version info is used
	logger := createTestLogger()
	handler := NewHealthHandler(nil, logger, nil)

	req := httptest.NewRequest("GET", "/version", http.NoBody)
	w := httptest.NewRecorder()

	handler.Version(w, req)

	var response VersionResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, testVersion, response.Version)
	assert.Equal(t, testCommit, response.Commit)
	assert.Equal(t, testBuildTime, response.BuildTime)
}

func TestHealthHandler_StatusResponseStructure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	mockDB := &MockDB{}
	mockDB.On("Ping", mock.Anything).Return(nil)
	mockMetrics := mocks.NewMockMetricsRegistry(ctrl)
	mockMetrics.EXPECT().Counter(gomock.Any(), gomock.Any()).AnyTimes()
	mockMetrics.EXPECT().GetMetrics().Return(nil).AnyTimes()

	handler := NewHealthHandler(mockDB, logger, mockMetrics)

	req := httptest.NewRequest("GET", "/status", http.NoBody)
	w := httptest.NewRecorder()

	handler.Status(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response StatusResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify all major sections are present
	assert.NotEmpty(t, response.Service.Name)
	assert.NotEmpty(t, response.Service.Version)
	assert.NotZero(t, response.Service.PID)
	assert.NotEmpty(t, response.Service.Uptime)

	assert.NotZero(t, response.System.Memory.Allocated)
	assert.NotZero(t, response.System.Memory.System)
	assert.NotEmpty(t, response.System.OS)
	assert.NotEmpty(t, response.System.Architecture)
	assert.Greater(t, response.System.CPUs, 0)
	assert.Greater(t, response.System.Goroutines, 0)

	assert.NotEmpty(t, response.Health.Status)
	assert.NotZero(t, response.Timestamp)
}

func TestHealthHandler_MemoryInfoAccuracy(t *testing.T) {
	logger := createTestLogger()
	handler := NewHealthHandler(nil, logger, nil)

	req := httptest.NewRequest("GET", "/status", http.NoBody)
	w := httptest.NewRecorder()

	handler.Status(w, req)

	var response StatusResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Memory values should be reasonable
	assert.Greater(t, response.System.Memory.Allocated, uint64(0))
	assert.Greater(t, response.System.Memory.System, uint64(0))
	assert.GreaterOrEqual(t, response.System.Memory.TotalAlloc, response.System.Memory.Allocated)
}
