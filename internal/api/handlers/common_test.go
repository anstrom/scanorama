package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	apierrors "github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/internal/metrics/mocks"
)

func createTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

func TestNewBaseHandler(t *testing.T) {
	tests := []struct {
		name    string
		logger  *slog.Logger
		metrics metrics.MetricsRegistry
	}{
		{
			name:    "with logger and metrics",
			logger:  createTestLogger(),
			metrics: metrics.NewRegistry(),
		},
		{
			name:    "with nil metrics",
			logger:  createTestLogger(),
			metrics: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewBaseHandler(tt.logger, tt.metrics)

			assert.NotNil(t, handler)
			assert.Equal(t, tt.logger, handler.logger)
			assert.Equal(t, tt.metrics, handler.metrics)
		})
	}
}

func TestGetRequestIDFromContext(t *testing.T) {
	tests := []struct {
		name       string
		setupCtx   func() context.Context
		expectedID string
	}{
		{
			name: "with request ID in context",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), ContextKey("request_id"), "test-req-123")
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
				return context.WithValue(context.Background(), ContextKey("request_id"), 12345)
			},
			expectedID: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			id := getRequestIDFromContext(ctx)
			assert.Equal(t, tt.expectedID, id)
		})
	}
}

func TestGetQueryParamInt(t *testing.T) {
	tests := []struct {
		name         string
		queryParams  map[string]string
		key          string
		defaultValue int
		expectedVal  int
		expectedErr  bool
	}{
		{
			name:         "valid integer parameter",
			queryParams:  map[string]string{"page": "5"},
			key:          "page",
			defaultValue: 1,
			expectedVal:  5,
			expectedErr:  false,
		},
		{
			name:         "missing parameter uses default",
			queryParams:  map[string]string{},
			key:          "page",
			defaultValue: 1,
			expectedVal:  1,
			expectedErr:  false,
		},
		{
			name:         "invalid integer parameter",
			queryParams:  map[string]string{"page": "invalid"},
			key:          "page",
			defaultValue: 1,
			expectedVal:  0,
			expectedErr:  true,
		},
		{
			name:         "empty parameter uses default",
			queryParams:  map[string]string{"page": ""},
			key:          "page",
			defaultValue: 10,
			expectedVal:  10,
			expectedErr:  false,
		},
		{
			name:         "negative number",
			queryParams:  map[string]string{"limit": "-5"},
			key:          "limit",
			defaultValue: 50,
			expectedVal:  -5,
			expectedErr:  false,
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

			val, err := getQueryParamInt(req, tt.key, tt.defaultValue)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedVal, val)
			}
		})
	}
}

func TestExtractUUIDFromPath(t *testing.T) {
	tests := []struct {
		name        string
		pathVars    map[string]string
		expectedID  uuid.UUID
		expectedErr bool
	}{
		{
			name:        "valid UUID",
			pathVars:    map[string]string{"id": "123e4567-e89b-12d3-a456-426614174000"},
			expectedID:  uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
			expectedErr: false,
		},
		{
			name:        "invalid UUID format",
			pathVars:    map[string]string{"id": "invalid-uuid"},
			expectedID:  uuid.Nil,
			expectedErr: true,
		},
		{
			name:        "missing ID parameter",
			pathVars:    map[string]string{},
			expectedID:  uuid.Nil,
			expectedErr: true,
		},
		{
			name:        "empty ID parameter",
			pathVars:    map[string]string{"id": ""},
			expectedID:  uuid.Nil,
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", http.NoBody)

			// Mock mux.Vars by creating a request with the expected path variables
			req = mux.SetURLVars(req, tt.pathVars)

			id, err := extractUUIDFromPath(req)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Equal(t, uuid.Nil, id)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedID, id)
			}
		})
	}
}

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

