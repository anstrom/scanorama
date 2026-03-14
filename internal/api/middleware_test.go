package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecoveryMiddleware(t *testing.T) {
	t.Run("handles panics gracefully", func(t *testing.T) {
		cfg := createTestConfig()
		server, err := New(cfg, nil)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/test", http.NoBody)
		rec := httptest.NewRecorder()

		handler := server.recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		}))

		// Should not panic the test
		assert.NotPanics(t, func() {
			handler.ServeHTTP(rec, req)
		})

		// Should return 500 error
		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})

	t.Run("passes through when no panic occurs", func(t *testing.T) {
		cfg := createTestConfig()
		server, err := New(cfg, nil)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/test", http.NoBody)
		rec := httptest.NewRecorder()

		handler := server.recoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		handler.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestContentTypeMiddleware(t *testing.T) {
	cfg := createTestConfig()
	server, err := New(cfg, nil)
	require.NoError(t, err)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("allows GET requests without content type", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", http.NoBody)
		rec := httptest.NewRecorder()

		server.contentTypeMiddleware(okHandler).ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("allows POST with application/json", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", http.NoBody)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		server.contentTypeMiddleware(okHandler).ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("allows POST with no content type", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", http.NoBody)
		rec := httptest.NewRecorder()

		server.contentTypeMiddleware(okHandler).ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("rejects POST with unsupported content type", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", http.NoBody)
		req.Header.Set("Content-Type", "text/xml")
		rec := httptest.NewRecorder()

		server.contentTypeMiddleware(okHandler).ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	})

	t.Run("rejects PUT with unsupported content type", func(t *testing.T) {
		req := httptest.NewRequest("PUT", "/test", http.NoBody)
		req.Header.Set("Content-Type", "text/plain")
		rec := httptest.NewRecorder()

		server.contentTypeMiddleware(okHandler).ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
	})
}

func TestResponseWriter(t *testing.T) {
	t.Run("captures status code", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &responseWriter{ResponseWriter: rec, statusCode: 200}

		rw.WriteHeader(http.StatusNotFound)
		assert.Equal(t, http.StatusNotFound, rw.statusCode)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("default status code is 200", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := &responseWriter{ResponseWriter: rec, statusCode: 200}

		assert.Equal(t, http.StatusOK, rw.statusCode)
	})
}

func TestSecurityHeadersWiring(t *testing.T) {
	t.Run("security headers are set on every response", func(t *testing.T) {
		cfg := createTestConfig()
		server, err := New(cfg, nil)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/api/v1/liveness", http.NoBody)
		rec := httptest.NewRecorder()

		server.router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
		assert.Equal(t, "1; mode=block", rec.Header().Get("X-XSS-Protection"))
		assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
		assert.Equal(t, "default-src 'self'", rec.Header().Get("Content-Security-Policy"))
	})

	t.Run("security headers are present on POST with unsupported content type", func(t *testing.T) {
		cfg := createTestConfig()
		server, err := New(cfg, nil)
		require.NoError(t, err)

		// POST with bad content type triggers a 415 from our contentTypeMiddleware,
		// which runs after the security headers middleware in the chain
		req := httptest.NewRequest("POST", "/api/v1/scans", http.NoBody)
		req.Header.Set("Content-Type", "text/xml")
		rec := httptest.NewRecorder()

		server.router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnsupportedMediaType, rec.Code)
		assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
		assert.Equal(t, "1; mode=block", rec.Header().Get("X-XSS-Protection"))
		assert.Equal(t, "strict-origin-when-cross-origin", rec.Header().Get("Referrer-Policy"))
		assert.Equal(t, "default-src 'self'", rec.Header().Get("Content-Security-Policy"))
	})
}

func TestRateLimitWiring(t *testing.T) {
	t.Run("rate limiting enforced when enabled", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.API.RateLimitEnabled = true
		cfg.API.RateLimitRequests = 3
		cfg.API.RateLimitWindow = time.Minute

		server, err := New(cfg, nil)
		require.NoError(t, err)

		// First 3 requests should succeed
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest("GET", "/api/v1/liveness", http.NoBody)
			req.RemoteAddr = "10.0.0.1:12345"
			rec := httptest.NewRecorder()

			server.router.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code, "request %d should succeed", i+1)
			assert.Equal(t, "3", rec.Header().Get("X-RateLimit-Limit"))
		}

		// 4th request should be rate limited
		req := httptest.NewRequest("GET", "/api/v1/liveness", http.NoBody)
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()

		server.router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	})

	t.Run("rate limiting not enforced when disabled", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.API.RateLimitEnabled = false
		cfg.API.RateLimitRequests = 1
		cfg.API.RateLimitWindow = time.Minute

		server, err := New(cfg, nil)
		require.NoError(t, err)

		// Even with a limit of 1, all requests should pass when disabled
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest("GET", "/api/v1/liveness", http.NoBody)
			req.RemoteAddr = "10.0.0.1:12345"
			rec := httptest.NewRecorder()

			server.router.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code, "request %d should succeed when rate limiting disabled", i+1)
			assert.Empty(t, rec.Header().Get("X-RateLimit-Limit"), "no rate limit headers when disabled")
		}
	})

	t.Run("different clients have separate rate limits", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.API.RateLimitEnabled = true
		cfg.API.RateLimitRequests = 2
		cfg.API.RateLimitWindow = time.Minute

		server, err := New(cfg, nil)
		require.NoError(t, err)

		// Exhaust limit for client A
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest("GET", "/api/v1/liveness", http.NoBody)
			req.RemoteAddr = "10.0.0.1:12345"
			rec := httptest.NewRecorder()
			server.router.ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
		}

		// Client A should be rate limited
		req := httptest.NewRequest("GET", "/api/v1/liveness", http.NoBody)
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()
		server.router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)

		// Client B should still succeed
		req = httptest.NewRequest("GET", "/api/v1/liveness", http.NoBody)
		req.RemoteAddr = "10.0.0.2:12345"
		rec = httptest.NewRecorder()
		server.router.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
