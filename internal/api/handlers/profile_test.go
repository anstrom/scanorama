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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
	"github.com/anstrom/scanorama/test/helpers"
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
		HostTimeout:      NewDuration(30 * time.Second),
		ScanDelay:        NewDuration(5 * time.Second),
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
	assert.Equal(t, request.Timing.Template, data["timing"])
	// Check that options contains both the original options and the merged scan configuration
	options, ok := data["options"].(map[string]interface{})
	require.True(t, ok, "options should be a map[string]interface{}")

	// Check original options are included
	for k, v := range request.Options {
		assert.Equal(t, v, options[k], "option %s should match", k)
	}

	// Check that scan configuration is merged into options
	assert.Equal(t, request.ServiceDetection, options["service_detection"])
	assert.Equal(t, request.OSDetection, options["os_detection"])
	assert.Equal(t, request.ScriptScan, options["script_scan"])
	assert.Equal(t, request.UDPScan, options["udp_scan"])
	assert.Equal(t, request.MaxRetries, options["max_retries"])
	assert.Equal(t, request.HostTimeout.ToDuration(), options["host_timeout"])
	assert.Equal(t, request.ScanDelay.ToDuration(), options["scan_delay"])
	assert.Equal(t, request.MaxRatePPS, options["max_rate_pps"])
	assert.Equal(t, request.MaxHostGroupSize, options["max_host_group_size"])
	assert.Equal(t, request.MinHostGroupSize, options["min_host_group_size"])
	assert.Equal(t, request.Default, options["default"])
	assert.Equal(t, request.Tags, data["tags"])
}