func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		data           interface{}
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "successful response",
			statusCode:     http.StatusOK,
			data:           map[string]string{"message": "success"},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"message":"success"}`,
		},
		{
			name:           "created response",
			statusCode:     http.StatusCreated,
			data:           map[string]interface{}{"id": 123, "name": "test"},
			expectedStatus: http.StatusCreated,
			expectedBody:   `{"id":123,"name":"test"}`,
		},
		{
			name:           "nil data",
			statusCode:     http.StatusNoContent,
			data:           nil,
			expectedStatus: http.StatusNoContent,
			expectedBody:   "null",
		},
		{
			name:       "complex object",
			statusCode: http.StatusOK,
			data: struct {
				ID        uuid.UUID `json:"id"`
				Name      string    `json:"name"`
				CreatedAt time.Time `json:"created_at"`
			}{
				ID:        uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
				Name:      "Test Object",
				CreatedAt: time.Date(2023, 12, 1, 10, 0, 0, 0, time.UTC),
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			ctx := context.WithValue(req.Context(), ContextKey("request_id"), "test-req-123")
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			writeJSON(w, req, tt.statusCode, tt.data)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			if tt.expectedBody != "" {
				assert.JSONEq(t, tt.expectedBody, w.Body.String())
			} else {
				// For complex objects, just verify it's valid JSON
				var result interface{}
				err := json.Unmarshal(w.Body.Bytes(), &result)
				assert.NoError(t, err)
			}
		})
	}
}

func TestWriteError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		err            error
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "bad request error",
			statusCode:     http.StatusBadRequest,
			err:            errors.New("invalid input"),
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Bad Request",
		},
		{
			name:           "not found error",
			statusCode:     http.StatusNotFound,
			err:            errors.New("resource not found"),
			expectedStatus: http.StatusNotFound,
			expectedError:  "Not Found",
		},
		{
			name:           "internal server error",
			statusCode:     http.StatusInternalServerError,
			err:            errors.New("database connection failed"),
			expectedStatus: http.StatusInternalServerError,
			expectedError:  "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			ctx := context.WithValue(req.Context(), ContextKey("request_id"), "test-req-456")
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			writeError(w, req, tt.statusCode, tt.err)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var response ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedError, response.Error)
			assert.Equal(t, tt.err.Error(), response.Message)
			assert.Equal(t, "test-req-456", response.RequestID)
			assert.NotZero(t, response.Timestamp)
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

func TestParseJSON(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		dest        interface{}
		expectedErr bool
		setup       func() interface{}
	}{
		{
			name: "valid JSON object",
			body: `{"name": "test", "value": 123}`,
			setup: func() interface{} {
				return &struct {
					Name  string `json:"name"`
					Value int    `json:"value"`
				}{}
			},
			expectedErr: false,
		},
		{
			name: "valid JSON array",
			body: `["item1", "item2", "item3"]`,
			setup: func() interface{} {
				return &[]string{}
			},
			expectedErr: false,
		},
		{
			name: "invalid JSON",
			body: `{"name": "test", "value":}`,
			setup: func() interface{} {
				return &map[string]interface{}{}
			},
			expectedErr: true,
		},
		{
			name: "empty body",
			body: "",
			setup: func() interface{} {
				return &map[string]interface{}{}
			},
			expectedErr: true,
		},
		{
			name: "unknown fields",
			body: `{"name": "test", "unknown": "field"}`,
			setup: func() interface{} {
				return &struct {
					Name string `json:"name"`
				}{}
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *strings.Reader
			if tt.body != "" {
				body = strings.NewReader(tt.body)
			}

			var req *http.Request
			if body != nil {
				req = httptest.NewRequest("POST", "/test", body)
			} else {
				req = httptest.NewRequest("POST", "/test", http.NoBody)
			}

			dest := tt.setup()
			err := parseJSON(req, dest)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHandleDatabaseError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		operation      string
		entityType     string
		expectedStatus int
		expectedMsg    string
	}{
		{
			name:           "not found error",
			err:            apierrors.ErrNotFound("user"),
			operation:      "get",
			entityType:     "user",
			expectedStatus: http.StatusNotFound,
			expectedMsg:    "user not found",
		},
		{
			name:           "conflict error",
			err:            apierrors.ErrConflict("profile"),
			operation:      "create",
			entityType:     "profile",
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "generic database error",
			err:            errors.New("connection timeout"),
			operation:      "update",
			entityType:     "scan",
			expectedStatus: http.StatusInternalServerError,
			expectedMsg:    "failed to update scan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			req := httptest.NewRequest("GET", "/test", http.NoBody)
			ctx := context.WithValue(req.Context(), ContextKey("request_id"), "test-req-789")
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			handleDatabaseError(w, req, tt.err, tt.operation, tt.entityType, logger)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

			var response ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.NotEmpty(t, response.Error)
			assert.NotEmpty(t, response.Message)
			assert.Equal(t, "test-req-789", response.RequestID)
			assert.NotZero(t, response.Timestamp)

			if tt.expectedMsg != "" {
				assert.Contains(t, response.Message, tt.expectedMsg)
			}
		})
	}
}

func TestRecordCRUDMetric(t *testing.T) {
	tests := []struct {
		name       string
		metricName string
		labels     map[string]string
		hasMetrics bool
	}{
		{
			name:       "with metrics registry",
			metricName: "scans_created_total",
			labels:     map[string]string{"status": "success"},
			hasMetrics: true,
		},
		{
			name:       "without metrics registry",
			metricName: "profiles_updated_total",
			labels:     map[string]string{"status": "failed"},
			hasMetrics: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var metricsRegistry metrics.MetricsRegistry

			if tt.hasMetrics {
				ctrl := gomock.NewController(t)
				defer ctrl.Finish()

				mockRegistry := mocks.NewMockMetricsRegistry(ctrl)
				mockRegistry.EXPECT().Counter(tt.metricName, gomock.Any()).AnyTimes()
				metricsRegistry = mockRegistry
			}

			assert.NotPanics(t, func() {
				recordCRUDMetric(metricsRegistry, tt.metricName, tt.labels)
			})

			// GoMock automatically verifies expectations when ctrl.Finish() is called
		})
	}
}

func TestListOperation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	metricsRegistry := mocks.NewMockMetricsRegistry(ctrl)
	metricsRegistry.EXPECT().Counter("test_entities_listed_total", gomock.Any()).AnyTimes()

	// Define test types
	type TestEntity struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}

	type TestFilter struct {
		Name string
	}

	// Create test entities
	testEntities := []TestEntity{
		{ID: uuid.New(), Name: "Entity 1"},
		{ID: uuid.New(), Name: "Entity 2"},
	}

	// Define operation
	operation := &ListOperation[TestEntity, TestFilter]{
		EntityType: "test_entity",
		MetricName: "test_entities_listed_total",
		Logger:     logger,
		Metrics:    metricsRegistry,
		GetFilters: func(r *http.Request) TestFilter {
			return TestFilter{Name: r.URL.Query().Get("name")}
		},
		ListFromDB: func(ctx context.Context, filters TestFilter, offset, limit int) ([]TestEntity, int64, error) {
			return testEntities, int64(len(testEntities)), nil
		},
		ToResponse: func(entity TestEntity) interface{} {
			return map[string]interface{}{
				"id":   entity.ID.String(),
				"name": entity.Name,
			}
		},
	}

	req := httptest.NewRequest("GET", "/test?page=1&page_size=10", http.NoBody)
	ctx := context.WithValue(req.Context(), ContextKey("request_id"), "test-req")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	operation.Execute(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response PaginatedResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Len(t, response.Data, 2)
	assert.Equal(t, int64(2), response.Pagination.TotalItems)

	// GoMock automatically verifies expectations when ctrl.Finish() is called
}

func TestCRUDOperation_ExecuteGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	metricsRegistry := mocks.NewMockMetricsRegistry(ctrl)
	metricsRegistry.EXPECT().Counter("test_entities_retrieved_total", gomock.Any()).AnyTimes()

	type TestEntity struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}

	testID := uuid.New()
	// Test entity for reference

	operation := &CRUDOperation[TestEntity]{
		EntityType: "test_entity",
		Logger:     logger,
		Metrics:    metricsRegistry,
	}

	getFromDB := func(ctx context.Context, id uuid.UUID) (*TestEntity, error) {
		if id == testID {
			return &TestEntity{ID: testID, Name: "Test Entity"}, nil
		}
		return nil, apierrors.ErrNotFound("test_entity")
	}

	toResponse := func(entity *TestEntity) interface{} {
		return map[string]interface{}{
			"id":   entity.ID.String(),
			"name": entity.Name,
		}
	}

	req := httptest.NewRequest("GET", "/test", http.NoBody)
	ctx := context.WithValue(req.Context(), ContextKey("request_id"), "test-req")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	operation.ExecuteGet(w, req, testID, getFromDB, toResponse, "test_entities_retrieved_total")

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, testID.String(), response["id"])
	assert.Equal(t, "Test Entity", response["name"])

	// GoMock automatically verifies expectations when ctrl.Finish() is called
}

func TestCRUDOperation_ExecuteDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	metricsRegistry := mocks.NewMockMetricsRegistry(ctrl)
	metricsRegistry.EXPECT().Counter("test_entities_deleted_total", gomock.Any()).AnyTimes()

	type TestEntity struct {
		ID uuid.UUID
	}

	testID := uuid.New()

	operation := &CRUDOperation[TestEntity]{
		EntityType: "test_entity",
		Logger:     logger,
		Metrics:    metricsRegistry,
	}

	deleteFromDB := func(ctx context.Context, id uuid.UUID) error {
		if id == testID {
			return nil
		}
		return apierrors.ErrNotFound("test_entity")
	}

	req := httptest.NewRequest("DELETE", "/test", http.NoBody)
	ctx := context.WithValue(req.Context(), ContextKey("request_id"), "test-req")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	operation.ExecuteDelete(w, req, testID, deleteFromDB, "test_entities_deleted_total")

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.String())

	// GoMock automatically verifies expectations when ctrl.Finish() is called
}

func TestJobControlOperation_ExecuteStart(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	metricsRegistry := mocks.NewMockMetricsRegistry(ctrl)
	metricsRegistry.EXPECT().Counter("test_jobs_started_total", gomock.Any()).AnyTimes()

	type TestJob struct {
		ID     uuid.UUID `json:"id"`
		Status string    `json:"status"`
	}

	testID := uuid.New()
	testJob := &TestJob{ID: testID, Status: "running"}

	operation := &JobControlOperation{
		EntityType: "test_job",
		Logger:     logger,
		Metrics:    metricsRegistry,
	}

	startInDB := func(ctx context.Context, id uuid.UUID) error {
		if id == testID {
			return nil
		}
		return apierrors.ErrNotFound("test_job")
	}

	getFromDB := func(ctx context.Context, id uuid.UUID) (interface{}, error) {
		if id == testID {
			return testJob, nil
		}
		return nil, apierrors.ErrNotFound("test_job")
	}

	toResponse := func(job interface{}) interface{} {
		if j, ok := job.(*TestJob); ok {
			return map[string]interface{}{
				"id":     j.ID.String(),
				"status": j.Status,
			}
		}
		return job
	}

	req := httptest.NewRequest("POST", "/test", http.NoBody)
	ctx := context.WithValue(req.Context(), ContextKey("request_id"), "test-req")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	operation.ExecuteStart(w, req, testID, startInDB, getFromDB, toResponse, "test_jobs_started_total")

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, testID.String(), response["id"])
	assert.Equal(t, "started", response["status"])
	assert.Contains(t, response["message"], "queued for execution")

	// GoMock automatically verifies expectations when ctrl.Finish() is called
}

func TestJobControlOperation_ExecuteStop(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	metricsRegistry := mocks.NewMockMetricsRegistry(ctrl)
	metricsRegistry.EXPECT().Counter("test_jobs_stopped_total", gomock.Any()).AnyTimes()

	testID := uuid.New()

	operation := &JobControlOperation{
		EntityType: "test_job",
		Logger:     logger,
		Metrics:    metricsRegistry,
	}

	stopInDB := func(ctx context.Context, id uuid.UUID) error {
		if id == testID {
			return nil
		}
		return apierrors.ErrNotFound("test_job")
	}

	req := httptest.NewRequest("POST", "/test", http.NoBody)
	ctx := context.WithValue(req.Context(), ContextKey("request_id"), "test-req")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	operation.ExecuteStop(w, req, testID, stopInDB, "test_jobs_stopped_total")

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, testID.String(), response["id"])
	assert.Equal(t, "stopped", response["status"])
	assert.Contains(t, response["message"], "has been stopped")

	// GoMock automatically verifies expectations when ctrl.Finish() is called
}

func TestCreateEntity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	metricsRegistry := mocks.NewMockMetricsRegistry(ctrl)
	metricsRegistry.EXPECT().Counter("test_entities_created_total", gomock.Any()).AnyTimes()

	type TestEntity struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}

	type TestRequest struct {
		Name string `json:"name"`
	}

	parseAndConvert := func(r *http.Request) (interface{}, error) {
		var req TestRequest
		if err := parseJSON(r, &req); err != nil {
			return nil, err
		}
		return req, nil
	}

	createInDB := func(ctx context.Context, data interface{}) (*TestEntity, error) {
		if req, ok := data.(TestRequest); ok {
			return &TestEntity{ID: uuid.New(), Name: req.Name}, nil
		}
		return nil, errors.New("invalid data type")
	}

	toResponse := func(entity *TestEntity) interface{} {
		return map[string]interface{}{
			"id":   entity.ID.String(),
			"name": entity.Name,
		}
	}

	requestBody := `{"name": "Test Entity"}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), ContextKey("request_id"), "test-req")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	CreateEntity[TestEntity, TestRequest](w, req, "test_entity", logger, metricsRegistry,
		parseAndConvert, createInDB, toResponse, "test_entities_created_total")

	assert.Equal(t, http.StatusCreated, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.NotEmpty(t, response["id"])
	assert.Equal(t, "Test Entity", response["name"])

	// GoMock automatically verifies expectations when ctrl.Finish() is called
}

