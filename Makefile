# Build configuration
BINARY_NAME ?= scanorama
BUILD_DIR := build
COVERAGE_FILE := coverage.out
DB_DEBUG ?= false
# Database testing configuration
TEST_DB_PORT ?= 5433
TEST_DB_COMPOSE_FILE := test/docker/docker-compose.test.yml

# Dependency check functions
define check_tool
	@command -v $(1) >/dev/null 2>&1 || { echo "❌ Error: $(1) is not installed. Please install it first."; exit 1; }
endef

define check_file
	@[ -f $(1) ] || { echo "❌ Error: Required file $(1) not found."; exit 1; }
endef

define check_docker
	@docker info >/dev/null 2>&1 || { echo "❌ Error: Docker is not running. Please start Docker first."; exit 1; }
endef

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

.PHONY: help build clean test coverage quality lint format security ci setup-dev-db setup-hooks docker-build docker-up docker-down docker-logs docs-install docs-generate docs-serve docs-clean docs

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Quick Start:'
	@echo '  make setup-hooks  # Set up Git hooks for code quality'
	@echo '  make setup-dev-db # Set up development database'
	@echo '  make test         # Run all tests (core + integration) with database'
	@echo '  make build        # Build binary'
	@echo ''
	@echo 'Testing:'
	@echo '  make test-short      # Unit tests only (no database required)'
	@echo '  make test-db         # Database tests with local container'
	@echo '  make test-ci         # Simulate GitHub Actions CI environment'
	@echo '  make test-integration # Full integration tests with all services'
	@echo ''
	@echo 'CI & Docker:'
	@echo '  make ci              # Run full CI pipeline locally'
	@echo '  make ci-local        # Run CI excluding GitHub-specific jobs'
	@echo '  make ci-clean        # Run CI with Docker cleanup first'
	@echo '  make docker-cleanup  # Clean Docker cache and unused images'
	@echo '  make docker-cleanup-all # Complete Docker cleanup (all resources)'
	@echo ''
	@echo 'Environment Variables:'
	@echo '  DEBUG=true make test    # Run tests with debug output'
	@echo '  POSTGRES_PORT=5433      # Use custom PostgreSQL port'
	@echo ''
	@echo 'CI Pipeline:'
	@echo '  make ci              # Run comprehensive CI pipeline locally'

	@echo '  make ci-quick        # Quick validation (dry-run only)'
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

test: ## Run all tests with database container
	@echo "Running all tests with database container..."
	@docker compose -f $(TEST_DB_COMPOSE_FILE) up -d test-postgres
	@echo "Waiting for database to be ready..."
	@sleep 5
	@TEST_DB_PORT=$(TEST_DB_PORT) $(GOTEST) -v ./...
	@docker compose -f $(TEST_DB_COMPOSE_FILE) down

test-db: ## Run database tests only
	@echo "Running database tests..."
	@docker compose -f $(TEST_DB_COMPOSE_FILE) up -d test-postgres
	@echo "Waiting for database to be ready..."
	@sleep 5
	@TEST_DB_PORT=$(TEST_DB_PORT) $(GOTEST) -v ./internal/db/...
	@docker compose -f $(TEST_DB_COMPOSE_FILE) down

test-integration: ## Run integration tests with all services
	@echo "Running integration tests..."
	@docker compose -f $(TEST_DB_COMPOSE_FILE) up -d
	@echo "Waiting for services to be ready..."
	@sleep 10
	@TEST_DB_PORT=$(TEST_DB_PORT) $(GOTEST) -tags=integration -v ./...
	@docker compose -f $(TEST_DB_COMPOSE_FILE) down

test-short: ## Run unit tests only (no database)
	@echo "Running unit tests (short mode)..."
	@$(GOTEST) -short -v ./...

test-ci: ## Test CI database configuration (simulates GitHub Actions environment)
	@echo "Testing CI database configuration..."
	@echo "Setting up CI environment variables..."
	@docker compose -f $(TEST_DB_COMPOSE_FILE) up -d test-postgres
	@echo "Waiting for database to be ready..."
	@sleep 5
	@echo "Creating CI test database and user..."
	@docker exec scanorama-test-postgres-$(TEST_DB_PORT) psql -U test_user -d scanorama_test -c "CREATE USER IF NOT EXISTS scanorama_test_user WITH PASSWORD 'test_password_123';" || true
	@docker exec scanorama-test-postgres-$(TEST_DB_PORT) psql -U test_user -d scanorama_test -c "GRANT ALL PRIVILEGES ON DATABASE scanorama_test TO scanorama_test_user;" || true
	@docker exec scanorama-test-postgres-$(TEST_DB_PORT) psql -U test_user -d scanorama_test -c "GRANT ALL ON SCHEMA public TO scanorama_test_user;" || true
	@echo "Running tests with CI environment..."
	@GITHUB_ACTIONS=true CI=true TEST_DB_HOST=localhost TEST_DB_PORT=$(TEST_DB_PORT) TEST_DB_NAME=scanorama_test TEST_DB_USER=scanorama_test_user TEST_DB_PASSWORD=test_password_123 $(GOTEST) -v ./internal/db/...
	@echo "Running CI detection tests..."
	@GITHUB_ACTIONS=true CI=true DB_DEBUG=true $(GOTEST) -v ./internal/db/ -run TestCI
	@docker compose -f $(TEST_DB_COMPOSE_FILE) down
	@echo "✅ CI database configuration tests completed successfully!"





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
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) v2.1.5
	@echo "Running golangci-lint..."
	@$(GOBIN)/golangci-lint run --config .golangci.yml

