#!/bin/bash

# Agent Events Test Runner
# This script runs the focused events test

set -e

echo "🚀 AGENT EVENTS TEST RUNNER"
echo "============================"

# Check if we're in the right directory
if [ ! -f "agent_events.go" ]; then
    echo "❌ Error: agent_events.go not found in current directory"
    echo "💡 Please run this script from external_example/agent_with_events/"
    exit 1
fi

# Check if Go is available
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed or not in PATH"
    exit 1
fi

echo "✅ Go found: $(go version)"
echo "✅ Events test file found: agent_events.go"

# Check for .env file
if [ -f "../../.env" ]; then
    echo "✅ .env file found in project root"
elif [ -f ".env" ]; then
    echo "✅ .env file found in current directory"
else
    echo "⚠️  Warning: No .env file found"
    echo "💡 The test will use system environment variables"
fi

echo ""
echo "🔧 Building and running events test..."
echo ""

# Build and run the test
go run agent_events.go

echo ""
echo "🎉 Events test completed!"
echo "📁 Check the console output above for event capture and analysis"
