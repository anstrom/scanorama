# Scanorama Testing Framework

This directory contains the testing framework for the Scanorama project, including configurations, fixtures, and tools for running tests.

## Directory Structure

- **docker/** - Docker configuration for running tests with containerized services
- **fixtures/** - Test data and configuration files
- **.env.test** - Environment variables for testing

## Testing Approach

Scanorama uses a hybrid testing approach that allows tests to run in different environments:

1. **Local Development** - Tests can run with or without Docker, adapting to available services
2. **CI/CD Pipeline** - Tests run in a containerized environment with all required services

## Docker Test Environment

The Docker test environment provides:

- PostgreSQL 16 for database tests
- NGINX for HTTP service testing
- SSH server for SSH scanning tests
- Redis server for testing Redis scanning
- Flask application for API testing

### Usage

```bash
# Start the test environment
./test/docker/test-env.sh up

# Run tests
go test ./...

# Stop the test environment
./test/docker/test-env.sh down

# Get status of test containers
./test/docker/test-env.sh status

# View logs
./test/docker/test-env.sh logs [service_name]

# Open shell in a container
./test/docker/test-env.sh shell [service_name]
```

Or use the Makefile shortcuts:

```bash
# Run tests with Docker environment
make test

# Start test environment
make test-up

# Stop test environment
make test-down
```

## Test Configuration

### Database

Database configuration for tests is stored in `fixtures/database.yml`. It contains settings for:

- **test** - Local development testing
- **ci** - CI/CD pipeline testing
- **development** - Development environment

The test code will automatically detect the appropriate configuration based on the environment.

### Test Fixtures

Test fixtures in the `fixtures/` directory include:

- **db_schema.sql** - Database schema for test database initialization
- **database.yml** - Database configuration for different environments

## Graceful Degradation

Tests are designed to gracefully degrade when certain services aren't available:

- Database tests will skip if no database is available
- Scanning tests will run with available services and skip tests requiring unavailable services

This allows basic testing on development machines without requiring a full Docker setup.

## Adding New Tests

When adding new tests:

1. Use the existing test patterns
2. Make tests robust to missing services
3. Add any new fixtures to the `fixtures/` directory
4. Update Docker configuration if new services are required

## Continuous Integration

The testing framework is designed to work seamlessly with CI/CD pipelines. The Docker environment provides all necessary services for comprehensive testing.