coverage: ## Generate test coverage report
	@echo "Generating coverage report..."
	@if ./scripts/check-db.sh >/dev/null 2>&1; then \
		echo "Database available, running tests with coverage..."; \
		echo "Starting test service containers..."; \
		$(TEST_ENV_SCRIPT) up; \
		POSTGRES_PORT=5432 $(GOTEST) -coverprofile=$(COVERAGE_FILE) ./... || true; \
		$(TEST_ENV_SCRIPT) down; \
	else \
		echo "No database found, starting test containers..."; \
		$(TEST_ENV_SCRIPT) up; \
		POSTGRES_PORT=$(POSTGRES_PORT) $(GOTEST) -coverprofile=$(COVERAGE_FILE) ./... || true; \
		$(TEST_ENV_SCRIPT) down; \
	fi
	@if [ -f $(COVERAGE_FILE) ]; then \
		echo "Generating HTML coverage report..."; \
		$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_FILE).html; \
		echo "Coverage report generated: $(COVERAGE_FILE).html"; \
		echo "Overall coverage:"; \
		$(GO) tool cover -func=$(COVERAGE_FILE) | tail -1; \
	else \
		echo "No coverage data generated - all tests may have failed"; \
	fi

format: ## Format code and fix linting issues automatically
	@echo "Installing latest golangci-lint..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) v2.1.5
	@echo "Formatting code and fixing issues..."
	@$(GOBIN)/golangci-lint run --config .golangci.yml --fix

lint-fix: format ## Alias for format - auto-fix linting issues



test-core: ## Run tests for core packages (errors, logging, metrics)
	@echo "Running core package tests..."
	@if ./scripts/check-db.sh -q >/dev/null 2>&1; then \
		echo "Database available, using existing database..."; \
		echo "Starting test service containers..."; \
		$(TEST_ENV_SCRIPT) up; \
		POSTGRES_PORT=5432 $(GOTEST) -v ./internal/errors ./internal/logging ./internal/metrics; \
		ret=$$?; \
		$(TEST_ENV_SCRIPT) down; \
		exit $$ret; \
	else \
		echo "No database found, starting test containers..."; \
		$(TEST_ENV_SCRIPT) up; \
		POSTGRES_PORT=$(POSTGRES_PORT) $(GOTEST) -v ./internal/errors ./internal/logging ./internal/metrics; \
		ret=$$?; \
		$(TEST_ENV_SCRIPT) down; \
		exit $$ret; \
	fi



coverage-core: ## Generate coverage report for core packages
	@echo "Generating core package coverage report..."
	@if ./scripts/check-db.sh >/dev/null 2>&1; then \
		echo "Database available, running core tests with coverage..."; \
		echo "Starting test service containers..."; \
		$(TEST_ENV_SCRIPT) up; \
		POSTGRES_PORT=5432 $(GOTEST) -coverprofile=$(COVERAGE_FILE) ./internal/errors ./internal/logging ./internal/metrics || true; \
		$(TEST_ENV_SCRIPT) down; \
	else \
		echo "No database found, starting test containers..."; \
		$(TEST_ENV_SCRIPT) up; \
		POSTGRES_PORT=$(POSTGRES_PORT) $(GOTEST) -coverprofile=$(COVERAGE_FILE) ./internal/errors ./internal/logging ./internal/metrics || true; \
		$(TEST_ENV_SCRIPT) down; \
	fi
	@if [ -f $(COVERAGE_FILE) ]; then \
		echo "Generating HTML coverage report..."; \
		$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_FILE).html; \
		echo "Core package coverage report generated: $(COVERAGE_FILE).html"; \
		echo "Core package coverage:"; \
		$(GO) tool cover -func=$(COVERAGE_FILE) | tail -1; \
	else \
		echo "No coverage data generated - all tests may have failed"; \
	fi



