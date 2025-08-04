#!/bin/bash

# Scanorama Database Test Environment
# Manages Docker containers for database testing without Docker Compose

set -e

# Configuration
POSTGRES_CONTAINER="scanorama-postgres-test"
POSTGRES_IMAGE="postgres:15-alpine"
POSTGRES_PORT="5433"
POSTGRES_DB="scanorama_test"
POSTGRES_USER="scanorama_test"
POSTGRES_PASSWORD="test_password"

TEST_CONTAINER="scanorama-test-runner"
TEST_IMAGE="golang:1.23-alpine"

NETWORK_NAME="scanorama-test-net"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')] $1${NC}"
}

success() {
    echo -e "${GREEN}✓ $1${NC}"
}

warn() {
    echo -e "${YELLOW}⚠ $1${NC}"
}

error() {
    echo -e "${RED}✗ $1${NC}"
    exit 1
}

# Check if Docker is available
check_docker() {
    if ! command -v docker &> /dev/null; then
        error "Docker is not installed or not in PATH"
    fi

    if ! docker info &> /dev/null; then
        error "Docker daemon is not running or not accessible"
    fi
}

# Create Docker network for tests
create_network() {
    if ! docker network ls | grep -q "$NETWORK_NAME"; then
        log "Creating Docker network: $NETWORK_NAME"
        docker network create "$NETWORK_NAME" --subnet=172.20.0.0/16
        success "Network created"
    else
        log "Network $NETWORK_NAME already exists"
    fi
}

# Remove Docker network
remove_network() {
    if docker network ls | grep -q "$NETWORK_NAME"; then
        log "Removing Docker network: $NETWORK_NAME"
        docker network rm "$NETWORK_NAME" 2>/dev/null || true
        success "Network removed"
    fi
}

# Start PostgreSQL container
start_postgres() {
    log "Starting PostgreSQL container..."

    # Stop existing container if running
    if docker ps -a --format "table {{.Names}}" | grep -q "^${POSTGRES_CONTAINER}$"; then
        log "Stopping existing PostgreSQL container"
        docker stop "$POSTGRES_CONTAINER" &>/dev/null || true
        docker rm "$POSTGRES_CONTAINER" &>/dev/null || true
    fi

    # Start new container
    docker run -d \
        --name "$POSTGRES_CONTAINER" \
        --network "$NETWORK_NAME" \
        -p "${POSTGRES_PORT}:5432" \
        -e POSTGRES_DB="$POSTGRES_DB" \
        -e POSTGRES_USER="$POSTGRES_USER" \
        -e POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
        -e POSTGRES_INITDB_ARGS="--encoding=UTF-8 --lc-collate=C --lc-ctype=C" \
        "$POSTGRES_IMAGE"

    success "PostgreSQL container started"

    # Wait for PostgreSQL to be ready
    log "Waiting for PostgreSQL to be ready..."
    for i in {1..30}; do
        if docker exec "$POSTGRES_CONTAINER" pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB" &>/dev/null; then
            success "PostgreSQL is ready"
            return 0
        fi
        echo -n "."
        sleep 1
    done

    error "PostgreSQL failed to start within 30 seconds"
}

# Stop PostgreSQL container
stop_postgres() {
    if docker ps --format "table {{.Names}}" | grep -q "^${POSTGRES_CONTAINER}$"; then
        log "Stopping PostgreSQL container"
        docker stop "$POSTGRES_CONTAINER" &>/dev/null
        docker rm "$POSTGRES_CONTAINER" &>/dev/null
        success "PostgreSQL container stopped"
    else
        warn "PostgreSQL container is not running"
    fi
}

# Get PostgreSQL connection details
get_db_env() {
    cat << EOF
SCANORAMA_DB_HOST=localhost
SCANORAMA_DB_PORT=$POSTGRES_PORT
SCANORAMA_DB_NAME=$POSTGRES_DB
SCANORAMA_DB_USER=$POSTGRES_USER
SCANORAMA_DB_PASSWORD=$POSTGRES_PASSWORD
SCANORAMA_DB_SSLMODE=disable
EOF
}

# Run database tests inside container
run_db_tests() {
    local test_type="${1:-unit}"

    log "Running database tests ($test_type)..."

    # Build test image if needed
    if ! docker images --format "table {{.Repository}}" | grep -q "scanorama-test"; then
        log "Building test image..."
        docker build -t scanorama-test -f test/docker/Dockerfile.testing .
    fi

    # Set test command based on type
    local test_cmd
    case "$test_type" in
        "unit")
            test_cmd="go test ./internal/db -v"
            ;;
        "integration")
            test_cmd="go test ./internal/db -tags=integration -v"
            ;;
        "migration")
            test_cmd="go test ./internal/db -tags=migration -v"
            ;;
        "all")
            test_cmd="go test ./... -v"
            ;;
        *)
            test_cmd="$test_type"
            ;;
    esac

    # Remove existing test container
    docker rm "$TEST_CONTAINER" &>/dev/null || true

    # Run tests
    docker run --rm \
        --name "$TEST_CONTAINER" \
        --network "$NETWORK_NAME" \
        -v "$(pwd):/app" \
        -w /app \
        -e SCANORAMA_DB_HOST="$POSTGRES_CONTAINER" \
        -e SCANORAMA_DB_PORT=5432 \
        -e SCANORAMA_DB_NAME="$POSTGRES_DB" \
        -e SCANORAMA_DB_USER="$POSTGRES_USER" \
        -e SCANORAMA_DB_PASSWORD="$POSTGRES_PASSWORD" \
        -e SCANORAMA_DB_SSLMODE=disable \
        -e GO_ENV=test \
        -e CGO_ENABLED=1 \
        scanorama-test \
        sh -c "CGO_ENABLED=1 $test_cmd"

    success "Database tests completed"
}

