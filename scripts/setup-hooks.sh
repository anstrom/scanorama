#!/bin/bash

# Setup script for Git hooks
# This script configures Git to use the project's custom hooks

set -e

REPO_ROOT=$(git rev-parse --show-toplevel)
HOOKS_DIR="$REPO_ROOT/.githooks"

echo "🔧 Setting up Git hooks for scanorama..."

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo "❌ Error: Not in a Git repository"
    exit 1
fi

# Check if hooks directory exists
if [ ! -d "$HOOKS_DIR" ]; then
    echo "❌ Error: Hooks directory not found at $HOOKS_DIR"
    exit 1
fi

# Make all hooks executable
echo "📝 Making hooks executable..."
chmod +x "$HOOKS_DIR"/*

# Configure Git to use the custom hooks directory
echo "⚙️  Configuring Git to use custom hooks directory..."
git config core.hooksPath .githooks

# Hooks left in .git/hooks no longer run once core.hooksPath points at
# .githooks; point them out so a stale copy isn't mistaken for the real one.
GIT_HOOKS_DIR="$(git rev-parse --git-common-dir)/hooks"
STALE_HOOKS=$(find "$GIT_HOOKS_DIR" -maxdepth 1 -type f ! -name '*.sample' 2>/dev/null || true)
if [ -n "$STALE_HOOKS" ]; then
    echo "⚠️  These hooks in $GIT_HOOKS_DIR are now inactive (core.hooksPath=.githooks):"
    while IFS= read -r hook; do
        echo "     $hook"
    done <<< "$STALE_HOOKS"
    echo "   Delete them once you have checked nothing in them is still needed."
fi

# Verify the configuration
CONFIGURED_PATH=$(git config core.hooksPath)
if [ "$CONFIGURED_PATH" = ".githooks" ]; then
    echo "✅ Git hooks successfully configured!"
    echo ""
    echo "📋 Available hooks:"
    ls -la "$HOOKS_DIR"
    echo ""
    echo "🔍 The pre-commit hook will now run linting checks before each commit."
    echo "💡 You can run 'make lint-fix' to automatically fix linting issues."
    echo "🚀 You can run 'make ci' to run all CI checks locally."
else
    echo "❌ Error: Failed to configure Git hooks"
    exit 1
fi
