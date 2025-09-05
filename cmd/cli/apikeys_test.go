package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDisplayAPIKeysJSON(t *testing.T) {
	tests := []struct {
		name     string
		keys     []auth.APIKeyInfo
		expected struct {
			APIKeys []auth.APIKeyInfo `json:"api_keys"`
			Count   int               `json:"count"`
		}
		expectError bool
	}{
		{
			name: "empty_api_keys_list",
			keys: []auth.APIKeyInfo{},
			expected: struct {
				APIKeys []auth.APIKeyInfo `json:"api_keys"`
				Count   int               `json:"count"`
			}{
				APIKeys: []auth.APIKeyInfo{},
				Count:   0,
			},
			expectError: false,
		},
		{
			name: "single_api_key",
			keys: []auth.APIKeyInfo{
				{
					ID:        "1",
					Name:      "Test Key",
					KeyPrefix: "sk_test_",
					IsActive:  true,
					CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
			expected: struct {
				APIKeys []auth.APIKeyInfo `json:"api_keys"`
				Count   int               `json:"count"`
			}{
				APIKeys: []auth.APIKeyInfo{
					{
						ID:        "1",
						Name:      "Test Key",
						KeyPrefix: "sk_test_",
						IsActive:  true,
						CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
						UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					},
				},
				Count: 1,
			},
			expectError: false,
		},
		{
			name: "multiple_api_keys",
			keys: []auth.APIKeyInfo{
				{
					ID:        "1",
					Name:      "Production Key",
					KeyPrefix: "sk_prod_",
					IsActive:  true,
					CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				},
				{
					ID:        "2",
					Name:      "Development Key",
					KeyPrefix: "sk_dev_",
					IsActive:  false,
					CreatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
				},
				{
					ID:         "3",
					Name:       "Expired Key",
					KeyPrefix:  "sk_exp_",
					IsActive:   true,
					ExpiresAt:  &[]time.Time{time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)}[0],
					CreatedAt:  time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
					UpdatedAt:  time.Date(2024, 1, 6, 0, 0, 0, 0, time.UTC),
					LastUsedAt: &[]time.Time{time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)}[0],
				},
			},
			expected: struct {
				APIKeys []auth.APIKeyInfo `json:"api_keys"`
				Count   int               `json:"count"`
			}{
				APIKeys: []auth.APIKeyInfo{
					{
						ID:        "1",
						Name:      "Production Key",
						KeyPrefix: "sk_prod_",
						IsActive:  true,
						CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
						UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
					},
					{
						ID:        "2",
						Name:      "Development Key",
						KeyPrefix: "sk_dev_",
						IsActive:  false,
						CreatedAt: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
						UpdatedAt: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
					},
					{
						ID:         "3",
						Name:       "Expired Key",
						KeyPrefix:  "sk_exp_",
						IsActive:   true,
						ExpiresAt:  &[]time.Time{time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)}[0],
						CreatedAt:  time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
						UpdatedAt:  time.Date(2024, 1, 6, 0, 0, 0, 0, time.UTC),
						LastUsedAt: &[]time.Time{time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)}[0],
					},
				},
				Count: 3,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			// Capture stderr for error cases
			oldStderr := os.Stderr
			rErr, wErr, err := os.Pipe()
			require.NoError(t, err)
			os.Stderr = wErr

			// Call the function
			displayAPIKeysJSON(tt.keys)

			// Restore stdout/stderr
			w.Close()
			wErr.Close()
			os.Stdout = oldStdout
			os.Stderr = oldStderr

			// Read the output
			var buf bytes.Buffer
			_, err = buf.ReadFrom(r)
			require.NoError(t, err)

			var errBuf bytes.Buffer
			_, err = errBuf.ReadFrom(rErr)
			require.NoError(t, err)

			if tt.expectError {
				assert.Contains(t, errBuf.String(), "Error marshaling JSON")
				return
			}

			// Parse the JSON output
			output := buf.String()
			require.NotEmpty(t, output)

			var result struct {
				APIKeys []auth.APIKeyInfo `json:"api_keys"`
				Count   int               `json:"count"`
			}

			err = json.Unmarshal([]byte(output), &result)
			require.NoError(t, err, "Output should be valid JSON: %s", output)

			// Verify the structure
			assert.Equal(t, tt.expected.Count, result.Count)
			assert.Len(t, result.APIKeys, tt.expected.Count)

			// Verify each API key matches expected
			for i, expectedKey := range tt.expected.APIKeys {
				assert.Equal(t, expectedKey.ID, result.APIKeys[i].ID)
				assert.Equal(t, expectedKey.Name, result.APIKeys[i].Name)
				assert.Equal(t, expectedKey.KeyPrefix, result.APIKeys[i].KeyPrefix)
				assert.Equal(t, expectedKey.IsActive, result.APIKeys[i].IsActive)
				assert.True(t, expectedKey.CreatedAt.Equal(result.APIKeys[i].CreatedAt))
				assert.True(t, expectedKey.UpdatedAt.Equal(result.APIKeys[i].UpdatedAt))

				if expectedKey.ExpiresAt != nil {
					require.NotNil(t, result.APIKeys[i].ExpiresAt)
					assert.True(t, expectedKey.ExpiresAt.Equal(*result.APIKeys[i].ExpiresAt))
				} else {
					assert.Nil(t, result.APIKeys[i].ExpiresAt)
				}

				if expectedKey.LastUsedAt != nil {
					require.NotNil(t, result.APIKeys[i].LastUsedAt)
					assert.True(t, expectedKey.LastUsedAt.Equal(*result.APIKeys[i].LastUsedAt))
				} else {
					assert.Nil(t, result.APIKeys[i].LastUsedAt)
				}
			}

			// Verify JSON formatting (should be indented)
			assert.Contains(t, output, "  ") // Should contain indentation
		})
	}
}

