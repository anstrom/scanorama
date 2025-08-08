package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// escapeXML escapes special XML characters in a string.
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func TestSaveAndLoadXML(t *testing.T) {
	// Create test data
	testResult := &ScanResult{
		Hosts: []Host{
			{
				Address: "192.168.1.1",
				Status:  "up",
				Ports: []Port{
					{
						Number:   80,
						Protocol: "tcp",
						State:    "open",
						Service:  "http",
						Version:  "nginx 1.18.0",
					},
				},
			},
		},
	}

	// Create temporary directory and file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_scan.xml")

	// Test saving XML
	t.Run("SaveXML", func(t *testing.T) {
		err := SaveResults(testResult, tmpFile)
		if err != nil {
			t.Fatalf("Failed to save XML: %v", err)
		}

		// Verify file exists
		if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
			t.Error("XML file was not created")
		}

		// Read file content
		content, err := os.ReadFile(tmpFile)
		if err != nil {
			t.Fatalf("Failed to read XML file: %v", err)
		}

		// Basic content validation
		if len(content) == 0 {
			t.Error("XML file is empty")
		}
	})

	// Test loading XML
	t.Run("LoadXML", func(t *testing.T) {
		result, err := LoadResults(tmpFile)
		if err != nil {
			t.Fatalf("Failed to load XML: %v", err)
		}

		// Verify loaded data
		if len(result.Hosts) != 1 {
			t.Fatalf("Expected 1 host, got %d", len(result.Hosts))
		}

		host := result.Hosts[0]
		if host.Address != "192.168.1.1" {
			t.Errorf("Expected address 192.168.1.1, got %s", host.Address)
		}

		if len(host.Ports) != 1 {
			t.Fatalf("Expected 1 port, got %d", len(host.Ports))
		}

		port := host.Ports[0]
		if port.Number != 80 {
			t.Errorf("Expected port 80, got %d", port.Number)
		}
	})
}

func TestXMLErrorHandling(t *testing.T) {
	t.Run("SaveWithNilResult", func(t *testing.T) {
		err := SaveResults(nil, "test.xml")
		assert.Error(t, err, "Expected error when saving nil result")
		assert.Contains(t, err.Error(), "nil result", "Error should mention nil result")
	})

	t.Run("SaveToInvalidPath", func(t *testing.T) {
		result := &ScanResult{Hosts: []Host{}}
		err := SaveResults(result, "/nonexistent/directory/file.xml")
		assert.Error(t, err, "Expected error when saving to invalid path")
	})

	t.Run("LoadNonexistentFile", func(t *testing.T) {
		_, err := LoadResults("nonexistent.xml")
		assert.Error(t, err, "Expected error when loading nonexistent file")
		assert.Contains(t, err.Error(), "no such file", "Error should mention missing file")
	})

	t.Run("LoadInvalidXML", func(t *testing.T) {
		// Test cases for different types of invalid XML
		invalidXMLs := []struct {
			name    string
			content string
		}{
			{"Empty File", ""},
			{"Invalid XML", "invalid xml content"},
			{"Incomplete XML", "<scanresult>"},
			{"Wrong Root Element", "<wrongroot></wrongroot>"},
			{"Malformed XML", "<scanresult><host>incomplete</host"},
		}

		for _, tc := range invalidXMLs {
			t.Run(tc.name, func(t *testing.T) {
				tmpDir := t.TempDir()
				tmpFile := filepath.Join(tmpDir, "invalid.xml")
				err := os.WriteFile(tmpFile, []byte(tc.content), 0o600)
				require.NoError(t, err, "Failed to create test file")

				_, err = LoadResults(tmpFile)
				assert.Error(t, err, "Expected error when loading invalid XML")
			})
		}
	})

	t.Run("SaveWithEmptyResult", func(t *testing.T) {
		result := &ScanResult{Hosts: []Host{}}
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "empty.xml")

		err := SaveResults(result, tmpFile)
		assert.NoError(t, err, "Should save empty result without error")

		loaded, err := LoadResults(tmpFile)
		assert.NoError(t, err, "Should load empty result without error")
		assert.Empty(t, loaded.Hosts, "Loaded result should have empty hosts")
	})
}

