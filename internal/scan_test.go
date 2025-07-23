package internal

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testServices defines the ports and services available in our Docker test environment
var testServices = struct {
	SSH           string
	HTTP          string
	HTTPS         string
	Redis         string
	Flask         string
	containerName string
	timeout       int
}{
	SSH:           "8022",
	HTTP:          "8080",
	HTTPS:         "8443",
	Redis:         "8379",
	Flask:         "8888",
	containerName: "scanorama-test",
	timeout:       5,
}

// setupTestEnvironment ensures the Docker test environment is running
func setupTestEnvironment(t *testing.T) {
	maxRetries := 5
	retryDelay := time.Second

	// Try connecting to services with retries
	services := map[string]string{
		"SSH":   testServices.SSH,
		"HTTP":  testServices.HTTP,
		"Redis": testServices.Redis,
		"Flask": testServices.Flask,
	}

	for name, port := range services {
		var connected bool
		for i := 0; i < maxRetries; i++ {
			conn, err := net.DialTimeout("tcp", "localhost:"+port, 2*time.Second)
			if err == nil && conn != nil {
				conn.Close()
				connected = true
				break
			}
			if i < maxRetries-1 {
				t.Logf("Retrying connection to %s service on port %s (attempt %d/%d)", name, port, i+1, maxRetries)
				time.Sleep(retryDelay)
			}
		}
		if !connected {
			t.Fatalf("Service %s not available on port %s after %d attempts", name, port, maxRetries)
		}
	}
}

// parsePortRange converts a port string specification into a slice of port numbers.
// This is a test helper function only.
func parsePortRange(ports string) ([]int, error) {
	var result []int
	parts := strings.Split(ports, ",")

	for _, part := range parts {
		// Check for range (e.g., "80-100")
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid port range format: %s", part)
			}

			start, err := strconv.Atoi(rangeParts[0])
			if err != nil {
				return nil, err
			}

			end, err := strconv.Atoi(rangeParts[1])
			if err != nil {
				return nil, err
			}

			if start < 1 || end > 65535 || start > end {
				return nil, fmt.Errorf("invalid port range: %s", part)
			}

			for i := start; i <= end; i++ {
				result = append(result, i)
			}
		} else {
			// Single port
			port, err := strconv.Atoi(part)
			if err != nil {
				return nil, err
			}

			if port < 1 || port > 65535 {
				return nil, fmt.Errorf("port %d out of valid range (1-65535)", port)
			}

			result = append(result, port)
		}
	}

	return result, nil
}

// TestAggressiveScan removed - aggressive mode deprecated

func TestValidateScanConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    ScanConfig
		wantError bool
	}{
		{
			name: "valid config",
			config: ScanConfig{
				Targets:  []string{"localhost"},
				Ports:    "80",
				ScanType: "connect",
			},
			wantError: false,
		},
		{
			name: "empty targets",
			config: ScanConfig{
				Ports:    "80",
				ScanType: "connect",
			},
			wantError: true,
		},
		{
			name: "invalid scan type",
			config: ScanConfig{
				Targets:  []string{"localhost"},
				Ports:    "80",
				ScanType: "invalid",
			},
			wantError: true,
		},
		{
			name: "invalid port format",
			config: ScanConfig{
				Targets:  []string{"localhost"},
				Ports:    "invalid",
				ScanType: "connect",
			},
			wantError: true,
		},
		{
			name: "port out of range",
			config: ScanConfig{
				Targets:  []string{"localhost"},
				Ports:    "65536",
				ScanType: "connect",
			},
			wantError: true,
		},
		{
			name: "negative port",
			config: ScanConfig{
				Targets:  []string{"localhost"},
				Ports:    "-80",
				ScanType: "connect",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateScanConfig(tt.config)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPortParsing(t *testing.T) {
	tests := []struct {
		name    string
		ports   string
		want    []int
		wantErr bool
	}{
		{
			name:    "single port",
			ports:   "80",
			want:    []int{80},
			wantErr: false,
		},
		{
			name:    "multiple ports",
			ports:   "80,443",
			want:    []int{80, 443},
			wantErr: false,
		},
		{
			name:    "port range",
			ports:   "80-82",
			want:    []int{80, 81, 82},
			wantErr: false,
		},
		{
			name:    "invalid format",
			ports:   "invalid",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "out of range",
			ports:   "65536",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePortRange(tt.ports)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestLocalScan(t *testing.T) {
	// Start a local test server to have a guaranteed open port
	setupTestEnvironment(t)

	tests := []struct {
		name      string
		port      string
		scanType  string
		wantState string
	}{
		{"HTTP Service", testServices.HTTP, "connect", "open"},
		{"SSH Service", testServices.SSH, "connect", "open"},
		{"Redis Service", testServices.Redis, "connect", "open"},
		{"Invalid Port", "65530", "connect", "closed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := ScanConfig{
				Targets:  []string{"localhost"},
				Ports:    tt.port,
				ScanType: tt.scanType,
			}

			result, err := RunScan(config)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotEmpty(t, result.Hosts)

			host := result.Hosts[0]
			require.NotEmpty(t, host.Ports)

			foundPort := host.Ports[0]
			portNum, _ := strconv.ParseUint(tt.port, 10, 16)
			assert.Equal(t, uint16(portNum), foundPort.Number)
			assert.Equal(t, tt.wantState, foundPort.State)
		})
	}
}

func TestScanTimeout(t *testing.T) {
	setupTestEnvironment(t)
	tests := []struct {
		name      string
		config    ScanConfig
		wantError bool
	}{
		{
			name: "Full Port Range With Short Timeout",
			config: ScanConfig{
				Targets:    []string{"127.0.0.1"}, // Use IP to avoid DNS lookup
				Ports:      "1-65535",
				ScanType:   "connect",
				TimeoutSec: 1, // Very short timeout to force error
			},
			wantError: true,
		},
		{
			name: "Single Port Normal Timeout",
			config: ScanConfig{
				Targets:    []string{"localhost"},
				Ports:      testServices.HTTP,
				ScanType:   "connect",
				TimeoutSec: 5,
			},
			wantError: false,
		},
		{
			name: "Multiple Services Normal Timeout",
			config: ScanConfig{
				Targets:    []string{"127.0.0.1"}, // Use IP to avoid DNS lookup
				Ports:      fmt.Sprintf("%s,%s,%s", testServices.HTTP, testServices.SSH, testServices.Redis),
				ScanType:   "connect",
				TimeoutSec: 2,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			_, err := RunScan(tt.config)
			duration := time.Since(start)

			if tt.wantError {
				assert.Error(t, err, "Expected error for %s", tt.name)
				if err != nil && tt.name == "Full Port Range With Short Timeout" {
					assert.Contains(t, err.Error(), "timed out", "Expected timeout error")
					assert.LessOrEqual(t, duration.Seconds(), float64(tt.config.TimeoutSec)+1.0,
						"Scan should not exceed timeout by more than 1 second")
				}
			} else {
				assert.NoError(t, err, "Unexpected error for %s: %v", tt.name, err)
				assert.Less(t, duration.Seconds(), float64(tt.config.TimeoutSec),
					"Scan should complete within specified timeout")
			}
		})
	}
}

func TestScanResults(t *testing.T) {
	setupTestEnvironment(t)
	httpPort := testServices.HTTP
	sshPort := testServices.SSH

	config := ScanConfig{
		Targets:  []string{"localhost"},
		Ports:    fmt.Sprintf("%s,%s", httpPort, sshPort),
		ScanType: "connect",
	}

	result, err := RunScan(config)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Hosts)

	host := result.Hosts[0]
	assert.Contains(t, []string{"localhost", "127.0.0.1"}, host.Address)

	// Check that we found both the open and closed ports
	var foundHTTP, foundSSH bool
	for _, p := range host.Ports {
		portNum := strconv.Itoa(int(p.Number))
		if portNum == httpPort && p.State == "open" {
			foundHTTP = true
		}
		if portNum == sshPort && p.State == "open" {
			foundSSH = true
		}
	}

	assert.True(t, foundHTTP, "Should find open HTTP port")
	assert.True(t, foundSSH, "Should find open SSH port")
}

func TestPrintResults(t *testing.T) {
	tests := []struct {
		name   string
		result *ScanResult
	}{
		{
			name:   "Nil Result",
			result: nil,
		},
		{
			name: "Empty Result",
			result: &ScanResult{
				Hosts: []Host{},
			},
		},
		{
			name: "Host Down",
			result: &ScanResult{
				Hosts: []Host{
					{
						Address: "192.168.1.1",
						Status:  "down",
					},
				},
			},
		},
		{
			name: "Host Up No Ports",
			result: &ScanResult{
				Hosts: []Host{
					{
						Address: "192.168.1.1",
						Status:  "up",
						Ports:   []Port{},
					},
				},
			},
		},
		{
			name: "Full Result",
			result: &ScanResult{
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
								Version:     "1.18.0",
								ServiceInfo: "nginx",
							},
							{
								Number:   443,
								Protocol: "tcp",
								State:    "closed",
								Service:  "https",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			PrintResults(tt.result)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			if _, err := io.Copy(&buf, r); err != nil {
				t.Errorf("Failed to capture output: %v", err)
				return
			}
			output := buf.String()

			// Basic output validation
			if tt.result == nil {
				assert.Contains(t, output, "No results available")
			} else if len(tt.result.Hosts) == 0 {
				assert.Contains(t, output, "Scan Results:")
			} else {
				assert.Contains(t, output, "Host: "+tt.result.Hosts[0].Address)
				if tt.result.Hosts[0].Status == "up" && len(tt.result.Hosts[0].Ports) > 0 {
					assert.Contains(t, output, "Open Ports:")
				}
			}
		})
	}
}

func TestServiceDetection(t *testing.T) {
	setupTestEnvironment(t)

	// Test basic port scanning without version detection
	config := ScanConfig{
		Targets:  []string{"127.0.0.1"},
		Ports:    fmt.Sprintf("%s,%s,%s", testServices.HTTP, testServices.SSH, testServices.Redis),
		ScanType: "connect",
	}

	result, err := RunScan(config)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Hosts)

	host := result.Hosts[0]
	assert.Contains(t, []string{"localhost", "127.0.0.1"}, host.Address)

	// Map to store found ports
	foundPorts := make(map[string]Port)
	for _, p := range host.Ports {
		portNum := strconv.Itoa(int(p.Number))
		foundPorts[portNum] = p
	}

	// Check that ports are open
	expectedPorts := []string{testServices.HTTP, testServices.SSH, testServices.Redis}
	for _, portNum := range expectedPorts {
		if port, ok := foundPorts[portNum]; ok {
			assert.Equal(t, "tcp", port.Protocol)
			assert.Equal(t, "open", port.State)
		} else {
			t.Errorf("Port %s not found", portNum)
		}
	}
}

func TestXMLFormatting(t *testing.T) {
	result := &ScanResult{
		Hosts: []Host{
			{
				Address: "localhost",
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

	// Test saving
	tmpFile := "test_scan.xml"
	err := SaveResults(result, tmpFile)
	require.NoError(t, err)
	defer os.Remove(tmpFile)

	// Test loading
	loaded, err := LoadResults(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Compare original and loaded data
	assert.Equal(t, len(result.Hosts), len(loaded.Hosts))
	assert.Equal(t, result.Hosts[0].Address, loaded.Hosts[0].Address)
	assert.Equal(t, result.Hosts[0].Ports[0].Number, loaded.Hosts[0].Ports[0].Number)
}
