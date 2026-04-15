# Scanorama Makefile
# Run `make` or `make help` to see available targets.

# ─── Configuration ───────────────────────────────────────────────────────────

BINARY_NAME  ?= scanorama
BUILD_DIR    := build
COVERAGE_FILE := coverage.out
PID_FILE         := .backend.pid
FRONTEND_PID_FILE := .frontend.pid

# Version from git
GIT_VERSION := $(shell git tag --sort=-v:refname 2>/dev/null | head -1)
VERSION     ?= $(if $(GIT_VERSION),$(GIT_VERSION),dev)
COMMIT      := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS     := -X 'main.version=$(VERSION)' -X 'main.commit=$(COMMIT)' -X 'main.buildTime=$(BUILD_TIME)'

# Go
GO      := go
GOTEST  := $(GO) test -race
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
	@printf '  make dev           Ensure DB is up; rebuild + restart backend + frontend in the background\n'
	@printf '  make stop          Stop background backend and frontend\n'
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

.PHONY: stop
stop: ## Stop background backend and frontend (started by 'make dev')
	@if [ -f $(PID_FILE) ]; then \
		PID=$$(cat $(PID_FILE)); \
		if kill -0 "$$PID" 2>/dev/null; then \
			kill "$$PID" && echo "✓ Backend stopped  (pid $$PID)"; \
		else \
			echo "  Backend pid $$PID is not running"; \
		fi; \
		rm -f $(PID_FILE); \
	else \
		echo "  No backend PID file found"; \
	fi
	@if [ -f $(FRONTEND_PID_FILE) ]; then \
		PID=$$(cat $(FRONTEND_PID_FILE)); \
		if kill -0 "$$PID" 2>/dev/null; then \
			kill "$$PID" && echo "✓ Frontend stopped (pid $$PID)"; \
		else \
			echo "  Frontend pid $$PID is not running"; \
		fi; \
		rm -f $(FRONTEND_PID_FILE); \
	else \
		echo "  No frontend PID file found"; \
	fi



.PHONY: nmap-check
nmap-check: ## Check whether nmap has the SUID bit set; set it if missing (requires sudo)
	@NMAP_BIN=$$(readlink -f $$(which nmap) 2>/dev/null || which nmap 2>/dev/null); \
	if [ -z "$$NMAP_BIN" ]; then \
		echo "✗ nmap not found in PATH — install it first (brew install nmap)"; \
		exit 1; \
	fi; \
	if [ -u "$$NMAP_BIN" ]; then \
		echo "✓ nmap SUID OK ($$NMAP_BIN)"; \
	else \
		echo "⚠ nmap at $$NMAP_BIN does not have the SUID bit set."; \
		echo "  SYN scans and OS detection require nmap to run as root."; \
		echo "  Setting SUID now (you may be prompted for your password)..."; \
		sudo chmod u+s "$$NMAP_BIN" && echo "✓ SUID set on $$NMAP_BIN" || \
			echo "  Could not set SUID — SYN/OS-detection scans will fail until it is set."; \
	fi

.PHONY: dev
dev: build dev-db-up dev-config frontend-deps ## Ensure DB is up; rebuild + restart backend + frontend in the background
	@# ── backend ────────────────────────────────────────────────────────────
	@if [ -f $(PID_FILE) ]; then \
		PID=$$(cat $(PID_FILE)); \
		if kill -0 "$$PID" 2>/dev/null; then \
			kill "$$PID" && echo "↺ Stopped existing backend (pid $$PID)"; \
			sleep 0.3; \
		fi; \
		rm -f $(PID_FILE); \
	fi
	@# Authenticate sudo upfront (foreground, so the password prompt works),
	@# then start the daemon as root in the background using the cached token.
	@sudo -v
	@# Run as root so nmap can perform SYN scans and OS detection without SUID.
	@# Privileges are dropped to daemon.user after initialisation if configured.
	@sudo -E $(BUILD_DIR)/$(BINARY_NAME) api \
		--config $(DEV_CONFIG) \
		--host $(HOST) \
		--port $(PORT) \
		--verbose \
		> /tmp/scanorama-backend.log 2>&1 & \
	echo $$! > $(PID_FILE)
	@echo "✓ Backend  (pid $$(cat $(PID_FILE))) — http://$(HOST):$(PORT)/api/v1/health"
	@echo "  logs: tail -f /tmp/scanorama-backend.log"
	@# ── frontend ───────────────────────────────────────────────────────────
	@if [ -f $(FRONTEND_PID_FILE) ]; then \
		PID=$$(cat $(FRONTEND_PID_FILE)); \
		if kill -0 "$$PID" 2>/dev/null; then \
			kill "$$PID" && echo "↺ Stopped existing frontend (pid $$PID)"; \
			sleep 0.3; \
		fi; \
		rm -f $(FRONTEND_PID_FILE); \
	fi
	@cd frontend && npx vite --clearScreen false \
		> /tmp/scanorama-frontend.log 2>&1 & \
	FPID=$$!; \
	echo $$FPID > ../$(FRONTEND_PID_FILE); \
	echo "✓ Frontend (pid $$FPID) — http://localhost:5173"; \
	echo "  logs: tail -f /tmp/scanorama-frontend.log"



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

.PHONY: coverage-diff
coverage-diff: coverage ## Show per-file coverage for Go files changed vs main (mirrors Codecov patch check)
	@echo ""
	@echo "Coverage for files changed vs main:"
	@BASE=$$(git merge-base origin/main HEAD 2>/dev/null || echo "origin/main"); \
	git diff --name-only $$BASE HEAD -- '*.go' \
		| grep -v '_test\.go' | grep -v '/mocks/' \
		| while read f; do \
			$(GO) tool cover -func=$(COVERAGE_FILE) 2>/dev/null \
				| grep "$$f" | tail -1; \
		done | grep -v '^$$' || echo "  (no data — run make coverage first)"
	@echo ""
	@echo "Total project coverage:"
	@$(GO) tool cover -func=$(COVERAGE_FILE) 2>/dev/null | tail -1 || echo "  (no data — run make coverage first)"
	@echo ""
	@echo "Codecov thresholds (.codecov.yml):"
	@echo "  patch:   >=40% of new/changed lines covered (informational only)"
	@echo "  project: must not drop >2% from base branch (blocking)"
	@echo ""
	@echo "Tip: open coverage.out.html for a line-by-line view — red = uncovered."



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



# ─── Docs ────────────────────────────────────────────────────────────────────

.PHONY: docs
docs: frontend-deps ## Generate Swagger/OpenAPI docs and regenerate frontend types
	@command -v swag >/dev/null 2>&1 \
		|| (echo "Install: go install github.com/swaggo/swag/cmd/swag@latest" && exit 1)
	@cd docs && swag init -g swagger_docs.go -o ./swagger --parseDependency --parseInternal
	@echo "✓ Swagger docs generated"
	@cd frontend && npm run generate-types
	@echo "✓ Frontend types regenerated (src/api/types.ts)"

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
	@echo "✓ Ready. Run 'make dev' to start developing."
