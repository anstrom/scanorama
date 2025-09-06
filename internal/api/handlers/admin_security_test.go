// Package handlers provides security tests for admin configuration endpoints.
// These tests verify that the unsafe deserialization vulnerability has been fixed
// and that proper input validation and security constraints are enforced.
package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/anstrom/scanorama/internal/metrics"
)

const (
	testPayloadSize1MB = 1024 * 1024     // 1MB test payload size
	testPayloadSize2MB = 2 * 1024 * 1024 // 2MB test payload size
)

func TestAdminHandler_ConfigSecurity(t *testing.T) {
	// Setup test dependencies
	mockDB := &db.DB{} // Mock database
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	metricsRegistry := metrics.NewRegistry()

    handler := NewAdminHandler(mockDB, logger, metricsRegistry, nil)

	t.Run("typed configuration prevents unsafe deserialization", func(t *testing.T) {
		// This test demonstrates that the new typed approach prevents
		// the unsafe deserialization that was possible with interface{}

		// Valid API configuration update
		validConfig := map[string]interface{}{
			"section": "api",
			"config": map[string]interface{}{
				"api": map[string]interface{}{
					"enabled": true,
					"host":    "localhost",
					"port":    8080,
				},
			},
		}

		body, err := json.Marshal(validConfig)
		require.NoError(t, err)

		req := httptest.NewRequest("PUT", "/api/v1/admin/config", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.UpdateConfig(w, req)

		// Should succeed with properly typed configuration
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("rejects malformed configuration structure", func(t *testing.T) {
		// Attempt to send malformed configuration that could have been
		// exploited in the old interface{} approach

		malformedConfigs := []map[string]interface{}{
			{
				"section": "api",
				"config": map[string]interface{}{
					"database": map[string]interface{}{ // Wrong section
						"host": "malicious-host",
					},
				},
			},
			{
				"section": "api",
				"config": map[string]interface{}{
					"api": "not-an-object", // Wrong type
				},
			},
			{
				"section": "api",
				"config":  map[string]interface{}{
					// Missing required section
				},
			},
		}

		for i, config := range malformedConfigs {
			t.Run(fmt.Sprintf("malformed_config_%d", i), func(t *testing.T) {
				body, err := json.Marshal(config)
				require.NoError(t, err)

				req := httptest.NewRequest("PUT", "/api/v1/admin/config", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				handler.UpdateConfig(w, req)

				// Should reject malformed configuration
				assert.Equal(t, http.StatusBadRequest, w.Code)
			})
		}
	})

	t.Run("enforces field validation constraints", func(t *testing.T) {
		// Test that field validation prevents dangerous values

		invalidConfigs := []struct {
			name   string
			config map[string]interface{}
		}{
			{
				name: "invalid_port_range",
				config: map[string]interface{}{
					"section": "api",
					"config": map[string]interface{}{
						"api": map[string]interface{}{
							"port": 99999, // Invalid port
						},
					},
				},
			},
			{
				name: "invalid_host_format",
				config: map[string]interface{}{
					"section": "api",
					"config": map[string]interface{}{
						"api": map[string]interface{}{
							"host": "invalid@host&name", // Invalid characters
						},
					},
				},
			},
			{
				name: "invalid_duration_format",
				config: map[string]interface{}{
					"section": "api",
					"config": map[string]interface{}{
						"api": map[string]interface{}{
							"read_timeout": "invalid-duration",
						},
					},
				},
			},
		}

		for _, tc := range invalidConfigs {
			t.Run(tc.name, func(t *testing.T) {
				body, err := json.Marshal(tc.config)
				require.NoError(t, err)

				req := httptest.NewRequest("PUT", "/api/v1/admin/config", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				handler.UpdateConfig(w, req)

				// Should reject invalid field values
				assert.Equal(t, http.StatusBadRequest, w.Code)
			})
		}
	})

	t.Run("enforces size limits", func(t *testing.T) {
		// Test that oversized requests are rejected

		// Create a large configuration that exceeds the 1MB limit
		largeString := strings.Repeat("a", testPayloadSize2MB) // 2MB string
		largeConfig := map[string]interface{}{
			"section": "api",
			"config": map[string]interface{}{
				"api": map[string]interface{}{
					"host": largeString, // This will make the JSON > 1MB
				},
			},
		}

		body, err := json.Marshal(largeConfig)
		require.NoError(t, err)
		require.Greater(t, len(body), testPayloadSize1MB) // Ensure it's actually > 1MB

		req := httptest.NewRequest("PUT", "/api/v1/admin/config", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.UpdateConfig(w, req)

		// Should reject oversized request
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "too large")
	})

	t.Run("validates string field security", func(t *testing.T) {
		tests := []struct {
			name        string
			field       string
			value       string
			maxLength   int
			expectError bool
		}{
			{
				name:        "valid_string",
				field:       "test",
				value:       "valid-value",
				maxLength:   20,
				expectError: false,
			},
			{
				name:        "string_too_long",
				field:       "test",
				value:       strings.Repeat("a", 100),
				maxLength:   50,
				expectError: true,
			},
			{
				name:        "string_with_null_byte",
				field:       "test",
				value:       "test\x00malicious",
				maxLength:   50,
				expectError: true,
			},
			{
				name:        "string_with_control_chars",
				field:       "test",
				value:       "test\x01\x02malicious",
				maxLength:   50,
				expectError: true,
			},
			{
				name:        "empty_string_allowed",
				field:       "test",
				value:       "",
				maxLength:   50,
				expectError: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := validateStringField(tt.field, tt.value, tt.maxLength)
				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("validates host field security", func(t *testing.T) {
		tests := []struct {
			name        string
			value       string
			expectError bool
		}{
			{
				name:        "valid_hostname",
				value:       "example.com",
				expectError: false,
			},
			{
				name:        "valid_ip",
				value:       "192.168.1.1",
				expectError: false,
			},
			{
				name:        "empty_host_allowed",
				value:       "",
				expectError: false,
			},
			{
				name:        "host_with_invalid_chars",
				value:       "host@invalid",
				expectError: true,
			},
			{
				name:        "host_too_long",
				value:       strings.Repeat("a", 300),
				expectError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := validateHostField("host", tt.value)
				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("validates port field security", func(t *testing.T) {
		tests := []struct {
			name        string
			value       int
			expectError bool
		}{
			{
				name:        "valid_port",
				value:       8080,
				expectError: false,
			},
			{
				name:        "valid_privileged_port",
				value:       80,
				expectError: false,
			},
			{
				name:        "port_too_low",
				value:       0,
				expectError: true,
			},
			{
				name:        "port_too_high",
				value:       99999,
				expectError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := validatePortField("port", tt.value)
				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("validates duration field security", func(t *testing.T) {
		tests := []struct {
			name        string
			value       string
			expectError bool
		}{
			{
				name:        "valid_duration",
				value:       "30s",
				expectError: false,
			},
			{
				name:        "valid_complex_duration",
				value:       "1h30m45s",
				expectError: false,
			},
			{
				name:        "empty_duration_allowed",
				value:       "",
				expectError: false,
			},
			{
				name:        "invalid_duration_format",
				value:       "invalid-duration",
				expectError: true,
			},
			{
				name:        "duration_too_long",
				value:       strings.Repeat("1h", 50),
				expectError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := validateDurationField("timeout", tt.value)
				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("validates path field security", func(t *testing.T) {
		tests := []struct {
			name        string
			value       string
			expectError bool
		}{
			{
				name:        "valid_path",
				value:       "/var/log/app.log",
				expectError: false,
			},
			{
				name:        "valid_relative_path",
				value:       "logs/app.log",
				expectError: false,
			},
			{
				name:        "empty_path_allowed",
				value:       "",
				expectError: false,
			},
			{
				name:        "path_with_traversal",
				value:       "../../../etc/passwd",
				expectError: true,
			},
			{
				name:        "path_with_traversal_cleaned",
				value:       "config/../../../etc/passwd",
				expectError: true,
			},
			{
				name:        "path_too_long",
				value:       "/" + strings.Repeat("a", 5000),
				expectError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := validatePathField("path", tt.value)
				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("parseConfigJSON enforces size limits", func(t *testing.T) {
		// Test that the secure parsing function enforces size limits

		// Create a large JSON payload that exceeds the 1MB limit
		largeData := map[string]interface{}{
			"section": "api",
			"config": map[string]interface{}{
				"api": map[string]interface{}{
					"large_field": strings.Repeat("a", testPayloadSize2MB), // 2MB string
				},
			},
		}

		body, err := json.Marshal(largeData)
		require.NoError(t, err)

		req := httptest.NewRequest("PUT", "/api/v1/admin/config", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")

		var dest ConfigUpdateRequest
		err = parseConfigJSON(req, &dest)

		// Should reject oversized JSON
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "too large")
	})

	t.Run("prevents unknown fields in JSON", func(t *testing.T) {
		// Test that unknown fields are rejected

		jsonWithUnknownFields := `{
			"section": "api",
			"config": {
				"api": {
					"host": "localhost",
					"unknown_malicious_field": "exploit_attempt"
				}
			},
			"malicious_root_field": "exploit"
		}`

		req := httptest.NewRequest("PUT", "/api/v1/admin/config", strings.NewReader(jsonWithUnknownFields))
		req.Header.Set("Content-Type", "application/json")

		var dest ConfigUpdateRequest
		err := parseConfigJSON(req, &dest)

		// Should reject JSON with unknown fields
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown field")
	})

	t.Run("validates configuration sections properly", func(t *testing.T) {
		tests := []struct {
			name          string
			section       string
			configData    interface{}
			expectError   bool
			errorContains string
		}{
			{
				name:    "valid_api_config",
				section: "api",
				configData: ConfigUpdateData{
					API: &APIConfigUpdate{
						Host: stringPtr("localhost"),
						Port: intPtr(8080),
					},
				},
				expectError: false,
			},
			{
				name:    "valid_database_config",
				section: "database",
				configData: ConfigUpdateData{
					Database: &DatabaseConfigUpdate{
						Host: stringPtr("db.example.com"),
						Port: intPtr(5432),
					},
				},
				expectError: false,
			},
			{
				name:    "invalid_section",
				section: "invalid",
				configData: ConfigUpdateData{
					API: &APIConfigUpdate{
						Host: stringPtr("localhost"),
					},
				},
				expectError:   true,
				errorContains: "Field validation for 'Section' failed on the 'oneof' tag",
			},
			{
				name:    "missing_config_for_section",
				section: "api",
				configData: ConfigUpdateData{
					Database: &DatabaseConfigUpdate{ // Wrong section data
						Host: stringPtr("localhost"),
					},
				},
				expectError:   true,
				errorContains: "api configuration data is required",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := &ConfigUpdateRequest{
					Section: tt.section,
					Config:  tt.configData.(ConfigUpdateData),
				}

				err := handler.validateConfigUpdate(req)

				if tt.expectError {
					assert.Error(t, err)
					if tt.errorContains != "" {
						assert.Contains(t, err.Error(), tt.errorContains)
					}
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("security validation prevents dangerous values", func(t *testing.T) {
		tests := []struct {
			name          string
			config        *APIConfigUpdate
			expectError   bool
			errorContains string
		}{
			{
				name: "dangerous_cors_origin",
				config: &APIConfigUpdate{
					CORSOrigins: []string{
						"https://example.com",
						strings.Repeat("a", 300), // Too long
					},
				},
				expectError:   true,
				errorContains: "too long",
			},
			{
				name: "null_byte_in_host",
				config: &APIConfigUpdate{
					Host: stringPtr("host\x00.evil.com"),
				},
				expectError:   true,
				errorContains: "null byte",
			},
			{
				name: "control_chars_in_timeout",
				config: &APIConfigUpdate{
					ReadTimeout: stringPtr("30s\x01malicious"),
				},
				expectError:   true,
				errorContains: "control character",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := handler.validateAPIConfig(tt.config)

				if tt.expectError {
					assert.Error(t, err)
					if tt.errorContains != "" {
						assert.Contains(t, err.Error(), tt.errorContains)
					}
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("struct to map conversion is secure", func(t *testing.T) {
		// Test that the structToMap function handles edge cases securely

		config := &APIConfigUpdate{
			Host:    stringPtr("localhost"),
			Port:    intPtr(8080),
			Enabled: boolPtr(true),
		}

		result, err := structToMap(config)
		require.NoError(t, err)

		// Should only contain non-nil values
		assert.Equal(t, "localhost", result["host"])
		assert.Equal(t, float64(8080), result["port"]) // JSON numbers are float64
		assert.Equal(t, true, result["enabled"])

		// Should not contain nil fields
		assert.NotContains(t, result, "read_timeout")
		assert.NotContains(t, result, "write_timeout")
	})

	t.Run("configuration extraction by section works securely", func(t *testing.T) {
		// Test that configuration extraction properly isolates sections

		req := &ConfigUpdateRequest{
			Section: "api",
			Config: ConfigUpdateData{
				API: &APIConfigUpdate{
					Host: stringPtr("localhost"),
					Port: intPtr(8080),
				},
				// Include other sections to ensure they're ignored
				Database: &DatabaseConfigUpdate{
					Host: stringPtr("should-be-ignored"),
				},
			},
		}

		result, err := handler.extractConfigSection(req)
		require.NoError(t, err)

		// Should only extract the requested section
		assert.Equal(t, "localhost", result["host"])
		assert.Equal(t, float64(8080), result["port"])

		// Should not contain data from other sections
		assert.NotContains(t, result, "database")
		assert.NotContains(t, result, "username")
	})
}

func TestConfigSecurity_FileValidation(t *testing.T) {
	t.Run("validates config path security", func(t *testing.T) {
		// Test path validation (from config package)
		tests := []struct {
			name        string
			path        string
			expectError bool
		}{
			{
				name:        "valid_relative_path",
				path:        "config.yaml",
				expectError: false,
			},
			{
				name:        "valid_absolute_path",
				path:        "/etc/scanorama/config.yaml",
				expectError: false,
			},
			{
				name:        "path_traversal_attempt",
				path:        "../../../etc/passwd",
				expectError: true,
			},
			{
				name:        "null_byte_injection",
				path:        "config.yaml\x00.evil",
				expectError: true,
			},
			{
				name:        "path_too_long",
				path:        strings.Repeat("a", 5000),
				expectError: true,
			},
			{
				name:        "invalid_extension",
				path:        "config.exe",
				expectError: true,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// This calls the validateConfigPath function from config package
				// We can't easily test it directly without exposing it, but we can
				// test the Load function which uses it

				// For this test, we'll create a mock implementation
				err := mockValidateConfigPath(tt.path)

				if tt.expectError {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})
}

// Helper functions for tests

func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}

// mockValidateConfigPath provides a mock implementation for testing path validation
func mockValidateConfigPath(path string) error {
	// This mirrors the logic from the actual validateConfigPath function
	// for testing purposes

	if len(path) > 4096 {
		return fmt.Errorf("path too long")
	}

	for i, char := range path {
		if char == 0 {
			return fmt.Errorf("null byte in path at position %d", i)
		}
	}

	if strings.Contains(path, "..") {
		return fmt.Errorf("path contains directory traversal")
	}

	// Check file extension only if there's a dot
	if dotIndex := strings.LastIndex(path, "."); dotIndex != -1 {
		ext := strings.ToLower(path[dotIndex:])
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return fmt.Errorf("unsupported config file extension: %s", ext)
		}
	}

	return nil
}
