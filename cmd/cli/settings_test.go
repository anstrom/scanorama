package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── displaySettingsTable ────────────────────────────────────────────────────

func TestDisplaySettingsTable(t *testing.T) {
	tests := []struct {
		name             string
		settings         []Setting
		expectedHeaders  []string
		expectedRowCount int
		expectedContains []string
	}{
		{
			name:             "empty_list",
			settings:         []Setting{},
			expectedHeaders:  []string{"KEY", "TYPE", "VALUE", "DESCRIPTION", "UPDATED"},
			expectedRowCount: 0,
		},
		{
			name: "single_setting",
			settings: []Setting{
				{
					Key:         "scan.timeout",
					Value:       "30",
					Description: "Scan timeout in seconds",
					Type:        "int",
					UpdatedAt:   "2024-01-15T10:00:00Z",
				},
			},
			expectedHeaders:  []string{"KEY", "TYPE", "VALUE", "DESCRIPTION", "UPDATED"},
			expectedRowCount: 1,
			expectedContains: []string{"scan.timeout", "30", "int", "Scan timeout in seconds"},
		},
		{
			name: "multiple_settings",
			settings: []Setting{
				{
					Key:         "log.level",
					Value:       `"info"`,
					Description: "Logging level",
					Type:        "string",
					UpdatedAt:   "2024-01-10T08:00:00Z",
				},
				{
					Key:         "feature.enabled",
					Value:       "true",
					Description: "Feature flag",
					Type:        "bool",
					UpdatedAt:   "2024-01-12T09:30:00Z",
				},
				{
					Key:         "scan.timeout",
					Value:       "30",
					Description: "Scan timeout in seconds",
					Type:        "int",
					UpdatedAt:   "2024-01-14T11:45:00Z",
				},
			},
			expectedHeaders:  []string{"KEY", "TYPE", "VALUE", "DESCRIPTION", "UPDATED"},
			expectedRowCount: 3,
			expectedContains: []string{
				"log.level", "feature.enabled", "scan.timeout",
				"string", "bool", "int",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(t, func() {
				displaySettingsTable(tt.settings)
			})

			for _, hdr := range tt.expectedHeaders {
				assert.Contains(t, output, hdr,
					"table should contain header %q", hdr)
			}

			for _, want := range tt.expectedContains {
				assert.Contains(t, output, want,
					"table should contain %q", want)
			}

			// Count data rows (lines with at least two │ separators, excluding
			// the header row which contains the column name "KEY").
			lines := strings.Split(output, "\n")
			dataRows := 0
			for _, line := range lines {
				if strings.TrimSpace(line) == "" {
					continue
				}
				if strings.Contains(line, "─") ||
					strings.Contains(line, "┼") ||
					strings.Contains(line, "┌") ||
					strings.Contains(line, "└") ||
					strings.Contains(line, "KEY") {
					continue
				}
				if strings.Count(line, "│") >= 2 {
					dataRows++
				}
			}

			assert.Equal(t, tt.expectedRowCount, dataRows,
				"expected %d data rows, got %d; output:\n%s",
				tt.expectedRowCount, dataRows, output)
		})
	}
}

// ─── displaySettingsTable — value truncation ─────────────────────────────────

func TestDisplaySettingsTableTruncatesLongValues(t *testing.T) {
	longValue := strings.Repeat("x", 60) // 60 chars > settingsMaxValueLen (40)

	settings := []Setting{
		{
			Key:         "some.key",
			Value:       longValue,
			Description: "desc",
			Type:        "string",
			UpdatedAt:   "2024-01-01T00:00:00Z",
		},
	}

	output := captureStdout(t, func() {
		displaySettingsTable(settings)
	})

	// The full 60-char value must not appear verbatim.
	assert.NotContains(t, output, longValue,
		"long value should be truncated in table output")

	// An ellipsis must be present to signal truncation.
	assert.Contains(t, output, "...",
		"truncated value should end with '...'")
}

// ─── displaySettingsJSON ──────────────────────────────────────────────────────

