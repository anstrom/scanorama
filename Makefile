# Build configuration
BINARY_NAME ?= scanorama
BUILD_DIR := build
COVERAGE_FILE := coverage.out
TEST_ENV_SCRIPT := ./test/docker/test-env.sh
DB_DEBUG ?= false
# Use default PostgreSQL port for simplicity
POSTGRES_PORT ?= 5432

# Version information - use git tag if available, otherwise default to v0.1.0-dev
GIT_TAG := $(shell git describe --tags --exact-match 2>/dev/null)
VERSION ?= $(if $(GIT_TAG),$(GIT_TAG),v0.1.0-dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X 'main.version=$(VERSION)' -X 'main.commit=$(COMMIT)' -X 'main.buildTime=$(BUILD_TIME)'

# Go commands
GO := go
GOTEST := $(GO) test
GOBUILD := $(GO) build
GOMOD := $(GO) mod

# Get GOPATH and GOBIN
GOPATH := $(shell $(GO) env GOPATH)
GOBIN := $(GOPATH)/bin
export PATH := $(GOBIN):$(PATH)

# Docker compose commands
DOCKER_COMPOSE := docker compose
COMPOSE_FILE := ./test/docker/docker-compose.yml

.PHONY: help build clean clean-test test test-up test-down test-logs test-debug test-local coverage lint lint-install lint-fix deps install run fmt vet check all version ci-local setup-dev-db db-up db-down db-status

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Quick Start:'
	@echo '  make setup-dev-db # Set up development database'
	@echo '  make ci-local     # Run all CI checks locally before pushing'
	@echo '  make test         # Run tests with database'
	@echo '  make build        # Build binary'
	@echo ''
	@echo 'All Targets:'
	@awk '/^[a-zA-Z_-]+:.*?## / { \
		printf "  \033[36m%-15s\033[0m %s\n", \
		substr($$1, 1, length($$1)-1), \
		substr($$0, index($$0, "##") + 3) \
	}' $(MAKEFILE_LIST)

build: deps ## Build the binary
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/scanorama

version: ## Show version information
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

clean: test-down ## Remove build artifacts and stop test containers
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(COVERAGE_FILE) $(COVERAGE_FILE).html

clean-test: test-down ## Clean up test artifacts
	@echo "Cleaning test artifacts..."
	@rm -f $(COVERAGE_FILE) $(COVERAGE_FILE).html
	@find . -name "*.test" -type f -delete
	@find . -name "test_*.xml" -type f -delete
	@find . -name "*.tmp" -type f -delete

test-up: ## Start test containers
	@echo "Starting test environment..."
	@$(TEST_ENV_SCRIPT) up

test-down: ## Stop test containers
	@echo "Stopping test environment..."
	@$(TEST_ENV_SCRIPT) down

test: test-up ## Run all tests
	@echo "Running tests..."
	@POSTGRES_PORT=$(POSTGRES_PORT) $(GOTEST) -v ./... ; ret=$$? ; \
	make test-down ; \
	exit $$ret

test-debug: ## Run tests with debug output
	@echo "Running tests with debug output..."
	@echo "Starting test environment..."
	@$(TEST_ENV_SCRIPT) up
	@echo "Running tests with DB_DEBUG=true..."
	@POSTGRES_PORT=$(POSTGRES_PORT) DB_DEBUG=true $(GOTEST) -v ./... ; ret=$$? ; \
	$(TEST_ENV_SCRIPT) down ; \
	exit $$ret

test-local: ## Run tests against local PostgreSQL without Docker
	@echo "Running tests directly against local PostgreSQL..."
	@echo "Make sure PostgreSQL is running on port $(POSTGRES_PORT) with:"
	@echo "  - Database: scanorama_test"
	@echo "  - Username: test_user"
	@echo "  - Password: test_password"
	@POSTGRES_PORT=$(POSTGRES_PORT) DB_DEBUG=true $(GOTEST) -v ./...

test-integration: ## Run integration tests with database
	@echo "Running integration tests..."
	@echo "Make sure PostgreSQL is running on port 5432 with development database"
	@$(GOTEST) -v ./test -run TestIntegration -timeout 30m

test-benchmark: ## Run benchmark tests
	@echo "Running benchmark tests..."
	@echo "Make sure PostgreSQL is running on port 5432 with development database"
	@$(GOTEST) -v ./test -bench=. -benchmem -timeout 30m

test-db: ## Run database-specific integration tests
	@echo "Running database integration tests..."
	@echo "Make sure PostgreSQL is running on port 5432 with development database"
	@$(GOTEST) -v ./test -run "TestScanWithDatabaseStorage|TestDiscoveryWithDatabaseStorage|TestQueryScanResults" -timeout 15m

test-all: db-up ## Run all tests including integration and benchmarks
	@echo "Running all tests..."
	@sleep 3
	@$(GOTEST) -v ./... ; ret1=$$? ; \
	$(GOTEST) -v ./test -timeout 30m ; ret2=$$? ; \
	if [ $$ret1 -ne 0 ] || [ $$ret2 -ne 0 ]; then \
		echo "Some tests failed" ; \
		exit 1 ; \
	fi
	@echo "All tests passed!"

setup-dev-db: ## Set up development PostgreSQL database
	@echo "Setting up development database..."
	@./scripts/setup-dev-db.sh

db-up: ## Start PostgreSQL container for development
	@echo "Starting PostgreSQL container for development..."
	@docker run --name scanorama-dev-postgres \
		-e POSTGRES_DB=scanorama_dev \
		-e POSTGRES_USER=scanorama_dev \
		-e POSTGRES_PASSWORD=dev_password \
		-p 5432:5432 \
		-d postgres:16-alpine || echo "Container may already exist"
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 5
	@echo "PostgreSQL is ready on localhost:5432"
	@echo "Database: scanorama_dev, User: scanorama_dev, Password: dev_password"

db-down: ## Stop and remove PostgreSQL development container
	@echo "Stopping PostgreSQL development container..."
	@docker stop scanorama-dev-postgres || true
	@docker rm scanorama-dev-postgres || true

db-status: ## Check PostgreSQL development container status
	@echo "PostgreSQL development container status:"
	@docker ps -a --filter name=scanorama-dev-postgres --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" || echo "Container not found"

coverage: test-up ## Generate test coverage report
	@echo "Generating coverage report..."
	@POSTGRES_PORT=$(POSTGRES_PORT) $(GOTEST) -cover ./... -coverprofile=$(COVERAGE_FILE) ; ret=$$? ; \
	if [ $$ret -eq 0 ]; then \
		$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_FILE).html && \
		echo "Coverage report: $(COVERAGE_FILE).html" ; \
	fi ; \
	make test-down ; \
	exit $$ret

