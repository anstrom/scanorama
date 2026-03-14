package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPaginationParams(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    map[string]string
		expectedParams PaginationParams
		expectedErr    bool
	}{
		{
			name:        "default parameters",
			queryParams: map[string]string{},
			expectedParams: PaginationParams{
				Page:     1,
				PageSize: 50,
				Offset:   0,
			},
			expectedErr: false,
		},
		{
			name:        "custom valid parameters",
			queryParams: map[string]string{"page": "3", "page_size": "25"},
			expectedParams: PaginationParams{
				Page:     3,
				PageSize: 25,
				Offset:   50,
			},
			expectedErr: false,
		},
		{
			name:        "invalid page parameter",
			queryParams: map[string]string{"page": "invalid"},
			expectedErr: true,
		},
		{
			name:        "invalid page_size parameter",
			queryParams: map[string]string{"page_size": "invalid"},
			expectedErr: true,
		},
		{
			name:        "negative page number",
			queryParams: map[string]string{"page": "-1"},
			expectedParams: PaginationParams{
				Page:     1,
				PageSize: 50,
				Offset:   0,
			},
			expectedErr: false,
		},
		{
			name:        "zero page size",
			queryParams: map[string]string{"page_size": "0"},
			expectedParams: PaginationParams{
				Page:     1,
				PageSize: 50,
				Offset:   0,
			},
			expectedErr: false,
		},
		{
			name:        "page size exceeds maximum",
			queryParams: map[string]string{"page_size": "2000"},
			expectedParams: PaginationParams{
				Page:     1,
				PageSize: 1000,
				Offset:   0,
			},
			expectedErr: false,
		},
		{
			name:        "large page number",
			queryParams: map[string]string{"page": "100", "page_size": "10"},
			expectedParams: PaginationParams{
				Page:     100,
				PageSize: 10,
				Offset:   990,
			},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request with query parameters
			url := "/test?"
			for key, value := range tt.queryParams {
				url += fmt.Sprintf("%s=%s&", key, value)
			}
			req := httptest.NewRequest("GET", url, http.NoBody)

			params, err := getPaginationParams(req)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedParams, params)
			}
		})
	}
}

func TestWritePaginatedResponse(t *testing.T) {
	tests := []struct {
		name       string
		data       interface{}
		params     PaginationParams
		totalItems int64
		expected   PaginatedResponse
	}{
		{
			name: "first page",
			data: []string{"item1", "item2", "item3"},
			params: PaginationParams{
				Page:     1,
				PageSize: 10,
				Offset:   0,
			},
			totalItems: 25,
			expected: PaginatedResponse{
				Data: []string{"item1", "item2", "item3"},
			},
		},
		{
			name: "middle page",
			data: []string{"item11", "item12"},
			params: PaginationParams{
				Page:     3,
				PageSize: 5,
				Offset:   10,
			},
			totalItems: 17,
			expected: PaginatedResponse{
				Data: []string{"item11", "item12"},
			},
		},
		{
			name: "empty results",
			data: []string{},
			params: PaginationParams{
				Page:     1,
				PageSize: 10,
				Offset:   0,
			},
			totalItems: 0,
			expected: PaginatedResponse{
				Data: []string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			w := httptest.NewRecorder()

			writePaginatedResponse(w, req, tt.data, tt.params, tt.totalItems)

			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var response PaginatedResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			// Verify data
			expectedDataJSON, _ := json.Marshal(tt.data)
			actualDataJSON, _ := json.Marshal(response.Data)
			assert.JSONEq(t, string(expectedDataJSON), string(actualDataJSON))

			// Verify pagination
			assert.Equal(t, tt.params.Page, response.Pagination.Page)
			assert.Equal(t, tt.params.PageSize, response.Pagination.PageSize)
			assert.Equal(t, tt.totalItems, response.Pagination.TotalItems)

			expectedTotalPages := int((tt.totalItems + int64(tt.params.PageSize) - 1) / int64(tt.params.PageSize))
			assert.Equal(t, expectedTotalPages, response.Pagination.TotalPages)
		})
	}
}

func TestPaginationCalculations(t *testing.T) {
	tests := []struct {
		name           string
		page           int
		pageSize       int
		expectedOffset int
	}{
		{"first page", 1, 10, 0},
		{"second page", 2, 10, 10},
		{"third page", 3, 25, 50},
		{"large page", 100, 5, 495},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := PaginationParams{
				Offset: (tt.page - 1) * tt.pageSize,
			}

			assert.Equal(t, tt.expectedOffset, params.Offset)
		})
	}
}

// Benchmark tests for pagination performance.
func BenchmarkGetPaginationParams(b *testing.B) {
	req := httptest.NewRequest("GET", "/test?page=5&page_size=25", http.NoBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = getPaginationParams(req)
	}
}

func TestCommonHandlers_RealWorldScenarios_Pagination(t *testing.T) {
	t.Run("pagination with large dataset", func(t *testing.T) {
		// Test pagination calculations for large datasets
		totalItems := int64(10000)
		pageSize := 100

		for page := 1; page <= 10; page++ {
			params := PaginationParams{
				Page:     page,
				PageSize: pageSize,
				Offset:   (page - 1) * pageSize,
			}

			req := httptest.NewRequest("GET", "/test", http.NoBody)
			w := httptest.NewRecorder()

			writePaginatedResponse(w, req, []string{}, params, totalItems)

			assert.Equal(t, http.StatusOK, w.Code)

			var response PaginatedResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			expectedTotalPages := int((totalItems + int64(pageSize) - 1) / int64(pageSize))
			assert.Equal(t, expectedTotalPages, response.Pagination.TotalPages)
			assert.Equal(t, page, response.Pagination.Page)
			assert.Equal(t, pageSize, response.Pagination.PageSize)
			assert.Equal(t, totalItems, response.Pagination.TotalItems)
		}
	})
}
