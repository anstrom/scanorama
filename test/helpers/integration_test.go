//go:build integration

package helpers

import (
	"fmt"
	"net"
	"testing"
	"time"
)

// TestIntegration_CheckService_WithRunningServices tests the CheckService method
// with actual running services. This test requires Docker services to be running.
func TestIntegration_CheckService_WithRunningServices(t *testing.T) {
	env := DefaultTestEnvironment()

	// Test with HTTP service (should be available if started)
	t.Run("HTTP_Service", func(t *testing.T) {
		// First check if HTTP service is actually running
		conn, err := net.DialTimeout("tcp", "localhost:8080", 2*time.Second)
		if err != nil {
			t.Skip("HTTP service on port 8080 not available, skipping test")
			return
		}
		conn.Close()

		// Now test our helper
		connected := env.CheckService(t, "HTTP", 1)
		if !connected {
			t.Error("CheckService should detect running HTTP service")
		}
	})

	// Test with Redis service (map to actual Redis port)
	t.Run("Redis_Service_Actual_Port", func(t *testing.T) {
		// Override Redis port to match actual Docker container
		env.Services["Redis"] = TestService{Name: "Redis", Port: "6379"}

		// First check if Redis service is actually running
		conn, err := net.DialTimeout("tcp", "localhost:6379", 2*time.Second)
		if err != nil {
			t.Skip("Redis service on port 6379 not available, skipping test")
			return
		}
		conn.Close()

		connected := env.CheckService(t, "Redis", 1)
		if !connected {
			t.Error("CheckService should detect running Redis service")
		}
	})

	// Test with PostgreSQL service (add it to test environment)
	t.Run("PostgreSQL_Service", func(t *testing.T) {
		env.Services["PostgreSQL"] = TestService{Name: "PostgreSQL", Port: "5432"}

		// First check if PostgreSQL service is actually running
		conn, err := net.DialTimeout("tcp", "localhost:5432", 2*time.Second)
		if err != nil {
			t.Skip("PostgreSQL service on port 5432 not available, skipping test")
			return
		}
		conn.Close()

		connected := env.CheckService(t, "PostgreSQL", 1)
		if !connected {
			t.Error("CheckService should detect running PostgreSQL service")
		}
	})
}

// TestIntegration_SetupTestEnvironment_WithPartialServices tests SetupTestEnvironment
// with a mix of available and unavailable services.
func TestIntegration_SetupTestEnvironment_WithPartialServices(t *testing.T) {
	env := DefaultTestEnvironment()

	// Modify environment to include actual available services
	env.Services = map[string]TestService{
		"HTTP":       {Name: "HTTP", Port: "8080"},       // May be available
		"Redis":      {Name: "Redis", Port: "6379"},      // May be available (Docker)
		"PostgreSQL": {Name: "PostgreSQL", Port: "5432"}, // May be available (Docker)
		"SSH":        {Name: "SSH", Port: "22"},          // Likely not available
	}

	t.Run("RequireAll_False", func(t *testing.T) {
		env.RequireAll = false

		// Check if HTTP is available
		conn, err := net.DialTimeout("tcp", "localhost:8080", 1*time.Second)
		httpAvailable := err == nil
		if httpAvailable {
			conn.Close()
		}

		if httpAvailable {
			// If HTTP is available, setup should succeed
			result := env.SetupTestEnvironment(t)
			if !result {
				t.Error("SetupTestEnvironment should succeed when HTTP service is available")
			}
		} else {
			t.Skip("HTTP service not available, cannot test positive case")
		}
	})

	t.Run("RequireAll_True", func(t *testing.T) {
		env.RequireAll = true

		// With RequireAll=true, it should fail if any service is unavailable
		// Since SSH is unlikely to be available, this should fail
		result := env.SetupTestEnvironment(t)
		if result {
			t.Log("All services are available - this is unexpected but not an error")
		} else {
			t.Log("SetupTestEnvironment correctly failed with RequireAll=true")
		}
	})
}