func TestUpdateEntity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := createTestLogger()
	metricsRegistry := mocks.NewMockMetricsRegistry(ctrl)
	metricsRegistry.EXPECT().Counter("test_entities_updated_total", gomock.Any()).AnyTimes()

	type TestEntity struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}

	type TestRequest struct {
		Name string `json:"name"`
	}

	testID := uuid.New()

	parseAndConvert := func(r *http.Request) (interface{}, error) {
		var req TestRequest
		if err := parseJSON(r, &req); err != nil {
			return nil, err
		}
		return req, nil
	}

	updateInDB := func(ctx context.Context, id uuid.UUID, data interface{}) (*TestEntity, error) {
		if req, ok := data.(TestRequest); ok {
			return &TestEntity{ID: id, Name: req.Name}, nil
		}
		return nil, errors.New("invalid data type")
	}

	toResponse := func(entity *TestEntity) interface{} {
		return map[string]interface{}{
			"id":   entity.ID.String(),
			"name": entity.Name,
		}
	}

	requestBody := `{"name": "Updated Entity"}`
	req := httptest.NewRequest("PUT", "/test/"+testID.String(), strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": testID.String()})
	ctx := context.WithValue(req.Context(), ContextKey("request_id"), "test-req")
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	UpdateEntity[TestEntity, TestRequest](w, req, "test_entity", logger, metricsRegistry,
		parseAndConvert, updateInDB, toResponse, "test_entities_updated_total")

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, testID.String(), response["id"])
	assert.Equal(t, "Updated Entity", response["name"])

	// GoMock automatically verifies expectations when ctrl.Finish() is called
}

