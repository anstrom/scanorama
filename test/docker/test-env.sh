#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.yml"
ENV_FILE="$SCRIPT_DIR/../.env.test"

# Check if running in CI environment
IS_CI="${GITHUB_ACTIONS:-false}"
if [ "$IS_CI" = "true" ]; then
  echo "Running in CI environment"
  # Use the port provided by GitHub Actions
  export POSTGRES_PORT="5432"
fi

# Create fixtures directory if it doesn't exist
FIXTURES_DIR="${PROJECT_ROOT}/test/fixtures"
if [ ! -d "$FIXTURES_DIR" ]; then
    echo "Creating fixtures directory at $FIXTURES_DIR"
    mkdir -p "$FIXTURES_DIR"
fi

# Create default database init script if it doesn't exist
if [ ! -f "${FIXTURES_DIR}/init.sql" ]; then
    echo "Creating default database init script"
    cat > "${FIXTURES_DIR}/init.sql" << EOF
-- Default database initialization script
-- This is created by the test-env.sh script
CREATE DATABASE IF NOT EXISTS scanorama_test;
GRANT ALL PRIVILEGES ON DATABASE scanorama_test TO test_user;
EOF
fi

# Load environment variables if file exists
if [[ -f "$ENV_FILE" ]]; then
    source "$ENV_FILE"
fi

# Make sure PATH includes Docker Desktop bin directory for credential helper
export PATH="/Applications/Docker.app/Contents/Resources/bin:$PATH"

usage() {
    echo "Test environment management script for Scanorama"
    echo
    echo "Usage: $0 <command>"
    echo "Commands:"
    echo "  up        - Start test environment"
    echo "  down      - Stop test environment and remove containers"
    echo "  restart   - Restart test environment"
    echo "  status    - Check status of test services"
    echo "  logs      - Show logs from test containers"
    echo "  shell     - Open shell in a specific container"
    echo "  recreate  - Force recreate containers"
    echo "  clean     - Remove containers, volumes, and networks"
    echo
    echo "Examples:"
    echo "  $0 up             # Start all services"
    echo "  $0 logs postgres  # Show postgres logs"
    echo "  $0 shell postgres # Open shell in postgres container"
}

docker_compose() {
    # Run from project root to ensure paths are correct
    cd "$PROJECT_ROOT"

    # Ensure we set the PostgreSQL port for local environments if not already set
    if [ -z "$POSTGRES_PORT" ]; then
        export POSTGRES_PORT="5432"
    fi

    # In CI environment, we need to handle services differently
    if [ "$IS_CI" = "true" ]; then
        if [ "$1" = "up" ]; then
            # In CI, don't start postgres since GitHub Actions provides it
            echo "CI mode: Skipping PostgreSQL service"
            shift
            docker compose -f "$COMPOSE_FILE" up -d --scale scanorama-postgres=0 "$@"
        else
            # For other commands in CI
            docker compose -f "$COMPOSE_FILE" "$@"
        fi
    else
        # Run all services locally
        docker compose -f "$COMPOSE_FILE" "$@"
    fi
}

up() {
    echo "Starting test environment..."

    # Print CI status
    if [ "$IS_CI" = "true" ]; then
        echo "CI mode: Using GitHub Actions PostgreSQL service on port $POSTGRES_PORT"
    fi
    # Ensure Flask app directory exists
    if [ ! -d "${PROJECT_ROOT}/test/docker/flask" ]; then
        echo "Creating Flask app directory"
        mkdir -p "${PROJECT_ROOT}/test/docker/flask"
    fi

    # Ensure Flask requirements directory exists
    if [ ! -d "${PROJECT_ROOT}/test/docker/flask/requirements" ]; then
        echo "Creating Flask requirements directory"
        mkdir -p "${PROJECT_ROOT}/test/docker/flask/requirements"
    fi

    # Create default Flask app if it doesn't exist
    if [ ! -f "${PROJECT_ROOT}/test/docker/flask/app.py" ]; then
        echo "Creating default Flask app"
        cat > "${PROJECT_ROOT}/test/docker/flask/app.py" << EOF
from flask import Flask, jsonify

app = Flask(__name__)

@app.route('/')
def index():
    return jsonify({
        "status": "ok",
        "service": "scanorama-test-flask"
    })

@app.route('/health')
def health():
    return jsonify({
        "status": "healthy",
        "version": "1.0.0"
    })

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000)
EOF
    fi

    # Create default Flask requirements if they don't exist
    if [ ! -f "${PROJECT_ROOT}/test/docker/flask/requirements/requirements.txt" ]; then
        echo "Creating default Flask requirements"
        cat > "${PROJECT_ROOT}/test/docker/flask/requirements/requirements.txt" << EOF
flask>=2.0.0,<3.0.0
werkzeug>=2.0.0,<3.0.0
EOF
    fi

    docker_compose up -d
    echo "Waiting for services to be ready..."
    sleep 5  # Basic delay to allow services to initialize

    # Check if postgres is ready
    # Only check PostgreSQL container if we're not in CI
    if [ "$IS_CI" != "true" ]; then
        for i in {1..30}; do
            if docker_compose exec scanorama-postgres pg_isready -U test_user -d scanorama_test &>/dev/null; then
                echo "PostgreSQL is ready on port $POSTGRES_PORT"
                break
            fi
            echo "Waiting for PostgreSQL to be ready ($i/30)..."
            sleep 1
        done
    else
        # In CI, check if the GitHub Actions PostgreSQL service is ready
        for i in {1..10}; do
            if psql -h localhost -p $POSTGRES_PORT -U test_user -d scanorama_test -c "SELECT 1" &>/dev/null; then
                echo "GitHub Actions PostgreSQL service is ready on port $POSTGRES_PORT"
                break
            fi
            echo "Waiting for GitHub Actions PostgreSQL to be ready ($i/10)..."
            sleep 1
        done
    fi

    echo "Test environment is ready"
}

down() {
    echo "Stopping test environment..."
    docker_compose down -v
}

status() {
    echo "Test services status:"
    docker_compose ps
}

logs() {
    if [ -z "$1" ]; then
        docker_compose logs
    else
        docker_compose logs "$1"
    fi
}

shell() {
    if [ -z "$1" ]; then
        echo "Error: Please specify a service name"
        echo "Available services:"
        docker_compose ps --services
        return 1
    fi

    # Use bash if available, otherwise use sh
    if docker_compose exec "$1" bash -c "exit" &>/dev/null; then
        docker_compose exec "$1" bash
    else
        docker_compose exec "$1" sh
    fi
}

recreate() {
    echo "Recreating test environment..."
    docker_compose up -d --force-recreate
    echo "Test environment recreated"
}

clean() {
    echo "Cleaning test environment..."
    docker_compose down -v --remove-orphans
    echo "Test environment cleaned"
}

restart() {
    down
    up
}

case "$1" in
    up) up ;;
    down) down ;;
    restart) restart ;;
    status) status ;;
    logs) logs "$2" ;;
    shell) shell "$2" ;;
    recreate) recreate ;;
    clean) clean ;;
    *) usage; exit 1 ;;
esac
