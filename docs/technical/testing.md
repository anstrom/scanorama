# Scanorama Testing Guide

This document provides a comprehensive guide to testing in the Scanorama project, covering test structure, running tests, and adding new tests.

## Test Structure

The Scanorama project uses a well-organized testing structure:

```
scanorama/
├── test/
│   ├── docker/           # Docker configuration for test services
│   │   ├── docker-compose.yml
│   │   ├── test-env.sh   # Script to manage test environment
│   │   └── flask/        # Test Flask application
│   ├── fixtures/         # Test data and configuration
│   │   ├── database.yml  # Database configuration
│   │   └── init.sql      # Database initialization schema
│   └── .env.test         # Environment variables for testing
├── internal/             # Internal packages with tests (test files alongside code)
└── docs/
    └── testing.md        # This document
```

Tests are primarily written using Go's standard testing package and follow these principles:

1. **Co-location**: Test files are located in the same package as the code they test, with a `_test.go` suffix
2. **Isolation**: Tests should not depend on each other
3. **Determinism**: Tests should be deterministic and repeatable
4. **Graceful degradation**: Tests should work with or without Docker services

## Running Tests

### Prerequisites

- Go 1.25.3 or higher
- Docker and Docker Compose (optional, for full test environment)
- Make (optional, for using Makefile commands)

### Basic Test Commands

```bash
# Run all tests
go test ./...

# Run tests for a specific package
go test ./internal/db

# Run a specific test
go test ./internal/db -run TestConnect

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -cover ./...
```

### Using Make Commands

The project includes several Make targets for testing:

```bash
# Run all tests with Docker test environment
make test

# Start Docker test environment
make test-up

# Stop Docker test environment
make test-down

# Generate test coverage report
make coverage

# Clean up test artifacts
make clean-test
```

### Manual Test Environment Management

You can manually manage the test environment using the test-env.sh script:

```bash
# Start test environment
./test/docker/test-env.sh up

# Check status of test services
./test/docker/test-env.sh status

# View logs
./test/docker/test-env.sh logs [service_name]

# Open shell in a container
./test/docker/test-env.sh shell [service_name]

# Stop test environment
./test/docker/test-env.sh down
```

## Docker Test Environment

The Docker test environment provides the following services:

| Service | Port | Description |
|---------|------|-------------|
| PostgreSQL | 5432 | Database for persistence tests |
| NGINX | 8080 | HTTP server for scanning tests |
| SSH | 8022 | SSH server for scanning tests |
| Redis | 8379 | Redis server for scanning tests |
| Flask | 8888 | Flask API for service detection tests |

These services are defined in `test/docker/docker-compose.yml` and can be managed using the `test-env.sh` script or Make commands.

## Writing Tests

### Test Naming Conventions

- Test functions should be named `Test<FunctionName>` or `Test<Behavior>`
- Table-driven test cases should have descriptive names
- Helper functions should be prefixed with `test` or have a clear purpose in their name

### Example Test Structure

```go
func TestSomething(t *testing.T) {
    // Arrange
    input := "test"
    expected := "result"
    
    // Act
    actual := DoSomething(input)
    
    // Assert
    if actual != expected {
        t.Errorf("Expected %s, got %s", expected, actual)
    }
}

func TestWithTableDriven(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected string
        wantErr  bool
    }{
        {
            name:     "normal case",
            input:    "test",
            expected: "result",
            wantErr:  false,
        },
        {
            name:     "error case",
            input:    "",
            expected: "",
            wantErr:  true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            actual, err := DoSomething(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("DoSomething() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if actual != tt.expected {
                t.Errorf("DoSomething() = %v, want %v", actual, tt.expected)
            }
        })
    }
}
```

### Testing Database Code

Database tests should follow these principles:

1. Use the test database configuration in `test/fixtures/database.yml`
2. Skip tests gracefully when the database is not available
3. Clean up after tests to leave the database in a consistent state
4. Use transactions where appropriate to isolate test data

Example database test:

```go
func TestDatabaseOperation(t *testing.T) {
    // Set up test database connection
    db, cleanup := setupTestDB(t)
    defer cleanup()
    
    // Skip if database not available
    if db == nil {
        return
    }
    
    // Test database operations
    // ...
}
```

### Testing Scanning Features

Scanning tests should follow these principles:

1. Detect available services and adapt tests accordingly
2. Skip tests that require unavailable services
3. Use predictable services in the Docker environment
4. Don't depend on external services that might be unavailable

Example scanning test:

```go
func TestScanning(t *testing.T) {
    // Check for HTTP service availability (minimum requirement)
    conn, err := net.DialTimeout("tcp", "localhost:8080", 2*time.Second)
    if err != nil {
        t.Skip("Skipping test: HTTP service not available")
        return
    }
    conn.Close()
    
    // Test scanning functionality
    // ...
}
```

## Continuous Integration

The project includes GitHub Actions workflows in `.github/workflows/` that run tests on every push and pull request. These workflows:

1. Set up the test environment using Docker Compose
2. Run linting checks
3. Run all tests
4. Generate and upload coverage reports

## Troubleshooting

### Docker Service Issues

If you encounter issues with Docker services:

1. Check the logs: `./test/docker/test-env.sh logs [service_name]`
2. Restart services: `./test/docker/test-env.sh restart`
3. Recreate containers: `./test/docker/test-env.sh recreate`
4. Check for port conflicts on your machine

### Test Failures

If tests are failing:

1. Run with verbose output: `go test -v ./...`
2. Check if services are available: `./test/docker/test-env.sh status`
3. Run specific test with verbose output: `go test -v ./path/to/package -run TestName`
4. Check for database issues: `./test/docker/test-env.sh shell scanorama-postgres`

## Best Practices

1. **Write both unit and integration tests**: Unit tests for isolated functionality, integration tests for components working together
2. **Keep tests fast**: Avoid unnecessary waits or slow operations in tests
3. **Don't test third-party code**: Focus on testing your own code, not libraries
4. **Use table-driven tests**: For testing multiple cases with the same logic
5. **Mock external dependencies**: Use interfaces and mock implementations for external services
6. **Test edge cases**: Include tests for error conditions and boundary values
7. **Maintain test independence**: Tests should not depend on each other's state
8. **Test public APIs**: Focus on testing public functions and methods
9. **Keep test code clean**: Apply the same code quality standards to tests as production code
10. **Update tests when changing code**: Tests should evolve with the codebase