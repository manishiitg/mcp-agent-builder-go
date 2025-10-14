#!/bin/bash

# External Logging Test Runner
# This script runs the focused logger test

set -e

echo "🚀 EXTERNAL LOGGING TEST RUNNER"
echo "================================"

# Check if we're in the right directory
if [ ! -f "logger_test.go" ]; then
    echo "❌ Error: logger_test.go not found in current directory"
    echo "💡 Please run this script from agent_go/examples/external_logging/"
    exit 1
fi

# Check if Go is available
if ! command -v go &> /dev/null; then
    echo "❌ Error: Go is not installed or not in PATH"
    exit 1
fi

echo "✅ Go found: $(go version)"
echo "✅ Logger test file found: logger_test.go"

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
echo "🔧 Building and running logger test..."
echo ""

# Build and run the test
go run logger_test.go

echo ""
echo "🎉 Logger test completed!"
echo "📁 Check logger_test.log for detailed output"
echo "🔍 Review the console output above for test results"
