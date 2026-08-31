#!/bin/bash

# Install Git Hooks for Gitleaks Secret Scanning
# This script sets up pre-commit hooks to automatically scan for secrets and run linting

set -e

echo "🔒 Setting up pre-commit hooks (Gitleaks secret scanning)..."

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo -e "${RED}❌ Error: Not in a git repository${NC}"
    echo "Please run this script from the root of your git repository."
    exit 1
fi

# Check if gitleaks is installed
if ! command -v gitleaks &> /dev/null; then
    echo -e "${YELLOW}⚠️  Gitleaks not found. Installing...${NC}"
    
    # Detect OS and install gitleaks
    if [[ "$OSTYPE" == "darwin"* ]]; then
        # macOS
        if command -v brew &> /dev/null; then
            echo "Installing gitleaks via Homebrew..."
            brew install gitleaks
        else
            echo -e "${RED}❌ Homebrew not found. Please install gitleaks manually:${NC}"
            echo "Visit: https://github.com/gitleaks/gitleaks#installation"
            exit 1
        fi
    elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
        # Linux
        echo "Installing gitleaks via curl..."
        curl -sSfL https://github.com/gitleaks/gitleaks/releases/latest/download/gitleaks_8.18.0_linux_x64.tar.gz | tar -xz -C /tmp
        sudo mv /tmp/gitleaks /usr/local/bin/
    else
        echo -e "${RED}❌ Unsupported OS. Please install gitleaks manually:${NC}"
        echo "Visit: https://github.com/gitleaks/gitleaks#installation"
        exit 1
    fi
fi

# Verify gitleaks installation
if ! command -v gitleaks &> /dev/null; then
    echo -e "${RED}❌ Failed to install gitleaks${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Gitleaks installed successfully${NC}"

# Create scripts directory if it doesn't exist
mkdir -p scripts

# Copy the current pre-commit hook from the repo
# The hook source of truth is maintained in .git/hooks/pre-commit
# but we also keep a tracked copy for new clones
cat > .git/hooks/pre-commit << 'HOOKEOF'
#!/bin/bash

# Pre-commit Hook
# Scans staged files for secrets and sensitive data

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🔒 Scanning for secrets with gitleaks...${NC}"

# Check if gitleaks is available
if ! command -v gitleaks &> /dev/null; then
    echo -e "${YELLOW}⚠️  Gitleaks not found. Skipping secret scan.${NC}"
    echo "Run './scripts/install-git-hooks.sh' to install gitleaks."
    exit 0
fi

# Run gitleaks on staged files
if gitleaks protect --staged --config .gitleaks.toml --verbose; then
    echo -e "${GREEN}✅ No secrets detected.${NC}"
else
    echo -e "${RED}❌ Secrets detected! Commit blocked.${NC}"
    echo ""
    echo "Please remove or replace the detected secrets before committing."
    echo "Common solutions:"
    echo "  • Use environment variables instead of hardcoded secrets"
    echo "  • Move secrets to .env files (not tracked by git)"
    echo "  • Use placeholder values in example files"
    echo ""
    echo "For more information, see agent_go/SECURITY.md"
    exit 1
fi

# Check for sensitive file patterns (bank statements, personal data, screenshots)
echo -e "${BLUE}🔍 Checking for sensitive file patterns...${NC}"
SENSITIVE_PATTERNS=(
    '**/Downloads/**'
    '**/Acct_Statement*'
    '**/DetailedStatement*'
    '**/Statement_*.txt'
    '**/Statement_*.xlsx'
    '**/Statement_*.xls'
    '**/statement_*.txt'
    '**/OpTransactionHistory*'
    '**/*login*.png'
    '**/*dashboard*.png'
    '**/*screenshot*.png'
    '**/*snapshot*.png'
    '*.psv'
)
SENSITIVE_FILES=""
for pattern in "${SENSITIVE_PATTERNS[@]}"; do
    MATCHES=$(git diff --cached --diff-filter=ACMR --name-only -- "$pattern" 2>/dev/null || true)
    if [ -n "$MATCHES" ]; then
        SENSITIVE_FILES="$SENSITIVE_FILES$MATCHES"$'\n'
    fi
done
SENSITIVE_FILES=$(echo "$SENSITIVE_FILES" | sed '/^$/d')
if [ -n "$SENSITIVE_FILES" ]; then
    echo -e "${RED}❌ Sensitive files detected! Commit blocked.${NC}"
    echo ""
    echo "The following files appear to contain personal/financial data:"
    echo "$SENSITIVE_FILES" | head -20
    echo ""
    echo "These files should NOT be committed to the repository."
    echo "Remove them with: git reset HEAD <file>"
    exit 1
