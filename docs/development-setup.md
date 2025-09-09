# Scanorama Development Setup Guide

This guide covers the complete development environment setup for Scanorama, including backend (Go), frontend (TypeScript/React), database, and code quality tools.

## Prerequisites

### System Requirements

- **Go**: >= 1.21
- **Node.js**: >= 18.0.0
- **npm**: >= 9.0.0
- **PostgreSQL**: >= 13
- **Docker**: >= 20.10 (optional, for containerized development)
- **Git**: >= 2.30

### Platform Support

- **macOS**: ARM64 and x86_64
- **Linux**: x86_64, ARM64
- **Windows**: WSL2 recommended

## Quick Start

```bash
# Clone the repository
git clone https://github.com/anstrom/scanorama.git
cd scanorama

# Set up development environment
make dev

# Start development database
make setup-dev-db

# Start API server (in one terminal)
make dev-api

# Start frontend (in another terminal)
make dev-frontend

# OR start everything together
make dev-server
```

This will:
1. Install all dependencies (Go modules + npm packages)
2. Set up the development database using Docker
3. Run initial code quality checks
4. Start the PostgreSQL database, API server, and frontend development server

## Detailed Setup

### 1. Backend (Go) Setup

#### Install Go Dependencies

```bash
# Install Go modules
go mod download

# Verify Go installation
go version
```

#### Environment Configuration

```bash
# Copy example environment file
cp .secrets.local.example .secrets.local

# Edit configuration
vim .secrets.local
```

Configuration is handled via YAML files in `config/environments/`:
- `config/environments/config.dev.yaml` - Development configuration
- `config/environments/config.test.yaml` - Testing configuration  
- `config/environments/config.example.yaml` - Example configuration

The development database credentials are:
```yaml
database:
  host: "localhost"
  port: 5432
  database: "scanorama_dev"
  username: "scanorama_dev"
  password: "dev_password"
```

These are automatically configured when using the Docker development database.

#### Go Tools and Linting

```bash
# Install golangci-lint (done automatically by make lint)
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin

# Install additional tools
go install golang.org/x/tools/cmd/goimports@latest
go install github.com/swaggo/swag/cmd/swag@latest
```

### 2. Frontend (TypeScript/React) Setup

#### Install Node.js Dependencies

```bash
cd frontend
npm install
```

#### Frontend Environment

```bash
# Create frontend environment file
cat > frontend/.env.local << EOF
VITE_API_BASE_URL=http://localhost:8080
VITE_WS_URL=ws://localhost:8080/api/v1/ws
VITE_ENV=development
EOF
```

#### Frontend Development Server

```bash
# Start development server (from frontend directory)
npm run dev

# Or from project root
make dev-frontend
```

The frontend will be available at `http://localhost:3000`.

### 3. Database Setup

#### PostgreSQL Installation

**macOS (using Homebrew):**
```bash
brew install postgresql
brew services start postgresql
```

**Ubuntu/Debian:**
```bash
sudo apt update
sudo apt install postgresql postgresql-contrib
sudo systemctl start postgresql
sudo systemctl enable postgresql
```

**Docker (alternative):**
```bash
make docker-up  # Includes PostgreSQL container
```

#### Database Configuration

The development database runs in Docker and is automatically configured:

```bash
# Start development database (Docker)
make setup-dev-db

# Check database status
docker ps | grep postgres

# Connect to development database
psql -h localhost -p 5432 -U scanorama_dev -d scanorama_dev
# Password: dev_password
```

#### Database Operations

```bash
make setup-dev-db    # Start PostgreSQL in Docker
make dev-stop        # Stop all development services
docker compose -f docker/docker-compose.dev.yml logs postgres  # View database logs
```

The database will be available at `localhost:5432` with:
- Database: `scanorama_dev`
- Username: `scanorama_dev` 
- Password: `dev_password`

Migrations are automatically applied when the API server starts.

### 4. Code Quality Setup

#### ESLint and Prettier (Frontend)

ESLint configuration is already set up in `frontend/.eslintrc.json`:

```bash
# Lint frontend code
make lint-frontend

# Auto-fix frontend issues
make format-frontend

# Strict CI linting (no warnings)
make lint-frontend-ci
```

#### golangci-lint (Backend)

Configuration is in `.golangci.yml`:

```bash
# Lint Go code
make lint-backend

# Auto-fix Go issues
make format-backend
```

#### Combined Linting

```bash
# Lint everything
make lint

# Format everything
make format

# Run all quality checks
make quality
```

### 5. Git Hooks Setup

