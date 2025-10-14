#!/bin/bash

# Test OpenRouter Provider with Simple Agent
# Quick test script for OpenRouter integration

set -e

# Source .env file if it exists
if [ -f ".env" ]; then
    echo "📁 Loading environment variables from .env file..."
    export $(cat .env | grep -v '^#' | xargs)
    echo "✅ Environment variables loaded"
fi

echo "🚀 Testing OpenRouter Provider with Simple Agent"
echo "================================================"

# Check if we're in the right directory
if [ ! -f "main.go" ]; then
    echo "❌ Error: Please run this script from the agent_go directory"
    echo "   Current directory: $(pwd)"
    echo "   Expected: agent_go/"
    exit 1
fi

# Check OpenRouter API key
if [ -z "$OPEN_ROUTER_API_KEY" ]; then
    echo "❌ Error: OPEN_ROUTER_API_KEY environment variable is not set"
    echo ""
    echo "💡 To set it:"
    echo "   export OPEN_ROUTER_API_KEY=your_openrouter_api_key"
    echo "   or add it to your .env file"
    echo ""
    echo "🔗 Get your API key from: https://openrouter.ai/keys"
    exit 1
fi

echo "✅ OpenRouter API key found"

# Create logs directory if it doesn't exist
mkdir -p logs

# Get current timestamp for log file
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
LOG_FILE="logs/simple-test-openrouter-${TIMESTAMP}.log"

echo ""
echo "🧪 Running Simple Agent Test with OpenRouter"
echo "---------------------------------------------"
echo "📝 Test: Simple file listing and project structure analysis"
echo "📁 Log file: ${LOG_FILE}"
echo "🔧 Using: mcp_servers_simple.json (local filesystem & memory servers)"
echo ""

# Run the simple agent test
echo "🚀 Starting test..."
go run main.go test agent --simple --provider openrouter --config configs/mcp_servers_simple.json --log-file "${LOG_FILE}"

echo ""
echo "✅ Test completed successfully!"
echo ""
echo "📊 Test Results:"
echo "================="
if [ -f "${LOG_FILE}" ]; then
    echo "📁 Log file: ${LOG_FILE}"
    echo ""
    echo "🔍 Key metrics:"
    grep -i "response_length" "${LOG_FILE}" || echo "   No response length data found"
    grep -i "duration" "${LOG_FILE}" || echo "   No duration data found"
    echo ""
    echo "📋 Full log:"
    echo "   tail -f ${LOG_FILE}"
    echo ""
    echo "🔍 View traces in Langfuse:"
    echo "   go run main.go langfuse traces --filter 'simple' --limit 5"
else
    echo "❌ Log file not found: ${LOG_FILE}"
fi

echo ""
echo "🎯 Next steps:"
echo "   - Test other providers: ./test_all_providers_simple.sh"
echo "   - Run comprehensive tests: go run main.go test comprehensive-react --provider openrouter"
echo "   - Test token usage: go run main.go test token-usage"
