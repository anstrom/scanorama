package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBuiltinHandlers(t *testing.T) {
	cfg := createTestConfig()
	database := createTestDatabase()
	database.On("Ping", mock.Anything).Return(nil)

	server, err := New(cfg, nil)
	require.NoError(t, err)

	tests := []struct {
		name           string
		path           string
		method         string
		expectedStatus int
		checkResponse  func(t *testing.T, body []byte)
	}{
		{
			name:           "liveness endpoint",
			path:           "/api/v1/liveness",
			method:         "GET",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "alive", response["status"])
			},
		},
		{
			name:           "health endpoint",
			path:           "/api/v1/health",
			method:         "GET",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Equal(t, "healthy", response["status"])
				assert.Contains(t, response, "timestamp")
				assert.Contains(t, response, "checks")
			},
		},
		{
			name:           "version endpoint",
			path:           "/api/v1/version",
			method:         "GET",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				require.NoError(t, err)
				assert.Contains(t, response, "version")
				assert.Contains(t, response, "service")
			},
		},
		{
			name:           "metrics endpoint",
			path:           "/api/v1/metrics",
			method:         "GET",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				// Metrics should return some form of metrics data
				assert.NotEmpty(t, body)
			},
		},
		{
			name:           "root redirect",
			path:           "/",
			method:         "GET",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, body []byte) {
				// Returns a simple response instead of redirect
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, http.NoBody)
			rec := httptest.NewRecorder()

			server.router.ServeHTTP(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.checkResponse != nil {
				tt.checkResponse(t, rec.Body.Bytes())
			}
		})
	}
}

func TestMetricsExporter_ExposesHTTPMetricsWithRouteTemplate(t *testing.T) {
	cfg := createTestConfig()
	server, err := New(cfg, nil)
	require.NoError(t, err)

	// 1) Hit a real route to exercise loggingMiddleware and metrics
	req := httptest.NewRequest(http.MethodGet, "/api/v1/version", http.NoBody)
	rec := httptest.NewRecorder()
	server.router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// 2) Fetch Prometheus metrics
	mreq := httptest.NewRequest(http.MethodGet, "/metrics", http.NoBody)
	mrec := httptest.NewRecorder()
	server.router.ServeHTTP(mrec, mreq)
	require.Equal(t, http.StatusOK, mrec.Code)

	body := mrec.Body.String()
	// Ensure our API request counter is present with templated path
	require.Contains(t, body, "scanorama_api_requests_total")
	require.Contains(t, body, "path=\"/api/v1/version\"")
	require.Contains(t, body, "method=\"GET\"")

	// Standard Go collector should be registered
	require.Contains(t, body, "go_goroutines")
}
