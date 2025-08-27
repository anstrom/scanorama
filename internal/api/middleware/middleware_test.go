package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/metrics/mocks"
)

func createTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

// Test RateLimiter.
func TestNewRateLimiter(t *testing.T) {
	tests := []struct {
		name   string
		limit  int
		window time.Duration
	}{
		{"normal limits", 10, time.Minute},
		{"high limits", 1000, time.Second},
		{"low limits", 1, time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewRateLimiter(tt.limit, tt.window)

			assert.NotNil(t, limiter)
			assert.Equal(t, tt.limit, limiter.limit)
			assert.Equal(t, tt.window, limiter.window)
			assert.NotNil(t, limiter.requests)
		})
	}
}

func TestRateLimiter_Allow(t *testing.T) {
	tests := []struct {
		name     string
		limit    int
		window   time.Duration
		requests []string
		expected []bool
	}{
		{
			name:     "under limit",
			limit:    5,
			window:   time.Minute,
			requests: []string{"1.1.1.1", "1.1.1.1", "1.1.1.1"},
			expected: []bool{true, true, true},
		},
		{
			name:     "at limit",
			limit:    2,
			window:   time.Minute,
			requests: []string{"1.1.1.1", "1.1.1.1"},
			expected: []bool{true, true},
		},
		{
			name:     "over limit",
			limit:    2,
			window:   time.Minute,
			requests: []string{"1.1.1.1", "1.1.1.1", "1.1.1.1"},
			expected: []bool{true, true, false},
		},
		{
			name:     "different IPs",
			limit:    1,
			window:   time.Minute,
			requests: []string{"1.1.1.1", "2.2.2.2", "1.1.1.1"},
			expected: []bool{true, true, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewRateLimiter(tt.limit, tt.window)

			for i, ip := range tt.requests {
				result := limiter.Allow(ip)
				assert.Equal(t, tt.expected[i], result,
					"Request %d for IP %s", i+1, ip)
			}
		})
	}
}

func TestRateLimiter_WindowExpiry(t *testing.T) {
	limiter := NewRateLimiter(1, 100*time.Millisecond)

	// First request should be allowed
	assert.True(t, limiter.Allow("1.1.1.1"))

	// Second request should be blocked
	assert.False(t, limiter.Allow("1.1.1.1"))

	// Wait for window to expire
	time.Sleep(150 * time.Millisecond)

	// Request should be allowed again
	assert.True(t, limiter.Allow("1.1.1.1"))
}

func TestRateLimiter_Cleanup(t *testing.T) {
	limiter := NewRateLimiter(10, 100*time.Millisecond)

	// Add some requests
	limiter.Allow("1.1.1.1")
	limiter.Allow("2.2.2.2")
	limiter.Allow("3.3.3.3")

	// Verify requests are tracked
	limiter.mutex.RLock()
	initialCount := len(limiter.requests)
	limiter.mutex.RUnlock()
	assert.Equal(t, 3, initialCount)

	// Wait for entries to become old
	time.Sleep(250 * time.Millisecond)

	// Run cleanup
	limiter.Cleanup()

	// Verify old entries are removed
	limiter.mutex.RLock()
	finalCount := len(limiter.requests)
	limiter.mutex.RUnlock()
	assert.Equal(t, 0, finalCount)
}

func TestLoggingMiddleware(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		path          string
		query         string
		userAgent     string
		contentLength int64
	}{
		{
			name:          "GET request",
			method:        "GET",
			path:          "/api/v1/health",
			query:         "",
			userAgent:     "test-agent/1.0",
			contentLength: 0,
		},
		{
			name:          "POST request with query",
			method:        "POST",
			path:          "/api/v1/scans",
			query:         "format=json&verbose=true",
			userAgent:     "curl/7.68.0",
			contentLength: 123,
		},
		{
			name:          "request with special characters",
			method:        "GET",
			path:          "/api/v1/search",
			query:         "q=test%20search&limit=10",
			userAgent:     "Mozilla/5.0",
			contentLength: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()

			// Create test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify request ID was added to context
				requestID := GetRequestID(r)
				assert.NotEmpty(t, requestID)
				assert.Contains(t, requestID, "req_")

				// Verify start time was added
				if startTime, ok := r.Context().Value(StartTimeKey).(time.Time); ok {
					assert.True(t, time.Since(startTime) < time.Second)
				}

				w.WriteHeader(http.StatusOK)
				w.Write([]byte("test response"))
			})

			// Apply logging middleware
			middleware := Logging(logger)
			handler := middleware(testHandler)

			// Create request
			url := tt.path
			if tt.query != "" {
				url += "?" + tt.query
			}
			req := httptest.NewRequest(tt.method, url, http.NoBody)
			req.Header.Set("User-Agent", tt.userAgent)
			req.ContentLength = tt.contentLength

			w := httptest.NewRecorder()

			// Execute
			handler.ServeHTTP(w, req)

			// Verify response
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, "test response", w.Body.String())

			// Verify request ID header was set
			assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
			assert.Contains(t, w.Header().Get("X-Request-ID"), "req_")
		})
	}
}

func TestMetricsMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		responseStatus int
		responseSize   int
		expectError    bool
	}{
		{
			name:           "successful request",
			method:         "GET",
			path:           "/api/v1/health",
			responseStatus: http.StatusOK,
			responseSize:   100,
			expectError:    false,
		},
		{
			name:           "client error",
			method:         "POST",
			path:           "/api/v1/scans",
			responseStatus: http.StatusBadRequest,
			responseSize:   50,
			expectError:    true,
		},
		{
			name:           "server error",
			method:         "PUT",
			path:           "/api/v1/profiles/123",
			responseStatus: http.StatusInternalServerError,
			responseSize:   200,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			metricsRegistry := mocks.NewMockMetricsRegistry(ctrl)

			// Expect basic metrics calls
			metricsRegistry.EXPECT().Counter("http_requests_total", gomock.Any()).AnyTimes()
			metricsRegistry.EXPECT().Histogram("http_request_duration_seconds", gomock.Any(), gomock.Any()).AnyTimes()
			metricsRegistry.EXPECT().Histogram("http_response_size_bytes", gomock.Any(), gomock.Any()).AnyTimes()

			// For error requests, expect error counter
			if tt.responseStatus >= 400 {
				metricsRegistry.EXPECT().Counter("http_errors_total", gomock.Any()).AnyTimes()
			}

			// Create test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.responseStatus)
				w.Write(make([]byte, tt.responseSize))
			})

			// Apply metrics middleware
			middleware := Metrics(metricsRegistry)
			handler := middleware(testHandler)

			// Create request
			req := httptest.NewRequest(tt.method, tt.path, http.NoBody)
			w := httptest.NewRecorder()

			// Execute
			handler.ServeHTTP(w, req)

			// Verify response
			assert.Equal(t, tt.responseStatus, w.Code)
			assert.Equal(t, tt.responseSize, w.Body.Len())

			// GoMock automatically verifies expectations when ctrl.Finish() is called
		})
	}
}

func TestRecoveryMiddleware(t *testing.T) {
	tests := []struct {
		name        string
		panicValue  interface{}
		shouldPanic bool
	}{
		{
			name:        "string panic",
			panicValue:  "something went wrong",
			shouldPanic: true,
		},
		{
			name:        "error panic",
			panicValue:  fmt.Errorf("test error"),
			shouldPanic: true,
		},
		{
			name:        "no panic",
			panicValue:  nil,
			shouldPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()

			// Create test handler that may panic
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.shouldPanic {
					panic(tt.panicValue)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			})

			// Apply recovery middleware
			middleware := Recovery(logger)
			handler := middleware(testHandler)

			// Create request with request ID
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			ctx := context.WithValue(req.Context(), RequestIDKey, "test-req-123")
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			// Execute - should not panic
			assert.NotPanics(t, func() {
				handler.ServeHTTP(w, req)
			})

			if tt.shouldPanic {
				// Should return 500 error
				assert.Equal(t, http.StatusInternalServerError, w.Code)
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Equal(t, "Internal server error", response["error"])
				assert.Equal(t, "test-req-123", response["request_id"])
			} else {
				assert.Equal(t, http.StatusOK, w.Code)
				assert.Equal(t, "success", w.Body.String())
			}
		})
	}
}

