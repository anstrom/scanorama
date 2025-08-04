#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.yml"
ENV_FILE="$SCRIPT_DIR/../.env.test"

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
    docker compose -f "$COMPOSE_FILE" "$@"
}

up() {
    echo "Starting test environment..."
    docker_compose up -d
    echo "Waiting for services to be ready..."
    sleep 5  # Basic delay to allow services to initialize

    # Check if postgres is ready
    for i in {1..30}; do
        if docker_compose exec scanorama-postgres pg_isready -U test_user -d scanorama_test &>/dev/null; then
            echo "PostgreSQL is ready"
            break
        fi
        echo "Waiting for PostgreSQL to be ready ($i/30)..."
        sleep 1
    done

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
