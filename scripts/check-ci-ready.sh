#!/bin/bash

# Simplified CI Readiness Check for Scanorama
# Focuses on key linting and CI issues

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}🔍 Scanorama CI Readiness Check${NC}"
echo "=================================="
echo

# Track results
CHECKS_PASSED=0
CHECKS_FAILED=0

check_pass() {
    echo -e "${GREEN}✅ $1${NC}"
    ((CHECKS_PASSED++))
}

check_fail() {
    echo -e "${RED}❌ $1${NC}"
    ((CHECKS_FAILED++))
}

check_warn() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

# 1. Basic Go environment
echo -e "${BLUE}📋 Go Environment${NC}"
if go version >/dev/null 2>&1; then
    GO_VERSION=$(go version | cut -d' ' -f3)
    check_pass "Go is installed ($GO_VERSION)"
else
    check_fail "Go is not installed"
fi

# 2. Module verification
echo -e "\n${BLUE}📋 Module Status${NC}"
if go mod verify >/dev/null 2>&1; then
    check_pass "Go modules are verified"
else
    check_fail "Go module verification failed"
fi

# 3. Code compilation
echo -e "\n${BLUE}📋 Code Compilation${NC}"
if go build ./... >/dev/null 2>&1; then
    check_pass "Code compiles successfully"
else
    check_fail "Code compilation failed"
fi

# 4. Critical functions exist
echo -e "\n${BLUE}📋 Function Visibility${NC}"
if grep -q "func RunScanWithContext" internal/scan.go; then
    check_pass "RunScanWithContext function found"
else
    check_fail "RunScanWithContext function not found"
fi

if grep -q "func PrintResults" internal/scan.go; then
    check_pass "PrintResults function found"
else
    check_fail "PrintResults function not found"
fi

if grep -q "func RunScan" internal/scan.go; then
    check_pass "RunScan function found"
else
    check_fail "RunScan function not found"
fi

# 5. golangci-lint setup
echo -e "\n${BLUE}📋 Linting Setup${NC}"
GOLANGCI_LINT=""
if command -v golangci-lint >/dev/null 2>&1; then
    GOLANGCI_LINT="golangci-lint"
elif [ -x "$(go env GOPATH)/bin/golangci-lint" ]; then
    GOLANGCI_LINT="$(go env GOPATH)/bin/golangci-lint"
else
    check_warn "golangci-lint not found, installing..."
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    GOLANGCI_LINT="$(go env GOPATH)/bin/golangci-lint"
fi

if [ -n "$GOLANGCI_LINT" ]; then
    check_pass "golangci-lint is available"
else
    check_fail "golangci-lint could not be installed"
fi

# 6. Configuration validation
if [ -f ".golangci.yml" ]; then
    check_pass "golangci-lint config file exists"

    # Check for the specific conflict that was causing issues
    if grep -q "disable-all: true" .golangci.yml && grep -q "disable:" .golangci.yml; then
        check_fail "Configuration conflict: cannot use both 'disable-all' and 'disable' sections"
    else
        check_pass "No disable-all/disable conflict detected"
    fi
else
    check_fail "golangci-lint config file missing"
fi

# 7. Lint execution test
echo -e "\n${BLUE}📋 Linting Execution${NC}"
if [ -n "$GOLANGCI_LINT" ]; then
    if $GOLANGCI_LINT run --timeout=5m >/dev/null 2>&1; then
        check_pass "Linting executes successfully"
    else
        check_fail "Linting failed - check configuration"
        echo "  Try running: $GOLANGCI_LINT run --verbose"
    fi
fi

# 8. CI Configuration consistency
echo -e "\n${BLUE}📋 CI Configuration${NC}"
if [ -f "go.mod" ] && [ -f ".github/workflows/test.yml" ]; then
    GO_MOD_VERSION=$(grep '^go ' go.mod | cut -d' ' -f2)
    CI_GO_VERSION=$(grep 'go-version:' .github/workflows/test.yml | head -1 | sed 's/.*go-version: *"\([^"]*\)".*/\1/')

    if [ "$GO_MOD_VERSION" = "$CI_GO_VERSION" ]; then
        check_pass "Go version consistent between go.mod and CI"
    else
        check_warn "Go version mismatch: go.mod ($GO_MOD_VERSION) vs CI ($CI_GO_VERSION)"
    fi

    if grep -q "GOPRIVATE.*github.com/anstrom" .github/workflows/test.yml; then
        check_pass "GOPRIVATE configured in CI"
    else
        check_warn "GOPRIVATE not configured in CI workflows"
    fi
else
    check_fail "Missing go.mod or CI workflow files"
fi

# 9. Make targets
echo -e "\n${BLUE}📋 Build System${NC}"
if [ -f "Makefile" ]; then
    check_pass "Makefile exists"

    if make lint >/dev/null 2>&1; then
        check_pass "make lint executes successfully"
    else
        check_fail "make lint failed"
    fi

    if make build >/dev/null 2>&1; then
        check_pass "make build executes successfully"
    else
        check_fail "make build failed"
    fi
else
    check_fail "Makefile missing"
fi

# Summary
echo -e "\n${BLUE}📋 Summary${NC}"
echo "=================================="
TOTAL_CHECKS=$((CHECKS_PASSED + CHECKS_FAILED))
echo "Total checks: $TOTAL_CHECKS"
echo -e "Passed: ${GREEN}$CHECKS_PASSED${NC}"
echo -e "Failed: ${RED}$CHECKS_FAILED${NC}"

if [ $CHECKS_FAILED -eq 0 ]; then
    echo -e "\n${GREEN}🎉 All checks passed! Your project is CI-ready.${NC}"
    exit 0
else
    echo -e "\n${RED}⚠️  $CHECKS_FAILED check(s) failed. Please address the issues above.${NC}"
    echo -e "\n${YELLOW}Common fixes:${NC}"
    echo "  • Install golangci-lint: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
    echo "  • Fix .golangci.yml configuration conflicts"
    echo "  • Ensure all functions are properly exported"
    echo "  • Update CI workflow Go versions to match go.mod"
    exit 1
fi
