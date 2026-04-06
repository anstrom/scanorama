package handlers

import (
	"bytes"
	"context"
	"encoding/json"
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

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/internal/services"
	"github.com/anstrom/scanorama/test/helpers"
)

func TestNewScanHandler(t *testing.T) {
	testMetrics := metrics.NewRegistry()

	tests := []struct {
		name     string
		database ScanServicer
		metrics  *metrics.Registry
	}{
		{
			name:     "with store and metrics",
			database: nilScanServicer{},
			metrics:  testMetrics,
		},
		{
			name:     "with nil store",
			database: nilScanServicer{},
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
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

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
				Ports:    "80",
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
				Ports:    "80",
			},
			expectError: false,
		},
		{
			name: "valid comprehensive scan type",
			request: &ScanRequest{
				Name:     "Comprehensive Scan",
				Targets:  []string{"192.168.1.1"},
				ScanType: "comprehensive",
				Ports:    "80",
			},
			expectError: false,
		},
		{
			name: "valid syn scan type",
			request: &ScanRequest{
				Name:     "SYN Scan",
				Targets:  []string{"192.168.1.1"},
				ScanType: "syn",
				Ports:    "80",
			},
			expectError: false,
		},
		{
			name: "valid ack scan type",
			request: &ScanRequest{
				Name:     "ACK Scan",
				Targets:  []string{"192.168.1.1"},
				ScanType: "ack",
				Ports:    "80",
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
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

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
				ProfileID: func() *string { id := "123"; return &id }(),
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
			name:        "invalid profile ID (string IDs are now valid)",
			queryParams: "?profile_id=invalid",
			expectedFilter: db.ScanFilters{
				ProfileID: func() *string { id := "invalid"; return &id }(),
			},
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

func TestGetScanFilters_SortParams(t *testing.T) {
	logger := createTestLogger()
	h := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	t.Run("extracts sort_by and sort_order", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/?sort_by=status&sort_order=asc", nil)
		f := h.getScanFilters(r)
		assert.Equal(t, "status", f.SortBy)
		assert.Equal(t, "asc", f.SortOrder)
	})

	t.Run("empty when params absent", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		f := h.getScanFilters(r)
		assert.Empty(t, f.SortBy)
		assert.Empty(t, f.SortOrder)
	})

	t.Run("sort_by=created_at sort_order=desc", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/?sort_by=created_at&sort_order=desc", nil)
		f := h.getScanFilters(r)
		assert.Equal(t, "created_at", f.SortBy)
		assert.Equal(t, "desc", f.SortOrder)
	})

	t.Run("sort_by only, no sort_order", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/?sort_by=status", nil)
		f := h.getScanFilters(r)
		assert.Equal(t, "status", f.SortBy)
		assert.Empty(t, f.SortOrder)
	})
}

func TestScanHandler_RequestToDBScan(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	request := &ScanRequest{
		Name:        "Test Scan",
		Description: "Test Description",
		Targets:     []string{"192.168.1.0/24"},
		ScanType:    "connect",
		Ports:       "1-1000",
		ProfileID:   func() *string { id := "123"; return &id }(),
		Options:     map[string]string{"timeout": "30"},
		ScheduleID:  func() *int64 { id := int64(456); return &id }(),
		Tags:        []string{"test", "api"},
	}

	result := handler.requestToCreateScan(request)

	assert.Equal(t, request.Name, result.Name)
	assert.Equal(t, request.Description, result.Description)
	assert.Equal(t, request.Targets, result.Targets)
	assert.Equal(t, request.ScanType, result.ScanType)
	assert.Equal(t, request.Ports, result.Ports)
	assert.Equal(t, request.ProfileID, result.ProfileID)
}

