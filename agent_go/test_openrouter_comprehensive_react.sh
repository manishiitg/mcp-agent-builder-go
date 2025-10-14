#!/bin/bash

# Test OpenRouter Provider with Comprehensive ReAct Test
# Tests the ReAct agent with OpenRouter for complex reasoning tasks

set -e

# Source .env file if it exists
if [ -f ".env" ]; then
    echo "📁 Loading environment variables from .env file..."
    export $(cat .env | grep -v '^#' | xargs)
    echo "✅ Environment variables loaded"
fi

echo "🚀 Testing OpenRouter Provider with Comprehensive ReAct Test"
echo "============================================================"

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
    echo ""
    echo "🔗 Get your API key from: https://openrouter.ai/keys"
    exit 1
fi

# Create logs directory
mkdir -p logs

# Generate log filename with timestamp
LOG_FILE="logs/comprehensive-react-openrouter-$(date +%Y%m%d-%H%M%S).log"

echo "🧪 Running Comprehensive ReAct Test with OpenRouter"
echo "--------------------------------------------------"
echo "📝 Test: Complex reasoning with AWS and Scripts tools"
echo "📁 Log file: ${LOG_FILE}"
echo "🔧 Using: OpenRouter provider with ReAct agent mode"
echo ""

# Run the comprehensive ReAct test
echo "🚀 Starting test..."
go run main.go test comprehensive-react --provider openrouter --log-file "${LOG_FILE}" --verbose

echo ""
echo "✅ Test completed! Check log file: ${LOG_FILE}"
echo "🔍 To view traces: go run main.go test langfuse get --filter 'openrouter'"