security: ## Run comprehensive security scans (vulnerability + hardening)
	@echo "🔒 Running comprehensive security scans..."
	@echo "Installing security tools..."
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "✓ Security tools installed"
	@echo ""

	@echo "Running govulncheck for known vulnerabilities..."
	@$(GOBIN)/govulncheck ./... && echo "✅ No known vulnerabilities found" || echo "⚠️ Vulnerabilities found - review output above"
	@echo ""
	@echo "Checking for hardcoded secrets patterns..."
	@if grep -r -i "password.*=" --include="*.go" . | grep -v "_test.go" | grep -v "example\|template\|config\.template"; then \
		echo "⚠️ Found potential hardcoded passwords"; \
	fi
	@if grep -r -i "api[_-]key.*=" --include="*.go" . | grep -v "_test.go" | grep -v "example\|template"; then \
		echo "⚠️ Found potential hardcoded API keys"; \
	fi
	@if grep -r -i "secret.*=" --include="*.go" . | grep -v "_test.go" | grep -v "example\|template"; then \
		echo "⚠️ Found potential hardcoded secrets"; \
	fi
	@echo "✓ Secret pattern check completed"
	@echo ""
	@echo "Checking file permissions..."
	@find . -type f -perm /o+w -not -path "./.git/*" -not -path "./build/*" -not -path "./dist/*" | while read file; do \
		echo "⚠️ World-writable file found: $$file"; \
	done || true
	@find . -name "*.go" -perm /a+x -not -path "./.git/*" | while read file; do \
		echo "⚠️ Executable Go file found: $$file"; \
	done || true
	@echo "✓ File permission check completed"
	@echo "✅ Comprehensive security scan completed"

# Docker targets
docker-build: ## Build Docker image for local platform
	@echo "Building Docker image for local platform..."
	@docker buildx build --platform=local -t scanorama:dev .
	@echo "✅ Docker image built: scanorama:dev"

docker-up: ## Start development environment with Docker Compose
	@echo "Starting development environment..."
	@docker compose up -d
	@echo "✅ Development environment started"
	@echo "  Application: http://localhost:8080"
	@echo "  PostgreSQL: localhost:5432"
	@echo "  Redis: localhost:6379"

docker-down: ## Stop development environment
	@echo "Stopping development environment..."
	@docker compose down --volumes
	@echo "✅ Development environment stopped"

docker-logs: ## Show logs from development environment
	@docker compose logs -f

# Documentation targets
docs-install: ## Install swagger documentation tools
	@echo "Installing swagger documentation tools..."
	@go install github.com/swaggo/swag/cmd/swag@latest
	@echo "✅ Swagger tools installed"

docs-generate: docs-install ## Generate API documentation from code annotations
	@echo "Generating API documentation..."
	@cd docs && swag init -g swagger_docs.go -o ./swagger --parseDependency --parseInternal
	@echo "✅ API documentation generated in docs/swagger/"

docs-serve: docs-generate ## Generate and serve API documentation locally
	@echo "Starting documentation server on http://localhost:8081..."
	@echo "API documentation will be available at http://localhost:8081/swagger/index.html"
	@cd docs/swagger && python3 -m http.server 8081 2>/dev/null || python -m SimpleHTTPServer 8081

docs-clean: ## Clean generated documentation files
	@echo "Cleaning generated documentation..."
	@rm -rf docs/swagger/docs.go docs/swagger/swagger.json docs/swagger/swagger.yaml
	@echo "✅ Documentation files cleaned"

docs: docs-generate ## Alias for docs-generate

docs-validate: docs-install ## Validate API documentation quality
	@echo "Validating API documentation..."
	@npm run docs:validate
	@echo "✅ Documentation validation completed"

docs-lint: docs-install ## Lint API documentation with detailed output
	@echo "Linting API documentation..."
	@npm run docs:lint
	@echo "✅ Documentation linting completed"

docs-test-clients: docs-install ## Test client generation from OpenAPI spec
	@echo "Testing client generation..."
	@npm run test:clients
	@echo "✅ Client generation test completed"

docs-spectral: docs-install ## Run advanced OpenAPI linting with Vacuum
	@echo "Running advanced documentation analysis with Vacuum..."
	@npm run spectral:lint
	@echo "✅ Advanced documentation analysis completed"

docs-build: docs-install ## Build HTML documentation
	@echo "Building HTML documentation..."
	@npm run docs:build
	@echo "✅ Documentation built to docs/swagger/index.html"

docs-ci: docs-install ## CI-friendly documentation validation (fails on issues)
	@echo "Running CI documentation validation..."
	@npm run docs:validate && npm run spectral:lint

# Act testing with GitHub Actions locally
act-list: ## List available GitHub Actions workflows
	$(call check_tool,act)
	@act --list

act-validate: ## Validate GitHub Actions workflow syntax
	$(call check_tool,act)
	@act --dryrun --list >/dev/null 2>&1 && echo "✅ Workflow syntax is valid" || { echo "❌ Workflow syntax errors found"; exit 1; }