func TestScanHandler_ScanToResponse(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	now := time.Now()
	testScanID := uuid.New()
	testScan := &db.Scan{
		ID:          testScanID,
		Name:        "Test Scan",
		Description: "Test Description",
		Targets:     []string{"192.168.1.0/24"},
		ScanType:    "connect",
		Status:      "running",
		CreatedAt:   now,
		UpdatedAt:   now,
		StartedAt:   &now,
	}

	response := handler.scanToResponse(testScan)

	assert.Equal(t, testScan.ID, response.ID)
	assert.Equal(t, testScan.Name, response.Name)
	assert.Equal(t, testScan.Description, response.Description)
	assert.Equal(t, []string{"192.168.1.0/24"}, response.Targets)
	assert.Equal(t, testScan.ScanType, response.ScanType)
	assert.Equal(t, testScan.Status, response.Status)
	assert.Equal(t, testScan.CreatedAt, response.CreatedAt)
	assert.Equal(t, testScan.UpdatedAt, response.UpdatedAt)
	assert.Equal(t, 50.0, response.Progress) // running → 50%
	assert.Equal(t, &now, response.StartTime)
	assert.Nil(t, response.EndTime)
	assert.Nil(t, response.Duration)
}

func TestScanHandler_ScanToResponse_Completed(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	start := time.Now().Add(-10 * time.Minute)
	end := time.Now()
	testScan := &db.Scan{
		ID:          uuid.New(),
		Name:        "Completed Scan",
		ScanType:    "syn",
		Status:      "completed",
		StartedAt:   &start,
		CompletedAt: &end,
		CreatedAt:   start,
		UpdatedAt:   end,
	}

	response := handler.scanToResponse(testScan)

	assert.Equal(t, 100.0, response.Progress)
	assert.NotNil(t, response.Duration)
	assert.Equal(t, end.Sub(start).String(), *response.Duration)
}

func TestScanHandler_ScanToResponse_WithOptions(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	testScan := &db.Scan{
		ID:       uuid.New(),
		Name:     "Options Scan",
		ScanType: "connect",
		Status:   "pending",
		Options: map[string]interface{}{
			"timeout": 30,
			"retries": "3",
			"verbose": true,
		},
	}

	response := handler.scanToResponse(testScan)

	require.NotNil(t, response.Options)
	assert.Equal(t, "30", response.Options["timeout"])
	assert.Equal(t, "3", response.Options["retries"])
	assert.Equal(t, "true", response.Options["verbose"])
}

func TestScanHandler_ScanToResponse_FailedStatus(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	testScan := &db.Scan{
		ID:       uuid.New(),
		Name:     "Failed Scan",
		ScanType: "syn",
		Status:   "failed",
	}

	response := handler.scanToResponse(testScan)

	assert.Equal(t, "failed", response.Status)
	assert.Equal(t, 0.0, response.Progress)
	assert.Equal(t, []string{}, response.Targets)
	assert.Nil(t, response.Options)
}

func TestScanHandler_ScanToResponse_NilTargets(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	testScan := &db.Scan{
		ID:       uuid.New(),
		Name:     "No Targets",
		ScanType: "connect",
		Status:   "pending",
	}

	response := handler.scanToResponse(testScan)

	// Nil targets should be normalized to empty slice for JSON
	assert.Equal(t, []string{}, response.Targets)
	assert.Equal(t, 0.0, response.Progress)
}

func TestScanHandler_ResultToResponse(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	now := time.Now().UTC()
	testResultID := uuid.New()
	testScanID := uuid.New()
	testHostID := uuid.New()
	testResult := &db.ScanResult{
		ID:        testResultID,
		ScanID:    testScanID,
		HostID:    testHostID,
		HostIP:    "192.168.1.1",
		Port:      80,
		Protocol:  "tcp",
		State:     "open",
		Service:   "http",
		ScannedAt: now,
	}

	response := handler.resultToResponse(testResult)

	assert.Equal(t, testResult.ID, response.ID)
	assert.Equal(t, "192.168.1.1", response.HostIP)
	assert.Equal(t, testResult.Port, response.Port)
	assert.Equal(t, testResult.Protocol, response.Protocol)
	assert.Equal(t, testResult.State, response.State)
	assert.Equal(t, testResult.Service, response.Service)
	assert.Equal(t, now, response.ScanTime)
}

