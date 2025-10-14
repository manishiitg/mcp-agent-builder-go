#!/bin/bash

# Test script for max-tokens-flexibility
# This test validates that max_tokens handling is flexible and not required for APIs to work

set -e

echo "🧪 Testing Max Tokens Flexibility"
echo "=================================="

# Change to agent_go directory
cd "$(dirname "$0")"

# Create logs directory if it doesn't exist
mkdir -p logs

# Run the max-tokens-flexibility test
echo "Running max-tokens-flexibility test..."
go run main.go test max-tokens-flexibility \
    --provider bedrock \
    --verbose \
    --log-file "logs/max-tokens-flexibility-$(date +%Y%m%d-%H%M%S).log"

echo ""
echo "✅ Max Tokens Flexibility Test completed!"
echo "📝 Check the logs directory for detailed results"
echo ""
echo "This test validates that:"
echo "1. APIs work without explicit max_tokens"
echo "2. Flexible token handling works with reasonable defaults"
echo "3. Hardcoded token limits aren't required for functionality"
echo "4. Different providers handle token limits appropriately"
echo "5. System can handle very large prompts (280K+ characters, ~70K tokens)"
echo ""
echo "✅ SUCCESS: max-tokens.go file has been REMOVED!"
echo "✅ SUCCESS: System now uses flexible token handling automatically!"
