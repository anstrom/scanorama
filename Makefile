# Build configuration
BINARY_NAME := scanorama
BUILD_DIR := build
COVERAGE_FILE := coverage.out
TEST_ENV_SCRIPT := ./test/docker/test-env.sh

# Go commands
GO := go
GOTEST := $(GO) test
GOBUILD := $(GO) build
GOMOD := $(GO) mod

# Get GOPATH and GOBIN
GOPATH := $(shell $(GO) env GOPATH)
GOBIN := $(GOPATH)/bin
export PATH := $(GOBIN):$(PATH)

.PHONY: help build clean test coverage lint lint-install lint-fix deps install run

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z_-]+:.*?## / { \
		printf "  \033[36m%-15s\033[0m %s\n", \
		substr($$1, 1, length($$1)-1), \
		substr($$0, index($$0, "##") + 3) \
	}' $(MAKEFILE_LIST)

build: deps ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/scanorama

clean: ## Remove build artifacts
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(COVERAGE_FILE) $(COVERAGE_FILE).html

test: ## Run all tests
	@echo "Running tests..."
	@if [ -x "$(TEST_ENV_SCRIPT)" ]; then \
		$(TEST_ENV_SCRIPT) build && \
		$(TEST_ENV_SCRIPT) start && \
		$(GOTEST) -v ./... ; \
		test_result=$$? ; \
		$(TEST_ENV_SCRIPT) stop ; \
		exit $$test_result ; \
	else \
		$(GOTEST) -v ./... ; \
	fi

coverage: ## Generate test coverage report
	@echo "Generating coverage report..."
	@if [ -x "$(TEST_ENV_SCRIPT)" ]; then \
		$(TEST_ENV_SCRIPT) build && \
		$(TEST_ENV_SCRIPT) start && \
		$(GOTEST) -cover ./... -coverprofile=$(COVERAGE_FILE) ; \
		test_result=$$? ; \
		$(TEST_ENV_SCRIPT) stop ; \
		if [ $$test_result -eq 0 ]; then \
			$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_FILE).html && \
			echo "Coverage report: $(COVERAGE_FILE).html" ; \
		fi ; \
		exit $$test_result ; \
	else \
		$(GOTEST) -cover ./... -coverprofile=$(COVERAGE_FILE) && \
		$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_FILE).html && \
		echo "Coverage report: $(COVERAGE_FILE).html" ; \
	fi

lint-install: ## Install golangci-lint
	@echo "Installing golangci-lint..."
	@$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

lint: lint-install ## Run golangci-lint
	@echo "Running golangci-lint..."
	@$(GOBIN)/golangci-lint run

lint-fix: lint-install ## Run golangci-lint with auto-fix
	@echo "Running golangci-lint with fixes..."
	@$(GOBIN)/golangci-lint run --fix

deps: ## Download and tidy dependencies
	@echo "Installing dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy

install: build ## Install binary to GOPATH
	@echo "Installing $(BINARY_NAME)..."
	@$(GO) install ./cmd/scanorama

run: build ## Build and run the application
	@echo "Running $(BINARY_NAME)..."
	@./$(BUILD_DIR)/$(BINARY_NAME)

# Database testing targets
test-db-start: ## Start test database in Docker
	@echo "Starting test database..."
	@./test/docker/test-db.sh start

test-db-stop: ## Stop test database
	@echo "Stopping test database..."
	@./test/docker/test-db.sh stop

test-db-status: ## Show test database status
	@./test/docker/test-db.sh status

test-db-cleanup: ## Clean up all test containers and networks
	@echo "Cleaning up test environment..."
	@./test/docker/test-db.sh cleanup

test-unit: ## Run unit tests (no database required)
	@echo "Running unit tests..."
	@$(GOTEST) ./... -short -v

test-db-unit: ## Run database unit tests in Docker
	@echo "Running database unit tests..."
	@./test/docker/test-db.sh test unit

test-db-integration: ## Run database integration tests in Docker
	@echo "Running database integration tests..."
	@./test/docker/test-db.sh test integration

test-db-migration: ## Run database migration tests in Docker
	@echo "Running database migration tests..."
	@./test/docker/test-db.sh test migration

test-db-all: ## Run all database tests in Docker
	@echo "Running all database tests..."
	@./test/docker/test-db.sh test all

test-db-shell: ## Start interactive shell in test environment
	@echo "Starting test shell..."
	@./test/docker/test-db.sh shell

test-db-connect: ## Connect to test database directly
	@echo "Connecting to test database..."
	@./test/docker/test-db.sh db

test-full: test-unit test-db-integration ## Run all tests (unit + integration)

# Local development helpers
validate: ## Validate code formatting and basic checks
	@echo "Validating code formatting..."
	@test -z "$$(gofmt -s -l . | tee /dev/stderr)" || (echo "Files not formatted properly" && exit 1)
	@echo "Running golangci-lint..."
	@$(MAKE) lint

security: ## Run security checks
	@echo "Running security checks..."
	@$(GO) install golang.org/x/vuln/cmd/govulncheck@latest 2>/dev/null || echo "govulncheck install failed"
	@$(GOBIN)/govulncheck ./... 2>/dev/null || echo "govulncheck not available, skipping vulnerability check"

docker-test: ## Test Docker infrastructure locally
	@echo "Testing Docker infrastructure..."
	@docker build -t scanorama-test-local -f test/docker/Dockerfile.testing .
	@docker run --rm scanorama-test-local go version
	@docker run --rm scanorama-test-local nmap --version
	@./test/docker/test-db.sh start
	@sleep 5
	@./test/docker/test-db.sh status
	@./test/docker/test-db.sh cleanup

check: validate lint security test-unit test-db-integration ## Run all checks locally

.DEFAULT_GOAL := help