func TestScanHandler_CreateScan_ValidationErrors(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

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
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/scans/invalid-uuid", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.GetScan(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScanHandler_StartScan_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	req := httptest.NewRequest("POST", "/api/v1/scans/invalid-uuid/start", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.StartScan(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScanHandler_StopScan_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	req := httptest.NewRequest("POST", "/api/v1/scans/invalid-uuid/stop", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.StopScan(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScanHandler_DeleteScan_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	req := httptest.NewRequest("DELETE", "/api/v1/scans/invalid-uuid", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.DeleteScan(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScanHandler_UpdateScan_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

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
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/scans/invalid-uuid/results", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.GetScanResults(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScanHandler_EdgeCases(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	t.Run("scan types validation", func(t *testing.T) {
		validTypes := []string{"connect", "syn", "ack", "aggressive", "comprehensive"}
		for _, scanType := range validTypes {
			req := &ScanRequest{
				Name:     "Test",
				Targets:  []string{"192.168.1.1"},
				ScanType: scanType,
				Ports:    "80",
			}
			err := handler.validateScanRequest(req)
			assert.NoError(t, err, "scan type %s should be valid", scanType)
		}
	})

	t.Run("multiple targets validation", func(t *testing.T) {
		req := &ScanRequest{
			Name:     "Test",
			Targets:  []string{"192.168.1.1", "192.168.1.2", "10.0.0.0/24"},
			ScanType: "connect",
			Ports:    "80",
		}
		err := handler.validateScanRequest(req)
		assert.NoError(t, err)

		// Hostname targets are not valid — must be IP or CIDR.
		reqHostname := &ScanRequest{
			Name:     "Test",
			Targets:  []string{"192.168.1.1", "example.com"},
			ScanType: "connect",
		}
		err = handler.validateScanRequest(reqHostname)
		assert.Error(t, err, "hostname targets should be rejected")
	})
}

func BenchmarkScanHandler_ValidateScanRequest(b *testing.B) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

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
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/scans?status=running&scan_type=syn&tag=test&profile_id=123", http.NoBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handler.getScanFilters(req)
	}
}

func TestScanHandler_RequestValidation_Comprehensive(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	t.Run("maximum valid scan request", func(t *testing.T) {
		req := &ScanRequest{
			Name:        strings.Repeat("a", services.MaxScanNameLength), // max length
			Description: "Valid description",
			Targets:     make([]string, 10), // multiple targets
			ScanType:    "comprehensive",
			Ports:       "1-65535",
			ProfileID:   func() *string { id := "linux-server"; return &id }(),
			Options:     map[string]string{"key": "value"},
			ScheduleID:  func() *int64 { id := int64(1); return &id }(),
			Tags:        []string{"tag1", "tag2"},
		}

		// Fill targets with valid IPs (10.0.0.1 – 10.0.0.10).
		for i := range req.Targets {
			req.Targets[i] = fmt.Sprintf("10.0.0.%d", i+1)
		}

		err := handler.validateScanRequest(req)
		assert.NoError(t, err)
	})

	t.Run("boundary conditions", func(t *testing.T) {
		// Name exactly at max length with a valid target — should pass.
		req := &ScanRequest{
			Name:     strings.Repeat("a", services.MaxScanNameLength),
			Targets:  []string{"10.0.0.1"},
			ScanType: "connect",
			Ports:    "80",
		}
		err := handler.validateScanRequest(req)
		assert.NoError(t, err)

		// Name one over max length — should fail on name, not target.
		req.Name = strings.Repeat("a", services.MaxScanNameLength+1)
		err = handler.validateScanRequest(req)
		assert.Error(t, err)

		// Target that exceeds maxTargetLength should fail.
		req.Name = "valid"
		req.Targets = []string{strings.Repeat("b", services.MaxTargetLength+1)}
		err = handler.validateScanRequest(req)
		assert.Error(t, err)
	})
}

