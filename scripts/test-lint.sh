#!/bin/bash

# Script to test golangci-lint installation and running
set -e
set -o pipefail

echo "=== Testing golangci-lint installation and configuration ==="

# Get the directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Navigate to project root
cd "${PROJECT_ROOT}"

echo "1. Checking if golangci-lint is installed..."
GOLANGCI_LINT=$(go env GOPATH)/bin/golangci-lint
echo "Removing any existing golangci-lint installation..."
rm -f "${GOLANGCI_LINT}"
echo "Installing golangci-lint..."
make lint-install

echo "2. Checking golangci-lint version..."
${GOLANGCI_LINT} --version

echo "3. Validating .golangci.yml configuration..."
${GOLANGCI_LINT} config path

echo "4. Running lint on a small subset of files..."
${GOLANGCI_LINT} run ./cmd/... --timeout=30s

echo "5. Checking full project lint..."
${GOLANGCI_LINT} run --timeout=120s

echo "=== All lint tests completed successfully ==="