func TestAuthenticationMiddleware(t *testing.T) {
	validConfigKeys := []string{"key1", "key2", "secret-key-123"}

	tests := []struct {
		name           string
		path           string
		headers        map[string]string
		expectedStatus int
		shouldCallNext bool
	}{
		{
			name:           "valid API key in X-API-Key header",
			path:           "/api/v1/scans",
			headers:        map[string]string{"X-API-Key": "key1"},
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "valid API key in Authorization header",
			path:           "/api/v1/profiles",
			headers:        map[string]string{"Authorization": "Bearer key2"},
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "invalid API key",
			path:           "/api/v1/scans",
			headers:        map[string]string{"X-API-Key": "invalid"},
			expectedStatus: http.StatusUnauthorized,
			shouldCallNext: false,
		},
		{
			name:           "missing API key",
			path:           "/api/v1/scans",
			headers:        map[string]string{},
			expectedStatus: http.StatusUnauthorized,
			shouldCallNext: false,
		},
		{
			name:           "health endpoint bypass",
			path:           "/api/v1/health",
			headers:        map[string]string{},
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "version endpoint bypass",
			path:           "/api/v1/version",
			headers:        map[string]string{},
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "liveness endpoint bypass",
			path:           "/api/v1/liveness",
			headers:        map[string]string{},
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			nextCalled := false

			// Create test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("authenticated"))
			})

			// Apply authentication middleware (config-based keys only for this test)
			middleware := Authentication(validConfigKeys, nil, logger)
			handler := middleware(testHandler)

			// Create request
			req := httptest.NewRequest("GET", tt.path, http.NoBody)
			ctx := context.WithValue(req.Context(), RequestIDKey, "test-req-123")
			req = req.WithContext(ctx)

			// Set headers
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			w := httptest.NewRecorder()

			// Execute
			handler.ServeHTTP(w, req)

			// Verify response
			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Equal(t, tt.shouldCallNext, nextCalled)

			if tt.expectedStatus == http.StatusUnauthorized {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Contains(t, response["error"], "Authentication")
				assert.Equal(t, "test-req-123", response["request_id"])
			}
		})
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		requests       int
		window         time.Duration
		clientRequests []string
		expectedStatus []int
	}{
		{
			name:           "under limit",
			requests:       5,
			window:         time.Minute,
			clientRequests: []string{"1.1.1.1", "1.1.1.1"},
			expectedStatus: []int{http.StatusOK, http.StatusOK},
		},
		{
			name:           "over limit",
			requests:       2,
			window:         time.Minute,
			clientRequests: []string{"1.1.1.1", "1.1.1.1", "1.1.1.1"},
			expectedStatus: []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests},
		},
		{
			name:           "different IPs",
			requests:       1,
			window:         time.Minute,
			clientRequests: []string{"1.1.1.1", "2.2.2.2"},
			expectedStatus: []int{http.StatusOK, http.StatusOK},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()

			// Create test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			})

			// Apply rate limit middleware
			middleware := RateLimit(tt.requests, tt.window, logger)
			handler := middleware(testHandler)

			for i, clientIP := range tt.clientRequests {
				req := httptest.NewRequest("GET", "/test", http.NoBody)
				req.RemoteAddr = clientIP + ":12345"
				ctx := context.WithValue(req.Context(), RequestIDKey, fmt.Sprintf("req-%d", i))
				req = req.WithContext(ctx)

				w := httptest.NewRecorder()

				handler.ServeHTTP(w, req)

				assert.Equal(t, tt.expectedStatus[i], w.Code)

				// Check rate limit headers
				assert.Equal(t, strconv.Itoa(tt.requests), w.Header().Get("X-RateLimit-Limit"))
				assert.Equal(t, tt.window.String(), w.Header().Get("X-RateLimit-Window"))

				if tt.expectedStatus[i] == http.StatusTooManyRequests {
					var response map[string]interface{}
					err := json.Unmarshal(w.Body.Bytes(), &response)
					require.NoError(t, err)

					assert.Equal(t, "Rate limit exceeded", response["error"])
					assert.Contains(t, response["message"], fmt.Sprintf("%d requests", tt.requests))
				}
			}
		})
	}
}

func TestContentTypeMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		contentType    string
		expectedStatus int
		shouldCallNext bool
	}{
		{
			name:           "GET request - no validation",
			method:         "GET",
			contentType:    "",
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "DELETE request - no validation",
			method:         "DELETE",
			contentType:    "",
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "OPTIONS request - no validation",
			method:         "OPTIONS",
			contentType:    "",
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "POST with valid JSON",
			method:         "POST",
			contentType:    "application/json",
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "POST with JSON charset",
			method:         "POST",
			contentType:    "application/json; charset=utf-8",
			expectedStatus: http.StatusOK,
			shouldCallNext: true,
		},
		{
			name:           "POST with invalid content type",
			method:         "POST",
			contentType:    "text/plain",
			expectedStatus: http.StatusUnsupportedMediaType,
			shouldCallNext: false,
		},
		{
			name:           "PUT with invalid content type",
			method:         "PUT",
			contentType:    "application/xml",
			expectedStatus: http.StatusUnsupportedMediaType,
			shouldCallNext: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled := false

			// Create test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			})

			// Apply content type middleware
			middleware := ContentType()
			handler := middleware(testHandler)

			// Create request
			req := httptest.NewRequest(tt.method, "/test", http.NoBody)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			ctx := context.WithValue(req.Context(), RequestIDKey, "test-req-123")
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			// Execute
			handler.ServeHTTP(w, req)

			// Verify
			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Equal(t, tt.shouldCallNext, nextCalled)

			if tt.expectedStatus == http.StatusUnsupportedMediaType {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Equal(t, "Unsupported media type", response["error"])
				assert.Equal(t, "application/json", response["expected"])
			}
		})
	}
}

func TestCORSMiddleware(t *testing.T) {
	tests := []struct {
		name            string
		origins         []string
		headers         []string
		methods         []string
		requestOrigin   string
		requestMethod   string
		expectedHeaders map[string]string
		shouldCallNext  bool
	}{
		{
			name:          "wildcard origin",
			origins:       []string{"*"},
			headers:       []string{"Content-Type", "Authorization"},
			methods:       []string{"GET", "POST", "PUT", "DELETE"},
			requestOrigin: "https://example.com",
			requestMethod: "GET",
			expectedHeaders: map[string]string{
				"Access-Control-Allow-Origin":      "https://example.com",
				"Access-Control-Allow-Headers":     "Content-Type, Authorization",
				"Access-Control-Allow-Methods":     "GET, POST, PUT, DELETE",
				"Access-Control-Allow-Credentials": "true",
				"Access-Control-Max-Age":           "3600",
			},
			shouldCallNext: true,
		},
		{
			name:          "OPTIONS preflight request",
			origins:       []string{"*"},
			headers:       []string{"Content-Type"},
			methods:       []string{"GET", "POST"},
			requestOrigin: "https://example.com",
			requestMethod: "OPTIONS",
			expectedHeaders: map[string]string{
				"Access-Control-Allow-Origin":      "https://example.com",
				"Access-Control-Allow-Headers":     "Content-Type",
				"Access-Control-Allow-Methods":     "GET, POST",
				"Access-Control-Allow-Credentials": "true",
			},
			shouldCallNext: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nextCalled := false

			// Create test handler
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			})

			// Apply CORS middleware
			middleware := CORS(tt.origins, tt.headers, tt.methods)
			handler := middleware(testHandler)

			// Create request
			req := httptest.NewRequest(tt.requestMethod, "/test", http.NoBody)
			if tt.requestOrigin != "" {
				req.Header.Set("Origin", tt.requestOrigin)
			}

			w := httptest.NewRecorder()

			// Execute
			handler.ServeHTTP(w, req)

			// Verify headers
			for key, expectedValue := range tt.expectedHeaders {
				assert.Equal(t, expectedValue, w.Header().Get(key))
			}

			// Handle OPTIONS requests
			if tt.requestMethod == "OPTIONS" {
				assert.Equal(t, http.StatusNoContent, w.Code)
				assert.False(t, nextCalled)
			} else {
				assert.Equal(t, tt.shouldCallNext, nextCalled)
			}
		})
	}
}

