# Build configuration
BINARY_NAME := scanorama
BUILD_DIR := build
COVERAGE_FILE := coverage.out
TEST_ENV_SCRIPT := ./test/docker/test-env.sh
DB_DEBUG ?= false
# Use default PostgreSQL port for simplicity
POSTGRES_PORT ?= 5432

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

.PHONY: help build clean clean-test test test-up test-down test-logs test-debug test-local coverage lint lint-install lint-fix deps install run fmt vet check all

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

coverage: test-up ## Generate test coverage report
	@echo "Generating coverage report..."
	@POSTGRES_PORT=$(POSTGRES_PORT) $(GOTEST) -cover ./... -coverprofile=$(COVERAGE_FILE) ; ret=$$? ; \
	if [ $$ret -eq 0 ]; then \
		$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_FILE).html && \
		echo "Coverage report: $(COVERAGE_FILE).html" ; \
	fi ; \
	make test-down ; \
	exit $$ret

lint-install: ## Install simple linting tools
	@echo "Installing basic linting tools..."
	@go install github.com/client9/misspell/cmd/misspell@latest

lint: lint-install ## Run linters
	@echo "Running linters..."
	@./scripts/simple-lint.sh

lint-fix: lint-install ## Fix formatting and common issues
	@echo "Running formatters with fixes..."
	@find . -type f -name "*.go" -not -path "./vendor/*" | xargs gofmt -s -w
	@misspell -w .

fmt: ## Run gofmt on all Go files
	@echo "Running gofmt..."
	@find . -type f -name "*.go" -not -path "./vendor/*" | xargs gofmt -s -w

vet: ## Run go vet on all packages
	@echo "Running go vet..."
	@$(GO) vet ./...

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

check: fmt vet lint test ## Run format, vet, lint and tests
	@echo "All checks passed!"

test-logs: ## View logs from test containers
	@echo "Viewing test container logs..."
	@$(TEST_ENV_SCRIPT) logs

all: clean deps check build ## Clean, install dependencies, run checks and build

.DEFAULT_GOAL := help
