#!/bin/bash

# Script to verify database test connection
# This helps identify configuration issues with the test database

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FIXTURES_DIR="${PROJECT_ROOT}/test/fixtures"
TEST_ENV_SCRIPT="${PROJECT_ROOT}/test/docker/test-env.sh"

echo "=== Database Test Configuration Checker ==="
echo "This script will verify your database test configuration"
echo

# Check if database.yml exists
if [ -f "${FIXTURES_DIR}/database.yml" ]; then
    echo "✅ Found database.yml configuration file"
    cat "${FIXTURES_DIR}/database.yml"
    echo
else
    echo "❌ database.yml not found in fixtures directory"
    echo "Expected location: ${FIXTURES_DIR}/database.yml"
    exit 1
fi

# Check if we're running in CI
IS_CI="${GITHUB_ACTIONS:-false}"
if [ "$IS_CI" = "true" ]; then
    echo "Running in CI environment"
    # This should use CI-specific configuration
else
    echo "Running in local environment"
    # This should use local-specific configuration
fi

# Check PostgreSQL port configuration
POSTGRES_PORT="${POSTGRES_PORT:-5432}"
echo "Current PostgreSQL port: $POSTGRES_PORT"

# Check if test environment script exists
if [ -f "$TEST_ENV_SCRIPT" ]; then
    echo "✅ Found test environment script"
else
    echo "❌ Test environment script not found"
    echo "Expected location: $TEST_ENV_SCRIPT"
    exit 1
fi

# Try to start test environment
echo
echo "Starting test environment..."
$TEST_ENV_SCRIPT up

echo
echo "Checking database connectivity..."
cd "$PROJECT_ROOT"

# Run a simple database test
go test ./internal/db -v -run TestPing

# Clean up
echo
echo "Cleaning up test environment..."
$TEST_ENV_SCRIPT down

echo
echo "Test complete"
