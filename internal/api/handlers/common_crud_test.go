package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		ListFromDB: func(_ context.Context, _ TestFilter, offset, limit int) ([]TestEntity, int64, error) {
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

	getFromDB := func(_ context.Context, id uuid.UUID) (*TestEntity, error) {
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

	deleteFromDB := func(_ context.Context, id uuid.UUID) error {
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

	startInDB := func(_ context.Context, id uuid.UUID) error {
		if id == testID {
			return nil
		}
		return apierrors.ErrNotFound("test_job")
	}

	getFromDB := func(_ context.Context, id uuid.UUID) (interface{}, error) {
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

	stopInDB := func(_ context.Context, id uuid.UUID) error {
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

	createInDB := func(_ context.Context, data interface{}) (*TestEntity, error) {
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

	updateInDB := func(_ context.Context, id uuid.UUID, data interface{}) (*TestEntity, error) {
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
			func(_ context.Context, data interface{}) (*TestEntity, error) {
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

// Benchmark tests for CRUD-related operations.
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

func TestCommonHandlers_RealWorldScenarios_ErrorHandling(t *testing.T) {
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
