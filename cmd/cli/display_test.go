// Package cli provides command-line interface commands for the Scanorama network scanner.
package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anstrom/scanorama/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureOutput redirects os.Stdout to a pipe for the duration of fn, then
// returns everything written to it as a string. This is a local helper for
// display_test.go to avoid conflicting with the captureStdout declaration in
// command_execution_test.go.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	old := os.Stdout
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, err = buf.ReadFrom(r)
	require.NoError(t, err)
	return buf.String()
}

// ---------------------------------------------------------------------------
// buildHostsFilters
// ---------------------------------------------------------------------------

func TestBuildHostsFilters_Defaults(t *testing.T) {
	// Save and restore all package-level vars read by buildHostsFilters.
	origStatus, origOS, origLastSeen, origIgnored :=
		hostsStatus, hostsOSFamily, hostsLastSeen, hostsShowIgnored
	defer func() {
		hostsStatus = origStatus
		hostsOSFamily = origOS
		hostsLastSeen = origLastSeen
		hostsShowIgnored = origIgnored
	}()

	hostsStatus = ""
	hostsOSFamily = ""
	hostsLastSeen = ""
	hostsShowIgnored = false

	f := buildHostsFilters()

	assert.Equal(t, "", f.Status)
	assert.Equal(t, "", f.OSFamily)
	assert.Equal(t, false, f.ShowIgnored)
	assert.Equal(t, time.Duration(0), f.LastSeenDur)
}

func TestBuildHostsFilters_Status(t *testing.T) {
	orig := hostsStatus
	defer func() { hostsStatus = orig }()

	hostsStatus = "up"
	f := buildHostsFilters()
	assert.Equal(t, "up", f.Status)
}

func TestBuildHostsFilters_OSFamily(t *testing.T) {
	orig := hostsOSFamily
	defer func() { hostsOSFamily = orig }()

	hostsOSFamily = "linux"
	f := buildHostsFilters()
	assert.Equal(t, "linux", f.OSFamily)
}

func TestBuildHostsFilters_ShowIgnored(t *testing.T) {
	orig := hostsShowIgnored
	defer func() { hostsShowIgnored = orig }()

	hostsShowIgnored = true
	f := buildHostsFilters()
	assert.True(t, f.ShowIgnored)
}

func TestBuildHostsFilters_LastSeen_Hours(t *testing.T) {
	orig := hostsLastSeen
	defer func() { hostsLastSeen = orig }()

	hostsLastSeen = "24h"
	f := buildHostsFilters()
	assert.Equal(t, 24*time.Hour, f.LastSeenDur)
}

func TestBuildHostsFilters_LastSeen_Days(t *testing.T) {
	orig := hostsLastSeen
	defer func() { hostsLastSeen = orig }()

	hostsLastSeen = "7d"
	f := buildHostsFilters()
	assert.Equal(t, 7*24*time.Hour, f.LastSeenDur)
}

// ---------------------------------------------------------------------------
// displayHosts
// ---------------------------------------------------------------------------

func TestDisplayHosts_Empty(t *testing.T) {
	out := captureOutput(t, func() {
		displayHosts([]Host{})
	})
	assert.Contains(t, out, "No hosts found")
}

func TestDisplayHosts_SingleUpHost(t *testing.T) {
	hosts := []Host{
		{
			IP:        "192.168.1.1",
			Status:    "up",
			OSFamily:  "linux",
			OSName:    "Ubuntu 22.04",
			LastSeen:  time.Now().Add(-30 * time.Minute),
			FirstSeen: time.Now().Add(-24 * time.Hour),
		},
	}

	out := captureOutput(t, func() {
		displayHosts(hosts)
	})

	assert.Contains(t, out, "192.168.1.1")
	assert.Contains(t, out, "up")
	assert.Contains(t, out, "linux")
	assert.Contains(t, out, "Summary: 1 up, 0 down")
}