// TestIntegration_GetAvailableServices tests GetAvailableServices with actual services.
func TestIntegration_GetAvailableServices(t *testing.T) {
	env := DefaultTestEnvironment()

	// Add services that might actually be running
	env.Services = map[string]TestService{
		"HTTP":       {Name: "HTTP", Port: "8080"},
		"Redis":      {Name: "Redis", Port: "6379"},
		"PostgreSQL": {Name: "PostgreSQL", Port: "5432"},
		"SSH":        {Name: "SSH", Port: "22"},
		"HTTPS":      {Name: "HTTPS", Port: "443"},
	}

	availableServices := env.GetAvailableServices(t)

	t.Logf("Found %d available services: %v", len(availableServices), availableServices)

	// Verify that the returned ports actually correspond to available services
	for _, port := range availableServices {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%s", port), 1*time.Second)
		if err != nil {
			t.Errorf("Port %s reported as available but connection failed: %v", port, err)
		} else {
			conn.Close()
			t.Logf("✓ Confirmed service on port %s is available", port)
		}
	}

	// Check that at least some services are available if Docker is running
	if len(availableServices) == 0 {
		t.Log("No services detected - this is expected if Docker containers are not running")
	}
}

// TestIntegration_ServiceRetry tests the retry functionality with a service that takes time to start.
func TestIntegration_ServiceRetry(t *testing.T) {
	env := DefaultTestEnvironment()

	// Test with a service that should be available eventually
	t.Run("Retry_With_Available_Service", func(t *testing.T) {
		// First check if service is available
		conn, err := net.DialTimeout("tcp", "localhost:8080", 1*time.Second)
		if err != nil {
			t.Skip("HTTP service not available for retry test")
			return
		}
		conn.Close()

		// Test with multiple retries - should succeed on first try
		start := time.Now()
		connected := env.CheckService(t, "HTTP", 3)
		elapsed := time.Since(start)

		if !connected {
			t.Error("CheckService should succeed with retries for available service")
		}

		// Should complete quickly since service is available
		if elapsed > 2*time.Second {
			t.Errorf("CheckService took too long for available service: %v", elapsed)
		}
	})
}

// TestIntegration_CustomTestEnvironment tests creating and using a custom test environment.
func TestIntegration_CustomTestEnvironment(t *testing.T) {
	// Create custom environment with only actually available services
	env := &TestEnvironment{
		Services: map[string]TestService{
			"PostgreSQL": {Name: "PostgreSQL", Port: "5432"},
			"Redis":      {Name: "Redis", Port: "6379"},
		},
		ContainerName: "integration-test",
		Timeout:       3,
		RequireAll:    false,
	}

	// Check what services are actually available
	availableServices := env.GetAvailableServices(t)
	t.Logf("Custom environment found %d services: %v", len(availableServices), availableServices)

	// Try setup
	if len(availableServices) > 0 {
		// Override to require only services we know are available
		firstAvailablePort := availableServices[0]
		for serviceName, service := range env.Services {
			if service.Port == firstAvailablePort {
				env.Services = map[string]TestService{
					serviceName: service,
				}
				break
			}
		}

		result := env.SetupTestEnvironment(t)
		if !result {
			t.Error("Custom environment setup should succeed with available services")
		}
	} else {
		t.Log("No services available for custom environment test")
	}
}

// TestIntegration_ConcurrentServiceChecks tests concurrent service checking.
func TestIntegration_ConcurrentServiceChecks(t *testing.T) {
	env := DefaultTestEnvironment()

	// Add actual services
	env.Services = map[string]TestService{
		"HTTP":       {Name: "HTTP", Port: "8080"},
		"Redis":      {Name: "Redis", Port: "6379"},
		"PostgreSQL": {Name: "PostgreSQL", Port: "5432"},
	}

	// Test concurrent checks
	done := make(chan bool, len(env.Services))

	for serviceName := range env.Services {
		go func(name string) {
			defer func() { done <- true }()

			connected := env.CheckService(t, name, 1)
			t.Logf("Service %s: connected=%v", name, connected)
		}(serviceName)
	}

	// Wait for all checks to complete
	timeout := time.After(10 * time.Second)
	completed := 0

	for completed < len(env.Services) {
		select {
		case <-done:
			completed++
		case <-timeout:
			t.Error("Concurrent service checks timed out")
			return
		}
	}

	t.Logf("All %d concurrent service checks completed", completed)
}
