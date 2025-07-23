package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
				err := os.WriteFile(tmpFile, []byte(tc.content), 0644)
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

func TestXMLRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		original *ScanResult
	}{
		{
			name: "Complex Result",
			original: &ScanResult{
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
			},
		},
		{
			name: "Special Characters",
			original: &ScanResult{
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
			},
		},
		{
			name: "Empty Ports",
			original: &ScanResult{
				Hosts: []Host{
					{
						Address: "localhost",
						Status:  "up",
						Ports:   []Port{},
					},
				},
			},
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
			require.Equal(t, len(tt.original.Hosts), len(loaded.Hosts), "Host count mismatch")

			for i, origHost := range tt.original.Hosts {
				loadedHost := loaded.Hosts[i]
				assert.Equal(t, origHost.Address, loadedHost.Address, "Host address mismatch")
				assert.Equal(t, origHost.Status, loadedHost.Status, "Host status mismatch")
				assert.Equal(t, len(origHost.Ports), len(loadedHost.Ports), "Port count mismatch")

				for j, origPort := range origHost.Ports {
					loadedPort := loadedHost.Ports[j]
					assert.Equal(t, origPort.Number, loadedPort.Number, "Port number mismatch")
					assert.Equal(t, origPort.Protocol, loadedPort.Protocol, "Protocol mismatch")
					assert.Equal(t, origPort.State, loadedPort.State, "Port state mismatch")
					assert.Equal(t, origPort.Service, loadedPort.Service, "Service mismatch")
					assert.Equal(t, origPort.Version, loadedPort.Version, "Version mismatch")
					assert.Equal(t, origPort.ServiceInfo, loadedPort.ServiceInfo, "ServiceInfo mismatch")
				}
			}

			// Verify file contents
			content, err := os.ReadFile(tmpFile)
			require.NoError(t, err, "Failed to read XML file")
			assert.Contains(t, string(content), "<?xml version=\"1.0\" encoding=\"UTF-8\"?>")
			assert.Contains(t, string(content), "<scanresult>")
			assert.Contains(t, string(content), "</scanresult>")
		})
	}
}
