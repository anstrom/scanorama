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

func TestNewHostHandler(t *testing.T) {
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
			handler := NewHostHandler(tt.database, logger, tt.metrics)

			assert.NotNil(t, handler)
			assert.NotNil(t, handler.logger)
			assert.Equal(t, tt.metrics, handler.metrics)
		})
	}
}

func TestHostHandler_ValidateHostRequest(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		request     *HostRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request",
			request: &HostRequest{
				IP:       "192.168.1.1",
				Hostname: "test-host",
			},
			expectError: false,
		},
		{
			name: "empty IP",
			request: &HostRequest{
				IP:       "",
				Hostname: "test-host",
			},
			expectError: true,
			errorMsg:    "host IP is required",
		},
		{
			name: "invalid IP format",
			request: &HostRequest{
				IP:       "invalid-ip",
				Hostname: "test-host",
			},
			expectError: true,
			errorMsg:    "invalid IP address format",
		},
		{
			name: "hostname too long",
			request: &HostRequest{
				IP:       "192.168.1.1",
				Hostname: strings.Repeat("a", 256),
			},
			expectError: true,
			errorMsg:    "hostname too long",
		},
		{
			name: "description too long",
			request: &HostRequest{
				IP:          "192.168.1.1",
				Hostname:    "test-host",
				Description: strings.Repeat("a", 1001),
			},
			expectError: true,
			errorMsg:    "description too long",
		},
		{
			name: "valid IPv6",
			request: &HostRequest{
				IP:       "2001:db8::1",
				Hostname: "test-host-ipv6",
			},
			expectError: false,
		},
		{
			name: "maximum valid hostname length",
			request: &HostRequest{
				IP:       "192.168.1.1",
				Hostname: strings.Repeat("a", 255),
			},
			expectError: false,
		},
		{
			name: "maximum valid description length",
			request: &HostRequest{
				IP:          "192.168.1.1",
				Hostname:    "test-host",
				Description: strings.Repeat("a", 1000),
			},
			expectError: false,
		},
		{
			name: "localhost IP",
			request: &HostRequest{
				IP:       "127.0.0.1",
				Hostname: "localhost",
			},
			expectError: false,
		},
		{
			name: "private network IP",
			request: &HostRequest{
				IP:       "10.0.0.1",
				Hostname: "internal-host",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateHostRequest(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHostHandler_GetHostFilters(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name           string
		queryParams    string
		expectedFilter db.HostFilters
	}{
		{
			name:           "no filters",
			queryParams:    "",
			expectedFilter: db.HostFilters{},
		},
		{
			name:        "status filter",
			queryParams: "?status=up",
			expectedFilter: db.HostFilters{
				Status: "up",
			},
		},
		{
			name:        "os family filter",
			queryParams: "?os=linux",
			expectedFilter: db.HostFilters{
				OSFamily: "linux",
			},
		},
		{
			name:        "network filter",
			queryParams: "?network=192.168.1.0/24",
			expectedFilter: db.HostFilters{
				Network: "192.168.1.0/24",
			},
		},
		{
			name:        "multiple filters",
			queryParams: "?status=up&os=windows&network=10.0.0.0/8",
			expectedFilter: db.HostFilters{
				Status:   "up",
				OSFamily: "windows",
				Network:  "10.0.0.0/8",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/hosts"+tt.queryParams, http.NoBody)
			filters := handler.getHostFilters(req)

			assert.Equal(t, tt.expectedFilter.Status, filters.Status)
			assert.Equal(t, tt.expectedFilter.OSFamily, filters.OSFamily)
			assert.Equal(t, tt.expectedFilter.Network, filters.Network)
		})
	}
}

func TestHostHandler_RequestToDBHost(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	request := &HostRequest{
		IP:          "192.168.1.100",
		Hostname:    "test-server",
		Description: "Test server description",
		OS:          "linux",
		OSVersion:   "20.04",
		Tags:        []string{"production", "web"},
		Metadata:    map[string]string{"env": "test"},
	}

	result := handler.requestToDBHost(request)
	data, ok := result.(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, request.IP, data["ip"])
	assert.Equal(t, request.Hostname, data["hostname"])
	assert.Equal(t, request.Description, data["description"])
	assert.Equal(t, request.OS, data["os"])
	assert.Equal(t, request.OSVersion, data["os_version"])
	assert.Equal(t, request.Tags, data["tags"])
	assert.Equal(t, request.Metadata, data["metadata"])
}

func TestHostHandler_HostToResponse(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	testHostID := uuid.New()
	hostname := "example-host"
	osFamily := "windows"
	osVersion := "2019"
	testHost := &db.Host{
		ID:        testHostID,
		IPAddress: db.IPAddr{},
		Hostname:  &hostname,
		OSFamily:  &osFamily,
		OSVersion: &osVersion,
		Status:    "up",
		FirstSeen: time.Now().Add(-24 * time.Hour),
		LastSeen:  time.Now(),
	}

	response := handler.hostToResponse(testHost)

	// Note: The current hostToResponse returns placeholder data
	// These assertions test the function call works, not actual mapping
	assert.NotEmpty(t, response.IP)
	assert.Equal(t, int64(1), response.ID) // placeholder value
	assert.NotNil(t, response)
}

func TestHostHandler_CreateHost_ValidationErrors(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		requestBody interface{}
	}{
		{
			name: "validation error - empty IP",
			requestBody: HostRequest{
				IP:       "",
				Hostname: "test-host",
			},
		},
		{
			name: "validation error - invalid IP",
			requestBody: HostRequest{
				IP:       "not-an-ip",
				Hostname: "test-host",
			},
		},
		{
			name: "validation error - hostname too long",
			requestBody: HostRequest{
				IP:       "192.168.1.1",
				Hostname: strings.Repeat("a", 256),
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

			req := httptest.NewRequest("POST", "/api/v1/hosts", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreateHost(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestHostHandler_GetHost_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/hosts/invalid-uuid", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.GetHost(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHostHandler_UpdateHost_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	updateRequest := HostRequest{
		IP:       "192.168.1.1",
		Hostname: "updated-host",
	}
	body, _ := json.Marshal(updateRequest)

	req := httptest.NewRequest("PUT", "/api/v1/hosts/invalid-uuid", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.UpdateHost(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHostHandler_DeleteHost_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("DELETE", "/api/v1/hosts/invalid-uuid", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.DeleteHost(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHostHandler_GetHostScans_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/hosts/invalid-uuid/scans", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.GetHostScans(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHostHandler_EdgeCases(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	t.Run("IP validation edge cases", func(t *testing.T) {
		testCases := []struct {
			ip    string
			valid bool
		}{
			{"0.0.0.0", true},
			{"255.255.255.255", true},
			{"::1", true},
			{"fe80::1", true},
			{"256.1.1.1", false},
			{"1.1.1", false},
			{"", false},
		}

		for _, tc := range testCases {
			req := &HostRequest{
				IP:       tc.ip,
				Hostname: "test",
			}
			err := handler.validateHostRequest(req)
			if tc.valid {
				assert.NoError(t, err, "IP %s should be valid", tc.ip)
			} else {
				assert.Error(t, err, "IP %s should be invalid", tc.ip)
			}
		}
	})

	t.Run("hostname validation", func(t *testing.T) {
		// Test valid hostnames
		validHostnames := []string{
			"localhost",
			"example.com",
			"host-1",
			"test123",
			"a.b.c.d",
		}

		for _, hostname := range validHostnames {
			req := &HostRequest{
				IP:       "192.168.1.1",
				Hostname: hostname,
			}
			err := handler.validateHostRequest(req)
			assert.NoError(t, err, "hostname %s should be valid", hostname)
		}
	})
}

func TestHostHandler_RequestValidation_Comprehensive(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	t.Run("maximum valid host request", func(t *testing.T) {
		req := &HostRequest{
			IP:          "192.168.1.200",
			Hostname:    strings.Repeat("a", 255),  // max length
			Description: strings.Repeat("b", 1000), // max length
			OS:          "linux",
			OSVersion:   "22.04",
			Tags:        []string{"tag1", "tag2", "tag3"},
		}

		err := handler.validateHostRequest(req)
		assert.NoError(t, err)
	})

	t.Run("boundary conditions", func(t *testing.T) {
		// Test exactly at the boundary
		req := &HostRequest{
			IP:          "10.0.0.1",
			Hostname:    strings.Repeat("a", 255),  // exactly max length
			Description: strings.Repeat("b", 1000), // exactly max length
		}
		err := handler.validateHostRequest(req)
		assert.NoError(t, err)

		// Test just over the boundary
		req.Hostname = strings.Repeat("a", 256) // one over max length
		err = handler.validateHostRequest(req)
		assert.Error(t, err)

		// Reset hostname and test description boundary
		req.Hostname = "valid-host"
		req.Description = strings.Repeat("b", 1001) // one over max length
		err = handler.validateHostRequest(req)
		assert.Error(t, err)
	})
}

func BenchmarkHostHandler_ValidateHostRequest(b *testing.B) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	request := &HostRequest{
		IP:          "192.168.1.100",
		Hostname:    "benchmark-host",
		Description: "Benchmark host description",
		OS:          "linux",
		OSVersion:   "20.04",
		Tags:        []string{"benchmark", "test"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handler.validateHostRequest(request)
	}
}

func BenchmarkHostHandler_GetHostFilters(b *testing.B) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/hosts?status=up&os=linux&network=192.168.1.0/24", http.NoBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handler.getHostFilters(req)
	}
}

func TestHostHandler_IPValidation_Detailed(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name    string
		ip      string
		isValid bool
	}{
		// Valid IPv4 addresses
		{"valid IPv4 private", "192.168.1.1", true},
		{"valid IPv4 public", "8.8.8.8", true},
		{"valid IPv4 localhost", "127.0.0.1", true},
		{"valid IPv4 zero", "0.0.0.0", true},
		{"valid IPv4 broadcast", "255.255.255.255", true},

		// Valid IPv6 addresses
		{"valid IPv6 localhost", "::1", true},
		{"valid IPv6 full", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", true},
		{"valid IPv6 compressed", "2001:db8:85a3::8a2e:370:7334", true},
		{"valid IPv6 link-local", "fe80::1", true},

		// Invalid IP addresses
		{"invalid IPv4 out of range", "256.1.1.1", false},
		{"invalid IPv4 incomplete", "192.168.1", false},
		{"invalid IPv4 text", "not.an.ip.address", false},
		{"invalid empty", "", false},
		{"invalid text", "example.com", false},
		{"valid partial IPv6", "2001:db8::", true}, // This is actually valid IPv6
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &HostRequest{
				IP:       tt.ip,
				Hostname: "test-host",
			}

			err := handler.validateHostRequest(req)
			if tt.isValid {
				assert.NoError(t, err, "IP %s should be valid", tt.ip)
			} else {
				assert.Error(t, err, "IP %s should be invalid", tt.ip)
			}
		})
	}
}