func TestRequestTimeoutMiddleware(t *testing.T) {
	tests := []struct {
		name            string
		timeout         time.Duration
		handlerDelay    time.Duration
		expectedTimeout bool
	}{
		{
			name:            "request within timeout",
			timeout:         100 * time.Millisecond,
			handlerDelay:    10 * time.Millisecond,
			expectedTimeout: false,
		},
		{
			name:            "request exceeds timeout",
			timeout:         10 * time.Millisecond,
			handlerDelay:    50 * time.Millisecond,
			expectedTimeout: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			completed := false

			// Create test handler with delay
			testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(tt.handlerDelay):
					completed = true
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("completed"))
				}
			})

			// Apply timeout middleware
			middleware := RequestTimeout(tt.timeout)
			handler := middleware(testHandler)

			// Create request
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			w := httptest.NewRecorder()

			// Execute
			start := time.Now()
			handler.ServeHTTP(w, req)
			duration := time.Since(start)

			if tt.expectedTimeout {
				assert.True(t, duration < tt.timeout+20*time.Millisecond)
				assert.False(t, completed)
			} else {
				assert.True(t, completed)
				assert.Equal(t, http.StatusOK, w.Code)
				assert.Equal(t, "completed", w.Body.String())
			}
		})
	}
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	expectedHeaders := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"X-XSS-Protection":        "1; mode=block",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"Content-Security-Policy": "default-src 'self'",
	}

	// Create test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("secure"))
	})

	// Apply security headers middleware
	middleware := SecurityHeaders()
	handler := middleware(testHandler)

	// Create request
	req := httptest.NewRequest("GET", "/test", http.NoBody)
	w := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "secure", w.Body.String())

	// Verify all security headers are set
	for key, expectedValue := range expectedHeaders {
		assert.Equal(t, expectedValue, w.Header().Get(key))
	}
}

func TestCompressionMiddleware(t *testing.T) {
	// Create test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	// Apply compression middleware
	middleware := Compression()
	handler := middleware(testHandler)

	// Create request
	req := httptest.NewRequest("GET", "/test", http.NoBody)
	w := httptest.NewRecorder()

	// Execute
	handler.ServeHTTP(w, req)

	// Verify (currently just passes through)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "test response", w.Body.String())
}

func TestGenerateRequestID(t *testing.T) {
	// Test that IDs are unique and have correct format
	ids := make(map[string]bool)
	const numIDs = 1000

	for i := 0; i < numIDs; i++ {
		id := generateRequestID()

		// Should start with "req_"
		assert.True(t, strings.HasPrefix(id, "req_"))

		// Should be unique
		assert.False(t, ids[id], "Generated duplicate ID: %s", id)
		ids[id] = true

		// Should have reasonable length
		assert.True(t, len(id) > 8, "ID too short: %s", id)
		assert.True(t, len(id) < 50, "ID too long: %s", id)
	}
}

func TestGetRequestID(t *testing.T) {
	tests := []struct {
		name       string
		setupCtx   func() context.Context
		expectedID string
	}{
		{
			name: "with request ID in context",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), RequestIDKey, "test-req-123")
			},
			expectedID: "test-req-123",
		},
		{
			name:       "without request ID in context",
			setupCtx:   context.Background,
			expectedID: "unknown",
		},
		{
			name: "with wrong type in context",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), RequestIDKey, 12345)
			},
			expectedID: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			req = req.WithContext(tt.setupCtx())

			id := GetRequestID(req)
			assert.Equal(t, tt.expectedID, id)
		})
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		expectedIP string
	}{
		{
			name:       "X-Forwarded-For single IP",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.1"},
			remoteAddr: "10.0.0.1:12345",
			expectedIP: "192.168.1.1",
		},
		{
			name:       "X-Forwarded-For multiple IPs",
			headers:    map[string]string{"X-Forwarded-For": "192.168.1.1, 10.0.0.1, 172.16.0.1"},
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "192.168.1.1",
		},
		{
			name:       "X-Real-IP header",
			headers:    map[string]string{"X-Real-IP": "203.0.113.1"},
			remoteAddr: "10.0.0.1:12345",
			expectedIP: "203.0.113.1",
		},
		{
			name:       "RemoteAddr fallback",
			headers:    map[string]string{},
			remoteAddr: "198.51.100.1:54321",
			expectedIP: "198.51.100.1",
		},
		{
			name:       "invalid RemoteAddr",
			headers:    map[string]string{},
			remoteAddr: "invalid",
			expectedIP: "unknown",
		},
		{
			name: "X-Forwarded-For precedence over X-Real-IP",
			headers: map[string]string{
				"X-Forwarded-For": "192.168.1.1",
				"X-Real-IP":       "10.0.0.1",
			},
			remoteAddr: "127.0.0.1:12345",
			expectedIP: "192.168.1.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			req.RemoteAddr = tt.remoteAddr

			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			ip := getClientIP(req)
			assert.Equal(t, tt.expectedIP, ip)
		})
	}
}

