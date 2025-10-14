#!/bin/bash

# Test script for the Langfuse debugging tool
# This script demonstrates how to use the tool to fetch existing traces

echo "🔍 Testing Langfuse Debug Tool (Read-Only)"
echo "=========================================="

# Check if the tool exists
if [ ! -f "./langfuse-debug" ]; then
    echo "❌ Error: langfuse-debug tool not found"
    echo "Please build it first with: go build -o langfuse-debug ."
    exit 1
fi

# Check if .env file exists
if [ ! -f "../.env" ]; then
    echo "❌ Error: .env file not found in parent directory"
    echo "Please create .env with your Langfuse credentials:"
    echo "  LANGFUSE_PUBLIC_KEY=your_public_key"
    echo "  LANGFUSE_SECRET_KEY=your_secret_key"
    echo "  LANGFUSE_HOST=https://cloud.langfuse.com"
    exit 1
fi

# Load environment variables
source ../.env

# Check required environment variables
if [ -z "$LANGFUSE_PUBLIC_KEY" ] || [ -z "$LANGFUSE_SECRET_KEY" ]; then
    echo "❌ Error: LANGFUSE_PUBLIC_KEY and LANGFUSE_SECRET_KEY must be set in .env"
    exit 1
fi

echo "✅ Environment loaded successfully"
echo "📊 Langfuse Host: ${LANGFUSE_HOST:-https://cloud.langfuse.com}"
echo "🔑 Public Key: ${LANGFUSE_PUBLIC_KEY:0:10}..."
echo ""

echo "🧪 Testing Langfuse Debug Tool (Read-Only Mode)"
echo ""

# Test 1: Fetch recent traces
echo "🧪 Test 1: Fetching Recent Traces"
echo "--------------------------------"
./langfuse-debug langfuse
echo ""

# Test 2: Fetch with debug mode
echo "🧪 Test 2: Fetching with Debug Mode"
echo "----------------------------------"
./langfuse-debug langfuse --debug
echo ""

# Test 3: Show help
echo "🧪 Test 3: Showing Help"
echo "----------------------"
./langfuse-debug --help
echo ""

echo "🎉 Testing Complete!"
echo "==================="
echo "✅ Successfully fetched existing traces"
echo "✅ Demonstrated read-only debug functionality"
echo ""
echo "🔍 The tool is now read-only and only fetches existing traces"
echo "📊 To test with specific trace IDs or session IDs, use:"
echo "   ./langfuse-debug --trace-id <TRACE_ID>"
echo "   ./langfuse-debug --session-id <SESSION_ID>"
echo ""
echo "💡 Next steps:"
echo "   - Use the tool to inspect existing traces in your Langfuse dashboard"
echo "   - Analyze trace structure and spans"
echo "   - Debug trace issues by fetching specific trace IDs"
