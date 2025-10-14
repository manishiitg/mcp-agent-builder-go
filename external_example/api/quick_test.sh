#!/bin/bash

echo "🚀 Quick API Test"
echo "================="

# Truncate log file before starting test
echo "🧹 Truncating log file..."
if [ -f "logs/mcp-agent.log" ]; then
    > logs/mcp-agent.log
    echo "✅ Log file truncated"
else
    echo "ℹ️  Log file not found, will be created"
fi

# Function to cleanup server
cleanup_server() {
    echo ""
    echo "🧹 Cleaning up server..."
    
    # Kill server by port
    if lsof -ti:8080 > /dev/null 2>&1; then
        echo "🔌 Killing server on port 8080..."
        lsof -ti:8080 | xargs kill -9
        sleep 1
        echo "✅ Server killed"
    else
        echo "ℹ️  No server found running on port 8080"
    fi
}

# Set trap to cleanup on script exit
trap cleanup_server EXIT

# Check if server is running
if ! curl -s http://localhost:8080/api/stats > /dev/null 2>&1; then
    echo "❌ Server not running. Start with: go run ."
    exit 1
fi

echo "✅ Server running"
echo "🧪 Testing simple query..."

# Test simple query
curl -s -X POST http://localhost:8080/api/query \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Hello! How are you today?",
    "conversation_id": "quick_test_001"
  }' | jq '.'

echo ""
echo "✅ Test completed!"
echo "🔄 Server will be automatically killed when script exits"
