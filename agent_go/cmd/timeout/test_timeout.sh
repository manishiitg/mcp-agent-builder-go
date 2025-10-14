#!/bin/bash

# Test script for the MCP Timeout Server
# This script demonstrates the timeout server functionality

echo "🧪 Testing MCP Timeout Server"
echo "=============================="

# Check if we're in the right directory
if [ ! -f "timeout.go" ]; then
    echo "❌ Please run this script from the timeout folder"
    exit 1
fi

echo "✅ Running from timeout folder"

# Test 1: Help command
echo -e "\n📋 Test 1: Help command"
go run timeout-bin/main.go --help

# Test 2: Start SSE server in background (using port 7088 to avoid conflicts)
echo -e "\n📋 Test 2: Starting SSE server on port 7088"
go run timeout-bin/main.go --transport sse --port 7088 &
TIMEOUT_PID=$!

# Wait a moment for server to start
sleep 2

# Check if server is running
if ps -p $TIMEOUT_PID > /dev/null; then
    echo "✅ SSE server started successfully (PID: $TIMEOUT_PID)"
    
    # Test 3: Check if server is listening
    echo -e "\n📋 Test 3: Checking if server is listening on port 7088"
    if lsof -i :7088 > /dev/null 2>&1; then
        echo "✅ Server is listening on port 7088"
    else
        echo "❌ Server is not listening on port 7088"
    fi
    
    # Test 4: Test SSE endpoint
    echo -e "\n📋 Test 4: Testing SSE endpoint"
    curl -s -N "http://localhost:7088/sse" &
    CURL_PID=$!
    sleep 3
    kill $CURL_PID 2>/dev/null
    echo "✅ SSE endpoint test completed"
    
    # Stop the server
    echo -e "\n🛑 Stopping SSE server..."
    kill $TIMEOUT_PID
    wait $TIMEOUT_PID 2>/dev/null
    echo "✅ SSE server stopped"
else
    echo "❌ Failed to start SSE server"
fi

echo -e "\n🎉 Timeout server testing completed!"
echo -e "\n💡 Usage examples:"
echo "  go run timeout-bin/main.go                    # Start with stdio transport"
echo "  go run timeout-bin/main.go --transport sse   # Start with SSE transport"
echo "  go run timeout-bin/main.go --transport sse --port 7088  # Custom port"