func TestResponseWriter(t *testing.T) {
	t.Run("captures status code and size", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		wrapper := &responseWriter{
			ResponseWriter: recorder,
			statusCode:     http.StatusOK,
			size:           0,
		}

		// Test WriteHeader
		wrapper.WriteHeader(http.StatusCreated)
		assert.Equal(t, http.StatusCreated, wrapper.statusCode)

		// Test Write
		testData := []byte("test response data")
		n, err := wrapper.Write(testData)
		assert.NoError(t, err)
		assert.Equal(t, len(testData), n)
		assert.Equal(t, len(testData), wrapper.size)

		// Test multiple writes accumulate size
		moreData := []byte(" more data")
		n2, err2 := wrapper.Write(moreData)
		assert.NoError(t, err2)
		assert.Equal(t, len(moreData), n2)
		assert.Equal(t, len(testData)+len(moreData), wrapper.size)
	})
}

func TestMiddlewareChaining(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	metricsRegistry := mocks.NewMockMetricsRegistry(ctrl)

	// Setup metrics expectations
	metricsRegistry.EXPECT().Counter("http_requests_total", gomock.Any()).AnyTimes()
	metricsRegistry.EXPECT().Histogram("http_request_duration_seconds", gomock.Any(), gomock.Any()).AnyTimes()
	metricsRegistry.EXPECT().Histogram("http_response_size_bytes", gomock.Any(), gomock.Any()).AnyTimes()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify middleware effects
		requestID := GetRequestID(r)
		assert.NotEmpty(t, requestID)
		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("chained response"))
	})

	// Chain multiple middleware
	handler := SecurityHeaders()(
		Logging(logger)(
			Metrics(metricsRegistry)(
				Recovery(logger)(testHandler))))

	req := httptest.NewRequest("GET", "/test", http.NoBody)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "chained response", w.Body.String())
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))

	// GoMock automatically verifies expectations when ctrl.Finish() is called
}

func TestMiddleware_EdgeCases(t *testing.T) {
	t.Run("nil logger handling", func(t *testing.T) {
		assert.NotPanics(t, func() {
			middleware := Logging(nil)
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/test", http.NoBody)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		})
	})

	t.Run("nil metrics handling", func(t *testing.T) {
		assert.NotPanics(t, func() {
			middleware := Metrics(nil)
			handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/test", http.NoBody)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
		})
	})

	t.Run("empty API keys", func(t *testing.T) {
		logger := createTestLogger()
		middleware := Authentication([]string{}, nil, logger)
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/api/v1/scans", http.NoBody)
		req.Header.Set("X-API-Key", "any-key")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestMiddleware_ConcurrentSafety(t *testing.T) {
	t.Run("rate limiter concurrent access", func(t *testing.T) {
		limiter := NewRateLimiter(1000, time.Minute)

		const numGoroutines = 50
		const requestsPerGoroutine = 20
		var wg sync.WaitGroup

		results := make(chan bool, numGoroutines*requestsPerGoroutine)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				ip := fmt.Sprintf("192.168.%d.1", id%256)

				for j := 0; j < requestsPerGoroutine; j++ {
					result := limiter.Allow(ip)
					results <- result
				}
			}(i)
		}

		wg.Wait()
		close(results)

		// Count results
		allowedCount := 0
		for result := range results {
			if result {
				allowedCount++
			}
		}

		// Should handle concurrent access without issues
		assert.Greater(t, allowedCount, 0)
		assert.LessOrEqual(t, allowedCount, numGoroutines*requestsPerGoroutine)
	})

	t.Run("logging middleware concurrent requests", func(t *testing.T) {
		logger := createTestLogger()

		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(1 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})

		middleware := Logging(logger)
		handler := middleware(testHandler)

		const numRequests = 20
		var wg sync.WaitGroup

		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				req := httptest.NewRequest("GET", "/test", http.NoBody)
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, req)

				assert.Equal(t, http.StatusOK, w.Code)
				assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
			}()
		}

		wg.Wait()
	})
}