func TestCommonHandlers_ErrorHandling(t *testing.T) {
	t.Run("invalid UUID in path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test/invalid-uuid", http.NoBody)
		req = mux.SetURLVars(req, map[string]string{"id": "invalid-uuid"})

		_, err := extractUUIDFromPath(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid id")
	})

	t.Run("missing ID in path", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", http.NoBody)
		req = mux.SetURLVars(req, map[string]string{})

		_, err := extractUUIDFromPath(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "id not provided")
	})

	t.Run("invalid pagination parameters", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?page=invalid&page_size=also_invalid", http.NoBody)

		_, err := getPaginationParams(req)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid page parameter")
	})

	t.Run("nil request body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/test", http.NoBody)

		var dest map[string]interface{}
		err := parseJSON(req, &dest)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "EOF")
	})
}

func TestCommonHandlers_ContextKeys(t *testing.T) {
	t.Run("context key types", func(t *testing.T) {
		key := ContextKey("test")
		assert.IsType(t, ContextKey(""), key)
		assert.Equal(t, "test", string(key))
	})
}

func TestCommonHandlers_Integration(t *testing.T) {
	t.Run("full CRUD workflow", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		logger := createTestLogger()
		metricsRegistry := mocks.NewMockMetricsRegistry(ctrl)

		// Setup metrics expectations for all operations
		metricsRegistry.EXPECT().Counter(gomock.Any(), gomock.Any()).AnyTimes()

		type TestEntity struct {
			ID        uuid.UUID `json:"id"`
			Name      string    `json:"name"`
			CreatedAt time.Time `json:"created_at"`
		}

		// Simulate in-memory storage
		storage := make(map[uuid.UUID]*TestEntity)

		// Test Create
		createReq := `{"name": "Test Entity"}`
		req := httptest.NewRequest("POST", "/entities", strings.NewReader(createReq))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), ContextKey("request_id"), "create-req")
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()

		CreateEntity[TestEntity, map[string]string](w, req, "entity", logger, metricsRegistry,
			func(r *http.Request) (interface{}, error) {
				var data map[string]string
				if err := parseJSON(r, &data); err != nil {
					return nil, err
				}
				return data, nil
			},
			func(ctx context.Context, data interface{}) (*TestEntity, error) {
				if req, ok := data.(map[string]string); ok {
					entity := &TestEntity{
						ID:        uuid.New(),
						Name:      req["name"],
						CreatedAt: time.Now(),
					}
					storage[entity.ID] = entity
					return entity, nil
				}
				return nil, errors.New("invalid data")
			},
			func(entity *TestEntity) interface{} {
				return map[string]interface{}{
					"id":         entity.ID.String(),
					"name":       entity.Name,
					"created_at": entity.CreatedAt.Format(time.RFC3339),
				}
			},
			"entities_created_total")

		assert.Equal(t, http.StatusCreated, w.Code)

		var createResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &createResponse)
		require.NoError(t, err)

		createdID := createResponse["id"].(string)
		assert.NotEmpty(t, createdID)
		assert.Equal(t, "Test Entity", createResponse["name"])

		// GoMock automatically verifies expectations when ctrl.Finish() is called
	})
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

