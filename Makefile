# Scanorama Makefile - Simplified and Fixed

# Build configuration
BINARY_NAME ?= scanorama
BUILD_DIR := build
COVERAGE_FILE := coverage.out

# Version information
GIT_VERSION := $(shell git describe --tags --always 2>/dev/null)
VERSION ?= $(if $(GIT_VERSION),$(GIT_VERSION),dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X 'main.version=$(VERSION)' -X 'main.commit=$(COMMIT)' -X 'main.buildTime=$(BUILD_TIME)'

# Go commands
GO := go
GOTEST := $(GO) test
GOBUILD := $(GO) build

# Docker compose
DOCKER_COMPOSE := docker compose
TEST_COMPOSE_FILE := docker/docker-compose.test.yml

# Test database environment variables
export TEST_DB_HOST := localhost
export TEST_DB_PORT := 5432
export TEST_DB_NAME := scanorama_test
export TEST_DB_USER := test_user
export TEST_DB_PASSWORD := test_password

.PHONY: help
help: ## Show this help message
	@echo 'Scanorama - Quick Reference'
	@echo ''
	@echo 'Common Tasks:'
	@echo '  make test          - Run all tests (starts DB automatically)'
	@echo '  make test-unit     - Run only unit tests (no DB needed)'
	@echo '  make coverage      - Generate coverage report'
	@echo '  make build         - Build the binary'
	@echo '  make clean         - Clean build artifacts'
	@echo ''
	@echo 'Database:'
	@echo '  make db-up         - Start test database'
	@echo '  make db-down       - Stop test database'
	@echo '  make db-reset      - Reset test database (down + up)'
	@echo ''
	@echo 'Development:'
	@echo '  make lint          - Run linter'
	@echo '  make fmt           - Format code'
	@echo ''
	@echo 'All targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build the scanorama binary
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/scanorama
	@echo "✓ Binary built: $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: clean
clean: ## Remove build artifacts and test files
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(COVERAGE_FILE) $(COVERAGE_FILE).html
	@find . -name "*.test" -type f -delete
	@find . -name "coverage.txt" -type f -delete
	@echo "✓ Clean complete"

.PHONY: version
version: ## Show version information
	@echo "Version:    $(VERSION)"
	@echo "Commit:     $(COMMIT)"
	@echo "Build Time: $(BUILD_TIME)"

# =============================================================================
# Database Management
# =============================================================================

.PHONY: db-up
db-up: ## Start PostgreSQL test database
	@echo "Starting test database..."
	@$(DOCKER_COMPOSE) -f $(TEST_COMPOSE_FILE) up -d --wait
	@echo "✓ Database started"

.PHONY: db-down
db-down: ## Stop PostgreSQL test database
	@echo "Stopping test database..."
	@$(DOCKER_COMPOSE) -f $(TEST_COMPOSE_FILE) down -v
	@echo "✓ Database stopped"

.PHONY: db-reset
db-reset: db-down db-up ## Reset test database (stop and start fresh)

.PHONY: db-logs
db-logs: ## Show database logs
	@$(DOCKER_COMPOSE) -f $(TEST_COMPOSE_FILE) logs -f postgres

.PHONY: db-shell
db-shell: ## Connect to database with psql
	@$(DOCKER_COMPOSE) -f $(TEST_COMPOSE_FILE) exec postgres psql -U $(TEST_DB_USER) -d $(TEST_DB_NAME)

# =============================================================================
# Testing
# =============================================================================

.PHONY: test-unit
test-unit: ## Run unit tests only (no database required)
	@echo "Running unit tests..."
	@$(GOTEST) -short -v ./...
	@echo "✓ Unit tests complete"

.PHONY: test
test: ## Run all tests (starts database automatically)
	@echo "Starting test database..."
	@$(MAKE) db-up
	@echo "Running all tests..."
	@$(GOTEST) -v ./... || (echo "✗ Tests failed"; $(MAKE) db-down; exit 1)
	@echo "✓ Tests passed"
	@$(MAKE) db-down

.PHONY: test-keep-db
test-keep-db: db-up ## Run all tests and keep database running
	@echo "Running all tests (database will stay running)..."
	@$(GOTEST) -v ./...

.PHONY: coverage
coverage: ## Generate test coverage report
	@echo "Starting test database..."
	@$(MAKE) db-up
	@echo "Generating coverage report..."
	@$(GOTEST) -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./... || (echo "✗ Coverage generation failed"; $(MAKE) db-down; exit 1)
	@echo "✓ Tests passed"
	@$(MAKE) db-down
	@if [ -f $(COVERAGE_FILE) ]; then \
		echo "Generating HTML report..."; \
		$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_FILE).html; \
		echo ""; \
		echo "Coverage Summary:"; \
		$(GO) tool cover -func=$(COVERAGE_FILE) | tail -1; \
		echo ""; \
		echo "✓ Coverage report: $(COVERAGE_FILE).html"; \
	fi

