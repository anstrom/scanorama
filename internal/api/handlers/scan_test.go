package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

func TestNewScanHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	testMetrics := metrics.NewRegistry()

	tests := []struct {
		name     string
		database *db.DB
		metrics  *metrics.Registry
	}{
		{
			name:     "with database and metrics",
			database: &db.DB{},
			metrics:  testMetrics,
		},
		{
			name:     "with nil database",
			database: nil,
			metrics:  testMetrics,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			handler := NewScanHandler(tt.database, logger, tt.metrics)

			assert.NotNil(t, handler)
			assert.NotNil(t, handler.logger)
			assert.Equal(t, tt.metrics, handler.metrics)
		})
	}
}

func TestScanHandler_ValidateScanRequest(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		request     *ScanRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request",
			request: &ScanRequest{
				Name:     "Valid Scan",
				Targets:  []string{"192.168.1.1"},
				ScanType: "connect",
			},
			expectError: false,
		},
		{
			name: "empty name",
			request: &ScanRequest{
				Name:     "",
				Targets:  []string{"192.168.1.1"},
				ScanType: "connect",
			},
			expectError: true,
			errorMsg:    "scan name is required",
		},
		{
			name: "name too long",
			request: &ScanRequest{
				Name:     strings.Repeat("a", 256),
				Targets:  []string{"192.168.1.1"},
				ScanType: "connect",
			},
			expectError: true,
			errorMsg:    "scan name too long",
		},
		{
			name: "no targets",
			request: &ScanRequest{
				Name:     "Test Scan",
				Targets:  []string{},
				ScanType: "connect",
			},
			expectError: true,
			errorMsg:    "at least one target is required",
		},
		{
			name: "invalid scan type",
			request: &ScanRequest{
				Name:     "Test Scan",
				Targets:  []string{"192.168.1.1"},
				ScanType: "invalid",
			},
			expectError: true,
			errorMsg:    "invalid scan type",
		},
		{
			name: "empty target",
			request: &ScanRequest{
				Name:     "Test Scan",
				Targets:  []string{"192.168.1.1", ""},
				ScanType: "connect",
			},
			expectError: true,
			errorMsg:    "target 2 is empty",
		},
		{
			name: "target too long",
			request: &ScanRequest{
				Name:     "Test Scan",
				Targets:  []string{strings.Repeat("a", 256)},
				ScanType: "connect",
			},
			expectError: true,
			errorMsg:    "target 1 too long",
		},
		{
			name: "valid aggressive scan type",
			request: &ScanRequest{
				Name:     "Aggressive Scan",
				Targets:  []string{"192.168.1.1"},
				ScanType: "aggressive",
			},
			expectError: false,
		},
		{
			name: "valid comprehensive scan type",
			request: &ScanRequest{
				Name:     "Comprehensive Scan",
				Targets:  []string{"192.168.1.1"},
				ScanType: "comprehensive",
			},
			expectError: false,
		},
		{
			name: "valid syn scan type",
			request: &ScanRequest{
				Name:     "SYN Scan",
				Targets:  []string{"192.168.1.1"},
				ScanType: "syn",
			},
			expectError: false,
		},
		{
			name: "valid ack scan type",
			request: &ScanRequest{
				Name:     "ACK Scan",
				Targets:  []string{"192.168.1.1"},
				ScanType: "ack",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateScanRequest(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestScanHandler_GetScanFilters(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name           string
		queryParams    string
		expectedFilter db.ScanFilters
	}{
		{
			name:           "no filters",
			queryParams:    "",
			expectedFilter: db.ScanFilters{},
		},
		{
			name:        "status filter",
			queryParams: "?status=running",
			expectedFilter: db.ScanFilters{
				Status: "running",
			},
		},
		{
			name:        "scan type filter",
			queryParams: "?scan_type=syn",
			expectedFilter: db.ScanFilters{
				ScanType: "syn",
			},
		},
		{
			name:        "tag filter",
			queryParams: "?tag=production",
			expectedFilter: db.ScanFilters{
				Tags: []string{"production"},
			},
		},
		{
			name:        "profile ID filter",
			queryParams: "?profile_id=123",
			expectedFilter: db.ScanFilters{
				ProfileID: func() *int64 { id := int64(123); return &id }(),
			},
		},
		{
			name:        "multiple filters",
			queryParams: "?status=running&scan_type=syn&tag=test",
			expectedFilter: db.ScanFilters{
				Status:   "running",
				ScanType: "syn",
				Tags:     []string{"test"},
			},
		},
		{
			name:           "invalid profile ID",
			queryParams:    "?profile_id=invalid",
			expectedFilter: db.ScanFilters{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/scans"+tt.queryParams, http.NoBody)
			filters := handler.getScanFilters(req)

			assert.Equal(t, tt.expectedFilter.Status, filters.Status)
			assert.Equal(t, tt.expectedFilter.ScanType, filters.ScanType)
			assert.Equal(t, tt.expectedFilter.Tags, filters.Tags)

			if tt.expectedFilter.ProfileID != nil {
				require.NotNil(t, filters.ProfileID)
				assert.Equal(t, *tt.expectedFilter.ProfileID, *filters.ProfileID)
			} else {
				assert.Nil(t, filters.ProfileID)
			}
		})
	}
}

func TestScanHandler_RequestToDBScan(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	request := &ScanRequest{
		Name:        "Test Scan",
		Description: "Test Description",
		Targets:     []string{"192.168.1.0/24"},
		ScanType:    "connect",
		Ports:       "1-1000",
		ProfileID:   func() *int64 { id := int64(123); return &id }(),
		Options:     map[string]string{"timeout": "30"},
		ScheduleID:  func() *int64 { id := int64(456); return &id }(),
		Tags:        []string{"test", "api"},
	}

	result := handler.requestToDBScan(request)
	data, ok := result.(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, request.Name, data["name"])
	assert.Equal(t, request.Description, data["description"])
	assert.Equal(t, request.Targets, data["targets"])
	assert.Equal(t, request.ScanType, data["scan_type"])
	assert.Equal(t, request.Ports, data["ports"])
	assert.Equal(t, request.ProfileID, data["profile_id"])
	assert.Equal(t, request.Options, data["options"])
	assert.Equal(t, request.ScheduleID, data["schedule_id"])
	assert.Equal(t, request.Tags, data["tags"])
	assert.Equal(t, "pending", data["status"])
	assert.Contains(t, data, "created_at")
}

func TestScanHandler_ScanToResponse(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	testScanID := uuid.New()
	testScan := &db.Scan{
		ID:          testScanID,
		Name:        "Test Scan",
		Description: "Test Description",
		ScanType:    "connect",
		Status:      "running",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	response := handler.scanToResponse(testScan)

	assert.Equal(t, testScan.ID, response.ID)
	assert.Equal(t, testScan.Name, response.Name)
	assert.Equal(t, testScan.Description, response.Description)
	assert.Equal(t, testScan.ScanType, response.ScanType)
	assert.Equal(t, testScan.Status, response.Status)
	assert.Equal(t, testScan.CreatedAt, response.CreatedAt)
	assert.Equal(t, testScan.UpdatedAt, response.UpdatedAt)
	assert.Equal(t, 0.0, response.Progress)
}

func TestScanHandler_ResultToResponse(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	testResultID := uuid.New()
	testScanID := uuid.New()
	testHostID := uuid.New()
	testResult := &db.ScanResult{
		ID:       testResultID,
		ScanID:   testScanID,
		HostID:   testHostID,
		Port:     80,
		Protocol: "tcp",
		State:    "open",
		Service:  "http",
	}

	response := handler.resultToResponse(testResult)

	assert.Equal(t, testResult.ID, response.ID)
	assert.Equal(t, testResult.Port, response.Port)
	assert.Equal(t, testResult.Protocol, response.Protocol)
	assert.Equal(t, testResult.State, response.State)
	assert.Equal(t, testResult.Service, response.Service)
	assert.NotZero(t, response.ScanTime)
}

func TestScanHandler_CreateScan_ValidationErrors(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		requestBody interface{}
	}{
		{
			name: "validation error - empty name",
			requestBody: ScanRequest{
				Name:     "",
				Targets:  []string{"192.168.1.1"},
				ScanType: "connect",
			},
		},
		{
			name: "validation error - no targets",
			requestBody: ScanRequest{
				Name:     "Test Scan",
				Targets:  []string{},
				ScanType: "connect",
			},
		},
		{
			name: "validation error - invalid scan type",
			requestBody: ScanRequest{
				Name:     "Test Scan",
				Targets:  []string{"192.168.1.1"},
				ScanType: "invalid",
			},
		},
		{
			name:        "invalid JSON",
			requestBody: "invalid json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				require.NoError(t, err)
			}

			req := httptest.NewRequest("POST", "/api/v1/scans", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreateScan(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestScanHandler_GetScan_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/scans/invalid-uuid", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.GetScan(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScanHandler_StartScan_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("POST", "/api/v1/scans/invalid-uuid/start", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.StartScan(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScanHandler_StopScan_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("POST", "/api/v1/scans/invalid-uuid/stop", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.StopScan(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScanHandler_DeleteScan_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("DELETE", "/api/v1/scans/invalid-uuid", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.DeleteScan(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScanHandler_UpdateScan_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	updateRequest := ScanRequest{
		Name:     "Updated Scan",
		Targets:  []string{"192.168.1.0/24"},
		ScanType: "syn",
	}
	body, _ := json.Marshal(updateRequest)

	req := httptest.NewRequest("PUT", "/api/v1/scans/invalid-uuid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.UpdateScan(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScanHandler_GetScanResults_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/scans/invalid-uuid/results", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.GetScanResults(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScanHandler_EdgeCases(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	t.Run("scan types validation", func(t *testing.T) {
		validTypes := []string{"connect", "syn", "ack", "aggressive", "comprehensive"}
		for _, scanType := range validTypes {
			req := &ScanRequest{
				Name:     "Test",
				Targets:  []string{"192.168.1.1"},
				ScanType: scanType,
			}
			err := handler.validateScanRequest(req)
			assert.NoError(t, err, "scan type %s should be valid", scanType)
		}
	})

	t.Run("multiple targets validation", func(t *testing.T) {
		req := &ScanRequest{
			Name:     "Test",
			Targets:  []string{"192.168.1.1", "192.168.1.2", "example.com"},
			ScanType: "connect",
		}
		err := handler.validateScanRequest(req)
		assert.NoError(t, err)
	})
}

func BenchmarkScanHandler_ValidateScanRequest(b *testing.B) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	request := &ScanRequest{
		Name:     "Benchmark Scan",
		Targets:  []string{"192.168.1.0/24", "10.0.0.0/8"},
		ScanType: "connect",
		Ports:    "1-65535",
		Tags:     []string{"benchmark", "test"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handler.validateScanRequest(request)
	}
}

func BenchmarkScanHandler_GetScanFilters(b *testing.B) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/scans?status=running&scan_type=syn&tag=test&profile_id=123", http.NoBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handler.getScanFilters(req)
	}
}

func TestScanHandler_RequestValidation_Comprehensive(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nil, logger, metrics.NewRegistry())

	t.Run("maximum valid scan request", func(t *testing.T) {
		req := &ScanRequest{
			Name:        strings.Repeat("a", 255), // max length
			Description: "Valid description",
			Targets:     make([]string, 10), // multiple targets
			ScanType:    "comprehensive",
			Ports:       "1-65535",
			ProfileID:   func() *int64 { id := int64(1); return &id }(),
			Options:     map[string]string{"key": "value"},
			ScheduleID:  func() *int64 { id := int64(1); return &id }(),
			Tags:        []string{"tag1", "tag2"},
		}

		// Fill targets with valid values
		for i := range req.Targets {
			req.Targets[i] = "192.168.1." + string(rune('1'+i))
		}

		err := handler.validateScanRequest(req)
		assert.NoError(t, err)
	})

	t.Run("boundary conditions", func(t *testing.T) {
		// Test exactly at the boundary
		req := &ScanRequest{
			Name:     strings.Repeat("a", 255),           // exactly max length
			Targets:  []string{strings.Repeat("b", 255)}, // exactly max target length
			ScanType: "connect",
		}
		err := handler.validateScanRequest(req)
		assert.NoError(t, err)

		// Test just over the boundary
		req.Name = strings.Repeat("a", 256) // one over max length
		err = handler.validateScanRequest(req)
		assert.Error(t, err)
	})
}