func TestErrorResponseStructure(t *testing.T) {
	t.Run("error response fields", func(t *testing.T) {
		response := ErrorResponse{
			Error:     "Bad Request",
			Message:   "Invalid input provided",
			Timestamp: time.Now().UTC(),
			RequestID: "test-req-123",
		}

		data, err := json.Marshal(response)
		require.NoError(t, err)

		var parsed map[string]interface{}
		err = json.Unmarshal(data, &parsed)
		require.NoError(t, err)

		assert.Equal(t, "Bad Request", parsed["error"])
		assert.Equal(t, "Invalid input provided", parsed["message"])
		assert.Equal(t, "test-req-123", parsed["request_id"])
		assert.NotNil(t, parsed["timestamp"])
	})
}

func TestCRUDMetrics(t *testing.T) {
	t.Run("CRUD metrics structure", func(t *testing.T) {
		testMetrics := CRUDMetrics{
			Listed:    "entities_listed_total",
			Created:   "entities_created_total",
			Retrieved: "entities_retrieved_total",
			Updated:   "entities_updated_total",
			Deleted:   "entities_deleted_total",
			Started:   "entities_started_total",
			Stopped:   "entities_stopped_total",
		}

		assert.NotEmpty(t, testMetrics.Listed)
		assert.NotEmpty(t, testMetrics.Created)
		assert.NotEmpty(t, testMetrics.Retrieved)
		assert.NotEmpty(t, testMetrics.Updated)
		assert.NotEmpty(t, testMetrics.Deleted)
		assert.NotEmpty(t, testMetrics.Started)
		assert.NotEmpty(t, testMetrics.Stopped)
	})
}

