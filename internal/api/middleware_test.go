package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