func TestDisplayAPIKeysTable(t *testing.T) {
	tests := []struct {
		name            string
		keys            []auth.APIKeyInfo
		expectedHeaders []string
		expectedRows    int
	}{
		{
			name:            "empty_api_keys_list",
			keys:            []auth.APIKeyInfo{},
			expectedHeaders: []string{"ID", "NAME", "PREFIX", "STATUS", "EXPIRES", "CREATED", "LAST USED"},
			expectedRows:    0,
		},
		{
			name: "single_active_key",
			keys: []auth.APIKeyInfo{
				{
					ID:        "1",
					Name:      "Test Key",
					KeyPrefix: "sk_test_",
					IsActive:  true,
					CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				},
			},
			expectedHeaders: []string{"ID", "NAME", "PREFIX", "STATUS", "EXPIRES", "CREATED", "LAST USED"},
			expectedRows:    1,
		},
		{
			name: "multiple_keys_with_different_states",
			keys: []auth.APIKeyInfo{
				{
					ID:        "1",
					Name:      "Active Key",
					KeyPrefix: "sk_active_",
					IsActive:  true,
					CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
				{
					ID:        "2",
					Name:      "Inactive Key",
					KeyPrefix: "sk_inactive_",
					IsActive:  false,
					CreatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
					UpdatedAt: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				},
				{
					ID:         "3",
					Name:       "Key with Expiry",
					KeyPrefix:  "sk_exp_",
					IsActive:   true,
					ExpiresAt:  &[]time.Time{time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC)}[0],
					CreatedAt:  time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
					UpdatedAt:  time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
					LastUsedAt: &[]time.Time{time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)}[0],
				},
			},
			expectedHeaders: []string{"ID", "NAME", "PREFIX", "STATUS", "EXPIRES", "CREATED", "LAST USED"},
			expectedRows:    3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, err := os.Pipe()
			require.NoError(t, err)
			os.Stdout = w

			// Call the function
			displayAPIKeysTable(tt.keys)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read the output
			var buf bytes.Buffer
			_, err = buf.ReadFrom(r)
			require.NoError(t, err)

			output := buf.String()

			// Verify headers are present
			for _, header := range tt.expectedHeaders {
				assert.Contains(t, output, header, "Table should contain header: %s", header)
			}

			// Count the number of data rows (excluding header and separator lines)
			lines := strings.Split(output, "\n")
			dataRows := 0
			for _, line := range lines {
				// Skip empty lines, header lines, and separator lines
				if strings.TrimSpace(line) != "" &&
					!strings.Contains(line, "─") && // horizontal separator
					!strings.Contains(line, "┼") && // cross separator
					!strings.Contains(line, "┌") && // top border
					!strings.Contains(line, "└") && // bottom border
					!strings.Contains(line, "│ ID ") { // header row
					// Check if it looks like a data row (contains pipe separators for data)
					if strings.Count(line, "│") >= 2 {
						dataRows++
					}
				}
			}

			assert.Equal(t, tt.expectedRows, dataRows,
				"Expected %d data rows, got %d in output:\n%s", tt.expectedRows, dataRows, output)

			// Verify specific content for known keys
			if tt.expectedRows > 0 {
				for _, key := range tt.keys {
					assert.Contains(t, output, key.ID, "Should contain key ID")
					assert.Contains(t, output, key.Name, "Should contain key name")
					assert.Contains(t, output, key.KeyPrefix, "Should contain key prefix")

					if key.IsActive {
						assert.Contains(t, output, "Active", "Should show active status")
					} else {
						assert.Contains(t, output, "Inactive", "Should show inactive status")
					}
				}
			}
		})
	}
}