```bash
# Install Git hooks for pre-commit quality checks
make setup-hooks
```

This sets up hooks that run:
- Go linting and formatting
- Frontend linting and formatting
- Tests (fast subset)
- Security checks

## Development Workflow

### Daily Development

1. **Start development environment:**
   ```bash
   # Option 1: Start everything together
   make dev-server  # Full stack with hot reload
   
   # Option 2: Start components separately (recommended for debugging)
   make setup-dev-db      # Terminal 1: Database
   make dev-api          # Terminal 2: API server  
   make dev-frontend     # Terminal 3: Frontend
   ```

2. **Verify services are running:**
   ```bash
   # Check API health
   curl http://localhost:8080/api/v1/health
   
   # Check frontend (browser)
   open http://localhost:3000
   ```

3. **Make changes and test:**
   ```bash
   make test        # Run all tests
   make quick       # Fast validation (no DB tests)
   ```

4. **Quality checks before commit:**
   ```bash
   make quality     # Comprehensive quality checks
   ```

5. **Stop development servers:**
   ```bash
   make dev-stop    # Stop all services
   ```

### Code Quality Workflow

#### Backend (Go)

```bash
# Development cycle
make lint-backend        # Check code quality
make format-backend      # Auto-fix issues
make test-core          # Fast tests (no DB)
make test               # Full test suite

# Before commit
make quality            # All quality checks
```

#### Frontend (TypeScript/React)

```bash
# Development cycle
cd frontend
npm run lint           # Check with warnings
npm run lint:fix       # Auto-fix issues
npm run format         # Prettier formatting
npm run type-check     # TypeScript validation

# Strict checking (CI-ready)
npm run lint:ci        # No warnings allowed

# From project root
make lint-frontend     # Development linting
make format-frontend   # Format and fix
```

### Testing Workflow

#### Backend Tests

```bash
# Unit tests (fast)
make test-core

# Integration tests (requires DB)
make test-integration

# End-to-end tests
make test-e2e

# All tests with coverage
make coverage
```

#### Frontend Tests

```bash
cd frontend

# Unit tests
npm run test

# Tests with coverage
npm run test:coverage

# Watch mode
npm run test:watch
```

### Build and Deployment

#### Development Builds

```bash
# Backend
go build -o bin/scanorama ./cmd/scanorama

# Frontend
cd frontend && npm run build

# Both (using Makefile)
make build
```

#### Production Builds

```bash
# Docker build
make docker-build

# Release build (using goreleaser)
goreleaser build --snapshot --rm-dist
```

## IDE Setup

### VS Code

Recommended extensions:
- **Go**: Official Go extension
- **TypeScript**: Built-in TypeScript support
- **ESLint**: Frontend linting
- **Prettier**: Code formatting
- **GitLens**: Enhanced Git integration
- **Thunder Client**: API testing

Workspace settings (`.vscode/settings.json`):
```json
{
  "go.useLanguageServer": true,
  "go.formatTool": "goimports",
  "go.lintTool": "golangci-lint",
  "eslint.workingDirectories": ["frontend"],
  "typescript.preferences.importModuleSpecifier": "relative",
  "editor.formatOnSave": true,
  "editor.codeActionsOnSave": {
    "source.fixAll.eslint": true
  }
}
```

### GoLand/IntelliJ IDEA

1. Install Go plugin
2. Enable golangci-lint integration
3. Configure code style to match `.editorconfig`
4. Set up run configurations for tests and main application

## Troubleshooting

### Common Issues

#### 1. Go Module Issues

```bash
# Clean module cache
go clean -modcache

# Re-download modules
go mod download

# Verify modules
go mod verify
```

#### 2. Node.js/npm Issues

```bash
# Clear npm cache
npm cache clean --force

# Remove node_modules and reinstall
rm -rf frontend/node_modules frontend/package-lock.json
cd frontend && npm install
```

#### 3. Database Connection Issues

```bash
# Check Docker database status
docker ps | grep postgres

# Check database logs
docker compose -f docker/docker-compose.dev.yml logs postgres

# Test connection
psql -h localhost -p 5432 -U scanorama_dev -d scanorama_dev
# Password: dev_password

# Reset database
make dev-stop
docker compose -f docker/docker-compose.dev.yml down -v
make setup-dev-db
```

#### 4. Port Conflicts

```bash
# Check what's using port 8080 (API)
lsof -i :8080

# Check what's using port 3000 (Frontend)
lsof -i :3000

# Kill process using port
kill -9 $(lsof -ti:8080)
```

