#!/bin/bash

# Run Custom Logging Test with Environment Variables
# This script loads environment variables and runs the test

set -e

echo "🚀 Custom Logging Test with Environment Variables"
echo "================================================"

# Check if we're in the right directory
if [ ! -f "agent_logging.go" ]; then
    echo "❌ Error: agent_logging.go not found in current directory"
    exit 1
fi

# Load environment variables from agent_go directory
if [ -f "../../agent_go/.env" ]; then
    echo "✅ Loading environment variables from ../../agent_go/.env"
    export $(grep -v '^#' ../../agent_go/.env | xargs)
    echo "✅ Environment variables loaded"
else
    echo "❌ Error: .env file not found in ../../agent_go/"
    exit 1
fi

# Verify key environment variables
echo "🔍 Checking environment variables:"
if [ -n "$OPENAI_API_KEY" ]; then
    echo "✅ OPENAI_API_KEY is set"
else
    echo "❌ OPENAI_API_KEY is not set"
fi

if [ -n "$BEDROCK_PRIMARY_MODEL" ]; then
    echo "✅ BEDROCK_PRIMARY_MODEL is set: $BEDROCK_PRIMARY_MODEL"
else
    echo "❌ BEDROCK_PRIMARY_MODEL is not set"
fi

echo ""
echo "🔧 Running custom logging test..."
echo ""

# Run the test
go run agent_logging.go

echo ""
echo "🎉 Custom logging test completed!"
echo "📁 Check my_custom_logs.log for detailed output"
echo "🔍 Look for '[MY-AGENT]' prefix on ALL logs to verify custom logger is working"
echo "✅ Console should be completely clean (no output)"
