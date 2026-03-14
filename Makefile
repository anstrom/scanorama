# Scanorama Makefile
# Run `make` or `make help` to see available targets.

# ─── Configuration ───────────────────────────────────────────────────────────

BINARY_NAME  ?= scanorama
BUILD_DIR    := build
COVERAGE_FILE := coverage.out
PID_FILE     := .backend.pid

# Version from git
GIT_VERSION := $(shell git describe --tags --always 2>/dev/null)
VERSION     ?= $(if $(GIT_VERSION),$(GIT_VERSION),dev)
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -X 'main.version=$(VERSION)' -X 'main.commit=$(COMMIT)' -X 'main.buildTime=$(BUILD_TIME)'

# Go
GO      := go
GOTEST  := $(GO) test
GOBUILD := $(GO) build

# Docker compose
DOCKER_COMPOSE  := docker compose
DEV_COMPOSE     := docker/docker-compose.dev.yml
TEST_COMPOSE    := docker/docker-compose.test.yml

# Dev config — copied to root on first `make run`
DEV_CONFIG_SRC  := config/environments/config.dev.yaml
DEV_CONFIG      := config.yaml

# API server defaults (can override: make run PORT=9090)
HOST ?= 127.0.0.1
PORT ?= 8080

# Test database (used by `make test` — on port 5433 to avoid clashing with dev DB)
export TEST_DB_HOST     := localhost
export TEST_DB_PORT     := 5433
export TEST_DB_NAME     := scanorama_test
export TEST_DB_USER     := test_user
export TEST_DB_PASSWORD := test_password

# ─── Default ─────────────────────────────────────────────────────────────────

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@printf '\n\033[1mScanorama\033[0m — $(VERSION)\n\n'
	@printf '\033[1mQuick start:\033[0m\n'
	@printf '  make run           Start backend + frontend (localhost:5173)\n'
	@printf '  make test          Run all tests\n'
	@printf '  make build         Build the binary\n'
	@printf '\n'
	@printf '\033[1mAll targets:\033[0m\n'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@printf '\n'

# ─── Build ───────────────────────────────────────────────────────────────────

.PHONY: build
build: ## Build the scanorama binary
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/scanorama
	@echo "✓ $(BUILD_DIR)/$(BINARY_NAME)"

.PHONY: clean
clean: ## Remove build artifacts and coverage files
	@rm -rf $(BUILD_DIR)
	@rm -f $(COVERAGE_FILE) $(COVERAGE_FILE).html
	@find . -name "*.test" -type f -delete
	@find . -name "coverage.txt" -type f -delete
	@echo "✓ Clean"

# ─── Run ─────────────────────────────────────────────────────────────────────

.PHONY: run
run: build dev-db-up dev-config frontend-deps ## Start backend + frontend dev server
	@echo ""
	@echo "Starting scanorama..."
	@echo "  Backend:  http://$(HOST):$(PORT)/api/v1/health"
	@echo "  Frontend: http://localhost:5173"
	@echo "Press Ctrl-C to stop."
	@echo ""
	@cd frontend && npx vite --clearScreen false &
	@$(BUILD_DIR)/$(BINARY_NAME) api \
		--config $(DEV_CONFIG) \
		--host $(HOST) \
		--port $(PORT) \
		--verbose

.PHONY: run-backend
run-backend: build dev-db-up dev-config ## Start only the backend API server (restarts if running)
	@if [ -f $(PID_FILE) ]; then \
		PID=$$(cat $(PID_FILE)); \
		if kill -0 "$$PID" 2>/dev/null; then \
			kill "$$PID" && echo "↺ Stopped existing backend (pid $$PID)"; \
			sleep 0.5; \
		fi; \
		rm -f $(PID_FILE); \
	fi
	@echo ""
	@echo "Starting scanorama API server on $(HOST):$(PORT)..."
	@echo "Press Ctrl-C to stop."
	@echo ""
	@trap 'rm -f $(PID_FILE)' EXIT; \
	$(BUILD_DIR)/$(BINARY_NAME) api \
		--config $(DEV_CONFIG) \
		--host $(HOST) \
		--port $(PORT) \
		--verbose & \
	echo $$! > $(PID_FILE); \
	wait $$!