fi
echo -e "${GREEN}✅ No sensitive file patterns detected.${NC}"

# Schema drift — runs only when staged files plausibly affect generated
# schemas/types. See scripts/check-schema-drift.sh for the exact paths.
REPO_ROOT_FOR_DRIFT="$(git rev-parse --show-toplevel)"
if [ -x "$REPO_ROOT_FOR_DRIFT/scripts/check-schema-drift.sh" ]; then
    if ! "$REPO_ROOT_FOR_DRIFT/scripts/check-schema-drift.sh"; then
        exit 1
    fi
fi

# golangci-lint + go build (agent_go/workspace) + frontend build were removed
# from this hook — CI (desktop-release.yml's commit-artifact job) already
# rebuilds everything on every push to main, so re-running the full lint/
# build locally on every commit was pure duplicated latency, not an extra
# safety net. Run them yourself before committing if you want the earlier
# signal (`cd agent_go && golangci-lint run ./... && go build ./...`,
# `cd frontend && npm run build`).
echo ""
echo -e "${GREEN}✅ All pre-commit checks passed. Proceeding with commit.${NC}"
exit 0
HOOKEOF

# Make the pre-commit hook executable
chmod +x .git/hooks/pre-commit

# Create a manual scan script
cat > scripts/scan-secrets.sh << 'EOF'
#!/bin/bash

# Manual Secret Scanning Script
# Run this to scan for secrets in your repository

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🔒 Scanning repository for secrets...${NC}"

# Check if gitleaks is available
if ! command -v gitleaks &> /dev/null; then
    echo -e "${RED}❌ Gitleaks not found. Please install it first:${NC}"
    echo "Run './scripts/install-git-hooks.sh' to install gitleaks."
    exit 1
fi

# Default scan path
SCAN_PATH="${1:-.}"

echo "Scanning path: $SCAN_PATH"
echo ""

# Run gitleaks scan
if gitleaks detect --source "$SCAN_PATH" --config .gitleaks.toml --verbose --report-format json --report-path gitleaks-report.json; then
    echo -e "${GREEN}✅ No secrets detected in $SCAN_PATH${NC}"
    rm -f gitleaks-report.json
else
    echo -e "${RED}❌ Secrets detected in $SCAN_PATH${NC}"
    echo ""
    echo "Report saved to: gitleaks-report.json"
    echo ""
    echo "Please review and remove the detected secrets:"
    echo "  • Use environment variables instead of hardcoded secrets"
    echo "  • Move secrets to .env files (not tracked by git)"
    echo "  • Use placeholder values in example files"
    echo ""
    echo "For more information, see agent_go/SECURITY.md"
    exit 1
fi
EOF

# Make the scan script executable
chmod +x scripts/scan-secrets.sh

# Test the installation
echo -e "${BLUE}🧪 Testing gitleaks installation...${NC}"
if gitleaks version &> /dev/null; then
    echo -e "${GREEN}✅ Gitleaks is working correctly${NC}"
else
    echo -e "${RED}❌ Gitleaks test failed${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}🎉 Pre-commit hooks installed successfully!${NC}"
echo ""
echo -e "${BLUE}What happens now:${NC}"
echo "  • Every commit will be automatically scanned for secrets (gitleaks) and sensitive file patterns"
echo "  • Commits with secrets or sensitive files will be blocked"
echo "  • You'll get clear error messages if issues are detected"
echo "  • golangci-lint / go build / frontend build are NOT run on commit anymore — CI (desktop-release.yml)"
echo "    already rebuilds everything on every push to main, so this hook stays fast. Run them yourself"
echo "    before committing if you want that signal early: 'cd agent_go && golangci-lint run ./... && go build ./...',"
echo "    'cd frontend && npm run build'"
echo ""
echo -e "${BLUE}Manual scanning:${NC}"
echo "  • Run './scripts/scan-secrets.sh' to scan the entire repository"
echo "  • Run './scripts/scan-secrets.sh path/to/file' to scan specific files"
echo "  • Run 'cd agent_go && make lint' to run golangci-lint manually"
echo ""
echo -e "${BLUE}Configuration:${NC}"
echo "  • Edit '.gitleaks.toml' to customize secret detection rules"
echo "  • Edit 'agent_go/.golangci.yml' to customize linting rules"
echo "  • See 'agent_go/SECURITY.md' for security best practices"
echo ""
echo -e "${GREEN}Your repository is now protected against accidental secret commits! 🔒${NC}"
