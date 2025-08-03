#!/bin/bash

# CI Status Verification Script for Scanorama
# This script verifies that all CI components are working correctly

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Status tracking
TOTAL_CHECKS=0
PASSED_CHECKS=0
FAILED_CHECKS=0

print_header() {
    echo -e "\n${BOLD}${BLUE}🔍 Scanorama CI Status Verification${NC}"
    echo "===================================="
    echo "This script verifies that all CI components are working correctly."
    echo ""
}

print_section() {
    echo -e "\n${BOLD}${BLUE}📋 $1${NC}"
    echo "----------------------------------------"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
    ((PASSED_CHECKS++))
}

print_failure() {
    echo -e "${RED}❌ $1${NC}"
    ((FAILED_CHECKS++))
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

run_check() {
    local check_name="$1"
    local command="$2"

    ((TOTAL_CHECKS++))

    echo -n "Checking $check_name... "

    if eval "$command" >/dev/null 2>&1; then
        print_success "$check_name"
        return 0
    else
        print_failure "$check_name"
        return 1
    fi
}

run_check_with_output() {
    local check_name="$1"
    local command="$2"

    ((TOTAL_CHECKS++))

    echo "Checking $check_name..."

    if eval "$command"; then
        print_success "$check_name"
        return 0
    else
        print_failure "$check_name"
        return 1
    fi
}

# Start verification
print_header

# Basic environment checks
print_section "Environment Verification"

run_check "Go installation" "command -v go"
run_check "Go version compatibility" "go version | grep -E 'go1\.(23|24)'"
run_check "Git repository" "git status"
run_check "Go module validity" "go mod verify"

# Configuration checks
print_section "Configuration Verification"

run_check "golangci-lint config exists" "test -f .golangci.yml"
run_check "golangci-lint config is valid" "golangci-lint config verify 2>/dev/null || $(go env GOPATH)/bin/golangci-lint config verify 2>/dev/null"
run_check "Makefile exists" "test -f Makefile"
run_check "CI workflow exists" "test -f .github/workflows/test.yml"

# Code quality checks
print_section "Code Quality Verification"

run_check_with_output "Go module download" "go mod download"
run_check_with_output "Code compilation" "go build ./..."
run_check_with_output "Linting" "make lint"

# Test verification
print_section "Test Verification"

# Check if Docker is available for integration tests
if command -v docker >/dev/null 2>&1; then
    run_check_with_output "Full test suite" "make test"
else
    print_warning "Docker not available, skipping integration tests"
    run_check_with_output "Unit tests only" "go test ./... -short"
fi

# Build verification
print_section "Build Verification"

run_check_with_output "Binary build" "make build"
run_check "Binary execution" "./build/scanorama -version"

# Function visibility check (specific to our previous issues)
print_section "Function Visibility Check"

check_function_exists() {
    local func_name="$1"
    local package_path="$2"

    if grep -r "func.*$func_name" "$package_path" >/dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

run_check "RunScanWithContext function" "check_function_exists 'RunScanWithContext' 'internal/'"
run_check "PrintResults function" "check_function_exists 'PrintResults' 'internal/'"
run_check "RunScan function" "check_function_exists 'RunScan' 'internal/'"

# Import verification
print_section "Import Verification"

run_check "Internal package imports" "grep -r 'github.com/anstrom/scanorama/internal' . --include='*.go' >/dev/null 2>&1"
run_check "No problematic relative imports" "! grep -r '\"\\.\\.\\?/' . --include='*.go' >/dev/null 2>&1"

# CI-specific checks
print_section "CI Configuration Verification"

# Check Go version consistency
GO_MOD_VERSION=$(grep '^go ' go.mod | cut -d' ' -f2)
CI_GO_VERSION=$(grep 'go-version:' .github/workflows/test.yml | head -1 | cut -d'"' -f2)

if [ "$GO_MOD_VERSION" = "$CI_GO_VERSION" ]; then
    print_success "Go version consistency between go.mod ($GO_MOD_VERSION) and CI ($CI_GO_VERSION)"
    ((PASSED_CHECKS++))
else
    print_failure "Go version mismatch: go.mod ($GO_MOD_VERSION) vs CI ($CI_GO_VERSION)"
    ((FAILED_CHECKS++))
fi
((TOTAL_CHECKS++))

# Check GOPRIVATE setting in CI
if grep -q "GOPRIVATE.*github.com/anstrom" .github/workflows/test.yml 2>/dev/null; then
    print_success "GOPRIVATE configured in CI"
    ((PASSED_CHECKS++))
else
    print_failure "GOPRIVATE not configured in CI"
    ((FAILED_CHECKS++))
fi
((TOTAL_CHECKS++))

# Final status report
print_section "Verification Summary"

echo "Total checks: $TOTAL_CHECKS"
echo -e "Passed: ${GREEN}$PASSED_CHECKS${NC}"
echo -e "Failed: ${RED}$FAILED_CHECKS${NC}"

if [ $FAILED_CHECKS -eq 0 ]; then
    echo ""
    print_success "All checks passed! CI should work correctly."
    echo ""
    print_info "Your project is ready for CI/CD operations."
    exit 0
else
    echo ""
    print_failure "$FAILED_CHECKS out of $TOTAL_CHECKS checks failed."
    echo ""
    print_info "Please review the failed checks above and fix any issues."
    print_info "Common fixes:"
    print_info "  - Ensure golangci-lint is installed: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
    print_info "  - Check Go version consistency between go.mod and CI workflows"
    print_info "  - Verify all required functions are exported in internal package"
    print_info "  - Ensure GOPRIVATE is set correctly in CI environment"
    exit 1
fi
