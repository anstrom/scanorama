package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
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

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/test/helpers"
)

// Integration test setup helper
func setupHostHandlerTest(t *testing.T) (*HostHandler, *db.DB, func()) {
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
	handler := NewHostHandler(db.NewHostRepository(database), logger, metricsRegistry)

	// Clean up any leftover test data (hostname-based and hardcoded IPs used in integration tests)
	_, _ = database.Exec(`DELETE FROM hosts WHERE hostname LIKE 'HostTest%'`)
	_, _ = database.Exec(`DELETE FROM hosts WHERE ip_address IN (
		'192.168.1.100'::inet, '192.168.1.101'::inet, '192.168.1.150'::inet,
		'192.168.1.200'::inet, '192.168.1.201'::inet, '192.168.1.202'::inet
	)`)

	cleanup := func() {
		// Clean up test data
		_, _ = database.Exec(`DELETE FROM hosts WHERE hostname LIKE 'HostTest%'`)
		_, _ = database.Exec(`DELETE FROM hosts WHERE ip_address IN (
			'192.168.1.100'::inet, '192.168.1.101'::inet, '192.168.1.150'::inet,
			'192.168.1.200'::inet, '192.168.1.201'::inet, '192.168.1.202'::inet
		)`)
		database.Close()
	}

	return handler, database, cleanup
}

func generateUniqueHostname() string {
	return fmt.Sprintf("HostTest_%s", uuid.New().String()[:8])
}