#### 5. ESLint Configuration Issues

```bash
# Check ESLint configuration
cd frontend
npx eslint --print-config src/App.tsx

# Clear ESLint cache
npx eslint --cache-location=/tmp/eslint-cache src/

# Reinstall ESLint dependencies
npm uninstall eslint @typescript-eslint/eslint-plugin @typescript-eslint/parser
npm install --save-dev eslint @typescript-eslint/eslint-plugin @typescript-eslint/parser
```

#### 6. TypeScript Issues

```bash
# Full TypeScript check
cd frontend && npm run type-check

# Clear TypeScript cache
rm -rf frontend/node_modules/.cache

# Rebuild TypeScript
cd frontend && npx tsc --build --clean && npx tsc --build
```

### Performance Issues

#### Slow Tests
```bash
# Run tests in parallel
go test -parallel 4 ./...

# Run only fast tests
make test-core

# Profile tests
go test -cpuprofile cpu.prof ./...
```

#### Slow Frontend Build
```bash
# Analyze bundle size
cd frontend && npm run build -- --analyze

# Use faster development build
npm run dev

# Clear Vite cache
rm -rf frontend/node_modules/.vite
```

### Debug Mode

#### Backend Debugging
```bash
# Enable debug logging
export LOG_LEVEL=debug

# Run with race detection
go run -race ./cmd/scanorama

# Use delve debugger
dlv debug ./cmd/scanorama
```

#### Frontend Debugging
```bash
# Enable debug mode
export VITE_DEBUG=true

# Run with source maps
npm run dev

# Debug in browser DevTools
# - React Developer Tools
# - Redux DevTools (if using Redux)
```

## Advanced Configuration

### Custom Makefile Targets

Create local Makefile overrides:
```makefile
# Makefile.local (git-ignored)
dev-custom: ## Custom development setup
	@echo "Running custom development setup..."
	@$(MAKE) db-setup
	@$(MAKE) dev-frontend &
	@$(MAKE) dev-api

include Makefile.local
```

### Environment-Specific Configs

Configuration files are located in `config/environments/`:

```bash
# Development (used by make dev-api)
config/environments/config.dev.yaml

# Testing (used by test suite)  
config/environments/config.test.yaml

# Use specific config manually
go run ./cmd/scanorama --config config/environments/config.dev.yaml api

# Or use make targets (recommended)
make dev-api      # Uses config.dev.yaml automatically
make dev-server   # Uses config.dev.yaml automatically
```

**Important**: Config files need proper permissions:
```bash
chmod 600 config/environments/*.yaml
```

### Docker Development

```bash
# Full stack with Docker
make docker-up

# Development with hot reload
docker-compose -f docker-compose.dev.yml up

# Cleanup
make docker-down
```

## Best Practices

### Git Workflow

1. **Feature Branches**: Always work on feature branches
2. **Commit Messages**: Use conventional commits (feat:, fix:, docs:)
3. **Small Commits**: Make atomic, focused commits
4. **Quality Gates**: Ensure all checks pass before merging

### Code Organization

1. **Go**: Follow standard Go project layout
2. **TypeScript**: Use absolute imports for components
3. **Tests**: Co-locate tests with source files
4. **Documentation**: Keep docs updated with code changes

### Performance

1. **Backend**: Profile regularly, optimize database queries
2. **Frontend**: Use React Developer Tools, monitor bundle size
3. **Database**: Index frequently queried columns
4. **Caching**: Implement appropriate caching strategies

### Security

1. **Secrets**: Never commit secrets, use environment variables
2. **Dependencies**: Regularly audit and update dependencies
3. **Linting**: Use security-focused linting rules
4. **Testing**: Include security tests in your test suite

## Resources

### Documentation
- [Go Documentation](https://golang.org/doc/)
- [React Documentation](https://reactjs.org/docs)
- [TypeScript Handbook](https://www.typescriptlang.org/docs)
- [PostgreSQL Documentation](https://www.postgresql.org/docs/)

### Tools
- [golangci-lint](https://golangci-lint.run/)
- [ESLint](https://eslint.org/)
- [Prettier](https://prettier.io/)
- [Vite](https://vitejs.dev/)
- [React Query](https://react-query.tanstack.com/)

### Community
- [Go Community](https://golang.org/help/)
- [React Community](https://reactjs.org/community/support.html)
- [TypeScript Community](https://www.typescriptlang.org/community/)

---

This setup guide should get you up and running with Scanorama development. For specific feature development, refer to the architecture documentation and API specifications.