// createComplexTestResult creates a complex scan result for testing.
func createComplexTestResult() *ScanResult {
	return &ScanResult{
		Hosts: []Host{
			{
				Address: "192.168.1.1",
				Status:  "up",
				Ports: []Port{
					{
						Number:      80,
						Protocol:    "tcp",
						State:       "open",
						Service:     "http",
						Version:     "nginx 1.18.0",
						ServiceInfo: "Server",
					},
					{
						Number:   443,
						Protocol: "tcp",
						State:    "closed",
						Service:  "https",
					},
				},
			},
			{
				Address: "192.168.1.2",
				Status:  "down",
				Ports:   []Port{},
			},
		},
	}
}

// createSpecialCharsTestResult creates a scan result with special characters.
func createSpecialCharsTestResult() *ScanResult {
	return &ScanResult{
		Hosts: []Host{
			{
				Address: "test.local",
				Status:  "up",
				Ports: []Port{
					{
						Number:      8080,
						Protocol:    "tcp",
						State:       "open",
						Service:     "http-proxy",
						Version:     "1.0 & <2.0>",
						ServiceInfo: "Test & Demo",
					},
				},
			},
		},
	}
}

// createEmptyPortsTestResult creates a scan result with no ports.
func createEmptyPortsTestResult() *ScanResult {
	return &ScanResult{
		Hosts: []Host{
			{
				Address: "localhost",
				Status:  "up",
				Ports:   []Port{},
			},
		},
	}
}

func TestXMLRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		original *ScanResult
	}{
		{
			name:     "Complex Result",
			original: createComplexTestResult(),
		},
		{
			name:     "Special Characters",
			original: createSpecialCharsTestResult(),
		},
		{
			name:     "Empty Ports",
			original: createEmptyPortsTestResult(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "roundtrip.xml")

			// Test saving
			err := SaveResults(tt.original, tmpFile)
			require.NoError(t, err, "Failed to save XML")

			// Test loading
			loaded, err := LoadResults(tmpFile)
			require.NoError(t, err, "Failed to load XML")

			// Compare results
			compareResults(t, tt.original, loaded)

			// Verify file contents
			content, err := os.ReadFile(tmpFile)
			require.NoError(t, err, "Failed to read XML file")
			xmlContent := string(content)
			assert.Contains(t, xmlContent, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>")
			assert.Contains(t, xmlContent, "<scanresult start_time=")
			assert.Contains(t, xmlContent, "end_time=")
			assert.Contains(t, xmlContent, "duration=")

			// Verify host data
			for _, host := range tt.original.Hosts {
				assert.Contains(t, xmlContent, "<Address>"+host.Address+"</Address>")
				assert.Contains(t, xmlContent, "<Status>"+host.Status+"</Status>")

				for _, port := range host.Ports {
					if port.Version != "" {
						expectedVersion := escapeXML(port.Version)
						assert.Contains(t, xmlContent, "<Version>"+expectedVersion+"</Version>")
					}
					if port.ServiceInfo != "" {
						expectedServiceInfo := escapeXML(port.ServiceInfo)
						assert.Contains(t, xmlContent, "<ServiceInfo>"+expectedServiceInfo+"</ServiceInfo>")
					}
				}
			}
		})
	}
}

// compareResults compares two ScanResult objects for equality.
func compareResults(t *testing.T, original, loaded *ScanResult) {
	require.Equal(t, len(original.Hosts), len(loaded.Hosts), "Host count mismatch")

	for i, origHost := range original.Hosts {
		loadedHost := loaded.Hosts[i]
		compareHosts(t, origHost, loadedHost)
	}
}

// compareHosts compares two Host objects for equality.
func compareHosts(t *testing.T, original, loaded Host) {
	assert.Equal(t, original.Address, loaded.Address, "Host address mismatch")
	assert.Equal(t, original.Status, loaded.Status, "Host status mismatch")
	assert.Equal(t, len(original.Ports), len(loaded.Ports), "Port count mismatch")

	for j, origPort := range original.Ports {
		loadedPort := loaded.Ports[j]
		comparePorts(t, &origPort, &loadedPort)
	}
}

// comparePorts compares two Port objects for equality.
func comparePorts(t *testing.T, original, loaded *Port) {
	assert.Equal(t, original.Number, loaded.Number, "Port number mismatch")
	assert.Equal(t, original.Protocol, loaded.Protocol, "Protocol mismatch")
	assert.Equal(t, original.State, loaded.State, "Port state mismatch")
	assert.Equal(t, original.Service, loaded.Service, "Service mismatch")
	assert.Equal(t, original.Version, loaded.Version, "Version mismatch")
	assert.Equal(t, original.ServiceInfo, loaded.ServiceInfo, "ServiceInfo mismatch")
}
