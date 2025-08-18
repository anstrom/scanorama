# Container-Based Database Testing Approach

This document outlines Scanorama's approach to database testing, which uses **real PostgreSQL containers** instead of complex database mocks to provide more reliable and realistic testing.

## Philosophy: Containers Over Mocks

### Why Containers Are Better Than Mocks for Database Testing

**✅ Advantages of Container-Based Testing:**
- **No Complex Mocking** - Database interactions are notoriously difficult to mock accurately
- **Real Behavior** - Tests actual SQL queries, constraints, transactions, and database-specific features
- **Catches Real Issues** - SQL syntax errors, constraint violations, type mismatches, performance issues
- **Less Maintenance** - No need to update mocks when database schema changes
- **Better Confidence** - Tests run against the same database engine as production
- **Real Migrations** - Uses actual migration files to create proper schema
- **True Isolation** - Each test gets a clean transaction that's automatically rolled back

**❌ Problems with Database Mocking:**
- Complex to set up and maintain
- Doesn't catch SQL syntax errors
- Can't test database-specific features (constraints, triggers, etc.)
- Becomes outdated when schema changes
- Performance characteristics are completely different
- Transaction behavior is artificial
- Complex queries are nearly impossible to mock correctly

**⚖️ Trade-offs:**
- Slightly slower than mocked unit tests (but still fast with containers)
- Requires Docker (which most development environments have)
- Uses more resources than pure unit tests

## How It Works

### CI Integration

**GitHub Actions Integration:**
The testing framework automatically detects CI environments (GitHub Actions) and prioritizes the PostgreSQL service container:

- **Service Container**: PostgreSQL 17 Alpine running on port 5432
- **Database**: `scanorama_test` with user `scanorama_test_user` 
- **Credentials**: Password `test_password_123` (matches workflow configuration)
- **Performance**: Optimized for testing with disabled fsync, fast checkpoints
- **Isolation**: Each workflow run gets a fresh database instance

**Configuration Priority:**
1. **CI Environment**: Service container config (localhost:5432) takes absolute priority
2. **Local Development**: Environment variables → config files → defaults
3. **Fallback Chain**: Multiple database configs tried in order until connection succeeds

### Container Management

The testing framework automatically:

1. **Detects Environment**: Checks for `GITHUB_ACTIONS=true` or `CI=true` environment variables
2. **Prioritizes Configs**: In CI, service container config always comes first
3. **Connects Intelligently**: Tries each configuration until finding a working database
### Test Isolation and Safety

**Transaction Rollback:**
- Each test runs in its own transaction
- Automatic rollback ensures no test data pollution
- Tests can run in parallel safely

**Hard Failures:**
- Tests fail immediately if database is unavailable (no silent skips)
- Clear error messages guide developers to proper setup
- CI environment validation ensures consistent behavior

**Schema Validation:**
- Migration checksums prevent schema drift
- Automatic validation of required tables and constraints
- Integration tests verify critical queries work correctly

### Make Targets

**Available Testing Commands:**

```bash
# Local development testing
make test-short              # Unit tests only (no database required)
make test-db                 # Database tests with local container
make test-integration        # Full integration tests with all services

# CI simulation and validation  
make test-ci                 # Simulate GitHub Actions CI environment
make test                    # Full test suite with database container

# Development setup
make setup-dev-db            # Set up local development database
```

**CI-Specific Testing:**
- `make test-ci` simulates the exact GitHub Actions environment
- Sets up CI environment variables and service container credentials
- Validates that CI detection and configuration priority work correctly
- Tests both the service container setup and migration system

### Configuration Detection

**Environment-Based Configuration:**

The system intelligently chooses database configuration based on the environment:

**In CI (GitHub Actions):**
```yaml
# Automatically detected configuration
Host: localhost
Port: 5432  
Database: scanorama_test
Username: scanorama_test_user
Password: test_password_123
```

**In Local Development:**
```yaml
# From environment variables or defaults
Host: ${TEST_DB_HOST:-localhost}
Port: ${TEST_DB_PORT:-5433}
Database: ${TEST_DB_NAME:-scanorama_test}  
Username: ${TEST_DB_USER:-test_user}
Password: ${TEST_DB_PASSWORD:-test_password}
```

**Debug Mode:**
Set `DB_DEBUG=true` to see configuration detection in action:
```bash
DB_DEBUG=true go test ./internal/db/ -run TestCI -v
```