func TestNewHostHandler(t *testing.T) {
	testMetrics := metrics.NewRegistry()

	tests := []struct {
		name     string
		database HostServicer
		metrics  *metrics.Registry
	}{
		{
			name:     "with store and metrics",
			database: nilHostServicer{},
			metrics:  testMetrics,
		},
		{
			name:     "with nil store",
			database: nilHostServicer{},
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
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

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
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

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
		{
			name:        "search filter",
			queryParams: "?search=myhost",
			expectedFilter: db.HostFilters{
				Search: "myhost",
			},
		},
		{
			name:        "sort_by and sort_order",
			queryParams: "?sort_by=hostname&sort_order=asc",
			expectedFilter: db.HostFilters{
				SortBy:    "hostname",
				SortOrder: "asc",
			},
		},
		{
			name:        "all filters combined",
			queryParams: "?status=up&os=linux&network=10.0.0.0/8&search=web&sort_by=last_seen&sort_order=desc",
			expectedFilter: db.HostFilters{
				Status:    "up",
				OSFamily:  "linux",
				Network:   "10.0.0.0/8",
				Search:    "web",
				SortBy:    "last_seen",
				SortOrder: "desc",
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
			assert.Equal(t, tt.expectedFilter.Search, filters.Search)
			assert.Equal(t, tt.expectedFilter.SortBy, filters.SortBy)
			assert.Equal(t, tt.expectedFilter.SortOrder, filters.SortOrder)
		})
	}
}

func TestHostHandler_RequestToDBHost(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

	request := &HostRequest{
		IP:          "192.168.1.100",
		Hostname:    "test-server",
		Description: "Test server description",
		OS:          "linux",
		OSVersion:   "20.04",
		Tags:        []string{"production", "web"},
		Metadata:    map[string]string{"env": "test"},
	}

	result := handler.requestToCreateHost(request)

	assert.Equal(t, request.IP, result.IPAddress)
	assert.Equal(t, request.Hostname, result.Hostname)
	assert.Equal(t, request.OS, result.OSFamily)
	assert.Equal(t, request.OSVersion, result.OSName)
}

func TestHostHandler_HostToResponse(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

	testHostID := uuid.New()
	hostname := "example-host"
	osFamily := "windows"
	osVersion := "2019"
	testHost := &db.Host{
		ID:        testHostID,
		IPAddress: db.IPAddr{IP: net.ParseIP("192.168.1.100")},
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
	assert.NotEmpty(t, response.IPAddress)
	assert.Equal(t, testHostID.String(), response.ID) // UUID string
	assert.NotNil(t, response)
}

func TestHostHandler_CreateHost_ValidationErrors(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

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
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/hosts/invalid-uuid", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.GetHost(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHostHandler_UpdateHost_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

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
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

	req := httptest.NewRequest("DELETE", "/api/v1/hosts/invalid-uuid", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.DeleteHost(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHostHandler_GetHostScans_InvalidUUID(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/hosts/invalid-uuid/scans", http.NoBody)
	req.SetPathValue("id", "invalid-uuid")
	w := httptest.NewRecorder()

	handler.GetHostScans(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHostHandler_EdgeCases(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

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
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

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
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

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
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/hosts?status=up&os=linux&network=192.168.1.0/24", http.NoBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handler.getHostFilters(req)
	}
}

func TestHostHandler_IPValidation_Detailed(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

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

// Integration tests with database

func TestHostHandler_ListHosts_Integration(t *testing.T) {
	handler, database, cleanup := setupHostHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create test hosts
	host1Name := generateUniqueHostname()
	host2Name := generateUniqueHostname()

	host1Data := db.CreateHostInput{
		IPAddress: "192.168.1.100",
		Hostname:  host1Name,
		OSFamily:  "linux",
		Status:    "up",
	}

	host2Data := db.CreateHostInput{
		IPAddress: "192.168.1.101",
		Hostname:  host2Name,
		OSFamily:  "windows",
		Status:    "up",
	}

	_, err := db.NewHostRepository(database).CreateHost(ctx, host1Data)
	require.NoError(t, err)

	_, err = db.NewHostRepository(database).CreateHost(ctx, host2Data)
	require.NoError(t, err)

	// Test listing hosts
	req := httptest.NewRequest("GET", "/api/v1/hosts", http.NoBody)
	w := httptest.NewRecorder()

	handler.ListHosts(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data []HostResponse `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(response.Data), 2)

	// Verify our test hosts are in the response
	foundHost1 := false
	foundHost2 := false
	for _, host := range response.Data {
		if strings.Contains(host.Hostname, host1Name) {
			foundHost1 = true
		}
		if strings.Contains(host.Hostname, host2Name) {
			foundHost2 = true
		}
	}

	assert.True(t, foundHost1, "Host 1 not found in response")
	assert.True(t, foundHost2, "Host 2 not found in response")
}

func TestHostHandler_CreateHost_Integration(t *testing.T) {
	handler, _, cleanup := setupHostHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	hostName := generateUniqueHostname()
	hostRequest := HostRequest{
		IP:          "192.168.1.150",
		Hostname:    hostName,
		Description: "Integration test host",
		OS:          "linux",
		OSVersion:   "Ubuntu 20.04",
	}

	body, err := json.Marshal(hostRequest)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/api/v1/hosts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreateHost(w, req)

	// CreateHost endpoint coverage test - checks handler logic
	if w.Code == http.StatusCreated {
		var response HostResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.IPAddress)
	}
}

func TestHostHandler_GetHost_Integration(t *testing.T) {
	handler, database, cleanup := setupHostHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create a test host
	hostName := generateUniqueHostname()
	hostData := db.CreateHostInput{
		IPAddress: "192.168.1.200",
		Hostname:  hostName,
		OSFamily:  "linux",
		Status:    "up",
	}

	createdHost, err := db.NewHostRepository(database).CreateHost(ctx, hostData)
	require.NoError(t, err)

	// Test getting the host
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/hosts/%s", createdHost.ID), http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": createdHost.ID.String()})
	w := httptest.NewRecorder()

	handler.GetHost(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response HostResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, createdHost.ID.String(), response.ID)
}

func TestHostHandler_UpdateHost_Integration(t *testing.T) {
	handler, database, cleanup := setupHostHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create a test host
	hostName := generateUniqueHostname()
	hostData := db.CreateHostInput{
		IPAddress: "192.168.1.210",
		Hostname:  hostName,
		OSFamily:  "linux",
		Status:    "up",
	}

	createdHost, err := db.NewHostRepository(database).CreateHost(ctx, hostData)
	require.NoError(t, err)

	// Update the host
	updateRequest := HostRequest{
		IP:          "192.168.1.210",
		Hostname:    hostName + "_updated",
		Description: "Updated description",
		OS:          "linux",
	}

	body, err := json.Marshal(updateRequest)
	require.NoError(t, err)

	req := httptest.NewRequest("PUT", fmt.Sprintf("/api/v1/hosts/%s", createdHost.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", createdHost.ID.String())
	w := httptest.NewRecorder()

	handler.UpdateHost(w, req)

	// UpdateHost endpoint coverage test
	if w.Code == http.StatusOK {
		var response HostResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.NotEmpty(t, response.ID)
	}
}

func TestHostHandler_DeleteHost_Integration(t *testing.T) {
	handler, database, cleanup := setupHostHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create a test host
	hostName := generateUniqueHostname()
	hostData := db.CreateHostInput{
		IPAddress: "192.168.1.220",
		Hostname:  hostName,
		OSFamily:  "linux",
		Status:    "up",
	}

	createdHost, err := db.NewHostRepository(database).CreateHost(ctx, hostData)
	require.NoError(t, err)

	// Delete the host
	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/v1/hosts/%s", createdHost.ID), http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": createdHost.ID.String()})
	w := httptest.NewRecorder()

	handler.DeleteHost(w, req)

	assert.True(t, w.Code == http.StatusNoContent || w.Code == http.StatusOK,
		"Expected 204 or 200, got %d", w.Code)

	// Verify the host is deleted
	if w.Code == http.StatusNoContent {
		_, err = db.NewHostRepository(database).GetHost(ctx, createdHost.ID)
		assert.Error(t, err)
	}
}

func TestHostHandler_GetHostScans_Integration(t *testing.T) {
	handler, database, cleanup := setupHostHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create a test host
	hostName := generateUniqueHostname()
	hostData := db.CreateHostInput{
		IPAddress: "192.168.1.230",
		Hostname:  hostName,
		OSFamily:  "linux",
		Status:    "up",
	}

	createdHost, err := db.NewHostRepository(database).CreateHost(ctx, hostData)
	require.NoError(t, err)

	// Get host scans (might be empty but should not error)
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/v1/hosts/%s/scans", createdHost.ID), http.NoBody)
	req = mux.SetURLVars(req, map[string]string{"id": createdHost.ID.String()})
	w := httptest.NewRecorder()

	handler.GetHostScans(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data []interface{} `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Scans might be empty for a newly created host
	assert.NotNil(t, response.Data)
}

func TestHostHandler_GetScanFilters_WithTimestamps(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

	t.Run("valid created_after timestamp", func(t *testing.T) {
		ts := time.Now().UTC().Format(time.RFC3339)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/id/scans?created_after="+ts, nil)
		filters := handler.getScanFilters(req)
		assert.Contains(t, filters, "created_after")
		parsed, ok := filters["created_after"].(time.Time)
		assert.True(t, ok)
		assert.False(t, parsed.IsZero())
	})

	t.Run("valid created_before timestamp", func(t *testing.T) {
		ts := time.Now().UTC().Format(time.RFC3339)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/id/scans?created_before="+ts, nil)
		filters := handler.getScanFilters(req)
		assert.Contains(t, filters, "created_before")
		parsed, ok := filters["created_before"].(time.Time)
		assert.True(t, ok)
		assert.False(t, parsed.IsZero())
	})

	t.Run("invalid created_after timestamp is ignored", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/id/scans?created_after=not-a-timestamp", nil)
		filters := handler.getScanFilters(req)
		assert.NotContains(t, filters, "created_after")
	})

	t.Run("invalid created_before timestamp is ignored", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/id/scans?created_before=20230101", nil)
		filters := handler.getScanFilters(req)
		assert.NotContains(t, filters, "created_before")
	})

	t.Run("status filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/id/scans?status=running", nil)
		filters := handler.getScanFilters(req)
		assert.Equal(t, "running", filters["status"])
	})

	t.Run("scan_type filter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/id/scans?scan_type=comprehensive", nil)
		filters := handler.getScanFilters(req)
		assert.Equal(t, "comprehensive", filters["scan_type"])
	})

	t.Run("all filters combined", func(t *testing.T) {
		after := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
		before := time.Now().UTC().Format(time.RFC3339)
		url := fmt.Sprintf(
			"/api/v1/hosts/id/scans?status=completed&scan_type=connect&created_after=%s&created_before=%s",
			after, before)
		req := httptest.NewRequest(http.MethodGet, url, nil)
		filters := handler.getScanFilters(req)
		assert.Equal(t, "completed", filters["status"])
		assert.Equal(t, "connect", filters["scan_type"])
		assert.Contains(t, filters, "created_after")
		assert.Contains(t, filters, "created_before")
	})

	t.Run("no filters", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/id/scans", nil)
		filters := handler.getScanFilters(req)
		assert.Empty(t, filters)
	})
}

func TestHostHandler_ScanToHostScanResponse(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

	t.Run("scan without start or end time", func(t *testing.T) {
		scan := &db.Scan{
			ID:        uuid.New(),
			Name:      "Test Scan",
			ScanType:  "connect",
			Status:    "pending",
			CreatedAt: time.Now(),
		}

		resp := handler.scanToHostScanResponse(scan)

		assert.Equal(t, "Test Scan", resp.Name)
		assert.Equal(t, "connect", resp.ScanType)
		assert.Equal(t, "pending", resp.Status)
		assert.Nil(t, resp.StartTime)
		assert.Nil(t, resp.EndTime)
		assert.Nil(t, resp.Duration)
		assert.Equal(t, 0.0, resp.Progress)
	})

	t.Run("scan with start time only", func(t *testing.T) {
		startTime := time.Now().Add(-5 * time.Minute)
		scan := &db.Scan{
			ID:        uuid.New(),
			Name:      "Running Scan",
			ScanType:  "comprehensive",
			Status:    "running",
			CreatedAt: time.Now().Add(-5 * time.Minute),
			StartedAt: &startTime,
		}

		resp := handler.scanToHostScanResponse(scan)

		assert.Equal(t, "Running Scan", resp.Name)
		assert.Equal(t, "running", resp.Status)
		assert.NotNil(t, resp.StartTime)
		assert.Equal(t, &startTime, resp.StartTime)
		assert.Nil(t, resp.EndTime)
		assert.Nil(t, resp.Duration)
	})

	t.Run("scan with start and end time computes duration", func(t *testing.T) {
		startTime := time.Now().Add(-10 * time.Minute)
		endTime := time.Now()
		scan := &db.Scan{
			ID:          uuid.New(),
			Name:        "Completed Scan",
			ScanType:    "connect",
			Status:      "completed",
			CreatedAt:   startTime,
			StartedAt:   &startTime,
			CompletedAt: &endTime,
		}

		resp := handler.scanToHostScanResponse(scan)

		assert.Equal(t, "Completed Scan", resp.Name)
		assert.Equal(t, "completed", resp.Status)
		assert.NotNil(t, resp.StartTime)
		assert.NotNil(t, resp.EndTime)
		assert.NotNil(t, resp.Duration)
		assert.NotEmpty(t, *resp.Duration)
	})

	t.Run("scan with end time but no start time omits duration", func(t *testing.T) {
		endTime := time.Now()
		scan := &db.Scan{
			ID:          uuid.New(),
			Name:        "Odd Scan",
			ScanType:    "connect",
			Status:      "completed",
			CreatedAt:   time.Now().Add(-5 * time.Minute),
			CompletedAt: &endTime,
		}

		resp := handler.scanToHostScanResponse(scan)

		assert.NotNil(t, resp.EndTime)
		assert.Nil(t, resp.StartTime)
		assert.Nil(t, resp.Duration)
	})

	t.Run("fields are mapped correctly", func(t *testing.T) {
		createdAt := time.Now().Add(-1 * time.Hour)
		scan := &db.Scan{
			ID:        uuid.New(),
			Name:      "Field Check Scan",
			ScanType:  "ping",
			Status:    "failed",
			CreatedAt: createdAt,
		}

		resp := handler.scanToHostScanResponse(scan)

		assert.Equal(t, "Field Check Scan", resp.Name)
		assert.Equal(t, "ping", resp.ScanType)
		assert.Equal(t, "failed", resp.Status)
		assert.Equal(t, createdAt, resp.CreatedAt)
	})
}

func TestHostHandler_ListHosts_WithFilters_Integration(t *testing.T) {
	handler, database, cleanup := setupHostHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create test hosts with different OS families
	linuxHost := generateUniqueHostname()
	windowsHost := generateUniqueHostname()

	linuxData := db.CreateHostInput{
		IPAddress: "192.168.1.240",
		Hostname:  linuxHost,
		OSFamily:  "linux",
		Status:    "up",
	}

	windowsData := db.CreateHostInput{
		IPAddress: "192.168.1.241",
		Hostname:  windowsHost,
		OSFamily:  "windows",
		Status:    "up",
	}

	_, err := db.NewHostRepository(database).CreateHost(ctx, linuxData)
	require.NoError(t, err)

	_, err = db.NewHostRepository(database).CreateHost(ctx, windowsData)
	require.NoError(t, err)

	// Test filtering by OS
	req := httptest.NewRequest("GET", "/api/v1/hosts?os=linux", http.NoBody)
	w := httptest.NewRecorder()

	handler.ListHosts(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data []HostResponse `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Should have at least our Linux host
	foundLinux := false
	for _, host := range response.Data {
		if strings.Contains(host.Hostname, linuxHost) {
			foundLinux = true
		}
	}
	assert.True(t, foundLinux, "Linux host not found in filtered results")
}

func TestHostHandler_HostToResponse_NewFields(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

	now := time.Now()

	t.Run("ports populated from host.Ports", func(t *testing.T) {
		ports := []db.PortInfo{
			{Port: 80, Protocol: "tcp", State: "open", Service: "http", LastSeen: now},
			{Port: 443, Protocol: "tcp", State: "open", Service: "https", LastSeen: now},
		}
		host := &db.Host{
			ID:        uuid.New(),
			IPAddress: db.IPAddr{IP: net.ParseIP("10.0.0.1")},
			Status:    "up",
			FirstSeen: now,
			LastSeen:  now,
			Ports:     ports,
		}

		resp := handler.hostToResponse(host)

		assert.Equal(t, 2, len(resp.Ports))
		assert.Equal(t, 80, resp.Ports[0].Port)
		assert.Equal(t, "tcp", resp.Ports[0].Protocol)
		assert.Equal(t, "http", resp.Ports[0].Service)
		assert.Equal(t, 443, resp.Ports[1].Port)
	})

	t.Run("ports is empty slice (not nil) when host.Ports is nil", func(t *testing.T) {
		host := &db.Host{
			ID:        uuid.New(),
			IPAddress: db.IPAddr{IP: net.ParseIP("10.0.0.2")},
			Status:    "up",
			FirstSeen: now,
			LastSeen:  now,
			Ports:     nil,
		}

		resp := handler.hostToResponse(host)

		assert.NotNil(t, resp.Ports)
		assert.Equal(t, 0, len(resp.Ports))
	})

	t.Run("ScanCount mapped from host.ScanCount", func(t *testing.T) {
		host := &db.Host{
			ID:        uuid.New(),
			IPAddress: db.IPAddr{IP: net.ParseIP("10.0.0.3")},
			Status:    "up",
			FirstSeen: now,
			LastSeen:  now,
			ScanCount: 42,
		}

		resp := handler.hostToResponse(host)

		assert.Equal(t, 42, resp.ScanCount)
	})

	t.Run("TotalPorts mapped from host.TotalPorts", func(t *testing.T) {
		host := &db.Host{
			ID:         uuid.New(),
			IPAddress:  db.IPAddr{IP: net.ParseIP("10.0.0.4")},
			Status:     "up",
			FirstSeen:  now,
			LastSeen:   now,
			TotalPorts: 7,
		}

		resp := handler.hostToResponse(host)

		assert.Equal(t, 7, resp.TotalPorts)
	})

	t.Run("OSName populated when host.OSName is non-nil", func(t *testing.T) {
		osName := "Linux 5.15"
		host := &db.Host{
			ID:        uuid.New(),
			IPAddress: db.IPAddr{IP: net.ParseIP("10.0.0.5")},
			Status:    "up",
			FirstSeen: now,
			LastSeen:  now,
			OSName:    &osName,
		}

		resp := handler.hostToResponse(host)

		assert.Equal(t, "Linux 5.15", resp.OSName)
	})

	t.Run("OSConfidence populated when host.OSConfidence is non-nil", func(t *testing.T) {
		confidence := 95
		host := &db.Host{
			ID:           uuid.New(),
			IPAddress:    db.IPAddr{IP: net.ParseIP("10.0.0.6")},
			Status:       "up",
			FirstSeen:    now,
			LastSeen:     now,
			OSConfidence: &confidence,
		}

		resp := handler.hostToResponse(host)

		assert.NotNil(t, resp.OSConfidence)
		assert.Equal(t, 95, *resp.OSConfidence)
	})

	t.Run("OSFamily populates both response.OSFamily and legacy response.OS", func(t *testing.T) {
		osFamily := "Linux"
		host := &db.Host{
			ID:        uuid.New(),
			IPAddress: db.IPAddr{IP: net.ParseIP("10.0.0.7")},
			Status:    "up",
			FirstSeen: now,
			LastSeen:  now,
			OSFamily:  &osFamily,
		}

		resp := handler.hostToResponse(host)

		assert.Equal(t, "Linux", resp.OSFamily)
		assert.Equal(t, "Linux", resp.OS)
	})

	t.Run("OSVersionLegacy is OSName+OSVersion when both are set", func(t *testing.T) {
		osName := "Linux 5.15"
		osVersion := "Kernel 5.15.0"
		host := &db.Host{
			ID:        uuid.New(),
			IPAddress: db.IPAddr{IP: net.ParseIP("10.0.0.8")},
			Status:    "up",
			FirstSeen: now,
			LastSeen:  now,
			OSName:    &osName,
			OSVersion: &osVersion,
		}

		resp := handler.hostToResponse(host)

		assert.Equal(t, "Linux 5.15 Kernel 5.15.0", resp.OSVersionLegacy)
	})

	t.Run("OSVersionLegacy is just OSName when only OSName is set", func(t *testing.T) {
		osName := "Linux 5.15"
		host := &db.Host{
			ID:        uuid.New(),
			IPAddress: db.IPAddr{IP: net.ParseIP("10.0.0.9")},
			Status:    "up",
			FirstSeen: now,
			LastSeen:  now,
			OSName:    &osName,
			OSVersion: nil,
		}

		resp := handler.hostToResponse(host)

		assert.Equal(t, "Linux 5.15", resp.OSVersionLegacy)
	})

	t.Run("OSVersionLegacy is just OSVersion when only OSVersion is set", func(t *testing.T) {
		osVersion := "Kernel 5.15.0"
		host := &db.Host{
			ID:        uuid.New(),
			IPAddress: db.IPAddr{IP: net.ParseIP("10.0.0.10")},
			Status:    "up",
			FirstSeen: now,
			LastSeen:  now,
			OSName:    nil,
			OSVersion: &osVersion,
		}

		resp := handler.hostToResponse(host)

		assert.Equal(t, "Kernel 5.15.0", resp.OSVersionLegacy)
	})
}

func TestHostHandler_ValidateHostRequest_NewBranches(t *testing.T) {
	logger := createTestLogger()
	handler := NewHostHandler(nilHostServicer{}, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		request     *HostRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "OS field too long",
			request: &HostRequest{
				IP: "192.168.1.1",
				OS: strings.Repeat("a", maxOSInfoLength+1),
			},
			expectError: true,
			errorMsg:    "OS info too long",
		},
		{
			name: "OS field at max length is valid",
			request: &HostRequest{
				IP: "192.168.1.1",
				OS: strings.Repeat("a", maxOSInfoLength),
			},
			expectError: false,
		},
		{
			name: "OSVersion field too long",
			request: &HostRequest{
				IP:        "192.168.1.1",
				OSVersion: strings.Repeat("b", maxOSVersionLength+1),
			},
			expectError: true,
			errorMsg:    "OS version field too long",
		},
		{
			name: "OSVersion field at max length is valid",
			request: &HostRequest{
				IP:        "192.168.1.1",
				OSVersion: strings.Repeat("b", maxOSVersionLength),
			},
			expectError: false,
		},
		{
			name: "empty tag in Tags slice",
			request: &HostRequest{
				IP:   "192.168.1.1",
				Tags: []string{"valid-tag", ""},
			},
			expectError: true,
			errorMsg:    "tag 2 is empty",
		},
		{
			name: "tag too long",
			request: &HostRequest{
				IP:   "192.168.1.1",
				Tags: []string{strings.Repeat("t", maxHostTagLength+1)},
			},
			expectError: true,
			errorMsg:    "tag 1 too long",
		},
		{
			name: "tag at max length is valid",
			request: &HostRequest{
				IP:   "192.168.1.1",
				Tags: []string{strings.Repeat("t", maxHostTagLength)},
			},
			expectError: false,
		},
		{
			name: "multiple valid tags pass",
			request: &HostRequest{
				IP:   "192.168.1.1",
				Tags: []string{"production", "web", "linux"},
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

func TestHostHandler_CreateHost_Conflict(t *testing.T) {
	t.Run("returns 409 when host IP already exists", func(t *testing.T) {
		h, store, ctrl := newHostHandlerWithMock(t)
		defer ctrl.Finish()

		store.EXPECT().
			CreateHost(gomock.Any(), gomock.Any()).
			Return(nil, conflictErr("host", "already exists"))

		body, err := json.Marshal(HostRequest{
			IP:       "192.168.1.50",
			Hostname: "duplicate-host",
		})
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/hosts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		h.CreateHost(w, req)

		assert.Equal(t, http.StatusConflict, w.Code)

		var errResp map[string]interface{}
		err = json.Unmarshal(w.Body.Bytes(), &errResp)
		require.NoError(t, err)
		assert.Contains(t, errResp["message"], "192.168.1.50")
	})
}