.PHONY: frontend
frontend: frontend-deps ## Start the frontend dev server (needs backend running)
	@echo "Starting frontend dev server..."
	@echo "  Open http://localhost:5173"
	@echo ""
	@cd frontend && npx vite

.PHONY: stop-backend
stop-backend: ## Stop a background backend started by run-backend
	@if [ -f $(PID_FILE) ]; then \
		PID=$$(cat $(PID_FILE)); \
		if kill -0 "$$PID" 2>/dev/null; then \
			kill "$$PID" && echo "✓ Backend stopped (pid $$PID)"; \
		else \
			echo "Backend pid $$PID is not running"; \
		fi; \
		rm -f $(PID_FILE); \
	else \
		echo "No PID file found ($(PID_FILE))"; \
	fi

.PHONY: frontend-deps
frontend-deps:
	@cd frontend && npm install --silent

.PHONY: dev-config
dev-config:
	@if [ ! -f $(DEV_CONFIG) ]; then \
		cp $(DEV_CONFIG_SRC) $(DEV_CONFIG); \
		chmod 600 $(DEV_CONFIG); \
		echo "✓ Created $(DEV_CONFIG) from $(DEV_CONFIG_SRC)"; \
	fi

# ─── Dev Infrastructure ─────────────────────────────────────────────────────

.PHONY: dev-db-up
dev-db-up: ## Start dev PostgreSQL (port 5432)
	@echo "Starting dev database..."
	@$(DOCKER_COMPOSE) -f $(DEV_COMPOSE) up -d --wait postgres
	@echo "✓ Dev database ready (localhost:5432)"

.PHONY: dev-down
dev-down: ## Stop dev infrastructure
	@echo "Stopping dev stack..."
	@$(DOCKER_COMPOSE) -f $(DEV_COMPOSE) --profile tools --profile cache --profile targets down
	@echo "✓ Dev stack stopped"

.PHONY: dev-nuke
dev-nuke: ## Stop dev infrastructure and delete all data
	@echo "Destroying dev stack and volumes..."
	@$(DOCKER_COMPOSE) -f $(DEV_COMPOSE) --profile tools --profile cache --profile targets down -v
	@echo "✓ Dev stack destroyed"

.PHONY: dev-db-shell
dev-db-shell: ## Open psql shell to dev database
	@$(DOCKER_COMPOSE) -f $(DEV_COMPOSE) exec postgres psql -U scanorama_dev -d scanorama_dev

.PHONY: dev-logs
dev-logs: ## Tail dev infrastructure logs
	@$(DOCKER_COMPOSE) -f $(DEV_COMPOSE) logs -f

.PHONY: dev-targets
dev-targets: ## Start scan test targets (nginx + SSH)
	@$(DOCKER_COMPOSE) -f $(DEV_COMPOSE) --profile targets up -d
	@echo "✓ Test targets: nginx (8081/8443), SSH (2222)"

# ─── Test Database ───────────────────────────────────────────────────────────
# Test DB runs on port 5433 so it never conflicts with the dev DB on 5432.

.PHONY: test-db-up
test-db-up: ## Start test database (port 5433)
	@echo "Starting test database..."
	@TEST_DB_PORT=$(TEST_DB_PORT) $(DOCKER_COMPOSE) -f $(TEST_COMPOSE) up -d --wait
	@echo "✓ Test database ready (localhost:$(TEST_DB_PORT))"

.PHONY: test-db-down
test-db-down: ## Stop test database
	@$(DOCKER_COMPOSE) -f $(TEST_COMPOSE) down -v 2>/dev/null || true
	@echo "✓ Test database stopped"

