# Scanorama Makefile

# Configuration
BINARY_NAME := scanorama
BUILD_DIR := build
COVERAGE_FILE := coverage.out
GO := go
DOCKER_COMPOSE := docker compose
DEV_COMPOSE_FILE := docker/docker-compose.dev.yml

# Version information
GIT_VERSION := $(shell git describe --tags --always 2>/dev/null)
VERSION ?= $(if $(GIT_VERSION),$(GIT_VERSION),dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X 'main.version=$(VERSION)' -X 'main.commit=$(COMMIT)' -X 'main.buildTime=$(BUILD_TIME)'

.PHONY: help build test lint clean deps dev-db dev-server dev-frontend stop-dev

help: ## Show help
	@echo 'Available targets:'
	@echo '  build         - Build the application'
	@echo '  test          - Run tests'
	@echo '  lint          - Run linters'
	@echo '  clean         - Clean build artifacts'
	@echo '  deps          - Install dependencies'
	@echo '  dev-db        - Start development database'
	@echo '  dev-server    - Start full development environment (DB + API + Frontend)'
	@echo '  dev-frontend  - Start frontend development'
	@echo '  stop-dev      - Stop development services'

build: ## Build the application
	@echo "Building $(BINARY_NAME) $(VERSION)..."
	@mkdir -p $(BUILD_DIR)
	@$(GO) build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/$(BINARY_NAME)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

test: ## Run tests
	@echo "Running tests..."
	@$(GO) test -short -race ./...
	@echo "Tests complete"

lint: ## Run linters
	@echo "Running linters..."
	@if [ "$$(gofmt -s -l . | wc -l)" -gt 0 ]; then \
		echo "Code not formatted:"; \
		gofmt -s -l .; \
		exit 1; \
	fi
	@golangci-lint run || { echo "Install golangci-lint: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; exit 1; }
	@$(GO) mod verify
	@$(GO) mod tidy
	@if [ -d "frontend" ]; then cd frontend && npm run lint; fi
	@echo "Linting complete"

clean: ## Clean build artifacts
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(COVERAGE_FILE) *.log
	@$(GO) clean
	@echo "Clean complete"

deps: ## Install dependencies
	@echo "Installing dependencies..."
	@$(GO) mod download
	@$(GO) mod tidy
	@if [ -d "frontend" ]; then cd frontend && npm install; fi
	@if [ -f "package.json" ]; then npm install; fi
	@echo "Dependencies updated"

dev-db: ## Start development database
	@echo "Starting database..."
	@$(DOCKER_COMPOSE) -f $(DEV_COMPOSE_FILE) up -d postgres
	@if $(DOCKER_COMPOSE) -f $(DEV_COMPOSE_FILE) exec -T postgres pg_isready -U scanorama_dev -d scanorama_dev >/dev/null 2>&1; then \
		echo "Database ready"; \
	else \
		echo "Waiting for database..."; \
		timeout 60 sh -c 'until $(DOCKER_COMPOSE) -f $(DEV_COMPOSE_FILE) exec -T postgres pg_isready -U scanorama_dev -d scanorama_dev >/dev/null 2>&1; do sleep 1; done'; \
		echo "Database ready"; \
	fi

dev-server: build ## Start full development environment (DB + API + Frontend)
	@echo "🚀 Starting full development environment..."

	@echo "Checking database..."
	@if ! $(DOCKER_COMPOSE) -f $(DEV_COMPOSE_FILE) ps --filter "status=running" | grep -q postgres; then \
		echo "Starting database..."; \
		$(DOCKER_COMPOSE) -f $(DEV_COMPOSE_FILE) up -d postgres; \
		echo "Waiting for database..."; \
		timeout 60 sh -c 'until $(DOCKER_COMPOSE) -f $(DEV_COMPOSE_FILE) exec -T postgres pg_isready -U scanorama_dev -d scanorama_dev >/dev/null 2>&1; do sleep 1; done'; \
		echo "✅ Database ready"; \
	else \
		echo "✅ Database already running"; \
	fi

	@echo "Checking API server..."
	@if ! curl -s http://localhost:8080/api/v1/health >/dev/null 2>&1; then \
		echo "Starting API server..."; \
		pkill -f "$(BINARY_NAME) api" || true; \
		sleep 1; \
		./$(BUILD_DIR)/$(BINARY_NAME) api --config config/environments/config.dev.yaml > api-server.log 2>&1 & echo $$! > api-server.pid; \
		echo "Waiting for API server..."; \
		timeout 30 sh -c 'until curl -s http://localhost:8080/api/v1/health >/dev/null 2>&1; do sleep 1; done'; \
		if curl -s http://localhost:8080/api/v1/health >/dev/null 2>&1; then \
			echo "✅ API server ready at http://localhost:8080"; \
		else \
			echo "❌ API server failed to start. Check logs: cat api-server.log"; \
			exit 1; \
		fi \
	else \
		echo "✅ API server already running"; \
	fi

	@echo ""
	@echo "🎉 Backend services ready:"
	@echo "  Database: http://localhost:5432"
	@echo "  API: http://localhost:8080"
	@echo "  API Docs: http://localhost:8080/swagger/"
	@echo ""
	@echo "Starting frontend (Ctrl+C to stop all)..."
	@if [ ! -d "frontend/node_modules" ]; then \
		echo "Installing frontend dependencies..."; \
		cd frontend && npm install; \
	fi
	@cd frontend && npm run dev

dev-frontend: ## Start frontend development server only
	@echo "Starting frontend development server..."
	@if [ ! -d "frontend/node_modules" ]; then \
		echo "Installing frontend dependencies..."; \
		cd frontend && npm install; \
	fi
	@echo "Frontend will start at http://localhost:3000"
	@cd frontend && npm run dev

stop-dev: ## Stop development services
	@echo "Stopping services..."
	@$(DOCKER_COMPOSE) -f $(DEV_COMPOSE_FILE) down
	@pkill -f "$(BINARY_NAME) api" || true
	@pkill -f "vite" || true
	@rm -f api-server.pid
	@echo "Services stopped"

.DEFAULT_GOAL := help
