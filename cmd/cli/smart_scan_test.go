package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDisplaySuggestionsJSON verifies that the JSON output contains all
// expected top-level keys and correct values.
func TestDisplaySuggestionsJSON(t *testing.T) {
	summary := SuggestionSummary{
		NoOSInfo: SuggestionGroup{
			Count:       3,
			Description: "Hosts without OS info",
			Action:      "run OS detection",
		},
		NoPorts: SuggestionGroup{
			Count:       5,
			Description: "Hosts without port data",
			Action:      "run port scan",
		},
		NoServices: SuggestionGroup{
			Count:       2,
			Description: "Hosts without service banners",
			Action:      "run service scan",
		},
		Stale: SuggestionGroup{
			Count:       7,
			Description: "Stale hosts",
			Action:      "re-scan",
		},
		WellKnown: SuggestionGroup{
			Count:       1,
			Description: "Well-known hosts",
			Action:      "verify",
		},
		TotalHosts:  18,
		GeneratedAt: "2026-04-15T10:00:00Z",
	}

	out := captureStdout(t, func() { displaySuggestionsJSON(summary) })

	require.NotEmpty(t, out)

	var raw map[string]interface{}
	err := json.Unmarshal([]byte(out), &raw)
	require.NoError(t, err, "output should be valid JSON")

	assert.Contains(t, raw, "no_os_info")
	assert.Contains(t, raw, "no_ports")
	assert.Contains(t, raw, "no_services")
	assert.Contains(t, raw, "stale")
	assert.Contains(t, raw, "well_known")
	assert.Contains(t, raw, "total_hosts")
	assert.Contains(t, raw, "generated_at")

	assert.Equal(t, float64(18), raw["total_hosts"])
	assert.Equal(t, "2026-04-15T10:00:00Z", raw["generated_at"])

	// JSON output must be indented.
	assert.Contains(t, out, "  ", "JSON output should be indented")
}

// TestDisplayProfileRecommendationsTable verifies table output for empty and
// populated recommendation slices.
func TestDisplayProfileRecommendationsTable(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		out := captureStdout(t, func() {
			displayProfileRecommendationsTable([]ProfileRecommendation{})
		})
		assert.Contains(t, out, "No profile recommendations available.")
	})

	t.Run("populated list", func(t *testing.T) {
		recs := []ProfileRecommendation{
			{
				OSFamily:    "linux",
				HostCount:   10,
				ProfileID:   "prof-abc123",
				ProfileName: "linux-default",
				Action:      "run version scan",
			},
			{
				OSFamily:    "windows",
				HostCount:   4,
				ProfileID:   "prof-def456",
				ProfileName: "windows-standard",
				Action:      "run smb scan",
			},
		}

		out := captureStdout(t, func() {
			displayProfileRecommendationsTable(recs)
		})

		assert.Contains(t, out, "linux")
		assert.Contains(t, out, "windows")
		assert.Contains(t, out, "linux-default")
		assert.Contains(t, out, "windows-standard")
		assert.Contains(t, out, "run version scan")
		assert.Contains(t, out, "run smb scan")
		assert.Contains(t, out, "10")
		assert.Contains(t, out, "4")
	})
}

// TestDisplayScanStageTable verifies that stage and reason appear in output.
func TestDisplayScanStageTable(t *testing.T) {
	profileID := "prof-xyz789"
	stage := ScanStage{
		Stage:       "service_detection",
		ScanType:    "version",
		Ports:       "22,80,443",
		OSDetection: true,
		ProfileID:   &profileID,
		Reason:      "no service data present",
	}

	out := captureStdout(t, func() { displayScanStageTable(stage) })

	assert.Contains(t, out, "service_detection", "output should contain stage name")
	assert.Contains(t, out, "no service data present", "output should contain reason")
	assert.Contains(t, out, "version", "output should contain scan type")
	assert.Contains(t, out, "22,80,443", "output should contain ports")
	assert.Contains(t, out, "prof-xyz789", "output should contain profile ID")
}

func TestDisplayScanStageTable_NoProfileID(t *testing.T) {
	stage := ScanStage{
		Stage:       "initial",
		ScanType:    "connect",
		Ports:       "1-1024",
		OSDetection: false,
		ProfileID:   nil,
		Reason:      "first scan for host",
	}

	out := captureStdout(t, func() { displayScanStageTable(stage) })

	assert.Contains(t, out, "initial")
	assert.Contains(t, out, "first scan for host")
	assert.NotContains(t, out, "Profile ID", "profile ID line should be absent when nil")
}

