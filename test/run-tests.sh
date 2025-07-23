#!/bin/bash

# Script to run tests with Docker test environment
# Usage: ./run-tests.sh [--no-docker] [test_packages...]

set -e

# Default values
USE_DOCKER=true
TEST_PACKAGES=("./...")
DOCKER_DIR="$(dirname "$0")/docker"
TEST_ENV_SCRIPT="$DOCKER_DIR/test-env.sh"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --no-docker)
            USE_DOCKER=false
            shift
            ;;
        *)
            TEST_PACKAGES=("$@")
            break
            ;;
    esac
done

# Function to clean up Docker environment
cleanup() {
    if [ "$USE_DOCKER" = true ]; then
        echo "Cleaning up Docker environment..."
        "$TEST_ENV_SCRIPT" stop
    fi
}

# Register cleanup handler
trap cleanup EXIT

# Start Docker environment if needed
if [ "$USE_DOCKER" = true ]; then
    if [ ! -x "$TEST_ENV_SCRIPT" ]; then
        echo "Error: Test environment script not found or not executable: $TEST_ENV_SCRIPT"
        exit 1
    fi

    echo "Setting up Docker test environment..."
    "$TEST_ENV_SCRIPT" build
    "$TEST_ENV_SCRIPT" start

    # Check if services are running
    if ! "$TEST_ENV_SCRIPT" test; then
        echo "Error: Test environment services not running properly"
        "$TEST_ENV_SCRIPT" logs
        exit 1
    fi
fi

# Run the tests
echo "Running tests..."
if [ "${#TEST_PACKAGES[@]}" -eq 0 ]; then
    TEST_PACKAGES=("./...")
fi

# Add -v flag for verbose output in CI environment
if [ -n "$CI" ]; then
    go test -v "${TEST_PACKAGES[@]}"
else
    go test "${TEST_PACKAGES[@]}"
fi

# Show success message
echo "All tests passed successfully!"

# Clean up unless --no-docker was specified
if [ "$USE_DOCKER" = true ]; then
    echo "Cleaning up test environment..."
    "$TEST_ENV_SCRIPT" stop
fi