// Benchmark tests for performance.
func BenchmarkGetPaginationParams(b *testing.B) {
	req := httptest.NewRequest("GET", "/test?page=5&page_size=25", http.NoBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = getPaginationParams(req)
	}
}

func BenchmarkExtractUUIDFromPath(b *testing.B) {
	testID := uuid.New()
	req := httptest.NewRequest("GET", "/test/"+testID.String(), http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": testID.String()})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = extractUUIDFromPath(req)
	}
}

func BenchmarkWriteJSON(b *testing.B) {
	data := map[string]interface{}{
		"id":      uuid.New().String(),
		"name":    "Benchmark Test",
		"value":   12345,
		"active":  true,
		"created": time.Now(),
	}

	req := httptest.NewRequest("GET", "/test", http.NoBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		writeJSON(w, req, http.StatusOK, data)
	}
}

func TestCommonHandlers_RealWorldScenarios(t *testing.T) {
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

	t.Run("complex error handling workflow", func(t *testing.T) {
		logger := createTestLogger()

		errorTypes := []error{
			apierrors.ErrNotFound("entity"),
			apierrors.ErrConflict("entity"),
			errors.New("generic database error"),
		}

		expectedStatuses := []int{
			http.StatusNotFound,
			http.StatusConflict,
			http.StatusInternalServerError,
		}

		for i, err := range errorTypes {
			t.Run(fmt.Sprintf("error_%d", i), func(t *testing.T) {
				req := httptest.NewRequest("GET", "/test", http.NoBody)
				ctx := context.WithValue(req.Context(), ContextKey("request_id"), fmt.Sprintf("req-%d", i))
				req = req.WithContext(ctx)

				w := httptest.NewRecorder()

				handleDatabaseError(w, req, err, "test", "entity", logger)

				assert.Equal(t, expectedStatuses[i], w.Code)

				var response ErrorResponse
				jsonErr := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, jsonErr)

				assert.NotEmpty(t, response.Error)
				assert.NotEmpty(t, response.Message)
				assert.Equal(t, fmt.Sprintf("req-%d", i), response.RequestID)
			})
		}
	})
}