func TestDisplaySettingsJSON(t *testing.T) {
	tests := []struct {
		name     string
		settings []Setting
	}{
		{
			name:     "empty_list",
			settings: []Setting{},
		},
		{
			name: "single_setting",
			settings: []Setting{
				{
					Key:         "scan.timeout",
					Value:       "30",
					Description: "Scan timeout in seconds",
					Type:        "int",
					UpdatedAt:   "2024-01-15T10:00:00Z",
				},
			},
		},
		{
			name: "multiple_settings",
			settings: []Setting{
				{
					Key:         "log.level",
					Value:       `"info"`,
					Description: "Logging level",
					Type:        "string",
					UpdatedAt:   "2024-01-10T08:00:00Z",
				},
				{
					Key:         "feature.enabled",
					Value:       "true",
					Description: "Feature flag",
					Type:        "bool",
					UpdatedAt:   "2024-01-12T09:30:00Z",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStdout(t, func() {
				displaySettingsJSON(tt.settings)
			})

			require.NotEmpty(t, output, "JSON output must not be empty")

			// Must be valid JSON.
			var raw map[string]interface{}
			require.NoError(t, json.Unmarshal([]byte(output), &raw),
				"output must be valid JSON; got:\n%s", output)

			// Top-level keys must be present.
			assert.Contains(t, raw, "settings", "JSON must contain 'settings' key")
			assert.Contains(t, raw, "count", "JSON must contain 'count' key")

			// 'count' must equal len(settings).
			count, ok := raw["count"].(float64)
			require.True(t, ok, "'count' must be a number")
			assert.Equal(t, float64(len(tt.settings)), count)

			// 'settings' must be an array of the correct length.
			arr, ok := raw["settings"].([]interface{})
			require.True(t, ok, "'settings' must be an array")
			assert.Len(t, arr, len(tt.settings))

			// Each element must carry the expected field names.
			for i, elem := range arr {
				obj, ok := elem.(map[string]interface{})
				require.True(t, ok, "settings[%d] must be an object", i)

				for _, field := range []string{"key", "value", "description", "type", "updated_at"} {
					assert.Contains(t, obj, field,
						"settings[%d] must contain field %q", i, field)
				}

				// Verify the key value matches.
				assert.Equal(t, tt.settings[i].Key, obj["key"],
					"settings[%d].key mismatch", i)
			}

			// Output must be indented (pretty-printed).
			assert.Contains(t, output, "  ",
				"JSON output should be indented")
		})
	}
}

// ─── Command structure ────────────────────────────────────────────────────────

func TestSettingsCommandStructure(t *testing.T) {
	t.Run("settings help lists subcommands", func(t *testing.T) {
		out, errOut, err := executeCommand("settings", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "get",
			"settings help should list the 'get' subcommand")
		assert.Contains(t, all, "update",
			"settings help should list the 'update' subcommand")
	})

	t.Run("settings get help shows output flag", func(t *testing.T) {
		out, errOut, err := executeCommand("settings", "get", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "--output",
			"settings get help should document the --output flag")
	})

	t.Run("settings update help shows key and value flags", func(t *testing.T) {
		out, errOut, err := executeCommand("settings", "update", "--help")

		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "--key",
			"settings update help should document the --key flag")
		assert.Contains(t, all, "--value",
			"settings update help should document the --value flag")
	})

	t.Run("settings update with no flags returns error", func(t *testing.T) {
		_, _, err := executeCommand("settings", "update")
		assert.Error(t, err,
			"settings update without --key and --value must fail")
	})

	t.Run("settings update with only --key returns error", func(t *testing.T) {
		_, _, err := executeCommand("settings", "update", "--key", "some.key")
		assert.Error(t, err,
			"settings update without --value must fail")
	})

	t.Run("settings update with only --value returns error", func(t *testing.T) {
		_, _, err := executeCommand("settings", "update", "--value", "true")
		assert.Error(t, err,
			"settings update without --key must fail")
	})
}

// ─── truncateValue unit tests ─────────────────────────────────────────────────

func TestTruncateValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short_value_unchanged",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact_length_unchanged",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "long_value_truncated",
			input:    "hello world this is a very long value",
			maxLen:   10,
			expected: "hello w...",
		},
		{
			name:     "empty_string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateValue(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, got)
		})
	}
}
