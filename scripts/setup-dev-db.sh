#!/bin/bash

# Scanorama Development Database Setup Script
# This script sets up a local PostgreSQL database for development

set -e  # Exit on any error

# Configuration
DB_NAME="scanorama_dev"
DB_USER="scanorama_dev"
DB_PASSWORD="dev_password"
DB_HOST="localhost"
DB_PORT="5432"
SCHEMA_FILE="internal/db/001_initial_schema.sql"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Helper functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if PostgreSQL is running
check_postgres() {
    log_info "Checking if PostgreSQL is running..."

    if ! command -v psql &> /dev/null; then
        log_error "PostgreSQL client (psql) not found. Please install PostgreSQL."
        echo "On macOS: brew install postgresql"
        echo "On Ubuntu/Debian: sudo apt-get install postgresql-client"
        echo "On RHEL/CentOS: sudo yum install postgresql"
        exit 1
    fi

    # Try to connect to PostgreSQL
    if ! pg_isready -h "$DB_HOST" -p "$DB_PORT" &> /dev/null; then
        log_error "PostgreSQL is not running on $DB_HOST:$DB_PORT"
        echo ""
        echo "Please start PostgreSQL. Common commands:"
        echo "On macOS (Homebrew): brew services start postgresql"
        echo "On Ubuntu/Debian: sudo service postgresql start"
        echo "On RHEL/CentOS: sudo systemctl start postgresql"
        echo ""
        echo "Alternatively, you can use Docker:"
        echo "docker run --name scanorama-postgres -e POSTGRES_PASSWORD=postgres -p 5432:5432 -d postgres:16-alpine"
        exit 1
    fi

    log_success "PostgreSQL is running"
}

# Check if schema file exists
check_schema_file() {
    if [ ! -f "$SCHEMA_FILE" ]; then
        log_error "Schema file not found: $SCHEMA_FILE"
        echo "Please run this script from the scanorama project root directory."
        exit 1
    fi
    log_info "Schema file found: $SCHEMA_FILE"
}

# Create database and user
create_database() {
    log_info "Creating database and user..."

    # Check if we can connect as postgres user (common default)
    POSTGRES_USER="postgres"
    if psql -h "$DB_HOST" -p "$DB_PORT" -U "$POSTGRES_USER" -c "SELECT 1;" &> /dev/null; then
        log_info "Connected as postgres user"
    else
        # Try current system user
        POSTGRES_USER="$USER"
        if ! psql -h "$DB_HOST" -p "$DB_PORT" -U "$POSTGRES_USER" -c "SELECT 1;" &> /dev/null; then
            log_error "Cannot connect to PostgreSQL. Please ensure you have access."
            echo "Try running: createuser -s $USER"
            exit 1
        fi
        log_info "Connected as $POSTGRES_USER user"
    fi

    # Check if database already exists
    if psql -h "$DB_HOST" -p "$DB_PORT" -U "$POSTGRES_USER" -lqt | cut -d \| -f 1 | grep -qw "$DB_NAME"; then
        log_warning "Database '$DB_NAME' already exists"
        read -p "Do you want to drop and recreate it? (y/N): " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            log_info "Dropping existing database..."
            psql -h "$DB_HOST" -p "$DB_PORT" -U "$POSTGRES_USER" -c "DROP DATABASE IF EXISTS $DB_NAME;"
            psql -h "$DB_HOST" -p "$DB_PORT" -U "$POSTGRES_USER" -c "DROP USER IF EXISTS $DB_USER;"
        else
            log_info "Keeping existing database"
            return 0
        fi
    fi

    # Create user
    log_info "Creating user '$DB_USER'..."
    psql -h "$DB_HOST" -p "$DB_PORT" -U "$POSTGRES_USER" -c "CREATE USER $DB_USER WITH PASSWORD '$DB_PASSWORD';"

    # Create database
    log_info "Creating database '$DB_NAME'..."
    psql -h "$DB_HOST" -p "$DB_PORT" -U "$POSTGRES_USER" -c "CREATE DATABASE $DB_NAME OWNER $DB_USER;"

    # Grant privileges
    log_info "Granting privileges..."
    psql -h "$DB_HOST" -p "$DB_PORT" -U "$POSTGRES_USER" -c "GRANT ALL PRIVILEGES ON DATABASE $DB_NAME TO $DB_USER;"

    log_success "Database and user created successfully"
}

# Run schema migrations
run_schema() {
    log_info "Running schema migrations..."

    export PGPASSWORD="$DB_PASSWORD"
    psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f "$SCHEMA_FILE"
    unset PGPASSWORD

    log_success "Schema migrations completed"
}

# Verify setup
verify_setup() {
    log_info "Verifying database setup..."

    export PGPASSWORD="$DB_PASSWORD"

    # Check if tables exist
    TABLE_COUNT=$(psql -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -t -c "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public';")

    unset PGPASSWORD

    if [ "$TABLE_COUNT" -gt 0 ]; then
        log_success "Database setup verified - $TABLE_COUNT tables created"
    else
        log_error "Database setup verification failed - no tables found"
        exit 1
    fi
}

# Create config file
create_config() {
    if [ ! -f "config.yaml" ]; then
        log_info "Creating config.yaml from config.dev.yaml..."
        cp config.dev.yaml config.yaml
        log_success "Created config.yaml"
    else
        log_info "config.yaml already exists, not overwriting"
    fi
}

# Test connection
test_connection() {
    log_info "Testing Scanorama connection to database..."

    if [ -f "./scanorama" ]; then
        if ./scanorama version &> /dev/null; then
            log_success "Scanorama can connect to the database"
        else
            log_warning "Scanorama binary exists but connection test failed"
            echo "Try running: make build"
        fi
    else
        log_info "Scanorama binary not found. Run 'make build' to build it."
    fi
}

# Main execution
main() {
    echo "==================================="
    echo "Scanorama Development Database Setup"
    echo "==================================="
    echo ""

    # Change to script directory, then go to project root
    cd "$(dirname "$0")/.."

    check_postgres
    check_schema_file
    create_database
    run_schema
    verify_setup
    create_config
    test_connection

    echo ""
    log_success "Development database setup complete!"
    echo ""
    echo "Database Details:"
    echo "  Host: $DB_HOST"
    echo "  Port: $DB_PORT"
    echo "  Database: $DB_NAME"
    echo "  Username: $DB_USER"
    echo "  Password: $DB_PASSWORD"
    echo ""
    echo "Next steps:"
    echo "  1. Build the application: make build"
    echo "  2. Test the setup: ./scanorama profiles list"
    echo "  3. Start scanning: ./scanorama discover 192.168.1.0/24"
    echo ""
}

# Run main function
main "$@"
