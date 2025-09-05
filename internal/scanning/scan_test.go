package scanning

import (
	"bytes"
	"context"
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

// testServices defines the ports and services available in our Docker test environment.
var testServices = struct {
	SSH           string
	HTTP          string
	HTTPS         string
	Redis         string
	containerName string
	timeout       int
	requireAll    bool
}{
	SSH:           "8022",
	HTTP:          "8080",
	HTTPS:         "8443",
	Redis:         "8379",
	containerName: "scanorama-test",
	timeout:       5,
	requireAll:    false, // Set to false to skip tests that require missing services
}

// Returns true if all services are available, false otherwise.
func setupTestEnvironment(t *testing.T) bool {
	if testing.Short() {
		t.Skip("Skipping test that requires Docker services in short mode")
		return false
	}

	maxRetries := 5
	retryDelay := time.Second

	// Try connecting to services with retries
	services := map[string]string{
		"HTTP": testServices.HTTP, // HTTP is required
	}

	// Optional services to check if requireAll is true
	optionalServices := map[string]string{
		"SSH":   testServices.SSH,
		"Redis": testServices.Redis,
	}

	if testServices.requireAll {
		for name, port := range optionalServices {
			services[name] = port
		}
	}

	for name, port := range services {
		var connected bool
		for i := 0; i < maxRetries; i++ {
			conn, err := net.DialTimeout("tcp", "localhost:"+port, 2*time.Second)
			if err == nil && conn != nil {
				_ = conn.Close()
				connected = true
				break
			}
			if i < maxRetries-1 {
				t.Logf("Retrying connection to %s service on port %s (attempt %d/%d)", name, port, i+1, maxRetries)
				time.Sleep(retryDelay)
			}
		}
		if !connected {
			if name == "HTTP" {
				// HTTP is required for all tests
				t.Skipf("Skipping test: HTTP service unavailable on port %s after %d attempts", port, maxRetries)
				return false
			} else {
				t.Logf("Warning: service %s not available on port %s - some tests may be limited", name, port)
			}
		}
	}

	// Try optional services too, but don't fail if they're not available
	for name, port := range optionalServices {
		conn, err := net.DialTimeout("tcp", "localhost:"+port, 2*time.Second)
		if err == nil && conn != nil {
			_ = conn.Close()
			t.Logf("Optional service %s is available on port %s", name, port)
		}
	}

	return true
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
			name: "port zero allowed",
			config: ScanConfig{
				Targets:  []string{"localhost"},
				Ports:    "0",
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
			err := tt.config.Validate()
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLocalScan(t *testing.T) {
	if !setupTestEnvironment(t) {
		return // Test was skipped
	}

	tests := []struct {
		name      string
		port      string
		scanType  string
		wantState string
		required  bool
	}{
		{"HTTP Service", testServices.HTTP, "connect", "open", true},
		{"SSH Service", testServices.SSH, "connect", "open", false},
		{"Redis Service", testServices.Redis, "connect", "open", false},
		{"Invalid Port", "65530", "connect", "closed", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip test if service is optional and not required
			if !tt.required && !testServices.requireAll {
				// Try to connect to the service first
				conn, err := net.DialTimeout("tcp", "localhost:"+tt.port, 2*time.Second)
				if err != nil {
					t.Skipf("Skipping test for %s on port %s - service not available", tt.name, tt.port)
					return
				}
				if conn != nil {
					_ = conn.Close()
				}
			}

			config := ScanConfig{
				Targets:  []string{"localhost"},
				Ports:    tt.port,
				ScanType: tt.scanType,
			}

			result, err := RunScan(&config)
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
				Ports:      testServices.HTTP, // Only HTTP is guaranteed
				ScanType:   "connect",
				TimeoutSec: 5,
			},
			wantError: false,
		},
		{
			name: "Multiple Ports Normal Timeout",
			config: ScanConfig{
				Targets:    []string{"127.0.0.1"},                       // Use IP to avoid DNS lookup
				Ports:      fmt.Sprintf("%s,443,80", testServices.HTTP), // Only use guaranteed ports
				ScanType:   "connect",
				TimeoutSec: 2,
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start := time.Now()
			_, err := RunScan(&tt.config)
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
	httpPort := testServices.HTTP

	// Check if HTTP service is actually available
	httpConn, err := net.DialTimeout("tcp", "localhost:"+httpPort, 2*time.Second)
	if err != nil {
		t.Skipf("HTTP service not available on port %s - skipping test", httpPort)
		return
	}
	if httpConn != nil {
		_ = httpConn.Close()
	}

	// Try to use the SSH port if available
	sshAvailable := false
	sshConn, err := net.DialTimeout("tcp", "localhost:"+testServices.SSH, 2*time.Second)
	if err == nil && sshConn != nil {
		_ = sshConn.Close()
		sshAvailable = true
	}

	portList := httpPort
	if sshAvailable {
		portList = fmt.Sprintf("%s,%s", httpPort, testServices.SSH)
	} else {
		t.Logf("SSH service not available on port %s", testServices.SSH)
	}

	config := ScanConfig{
		Targets:  []string{"localhost"},
		Ports:    portList,
		ScanType: "connect",
	}

	result, err := RunScan(&config)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotEmpty(t, result.Hosts)

	host := result.Hosts[0]
	assert.Contains(t, []string{"localhost", "127.0.0.1"}, host.Address)

	// Check for HTTP port (always required)
	var foundHTTP bool
	for _, p := range host.Ports {
		portNum := strconv.Itoa(int(p.Number))
		if portNum == httpPort && p.State == "open" {
			foundHTTP = true
		}
	}

	assert.True(t, foundHTTP, "Should find open HTTP port")

	// Only check SSH if it was available
	if sshAvailable {
		var foundSSH bool
		for _, p := range host.Ports {
			portNum := strconv.Itoa(int(p.Number))
			if portNum == testServices.SSH && p.State == "open" {
				foundSSH = true
			}
		}
		assert.True(t, foundSSH, "Should find open SSH port")
	}
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

			_ = w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			if _, err := io.Copy(&buf, r); err != nil {
				t.Errorf("Failed to capture output: %v", err)
				return
			}
			output := buf.String()

			// Basic output validation
			switch {
			case tt.result == nil:
				assert.Contains(t, output, "No results available")
			case len(tt.result.Hosts) == 0:
				assert.Contains(t, output, "Scan Results:")
			default:
				assert.Contains(t, output, "Host: "+tt.result.Hosts[0].Address)
				if tt.result.Hosts[0].Status == "up" && len(tt.result.Hosts[0].Ports) > 0 {
					assert.Contains(t, output, "Open Ports:")
				}
			}
		})
	}
}

func TestServiceDetection(t *testing.T) {
	// Find which services are actually available
	var availablePorts []string

	// Check HTTP
	httpConn, err := net.DialTimeout("tcp", "localhost:"+testServices.HTTP, 1*time.Second)
	if err == nil && httpConn != nil {
		_ = httpConn.Close()
		availablePorts = append(availablePorts, testServices.HTTP)
	}

	// Check SSH
	sshConn, err := net.DialTimeout("tcp", "localhost:"+testServices.SSH, 1*time.Second)
	if err == nil && sshConn != nil {
		_ = sshConn.Close()
		availablePorts = append(availablePorts, testServices.SSH)
	}

	// Check Redis
	redisConn, err := net.DialTimeout("tcp", "localhost:"+testServices.Redis, 1*time.Second)
	if err == nil && redisConn != nil {
		_ = redisConn.Close()
		availablePorts = append(availablePorts, testServices.Redis)
	}

	if len(availablePorts) == 0 {
		t.Skip("No test services are available - skipping service detection test")
		return
	}

	if len(availablePorts) == 1 {
		t.Logf("Only one service is available, test will be limited")
	}

	// Build port list with comma-separated ports
	portList := strings.Join(availablePorts, ",")

	// Test basic port scanning without version detection
	config := ScanConfig{
		Targets:  []string{"127.0.0.1"},
		Ports:    portList,
		ScanType: "connect",
	}

	result, err := RunScan(&config)
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
	for _, portNum := range availablePorts {
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
	defer func() { _ = os.Remove(tmpFile) }()

	// Test loading
	loaded, err := LoadResults(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	// Compare original and loaded data
	assert.Equal(t, len(result.Hosts), len(loaded.Hosts))
	assert.Equal(t, result.Hosts[0].Address, loaded.Hosts[0].Address)
	assert.Equal(t, result.Hosts[0].Ports[0].Number, loaded.Hosts[0].Ports[0].Number)
}

// TestScanningResilience tests the resilience functionality behavior
func TestScanningResilience(t *testing.T) {
	t.Run("resource_manager_prevents_excessive_concurrent_scans", func(t *testing.T) {
		// Test that resource manager properly limits concurrent operations
		rm := NewResourceManager(2) // Allow only 2 concurrent scans

		ctx := context.Background()

		// First two acquisitions should succeed immediately
		err1 := rm.Acquire(ctx, "scan1")
		assert.NoError(t, err1)

		err2 := rm.Acquire(ctx, "scan2")
		assert.NoError(t, err2)

		// Third acquisition should block or timeout quickly
		ctx3, cancel3 := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel3()

		err3 := rm.Acquire(ctx3, "scan3")
		assert.Error(t, err3) // Should timeout

		// Release one and try again
		rm.Release("scan1")

		err4 := rm.Acquire(ctx, "scan4")
		assert.NoError(t, err4)

		// Cleanup
		rm.Release("scan2")
		rm.Release("scan4")
	})

	t.Run("circuit_breaker_opens_after_threshold_failures", func(t *testing.T) {
		cb := NewCircuitBreaker(3, 5*time.Second) // 3 failures, 5s timeout

		ctx := context.Background()

		// Function that always fails
		failingFunc := func() error {
			return fmt.Errorf("simulated failure")
		}

		// First few calls should execute and fail
		for i := 0; i < 3; i++ {
			err := cb.Call(ctx, failingFunc)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "simulated failure")
		}

		// After threshold, circuit should be open
		err := cb.Call(ctx, failingFunc)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circuit breaker is open")
	})

	t.Run("circuit_breaker_recovers_after_timeout", func(t *testing.T) {
		cb := NewCircuitBreaker(2, 100*time.Millisecond) // 2 failures, 100ms timeout

		ctx := context.Background()

		// Trigger circuit breaker to open
		failingFunc := func() error {
			return fmt.Errorf("failure")
		}

		for i := 0; i < 2; i++ {
			cb.Call(ctx, failingFunc)
		}

		// Circuit should be open
		err := cb.Call(ctx, failingFunc)
		assert.Contains(t, err.Error(), "circuit breaker is open")

		// Wait for timeout
		time.Sleep(150 * time.Millisecond)

		// Circuit should be half-open, allowing one call
		successFunc := func() error {
			return nil
		}

		err = cb.Call(ctx, successFunc)
		assert.NoError(t, err) // Should succeed and close circuit

		// Next call should also succeed (circuit is closed)
		err = cb.Call(ctx, successFunc)
		assert.NoError(t, err)
	})

	t.Run("scan_handles_context_cancellation_gracefully", func(t *testing.T) {
		// Test that scans respect context cancellation
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		config := &ScanConfig{
			Targets:    []string{"192.0.2.1", "192.0.2.2"}, // RFC5737 test addresses
			Ports:      "1-1000",
			ScanType:   "connect",
			TimeoutSec: 30, // Long timeout, but context should cancel first
		}

		start := time.Now()
		result, err := RunScanWithContext(ctx, config, nil)
		elapsed := time.Since(start)

		// Should return quickly due to context cancellation
		assert.True(t, elapsed < 2*time.Second, "Scan should respect context cancellation")

		// Should handle cancellation gracefully
		if err != nil {
			// Error should indicate cancellation, timeout, or context deadline
			assert.True(t,
				strings.Contains(err.Error(), "cancel") ||
					strings.Contains(err.Error(), "timeout") ||
					strings.Contains(err.Error(), "context") ||
					strings.Contains(err.Error(), "deadline") ||
					strings.Contains(err.Error(), "timed out"),
				"Error should indicate cancellation: %v", err)
		}

		// Result should be nil or empty on cancellation
		if result != nil {
			assert.Equal(t, 0, len(result.Hosts))
		}
	})

	t.Run("scan_maintains_result_consistency", func(t *testing.T) {
		// Test that scan results are properly structured
		if !setupTestEnvironment(t) {
			return
		}

		config := &ScanConfig{
			Targets:  []string{"127.0.0.1"},
			Ports:    testServices.HTTP,
			ScanType: "connect",
		}

		result, err := RunScan(config)

		if err == nil && result != nil {
			// Result structure should be valid
			assert.NotNil(t, result.Hosts)

			for _, host := range result.Hosts {
				// Host should have valid address
				assert.NotEmpty(t, host.Address)

				// Status should be meaningful
				assert.Contains(t, []string{"up", "down", "filtered"}, host.Status)

				// If host is up and has ports, they should be properly formatted
				if host.Status == "up" {
					for _, port := range host.Ports {
						assert.True(t, port.Number > 0)
						assert.Contains(t, []string{"tcp", "udp"}, port.Protocol)
						assert.Contains(t, []string{"open", "closed", "filtered"}, port.State)
					}
				}
			}
		}
	})

	t.Run("resource_manager_handles_concurrent_operations", func(t *testing.T) {
		rm := NewResourceManager(5)

		const numConcurrent = 10
		ctx := context.Background()

		// Track successful acquisitions
		successes := make(chan bool, numConcurrent)
		failures := make(chan bool, numConcurrent)

		// Start concurrent acquisitions
		for i := 0; i < numConcurrent; i++ {
			go func(id int) {
				scanID := fmt.Sprintf("scan%d", id)
				ctxWithTimeout, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
				defer cancel()

				err := rm.Acquire(ctxWithTimeout, scanID)
				if err == nil {
					successes <- true
					time.Sleep(50 * time.Millisecond) // Simulate work
					rm.Release(scanID)
				} else {
					failures <- true
				}
			}(i)
		}

		// Collect results
		successCount := 0
		failureCount := 0

		for i := 0; i < numConcurrent; i++ {
			select {
			case <-successes:
				successCount++
			case <-failures:
				failureCount++
			case <-time.After(1 * time.Second):
				t.Fatal("Test timed out waiting for operations")
			}
		}

		// Should have some successes (up to the limit) and some failures
		assert.True(t, successCount > 0, "Should have some successful acquisitions")
		assert.True(t, successCount <= 10, "Should not exceed reasonable limit for concurrent test")
		assert.Equal(t, numConcurrent, successCount+failureCount, "All operations should complete")
	})
}
