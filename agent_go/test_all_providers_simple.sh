#!/bin/bash

# Test Simple Agent with All 4 Providers
# This script runs the simple agent test with Bedrock, OpenAI, Anthropic, and OpenRouter

set -e

echo "🚀 Testing Simple Agent with All 4 Providers"
echo "=============================================="

# Create logs directory if it doesn't exist
mkdir -p logs

# Get current timestamp for log files
TIMESTAMP=$(date +%Y%m%d-%H%M%S)

echo ""
echo "1️⃣ Testing with Bedrock Provider (AWS Claude)"
echo "----------------------------------------------"
if [ -n "$AWS_REGION" ] && [ -n "$AWS_ACCESS_KEY_ID" ] && [ -n "$AWS_SECRET_ACCESS_KEY" ]; then
    echo "✅ AWS credentials found, running Bedrock test..."
    go run main.go test agent --simple --provider bedrock --config configs/mcp_servers_simple.json --log-file "logs/simple-test-bedrock-${TIMESTAMP}.log"
    echo "✅ Bedrock test completed"
else
    echo "⚠️  AWS credentials not found, skipping Bedrock test"
fi

echo ""
echo "2️⃣ Testing with OpenAI Provider (GPT models)"
echo "---------------------------------------------"
if [ -n "$OPENAI_API_KEY" ]; then
    echo "✅ OpenAI API key found, running OpenAI test..."
    go run main.go test agent --simple --provider openai --config configs/mcp_servers_simple.json --log-file "logs/simple-test-openai-${TIMESTAMP}.log"
    echo "✅ OpenAI test completed"
else
    echo "⚠️  OpenAI API key not found, skipping OpenAI test"
fi

echo ""
echo "3️⃣ Testing with Anthropic Provider (Claude models)"
echo "--------------------------------------------------"
if [ -n "$ANTHROPIC_API_KEY" ]; then
    echo "✅ Anthropic API key found, running Anthropic test..."
    go run main.go test agent --simple --provider anthropic --config configs/mcp_servers_simple.json --log-file "logs/simple-test-anthropic-${TIMESTAMP}.log"
    echo "✅ Anthropic test completed"
else
    echo "⚠️  Anthropic API key not found, skipping Anthropic test"
fi

echo ""
echo "4️⃣ Testing with OpenRouter Provider (Multiple models)"
echo "-----------------------------------------------------"
if [ -n "$OPEN_ROUTER_API_KEY" ]; then
    echo "✅ OpenRouter API key found, running OpenRouter test..."
    go run main.go test agent --simple --provider openrouter --config configs/mcp_servers_simple.json --log-file "logs/simple-test-openrouter-${TIMESTAMP}.log"
    echo "✅ OpenRouter test completed"
else
    echo "⚠️  OpenRouter API key not found, skipping OpenRouter test"
fi

echo ""
echo "🎉 All Provider Tests Completed!"
echo "================================"
echo "📁 Log files saved in logs/ directory"
echo "🔍 Check individual log files for detailed results"
echo ""
echo "💡 Additional Testing Options:"
echo "   # Comprehensive ReAct tests (complex reasoning)"
echo "   go run main.go test comprehensive-react --provider openrouter"
echo "   go run main.go test comprehensive-react --provider bedrock"
echo "   go run main.go test comprehensive-react --provider openai"
echo "   go run main.go test comprehensive-react --provider anthropic"
echo ""
echo "   # Or use the quick script:"
echo "   ./test_openrouter_comprehensive_react.sh"
