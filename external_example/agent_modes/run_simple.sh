#!/bin/bash

# Simple Agent Mode Test Runner
# This script runs the Simple agent demo

set -e

echo "🚀 SIMPLE AGENT MODE TEST RUNNER"
echo "================================="

# Check if we're in the right directory
if [ ! -f "agent_modes.go" ]; then
    echo "❌ Error: agent_modes.go not found in current directory"
    echo "💡 Please run this script from external_example/agent_modes/"
    exit 1
fi

# Check if Go is available
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed or not in PATH"
    exit 1
fi

echo "✅ Go found: $(go version)"
echo "✅ Agent modes file found: agent_modes.go"

# Check for .env file
if [ -f "../../agent_go/.env" ]; then
    echo "✅ .env file found in agent_go directory"
elif [ -f ".env" ]; then
    echo "✅ .env file found in current directory"
else
    echo "⚠️  Warning: No .env file found"
    echo "💡 The test will use system environment variables"
fi

echo ""
echo "🔧 Building and running Simple agent demo..."
echo ""

# Build and run the Simple agent demo
go run agent_modes.go simple

echo ""
echo "🎉 Simple agent demo completed!"
echo "📁 Check the console output above for Simple agent behavior"
echo "💡 Simple agent should: Use tools directly, faster response, fewer turns"
