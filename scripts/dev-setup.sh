#!/bin/bash

# Scanorama Development Setup Script
# This script sets up a complete development environment for Scanorama

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Script configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MIN_GO_VERSION="1.21"
MIN_NODE_VERSION="18"

# Logging functions
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

log_step() {
    echo -e "${PURPLE}[STEP]${NC} $1"
}

log_command() {
    echo -e "${CYAN}[CMD]${NC} $1"
}

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to compare versions
version_greater_equal() {
    printf '%s\n%s\n' "$2" "$1" | sort -V -C
}

# Function to get Go version
get_go_version() {
    go version | sed 's/go version go\([0-9.]*\).*/\1/'
}

# Function to get Node version
get_node_version() {
    node --version | sed 's/v\([0-9.]*\).*/\1/'
}

# Function to check prerequisites
check_prerequisites() {
    log_step "Checking prerequisites..."

    local missing_deps=()
    local warning_msgs=()

    # Check Go
    if command_exists go; then
        local go_version=$(get_go_version)
        if version_greater_equal "$go_version" "$MIN_GO_VERSION"; then
            log_success "Go $go_version is installed (minimum: $MIN_GO_VERSION)"
        else
            log_error "Go version $go_version is too old (minimum: $MIN_GO_VERSION)"
            missing_deps+=("go")
        fi
    else
        log_error "Go is not installed"
        missing_deps+=("go")
    fi

    # Check Docker
    if command_exists docker; then
        if docker info >/dev/null 2>&1; then
            log_success "Docker is installed and running"
        else
            log_warning "Docker is installed but not running"
            warning_msgs+=("Please start Docker Desktop or Docker daemon")
        fi
    else
        log_error "Docker is not installed"
        missing_deps+=("docker")
    fi

    # Check Node.js (for documentation tools)
    if command_exists node; then
        local node_version=$(get_node_version)
        if version_greater_equal "$node_version" "$MIN_NODE_VERSION"; then
            log_success "Node.js $node_version is installed (minimum: $MIN_NODE_VERSION)"
        else
            log_warning "Node.js version $node_version is older than recommended ($MIN_NODE_VERSION)"
            warning_msgs+=("Consider upgrading Node.js for better documentation tools support")
        fi
    else
        log_warning "Node.js is not installed (required for documentation tools)"
        missing_deps+=("node")
    fi

    # Check npm
    if command_exists npm; then
        log_success "npm is available"
    else
        log_warning "npm is not available"
        if [[ ! " ${missing_deps[@]} " =~ " node " ]]; then
            missing_deps+=("npm")
        fi
    fi

    # Check make
    if command_exists make; then
        log_success "make is available"
    else
        log_error "make is not installed"
        missing_deps+=("make")
    fi

    # Check git
    if command_exists git; then
        log_success "git is available"
    else
        log_error "git is not installed"
        missing_deps+=("git")
    fi

    # Optional tools
    if command_exists psql; then
        log_success "PostgreSQL client (psql) is available"
    else
        log_info "PostgreSQL client (psql) is not installed (optional, for database management)"
    fi

    if command_exists curl; then
        log_success "curl is available"
    else
        log_warning "curl is not installed (recommended for API testing)"
    fi

    if command_exists jq; then
        log_success "jq is available"
    else
        log_info "jq is not installed (optional, for JSON processing)"
    fi

    # Report missing dependencies
    if [ ${#missing_deps[@]} -gt 0 ]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        echo ""
        echo "Please install the missing dependencies:"
        echo ""

        for dep in "${missing_deps[@]}"; do
            case $dep in
                go)
                    echo "  Go: https://golang.org/doc/install"
                    ;;
                docker)
                    echo "  Docker: https://docs.docker.com/get-docker/"
                    ;;
                node)
                    echo "  Node.js: https://nodejs.org/en/download/"
                    ;;
                npm)
                    echo "  npm: Usually comes with Node.js"
                    ;;
                make)
                    echo "  make: Install build-essential (Linux) or Xcode Command Line Tools (macOS)"
                    ;;
                git)
                    echo "  git: https://git-scm.com/downloads"
                    ;;
            esac
        done

        echo ""
        echo "Run this script again after installing the missing dependencies."
        exit 1
    fi

    # Show warnings
    if [ ${#warning_msgs[@]} -gt 0 ]; then
        echo ""
        for msg in "${warning_msgs[@]}"; do
            log_warning "$msg"
        done
        echo ""
    fi

    log_success "All prerequisites checked!"
}

# Function to set up Go environment
setup_go_environment() {
    log_step "Setting up Go environment..."

    cd "$PROJECT_ROOT"

    # Download dependencies
    log_command "go mod download"
    go mod download

    # Tidy up
    log_command "go mod tidy"
    go mod tidy

    # Install development tools
    log_info "Installing Go development tools..."

    # golangci-lint for linting
    if ! command_exists golangci-lint; then
        log_command "Installing golangci-lint..."
        curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b "$(go env GOPATH)/bin" latest
    else
        log_success "golangci-lint already installed"
    fi

    # govulncheck for security scanning
    if ! command_exists govulncheck; then
        log_command "go install golang.org/x/vuln/cmd/govulncheck@latest"
        go install golang.org/x/vuln/cmd/govulncheck@latest
    else
        log_success "govulncheck already installed"
    fi

    # swag for API documentation
    if ! command_exists swag; then
        log_command "go install github.com/swaggo/swag/cmd/swag@latest"
        go install github.com/swaggo/swag/cmd/swag@latest
    else
        log_success "swag already installed"
    fi

    log_success "Go environment setup complete!"
}

# Function to set up Node.js environment
setup_node_environment() {
    log_step "Setting up Node.js environment..."

    if ! command_exists node; then
        log_warning "Node.js not available, skipping Node.js setup"
        return
    fi

    cd "$PROJECT_ROOT"

    # Install npm dependencies
    if [ -f "package.json" ]; then
        log_command "npm install"
        npm install
        log_success "npm dependencies installed"
    else
        log_info "No package.json found, skipping npm install"
    fi
}

# Function to set up development database
setup_database() {
    log_step "Setting up development database..."

    if ! command_exists docker; then
        log_warning "Docker not available, skipping database setup"
        log_info "You'll need to set up PostgreSQL manually"
        return
    fi

    # Check if Docker is running
    if ! docker info >/dev/null 2>&1; then
        log_warning "Docker is not running, skipping database setup"
        log_info "Please start Docker and run: make setup-dev-db"
        return
    fi

    cd "$PROJECT_ROOT"

    # Start test database
    if [ -f "test/docker/test-env.sh" ]; then
        log_command "./test/docker/test-env.sh up"
        ./test/docker/test-env.sh up

        # Wait for database to be ready
        log_info "Waiting for database to be ready..."
        sleep 5

        # Test database connection
        if ./scripts/check-db.sh -q >/dev/null 2>&1; then
            log_success "Database is ready"
        else
            log_warning "Database setup completed but connection test failed"
            log_info "You may need to wait a few more seconds for the database to fully initialize"
        fi
    else
        log_warning "Database setup script not found, using Docker Compose"
        if [ -f "docker-compose.yml" ]; then
            log_command "docker compose up -d postgres"
            docker compose up -d postgres
            log_success "Database started with Docker Compose"
        else
            log_info "No database setup method found. Please set up PostgreSQL manually."
        fi
    fi
}

# Function to create configuration files
setup_configuration() {
    log_step "Setting up configuration files..."

    cd "$PROJECT_ROOT"

    # Create config.yaml from template if it doesn't exist
    if [ -f "config/config.template.yaml" ] && [ ! -f "config.yaml" ]; then
        log_command "cp config/config.template.yaml config.yaml"
        cp config/config.template.yaml config.yaml
        log_success "Created config.yaml from template"
        log_info "Please review and customize config.yaml for your environment"
    elif [ -f "config.yaml" ]; then
        log_success "config.yaml already exists"
    else
        log_info "No configuration template found"
    fi

    # Create .env.local from example if it doesn't exist
    if [ -f ".env.local.example" ] && [ ! -f ".env.local" ]; then
        log_command "cp .env.local.example .env.local"
        cp .env.local.example .env.local
        log_success "Created .env.local from example"
    elif [ -f ".env.local" ]; then
        log_success ".env.local already exists"
    fi

    # Create .secrets.local from example if it doesn't exist
    if [ -f ".secrets.local.example" ] && [ ! -f ".secrets.local" ]; then
        log_command "cp .secrets.local.example .secrets.local"
        cp .secrets.local.example .secrets.local
        log_success "Created .secrets.local from example"
        log_warning "Please update .secrets.local with real secrets if needed"
    elif [ -f ".secrets.local" ]; then
        log_success ".secrets.local already exists"
    fi
}

# Function to run initial tests
run_initial_tests() {
    log_step "Running initial tests..."

    cd "$PROJECT_ROOT"

    # Run code validation
    log_info "Running code validation..."
    if make validate >/dev/null 2>&1; then
        log_success "Code validation passed"
    else
        log_warning "Code validation failed - you may need to format the code"
        log_info "Run 'make format' to auto-fix formatting issues"
    fi

    # Run unit tests
    log_info "Running unit tests..."
    if make test-unit >/dev/null 2>&1; then
        log_success "Unit tests passed"
    else
        log_warning "Some unit tests failed - this is normal for a development setup"
        log_info "Run 'make test-unit' to see detailed test output"
    fi

    # Build the application
    log_info "Building application..."
    if make build >/dev/null 2>&1; then
        log_success "Application built successfully"
    else
        log_error "Application build failed"
        log_info "Run 'make build' to see detailed build output"
    fi
}

# Function to generate API documentation
setup_documentation() {
    log_step "Setting up documentation..."

    cd "$PROJECT_ROOT"

    # Generate API documentation
    if command_exists swag; then
        log_info "Generating API documentation..."
        if make docs-generate >/dev/null 2>&1; then
            log_success "API documentation generated"
        else
            log_warning "API documentation generation failed"
            log_info "Run 'make docs-generate' to see detailed output"
        fi
    else
        log_info "swag not available, skipping API documentation generation"
    fi

    # Validate documentation if tools are available
    if command_exists npm && [ -f "package.json" ]; then
        log_info "Validating documentation..."
        if make docs-validate >/dev/null 2>&1; then
            log_success "Documentation validation passed"
        else
            log_warning "Documentation validation failed"
            log_info "Run 'make docs-validate' to see detailed output"
        fi
    fi
}

# Function to set up Git hooks
setup_git_hooks() {
    log_step "Setting up Git hooks..."

    cd "$PROJECT_ROOT"

    if [ -f "scripts/setup-hooks.sh" ]; then
        log_command "./scripts/setup-hooks.sh"
        if ./scripts/setup-hooks.sh >/dev/null 2>&1; then
            log_success "Git hooks installed"
        else
            log_warning "Git hooks setup failed"
        fi
    else
        log_info "Git hooks setup script not found"
    fi
}

# Function to display final information
show_final_info() {
    echo ""
    echo "==================================================================================="
    echo -e "${GREEN}🎉 Scanorama Development Environment Setup Complete! 🎉${NC}"
    echo "==================================================================================="
    echo ""
    echo -e "${CYAN}Project Location:${NC} $PROJECT_ROOT"
    echo ""
    echo -e "${CYAN}Quick Start Commands:${NC}"
    echo "  make dev           # Quick development setup and validation"
    echo "  make run           # Build and run the application"
    echo "  make test          # Run all tests"
    echo "  make docs-serve    # Serve API documentation locally"
    echo ""
    echo -e "${CYAN}Development Workflow:${NC}"
    echo "  make quick         # Quick validation + unit tests"
    echo "  make validate      # Code formatting and linting"
    echo "  make security      # Security vulnerability scanning"
    echo "  make coverage      # Generate test coverage report"
    echo ""
    echo -e "${CYAN}Database Management:${NC}"
    if command_exists docker && docker info >/dev/null 2>&1; then
        echo "  ./test/docker/test-env.sh status   # Check database status"
        echo "  ./test/docker/test-env.sh stop     # Stop database"
        echo "  ./test/docker/test-env.sh start    # Start database"
    else
        echo "  (Install and start Docker for database management commands)"
    fi
    echo ""
    echo -e "${CYAN}API Documentation:${NC}"
    echo "  make docs-generate # Generate API docs from code"
    echo "  make docs-serve    # Serve docs at http://localhost:8081"
    echo ""
    echo -e "${CYAN}Configuration Files:${NC}"
    echo "  config.yaml        # Main application configuration"
    echo "  .env.local         # Local environment variables"
    echo "  .secrets.local     # Local secrets (don't commit!)"
    echo ""
    echo -e "${CYAN}Next Steps:${NC}"
    echo "  1. Review and customize config.yaml"
    echo "  2. Run 'make run' to start the application"
    echo "  3. Visit http://localhost:8080/swagger for API documentation"
    echo "  4. Start developing! 🚀"
    echo ""
    if [ -f "CONTRIBUTING.md" ]; then
        echo -e "${CYAN}Contributing:${NC} See CONTRIBUTING.md for development guidelines"
    fi
    echo ""
    echo "==================================================================================="
}

# Main function
main() {
    echo ""
    echo "==================================================================================="
    echo -e "${BLUE}🚀 Scanorama Development Environment Setup${NC}"
    echo "==================================================================================="
    echo ""
    echo "This script will set up a complete development environment for Scanorama."
    echo "It will check prerequisites, install tools, and configure the development environment."
    echo ""

    # Change to project root
    cd "$PROJECT_ROOT"

    # Run setup steps
    check_prerequisites
    setup_go_environment
    setup_node_environment
    setup_configuration
    setup_database
    setup_git_hooks
    run_initial_tests
    setup_documentation

    # Show final information
    show_final_info
}

# Handle script arguments
case "${1:-}" in
    --help|-h)
        echo "Scanorama Development Setup Script"
        echo ""
        echo "Usage: $0 [options]"
        echo ""
        echo "Options:"
        echo "  --help, -h          Show this help message"
        echo "  --check-only        Only check prerequisites"
        echo "  --skip-database     Skip database setup"
        echo "  --skip-tests        Skip initial tests"
        echo ""
        echo "This script sets up a complete development environment including:"
        echo "  - Go dependencies and tools"
        echo "  - Node.js dependencies (for documentation)"
        echo "  - Development database (PostgreSQL)"
        echo "  - Configuration files"
        echo "  - Git hooks"
        echo "  - API documentation"
        echo ""
        exit 0
        ;;
    --check-only)
        check_prerequisites
        exit 0
        ;;
    --skip-database)
        SKIP_DATABASE=true
        ;;
    --skip-tests)
        SKIP_TESTS=true
        ;;
esac

# Run main function
main "$@"
