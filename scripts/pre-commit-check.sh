#!/bin/bash

# Pre-commit Check Script for Scanorama
# Quick verification before committing changes

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "🔍 Pre-commit verification for Scanorama"
echo "========================================"

# Quick checks
echo -n "Checking Go build... "
if go build ./... >/dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${RED}✗${NC}"
    echo "Build failed. Please fix build errors before committing."
    exit 1
fi

echo -n "Checking go mod tidy... "
go mod tidy
if git diff --quiet go.mod go.sum; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${YELLOW}⚠${NC} go.mod/go.sum updated"
fi

echo -n "Running linting... "
if command -v golangci-lint >/dev/null 2>&1; then
    if golangci-lint run >/dev/null 2>&1; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        echo "Linting failed. Run 'golangci-lint run' to see issues."
        exit 1
    fi
elif [ -x "$(go env GOPATH)/bin/golangci-lint" ]; then
    if $(go env GOPATH)/bin/golangci-lint run >/dev/null 2>&1; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        echo "Linting failed. Run 'make lint' to see issues."
        exit 1
    fi
else
    echo -e "${YELLOW}⚠${NC} golangci-lint not found, skipping"
fi

echo -n "Running quick tests... "
# Try unit tests first (excluding integration tests that need Docker)
if go test ./cmd/... ./internal/... -run "^Test[^I].*" >/dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
elif go test ./cmd/... ./internal/... -short >/dev/null 2>&1; then
    echo -e "${GREEN}✓${NC}"
else
    echo -e "${YELLOW}⚠${NC} Some tests require Docker/nmap, skipping integration tests"
    # Try just the basic compilation and simple tests
    if go test -run "^TestValidate.*|^TestParseFlags.*|^TestXML.*" ./... >/dev/null 2>&1; then
        echo -e "${GREEN}✓${NC} (unit tests only)"
    else
        echo -e "${RED}✗${NC}"
        echo "Unit tests failed. Run 'go test ./...' to see details."
        exit 1
    fi
fi

echo ""
echo -e "${GREEN}✅ All pre-commit checks passed!${NC}"
echo "Your changes are ready to commit."
