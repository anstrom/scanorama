#!/bin/bash

# CI Debugging Script for Scanorama
# This script helps identify environment differences that might cause CI failures

set -e

echo "🔍 Scanorama CI Environment Debug Script"
echo "========================================"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

print_section() {
    echo -e "\n${BLUE}📋 $1${NC}"
    echo "----------------------------------------"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_warning() {
    echo -e "${YELLOW}⚠️  $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

# Check Go environment
print_section "Go Environment"
echo "Go version: $(go version)"
echo "GOROOT: $(go env GOROOT)"
echo "GOPATH: $(go env GOPATH)"
echo "GOOS: $(go env GOOS)"
echo "GOARCH: $(go env GOARCH)"
echo "GOCACHE: $(go env GOCACHE)"
echo "GOMODCACHE: $(go env GOMODCACHE)"
echo "GOPRIVATE: ${GOPRIVATE:-'(not set)'}"
echo "CGO_ENABLED: $(go env CGO_ENABLED)"

# Check module information
print_section "Module Information"
echo "Current module: $(go mod print)"
echo -e "\nModule dependencies:"
go list -m all

echo -e "\nModule verification:"
if go mod verify; then
    print_success "All modules verified successfully"
else
    print_error "Module verification failed"
fi

# Check for any replace directives
echo -e "\nChecking for replace directives in go.mod:"
if grep -q "replace " go.mod; then
    print_warning "Found replace directives:"
    grep "replace " go.mod
else
    print_success "No replace directives found"
fi

# Check module download status
print_section "Module Download Status"
echo "Downloading modules..."
if go mod download -x; then
    print_success "Modules downloaded successfully"
else
    print_error "Module download failed"
fi

# Test Go build
print_section "Build Test"
echo "Testing Go build..."
if go build ./...; then
    print_success "Build successful"
else
    print_error "Build failed"
fi

# Test Go compilation
print_section "Compilation Test"
echo "Testing specific package compilation..."
if go build -o /tmp/scanorama-test ./cmd/scanorama; then
    print_success "Main package compilation successful"
    rm -f /tmp/scanorama-test
else
    print_error "Main package compilation failed"
fi

if go build ./internal; then
    print_success "Internal package compilation successful"
else
    print_error "Internal package compilation failed"
fi

# Check golangci-lint installation
print_section "Golangci-lint Environment"
GOBIN=$(go env GOPATH)/bin
echo "GOBIN: $GOBIN"
echo "PATH includes GOBIN: $(echo $PATH | grep -q $GOBIN && echo 'Yes' || echo 'No')"

if command -v golangci-lint &> /dev/null; then
    echo "golangci-lint found in PATH: $(which golangci-lint)"
    echo "golangci-lint version: $(golangci-lint version)"
    print_success "golangci-lint is available"
else
    print_warning "golangci-lint not found in PATH, checking GOBIN..."
    if [ -f "$GOBIN/golangci-lint" ]; then
        echo "golangci-lint found in GOBIN: $GOBIN/golangci-lint"
        echo "golangci-lint version: $($GOBIN/golangci-lint version)"
        print_success "golangci-lint is available in GOBIN"
    else
        print_error "golangci-lint not found"
        echo "Installing golangci-lint..."
        go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
        if [ -f "$GOBIN/golangci-lint" ]; then
            print_success "golangci-lint installed successfully"
        else
            print_error "golangci-lint installation failed"
        fi
    fi
fi

# Test golangci-lint configuration
print_section "Golangci-lint Configuration"
if [ -f ".golangci.yml" ]; then
    print_success "Found .golangci.yml configuration"
    echo "Configuration file size: $(wc -c < .golangci.yml) bytes"
else
    print_warning "No .golangci.yml configuration found"
fi

# Test golangci-lint dry run
print_section "Golangci-lint Dry Run"
echo "Testing golangci-lint with verbose output..."
LINT_CMD="golangci-lint"
if ! command -v golangci-lint &> /dev/null; then
    LINT_CMD="$GOBIN/golangci-lint"
fi

# Run with maximum verbosity to capture any issues
echo "Running: $LINT_CMD run --verbose --timeout=10m"
if $LINT_CMD run --verbose --timeout=10m; then
    print_success "golangci-lint completed successfully"
else
    exit_code=$?
    print_error "golangci-lint failed with exit code: $exit_code"

    echo -e "\n${YELLOW}Trying with different configurations...${NC}"

    # Try without config file
    echo "Trying without config file..."
    if $LINT_CMD run --no-config --timeout=10m; then
        print_warning "golangci-lint works without config file"
    else
        print_error "golangci-lint fails even without config file"
    fi

    # Try with minimal linters
    echo "Trying with minimal linters..."
    if $LINT_CMD run --disable-all --enable=gofmt --timeout=10m; then
        print_warning "golangci-lint works with minimal linters"
    else
        print_error "golangci-lint fails even with minimal linters"
    fi
fi

# Check specific functions that were mentioned as problematic
print_section "Function Visibility Check"
echo "Checking for specific functions mentioned in CI failures..."

check_function() {
    local func_name="$1"
    local file_pattern="$2"

    if grep -r "func.*$func_name" $file_pattern &> /dev/null; then
        print_success "Found function: $func_name"
        echo "Locations:"
        grep -rn "func.*$func_name" $file_pattern
    else
        print_error "Function not found: $func_name"
    fi
}

check_function "RunScanWithContext" "internal/"
check_function "PrintResults" "internal/"
check_function "RunScan" "internal/"

# Check import paths
print_section "Import Path Analysis"
echo "Checking import statements..."
echo "Internal package imports:"
grep -r "github.com/anstrom/scanorama/internal" . --include="*.go" || print_warning "No internal imports found"

echo -e "\nRelative imports:"
grep -r "\"\\./\\|\"\\.\\./" . --include="*.go" || print_success "No problematic relative imports found"

# Environment summary
print_section "Environment Summary"
echo "Operating System: $(uname -s) $(uname -r)"
echo "Architecture: $(uname -m)"
echo "Shell: $SHELL"
echo "User: $(whoami)"
echo "Working Directory: $(pwd)"
echo "Git Status:"
git status --porcelain || echo "Not in a git repository or git not available"

print_section "Debug Complete"
echo "If issues persist in CI, compare this output with your CI logs"
echo "to identify environment differences."
