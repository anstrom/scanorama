package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerHelperMethods(t *testing.T) {
	cfg := createTestConfig()
	server, err := New(cfg, nil)
	require.NoError(t, err)

	t.Run("WriteJSON writes correct JSON response", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", http.NoBody)
		rec := httptest.NewRecorder()

		data := map[string]interface{}{
			"message": "test",
			"value":   42,
		}

		server.WriteJSON(rec, req, http.StatusOK, data)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var response map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "test", response["message"])
		assert.Equal(t, float64(42), response["value"]) // JSON numbers are float64
	})

	t.Run("ParseJSON parses request body correctly", func(t *testing.T) {
		requestData := map[string]interface{}{
			"name":  "test",
			"count": 5,
		}
		jsonData, _ := json.Marshal(requestData)

		req := httptest.NewRequest("POST", "/test", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")

		var parsed map[string]interface{}
		err := server.ParseJSON(req, &parsed)

		require.NoError(t, err)
		assert.Equal(t, "test", parsed["name"])
		assert.Equal(t, float64(5), parsed["count"])
	})

	t.Run("ParseJSON handles invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", strings.NewReader("invalid json"))
		req.Header.Set("Content-Type", "application/json")

		var parsed map[string]interface{}
		err := server.ParseJSON(req, &parsed)

		assert.Error(t, err)
	})
}

func TestErrorHandling(t *testing.T) {
	cfg := createTestConfig()
	server, err := New(cfg, nil)
	require.NoError(t, err)

	t.Run("writeError creates proper error response", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", http.NoBody)
		rec := httptest.NewRecorder()

		testErr := fmt.Errorf("test error message")
		server.writeError(rec, req, http.StatusBadRequest, testErr)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var response ErrorResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "test error message", response.Error)
		assert.NotEmpty(t, response.Timestamp)
		assert.NotEmpty(t, response.RequestID)
	})
}

func TestServerEdgeCases(t *testing.T) {
	t.Run("handles extremely large JSON payloads", func(t *testing.T) {
		cfg := createTestConfig()
		server, err := New(cfg, nil)
		require.NoError(t, err)

		// Create a large JSON payload (near the limit)
		largeData := map[string]interface{}{
			"data": strings.Repeat("x", testPayloadSize1MB), // 1MB string
		}
		jsonData, _ := json.Marshal(largeData)

		req := httptest.NewRequest("POST", "/test", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")

		var parsed map[string]interface{}
		err = server.ParseJSON(req, &parsed)

		// Should handle large payloads gracefully (success or documented failure)
		if err != nil {
			// Error should be reasonable
			assert.Contains(t, err.Error(), "request entity too large", "large payload error should be descriptive")
		}
	})

	t.Run("handles empty request bodies", func(t *testing.T) {
		cfg := createTestConfig()
		server, err := New(cfg, nil)
		require.NoError(t, err)

		req := httptest.NewRequest("POST", "/test", http.NoBody)
		req.Header.Set("Content-Type", "application/json")

		var parsed map[string]interface{}
		err = server.ParseJSON(req, &parsed)

		assert.Error(t, err, "Empty body should result in error")
	})
}

func BenchmarkServerOperations(b *testing.B) {
	cfg := createTestConfig()
	server, err := New(cfg, nil)
	require.NoError(b, err)

	b.Run("WriteJSON", func(b *testing.B) {
		data := map[string]interface{}{
			"message": "benchmark test",
			"value":   42,
			"items":   []string{"a", "b", "c"},
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			rec := httptest.NewRecorder()
			server.WriteJSON(rec, req, http.StatusOK, data)
		}
	})

	b.Run("ParseJSON", func(b *testing.B) {
		data := map[string]interface{}{
			"name":  "benchmark",
			"count": 100,
			"items": []string{"item1", "item2", "item3"},
		}
		jsonData, _ := json.Marshal(data)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			req := httptest.NewRequest("POST", "/test", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			var parsed map[string]interface{}
			server.ParseJSON(req, &parsed)
		}
	})
}
