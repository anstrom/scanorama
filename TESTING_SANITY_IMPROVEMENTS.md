# Testing Sanity Check Improvements

This document outlines the sanity check improvements made to the Scanorama test suite to make tests more reliable, faster, and easier to run in different environments.

## Overview

The test suite has been improved to handle various environments gracefully, reduce flaky tests, and provide better feedback when dependencies are not available.

## Key Improvements Made

### 1. Database Test Environment Detection

**Problem**: Tests were failing with long timeouts when PostgreSQL wasn't available, causing slow and confusing test runs.

**Solution**: Implemented graceful skipping and faster failure detection:

- Added `SKIP_DB_TESTS=true` environment variable to skip database-dependent tests
- Reduced database connection timeout from 10s to 2s for faster feedback
- Added quick port connectivity check before attempting database connections
- Improved CI environment detection

**Files Modified**:
- `internal/db/db_test.go`
- `internal/db/integration_validation_test.go`
- `internal/db/migration_test.go`
- `test/integration_test.go`
- `test/helpers/db.go`

### 2. Improved Test Skipping Logic

**Problem**: Tests used `t.Fatal()` when dependencies weren't available, causing test failures instead of graceful skipping.

**Solution**: Replaced fatal errors with appropriate skips:

```go
// Before
if testing.Short() {
    t.Fatal("Database tests cannot run in short mode")
}

// After
if testing.Short() {
    t.Skip("Skipping database tests in short mode. Run without -short flag or use make test-db")
    return
}
```

**Environment Variables**:
- `SKIP_DB_TESTS=true` - Skip all database-dependent tests
- `DB_DEBUG=true` - Enable verbose database connection debugging

### 3. Faster Connection Timeouts

**Problem**: Long timeouts (10+ seconds) made test failures slow and frustrating.

**Solution**: Implemented faster timeouts with intelligent detection:

- Database connection timeout: 10s → 2s
- Port connectivity check: 1s before attempting database connection
- Retry interval: 500ms → 200ms

### 4. Better Server Test Synchronization

**Problem**: Server tests used hardcoded `time.Sleep()` calls, making them flaky and unreliable.

**Solution**: Replaced sleeps with proper synchronization:

```go
// Before
go server.Start(context.Background())
time.Sleep(100 * time.Millisecond)
assert.True(t, server.IsRunning())

// After
go server.Start(context.Background())
// Wait for server to be ready with timeout
ready := make(chan bool, 1)
go func() {
    for i := 0; i < 50; i++ { // Max 500ms wait
        if server.IsRunning() {
            ready <- true
            return
        }
        time.Sleep(10 * time.Millisecond)
    }
    ready <- false
}()
```

### 5. Concurrent Test Performance

**Problem**: Concurrent tests weren't running in parallel, reducing test suite performance.

**Solution**: Added `t.Parallel()` to appropriate concurrent tests:

- `TestMiddleware_ConcurrentSafety`
- `TestConcurrentLogging`

## Usage Guide

### Running Tests in Different Modes

1. **Full test suite** (requires PostgreSQL):
   ```bash
   go test ./...
   ```

2. **Skip database tests**:
   ```bash
   SKIP_DB_TESTS=true go test ./...
   ```

3. **Short mode** (unit tests only):
   ```bash
   go test -short ./...
   ```

4. **CI environment** (strict requirements):
   ```bash
   CI=true go test ./...
   ```

5. **Debug database connections**:
   ```bash
   DB_DEBUG=true go test ./internal/db
   ```

### Environment Variables

| Variable | Effect | Default |
|----------|--------|---------|
| `SKIP_DB_TESTS` | Skip all database-dependent tests | `false` |
| `DB_DEBUG` | Enable verbose database connection logging | `false` |
| `CI` | Enable strict CI mode (fail if no DB) | `false` |
| `GITHUB_ACTIONS` | GitHub Actions CI detection | `false` |

### Test Categories

1. **Unit Tests**: No external dependencies, run with `-short`
2. **Integration Tests**: Require database, skip with `SKIP_DB_TESTS=true`
3. **Database Tests**: Require PostgreSQL, include migrations and schema validation
4. **Concurrent Tests**: Test thread safety, run with `t.Parallel()`

## CI/CD Integration

The improvements support different CI environments:

1. **GitHub Actions**: Automatic detection via `GITHUB_ACTIONS=true`
2. **Other CI**: Set `CI=true` for strict mode
3. **Local Development**: Graceful skipping when dependencies unavailable

## Best Practices for Future Tests

### 1. Environment Detection
Always check for dependencies and skip gracefully:

```go
func TestSomeDatabaseFeature(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping database tests in short mode")
        return
    }
    
    if os.Getenv("SKIP_DB_TESTS") == "true" {
        t.Skip("Database tests skipped via SKIP_DB_TESTS environment variable")
        return
    }
    
    // Test implementation...
}
```

### 2. Timeout Handling
Use reasonable timeouts with context:

```go
ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
defer cancel()
```

### 3. Resource Cleanup
Always provide cleanup functions:

```go
func setupTestResource(t *testing.T) (resource *Resource, cleanup func()) {
    // Setup code...
    return resource, func() {
        if err := resource.Close(); err != nil {
            t.Logf("Warning: Failed to close resource: %v", err)
        }
    }
}
```

### 4. Concurrent Tests
Use `t.Parallel()` for independent tests:

```go
func TestConcurrentOperation(t *testing.T) {
    t.Parallel() // Safe for concurrent execution
    // Test implementation...
}
```

## Testing the Improvements

Verify the improvements work correctly:

```bash
# Should pass quickly without PostgreSQL
SKIP_DB_TESTS=true go test ./... -timeout 30s

# Should skip database tests in short mode
go test -short ./... -timeout 10s

# Should provide debug output
DB_DEBUG=true SKIP_DB_TESTS=true go test ./internal/db -v

# Should run concurrent tests efficiently
go test -parallel 8 ./...
```

## Migration Notes

### For Developers

- Database tests now skip gracefully by default when PostgreSQL isn't available
- Use `SKIP_DB_TESTS=true` to explicitly skip database tests
- Use `DB_DEBUG=true` when troubleshooting database connectivity

### For CI/CD

- Set `CI=true` to enforce strict database requirements
- Database tests will fail in CI if PostgreSQL service isn't available
- Integration tests automatically detect nmap availability

## Future Improvements

1. **Docker Integration**: Automatic PostgreSQL container setup for tests
2. **Test Fixtures**: Shared test data management
3. **Performance Benchmarks**: Automated performance regression detection
4. **Coverage Targets**: Enforce minimum code coverage thresholds
5. **Flaky Test Detection**: Identify and quarantine unreliable tests

## Troubleshooting

### Common Issues

1. **"Database tests failed"**: Ensure PostgreSQL is running or use `SKIP_DB_TESTS=true`
2. **"Timeout waiting for database"**: Check PostgreSQL service status
3. **"Tests hanging"**: Check for infinite loops or missing timeouts
4. **"Flaky test failures"**: Review synchronization and timing assumptions

### Debug Commands

```bash
# Check which tests are being skipped
go test -v ./... | grep SKIP

# Run only database tests
go test ./internal/db -v

# Test database connectivity
DB_DEBUG=true go test ./internal/db/TestConnect -v

# Check test timing
go test -v ./... | grep -E "PASS|FAIL" | tail -20
```

This document should be updated as new testing improvements are implemented.