func TestAuthenticationMiddleware_EdgeCases(t *testing.T) {
	logger := createTestLogger()
	validKeys := []string{"valid-key-1", "valid-key-2"}

	t.Run("case sensitivity", func(t *testing.T) {
		middleware := Authentication(validKeys, nil, logger)
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/api/v1/scans", http.NoBody)
		req.Header.Set("X-API-Key", "VALID-KEY-1") // Wrong case
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("both headers present", func(t *testing.T) {
		middleware := Authentication(validKeys, nil, logger)
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/api/v1/scans", http.NoBody)
		req.Header.Set("X-API-Key", "valid-key-1")
		req.Header.Set("Authorization", "Bearer invalid-key")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code) // X-API-Key takes precedence
	})
}

// Benchmark tests.
func BenchmarkLoggingMiddleware(b *testing.B) {
	logger := createTestLogger()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := Logging(logger)
	handler := middleware(testHandler)

	req := httptest.NewRequest("GET", "/test", http.NoBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkMetricsMiddleware(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	metricsRegistry := mocks.NewMockMetricsRegistry(ctrl)
	metricsRegistry.EXPECT().Counter(gomock.Any(), gomock.Any()).AnyTimes()
	metricsRegistry.EXPECT().Histogram(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := Metrics(metricsRegistry)
	handler := middleware(testHandler)

	req := httptest.NewRequest("GET", "/test", http.NoBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}

func BenchmarkRateLimiter_Allow(b *testing.B) {
	limiter := NewRateLimiter(1000, time.Minute)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow("192.168.1.1")
	}
}

func BenchmarkGenerateRequestID(t *testing.B) {
	for i := 0; i < t.N; i++ {
		_ = generateRequestID()
	}
}

// TestAuthenticationBehaviors tests the key authentication behaviors
func TestAuthenticationBehaviors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	scenarios := []struct {
		name         string
		apiKeys      []string
		requestPath  string
		authHeader   string
		expectAccess bool
		expectStatus int
		behaviorDesc string
	}{
		{
			name:         "should allow access with valid API key in X-API-Key header",
			apiKeys:      []string{"valid-key-123", "another-valid-key"},
			requestPath:  "/api/v1/hosts",
			authHeader:   "X-API-Key:valid-key-123",
			expectAccess: true,
			expectStatus: http.StatusOK,
			behaviorDesc: "Valid API key should grant access to protected endpoints",
		},
		{
			name:         "should allow access with valid API key in Authorization header",
			apiKeys:      []string{"bearer-token-456"},
			requestPath:  "/api/v1/networks",
			authHeader:   "Authorization:Bearer bearer-token-456",
			expectAccess: true,
			expectStatus: http.StatusOK,
			behaviorDesc: "Bearer token format should work for authentication",
		},
		{
			name:         "should reject access with invalid API key",
			apiKeys:      []string{"valid-key"},
			requestPath:  "/api/v1/scans",
			authHeader:   "X-API-Key:invalid-key",
			expectAccess: false,
			expectStatus: http.StatusUnauthorized,
			behaviorDesc: "Invalid API key should be rejected with 401",
		},
		{
			name:         "should reject access when no API key provided",
			apiKeys:      []string{"some-key"},
			requestPath:  "/api/v1/profiles",
			authHeader:   "",
			expectAccess: false,
			expectStatus: http.StatusUnauthorized,
			behaviorDesc: "Missing API key should result in authentication failure",
		},
		{
			name:         "should bypass authentication for health endpoints",
			apiKeys:      []string{"some-key"},
			requestPath:  "/api/v1/health",
			authHeader:   "",
			expectAccess: true,
			expectStatus: http.StatusOK,
			behaviorDesc: "Health endpoints should not require authentication",
		},
		{
			name:         "should bypass authentication for version endpoints",
			apiKeys:      []string{"some-key"},
			requestPath:  "/api/v1/version",
			authHeader:   "",
			expectAccess: true,
			expectStatus: http.StatusOK,
			behaviorDesc: "Version endpoints should be publicly accessible",
		},
		{
			name:         "should bypass authentication for liveness endpoints",
			apiKeys:      []string{"some-key"},
			requestPath:  "/api/v1/liveness",
			authHeader:   "",
			expectAccess: true,
			expectStatus: http.StatusOK,
			behaviorDesc: "Liveness endpoints should be publicly accessible for monitoring",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Create handler that tracks if it was called
			handlerCalled := false
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerCalled = true
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			})

			// Apply authentication middleware
			authMiddleware := Authentication(scenario.apiKeys, nil, logger)
			protectedHandler := authMiddleware(handler)

			// Create request
			req := httptest.NewRequest("GET", scenario.requestPath, nil)
			if scenario.authHeader != "" {
				parts := strings.SplitN(scenario.authHeader, ":", 2)
				req.Header.Set(parts[0], parts[1])
			}

			// Record response
			recorder := httptest.NewRecorder()
			protectedHandler.ServeHTTP(recorder, req)

			// Verify behavior
			assert.Equal(t, scenario.expectStatus, recorder.Code,
				"Status code should match expected behavior: %s", scenario.behaviorDesc)

			assert.Equal(t, scenario.expectAccess, handlerCalled,
				"Handler access should match expectation: %s", scenario.behaviorDesc)

			// Verify response structure for failures
			if !scenario.expectAccess {
				var response map[string]interface{}
				err := json.Unmarshal(recorder.Body.Bytes(), &response)
				assert.NoError(t, err, "Error response should be valid JSON")
				assert.Contains(t, response, "error", "Error response should contain error field")
				assert.Contains(t, response, "request_id", "Error response should contain request ID")
				assert.Contains(t, response, "timestamp", "Error response should contain timestamp")
			}
		})
	}
}