lint-install: ## Install golangci-lint
	@echo "Installing latest golangci-lint..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) latest

lint: lint-install ## Run golangci-lint
	@echo "Running golangci-lint..."
	@$(GOBIN)/golangci-lint run --config .golangci.yml

lint-verbose: lint-install ## Run golangci-lint with verbose output
	@echo "Running golangci-lint with verbose output..."
	@$(GOBIN)/golangci-lint run --config .golangci.yml --verbose

lint-fix: lint-install ## Fix formatting and linting issues automatically
	@echo "Running golangci-lint with auto-fix..."
	@$(GOBIN)/golangci-lint run --config .golangci.yml --fix

fmt: ## Format Go files (alias for lint-fix)
	@echo "Formatting Go files..."
	@$(MAKE) lint-fix

vet: ## Run go vet (included in golangci-lint)
	@echo "Running go vet via golangci-lint..."
	@$(MAKE) lint

deps: ## Download and tidy dependencies
	@echo "Installing dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy

install: build ## Install binary to GOPATH
	@echo "Installing $(BINARY_NAME)..."
	@$(GO) install -ldflags "$(LDFLAGS)" ./cmd/scanorama

run: build ## Build and run the application
	@echo "Running $(BINARY_NAME)..."
	@./$(BUILD_DIR)/$(BINARY_NAME)

check: lint test ## Run lint and tests
	@echo "All checks passed!"

test-logs: ## View logs from test containers
	@echo "Viewing test container logs..."
	@$(TEST_ENV_SCRIPT) logs

ci-local: ## Run full CI checks locally (lint, test, build, security)
	@echo "Running local CI checks..."
	@echo "=== Running linters ==="
	@$(MAKE) lint
	@echo "=== Running tests ==="
	@$(MAKE) test
	@echo "=== Running security checks ==="
	@$(MAKE) security-local
	@echo "=== Building ==="
	@$(MAKE) build
	@echo "=== Testing binary ==="
	@./$(BUILD_DIR)/$(BINARY_NAME) version
	@echo "✅ All local CI checks passed!"

security-local: ## Run security checks locally
	@echo "Running security scans..."
	@echo "Installing security tools..."
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "Running security linters via golangci-lint (includes gosec)..."
	@$(MAKE) lint
	@echo "Running govulncheck..."
	@$(GOBIN)/govulncheck ./... || echo "⚠️ Vulnerabilities found (informational only)"

all: clean deps check build ## Clean, install dependencies, run checks and build

.DEFAULT_GOAL := help
