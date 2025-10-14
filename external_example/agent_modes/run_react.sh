#!/bin/bash

# ReAct Agent Mode Test Runner
# This script runs the ReAct agent demo

set -e

echo "🚀 REACT AGENT MODE TEST RUNNER"
echo "================================"

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
echo "🔧 Building and running ReAct agent demo..."
echo ""

# Build and run the ReAct agent demo
go run agent_modes.go react

echo ""
echo "🎉 ReAct agent demo completed!"
echo "📁 Check the console output above for ReAct agent behavior"
echo "💡 ReAct agent should: Think step-by-step, detailed reasoning, more turns"