func TestDisplayHosts_IgnoredHost(t *testing.T) {
	hosts := []Host{
		{
			IP:        "10.0.0.1",
			Status:    "up",
			OSFamily:  "windows",
			OSName:    "Windows Server 2019",
			LastSeen:  time.Now().Add(-1 * time.Hour),
			FirstSeen: time.Now().Add(-48 * time.Hour),
			IsIgnored: true,
		},
	}

	out := captureOutput(t, func() {
		displayHosts(hosts)
	})

	assert.Contains(t, out, "YES")
}

func TestDisplayHosts_SummaryCounts(t *testing.T) {
	hosts := []Host{
		{
			IP:       "192.168.1.1",
			Status:   "up",
			OSFamily: "linux",
			OSName:   "Debian 11",
			LastSeen: time.Now().Add(-10 * time.Minute),
		},
		{
			IP:       "192.168.1.2",
			Status:   "up",
			OSFamily: "linux",
			OSName:   "Ubuntu 20.04",
			LastSeen: time.Now().Add(-20 * time.Minute),
		},
		{
			IP:       "192.168.1.3",
			Status:   "down",
			OSFamily: "unknown",
			OSName:   "Unknown",
			LastSeen: time.Now().Add(-2 * time.Hour),
		},
	}

	out := captureOutput(t, func() {
		displayHosts(hosts)
	})

	assert.Contains(t, out, "Summary: 2 up, 1 down")
}

func TestDisplayHosts_LongOSNameTruncated(t *testing.T) {
	// OSName longer than maxOSNameLength (18) must be truncated with "...".
	longName := "Ubuntu 22.04 LTS Extended Support Edition"
	require.Greater(t, len(longName), maxOSNameLength,
		"test precondition: longName must exceed maxOSNameLength")

	hosts := []Host{
		{
			IP:       "192.168.1.10",
			Status:   "up",
			OSFamily: "linux",
			OSName:   longName,
			LastSeen: time.Now().Add(-5 * time.Minute),
		},
	}

	out := captureOutput(t, func() {
		displayHosts(hosts)
	})

	// The full name must NOT appear verbatim in the output.
	assert.NotContains(t, out, longName)

	// The truncated form must be present.
	truncated := longName[:maxOSNameLength-3] + "..."
	assert.Contains(t, out, truncated)
}

// ---------------------------------------------------------------------------
// displayPortList
// ---------------------------------------------------------------------------

func TestDisplayPortList_FewCommaSeparatedPorts(t *testing.T) {
	out := captureOutput(t, func() {
		displayPortList("80,443,8080")
	})

	assert.Contains(t, out, "Specific ports: 3 ports specified")
	assert.Contains(t, out, "80,443,8080")
}

func TestDisplayPortList_ManyCommaSeparatedPorts(t *testing.T) {
	// Build a list of 15 ports (> 10) to trigger the truncation branch.
	ports := "80,443,8080,8443,22,21,25,110,143,3306,5432,6379,27017,9200,9300"
	portList := strings.Split(ports, ",")
	require.Greater(t, len(portList), 10,
		"test precondition: port list must have more than 10 entries")

	out := captureOutput(t, func() {
		displayPortList(ports)
	})

	assert.Contains(t, out, "and")
	assert.Contains(t, out, "more")
}

func TestDisplayPortList_Range(t *testing.T) {
	out := captureOutput(t, func() {
		displayPortList("1-1024")
	})

	assert.Contains(t, out, "Port range: 1-1024")
}

func TestDisplayPortList_TopPorts(t *testing.T) {
	out := captureOutput(t, func() {
		displayPortList("T:100")
	})

	assert.Contains(t, out, "Top ports: T:100")
}

func TestDisplayPortList_SinglePort(t *testing.T) {
	out := captureOutput(t, func() {
		displayPortList("443")
	})

	assert.Contains(t, out, "Port specification: 443")
}

// ---------------------------------------------------------------------------
// displayProfiles
// ---------------------------------------------------------------------------

func TestDisplayProfiles_Empty(t *testing.T) {
	out := captureOutput(t, func() {
		displayProfiles([]*db.ScanProfile{})
	})

	assert.Contains(t, out, "No scan profiles found")
}

