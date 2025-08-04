# Test Helpers for Scanorama

This package provides common test helper functions for Scanorama tests.

## Docker Environment Helpers

The `docker.go` module provides utilities for interacting with the Docker test environment:

- `TestEnvironment`: Configuration for the test environment and available services
- `SetupTestEnvironment`: Ensures Docker services are running and available
- `CheckService`: Tests connectivity to a specific service
- `GetAvailableServices`: Returns a list of all available service ports

## Usage Example

```go
import (
    "testing"
    
    "github.com/anstrom/scanorama/test/helpers"
)

func TestWithDocker(t *testing.T) {
    // Get default test environment
    env := helpers.DefaultTestEnvironment()
    
    // Skip the test if Docker environment is not ready
    if !env.SetupTestEnvironment(t) {
        return // Test was skipped
    }
    
    // Get list of available services
    availablePorts := env.GetAvailableServices(t)
    
    // Use services in your test...
}
```

## Adding New Helpers

When adding new helper functions:

1. Group related functionality in appropriate files
2. Add comprehensive documentation with examples
3. Ensure helpers are generic enough to be reused across tests
4. Update this README with information about new helpers