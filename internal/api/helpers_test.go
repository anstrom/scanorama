package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

	t.Run("GetPaginationParams returns correct defaults", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", http.NoBody)

		params := server.GetPaginationParams(req)

		assert.Equal(t, 1, params.Page)
		assert.Equal(t, 20, params.PageSize)
		assert.Equal(t, 0, params.Offset)
	})

	t.Run("GetPaginationParams parses query parameters", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?page=3&page_size=50", http.NoBody)

		params := server.GetPaginationParams(req)

		assert.Equal(t, 3, params.Page)
		assert.Equal(t, 50, params.PageSize)
		assert.Equal(t, 100, params.Offset) // (3-1) * 50
	})

	t.Run("GetPaginationParams enforces maximum page size", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?page_size=1000", http.NoBody)

		params := server.GetPaginationParams(req)

		assert.Equal(t, 100, params.PageSize) // Should be capped at max
	})
}

func TestPaginatedResponse(t *testing.T) {
	cfg := createTestConfig()
	server, err := New(cfg, nil)
	require.NoError(t, err)

	t.Run("WritePaginatedResponse formats correctly", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", http.NoBody)
		rec := httptest.NewRecorder()

		data := []string{"item1", "item2", "item3"}
		totalItems := int64(100)

		params := PaginationParams{
			Page:     1,
			PageSize: 10,
			Offset:   0,
		}
		server.WritePaginatedResponse(rec, req, data, params, totalItems)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var response PaginatedResponse
		err := json.Unmarshal(rec.Body.Bytes(), &response)
		require.NoError(t, err)

		// JSON unmarshaling converts []string to []interface{}
		assert.Len(t, response.Data, 3)
		assert.Equal(t, "item1", response.Data.([]interface{})[0])
		assert.Equal(t, "item2", response.Data.([]interface{})[1])
		assert.Equal(t, "item3", response.Data.([]interface{})[2])

		assert.Equal(t, 1, response.Pagination.Page)
		assert.Equal(t, 10, response.Pagination.PageSize) // Uses actual PageSize from params
		assert.Equal(t, int64(100), response.Pagination.TotalItems)
		assert.Equal(t, 10, response.Pagination.TotalPages) // 100/10
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

	t.Run("handles invalid pagination parameters", func(t *testing.T) {
		cfg := createTestConfig()
		server, err := New(cfg, nil)
		require.NoError(t, err)

		testCases := []struct {
			name     string
			query    string
			expected PaginationParams
		}{
			{"negative page", "page=-1", PaginationParams{Page: 1, PageSize: 20, Offset: 0}},
			{"zero page", "page=0", PaginationParams{Page: 1, PageSize: 20, Offset: 0}},
			{"negative page_size", "page_size=-10", PaginationParams{Page: 1, PageSize: 20, Offset: 0}},
			{"non-numeric page", "page=abc", PaginationParams{Page: 1, PageSize: 20, Offset: 0}},
			{"non-numeric page_size", "page_size=xyz", PaginationParams{Page: 1, PageSize: 20, Offset: 0}},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequest("GET", "/test?"+tc.query, http.NoBody)
				params := server.GetPaginationParams(req)

				assert.Equal(t, tc.expected.Page, params.Page)
				assert.Equal(t, tc.expected.PageSize, params.PageSize)
				assert.Equal(t, tc.expected.Offset, params.Offset)
			})
		}
	})
}

func TestPerformanceCharacteristics(t *testing.T) {
	cfg := createTestConfig()
	server, err := New(cfg, nil)
	require.NoError(t, err)

	t.Run("JSON parsing performance", func(t *testing.T) {
		data := map[string]interface{}{
			"name":        "performance test",
			"description": "testing JSON parsing performance",
			"items":       make([]string, 100),
		}
		jsonData, _ := json.Marshal(data)

		req := httptest.NewRequest("POST", "/test", bytes.NewBuffer(jsonData))
		req.Header.Set("Content-Type", "application/json")

		start := time.Now()
		var parsed map[string]interface{}
		err := server.ParseJSON(req, &parsed)
		duration := time.Since(start)

		require.NoError(t, err)
		assert.Less(t, duration, 10*time.Millisecond, "JSON parsing should be fast")
	})

	t.Run("pagination calculation performance", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?page=1000&page_size=100", http.NoBody)

		start := time.Now()
		params := server.GetPaginationParams(req)
		duration := time.Since(start)

		assert.Equal(t, 1000, params.Page)
		assert.Equal(t, 100, params.PageSize)
		assert.Equal(t, 99900, params.Offset)
		assert.Less(t, duration, time.Millisecond, "Pagination calculation should be very fast")
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

	b.Run("GetPaginationParams", func(b *testing.B) {
		req := httptest.NewRequest("GET", "/test?page=5&page_size=50", http.NoBody)

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			server.GetPaginationParams(req)
		}
	})
}
