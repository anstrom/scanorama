#!/bin/bash

# Simple lint script for Scanorama
# This script runs basic Go linters with minimal configuration

set -e

echo "=== Running simple linters for Scanorama ==="

# Ensure we're in the project root
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_ROOT}"

# Get command line arguments
LINT_DIRS=""
if [ "$#" -eq 0 ]; then
    # Default: Run on the entire project
    LINT_DIRS="./internal ./cmd"
else
    # Pass all arguments to linters
    LINT_DIRS="$*"
fi

# Run gofmt to check formatting
echo "Running gofmt..."
# Find Go files in specified directories
GO_FILES=""
for dir in ${LINT_DIRS}; do
    if [ -d "$dir" ]; then
        DIR_FILES=$(find "$dir" -name "*.go" 2>/dev/null)
        if [ -n "$DIR_FILES" ]; then
            if [ -n "$GO_FILES" ]; then
                GO_FILES="$GO_FILES $DIR_FILES"
            else
                GO_FILES="$DIR_FILES"
            fi
        fi
    fi
done

if [ -z "${GO_FILES}" ]; then
    echo "⚠️ No Go files found in specified paths"
else
    BADLY_FORMATTED=$(gofmt -l -s ${GO_FILES})
    if [ -z "$BADLY_FORMATTED" ]; then
        echo "✅ gofmt check passed"
    else
        echo "❌ gofmt check failed - code is not properly formatted"
        echo "$BADLY_FORMATTED"
        exit 1
    fi
fi

# Run go vet to find suspicious code
echo "Running go vet..."
VET_FAILED=0
for dir in ${LINT_DIRS}; do
    # Check if directory exists and contains Go files
    if [ -d "$dir" ] && ls "$dir"/*.go >/dev/null 2>&1; then
        if ! go vet "./$dir/..."; then
            echo "❌ go vet check failed for $dir"
            VET_FAILED=1
        fi
    fi
done

if [ $VET_FAILED -eq 0 ]; then
    echo "✅ go vet check passed"
else
    exit 1
fi

# Optionally check for misspellings
if command -v misspell >/dev/null 2>&1; then
    echo "Running misspell..."
    MISSPELL_FAILED=0
    for dir in ${LINT_DIRS}; do
        if [ -d "$dir" ]; then
            if ! misspell -error "$dir"; then
                echo "❌ misspell check failed for $dir"
                MISSPELL_FAILED=1
            fi
        fi
    done

    if [ $MISSPELL_FAILED -eq 0 ]; then
        echo "✅ misspell check passed"
    else
        exit 1
    fi
else
    echo "misspell not installed, skipping spell check"
fi

# Run go-critic if available
if command -v gocritic >/dev/null 2>&1; then
    echo "Running gocritic..."
    CRITIC_FAILED=0
    for dir in ${LINT_DIRS}; do
        # Check if directory exists and contains Go files
        if [ -d "$dir" ] && ls "$dir"/*.go >/dev/null 2>&1; then
            if ! gocritic check "./$dir/..."; then
                echo "❌ gocritic check failed for $dir"
                CRITIC_FAILED=1
            fi
        fi
    done

    if [ $CRITIC_FAILED -eq 0 ]; then
        echo "✅ gocritic check passed"
    else
        exit 1
    fi
else
    echo "gocritic not installed, skipping check"
fi

echo "✅ All lint checks passed!"