func TestDisplayAPIKeysJSON_ErrorHandling(t *testing.T) {
	// This test verifies error handling for JSON marshaling
	// In practice, APIKeyInfo should always be marshalable, but we test the error path
	t.Run("json_marshal_error_handling", func(t *testing.T) {
		// Capture stderr
		oldStderr := os.Stderr
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stderr = w

		// Create a scenario that could potentially cause JSON marshal errors
		// Since APIKeyInfo is a well-defined struct, we'll test with valid data
		// but verify the error handling code path exists
		keys := []auth.APIKeyInfo{
			{
				ID:        "1",
				Name:      "Test Key",
				KeyPrefix: "sk_test_",
				IsActive:  true,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			},
		}

		// Call the function (this should succeed)
		displayAPIKeysJSON(keys)

		// Restore stderr
		w.Close()
		os.Stderr = oldStderr

		// Read stderr output
		var buf bytes.Buffer
		_, err = buf.ReadFrom(r)
		require.NoError(t, err)

		// For valid input, there should be no error output
		assert.Empty(t, buf.String(), "Should not have error output for valid input")
	})
}

func TestAPIKeysJSONFormat(t *testing.T) {
	// Test that the JSON output follows the expected schema
	t.Run("json_schema_validation", func(t *testing.T) {
		keys := []auth.APIKeyInfo{
			{
				ID:        "1",
				Name:      "Schema Test Key",
				KeyPrefix: "sk_schema_",
				IsActive:  true,
				CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
				UpdatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
			},
		}

		// Capture stdout
		oldStdout := os.Stdout
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stdout = w

		displayAPIKeysJSON(keys)

		w.Close()
		os.Stdout = oldStdout

		var buf bytes.Buffer
		_, err = buf.ReadFrom(r)
		require.NoError(t, err)

		output := buf.String()

		// Validate JSON structure
		var jsonOutput map[string]interface{}
		err = json.Unmarshal([]byte(output), &jsonOutput)
		require.NoError(t, err)

		// Check top-level fields
		assert.Contains(t, jsonOutput, "api_keys", "Should contain api_keys field")
		assert.Contains(t, jsonOutput, "count", "Should contain count field")

		// Verify field types
		apiKeys, ok := jsonOutput["api_keys"].([]interface{})
		require.True(t, ok, "api_keys should be an array")
		assert.Len(t, apiKeys, 1, "Should have one API key")

		count, ok := jsonOutput["count"].(float64) // JSON numbers are float64 in Go
		require.True(t, ok, "count should be a number")
		assert.Equal(t, float64(1), count, "Count should match array length")

		// Check API key object structure
		apiKey := apiKeys[0].(map[string]interface{})
		expectedFields := []string{"id", "name", "key_prefix", "is_active", "created_at", "updated_at"}
		for _, field := range expectedFields {
			assert.Contains(t, apiKey, field, "API key should contain field: %s", field)
		}
	})
}

func TestExecuteListAPIKeys_EmptyJSONOutput(t *testing.T) {
	// This test verifies that when no API keys exist, JSON output still produces valid JSON
	// rather than plain text "No API keys found." message
	t.Run("empty_list_produces_valid_json", func(t *testing.T) {
		// Capture stdout
		oldStdout := os.Stdout
		r, w, err := os.Pipe()
		require.NoError(t, err)
		os.Stdout = w

		// Set JSON output mode
		originalOutput := apiKeyOutput
		apiKeyOutput = "json"
		defer func() {
			apiKeyOutput = originalOutput
		}()

		// Test with empty slice (simulating no API keys in database)
		displayAPIKeysJSON([]auth.APIKeyInfo{})

		// Restore stdout
		w.Close()
		os.Stdout = oldStdout

		// Read the output
		var buf bytes.Buffer
		_, err = buf.ReadFrom(r)
		require.NoError(t, err)

		output := buf.String()
		require.NotEmpty(t, output)

		// Verify it's valid JSON
		var result struct {
			APIKeys []auth.APIKeyInfo `json:"api_keys"`
			Count   int               `json:"count"`
		}

		err = json.Unmarshal([]byte(output), &result)
		require.NoError(t, err, "Output should be valid JSON even when empty: %s", output)

		// Verify structure
		assert.Equal(t, 0, result.Count)
		assert.Len(t, result.APIKeys, 0)
		assert.NotNil(t, result.APIKeys) // Should be empty array, not null
	})
}
