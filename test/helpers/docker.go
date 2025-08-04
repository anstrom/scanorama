// Package helpers provides test helper functions for Scanorama
package helpers

import (
	"fmt"
	"net"
	"testing"
	"time"
)

// TestService defines a service available in the Docker test environment
type TestService struct {
	Name string
	Port string
}

// TestEnvironment defines the test environment configuration
type TestEnvironment struct {
	Services      map[string]TestService
	ContainerName string
	Timeout       int
	RequireAll    bool
}

// DefaultTestEnvironment returns the default test environment configuration
func DefaultTestEnvironment() *TestEnvironment {
	return &TestEnvironment{
		Services: map[string]TestService{
			"HTTP":  {Name: "HTTP", Port: "8080"},
			"SSH":   {Name: "SSH", Port: "8022"},
			"HTTPS": {Name: "HTTPS", Port: "8443"},
			"Redis": {Name: "Redis", Port: "8379"},
			"Flask": {Name: "Flask", Port: "8888"},
		},
		ContainerName: "scanorama-test",
		Timeout:       5,
		RequireAll:    false,
	}
}

// CheckService attempts to connect to a service with retries
func (env *TestEnvironment) CheckService(t *testing.T, serviceName string, maxRetries int) bool {
	service, ok := env.Services[serviceName]
	if !ok {
		t.Logf("Service %s not defined in test environment", serviceName)
		return false
	}

	retryDelay := time.Second
	var connected bool

	for i := 0; i < maxRetries; i++ {
		conn, err := net.DialTimeout("tcp", "localhost:"+service.Port, 2*time.Second)
		if err == nil && conn != nil {
			conn.Close()
			connected = true
			break
		}
		if i < maxRetries-1 {
			t.Logf("Retrying connection to %s service on port %s (attempt %d/%d)",
				service.Name, service.Port, i+1, maxRetries)
			time.Sleep(retryDelay)
		}
	}

	if !connected {
		t.Logf("Service %s not available on port %s", service.Name, service.Port)
		return false
	}

	return true
}

// SetupTestEnvironment ensures the Docker test environment is running
// Returns true if all required services are available, false otherwise
func (env *TestEnvironment) SetupTestEnvironment(t *testing.T) bool {
	if testing.Short() {
		t.Skip("Skipping test that requires Docker services in short mode")
		return false
	}

	maxRetries := 5

	// Required services to check
	requiredServices := []string{"HTTP"} // HTTP is always required

	// Optional services to check if requireAll is true
	optionalServices := []string{"SSH", "Redis", "Flask"}

	if env.RequireAll {
		requiredServices = append(requiredServices, optionalServices...)
		optionalServices = []string{}
	}

	// Check required services
	for _, serviceName := range requiredServices {
		connected := env.CheckService(t, serviceName, maxRetries)
		if !connected {
			t.Skipf("Skipping test: required %s service not available", serviceName)
			return false
		}
	}

	// Check optional services (don't fail if not available)
	for _, serviceName := range optionalServices {
		if env.CheckService(t, serviceName, 1) {
			t.Logf("Optional service %s is available", serviceName)
		}
	}

	return true
}

// GetAvailableServices returns a list of available service ports
func (env *TestEnvironment) GetAvailableServices(t *testing.T) []string {
	availablePorts := []string{}

	for _, service := range env.Services {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%s", service.Port), 1*time.Second)
		if err == nil && conn != nil {
			conn.Close()
			availablePorts = append(availablePorts, service.Port)
			t.Logf("Service on port %s is available", service.Port)
		}
	}

	return availablePorts
}
