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

	"github.com/anstrom/scanorama/internal/db"
	apierrors "github.com/anstrom/scanorama/internal/errors"
	"github.com/anstrom/scanorama/internal/metrics"
)

func createTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
}

// nilScanServicer is a ScanServicer that panics if any method is called.
// Use it in tests that only exercise validation paths and never reach the DB.
type nilScanServicer struct{}

func (nilScanServicer) ListScans(_ context.Context, _ db.ScanFilters, _, _ int) ([]*db.Scan, int64, error) {
	panic("nilScanServicer: ListScans called unexpectedly")
}
func (nilScanServicer) CreateScan(_ context.Context, _ db.CreateScanInput) (*db.Scan, error) {
	panic("nilScanServicer: CreateScan called unexpectedly")
}
func (nilScanServicer) GetScan(_ context.Context, _ uuid.UUID) (*db.Scan, error) {
	panic("nilScanServicer: GetScan called unexpectedly")
}
func (nilScanServicer) UpdateScan(_ context.Context, _ uuid.UUID, _ db.UpdateScanInput) (*db.Scan, error) {
	panic("nilScanServicer: UpdateScan called unexpectedly")
}
func (nilScanServicer) DeleteScan(_ context.Context, _ uuid.UUID) error {
	panic("nilScanServicer: DeleteScan called unexpectedly")
}
func (nilScanServicer) StartScan(_ context.Context, _ uuid.UUID) (*db.Scan, error) {
	panic("nilScanServicer: StartScan called unexpectedly")
}
func (nilScanServicer) CompleteScan(_ context.Context, _ uuid.UUID) error {
	panic("nilScanServicer: CompleteScan called unexpectedly")
}
func (nilScanServicer) StopScan(_ context.Context, _ uuid.UUID, _ ...string) error {
	panic("nilScanServicer: StopScan called unexpectedly")
}
func (nilScanServicer) GetScanResults(_ context.Context, _ uuid.UUID, _, _ int) ([]*db.ScanResult, int64, error) {
	panic("nilScanServicer: GetScanResults called unexpectedly")
}
func (nilScanServicer) GetScanSummary(_ context.Context, _ uuid.UUID) (*db.ScanSummary, error) {
	panic("nilScanServicer: GetScanSummary called unexpectedly")
}
func (nilScanServicer) GetProfile(_ context.Context, _ string) (*db.ScanProfile, error) {
	panic("nilScanServicer: GetProfile called unexpectedly")
}

// nilScheduleServicer is a ScheduleServicer that panics if any method is called.
type nilScheduleServicer struct{}

func (nilScheduleServicer) ListSchedules(
	_ context.Context, _ db.ScheduleFilters, _, _ int,
) ([]*db.Schedule, int64, error) {
	panic("nilScheduleServicer: ListSchedules called unexpectedly")
}
func (nilScheduleServicer) CreateSchedule(_ context.Context, _ db.CreateScheduleInput) (*db.Schedule, error) {
	panic("nilScheduleServicer: CreateSchedule called unexpectedly")
}
func (nilScheduleServicer) GetSchedule(_ context.Context, _ uuid.UUID) (*db.Schedule, error) {
	panic("nilScheduleServicer: GetSchedule called unexpectedly")
}
func (nilScheduleServicer) UpdateSchedule(
	_ context.Context, _ uuid.UUID, _ db.UpdateScheduleInput,
) (*db.Schedule, error) {
	panic("nilScheduleServicer: UpdateSchedule called unexpectedly")
}
func (nilScheduleServicer) DeleteSchedule(_ context.Context, _ uuid.UUID) error {
	panic("nilScheduleServicer: DeleteSchedule called unexpectedly")
}
func (nilScheduleServicer) EnableSchedule(_ context.Context, _ uuid.UUID) error {
	panic("nilScheduleServicer: EnableSchedule called unexpectedly")
}
func (nilScheduleServicer) DisableSchedule(_ context.Context, _ uuid.UUID) error {
	panic("nilScheduleServicer: DisableSchedule called unexpectedly")
}
func (nilScheduleServicer) NextRun(_ context.Context, _ uuid.UUID) (time.Time, error) {
	panic("nilScheduleServicer: NextRun called unexpectedly")
}

// nilDiscoveryStore is a DiscoveryStore that panics if any method is called.
type nilDiscoveryStore struct{}

func (nilDiscoveryStore) ListDiscoveryJobs(
	_ context.Context, _ db.DiscoveryFilters, _, _ int,
) ([]*db.DiscoveryJob, int64, error) {
	panic("nilDiscoveryStore: ListDiscoveryJobs called unexpectedly")
}
func (nilDiscoveryStore) CreateDiscoveryJob(_ context.Context, _ db.CreateDiscoveryJobInput) (*db.DiscoveryJob, error) {
	panic("nilDiscoveryStore: CreateDiscoveryJob called unexpectedly")
}
func (nilDiscoveryStore) GetDiscoveryJob(_ context.Context, _ uuid.UUID) (*db.DiscoveryJob, error) {
	panic("nilDiscoveryStore: GetDiscoveryJob called unexpectedly")
}
func (nilDiscoveryStore) UpdateDiscoveryJob(
	_ context.Context, _ uuid.UUID, _ db.UpdateDiscoveryJobInput,
) (*db.DiscoveryJob, error) {
	panic("nilDiscoveryStore: UpdateDiscoveryJob called unexpectedly")
}
func (nilDiscoveryStore) DeleteDiscoveryJob(_ context.Context, _ uuid.UUID) error {
	panic("nilDiscoveryStore: DeleteDiscoveryJob called unexpectedly")
}
func (nilDiscoveryStore) StartDiscoveryJob(_ context.Context, _ uuid.UUID) error {
	panic("nilDiscoveryStore: StartDiscoveryJob called unexpectedly")
}
func (nilDiscoveryStore) StopDiscoveryJob(_ context.Context, _ uuid.UUID) error {
	panic("nilDiscoveryStore: StopDiscoveryJob called unexpectedly")
}
func (nilDiscoveryStore) ListDiscoveryJobsByNetwork(
	_ context.Context, _ uuid.UUID, _, _ int,
) ([]*db.DiscoveryJob, int64, error) {
	panic("nilDiscoveryStore: ListDiscoveryJobsByNetwork called unexpectedly")
}
func (nilDiscoveryStore) GetDiscoveryDiff(_ context.Context, _ uuid.UUID) (*db.DiscoveryDiff, error) {
	panic("nilDiscoveryStore: GetDiscoveryDiff called unexpectedly")
}
func (nilDiscoveryStore) CompareDiscoveryRuns(_ context.Context, _, _ uuid.UUID) (*db.DiscoveryCompareDiff, error) {
	panic("nilDiscoveryStore: CompareDiscoveryRuns called unexpectedly")
}