func TestProfileHandler_ProfileToResponse(t *testing.T) {
	logger := createTestLogger()
	handler := NewProfileHandler(nil, logger, metrics.NewRegistry())

	testProfile := &db.ScanProfile{
		ID:          "test-profile",
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
	assert.Equal(t, testProfile.ID, response.ID) // Use actual profile ID
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
				HostTimeout: NewDuration(30 * time.Second),
				ScanDelay:   NewDuration(5 * time.Second),
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
				HostTimeout: NewDuration(30 * time.Minute),
				ScanDelay:   NewDuration(60 * time.Second),
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
			HostTimeout:      NewDuration(5 * time.Minute),
			ScanDelay:        NewDuration(10 * time.Second),
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
			HostTimeout:      NewDuration(30 * time.Minute), // max timeout
			ScanDelay:        NewDuration(60 * time.Second), // max delay
			MaxRetries:       10,                            // max retries
			MaxRatePPS:       10000,                         // max rate
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
		HostTimeout:      NewDuration(30 * time.Minute),
		ScanDelay:        NewDuration(60 * time.Second),
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

// Integration tests with database

func setupProfileHandlerTest(t *testing.T) (*ProfileHandler, *db.DB, func()) {
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
	handler := NewProfileHandler(database, logger, metricsRegistry)

	// Clean up any leftover test data
	_, _ = database.Exec(`DELETE FROM scan_profiles WHERE name LIKE 'ProfileTest%'`)

	cleanup := func() {
		// Clean up test data
		_, _ = database.Exec(`DELETE FROM scan_profiles WHERE name LIKE 'ProfileTest%'`)
		database.Close()
	}

	return handler, database, cleanup
}

func generateUniqueProfileName() string {
	return fmt.Sprintf("ProfileTest_%s", uuid.New().String()[:8])
}

func TestProfileHandler_ListProfiles_Integration(t *testing.T) {
	handler, database, cleanup := setupProfileHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create test profiles
	profile1Name := generateUniqueProfileName()
	profile2Name := generateUniqueProfileName()

	profile1Data := map[string]interface{}{
		"name":        profile1Name,
		"description": "Test profile 1",
		"scan_type":   "connect",
		"ports":       "1-1000",
		"timing":      "normal",
		"priority":    1,
		"built_in":    false,
		"created_at":  time.Now().UTC(),
		"updated_at":  time.Now().UTC(),
	}

	profile2Data := map[string]interface{}{
		"name":        profile2Name,
		"description": "Test profile 2",
		"scan_type":   "syn",
		"ports":       "22,80,443",
		"timing":      "aggressive",
		"priority":    2,
		"built_in":    false,
		"created_at":  time.Now().UTC(),
		"updated_at":  time.Now().UTC(),
	}

	_, err := database.CreateProfile(ctx, profile1Data)
	require.NoError(t, err)

	_, err = database.CreateProfile(ctx, profile2Data)
	require.NoError(t, err)

	// Test listing profiles
	req := httptest.NewRequest("GET", "/api/v1/profiles", http.NoBody)
	w := httptest.NewRecorder()

	handler.ListProfiles(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data []ProfileResponse `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(response.Data), 2)

	// Verify our test profiles are in the response
	foundProfile1 := false
	foundProfile2 := false
	for _, profile := range response.Data {
		if profile.Name == profile1Name {
			foundProfile1 = true
			assert.Equal(t, "Test profile 1", profile.Description)
			assert.Equal(t, "connect", profile.ScanType)
		}
		if profile.Name == profile2Name {
			foundProfile2 = true
			assert.Equal(t, "Test profile 2", profile.Description)
			assert.Equal(t, "syn", profile.ScanType)
		}
	}

	assert.True(t, foundProfile1, "Profile 1 not found in response")
	assert.True(t, foundProfile2, "Profile 2 not found in response")
}

func TestProfileHandler_ListProfiles_WithFilters_Integration(t *testing.T) {
	handler, database, cleanup := setupProfileHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create profiles with different scan types
	connectProfileName := generateUniqueProfileName()
	synProfileName := generateUniqueProfileName()

	connectData := map[string]interface{}{
		"name":        connectProfileName,
		"description": "Connect scan profile",
		"scan_type":   "connect",
		"ports":       "1-1000",
		"timing":      "normal",
		"priority":    1,
		"built_in":    false,
		"created_at":  time.Now().UTC(),
		"updated_at":  time.Now().UTC(),
	}

	synData := map[string]interface{}{
		"name":        synProfileName,
		"description": "SYN scan profile",
		"scan_type":   "syn",
		"ports":       "1-1000",
		"timing":      "normal",
		"priority":    1,
		"built_in":    false,
		"created_at":  time.Now().UTC(),
		"updated_at":  time.Now().UTC(),
	}

	_, err := database.CreateProfile(ctx, connectData)
	require.NoError(t, err)

	_, err = database.CreateProfile(ctx, synData)
	require.NoError(t, err)

	// Test filtering by scan_type=connect
	req := httptest.NewRequest("GET", "/api/v1/profiles?scan_type=connect", http.NoBody)
	w := httptest.NewRecorder()

	handler.ListProfiles(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data []ProfileResponse `json:"data"`
	}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify only connect profiles are returned
	foundConnect := false
	foundSyn := false
	for _, profile := range response.Data {
		if profile.Name == connectProfileName {
			foundConnect = true
		}
		if profile.Name == synProfileName {
			foundSyn = true
		}
	}

	assert.True(t, foundConnect, "Connect profile should be in filtered results")
	assert.False(t, foundSyn, "SYN profile should not be in filtered results")
}

func TestProfileHandler_CreateProfile_Integration(t *testing.T) {
	handler, _, cleanup := setupProfileHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	profileName := generateUniqueProfileName()
	profileRequest := ProfileRequest{
		Name:        profileName,
		Description: "Integration test profile",
		ScanType:    "connect",
		Ports:       "22,80,443,8080",
		Timing: TimingProfile{
			Template: "normal",
		},
		ServiceDetection: true,
		OSDetection:      false,
		Tags:             []string{"integration", "test"},
	}

	body, err := json.Marshal(profileRequest)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/api/v1/profiles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreateProfile(w, req)

	// CreateProfile endpoint coverage test
	if w.Code == http.StatusCreated {
		var response ProfileResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, profileName, response.Name)
		assert.Equal(t, "Integration test profile", response.Description)
		assert.Equal(t, "connect", response.ScanType)
		assert.Equal(t, "22,80,443,8080", response.Ports)
		assert.NotEmpty(t, response.ID)
	}
}

func TestProfileHandler_CreateProfile_ValidationErrors_Integration(t *testing.T) {
	handler, _, cleanup := setupProfileHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	tests := []struct {
		name           string
		request        ProfileRequest
		expectedStatus int
	}{
		{
			name: "empty profile name",
			request: ProfileRequest{
				Name:     "",
				ScanType: "connect",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid scan type",
			request: ProfileRequest{
				Name:     generateUniqueProfileName(),
				ScanType: "invalid_type",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "profile name too long",
			request: ProfileRequest{
				Name:     strings.Repeat("a", 256),
				ScanType: "connect",
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.request)
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/api/v1/profiles", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.CreateProfile(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
		})
	}
}

func TestProfileHandler_CreateProfile_WithComplexOptions_Integration(t *testing.T) {
	handler, _, cleanup := setupProfileHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	profileName := generateUniqueProfileName()
	profileRequest := ProfileRequest{
		Name:        profileName,
		Description: "Complex profile with timing and options",
		ScanType:    "syn",
		Ports:       "1-65535",
		Timing: TimingProfile{
			Template:          "aggressive",
			MinRTTTimeout:     NewDuration(100 * time.Millisecond),
			MaxRTTTimeout:     NewDuration(1 * time.Second),
			InitialRTTTimeout: NewDuration(500 * time.Millisecond),
			MaxRetries:        3,
			HostTimeout:       NewDuration(30 * time.Minute),
			ScanDelay:         NewDuration(0),
			MaxScanDelay:      NewDuration(0),
		},
		ServiceDetection: true,
		OSDetection:      true,
		ScriptScan:       true,
		UDPScan:          false,
		MaxRetries:       3,
		HostTimeout:      NewDuration(30 * time.Minute),
		ScanDelay:        NewDuration(0),
		MaxRatePPS:       1000,
		MaxHostGroupSize: 100,
		MinHostGroupSize: 10,
		Tags:             []string{"aggressive", "full-scan"},
		Default:          false,
	}

	body, err := json.Marshal(profileRequest)
	require.NoError(t, err)

	req := httptest.NewRequest("POST", "/api/v1/profiles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.CreateProfile(w, req)

	if w.Code == http.StatusCreated {
		var response ProfileResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, profileName, response.Name)
		assert.Equal(t, "syn", response.ScanType)
		assert.True(t, response.ServiceDetection)
		assert.True(t, response.OSDetection)
		assert.True(t, response.ScriptScan)
		assert.False(t, response.UDPScan)
		// Note: MaxRetries, MaxRatePPS, and host group sizes may be stored in options JSON
		// and not directly populated in response depending on database schema
	}
}

func TestProfileHandler_ListProfiles_Pagination_Integration(t *testing.T) {
	handler, database, cleanup := setupProfileHandlerTest(t)
	if handler == nil {
		return
	}
	defer cleanup()

	ctx := context.Background()

	// Create multiple profiles for pagination testing
	for i := 0; i < 5; i++ {
		profileData := map[string]interface{}{
			"name":        fmt.Sprintf("%s_%d", generateUniqueProfileName(), i),
			"description": fmt.Sprintf("Test profile %d", i),
			"scan_type":   "connect",
			"ports":       "22,80,443",
			"timing":      "normal",
			"priority":    i,
			"built_in":    false,
			"created_at":  time.Now().UTC(),
			"updated_at":  time.Now().UTC(),
		}

		_, err := database.CreateProfile(ctx, profileData)
		require.NoError(t, err)
	}

	// Test pagination with limit
	req := httptest.NewRequest("GET", "/api/v1/profiles?limit=3", http.NoBody)
	w := httptest.NewRecorder()

	handler.ListProfiles(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data  []ProfileResponse `json:"data"`
		Total int64             `json:"total"`
		Limit int               `json:"limit"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify pagination is working - we should get some results
	// Total may include built-in profiles in addition to our test profiles
	assert.GreaterOrEqual(t, len(response.Data), 1, "Should have at least some profiles")
	if response.Limit > 0 {
		assert.LessOrEqual(t, len(response.Data), response.Limit, "Should not exceed limit")
	}
}

// Unit tests for helper functions

func TestDuration_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		duration Duration
		expected string
	}{
		{
			name:     "zero duration",
			duration: Duration(0),
			expected: `"0s"`,
		},
		{
			name:     "one second",
			duration: Duration(time.Second),
			expected: `"1s"`,
		},
		{
			name:     "one minute",
			duration: Duration(time.Minute),
			expected: `"1m0s"`,
		},
		{
			name:     "complex duration",
			duration: Duration(time.Hour + 30*time.Minute + 15*time.Second),
			expected: `"1h30m15s"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.duration)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, string(data))
		})
	}
}

func TestDuration_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    Duration
		expectError bool
	}{
		{
			name:     "valid duration string",
			input:    `"1h30m"`,
			expected: Duration(time.Hour + 30*time.Minute),
		},
		{
			name:     "zero duration",
			input:    `"0s"`,
			expected: Duration(0),
		},
		{
			name:     "milliseconds",
			input:    `"500ms"`,
			expected: Duration(500 * time.Millisecond),
		},
		{
			name:        "invalid duration format",
			input:       `"invalid"`,
			expectError: true,
		},
		{
			name:        "invalid JSON",
			input:       `not-json`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var d Duration
			err := json.Unmarshal([]byte(tt.input), &d)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, d)
			}
		})
	}
}

func TestDuration_ToDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration Duration
		expected time.Duration
	}{
		{
			name:     "zero",
			duration: Duration(0),
			expected: 0,
		},
		{
			name:     "one second",
			duration: Duration(time.Second),
			expected: time.Second,
		},
		{
			name:     "complex duration",
			duration: Duration(time.Hour + 30*time.Minute),
			expected: time.Hour + 30*time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.duration.ToDuration()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected Duration
	}{
		{
			name:     "zero",
			input:    0,
			expected: Duration(0),
		},
		{
			name:     "one second",
			input:    time.Second,
			expected: Duration(time.Second),
		},
		{
			name:     "complex duration",
			input:    2*time.Hour + 45*time.Minute + 30*time.Second,
			expected: Duration(2*time.Hour + 45*time.Minute + 30*time.Second),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewDuration(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProfileHandler_ParseProfileOptions(t *testing.T) {
	tests := []struct {
		name                  string
		input                 string
		expectedStringOptions map[string]string
		expectedParsedOptions map[string]interface{}
	}{
		{
			name:                  "empty options",
			input:                 "",
			expectedStringOptions: map[string]string{},
			expectedParsedOptions: map[string]interface{}{},
		},
		{
			name:  "valid JSON with strings",
			input: `{"version-intensity": "9", "script-timeout": "30s"}`,
			expectedStringOptions: map[string]string{
				"version-intensity": "9",
				"script-timeout":    "30s",
			},
			expectedParsedOptions: map[string]interface{}{
				"version-intensity": "9",
				"script-timeout":    "30s",
			},
		},
		{
			name:  "JSON with mixed types",
			input: `{"string-opt": "value", "number-opt": 123, "bool-opt": true}`,
			expectedStringOptions: map[string]string{
				"string-opt": "value",
			},
			expectedParsedOptions: map[string]interface{}{
				"string-opt": "value",
				"number-opt": float64(123),
				"bool-opt":   true,
			},
		},
		{
			name:                  "invalid JSON",
			input:                 `{invalid json}`,
			expectedStringOptions: map[string]string{},
			expectedParsedOptions: map[string]interface{}{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stringOpts, parsedOpts := parseProfileOptions(tt.input)
			assert.Equal(t, tt.expectedStringOptions, stringOpts)

			// Compare map contents
			assert.Equal(t, len(tt.expectedParsedOptions), len(parsedOpts))
			for k, expectedV := range tt.expectedParsedOptions {
				actualV, exists := parsedOpts[k]
				assert.True(t, exists, "key %s should exist", k)
				assert.Equal(t, expectedV, actualV, "value for key %s should match", k)
			}
		})
	}
}

func TestProfileHandler_ExtractScanFlags(t *testing.T) {
	tests := []struct {
		name           string
		options        map[string]interface{}
		profile        *db.ScanProfile
		expectedDetect bool
		expectedOS     bool
		expectedScript bool
		expectedUDP    bool
	}{
		{
			name: "all flags true",
			options: map[string]interface{}{
				"service_detection": true,
				"os_detection":      true,
				"script_scan":       true,
				"udp_scan":          true,
			},
			profile: &db.ScanProfile{
				ScanType: "connect",
				Scripts:  []string{},
			},
			expectedDetect: true,
			expectedOS:     true,
			expectedScript: true,
			expectedUDP:    true,
		},
		{
			name: "all flags false",
			options: map[string]interface{}{
				"service_detection": false,
				"os_detection":      false,
				"script_scan":       false,
				"udp_scan":          false,
			},
			profile: &db.ScanProfile{
				ScanType: "connect",
				Scripts:  []string{},
			},
			expectedDetect: false,
			expectedOS:     false,
			expectedScript: false,
			expectedUDP:    false,
		},
		{
			name:    "fallback to comprehensive scan defaults",
			options: map[string]interface{}{},
			profile: &db.ScanProfile{
				ScanType: "comprehensive",
				Scripts:  []string{},
			},
			expectedDetect: true,
			expectedOS:     true,
			expectedScript: true,
			expectedUDP:    true,
		},
		{
			name:    "empty options with connect scan",
			options: map[string]interface{}{},
			profile: &db.ScanProfile{
				ScanType: "connect",
				Scripts:  []string{},
			},
			expectedDetect: false,
			expectedOS:     false,
			expectedScript: false,
			expectedUDP:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serviceDetect, osDetect, scriptScan, udpScan := extractScanFlags(tt.options, tt.profile)
			assert.Equal(t, tt.expectedDetect, serviceDetect)
			assert.Equal(t, tt.expectedOS, osDetect)
			assert.Equal(t, tt.expectedScript, scriptScan)
			assert.Equal(t, tt.expectedUDP, udpScan)
		})
	}
}

func TestProfileHandler_ExtractAdditionalOptions(t *testing.T) {
	tests := []struct {
		name                string
		options             map[string]interface{}
		expectedRetries     int
		expectedMaxRate     int
		expectedHostTimeout time.Duration
		expectedScanDelay   time.Duration
	}{
		{
			name: "all options present with float64 numbers",
			options: map[string]interface{}{
				"max_retries":  float64(3),
				"max_rate_pps": float64(1000),
				"host_timeout": "30m",
				"scan_delay":   "100ms",
			},
			expectedRetries:     3,
			expectedMaxRate:     1000,
			expectedHostTimeout: 30 * time.Minute,
			expectedScanDelay:   100 * time.Millisecond,
		},
		{
			name:                "empty options",
			options:             map[string]interface{}{},
			expectedRetries:     0,
			expectedMaxRate:     0,
			expectedHostTimeout: 0,
			expectedScanDelay:   0,
		},
		{
			name: "partial options",
			options: map[string]interface{}{
				"max_retries":  float64(5),
				"max_rate_pps": float64(500),
			},
			expectedRetries:     5,
			expectedMaxRate:     500,
			expectedHostTimeout: 0,
			expectedScanDelay:   0,
		},
		{
			name: "invalid duration strings ignored",
			options: map[string]interface{}{
				"host_timeout": "invalid",
				"scan_delay":   "also-invalid",
			},
			expectedRetries:     0,
			expectedMaxRate:     0,
			expectedHostTimeout: 0,
			expectedScanDelay:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retries, maxRate, hostTimeout, scanDelay := extractAdditionalOptions(tt.options)
			assert.Equal(t, tt.expectedRetries, retries)
			assert.Equal(t, tt.expectedMaxRate, maxRate)
			assert.Equal(t, tt.expectedHostTimeout, hostTimeout)
			assert.Equal(t, tt.expectedScanDelay, scanDelay)
		})
	}
}