func TestCommonHandlers_EdgeCases(t *testing.T) {
	t.Run("zero total items pagination", func(t *testing.T) {
		params := PaginationParams{
			Page:     1,
			PageSize: 10,
			Offset:   0,
		}

		req := httptest.NewRequest("GET", "/test", http.NoBody)
		w := httptest.NewRecorder()

		writePaginatedResponse(w, req, []string{}, params, 0)

		var response PaginatedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, int64(0), response.Pagination.TotalItems)
		assert.Equal(t, 0, response.Pagination.TotalPages)
	})

	t.Run("single item pagination", func(t *testing.T) {
		params := PaginationParams{
			Page:     1,
			PageSize: 10,
			Offset:   0,
		}

		req := httptest.NewRequest("GET", "/test", http.NoBody)
		w := httptest.NewRecorder()

		writePaginatedResponse(w, req, []string{"item1"}, params, 1)

		var response PaginatedResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, int64(1), response.Pagination.TotalItems)
		assert.Equal(t, 1, response.Pagination.TotalPages)
	})

	t.Run("maximum page size limit", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test?page_size=5000", http.NoBody)

		params, err := getPaginationParams(req)
		require.NoError(t, err)

		assert.Equal(t, 1000, params.PageSize) // Should be capped at max
	})
}

// Helper function tests.
func TestHelperFunctions(t *testing.T) {
	t.Run("context key string conversion", func(t *testing.T) {
		key := ContextKey("test_key")
		assert.Equal(t, "test_key", string(key))
	})

	t.Run("pagination offset calculation", func(t *testing.T) {
		testCases := []struct {
			page     int
			pageSize int
			expected int
		}{
			{1, 10, 0},
			{2, 10, 10},
			{3, 20, 40},
			{10, 5, 45},
		}

		for _, tc := range testCases {
			offset := (tc.page - 1) * tc.pageSize
			assert.Equal(t, tc.expected, offset)
		}
	})
}

func TestCommonHandlers_TypeSafety(t *testing.T) {
	t.Run("pagination params type", func(t *testing.T) {
		params := PaginationParams{
			Page:     1,
			PageSize: 50,
			Offset:   0,
		}

		assert.IsType(t, int(0), params.Page)
		assert.IsType(t, int(0), params.PageSize)
		assert.IsType(t, int(0), params.Offset)
	})

	t.Run("error response type", func(t *testing.T) {
		response := ErrorResponse{
			Error:     "Test Error",
			Message:   "Test Message",
			Timestamp: time.Now(),
			RequestID: "test-req",
		}

		assert.IsType(t, string(""), response.Error)
		assert.IsType(t, string(""), response.Message)
		assert.IsType(t, time.Time{}, response.Timestamp)
		assert.IsType(t, string(""), response.RequestID)
	})
}