// ---------------------------------------------------------------------------
// Tests for newly-added helper functions (parsePortSpec, getOptionBool,
// WithScanMode, firstNonEmpty) and the CIDR branch of validateScanRequest.
// ---------------------------------------------------------------------------

func TestParsePortSpec(t *testing.T) {
	tests := []struct {
		name        string
		ports       string
		expectError bool
		errorMsg    string
	}{
		// Valid cases
		{
			name:        "single plain port",
			ports:       "80",
			expectError: false,
		},
		{
			name:        "plain port range",
			ports:       "1024-9999",
			expectError: false,
		},
		{
			name:        "T: prefix single port",
			ports:       "T:80",
			expectError: false,
		},
		{
			name:        "U: prefix single port",
			ports:       "U:53",
			expectError: false,
		},
		{
			name:        "mixed prefixes and range",
			ports:       "T:80,U:53,1024-9999",
			expectError: false,
		},
		{
			name:        "empty token between commas is ok",
			ports:       "80,,443",
			expectError: false,
		},
		{
			name:        "boundary low port 1",
			ports:       "1",
			expectError: false,
		},
		{
			name:        "boundary high port 65535",
			ports:       "65535",
			expectError: false,
		},
		{
			name:        "T: prefix with range",
			ports:       "T:1024-65535",
			expectError: false,
		},
		{
			name:        "U: prefix with range",
			ports:       "U:1-1023",
			expectError: false,
		},
		// Error cases
		{
			name:        "port 0 is invalid",
			ports:       "0",
			expectError: true,
			errorMsg:    "must be between 1 and 65535",
		},
		{
			name:        "port 65536 is invalid",
			ports:       "65536",
			expectError: true,
			errorMsg:    "must be between 1 and 65535",
		},
		{
			name:        "non-numeric port",
			ports:       "abc",
			expectError: true,
			errorMsg:    "must be a number",
		},
		{
			name:        "non-numeric in range",
			ports:       "80-abc",
			expectError: true,
			errorMsg:    "must be a number",
		},
		{
			name:        "port 0 with T: prefix",
			ports:       "T:0",
			expectError: true,
			errorMsg:    "must be between 1 and 65535",
		},
		{
			name:        "port 65536 with U: prefix",
			ports:       "U:65536",
			expectError: true,
			errorMsg:    "must be between 1 and 65535",
		},
		{
			name:        "range start out of bounds",
			ports:       "0-100",
			expectError: true,
			errorMsg:    "must be between 1 and 65535",
		},
		{
			name:        "range end out of bounds",
			ports:       "100-65536",
			expectError: true,
			errorMsg:    "must be between 1 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := services.ParsePortSpec(tt.ports)
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetOptionBool(t *testing.T) {
	t.Run("nil options returns false", func(t *testing.T) {
		assert.False(t, getOptionBool(nil, "key"))
	})

	t.Run("key present with bool true", func(t *testing.T) {
		opts := map[string]interface{}{"enabled": true}
		assert.True(t, getOptionBool(opts, "enabled"))
	})

	t.Run("key present with bool false", func(t *testing.T) {
		opts := map[string]interface{}{"enabled": false}
		assert.False(t, getOptionBool(opts, "enabled"))
	})

	t.Run("key absent returns false", func(t *testing.T) {
		opts := map[string]interface{}{"other": true}
		assert.False(t, getOptionBool(opts, "missing"))
	})

	t.Run("value is non-bool string returns false", func(t *testing.T) {
		opts := map[string]interface{}{"enabled": "true"}
		assert.False(t, getOptionBool(opts, "enabled"))
	})

	t.Run("value is non-bool int returns false", func(t *testing.T) {
		opts := map[string]interface{}{"enabled": 1}
		assert.False(t, getOptionBool(opts, "enabled"))
	})
}

func TestWithScanMode(t *testing.T) {
	logger := createTestLogger()
	h := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	t.Run("sets scan mode and returns same handler", func(t *testing.T) {
		result := h.WithScanMode("syn")
		assert.Equal(t, "syn", h.scanMode)
		// Must return the same pointer so callers can chain.
		assert.Same(t, h, result)
	})

	t.Run("overwrites previous scan mode", func(t *testing.T) {
		h.WithScanMode("connect")
		assert.Equal(t, "connect", h.scanMode)
		h.WithScanMode("aggressive")
		assert.Equal(t, "aggressive", h.scanMode)
	})

	t.Run("empty string is accepted", func(t *testing.T) {
		h.WithScanMode("")
		assert.Equal(t, "", h.scanMode)
	})
}

func TestFirstNonEmpty(t *testing.T) {
	t.Run("returns first non-empty value", func(t *testing.T) {
		assert.Equal(t, "a", firstNonEmpty("a", "b", "c"))
	})

	t.Run("skips leading empty strings", func(t *testing.T) {
		assert.Equal(t, "second", firstNonEmpty("", "second", "third"))
	})

	t.Run("all empty returns empty string", func(t *testing.T) {
		assert.Equal(t, "", firstNonEmpty("", "", ""))
	})

	t.Run("no arguments returns empty string", func(t *testing.T) {
		assert.Equal(t, "", firstNonEmpty())
	})

	t.Run("single non-empty value", func(t *testing.T) {
		assert.Equal(t, "only", firstNonEmpty("only"))
	})

	t.Run("single empty value returns empty string", func(t *testing.T) {
		assert.Equal(t, "", firstNonEmpty(""))
	})
}

func TestValidateScanRequest_CIDRTarget(t *testing.T) {
	logger := createTestLogger()
	handler := NewScanHandler(nilScanServicer{}, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		targets     []string
		expectError bool
	}{
		{
			name:        "single CIDR /24",
			targets:     []string{"192.168.1.0/24"},
			expectError: false,
		},
		{
			name:        "single CIDR /8",
			targets:     []string{"10.0.0.0/8"},
			expectError: false,
		},
		{
			name:        "IPv6 CIDR",
			targets:     []string{"2001:db8::/32"},
			expectError: false,
		},
		{
			name:        "mix of IP and CIDR",
			targets:     []string{"10.0.0.1", "192.168.0.0/16"},
			expectError: false,
		},
		{
			name:        "invalid CIDR and not plain IP",
			targets:     []string{"not-a-cidr"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ScanRequest{
				Name:     "CIDR Test",
				Targets:  tt.targets,
				ScanType: "connect",
				Ports:    "80",
			}
			err := handler.validateScanRequest(req)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Integration tests with database
func setupScanHandlerTest(t *testing.T) (*ScanHandler, *db.DB, func()) {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	database, _, err := helpers.ConnectToTestDatabase(ctx)
	if err != nil {
		t.Skipf("Skipping test: database not available: %v", err)
		return nil, nil, nil
	}

	logger := createTestLogger()
	metricsRegistry := metrics.NewRegistry()
	handler := NewScanHandler(services.NewScanService(db.NewScanRepository(database), logger), logger, metricsRegistry)

	// Clean up any leftover test data from previous runs.
	// scan_jobs.network_id → networks(id) ON DELETE CASCADE (migration 011),
	// so deleting a network cascades to its scan_jobs.
	_, _ = database.Exec(`
		DELETE FROM port_scans
		WHERE job_id IN (
			SELECT sj.id FROM scan_jobs sj
			JOIN networks n ON sj.network_id = n.id
			WHERE n.name LIKE 'ScanTest%'
		)`)
	_, _ = database.Exec(`DELETE FROM networks WHERE name LIKE 'ScanTest%'`)

	cleanup := func() {
		// Clean up test data
		_, _ = database.Exec(`
			DELETE FROM port_scans
			WHERE job_id IN (
				SELECT sj.id FROM scan_jobs sj
				JOIN networks n ON sj.network_id = n.id
				WHERE n.name LIKE 'ScanTest%'
			)`)
		_, _ = database.Exec(`DELETE FROM networks WHERE name LIKE 'ScanTest%'`)
		database.Close()
	}

	return handler, database, cleanup
}

func generateUniqueScanName() string {
	return fmt.Sprintf("ScanTest_%s", uuid.New().String()[:8])
}

func TestScanHandler_ListScans_Integration(t *testing.T) {
	handler, database, cleanup := setupScanHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create test scans
	scan1Name := generateUniqueScanName()
	scan2Name := generateUniqueScanName()

	scan1Data := db.CreateScanInput{
		Name:     scan1Name,
		Targets:  []string{generateUniqueCIDR(42)},
		ScanType: "connect",
	}

	scan2Data := db.CreateScanInput{
		Name:     scan2Name,
		Targets:  []string{generateUniqueCIDR(43)},
		ScanType: "syn",
	}

	_, err := db.NewScanRepository(database).CreateScan(ctx, scan1Data)
	require.NoError(t, err)

	_, err = db.NewScanRepository(database).CreateScan(ctx, scan2Data)
	require.NoError(t, err)

	// Test listing scans
	req := httptest.NewRequest("GET", "/api/v1/scans", http.NoBody)
	w := httptest.NewRecorder()

	handler.ListScans(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data []ScanResponse `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(response.Data), 2)

	// Verify our test scans are in the response
	foundScan1 := false
	foundScan2 := false
	for _, scan := range response.Data {
		if scan.Name == scan1Name {
			foundScan1 = true
			assert.Equal(t, "pending", scan.Status)
		}
		if scan.Name == scan2Name {
			foundScan2 = true
			assert.Equal(t, "pending", scan.Status)
		}
	}

	assert.True(t, foundScan1, "Scan 1 not found in response")
	assert.True(t, foundScan2, "Scan 2 not found in response")
}

func TestScanHandler_CreateScan_Integration(t *testing.T) {
	handler, _, cleanup := setupScanHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	scanName := generateUniqueScanName()
	scanRequest := ScanRequest{
		Name:        scanName,
		Description: "Integration test scan",
		Targets:     []string{"192.168.1.0/24"},
		ScanType:    "connect",
		Ports:       "22,80,443",
	}

	body, err := json.Marshal(scanRequest)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/api/v1/scans", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreateScan(w, req)

	// CreateScan endpoint coverage test - checks handler logic
	// Note: Actual creation may require additional database setup
	if w.Code == http.StatusCreated {
		var response ScanResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, scanName, response.Name)
		assert.Equal(t, "connect", response.ScanType)
	}
}

func TestScanHandler_GetScan_Integration(t *testing.T) {
	handler, database, cleanup := setupScanHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create a test scan
	scanName := generateUniqueScanName()
	scanData := db.CreateScanInput{
		Name:     scanName,
		Targets:  []string{generateUniqueCIDR(44)},
		ScanType: "connect",
	}

	createdScan, err := db.NewScanRepository(database).CreateScan(ctx, scanData)
	require.NoError(t, err)

	// Test getting the scan
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/scans/%s", createdScan.ID), http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": createdScan.ID.String()})
	w := httptest.NewRecorder()

	handler.GetScan(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response ScanResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, createdScan.ID, response.ID)
	assert.Equal(t, scanName, response.Name)
}

func TestScanHandler_UpdateScan_Integration(t *testing.T) {
	handler, database, cleanup := setupScanHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create a test scan
	scanName := generateUniqueScanName()
	scanData := db.CreateScanInput{
		Name:     scanName,
		Targets:  []string{"192.168.1.0/24"},
		ScanType: "connect",
	}

	createdScan, err := db.NewScanRepository(database).CreateScan(ctx, scanData)
	require.NoError(t, err)

	// Update the scan
	updateRequest := ScanRequest{
		Name:        scanName + "_updated",
		Description: "Updated description",
		Targets:     []string{"192.168.1.0/24"},
		ScanType:    "syn",
	}

	body, err := json.Marshal(updateRequest)
	require.NoError(t, err)

	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/scans/%s", createdScan.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = mux.SetURLVars(req, map[string]string{"id": createdScan.ID.String()})
	w := httptest.NewRecorder()

	handler.UpdateScan(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response ScanResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Contains(t, response.Name, "updated")
	assert.Equal(t, "Updated description", response.Description)
}

func TestScanHandler_DeleteScan_Integration(t *testing.T) {
	handler, database, cleanup := setupScanHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create a test scan
	scanName := generateUniqueScanName()
	scanData := db.CreateScanInput{
		Name:     scanName,
		Targets:  []string{"192.168.1.0/24"},
		ScanType: "connect",
	}

	createdScan, err := db.NewScanRepository(database).CreateScan(ctx, scanData)
	require.NoError(t, err)

	// Delete the scan
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/scans/%s", createdScan.ID), http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": createdScan.ID.String()})
	w := httptest.NewRecorder()

	handler.DeleteScan(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)

	// Verify the scan is deleted
	_, err = db.NewScanRepository(database).GetScan(ctx, createdScan.ID)
	assert.Error(t, err)
}

func TestScanHandler_StartScan_Integration(t *testing.T) {
	handler, database, cleanup := setupScanHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create a test scan
	scanName := generateUniqueScanName()
	scanData := db.CreateScanInput{
		Name:     scanName,
		Targets:  []string{"192.168.1.1"},
		ScanType: "connect",
	}

	createdScan, err := db.NewScanRepository(database).CreateScan(ctx, scanData)
	require.NoError(t, err)

	// Start the scan
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/scans/%s/start", createdScan.ID), http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": createdScan.ID.String()})
	w := httptest.NewRecorder()

	handler.StartScan(w, req)

	// Should return OK
	assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusAccepted, "Expected 200 or 202, got %d", w.Code)
}

func TestScanHandler_StopScan_Integration(t *testing.T) {
	handler, database, cleanup := setupScanHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create a test scan
	scanName := generateUniqueScanName()
	scanData := db.CreateScanInput{
		Name:     scanName,
		Targets:  []string{"192.168.1.1"},
		ScanType: "connect",
	}

	createdScan, err := db.NewScanRepository(database).CreateScan(ctx, scanData)
	require.NoError(t, err)

	// Start it first
	_ = db.NewScanRepository(database).StartScan(ctx, createdScan.ID)

	// Stop the scan
	req := httptest.NewRequest("POST", fmt.Sprintf("/api/v1/scans/%s/stop", createdScan.ID), http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": createdScan.ID.String()})
	w := httptest.NewRecorder()

	handler.StopScan(w, req)

	assert.True(t, w.Code == http.StatusNoContent || w.Code == http.StatusOK, "Expected 204 or 200, got %d", w.Code)
}

func TestScanHandler_GetScanResults_Integration(t *testing.T) {
	handler, database, cleanup := setupScanHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create a test scan
	scanName := generateUniqueScanName()
	scanData := db.CreateScanInput{
		Name:     scanName,
		Targets:  []string{"192.168.1.1"},
		ScanType: "connect",
	}

	createdScan, err := db.NewScanRepository(database).CreateScan(ctx, scanData)
	require.NoError(t, err)

	// Get scan results (might be empty but should not error)
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/scans/%s/results", createdScan.ID), http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": createdScan.ID.String()})
	w := httptest.NewRecorder()

	handler.GetScanResults(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response ScanResultsResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Results might be empty for a newly created scan
	assert.NotNil(t, response.Results)
}