// TestAPIKeySecurityBehaviors tests security-related behaviors
func TestAPIKeySecurityBehaviors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))

	t.Run("should handle malformed API keys safely", func(t *testing.T) {
		malformedKeys := []string{
			"key with spaces",
			"key\nwith\nnewlines",
			"key\twith\ttabs",
			"key;with;semicolons",
			"key\"with\"quotes",
			"key'with'apostrophes",
			"key<with>brackets",
			string([]byte{0x00, 0x01, 0x02}), // non-printable characters
		}

		apiKeys := []string{"valid-key"}
		authMiddleware := Authentication(apiKeys, nil, logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		for _, malformedKey := range malformedKeys {
			req := httptest.NewRequest("GET", "/api/v1/test", nil)
			req.Header.Set("X-API-Key", malformedKey)

			recorder := httptest.NewRecorder()
			authMiddleware(handler).ServeHTTP(recorder, req)

			assert.Equal(t, http.StatusUnauthorized, recorder.Code,
				"Malformed key should be rejected: %q", malformedKey)
		}
	})

	t.Run("should handle empty API key list gracefully", func(t *testing.T) {
		authMiddleware := Authentication([]string{}, nil, logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Handler should not be called with empty API key list")
		})

		req := httptest.NewRequest("GET", "/api/v1/test", nil)
		req.Header.Set("X-API-Key", "any-key")

		recorder := httptest.NewRecorder()
		authMiddleware(handler).ServeHTTP(recorder, req)

		assert.Equal(t, http.StatusUnauthorized, recorder.Code,
			"Empty API key list should reject all requests")
	})

	t.Run("should handle concurrent authentication requests safely", func(t *testing.T) {
		apiKeys := []string{"concurrent-key"}
		authMiddleware := Authentication(apiKeys, nil, logger)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		const numRequests = 100
		var wg sync.WaitGroup
		results := make(chan int, numRequests)

		// Launch concurrent requests
		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()

				req := httptest.NewRequest("GET", "/api/v1/test", nil)
				req.Header.Set("X-API-Key", "concurrent-key")

				recorder := httptest.NewRecorder()
				authMiddleware(handler).ServeHTTP(recorder, req)

				results <- recorder.Code
			}()
		}

		wg.Wait()
		close(results)

		// Verify all requests succeeded
		successCount := 0
		for code := range results {
			if code == http.StatusOK {
				successCount++
			}
		}

		assert.Equal(t, numRequests, successCount,
			"All concurrent requests with valid API key should succeed")
	})
}