func TestDisplayProfiles_SingleProfile(t *testing.T) {
	profile := &db.ScanProfile{
		ID:          "test-id-123",
		Name:        "test-profile",
		Description: "A test profile",
		OSFamily:    []string{"linux"},
		Ports:       "22,80,443",
		ScanType:    "connect",
		Timing:      "T3",
		BuiltIn:     false,
		CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	out := captureOutput(t, func() {
		displayProfiles([]*db.ScanProfile{profile})
	})

	assert.Contains(t, out, "test-profile")
	assert.Contains(t, out, "connect")
	// The header row must contain the "Built-in" column label.
	assert.Contains(t, out, "Built-in")
	// BuiltIn == false → row should show "No".
	assert.Contains(t, out, "No")
}

func TestDisplayProfiles_BuiltInProfile(t *testing.T) {
	profile := &db.ScanProfile{
		ID:          "builtin-id-456",
		Name:        "builtin-profile",
		Description: "A built-in profile",
		OSFamily:    []string{"windows"},
		Ports:       "T:100",
		ScanType:    "syn",
		Timing:      "T4",
		BuiltIn:     true,
		CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	out := captureOutput(t, func() {
		displayProfiles([]*db.ScanProfile{profile})
	})

	// BuiltIn == true → row should show "Yes".
	assert.Contains(t, out, "Yes")
}

func TestDisplayProfiles_LongDescriptionTruncated(t *testing.T) {
	longDesc := "This is a very long description that definitely exceeds the forty character display limit for profiles"
	require.Greater(t, len(longDesc), maxDescriptionDisplayLen,
		"test precondition: longDesc must exceed maxDescriptionDisplayLen")

	profile := &db.ScanProfile{
		ID:          "long-desc-id",
		Name:        "long-desc-profile",
		Description: longDesc,
		Ports:       "80,443",
		ScanType:    "connect",
		Timing:      "T3",
		BuiltIn:     false,
		CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	out := captureOutput(t, func() {
		displayProfiles([]*db.ScanProfile{profile})
	})

	// Full description must not appear verbatim.
	assert.NotContains(t, out, longDesc)
	// displayProfiles truncates at index 37 and appends "..." (total 40 chars).
	truncated := longDesc[:37] + "..."
	assert.Contains(t, out, truncated)
}

func TestDisplayProfiles_OSFamilyJoinedInOutput(t *testing.T) {
	profile := &db.ScanProfile{
		ID:          "os-family-id",
		Name:        "os-family-profile",
		Description: "Profile with OS family",
		OSFamily:    []string{"linux", "macos"},
		Ports:       "22,80",
		ScanType:    "connect",
		Timing:      "T3",
		BuiltIn:     false,
		CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	out := captureOutput(t, func() {
		displayProfiles([]*db.ScanProfile{profile})
	})

	// The joined OS families string "linux,macos" (or a truncated form of it)
	// must appear somewhere in the output row.
	assert.Contains(t, out, "linux")
}

func TestDisplayProfiles_UsageHints(t *testing.T) {
	profile := &db.ScanProfile{
		ID:          "hint-id",
		Name:        "hint-profile",
		Description: "A profile",
		Ports:       "80",
		ScanType:    "connect",
		Timing:      "T3",
		BuiltIn:     false,
		CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	out := captureOutput(t, func() {
		displayProfiles([]*db.ScanProfile{profile})
	})

	assert.Contains(t, out, "profiles show")
	assert.Contains(t, out, "profiles test")
}

// ---------------------------------------------------------------------------
// displayTestConfiguration
// ---------------------------------------------------------------------------

func TestDisplayTestConfiguration_ContainsRequiredFields(t *testing.T) {
	profile := &db.ScanProfile{
		ID:          "test-id-123",
		Name:        "test-profile",
		Description: "A test profile",
		OSFamily:    []string{"linux"},
		Ports:       "22,80,443",
		ScanType:    "connect",
		Timing:      "T3",
		BuiltIn:     false,
		CreatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	target := "192.168.1.50"

	out := captureOutput(t, func() {
		displayTestConfiguration(profile, target)
	})

	assert.Contains(t, out, "Test Configuration for Profile:")
	assert.Contains(t, out, profile.Name)
	assert.Contains(t, out, target)
	assert.Contains(t, out, profile.ScanType)
	assert.Contains(t, out, profile.Ports)
}
