# Build configuration
BINARY_NAME := scanorama
BUILD_DIR := build
COVERAGE_FILE := coverage.out
TEST_ENV_SCRIPT := ./test/docker/test-env.sh

# Version information
VERSION := $(shell git describe --tags --always --dirty)
COMMIT := $(shell git rev-parse --short HEAD)
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Build flags
BUILD_FLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)"

# Docker configuration
DEV_IMAGE := scanorama-dev
TEST_IMAGE := scanorama-test
DOCKER_RUN := docker run --rm -v $(PWD):/app -v /var/run/docker.sock:/var/run/docker.sock -w /app --network host --privileged

# Go commands
GO := go
GOFLAGS :=
GOTEST := $(GO) test -buildvcs=false
GOBUILD := $(GO) build -buildvcs=false
GOMOD := $(GO) mod
GOINSTALL := $(GO) install -buildvcs=false

# Cross compilation
PLATFORMS := linux/amd64 darwin/amd64 windows/amd64

# Get GOPATH and GOBIN
GOPATH := $(shell $(GO) env GOPATH)
GOBIN := $(GOPATH)/bin
export PATH := $(GOBIN):$(PATH)

.PHONY: all build clean test coverage deps help install run lint lint-install \
        docker-build docker-test docker-lint docker-dev docker-clean docker-coverage \
        docker-lint-go docker-lint-yaml docker-lint-markdown \
        lint-yaml lint-markdown lint-all test-env-start test-env-stop test-env-clean \
        cross-build release

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z_-]+:.*?## / { \
		printf "  \033[36m%-20s\033[0m %s\n", \
		substr($$1, 1, length($$1)-1), \
		substr($$0, index($$0, "##") + 3) \
	}' $(MAKEFILE_LIST)

all: clean deps lint test-env-clean test-env-start test build test-env-stop ## Clean, install deps, lint, test and build

build: deps ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@$(GOBUILD) -v -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/scanorama
	@echo "Built version $(VERSION) ($(COMMIT))"

cross-build: deps ## Build for all supported platforms
	@echo "Building $(BINARY_NAME) for multiple platforms..."
	@mkdir -p $(BUILD_DIR)
	@$(foreach platform,$(PLATFORMS),\
		GOOS=$(word 1,$(subst /, ,$(platform))) \
		GOARCH=$(word 2,$(subst /, ,$(platform))) \
		$(GOBUILD) -v -o $(BUILD_DIR)/$(BINARY_NAME)_$(word 1,$(subst /, ,$(platform)))_$(word 2,$(subst /, ,$(platform)))$(if $(findstring windows,$(platform)),.exe,) \
		./cmd/scanorama; \
	)

release: cross-build ## Create release artifacts
	@echo "Creating release artifacts..."
	@cd $(BUILD_DIR) && \
		$(foreach platform,$(PLATFORMS),\
			tar czf $(BINARY_NAME)_$(word 1,$(subst /, ,$(platform)))_$(word 2,$(subst /, ,$(platform))).tar.gz \
				$(BINARY_NAME)_$(word 1,$(subst /, ,$(platform)))_$(word 2,$(subst /, ,$(platform)))$(if $(findstring windows,$(platform)),.exe,); \
		)

