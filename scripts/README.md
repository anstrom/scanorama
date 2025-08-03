# Scripts Directory

This directory contains utility scripts for development workflow.

## 🚀 Development Scripts

### `pre-commit-check.sh`
Quick verification script to run before committing changes.

**Usage:**
```bash
./scripts/pre-commit-check.sh
```

**What it checks:**
- Go build compilation
- Module dependencies (`go mod tidy`)
- Code linting (if golangci-lint is available)
- Quick tests (`go test -short`)

**When to use:** Before every commit to ensure your changes don't break the build.

## 📋 Make Targets

You can also use the standard Make targets:

```bash
make lint     # Run golangci-lint
make test     # Run full test suite with Docker
make build    # Build the binary
make clean    # Clean build artifacts
```

## 📝 Development Workflow

1. Make your changes
2. Run `./scripts/pre-commit-check.sh` to verify locally
3. Commit and push - CI will run automatically
4. All tests and linting will be verified in CI

## 📦 Requirements

- Go 1.23.0+
- golangci-lint (for linting)
- Docker (for integration tests)
- nmap (for scan functionality tests)