// TestSmartScanCommandStructure verifies the command tree is registered correctly.
func TestSmartScanCommandStructure(t *testing.T) {
	t.Run("smart-scan --help is registered", func(t *testing.T) {
		out, errOut, err := executeCommand("smart-scan", "--help")
		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "suggestions",
			"smart-scan help should list the suggestions subcommand")
		assert.Contains(t, all, "profile-recommendations",
			"smart-scan help should list the profile-recommendations subcommand")
		assert.Contains(t, all, "stage",
			"smart-scan help should list the stage subcommand")
		assert.Contains(t, all, "trigger",
			"smart-scan help should list the trigger subcommand")
		assert.Contains(t, all, "trigger-batch",
			"smart-scan help should list the trigger-batch subcommand")
	})

	t.Run("smart-scan stage requires exactly one arg", func(t *testing.T) {
		_, _, err := executeCommand("smart-scan", "stage")
		assert.Error(t, err, "stage with no args should fail")
	})

	t.Run("smart-scan trigger requires exactly one arg", func(t *testing.T) {
		_, _, err := executeCommand("smart-scan", "trigger")
		assert.Error(t, err, "trigger with no args should fail")
	})

	t.Run("trigger-batch --help shows flags", func(t *testing.T) {
		out, errOut, err := executeCommand("smart-scan", "trigger-batch", "--help")
		require.NoError(t, err)
		all := combined(out, errOut)
		assert.Contains(t, all, "--stage",
			"trigger-batch help should document the --stage flag")
		assert.Contains(t, all, "--limit",
			"trigger-batch help should document the --limit flag")
	})
}

// TestUnmarshalAs verifies the generic round-trip helper.
func TestUnmarshalAs(t *testing.T) {
	t.Run("struct round-trip", func(t *testing.T) {
		input := map[string]interface{}{
			"stage":        "initial",
			"scan_type":    "connect",
			"ports":        "80,443",
			"os_detection": false,
			"reason":       "first time",
		}
		got, err := unmarshalAs[ScanStage](input)
		require.NoError(t, err)
		assert.Equal(t, "initial", got.Stage)
		assert.Equal(t, "first time", got.Reason)
	})

	t.Run("slice round-trip", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"os_family":    "linux",
				"host_count":   float64(3),
				"profile_id":   "p1",
				"profile_name": "linux-default",
				"action":       "scan",
			},
		}
		got, err := unmarshalAs[[]ProfileRecommendation](input)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "linux", got[0].OSFamily)
		assert.Equal(t, 3, got[0].HostCount)
	})
}

// TestPrintTriggerResult verifies the trigger response handling:
// queued=true prints the scan_id; queued=false prints the message.
func TestPrintTriggerResult(t *testing.T) {
	t.Run("queued=true prints scan_id", func(t *testing.T) {
		data := map[string]interface{}{
			"host_id": "host-abc",
			"queued":  true,
			"scan_id": "uuid-abc-123",
			"message": "",
		}
		out := captureStdout(t, func() {
			err := printTriggerResult(data)
			require.NoError(t, err)
		})
		assert.Contains(t, out, "uuid-abc-123")
	})

	t.Run("queued=false prints message", func(t *testing.T) {
		data := map[string]interface{}{
			"host_id": "host-abc",
			"queued":  false,
			"message": "host knowledge is already sufficient",
		}
		out := captureStdout(t, func() {
			err := printTriggerResult(data)
			require.NoError(t, err)
		})
		assert.Contains(t, out, "host knowledge is already sufficient")
	})

	t.Run("queued=false with no message uses default", func(t *testing.T) {
		data := map[string]interface{}{"host_id": "host-abc", "queued": false}
		out := captureStdout(t, func() {
			err := printTriggerResult(data)
			require.NoError(t, err)
		})
		assert.Contains(t, out, "no scan queued")
	})

	t.Run("queued=true but scan_id missing returns error", func(t *testing.T) {
		data := map[string]interface{}{"host_id": "host-abc", "queued": true}
		err := printTriggerResult(data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "scan_id missing")
	})
}

// TestDisplayTriggerBatchTable verifies queued and skipped counts are printed.
func TestDisplayTriggerBatchTable(t *testing.T) {
	result := map[string]interface{}{
		"queued":  float64(12),
		"skipped": float64(3),
		"details": []interface{}{},
	}
	out := captureStdout(t, func() { displayTriggerBatchTable(result) })
	assert.Contains(t, out, "12")
	assert.Contains(t, out, "3")
}

// TestDisplayProfileRecommendationsJSON verifies JSON output for recommendations.
func TestDisplayProfileRecommendationsJSON(t *testing.T) {
	recs := []ProfileRecommendation{
		{
			OSFamily:    "linux",
			HostCount:   5,
			ProfileID:   "prof-1",
			ProfileName: "linux-default",
			Action:      "scan",
		},
	}
	out := captureStdout(t, func() { displayProfileRecommendationsJSON(recs) })

	require.NotEmpty(t, out)
	var raw []map[string]interface{}
	err := json.Unmarshal([]byte(strings.TrimSpace(out)), &raw)
	require.NoError(t, err, "output should be valid JSON array")
	require.Len(t, raw, 1)
	assert.Equal(t, "linux", raw[0]["os_family"])
	assert.Equal(t, float64(5), raw[0]["host_count"])
}
