package helpers

import (
	"testing"
	"time"
)

func TestDefaultTestEnvironment(t *testing.T) {
	env := DefaultTestEnvironment()

	// Check basic structure
	if env == nil {
		t.Fatal("DefaultTestEnvironment returned nil")
	}

	// Check required fields are set
	if env.ContainerName == "" {
		t.Error("ContainerName should not be empty")
	}

	if env.Timeout == 0 {
		t.Error("Timeout should not be zero")
	}

	// Check expected services are defined
	expectedServices := []string{"HTTP", "SSH", "HTTPS", "Redis"}
	for _, serviceName := range expectedServices {
		service, exists := env.Services[serviceName]
		if !exists {
			t.Errorf("Expected service %s not found", serviceName)
			continue
		}

		if service.Name != serviceName {
			t.Errorf("Service name mismatch: expected %s, got %s", serviceName, service.Name)
		}

		if service.Port == "" {
			t.Errorf("Service %s has empty port", serviceName)
		}
	}

	// Check specific port assignments
	expectedPorts := map[string]string{
		"HTTP":  "8080",
		"SSH":   "8022",
		"HTTPS": "8443",
		"Redis": "8379",
	}

	for serviceName, expectedPort := range expectedPorts {
		if service, exists := env.Services[serviceName]; exists {
			if service.Port != expectedPort {
				t.Errorf("Service %s: expected port %s, got %s", serviceName, expectedPort, service.Port)
			}
		}
	}

	// Check default values
	if env.RequireAll {
		t.Error("RequireAll should default to false")
	}

	if env.Timeout != defaultTestTimeoutSeconds {
		t.Errorf("Timeout should default to %d, got %d", defaultTestTimeoutSeconds, env.Timeout)
	}
}

func TestCheckService_NonExistentService(t *testing.T) {
	env := DefaultTestEnvironment()

	// Test with a service that doesn't exist
	connected := env.CheckService(t, "NonExistent", 1)
	if connected {
		t.Error("CheckService should return false for non-existent service")
	}
}

func TestCheckService_UnavailableService(t *testing.T) {
	env := DefaultTestEnvironment()

	// Test with a service that exists but is not available (port not listening)
	// Using a high port number that's unlikely to be in use
	env.Services["TestService"] = TestService{Name: "TestService", Port: "65432"}

	connected := env.CheckService(t, "TestService", 1)
	if connected {
		t.Error("CheckService should return false for unavailable service")
	}
}

func TestSetupTestEnvironment_AllServicesUnavailable(t *testing.T) {
	env := DefaultTestEnvironment()

	// All services should be unavailable since we're not running Docker containers
	result := env.SetupTestEnvironment(t)
	if result {
		t.Error("SetupTestEnvironment should return false when no services are available")
	}
}

func TestSetupTestEnvironment_RequireAll(t *testing.T) {
	env := DefaultTestEnvironment()
	env.RequireAll = true

	// With RequireAll=true, it should fail if any service is unavailable
	result := env.SetupTestEnvironment(t)
	if result {
		t.Error("SetupTestEnvironment with RequireAll=true should return false when services are unavailable")
	}
}

func TestGetAvailableServices_NoServices(t *testing.T) {
	env := DefaultTestEnvironment()

	// Since no services are actually running, this should return an empty slice
	availableServices := env.GetAvailableServices(t)
	if len(availableServices) > 0 {
		t.Errorf("Expected no available services, got %d", len(availableServices))
	}
}

func TestTestService_Structure(t *testing.T) {
	service := TestService{
		Name: "TestName",
		Port: "8080",
	}

	if service.Name != "TestName" {
		t.Errorf("Expected Name 'TestName', got '%s'", service.Name)
	}

	if service.Port != "8080" {
		t.Errorf("Expected Port '8080', got '%s'", service.Port)
	}
}

func TestTestEnvironment_Modification(t *testing.T) {
	env := DefaultTestEnvironment()
	originalServiceCount := len(env.Services)

	// Test that we can modify the environment
	env.Services["NewService"] = TestService{Name: "NewService", Port: "9999"}
	env.RequireAll = true
	env.Timeout = 10

	if len(env.Services) != originalServiceCount+1 {
		t.Errorf("Expected %d services after addition, got %d", originalServiceCount+1, len(env.Services))
	}

	if !env.RequireAll {
		t.Error("RequireAll should be true after modification")
	}

	if env.Timeout != 10 {
		t.Errorf("Expected Timeout 10, got %d", env.Timeout)
	}

	// Check the new service was added correctly
	newService, exists := env.Services["NewService"]
	if !exists {
		t.Error("New service was not added")
	} else if newService.Name != "NewService" || newService.Port != "9999" {
		t.Errorf("New service has incorrect values: Name=%s, Port=%s", newService.Name, newService.Port)
	}
}

func TestConstants(t *testing.T) {
	if defaultTestTimeoutSeconds <= 0 {
		t.Errorf("defaultTestTimeoutSeconds should be positive, got %d", defaultTestTimeoutSeconds)
	}

	if connectionTimeoutSeconds <= 0 {
		t.Errorf("connectionTimeoutSeconds should be positive, got %d", connectionTimeoutSeconds)
	}

	// Ensure the timeout values are reasonable
	if defaultTestTimeoutSeconds > 30 {
		t.Errorf("defaultTestTimeoutSeconds seems too high: %d", defaultTestTimeoutSeconds)
	}

	if connectionTimeoutSeconds > 10 {
		t.Errorf("connectionTimeoutSeconds seems too high: %d", connectionTimeoutSeconds)
	}
}

// Benchmark test for DefaultTestEnvironment creation
func BenchmarkDefaultTestEnvironment(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = DefaultTestEnvironment()
	}
}

// Test to ensure CheckService respects timeout behavior
func TestCheckService_Timeout(t *testing.T) {
	env := DefaultTestEnvironment()

	// Add a service with a port that will timeout
	env.Services["TimeoutTest"] = TestService{Name: "TimeoutTest", Port: "1"}

	start := time.Now()
	connected := env.CheckService(t, "TimeoutTest", 1)
	elapsed := time.Since(start)

	if connected {
		t.Error("CheckService should return false for timeout test")
	}

	// Should complete within reasonable time (allowing for some overhead)
	maxExpectedTime := time.Duration(connectionTimeoutSeconds+1) * time.Second
	if elapsed > maxExpectedTime {
		t.Errorf("CheckService took too long: %v (expected < %v)", elapsed, maxExpectedTime)
	}
}