# Run shell in test environment
run_shell() {
    log "Starting interactive shell in test environment..."

    docker run --rm -it \
        --name "$TEST_CONTAINER" \
        --network "$NETWORK_NAME" \
        -v "$(pwd):/app" \
        -w /app \
        -e SCANORAMA_DB_HOST="$POSTGRES_CONTAINER" \
        -e SCANORAMA_DB_PORT=5432 \
        -e SCANORAMA_DB_NAME="$POSTGRES_DB" \
        -e SCANORAMA_DB_USER="$POSTGRES_USER" \
        -e SCANORAMA_DB_PASSWORD="$POSTGRES_PASSWORD" \
        -e SCANORAMA_DB_SSLMODE=disable \
        -e GO_ENV=test \
        scanorama-test \
        sh
}

# Connect to PostgreSQL directly
connect_db() {
    log "Connecting to PostgreSQL..."
    docker exec -it "$POSTGRES_CONTAINER" \
        psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"
}

# Show container status
status() {
    echo
    echo "=== Container Status ==="

    if docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep -q "$POSTGRES_CONTAINER"; then
        success "PostgreSQL: Running"
        docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" | grep "$POSTGRES_CONTAINER"
    else
        warn "PostgreSQL: Not running"
    fi

    if docker network ls | grep -q "$NETWORK_NAME"; then
        success "Network: $NETWORK_NAME exists"
    else
        warn "Network: $NETWORK_NAME does not exist"
    fi

    echo
    echo "=== Database Connection ==="
    echo "Host: localhost"
    echo "Port: $POSTGRES_PORT"
    echo "Database: $POSTGRES_DB"
    echo "Username: $POSTGRES_USER"
    echo "Password: $POSTGRES_PASSWORD"

    echo
    echo "=== Environment Variables ==="
    get_db_env
}

# Clean up all test containers and networks
cleanup() {
    log "Cleaning up test environment..."

    # Stop and remove containers
    docker stop "$POSTGRES_CONTAINER" &>/dev/null || true
    docker rm "$POSTGRES_CONTAINER" &>/dev/null || true
    docker rm "$TEST_CONTAINER" &>/dev/null || true

    # Remove network
    remove_network

    # Clean up volumes
    docker volume prune -f &>/dev/null || true

    success "Cleanup completed"
}

# Show usage information
usage() {
    cat << EOF
Scanorama Database Test Environment

USAGE:
    $0 <command> [options]

COMMANDS:
    start           Start PostgreSQL container and network
    stop            Stop PostgreSQL container
    test [type]     Run database tests
                    Types: unit, integration, migration, all
    shell           Start interactive shell in test environment
    db              Connect to PostgreSQL directly
    status          Show container and connection status
    cleanup         Stop all containers and remove network
    env             Show environment variables for local testing

EXAMPLES:
    $0 start                    # Start test database
    $0 test unit               # Run unit tests
    $0 test integration        # Run integration tests
    $0 shell                   # Interactive development
    $0 db                      # Connect to database
    $0 cleanup                 # Clean everything up

ENVIRONMENT:
    Set these variables for local testing (without Docker):
    $(get_db_env)

EOF
}

# Main command handling
main() {
    check_docker

    case "${1:-}" in
        "start")
            create_network
            start_postgres
            status
            ;;
        "stop")
            stop_postgres
            ;;
        "test")
            if ! docker ps --format "table {{.Names}}" | grep -q "^${POSTGRES_CONTAINER}$"; then
                log "PostgreSQL not running, starting it first..."
                create_network
                start_postgres
            fi
            run_db_tests "${2:-unit}"
            ;;
        "shell")
            if ! docker ps --format "table {{.Names}}" | grep -q "^${POSTGRES_CONTAINER}$"; then
                log "PostgreSQL not running, starting it first..."
                create_network
                start_postgres
            fi
            run_shell
            ;;
        "db")
            connect_db
            ;;
        "status")
            status
            ;;
        "cleanup")
            cleanup
            ;;
        "env")
            get_db_env
            ;;
        "help"|"-h"|"--help")
            usage
            ;;
        *)
            echo "Unknown command: ${1:-}"
            echo
            usage
            exit 1
            ;;
    esac
}

# Run main function with all arguments
main "$@"
