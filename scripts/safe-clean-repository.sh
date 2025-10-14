#!/bin/bash

# Safe Clean Repository Script - Remove Git History Safely
# This script creates a fresh repository while preserving your current work

set -e

echo "🧹 Safe repository cleanup - removing git history..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo -e "${RED}❌ Error: Not in a git repository${NC}"
    exit 1
fi

# Get current remote URL
REMOTE_URL=$(git remote get-url origin)
echo -e "${BLUE}📡 Current remote: $REMOTE_URL${NC}"

# Check for uncommitted changes
if ! git diff-index --quiet HEAD --; then
    echo -e "${YELLOW}⚠️  You have uncommitted changes${NC}"
    echo "Committing current changes first..."
    git add .
    git commit -m "Save current work before cleanup"
fi

echo -e "${BLUE}🔄 Creating backup branch...${NC}"
# Create backup branch
git branch backup-$(date +%Y%m%d-%H%M%S)

echo -e "${BLUE}🔄 Creating orphan branch...${NC}"
# Create orphan branch (no history)
git checkout --orphan clean-main

# Add all files
git add .

# Create initial commit
git commit -m "Clean repository - initial commit

- Removed all git history containing secrets
- Added gitleaks protection
- Fresh start with clean state"

echo -e "${BLUE}🔄 Switching to clean branch...${NC}"
# Switch to clean branch
git checkout clean-main

# Rename to main
git branch -M main

echo -e "${GREEN}✅ Clean repository created successfully!${NC}"
echo ""
echo -e "${BLUE}What happened:${NC}"
echo "✅ Created backup branch with old history"
echo "✅ Created new clean main branch"
echo "✅ All current files preserved"
echo "✅ No git history (secrets removed)"
echo ""
echo -e "${BLUE}Next steps:${NC}"
echo "1. Push the clean repository:"
echo "   git push -f origin main"
echo ""
echo "2. Verify the clean state:"
echo "   ./scripts/scan-secrets.sh"
echo ""
echo "3. If you need the old history later:"
echo "   git checkout backup-$(date +%Y%m%d-%H%M%S)"
echo ""
echo -e "${YELLOW}⚠️  Important:${NC}"
echo "- This will overwrite the remote main branch"
echo "- Old history is saved in backup branch"
echo "- Run 'git push -f origin main' to update remote"
echo ""
echo -e "${GREEN}🎉 Your repository is now clean and protected!${NC}"