.PHONY: test-db-reset
test-db-reset: test-db-down test-db-up ## Reset test database

.PHONY: test-db-shell
test-db-shell: ## Open psql shell to test database
	@$(DOCKER_COMPOSE) -f $(TEST_COMPOSE) exec postgres psql -U $(TEST_DB_USER) -d $(TEST_DB_NAME)

# ─── Testing ─────────────────────────────────────────────────────────────────

.PHONY: test-unit
test-unit: ## Run unit tests (no database needed)
	@echo "Running unit tests..."
	@$(GOTEST) -short ./...
	@echo "✓ Unit tests passed"

.PHONY: test
test: test-db-up ## Run all tests (starts test DB automatically)
	@echo "Running all tests..."
	@$(GOTEST) -v ./... || ($(MAKE) test-db-down; exit 1)
	@echo "✓ All tests passed"
	@$(MAKE) test-db-down

.PHONY: test-keep-db
test-keep-db: test-db-up ## Run all tests, keep test DB running after
	@$(GOTEST) -v ./...

.PHONY: coverage
coverage: test-db-up ## Generate coverage report
	@echo "Running tests with coverage..."
	@$(GOTEST) -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./... \
		|| ($(MAKE) test-db-down; exit 1)
	@$(MAKE) test-db-down
	@$(GO) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_FILE).html
	@echo ""
	@$(GO) tool cover -func=$(COVERAGE_FILE) | tail -1
	@echo "✓ Report: $(COVERAGE_FILE).html"

.PHONY: coverage-show
coverage-show: ## Open coverage report in browser
	@test -f $(COVERAGE_FILE) || (echo "No coverage file. Run 'make coverage' first." && exit 1)
	@$(GO) tool cover -html=$(COVERAGE_FILE)

# ─── Code Quality ────────────────────────────────────────────────────────────

.PHONY: fmt
fmt: ## Format Go code
	@gofmt -s -w .
	@echo "✓ Formatted"

.PHONY: vet
vet: ## Run go vet
	@$(GO) vet ./...
	@echo "✓ Vet passed"

.PHONY: lint
lint: ## Run golangci-lint
	@command -v golangci-lint >/dev/null 2>&1 \
		|| (echo "Install: https://golangci-lint.run/welcome/install/" && exit 1)
	@golangci-lint run ./...
	@echo "✓ Lint passed"

.PHONY: check
check: fmt vet test-unit ## Quick checks: format, vet, unit tests

# ─── Dependencies ────────────────────────────────────────────────────────────

.PHONY: deps
deps: ## Download and tidy Go dependencies
	@$(GO) mod download
	@$(GO) mod tidy
	@echo "✓ Dependencies ready"

.PHONY: deps-upgrade
deps-upgrade: ## Upgrade all Go dependencies
	@$(GO) get -u ./...
	@$(GO) mod tidy
	@echo "✓ Dependencies upgraded"

# ─── Docs ────────────────────────────────────────────────────────────────────

.PHONY: docs
docs: ## Generate Swagger/OpenAPI docs
	@command -v swag >/dev/null 2>&1 \
		|| (echo "Install: go install github.com/swaggo/swag/cmd/swag@latest" && exit 1)
	@cd docs && swag init -g swagger_docs.go -o ./swagger --parseDependency --parseInternal
	@echo "✓ Swagger docs generated"

# ─── CI / Workflows ─────────────────────────────────────────────────────────

.PHONY: ci
ci: deps check test ## Run full CI pipeline locally

.PHONY: dev-setup
dev-setup: deps frontend-deps ## Set up dev environment (install tools + deps)
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "Installing golangci-lint..."; \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
			| sh -s -- -b $$(go env GOPATH)/bin; \
	fi
	@echo ""
	@echo "✓ Ready. Run 'make run' to start developing."

.PHONY: all
all: clean deps build test ## Full rebuild from scratch