clean: ## Remove build artifacts
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(COVERAGE_FILE)
	@rm -f $(COVERAGE_FILE).html
	@rm -f $(BUILD_DIR)/*.tar.gz

test-env-start: ## Start test environment
	@echo "Starting test environment..."
	@$(TEST_ENV_SCRIPT) build
	@$(TEST_ENV_SCRIPT) start

test-env-stop: ## Stop test environment
	@echo "Stopping test environment..."
	@$(TEST_ENV_SCRIPT) stop

test-env-clean: ## Clean test environment
	@echo "Cleaning test environment..."
	@$(TEST_ENV_SCRIPT) clean

test: ## Run tests
	@echo "Running tests..."
	@$(GOTEST) -v -timeout 5m ./...

coverage: ## Generate test coverage report
	@echo "Generating coverage report..."
	@$(GOTEST) -cover -timeout 5m ./... -coverprofile=$(COVERAGE_FILE)
	@$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_FILE).html
	@echo "Coverage report generated: $(COVERAGE_FILE).html"

lint-install: ## Install golangci-lint
	@echo "Installing golangci-lint..."
	@$(GOINSTALL) github.com/golangci/golangci-lint/cmd/golangci-lint@latest

lint: lint-install ## Run linters
	@echo "Running linters..."
	@$(GO) vet ./...
	@$(GOBIN)/golangci-lint run

deps: ## Download dependencies
	@echo "Installing dependencies..."
	@$(GOMOD) download
	@$(GOMOD) tidy

install: build ## Install binary to $GOPATH/bin
	@echo "Installing $(BINARY_NAME)..."
	@$(GOINSTALL) ./cmd/scanorama

run: build ## Run the application
	@echo "Running $(BINARY_NAME)..."
	@./$(BUILD_DIR)/$(BINARY_NAME)

# Docker development environment
docker-build: ## Build development and test Docker images
	@echo "Building development image..."
	@docker build -t $(DEV_IMAGE) -f Dockerfile.dev .
	@echo "Building test environment image..."
	@chmod +x $(TEST_ENV_SCRIPT)
	@$(TEST_ENV_SCRIPT) build

docker-dev: docker-build ## Run development environment in Docker
	@echo "Starting development environment..."
	@$(DOCKER_RUN) -it $(DEV_IMAGE) bash

docker-test: docker-build ## Run tests in Docker
	@echo "Running tests in Docker..."
	@$(DOCKER_RUN) $(DEV_IMAGE) sh -c "\
		$(TEST_ENV_SCRIPT) clean && \
		$(TEST_ENV_SCRIPT) build && \
		$(TEST_ENV_SCRIPT) start && \
		go test -v ./... ; \
		status=\$$? ; \
		$(TEST_ENV_SCRIPT) clean ; \
		exit \$$status"

docker-coverage: docker-build ## Run tests with coverage in Docker
	@echo "Running tests with coverage in Docker..."
	@$(DOCKER_RUN) $(DEV_IMAGE) sh -c "\
		$(TEST_ENV_SCRIPT) clean && \
		$(TEST_ENV_SCRIPT) build && \
		$(TEST_ENV_SCRIPT) start && \
		go test -cover ./... -coverprofile=$(COVERAGE_FILE) && \
		go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_FILE).html ; \
		status=\$$? ; \
		$(TEST_ENV_SCRIPT) clean ; \
		exit \$$status"
	@echo "Coverage report generated: $(COVERAGE_FILE).html"

docker-lint: docker-build docker-lint-go docker-lint-yaml docker-lint-markdown ## Run all linters in Docker
	@echo "All linters completed"

docker-lint-go: docker-build ## Run Go linters only
	@echo "Running Go linters..."
	@$(DOCKER_RUN) $(DEV_IMAGE) sh -c "$(GO) vet ./..."

docker-lint-yaml: docker-build ## Run YAML linter only
	@echo "Running YAML linter..."
	@$(DOCKER_RUN) $(DEV_IMAGE) yamllint -c .yamllint .

docker-lint-markdown: docker-build ## Run Markdown linter only
	@echo "Running Markdown linter..."
	@$(DOCKER_RUN) $(DEV_IMAGE) markdownlint '**/*.md'



lint-go: docker-build ## Run Go linters only
	@echo "Running Go linters..."
	@$(DOCKER_RUN) $(DEV_IMAGE) sh -c "$(GO) vet ./..."

lint-yaml: docker-build ## Run YAML linter only
	@echo "Running YAML linter..."
	@$(DOCKER_RUN) $(DEV_IMAGE) yamllint -c .yamllint .

lint-markdown: docker-build ## Run Markdown linter only
	@echo "Running Markdown linter..."
	@$(DOCKER_RUN) $(DEV_IMAGE) markdownlint '**/*.md'



docker-clean: ## Clean Docker images
	@echo "Cleaning Docker images..."
	@docker rmi $(DEV_IMAGE) $(TEST_IMAGE) 2>/dev/null || true
	@$(TEST_ENV_SCRIPT) clean

.DEFAULT_GOAL := help