.PHONY: coverage-keep-db
coverage-keep-db: db-up ## Generate coverage and keep database running
	@echo "Generating coverage report (database will stay running)..."
	@$(GOTEST) -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@if [ -f $(COVERAGE_FILE) ]; then \
		$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_FILE).html; \
		echo "Coverage report: $(COVERAGE_FILE).html"; \
		$(GO) tool cover -func=$(COVERAGE_FILE) | tail -1; \
	fi

.PHONY: coverage-show
coverage-show: ## Show coverage in browser (requires existing coverage.out)
	@if [ ! -f $(COVERAGE_FILE) ]; then \
		echo "✗ No coverage file found. Run 'make coverage' first."; \
		exit 1; \
	fi
	@$(GO) tool cover -html=$(COVERAGE_FILE)

# =============================================================================
# Code Quality
# =============================================================================

.PHONY: fmt
fmt: ## Format code with gofmt
	@echo "Formatting code..."
	@gofmt -s -w .
	@echo "✓ Code formatted"

.PHONY: lint
lint: ## Run golangci-lint
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "✗ golangci-lint not installed"; \
		echo "Install with: curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b \$$(go env GOPATH)/bin"; \
		exit 1; \
	fi
	@echo "✓ Linting complete"

.PHONY: vet
vet: ## Run go vet
	@echo "Running go vet..."
	@$(GO) vet ./...
	@echo "✓ Vet complete"

.PHONY: check
check: fmt vet test-unit ## Run quick checks (format, vet, unit tests)
	@echo "✓ All checks passed"

# =============================================================================
# Dependencies
# =============================================================================

.PHONY: deps
deps: ## Download dependencies
	@echo "Downloading dependencies..."
	@$(GO) mod download
	@$(GO) mod tidy
	@echo "✓ Dependencies updated"

.PHONY: deps-upgrade
deps-upgrade: ## Upgrade dependencies
	@echo "Upgrading dependencies..."
	@$(GO) get -u ./...
	@$(GO) mod tidy
	@echo "✓ Dependencies upgraded"

# =============================================================================
# CI/Development Workflow
# =============================================================================

.PHONY: ci
ci: deps check test ## Run full CI pipeline locally
	@echo "✓ CI pipeline complete"

.PHONY: dev-setup
dev-setup: deps ## Setup development environment
	@echo "Setting up development environment..."
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "Installing golangci-lint..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin; \
	fi
	@echo "✓ Development environment ready"
	@echo ""
	@echo "Quick start:"
	@echo "  make db-up        # Start test database"
	@echo "  make test         # Run tests"
	@echo "  make db-down      # Stop database when done"

.PHONY: all
all: clean deps build test ## Build everything from scratch
	@echo "✓ Build complete"

# =============================================================================
# Documentation (for CI compatibility)
# =============================================================================

.PHONY: docs-generate
docs-generate: ## Generate API documentation (placeholder for CI)
	@echo "Generating API documentation..."
	@if command -v swag >/dev/null 2>&1; then \
		cd docs && swag init -g swagger_docs.go -o ./swagger --parseDependency --parseInternal; \
		echo "✓ API documentation generated"; \
	else \
		echo "⚠ swag not installed, skipping documentation generation"; \
		echo "Install with: go install github.com/swaggo/swag/cmd/swag@latest"; \
	fi
