#!/bin/bash

# Clean Repository Script - Remove All Git History
# This script creates a fresh repository with no commit history

set -e

echo "🧹 Cleaning repository - removing all git history..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Backup current remote URL
REMOTE_URL=$(git remote get-url origin)
echo -e "${BLUE}📡 Current remote: $REMOTE_URL${NC}"

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo -e "${RED}❌ Error: Not in a git repository${NC}"
    exit 1
fi

# Check for uncommitted changes
if ! git diff-index --quiet HEAD --; then
    echo -e "${YELLOW}⚠️  Warning: You have uncommitted changes${NC}"
    echo "Please commit or stash your changes before running this script."
    echo ""
    echo "To commit changes:"
    echo "  git add ."
    echo "  git commit -m 'Final commit before cleanup'"
    echo ""
    echo "To stash changes:"
    echo "  git stash"
    echo ""
    read -p "Do you want to continue anyway? (y/N): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Aborted."
        exit 1
    fi
fi

echo -e "${YELLOW}⚠️  This will permanently delete all git history!${NC}"
echo "Current branch: $(git branch --show-current)"
echo "Total commits: $(git rev-list --count HEAD)"
echo ""
read -p "Are you sure you want to proceed? (y/N): " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
fi

echo -e "${BLUE}🔄 Creating fresh repository...${NC}"

# Remove all git history
rm -rf .git

# Initialize new git repository
git init

# Add all files
git add .

# Create initial commit
git commit -m "Initial commit - clean repository without secrets

- Removed all previous git history
- Added gitleaks protection
- Clean state with no exposed secrets"

# Add remote
git remote add origin "$REMOTE_URL"

# Create and switch to main branch
git branch -M main

echo -e "${GREEN}✅ Fresh repository created successfully!${NC}"
echo ""
echo -e "${BLUE}Next steps:${NC}"
echo "1. Push the clean repository:"
echo "   git push -f origin main"
echo ""
echo "2. Verify the clean state:"
echo "   ./scripts/scan-secrets.sh"
echo ""
echo -e "${YELLOW}⚠️  Important:${NC}"
echo "- This will overwrite the remote repository"
echo "- All previous commits will be lost"
echo "- Make sure you have backups if needed"
echo ""
echo -e "${GREEN}🎉 Your repository is now clean and protected!${NC}"
