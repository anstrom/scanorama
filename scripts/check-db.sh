#!/bin/bash

# Database availability checker for Scanorama
# Returns 0 if a database is available, 1 if not

set -e

# Configuration
DB_HOST="localhost"
DB_PORT="5432"

# Development database config
DEV_DB_NAME="scanorama_dev"
DEV_DB_USER="scanorama_dev"
DEV_DB_PASSWORD="dev_password"

# Test database config
TEST_DB_NAME="scanorama_test"
TEST_DB_USER="test_user"
TEST_DB_PASSWORD="test_password"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1" >&2
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1" >&2
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1" >&2
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

# Check if PostgreSQL is running on the port
check_postgres_running() {
    if command -v nc >/dev/null 2>&1; then
        nc -z "$DB_HOST" "$DB_PORT" 2>/dev/null
    elif command -v telnet >/dev/null 2>&1; then
        timeout 2 telnet "$DB_HOST" "$DB_PORT" >/dev/null 2>&1
    else
        # Fallback: try to connect with psql
        timeout 2 psql -h "$DB_HOST" -p "$DB_PORT" -U postgres -d postgres -c "SELECT 1" >/dev/null 2>&1
    fi
}

# Test database connection
test_db_connection() {
    local db_name="$1"
    local db_user="$2"
    local db_password="$3"

    PGPASSWORD="$db_password" psql -h "$DB_HOST" -p "$DB_PORT" -U "$db_user" -d "$db_name" -c "SELECT 1" >/dev/null 2>&1
}

# Check schema exists
check_schema() {
    local db_name="$1"
    local db_user="$2"
    local db_password="$3"

    PGPASSWORD="$db_password" psql -h "$DB_HOST" -p "$DB_PORT" -U "$db_user" -d "$db_name" -t -c "
        SELECT COUNT(*)
        FROM information_schema.tables
        WHERE table_schema = 'public'
        AND table_name IN ('hosts', 'scan_jobs', 'port_scans')
    " 2>/dev/null | tr -d ' \t\n'
}

# Main function
main() {
    local quiet=false
    local verbose=false

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -q|--quiet)
                quiet=true
                shift
                ;;
            -v|--verbose)
                verbose=true
                shift
                ;;
            -h|--help)
                echo "Usage: $0 [OPTIONS]"
                echo "Check if Scanorama database is available"
                echo ""
                echo "Options:"
                echo "  -q, --quiet    Only output result, no status messages"
                echo "  -v, --verbose  Show detailed connection attempts"
                echo "  -h, --help     Show this help message"
                echo ""
                echo "Exit codes:"
                echo "  0 - Database is available and accessible"
                echo "  1 - No database found or not accessible"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                exit 1
                ;;
        esac
    done

    # Check if PostgreSQL is running
    if ! check_postgres_running; then
        if [ "$quiet" = false ]; then
            log_error "PostgreSQL is not running on $DB_HOST:$DB_PORT"
        fi
        exit 1
    fi

    if [ "$verbose" = true ]; then
        log_info "PostgreSQL is running on $DB_HOST:$DB_PORT"
    fi

    # Try development database first
    if test_db_connection "$DEV_DB_NAME" "$DEV_DB_USER" "$DEV_DB_PASSWORD"; then
        schema_count=$(check_schema "$DEV_DB_NAME" "$DEV_DB_USER" "$DEV_DB_PASSWORD")

        if [ "$schema_count" -ge 3 ]; then
            if [ "$quiet" = false ]; then
                log_success "Development database ($DEV_DB_NAME) is available with schema"
            fi
            echo "dev"
            exit 0
        else
            if [ "$verbose" = true ]; then
                log_warning "Development database accessible but schema incomplete ($schema_count/3 tables)"
            fi
        fi
    else
        if [ "$verbose" = true ]; then
            log_info "Development database ($DEV_DB_NAME) not accessible"
        fi
    fi

    # Try test database
    if test_db_connection "$TEST_DB_NAME" "$TEST_DB_USER" "$TEST_DB_PASSWORD"; then
        schema_count=$(check_schema "$TEST_DB_NAME" "$TEST_DB_USER" "$TEST_DB_PASSWORD")

        if [ "$schema_count" -ge 3 ]; then
            if [ "$quiet" = false ]; then
                log_success "Test database ($TEST_DB_NAME) is available with schema"
            fi
            echo "test"
            exit 0
        else
            if [ "$verbose" = true ]; then
                log_warning "Test database accessible but schema incomplete ($schema_count/3 tables)"
            fi
        fi
    else
        if [ "$verbose" = true ]; then
            log_info "Test database ($TEST_DB_NAME) not accessible"
        fi
    fi

    # No accessible database found
    if [ "$quiet" = false ]; then
        log_error "No accessible Scanorama database found"
        echo ""
        echo "Available options:"
        echo "  1. Start development database: make db-up"
        echo "  2. Start test containers: make test-up"
        echo "  3. Set up development database: make setup-dev-db"
    fi

    exit 1
}

main "$@"
