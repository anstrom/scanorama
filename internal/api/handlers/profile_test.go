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

func TestNewProfileHandler(t *testing.T) {
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
			handler := NewProfileHandler(tt.database, logger, tt.metrics)

			assert.NotNil(t, handler)
			assert.NotNil(t, handler.logger)
			assert.Equal(t, tt.metrics, handler.metrics)
		})
	}
}

func TestProfileHandler_ValidateProfileRequest(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		request     *ProfileRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request",
			request: &ProfileRequest{
				Name:     "Test Profile",
				ScanType: "connect",
				Ports:    "1-1000",
			},
			expectError: false,
		},
		{
			name: "empty name",
			request: &ProfileRequest{
				Name:     "",
				ScanType: "connect",
			},
			expectError: true,
			errorMsg:    "profile name is required",
		},
		{
			name: "name too long",
			request: &ProfileRequest{
				Name:     strings.Repeat("a", 256),
				ScanType: "connect",
			},
			expectError: true,
			errorMsg:    "profile name too long",
		},
		{
			name: "invalid scan type",
			request: &ProfileRequest{
				Name:     "Test Profile",
				ScanType: "invalid",
			},
			expectError: true,
			errorMsg:    "invalid scan type",
		},
		{
			name: "description too long",
			request: &ProfileRequest{
				Name:        "Test Profile",
				Description: strings.Repeat("a", 1001),
				ScanType:    "connect",
			},
			expectError: true,
			errorMsg:    "description too long",
		},
		{
			name: "valid syn scan type",
			request: &ProfileRequest{
				Name:     "SYN Profile",
				ScanType: "syn",
			},
			expectError: false,
		},
		{
			name: "valid ack scan type",
			request: &ProfileRequest{
				Name:     "ACK Profile",
				ScanType: "ack",
			},
			expectError: false,
		},
		{
			name: "valid aggressive scan type",
			request: &ProfileRequest{
				Name:     "Aggressive Profile",
				ScanType: "aggressive",
			},
			expectError: false,
		},
		{
			name: "valid comprehensive scan type",
			request: &ProfileRequest{
				Name:     "Comprehensive Profile",
				ScanType: "comprehensive",
			},
			expectError: false,
		},
		{
			name: "maximum valid name length",
			request: &ProfileRequest{
				Name:     strings.Repeat("a", 255),
				ScanType: "connect",
			},
			expectError: false,
		},
		{
			name: "maximum valid description length",
			request: &ProfileRequest{
				Name:        "Test Profile",
				Description: strings.Repeat("a", 1000),
				ScanType:    "connect",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateProfileRequest(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProfileHandler_GetProfileFilters(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name           string
		queryParams    string
		expectedFilter db.ProfileFilters
	}{
		{
			name:           "no filters",
			queryParams:    "",
			expectedFilter: db.ProfileFilters{},
		},
		{
			name:        "scan type filter",
			queryParams: "?scan_type=syn",
			expectedFilter: db.ProfileFilters{
				ScanType: "syn",
			},
		},
		{
			name:        "multiple filters",
			queryParams: "?scan_type=aggressive",
			expectedFilter: db.ProfileFilters{
				ScanType: "aggressive",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/profiles"+tt.queryParams, http.NoBody)
			filters := handler.getProfileFilters(req)

			assert.Equal(t, tt.expectedFilter.ScanType, filters.ScanType)
		})
	}
}

func TestProfileHandler_RequestToDBProfile(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	request := &ProfileRequest{
		Name:        "Test Profile",
		Description: "Test profile description",
		ScanType:    "comprehensive",
		Ports:       "1-65535",
		Timing: TimingProfile{
			Template: "normal",
		},
		HostTimeout:      30 * time.Second,
		ScanDelay:        5 * time.Second,
		MaxRetries:       3,
		MaxRatePPS:       1000,
		MinHostGroupSize: 10,
		MaxHostGroupSize: 100,
		Options:          map[string]string{"version-intensity": "9"},
		Tags:             []string{"comprehensive", "slow"},
	}

	result := handler.requestToDBProfile(request)
	data, ok := result.(map[string]interface{})
	require.True(t, ok)

	assert.Equal(t, request.Name, data["name"])
	assert.Equal(t, request.Description, data["description"])
	assert.Equal(t, request.ScanType, data["scan_type"])
	assert.Equal(t, request.Ports, data["ports"])
	assert.Equal(t, request.Timing, data["timing"])
	assert.Equal(t, request.HostTimeout, data["host_timeout"])
	assert.Equal(t, request.ScanDelay, data["scan_delay"])
	assert.Equal(t, request.MaxRetries, data["max_retries"])
	assert.Equal(t, request.MaxRatePPS, data["max_rate_pps"])
	assert.Equal(t, request.MinHostGroupSize, data["min_host_group_size"])
	assert.Equal(t, request.MaxHostGroupSize, data["max_host_group_size"])
	assert.Equal(t, request.Options, data["options"])
	assert.Equal(t, request.Tags, data["tags"])
}

func TestProfileHandler_ProfileToResponse(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	testProfile := &db.Profile{
		ID:          uuid.New(),
		Name:        "Test Profile",
		Description: "Test profile description",
		ScanType:    "comprehensive",
		Ports:       "1-65535",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	response := handler.profileToResponse(testProfile)

	// Note: The current profileToResponse returns placeholder data
	// These assertions test the function call works, not actual mapping
	assert.NotNil(t, response)
	assert.Equal(t, int64(1), response.ID) // placeholder value
}

func TestProfileHandler_CreateProfile_ValidationErrors(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		requestBody interface{}
	}{
		{
			name: "validation error - empty name",
			requestBody: ProfileRequest{
				Name:     "",
				ScanType: "connect",
			},
		},
		{
			name: "validation error - invalid scan type",
			requestBody: ProfileRequest{
				Name:     "Test Profile",
				ScanType: "invalid",
			},
		},
		{
			name: "validation error - name too long",
			requestBody: ProfileRequest{
				Name:     strings.Repeat("a", 256),
				ScanType: "connect",
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

			req := httptest.NewRequest("POST", "/api/v1/profiles", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreateProfile(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	}
}

func TestProfileHandler_GetProfile_InvalidID(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/profiles/invalid-id", http.NoBody)
	req.SetPathValue("id", "invalid-id")
	w := httptest.NewRecorder()

	handler.GetProfile(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProfileHandler_UpdateProfile_InvalidID(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	updateRequest := ProfileRequest{
		Name:     "Updated Profile",
		ScanType: "syn",
	}
	body, _ := json.Marshal(updateRequest)

	req := httptest.NewRequest("PUT", "/api/v1/profiles/invalid-id", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "invalid-id")
	w := httptest.NewRecorder()

	handler.UpdateProfile(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProfileHandler_DeleteProfile_InvalidID(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("DELETE", "/api/v1/profiles/invalid-id", http.NoBody)
	req.SetPathValue("id", "invalid-id")
	w := httptest.NewRecorder()

	handler.DeleteProfile(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestProfileHandler_ValidateTimingTemplate(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		request     *ProfileRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid paranoid timing",
			request: &ProfileRequest{
				Name:     "Test",
				ScanType: "connect",
				Timing: TimingProfile{
					Template: "paranoid",
				},
			},
			expectError: false,
		},
		{
			name: "valid polite timing",
			request: &ProfileRequest{
				Name:     "Test",
				ScanType: "connect",
				Timing: TimingProfile{
					Template: "polite",
				},
			},
			expectError: false,
		},
		{
			name: "valid normal timing",
			request: &ProfileRequest{
				Name:     "Test",
				ScanType: "connect",
				Timing: TimingProfile{
					Template: "normal",
				},
			},
			expectError: false,
		},
		{
			name: "valid aggressive timing",
			request: &ProfileRequest{
				Name:     "Test",
				ScanType: "connect",
				Timing: TimingProfile{
					Template: "aggressive",
				},
			},
			expectError: false,
		},
		{
			name: "valid insane timing",
			request: &ProfileRequest{
				Name:     "Test",
				ScanType: "connect",
				Timing: TimingProfile{
					Template: "insane",
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateTimingTemplate(tt.request.Timing.Template)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProfileHandler_ValidateProfileTimeouts(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		request     *ProfileRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid timeouts",
			request: &ProfileRequest{
				Name:        "Test",
				ScanType:    "connect",
				HostTimeout: 30 * time.Second,
				ScanDelay:   5 * time.Second,
			},
			expectError: false,
		},
		{
			name: "zero timeout",
			request: &ProfileRequest{
				Name:        "Test",
				ScanType:    "connect",
				HostTimeout: 0,
			},
			expectError: false,
		},
		{
			name: "maximum valid timeout",
			request: &ProfileRequest{
				Name:        "Test",
				ScanType:    "connect",
				HostTimeout: 30 * time.Minute,
				ScanDelay:   60 * time.Second,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateProfileTimeouts(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProfileHandler_ValidateProfileRateLimiting(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		request     *ProfileRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid rate limits",
			request: &ProfileRequest{
				Name:       "Test",
				ScanType:   "connect",
				MaxRatePPS: 1000,
				MaxRetries: 3,
			},
			expectError: false,
		},
		{
			name: "zero rates",
			request: &ProfileRequest{
				Name:       "Test",
				ScanType:   "connect",
				MaxRatePPS: 0,
				MaxRetries: 0,
			},
			expectError: false,
		},
		{
			name: "max rate only",
			request: &ProfileRequest{
				Name:       "Test",
				ScanType:   "connect",
				MaxRatePPS: 5000,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateProfileRateLimiting(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProfileHandler_ValidateHostGroupSizes(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	tests := []struct {
		name        string
		request     *ProfileRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid group sizes",
			request: &ProfileRequest{
				Name:             "Test",
				ScanType:         "connect",
				MinHostGroupSize: 10,
				MaxHostGroupSize: 100,
			},
			expectError: false,
		},
		{
			name: "equal group sizes",
			request: &ProfileRequest{
				Name:             "Test",
				ScanType:         "connect",
				MinHostGroupSize: 50,
				MaxHostGroupSize: 50,
			},
			expectError: false,
		},
		{
			name: "zero group sizes",
			request: &ProfileRequest{
				Name:             "Test",
				ScanType:         "connect",
				MinHostGroupSize: 0,
				MaxHostGroupSize: 0,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.validateHostGroupSizes(tt.request)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProfileHandler_EdgeCases(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	t.Run("scan types validation", func(t *testing.T) {
		validTypes := []string{"connect", "syn", "ack", "aggressive", "comprehensive"}
		for _, scanType := range validTypes {
			req := &ProfileRequest{
				Name:     "Test",
				ScanType: scanType,
			}
			err := handler.validateProfileRequest(req)
			assert.NoError(t, err, "scan type %s should be valid", scanType)
		}
	})

	t.Run("comprehensive profile with all options", func(t *testing.T) {
		req := &ProfileRequest{
			Name:        "Comprehensive Profile",
			Description: "Full featured profile",
			ScanType:    "comprehensive",
			Ports:       "1-65535",
			Timing: TimingProfile{
				Template: "normal",
			},
			HostTimeout:      5 * time.Minute,
			ScanDelay:        10 * time.Second,
			MaxRetries:       5,
			MaxRatePPS:       500,
			MinHostGroupSize: 5,
			MaxHostGroupSize: 50,
			Options:          map[string]string{"version-intensity": "9", "script-timeout": "30"},
			Tags:             []string{"comprehensive", "production"},
		}
		err := handler.validateProfileRequest(req)
		assert.NoError(t, err)
	})
}

func TestProfileHandler_RequestValidation_Comprehensive(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	t.Run("maximum valid profile request", func(t *testing.T) {
		req := &ProfileRequest{
			Name:        strings.Repeat("a", 255),  // max length
			Description: strings.Repeat("b", 1000), // max length
			ScanType:    "comprehensive",
			Ports:       "1-65535",
			Timing: TimingProfile{
				Template: "aggressive",
			},
			HostTimeout:      30 * time.Minute, // max timeout
			ScanDelay:        60 * time.Second, // max delay
			MaxRetries:       10,               // max retries
			MaxRatePPS:       10000,            // max rate
			MinHostGroupSize: 1,
			MaxHostGroupSize: 1000,
			Options:          map[string]string{"key1": "value1", "key2": "value2"},
			Tags:             []string{"tag1", "tag2", "tag3"},
		}

		err := handler.validateProfileRequest(req)
		assert.NoError(t, err)
	})

	t.Run("boundary conditions", func(t *testing.T) {
		// Test exactly at the boundary
		req := &ProfileRequest{
			Name:        strings.Repeat("a", 255),  // exactly max length
			Description: strings.Repeat("b", 1000), // exactly max length
			ScanType:    "connect",
		}
		err := handler.validateProfileRequest(req)
		assert.NoError(t, err)

		// Test just over the boundary
		req.Name = strings.Repeat("a", 256) // one over max length
		err = handler.validateProfileRequest(req)
		assert.Error(t, err)

		// Reset name and test description boundary
		req.Name = "valid-profile"
		req.Description = strings.Repeat("b", 1001) // one over max length
		err = handler.validateProfileRequest(req)
		assert.Error(t, err)
	})
}

func BenchmarkProfileHandler_ValidateProfileRequest(b *testing.B) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	request := &ProfileRequest{
		Name:        "Benchmark Profile",
		Description: "Benchmark profile description",
		ScanType:    "comprehensive",
		Ports:       "1-65535",
		Timing: TimingProfile{
			Template: "normal",
		},
		HostTimeout:      30 * time.Second,
		ScanDelay:        5 * time.Second,
		MaxRetries:       3,
		MaxRatePPS:       1000,
		MinHostGroupSize: 10,
		MaxHostGroupSize: 100,
		Options:          map[string]string{"version-intensity": "9"},
		Tags:             []string{"benchmark", "test"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handler.validateProfileRequest(request)
	}
}

func BenchmarkProfileHandler_GetProfileFilters(b *testing.B) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	req := httptest.NewRequest("GET", "/api/v1/profiles?scan_type=comprehensive", http.NoBody)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = handler.getProfileFilters(req)
	}
}
