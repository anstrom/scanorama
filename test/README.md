# Scanorama Testing Framework

This directory contains the testing framework for the Scanorama project, featuring a **container-based testing approach** that eliminates complex database mocking and provides reliable, realistic testing against actual database instances.

## Testing Philosophy: Containers Over Mocks

Scanorama uses **real database containers** for testing instead of complex mocks because:

✅ **No Complex Mocking** - Database interactions are notoriously difficult to mock accurately  
✅ **Real Behavior** - Tests actual SQL queries, constraints, transactions, and database-specific features  
✅ **Catches Real Issues** - SQL syntax errors, constraint violations, type mismatches  
✅ **Less Maintenance** - No need to update mocks when database schema changes  
✅ **Better Confidence** - Tests run against the same database engine as production  
✅ **Real Migrations** - Uses actual migration files to create proper schema

## Directory Structure

```
test/
├── README.md                    # This file
├── docker/                     # Docker configurations for test services
│   ├── docker-compose.yml      # Standard test services
│   ├── docker-compose.test.yml # Optimized test database with custom ports
│   └── test-env.sh             # Test environment management script
├── fixtures/                   # Test data and SQL files
│   ├── 00-create-user.sql      # Database user creation
│   ├── init.sql                # Database initialization
│   └── database.yml            # Database configurations
├── helpers/                    # Test utilities and helpers
│   ├── db.go                   # Database connection helpers
│   ├── testdb.go               # Container-based test database utilities
│   ├── docker.go               # Docker management utilities
│   └── testing.go              # General testing utilities
├── integration_test.go         # Integration tests
└── benchmark_test.go           # Performance benchmarks
```

## Quick Start

### 1. Run Database Tests

```bash
# Run database tests (automatically manages containers)
make test-db

# Run all tests with database container
make test

# Run unit tests only (no database)
make test-short
```

### 2. Run Integration Tests

```bash
# Run integration tests with all services
make test-integration

# Or manually with Docker Compose
docker-compose -f test/docker/docker-compose.test.yml up -d
go test -tags=integration ./...
docker-compose -f test/docker/docker-compose.test.yml down
```

## Container-Based Database Testing

### Simple Test Helpers

The test framework provides simple helpers that handle all the complexity:

```go
// WithTestTx automatically:
// 1. Sets up test database (container or CI service)
// 2. Runs all migrations to create proper schema
// 3. Starts a transaction for isolation
// 4. Automatically rolls back after test
func TestCreateHost(t *testing.T) {
    db.WithTestTx(t, func(t *testing.T, tx *db.TestTx) {
        ctx := context.Background()
        
        // Test with real database and schema
        var hostID uuid.UUID
        err := tx.QueryRowContext(ctx, `
            INSERT INTO hosts (ip_address, hostname, status)
            VALUES ($1, $2, $3)
            RETURNING id
        `, "192.168.1.100", "test.example.com", "up").Scan(&hostID)
        
        require.NoError(t, err)
        assert.NotEqual(t, uuid.Nil, hostID)
    })
    // Transaction automatically rolled back - no cleanup needed!
}
```

### Key Benefits Over Mocking

1. **Real Schema**: Uses actual migration files, not hardcoded test schema
2. **Real Constraints**: Test actual foreign key constraints, unique constraints, and check constraints
3. **Complex Queries**: Test complex JOINs, CTEs, and aggregations that are hard to mock
4. **Concurrency**: Test real database locking and transaction behavior
5. **Performance**: Benchmark actual database performance, not mock overhead
6. **Schema Evolution**: Tests automatically pick up schema changes from migrations

## Make Targets

Simple make targets handle all database testing:

```bash
# Database Tests
make test-db          # Run database tests only
make test             # Run all tests with database container
make test-short       # Run unit tests only (no database)
make test-integration # Run integration tests with all services

# Clean up
make clean            # Clean temporary files
```

### Environment Variables

```bash
# Database Configuration (optional - auto-detected if not set)
export TEST_DB_HOST=localhost       # Database host
export TEST_DB_PORT=5433            # Database port
export TEST_DB_NAME=scanorama_test   # Database name
export TEST_DB_USER=test_user        # Database user
export TEST_DB_PASSWORD=test_password # Database password
```

## CI/CD Integration

### GitHub Actions

The framework seamlessly integrates with CI service containers:

```yaml
services:
  postgres:
    image: postgres:17-alpine
    env:
      POSTGRES_DB: scanorama_test
      POSTGRES_USER: test_user
      POSTGRES_PASSWORD: test_password
    ports:
      - 5432:5432
    options: >-
      --health-cmd "pg_isready -U test_user -d scanorama_test"
      --health-interval 10s
      --health-timeout 5s
      --health-retries 5
```

