# Build configuration
BINARY_NAME ?= scanorama
BUILD_DIR := build
COVERAGE_FILE := coverage.out
TEST_ENV_SCRIPT := ./test/docker/test-env.sh
DB_DEBUG ?= false
# Use default PostgreSQL port for simplicity
POSTGRES_PORT ?= 5432

# Version information - use git describe for accurate version string
GIT_VERSION := $(shell git describe --tags --always 2>/dev/null)
VERSION ?= $(if $(GIT_VERSION),$(GIT_VERSION),dev)
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

.PHONY: help build clean test quality lint format security ci setup-dev-db setup-hooks

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Quick Start:'
	@echo '  make setup-hooks  # Set up Git hooks for code quality'
	@echo '  make setup-dev-db # Set up development database'
	@echo '  make ci           # Run full CI pipeline locally before pushing'
	@echo '  make test         # Run tests with database'
	@echo '  make build        # Build binary'
	@echo ''
	@echo 'Environment Variables:'
	@echo '  DEBUG=true make test    # Run tests with debug output'
	@echo '  POSTGRES_PORT=5433      # Use custom PostgreSQL port'
	@echo ''
	@echo 'All Targets:'
	@awk '/^[a-zA-Z_-]+:.*?## / { \
		printf "  \033[36m%-15s\033[0m %s\n", \
		substr($$1, 1, length($$1)-1), \
		substr($$0, index($$0, "##") + 3) \
	}' $(MAKEFILE_LIST)

build: ## Build the binary
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/scanorama

version: ## Show version information
	@echo "Version: $(VERSION)"
	@echo "Commit: $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

clean: ## Remove build artifacts and clean up test files
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(COVERAGE_FILE) $(COVERAGE_FILE).html
	@find . -name "*.test" -type f -delete
	@find . -name "test_*.xml" -type f -delete
	@find . -name "*.tmp" -type f -delete

test: ## Run all tests (checks for existing DB first)
	@echo "Running tests..."
	@if ./scripts/check-db.sh -q >/dev/null 2>&1; then \
		echo "Database available, using existing database..."; \
		echo "Using database on localhost:5432"; \
		if [ "$(DEBUG)" = "true" ]; then \
			echo "Running with debug output..."; \
			POSTGRES_PORT=5432 DB_DEBUG=true $(GOTEST) -v -p 1 ./...; \
		else \
			POSTGRES_PORT=5432 $(GOTEST) -v -p 1 ./...; \
		fi; \
	else \
		echo "No database found, starting test containers..."; \
		$(TEST_ENV_SCRIPT) up; \
		if [ "$(DEBUG)" = "true" ]; then \
			echo "Running with debug output..."; \
			POSTGRES_PORT=$(POSTGRES_PORT) DB_DEBUG=true $(GOTEST) -v -p 1 ./...; \
		else \
			POSTGRES_PORT=$(POSTGRES_PORT) $(GOTEST) -v -p 1 ./...; \
		fi; \
		ret=$$?; \
		$(TEST_ENV_SCRIPT) down; \
		exit $$ret; \
	fi




setup-dev-db: ## Set up development PostgreSQL database
	@echo "Setting up development database..."
	@./scripts/setup-dev-db.sh

setup-hooks: ## Set up Git hooks for code quality checks
	@echo "Setting up Git hooks..."
	@./scripts/setup-hooks.sh

quality: ## Run comprehensive code quality checks (lint + format + security)
	@echo "Running comprehensive code quality checks..."
	@$(MAKE) lint
	@$(MAKE) format
	@$(MAKE) security
	@echo "✅ All quality checks passed!"

lint: ## Run golangci-lint to check code quality
	@echo "Installing latest golangci-lint..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) latest
	@echo "Running golangci-lint..."
	@$(GOBIN)/golangci-lint run --config .golangci.yml

format: ## Format code and fix linting issues automatically
	@echo "Installing latest golangci-lint..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) latest
	@echo "Formatting code and fixing issues..."
	@$(GOBIN)/golangci-lint run --config .golangci.yml --fix

lint-fix: format ## Alias for format - auto-fix linting issues



ci: ## Run full CI pipeline locally (quality + test + build)
	@echo "Running local CI pipeline..."
	@echo "=== Checking database status ==="
	@./scripts/check-db.sh || echo "Note: Some tests may require database"
	@echo "=== Running quality checks ==="
	@$(MAKE) quality
	@echo "=== Running tests ==="
	@$(MAKE) test
	@echo "=== Building ==="
	@$(MAKE) build
	@echo "=== Testing binary ==="
	@./$(BUILD_DIR)/$(BINARY_NAME) --version
	@echo "✅ All CI checks passed!"

security: ## Run security vulnerability scans
	@echo "Running security scans..."
	@echo "Installing security tools..."
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "Running security linters via golangci-lint (includes gosec)..."
	@$(MAKE) lint
	@echo "Running govulncheck..."
	@$(GOBIN)/govulncheck ./... || echo "⚠️ Vulnerabilities found (informational only)"

.DEFAULT_GOAL := help
