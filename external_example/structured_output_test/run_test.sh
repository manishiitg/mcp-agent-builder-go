#!/bin/bash

# External Agent Structured Output Test Runner
# This script runs the structured output test for the external agent package

set -e

echo "🚀 Starting External Agent Structured Output Test"
echo "=================================================="

# Check if we're in the right directory
if [ ! -f "main.go" ]; then
    echo "❌ Error: main.go not found. Please run this script from the structured_output_test directory."
    exit 1
fi

# Check if go.mod exists
if [ ! -f "go.mod" ]; then
    echo "❌ Error: go.mod not found. Please run this script from the structured_output_test directory."
    exit 1
fi

# Set up environment variables
export TRACING_PROVIDER=console
export TOOL_OUTPUT_FOLDER=./logs
export TOOL_OUTPUT_THRESHOLD=1000

# Create logs directory if it doesn't exist
mkdir -p logs

echo "📋 Environment Setup:"
echo "  - TRACING_PROVIDER: $TRACING_PROVIDER"
echo "  - TOOL_OUTPUT_FOLDER: $TOOL_OUTPUT_FOLDER"
echo "  - TOOL_OUTPUT_THRESHOLD: $TOOL_OUTPUT_THRESHOLD"

# Check if OpenAI API key is set
if [ -z "$OPENAI_API_KEY" ]; then
    echo "⚠️  Warning: OPENAI_API_KEY not set. The test will fail if OpenAI is used."
    echo "   You can set it with: export OPENAI_API_KEY=your_key_here"
fi

# Check if AWS credentials are set
if [ -z "$AWS_ACCESS_KEY_ID" ] || [ -z "$AWS_SECRET_ACCESS_KEY" ]; then
    echo "⚠️  Warning: AWS credentials not set. The test will fail if AWS Bedrock is used."
    echo "   You can set them with:"
    echo "     export AWS_ACCESS_KEY_ID=your_key_here"
    echo "     export AWS_SECRET_ACCESS_KEY=your_secret_here"
    echo "     export AWS_REGION=us-east-1"
fi

echo ""
echo "🔧 Building and running test..."
echo ""

# Run go mod tidy to ensure dependencies are correct
echo "📦 Running go mod tidy..."
go mod tidy

# Build the test
echo "🔨 Building test binary..."
go build -o structured_output_test main.go

# Run the test
echo "🧪 Running structured output test..."
echo ""

./structured_output_test

echo ""
echo "✅ Test completed!"
echo ""
echo "📁 Logs are available in: $TOOL_OUTPUT_FOLDER"
echo "🗑️  Cleanup: rm structured_output_test"