Tests automatically detect and use service containers when available.

## Writing Container-Based Tests

### Database Tests (Recommended Pattern)

```go
// Test with automatic transaction isolation and real schema
func TestCreateHost_WithRealDatabase(t *testing.T) {
    db.WithTestTx(t, func(t *testing.T, tx *db.TestTx) {
        ctx := context.Background()
        
        // Uses real schema from migration files
        var hostID uuid.UUID
        err := tx.QueryRowContext(ctx, `
            INSERT INTO hosts (ip_address, hostname, status, first_seen, last_seen)
            VALUES ($1, $2, $3, $4, $5)
            RETURNING id
        `, "192.168.1.100", "test.example.com", "up", time.Now(), time.Now()).Scan(&hostID)
        
        require.NoError(t, err)
        assert.NotEqual(t, uuid.Nil, hostID)
        
        // Test constraints work
        _, err = tx.ExecContext(ctx, `
            INSERT INTO hosts (ip_address, hostname, status)
            VALUES ($1, $2, $3)
        `, "192.168.1.100", "duplicate.example.com", "up") // Same IP
        assert.Error(t, err) // Should fail due to unique constraint
    })
    // Transaction automatically rolled back - no cleanup needed!
}
```

### Testing Complex Queries

```go
func TestScanJobSummary_WithRealDatabase(t *testing.T) {
    db.WithTestTx(t, func(t *testing.T, tx *db.TestTx) {
        ctx := context.Background()
        
        // Set up test data using real schema
        // ... insert hosts, scan_jobs, port_scans ...
        
        // Test

## Local Development Workflow

### First Time Setup

```bash
# Clone and enter the project
git clone https://github.com/anstrom/scanorama.git
cd scanorama

# Make sure Docker is running
docker --version

# Start test database
./scripts/test-db.sh start

# Run database tests
go test ./internal/db/...
```

### Daily Development

```bash
# Check if test database is running
./scripts/test-db.sh status

# Run tests (starts database if needed)
make test

# Run specific database tests
go test -v ./internal/db/repository_test.go

# Clean up when done
./scripts/test-db.sh cleanup
```

## Test Data Management

### Fixtures

Test fixtures are located in `test/fixtures/`:

- `init.sql` - Database schema initialization
- `00-create-user.sql` - User creation script
- `database.yml` - Database configuration for different environments

### Isolation Strategy

Tests use **transaction-based isolation**:

1. Each test gets a fresh transaction
2. All changes are automatically rolled back
3. No test data pollution between tests
4. Fast execution (no database recreation)

## Performance Optimizations

The test database containers are optimized for speed:

```bash
# PostgreSQL test optimizations
-c fsync=off                    # Disable fsync for speed
-c synchronous_commit=off       # Async commits
-c full_page_writes=off         # Disable full page writes
-c shared_buffers=256MB         # Increase buffer size
--tmpfs /var/lib/postgresql/data # Use in-memory storage
```

## Troubleshooting

### Common Issues

**Port already in use:**
```bash
./scripts/test-db.sh start --port 5435  # Try different port
```

**Database not ready:**
```bash
./scripts/test-db.sh status              # Check status
./scripts/test-db.sh restart             # Restart if needed
```

**Docker issues:**
```bash
docker system prune                      # Clean up Docker
./scripts/test-db.sh cleanup             # Clean up test resources
```

**Permission issues:**
```bash
chmod +x scripts/test-db.sh             # Make script executable
```

### Debug Mode

```bash
export DB_DEBUG=true                     # Enable debug logging
./scripts/test-db.sh start               # Start with debug output
```

## Best Practices

### DO ✅

- Use `helpers.TestWithDatabase()` for isolated database tests
- Test real SQL constraints and database features
- Use transactions for test isolation
- Test complex queries that would be hard to mock
- Benchmark against real database performance

### DON'T ❌

- Mock database interfaces when you can use a real database
- Share test data between tests (use transactions for isolation)
- Hardcode database ports (use auto-detection)
- Skip database tests in short mode (they're fast with containers)

## Adding New Test Services

To add a new service to the test environment:

1. Add service to `test/docker/docker-compose.test.yml`
2. Update `scripts/test-db.sh` if needed
3. Add helper functions in `test/helpers/`
4. Create integration tests in appropriate packages

Example service addition:

```yaml
# In docker-compose.test.yml
test-mongodb:
  image: mongo:7
  container_name: scanorama-test-mongo
  ports:
    - "${TEST_MONGO_PORT:-27018}:27017"
  environment:
    MONGO_INITDB_DATABASE: scanorama_test
```

This container-based approach provides reliable, fast, and realistic testing that scales from local development to production CI/CD pipelines.