// nilHostServicer is a HostServicer that panics if any method is called.
type nilHostServicer struct{}

func (nilHostServicer) ListHosts(_ context.Context, _ *db.HostFilters, _, _ int) ([]*db.Host, int64, error) {
	panic("nilHostServicer: ListHosts called unexpectedly")
}
func (nilHostServicer) CreateHost(_ context.Context, _ db.CreateHostInput) (*db.Host, error) {
	panic("nilHostServicer: CreateHost called unexpectedly")
}
func (nilHostServicer) GetHost(_ context.Context, _ uuid.UUID) (*db.Host, error) {
	panic("nilHostServicer: GetHost called unexpectedly")
}
func (nilHostServicer) UpdateHost(_ context.Context, _ uuid.UUID, _ db.UpdateHostInput) (*db.Host, error) {
	panic("nilHostServicer: UpdateHost called unexpectedly")
}
func (nilHostServicer) UpdateCustomName(_ context.Context, _ uuid.UUID, _ *string) (*db.Host, error) {
	panic("nilHostServicer: UpdateCustomName called unexpectedly")
}
func (nilHostServicer) DeleteHost(_ context.Context, _ uuid.UUID) error {
	panic("nilHostServicer: DeleteHost called unexpectedly")
}
func (nilHostServicer) BulkDeleteHosts(_ context.Context, _ []uuid.UUID) (int64, error) {
	panic("nilHostServicer: BulkDeleteHosts called unexpectedly")
}
func (nilHostServicer) GetHostScans(_ context.Context, _ uuid.UUID, _, _ int) ([]*db.Scan, int64, error) {
	panic("nilHostServicer: GetHostScans called unexpectedly")
}
func (nilHostServicer) ListTags(_ context.Context) ([]string, error) {
	panic("nilHostServicer: ListTags called unexpectedly")
}
func (nilHostServicer) UpdateHostTags(_ context.Context, _ uuid.UUID, _ []string) error {
	panic("nilHostServicer: UpdateHostTags called unexpectedly")
}
func (nilHostServicer) AddHostTags(_ context.Context, _ uuid.UUID, _ []string) error {
	panic("nilHostServicer: AddHostTags called unexpectedly")
}
func (nilHostServicer) RemoveHostTags(_ context.Context, _ uuid.UUID, _ []string) error {
	panic("nilHostServicer: RemoveHostTags called unexpectedly")
}
func (nilHostServicer) BulkUpdateTags(_ context.Context, _ []uuid.UUID, _ []string, _ string) error {
	panic("nilHostServicer: BulkUpdateTags called unexpectedly")
}
func (nilHostServicer) GetHostGroups(_ context.Context, _ uuid.UUID) ([]db.HostGroupSummary, error) {
	panic("nilHostServicer: GetHostGroups called unexpectedly")
}
func (nilHostServicer) GetHostNetworks(_ context.Context, _ uuid.UUID) ([]*db.Network, error) {
	panic("nilHostServicer: GetHostNetworks called unexpectedly")
}

// nilProfileServicer is a ProfileServicer that panics if any method is called.
type nilProfileServicer struct{}

func (nilProfileServicer) ListProfiles(
	_ context.Context, _ db.ProfileFilters, _, _ int,
) ([]*db.ScanProfile, int64, error) {
	panic("nilProfileServicer: ListProfiles called unexpectedly")
}
func (nilProfileServicer) CreateProfile(_ context.Context, _ db.CreateProfileInput) (*db.ScanProfile, error) {
	panic("nilProfileServicer: CreateProfile called unexpectedly")
}
func (nilProfileServicer) GetProfile(_ context.Context, _ string) (*db.ScanProfile, error) {
	panic("nilProfileServicer: GetProfile called unexpectedly")
}
func (nilProfileServicer) UpdateProfile(_ context.Context, _ string, _ db.UpdateProfileInput) (*db.ScanProfile, error) {
	panic("nilProfileServicer: UpdateProfile called unexpectedly")
}
func (nilProfileServicer) DeleteProfile(_ context.Context, _ string) error {
	panic("nilProfileServicer: DeleteProfile called unexpectedly")
}
func (nilProfileServicer) CloneProfile(_ context.Context, _, _ string) (*db.ScanProfile, error) {
	panic("nilProfileServicer: CloneProfile called unexpectedly")
}
func (nilProfileServicer) GetProfileStats(_ context.Context, _ string) (*db.ProfileStats, error) {
	panic("nilProfileServicer: GetProfileStats called unexpectedly")
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

		assert.Equal(t, 100, params.PageSize) // Should be capped at max
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
