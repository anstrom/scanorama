#!/bin/bash

# CI Pipeline Improvements PR Creation Script
# This script creates a branch, commits changes, and prepares for PR creation

set -e

# Configuration
BRANCH_NAME="feature/ci-pipeline-improvements"
COMMIT_MESSAGE="feat: enhance CI pipeline with Docker cleanup and local workflows

- Add comprehensive Docker space management
- Create local CI workflow excluding GitHub-specific jobs
- Add new Makefile targets for CI and Docker operations
- Enhance security scanning with better gosec configuration
- Improve hardcoded secrets detection
- Add license compliance checking
- Implement better error handling and reporting

Resolves Docker space issues and improves developer experience."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 Creating CI Pipeline Improvements PR${NC}"
echo "=================================================="

# Check if we're in the right directory
if [ ! -f "Makefile" ] || [ ! -d ".github" ]; then
    echo -e "${RED}❌ Error: This script must be run from the project root directory${NC}"
    exit 1
fi

# Check if git is clean (allow for our new files)
if git diff --quiet && git diff --staged --quiet; then
    echo -e "${GREEN}✅ Working directory is clean${NC}"
else
    echo -e "${YELLOW}⚠️  You have uncommitted changes. Continuing...${NC}"
fi

# Create and switch to new branch
echo -e "${BLUE}🔄 Creating branch: ${BRANCH_NAME}${NC}"
git checkout -b "${BRANCH_NAME}" 2>/dev/null || {
    echo -e "${YELLOW}⚠️  Branch already exists, switching to it${NC}"
    git checkout "${BRANCH_NAME}"
}

# Add the files that were modified/created
echo -e "${BLUE}📁 Adding modified files...${NC}"

# Add the key files for CI improvements
git add Makefile
git add .github/workflows/local-ci.yml
git add .github/workflows/security.yml
git add .golangci.yml

# Add the PR description file if it exists
if [ -f "pr_description_ci_improvements.md" ]; then
    git add pr_description_ci_improvements.md
fi

# Add this script
git add scripts/create_ci_pr.sh

# Show what will be committed
echo -e "${BLUE}📋 Files to be committed:${NC}"
git diff --staged --name-only

# Commit the changes
echo -e "${BLUE}💾 Committing changes...${NC}"
git commit -m "${COMMIT_MESSAGE}"

# Push the branch
echo -e "${BLUE}⬆️  Pushing branch to origin...${NC}"
git push -u origin "${BRANCH_NAME}"

echo ""
echo -e "${GREEN}✅ Branch created and pushed successfully!${NC}"
echo "=================================================="
echo -e "${BLUE}🔗 Next Steps:${NC}"
echo ""
echo "1. Create a Pull Request at:"
echo "   https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^/]*\/[^.]*\).*/\1/')/compare/${BRANCH_NAME}"
echo ""
echo "2. Use the PR description from:"
echo "   pr_description_ci_improvements.md"
echo ""
echo "3. Test the changes locally:"
echo "   make ci-quick"
echo "   make ci-local"
echo ""
echo "4. Review the following areas:"
echo "   - Makefile targets functionality"
echo "   - Local CI workflow execution"
echo "   - Security scanning improvements"
echo "   - Docker cleanup effectiveness"
echo ""
echo -e "${GREEN}🎉 CI Pipeline Improvements PR is ready!${NC}"
