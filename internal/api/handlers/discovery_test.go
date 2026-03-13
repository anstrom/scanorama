package handlers

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

func TestNewDiscoveryHandler(t *testing.T) {
	testMetrics := metrics.NewRegistry()

	tests := []struct {
		name     string
		database DiscoveryStore
		metrics  *metrics.Registry
	}{
		{
			name:     "with store and metrics",
			database: nilDiscoveryStore{},
			metrics:  testMetrics,
		},
		{
			name:     "with nil store",
			database: nilDiscoveryStore{},
			metrics:  testMetrics,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := createTestLogger()
			handler := NewDiscoveryHandler(tt.database, logger, tt.metrics)

			assert.NotNil(t, handler)
			assert.NotNil(t, handler.logger)
			assert.Equal(t, tt.metrics, handler.metrics)
		})
	}
}

func TestDiscoveryHandler_ValidateDiscoveryRequest(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		request     *DiscoveryRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request with ping method",
			request: &DiscoveryRequest{
				Name:     "Test Discovery",
				Networks: []string{"192.168.1.0/24"},
				Method:   "ping",
				Enabled:  true,
			},
			expectError: false,
		},
		{
			name: "valid request with arp method",
			request: &DiscoveryRequest{
				Name:     "ARP Discovery",
				Networks: []string{"10.0.0.0/24"},
				Method:   "arp",
				Enabled:  true,
			},
			expectError: false,
		},
		{
			name: "valid request with icmp method",
			request: &DiscoveryRequest{
				Name:     "ICMP Discovery",
				Networks: []string{"172.16.0.0/16"},
				Method:   "icmp",
				Enabled:  true,
			},
			expectError: false,
		},
		{
			name: "valid request with tcp_connect method",
			request: &DiscoveryRequest{
				Name:     "TCP Discovery",
				Networks: []string{"192.168.1.0/24"},
				Method:   "tcp_connect",
				Ports:    "80,443",
				Enabled:  true,
			},
			expectError: false,
		},
		{
			name: "empty name",
			request: &DiscoveryRequest{
				Name:     "",
				Networks: []string{"192.168.1.0/24"},
				Method:   "ping",
			},
			expectError: true,
			errorMsg:    "name is required",
		},
		{
			name: "name too long",
			request: &DiscoveryRequest{
				Name:     string(make([]byte, 256)),
				Networks: []string{"192.168.1.0/24"},
				Method:   "ping",
			},
			expectError: true,
			errorMsg:    "name too long",
		},
		{
			name: "empty networks",
			request: &DiscoveryRequest{
				Name:     "Test Discovery",
				Networks: []string{},
				Method:   "ping",
			},
			expectError: true,
			errorMsg:    "at least one network is required",
		},
		{
			name: "invalid method",
			request: &DiscoveryRequest{
				Name:     "Test Discovery",
				Networks: []string{"192.168.1.0/24"},
				Method:   "invalid_method",
			},
			expectError: true,
			errorMsg:    "invalid discovery method",
		},
		{
			name: "invalid CIDR notation",
			request: &DiscoveryRequest{
				Name:     "Test Discovery",
				Networks: []string{"192.168.1.0/33"},
				Method:   "ping",
			},
			expectError: true,
			errorMsg:    "invalid format",
		},
		{
			name: "invalid IP format",
			request: &DiscoveryRequest{
				Name:     "Test Discovery",
				Networks: []string{"not-an-ip"},
				Method:   "ping",
			},
			expectError: true,
			errorMsg:    "invalid format",
		},
		{
			name: "retries too high",
			request: &DiscoveryRequest{
				Name:     "Test Discovery",
				Networks: []string{"192.168.1.0/24"},
				Method:   "ping",
				Retries:  15,
			},
			expectError: true,
			errorMsg:    "too many retries",
		},
		{
			name: "negative retries",
			request: &DiscoveryRequest{
				Name:     "Test Discovery",
				Networks: []string{"192.168.1.0/24"},
				Method:   "ping",
				Retries:  -1,
			},
			expectError: true,
			errorMsg:    "cannot be negative",
		},
		{
			name: "valid request with tags",
			request: &DiscoveryRequest{
				Name:     "Tagged Discovery",
				Networks: []string{"192.168.1.0/24"},
				Method:   "ping",
				Tags:     []string{"production", "internal"},
				Enabled:  true,
			},
			expectError: false,
		},
		{
			name: "tag too long",
			request: &DiscoveryRequest{
				Name:     "Test Discovery",
				Networks: []string{"192.168.1.0/24"},
				Method:   "ping",
				Tags:     []string{string(make([]byte, 51))},
			},
			expectError: true,
			errorMsg:    "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateDiscoveryRequest(tt.request)
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

func TestDiscoveryHandler_ValidateBasicFields(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		request     *DiscoveryRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid basic fields",
			request: &DiscoveryRequest{
				Name:        "Test Discovery",
				Description: "Test description",
				Networks:    []string{"192.168.1.0/24"},
			},
			expectError: false,
		},
		{
			name: "empty name",
			request: &DiscoveryRequest{
				Name:     "",
				Networks: []string{"192.168.1.0/24"},
			},
			expectError: true,
			errorMsg:    "name is required",
		},
		{
			name: "empty networks",
			request: &DiscoveryRequest{
				Name:     "Test",
				Networks: []string{},
			},
			expectError: true,
			errorMsg:    "at least one network is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateBasicFields(tt.request)
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

func TestDiscoveryHandler_ValidateMethod(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		method      string
		expectError bool
	}{
		{
			name:        "valid ping method",
			method:      "ping",
			expectError: false,
		},
		{
			name:        "valid arp method",
			method:      "arp",
			expectError: false,
		},
		{
			name:        "valid icmp method",
			method:      "icmp",
			expectError: false,
		},
		{
			name:        "valid tcp_connect method",
			method:      "tcp_connect",
			expectError: false,
		},
		{
			name:        "invalid method",
			method:      "invalid",
			expectError: true,
		},
		{
			name:        "empty method",
			method:      "",
			expectError: true,
		},
		{
			name:        "case sensitive method",
			method:      "PING",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateMethod(tt.method)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDiscoveryHandler_ValidateNetworks(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		networks    []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid single network",
			networks:    []string{"192.168.1.0/24"},
			expectError: false,
		},
		{
			name:        "valid multiple networks",
			networks:    []string{"192.168.1.0/24", "10.0.0.0/8"},
			expectError: false,
		},
		{
			name:        "valid /32 network",
			networks:    []string{"192.168.1.1/32"},
			expectError: false,
		},

		{
			name:        "invalid CIDR - prefix too large",
			networks:    []string{"192.168.1.0/33"},
			expectError: true,
			errorMsg:    "invalid format",
		},

		{
			name:        "invalid IP address",
			networks:    []string{"not.an.ip.address/24"},
			expectError: true,
			errorMsg:    "invalid format",
		},
		{
			name:        "mix of valid and invalid",
			networks:    []string{"192.168.1.0/24", "invalid/24"},
			expectError: true,
			errorMsg:    "invalid format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateNetworks(tt.networks)
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

func TestDiscoveryHandler_ValidateLimits(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		retries     int
		timeout     time.Duration
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid limits",
			retries:     3,
			timeout:     5 * time.Second,
			expectError: false,
		},
		{
			name:        "zero retries",
			retries:     0,
			timeout:     5 * time.Second,
			expectError: false,
		},
		{
			name:        "max retries",
			retries:     10,
			timeout:     5 * time.Second,
			expectError: false,
		},
		{
			name:        "retries too high",
			retries:     11,
			timeout:     5 * time.Second,
			expectError: true,
			errorMsg:    "too many retries",
		},
		{
			name:        "negative retries",
			retries:     -1,
			timeout:     5 * time.Second,
			expectError: true,
			errorMsg:    "cannot be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateLimits(tt.timeout, tt.retries)
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

func TestDiscoveryHandler_ValidateTags(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		tags        []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid tags",
			tags:        []string{"production", "internal"},
			expectError: false,
		},
		{
			name:        "empty tags",
			tags:        []string{},
			expectError: false,
		},
		{
			name:        "nil tags",
			tags:        nil,
			expectError: false,
		},
		{
			name:        "single tag",
			tags:        []string{"test"},
			expectError: false,
		},
		{
			name:        "tag at max length",
			tags:        []string{string(make([]byte, 50))},
			expectError: false,
		},
		{
			name:        "tag too long",
			tags:        []string{string(make([]byte, 51))},
			expectError: true,
			errorMsg:    "too long",
		},
		{
			name:        "multiple tags with one too long",
			tags:        []string{"valid", string(make([]byte, 51))},
			expectError: true,
			errorMsg:    "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateTags(tt.tags)
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

func TestDiscoveryHandler_GetDiscoveryFilters(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	tests := []struct {
		name           string
		queryParams    map[string]string
		expectedFilter db.DiscoveryFilters
	}{
		{
			name:        "no filters",
			queryParams: map[string]string{},
			expectedFilter: db.DiscoveryFilters{
				Method: "",
				Status: "",
			},
		},
		{
			name: "method filter",
			queryParams: map[string]string{
				"method": "ping",
			},
			expectedFilter: db.DiscoveryFilters{
				Method: "ping",
				Status: "",
			},
		},
		{
			name: "status filter",
			queryParams: map[string]string{
				"status": "active",
			},
			expectedFilter: db.DiscoveryFilters{
				Method: "",
				Status: "active",
			},
		},
		{
			name: "multiple filters",
			queryParams: map[string]string{
				"method": "arp",
				"status": "completed",
			},
			expectedFilter: db.DiscoveryFilters{
				Method: "arp",
				Status: "completed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/discovery", http.NoBody)
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()

			filters := handler.getDiscoveryFilters(req)
			assert.Equal(t, tt.expectedFilter.Method, filters.Method)
			assert.Equal(t, tt.expectedFilter.Status, filters.Status)
		})
	}
}

func TestDiscoveryHandler_RequestToDBDiscovery(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	tests := []struct {
		name     string
		request  *DiscoveryRequest
		validate func(t *testing.T, data interface{})
	}{
		{
			name: "basic request",
			request: &DiscoveryRequest{
				Name:     "Test Discovery",
				Networks: []string{"192.168.1.0/24"},
				Method:   "ping",
				Enabled:  true,
			},
			validate: func(t *testing.T, data interface{}) {
				m := data.(map[string]interface{})
				assert.Equal(t, "Test Discovery", m["name"])
				assert.Equal(t, "ping", m["method"])
				assert.Equal(t, true, m["enabled"])
				networks, ok := m["networks"].([]string)
				require.True(t, ok)
				assert.Equal(t, []string{"192.168.1.0/24"}, networks)
			},
		},
		{
			name: "request with all fields",
			request: &DiscoveryRequest{
				Name:        "Full Discovery",
				Description: "Complete test",
				Networks:    []string{"192.168.1.0/24", "10.0.0.0/8"},
				Method:      "tcp_connect",
				Ports:       "80,443",
				Timeout:     5 * time.Second,
				Retries:     3,
				Options:     map[string]string{"key": "value"},
				Tags:        []string{"test", "prod"},
				Enabled:     true,
			},
			validate: func(t *testing.T, data interface{}) {
				m := data.(map[string]interface{})
				assert.Equal(t, "Full Discovery", m["name"])
				assert.Equal(t, "Complete test", m["description"])
				assert.Equal(t, "tcp_connect", m["method"])
				assert.Equal(t, "80,443", m["ports"])
				assert.Equal(t, 5*time.Second, m["timeout"])
				assert.Equal(t, 3, m["retries"])
				assert.Equal(t, true, m["enabled"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := handler.requestToDBDiscovery(tt.request)
			tt.validate(t, result)
		})
	}
}

func TestDiscoveryHandler_CreateDiscoveryJob_ValidationErrors(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	tests := []struct {
		name           string
		requestBody    interface{}
		expectedStatus int
	}{
		{
			name: "empty name",
			requestBody: map[string]interface{}{
				"name":     "",
				"networks": []string{"192.168.1.0/24"},
				"method":   "ping",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid method",
			requestBody: map[string]interface{}{
				"name":     "Test",
				"networks": []string{"192.168.1.0/24"},
				"method":   "invalid_method",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "empty networks",
			requestBody: map[string]interface{}{
				"name":     "Test",
				"networks": []string{},
				"method":   "ping",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid CIDR",
			requestBody: map[string]interface{}{
				"name":     "Test",
				"networks": []string{"192.168.1.0/33"},
				"method":   "ping",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.requestBody)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/api/v1/discovery", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreateDiscoveryJob(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestDiscoveryHandler_DiscoveryToResponse_Completed(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	jobID := uuid.New()
	_, ipnet, _ := net.ParseCIDR("192.168.1.0/24")
	startedAt := time.Now().Add(-5 * time.Minute)
	completedAt := time.Now()
	now := time.Now().UTC()

	job := &db.DiscoveryJob{
		ID:              jobID,
		Network:         db.NetworkAddr{IPNet: *ipnet},
		Method:          "ping",
		StartedAt:       &startedAt,
		CompletedAt:     &completedAt,
		HostsDiscovered: 25,
		HostsResponsive: 20,
		Status:          "completed",
		CreatedAt:       now,
	}

	response := handler.discoveryToResponse(job)

	assert.Equal(t, jobID, response.ID)
	assert.Equal(t, []string{"192.168.1.0/24"}, response.Networks)
	assert.Equal(t, "ping", response.Method)
	assert.Equal(t, "completed", response.Status)
	assert.Equal(t, 100.0, response.Progress)
	assert.Equal(t, 25, response.HostsFound)
	assert.True(t, response.Enabled)
	assert.Equal(t, now, response.CreatedAt)
	assert.Equal(t, &startedAt, response.LastRun)
}

func TestDiscoveryHandler_DiscoveryToResponse_Running(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	_, ipnet, _ := net.ParseCIDR("10.0.0.0/8")
	startedAt := time.Now().Add(-1 * time.Minute)

	job := &db.DiscoveryJob{
		ID:              uuid.New(),
		Network:         db.NetworkAddr{IPNet: *ipnet},
		Method:          "arp",
		StartedAt:       &startedAt,
		HostsDiscovered: 5,
		Status:          "running",
		CreatedAt:       time.Now().UTC(),
	}

	response := handler.discoveryToResponse(job)

	assert.Equal(t, "running", response.Status)
	assert.Equal(t, 50.0, response.Progress)
	assert.Equal(t, 5, response.HostsFound)
	assert.True(t, response.Enabled)
	assert.Equal(t, &startedAt, response.LastRun)
}

func TestDiscoveryHandler_DiscoveryToResponse_Pending(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	_, ipnet, _ := net.ParseCIDR("172.16.0.0/16")

	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		Network:   db.NetworkAddr{IPNet: *ipnet},
		Method:    "tcp_connect",
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
	}

	response := handler.discoveryToResponse(job)

	assert.Equal(t, "pending", response.Status)
	assert.Equal(t, 0.0, response.Progress)
	assert.Equal(t, 0, response.HostsFound)
	assert.True(t, response.Enabled)
	assert.Nil(t, response.LastRun)
}

func TestDiscoveryHandler_DiscoveryToResponse_Failed(t *testing.T) {
	logger := createTestLogger()
	handler := NewDiscoveryHandler(nilDiscoveryStore{}, logger, metrics.NewRegistry())

	_, ipnet, _ := net.ParseCIDR("192.168.1.0/24")

	job := &db.DiscoveryJob{
		ID:        uuid.New(),
		Network:   db.NetworkAddr{IPNet: *ipnet},
		Method:    "ping",
		Status:    "failed",
		CreatedAt: time.Now().UTC(),
	}

	response := handler.discoveryToResponse(job)

	assert.Equal(t, "failed", response.Status)
	assert.Equal(t, 0.0, response.Progress)
	assert.False(t, response.Enabled) // failed jobs are not enabled
}