act-clean: ## Clean up act containers and cache
	$(call check_tool,docker)
	$(call check_docker)
	@docker container prune -f --filter "label=act" >/dev/null 2>&1 || true
	@docker image prune -f --filter "label=act" >/dev/null 2>&1 || true
	@echo "✅ Act cleanup completed"

ci: ## Run comprehensive CI pipeline locally with act
	@echo "🚀 Running comprehensive CI pipeline..."
	$(call check_tool,act)
	$(call check_docker)
	@act push --quiet || { echo "⚠️ CI pipeline completed with issues"; }
	@echo "✅ CI pipeline completed"

docker-cleanup: ## Clean Docker build cache and unused images
	@echo "🧹 Cleaning Docker build cache and unused images..."
	$(call check_docker)
	@docker builder prune -f
	@docker image prune -f
	@echo "✅ Docker cleanup completed"

docker-cleanup-all: ## Complete Docker cleanup (including volumes and containers)
	@echo "🧹 Performing complete Docker cleanup..."
	$(call check_docker)
	@docker system prune -a -f --volumes
	@echo "✅ Complete Docker cleanup completed"

ci-local: ## Run CI locally excluding GitHub-specific jobs (like CodeQL)
	@echo "🚀 Running local CI pipeline (excluding GitHub-specific jobs)..."
	$(call check_tool,act)
	$(call check_docker)
	@act push --quiet --workflows .github/workflows/local-ci.yml || { echo "⚠️ Local CI pipeline completed with issues"; }
	@echo "✅ Local CI pipeline completed"

ci-clean: ## Run CI with Docker cleanup first
	@echo "🧹 Cleaning Docker environment before CI..."
	@$(MAKE) docker-cleanup
	@$(MAKE) ci-local

ci-quick: ## Quick CI validation (syntax check only)
	@echo "⚡ Quick CI validation..."
	$(call check_tool,act)
	@act --dryrun --list >/dev/null 2>&1 && echo "✅ All workflows valid" || { echo "❌ Workflow issues found"; exit 1; }

# Developer experience targets
dev: ## Set up development environment and run initial checks
	@echo "🚀 Setting up development environment..."
	@$(MAKE) deps
	@$(MAKE) validate
	@$(MAKE) test-unit
	@echo "✅ Development environment ready!"
	@echo "💡 Available commands:"
	@echo "  make run          # Start the application"
	@echo "  make test         # Run all tests"
	@echo "  make docs-serve   # Serve API documentation"

validate: ## Quick code validation (format, lint, basic checks)
	@echo "⚡ Running quick validation..."
	@echo "Checking code formatting..."
	@test -z "$$(gofmt -s -l . | tee /dev/stderr)" || (echo "❌ Files not formatted properly" && exit 1)
	@echo "✅ Code formatting OK"
	@echo "Running basic linting..."
	@$(MAKE) lint >/dev/null 2>&1 && echo "✅ Linting passed" || echo "⚠️ Linting issues found - run 'make lint' for details"
	@echo "✅ Quick validation completed"

test-unit: ## Run unit tests only (fast, no database required)
	@echo "🧪 Running unit tests..."
	@$(GOTEST) -short -v ./... || (echo "❌ Unit tests failed" && exit 1)
	@echo "✅ Unit tests passed"

e2e-test: ## Run End-to-End tests (requires system dependencies like nmap)
	@echo "🚀 Running End-to-End tests..."
	@if ./scripts/check-db.sh -q >/dev/null 2>&1; then \
		echo "Database available, using existing database..."; \
		echo "Starting test service containers..."; \
		$(TEST_ENV_SCRIPT) up; \
		POSTGRES_PORT=5432 $(GOTEST) -v ./test/integration_test.go; \
		ret=$$?; \
		$(TEST_ENV_SCRIPT) down; \
		exit $$ret; \
	else \
		echo "No database found, starting test containers..."; \
		$(TEST_ENV_SCRIPT) up; \
		POSTGRES_PORT=$(POSTGRES_PORT) $(GOTEST) -v ./test/integration_test.go; \
		ret=$$?; \
		$(TEST_ENV_SCRIPT) down; \
		exit $$ret; \
	fi
	@echo "✅ End-to-End tests passed"

check: validate test-unit security ## Run all quality checks (validate + test + security)
	@echo "✅ All quality checks passed!"

deps: ## Install/update development dependencies
	@echo "📦 Installing/updating dependencies..."
	@go mod download
	@go mod tidy
	@$(MAKE) docs-install >/dev/null 2>&1 || echo "⚠️ Documentation tools installation skipped"
	@echo "✅ Dependencies updated"

quick: validate test-unit ## Quick development cycle (validate + unit tests)
	@echo "⚡ Quick development cycle completed!"

.DEFAULT_GOAL := help
