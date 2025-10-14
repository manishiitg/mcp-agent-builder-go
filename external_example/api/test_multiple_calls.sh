#!/bin/bash

echo "🧪 Testing Multiple API Calls with History and Event Capture"
echo "============================================================"
echo "🎯 This script tests:"
echo "   • Parallel API calls (race condition testing)"
echo "   • Event listener isolation between requests"
echo "   • Conversation history handling under load"
echo "   • Server stability with concurrent requests"
echo "   • Event capture accuracy during parallel execution"
echo ""

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
    
    # Kill server by PID if we have it
    if [ ! -z "$SERVER_PID" ]; then
        echo "🔌 Killing server with PID: $SERVER_PID"
        kill $SERVER_PID 2>/dev/null || true
        sleep 1
    fi
    
    # Also kill by port as backup
    if lsof -ti:8081 > /dev/null 2>&1; then
        echo "🔌 Killing server on port 8081..."
        lsof -ti:8081 | xargs kill -9
        sleep 1
        echo "✅ Server killed"
    else
        echo "ℹ️  No server found running on port 8081"
    fi
}

# Set trap to cleanup on script exit
trap cleanup_server EXIT

# Check if we have the required environment variables
if [ -z "$OPENAI_API_KEY" ]; then
    echo "❌ OPENAI_API_KEY environment variable is required"
    echo "Please set it and try again:"
    echo "export OPENAI_API_KEY=your_api_key_here"
    exit 1
fi

echo "✅ OpenAI API key found"

# Kill any existing servers on port 8081
echo "🔪 Killing any existing servers on port 8081..."
pkill -f "go run .*8081" 2>/dev/null || true
sleep 2

# Start the server in background
echo "🚀 Starting API server in background on port 8081..."
cd "$(dirname "$0")"  # Ensure we're in the right directory
go run . 8081 > server.log 2>&1 &
SERVER_PID=$!

echo "📝 Server started with PID: $SERVER_PID"
echo "📋 Server logs: server.log"

# Wait for server to start
echo "⏳ Waiting for server to start..."
for i in {1..30}; do
    if curl -s http://localhost:8081/api/stats > /dev/null 2>&1; then
        echo "✅ Server is ready on port 8081"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "❌ Server failed to start within 30 seconds"
        kill $SERVER_PID 2>/dev/null || true
        exit 1
    fi
    echo "⏳ Waiting... ($i/30)"
    sleep 1
done

echo ""
echo "🧪 Starting API call tests..."
echo ""

echo "🚀 Starting PARALLEL API calls to test race conditions..."
echo ""

# Test 1: Simple math query without history (background)
echo "🧪 Test 1: Simple math query (no history) - STARTING IN BACKGROUND"
curl -s -X POST http://localhost:8081/api/query \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What is 2+2?",
    "conversation_id": "math_test_001"
  }' > response1.json &
PID1=$!

# Test 2: Geography query with conversation history (background)
echo "🧪 Test 2: Geography query (with history) - STARTING IN BACKGROUND"
curl -s -X POST http://localhost:8081/api/query \
  -H "Content-Type: application/json" \
  -d '{
    "query": "What is the capital of France?",
    "conversation_id": "geo_test_001",
    "history": [
      {
        "role": "user",
        "content": "Tell me about Europe",
        "timestamp": "2025-01-27T10:00:00Z"
      },
      {
        "role": "assistant",
        "content": "Europe is a continent located in the Northern Hemisphere.",
        "timestamp": "2025-01-27T10:01:00Z"
      }
    ]
  }' > response2.json &
PID2=$!

# Test 3: Creative query with extended history (background)
echo "🧪 Test 3: Creative query (extended history) - STARTING IN BACKGROUND"
curl -s -X POST http://localhost:8081/api/query \
  -H "Content-Type: application/json" \
  -d '{
    "query": "Tell me a short joke about programming",
    "conversation_id": "creative_test_001",
    "history": [
      {
        "role": "user",
        "content": "I am a software developer",
        "timestamp": "2025-01-27T10:00:00Z"
      },
      {
        "role": "assistant",
        "content": "Great! Software development is a fascinating field.",
        "timestamp": "2025-01-27T10:01:00Z"
      },
      {
        "role": "user",
        "content": "I work with Go and Python",
        "timestamp": "2025-01-27T10:02:00Z"
      },
      {
        "role": "assistant",
        "content": "Excellent choice! Both Go and Python are powerful languages.",
        "timestamp": "2025-01-27T10:03:00Z"
      }
    ]
  }' > response3.json &
PID3=$!

# Test 4: Tool usage query with history (background)
echo "🧪 Test 4: Tool usage query (with history) - STARTING IN BACKGROUND"
curl -s -X POST http://localhost:8081/api/query \
  -H "Content-Type: application/json" \
  -d '{
    "query": "List the files in the current directory and tell me what we discussed earlier",
    "conversation_id": "tool_test_001",
    "history": [
      {
        "role": "user",
        "content": "What is the current working directory?",
        "timestamp": "2025-01-27T10:00:00Z"
      },
      {
        "role": "assistant",
        "content": "I can help you check the current working directory.",
        "timestamp": "2025-01-27T10:01:00Z"
      }
    ]
  }' > response4.json &
PID4=$!

echo ""
echo "⏳ All 4 parallel requests started. Waiting for completion..."
echo "📊 Monitoring progress..."

# Wait for all requests to complete
wait $PID1 $PID2 $PID3 $PID4

echo ""
echo "✅ All parallel requests completed!"
echo ""

# Display results
echo "📋 Results from parallel execution:"
echo "===================================="

echo ""
echo "🧪 Test 1: Math query result"
echo "-----------------------------"
if [ -f response1.json ]; then
    cat response1.json | jq '.' 2>/dev/null || cat response1.json
else
    echo "❌ Response file not found"
fi

echo ""
echo "🧪 Test 2: Geography query result"
echo "----------------------------------"
if [ -f response2.json ]; then
    cat response2.json | jq '.' 2>/dev/null || cat response2.json
else
    echo "❌ Response file not found"
fi

echo ""
echo "🧪 Test 3: Creative query result"
echo "--------------------------------"
if [ -f response3.json ]; then
    cat response3.json | jq '.' 2>/dev/null || cat response3.json
else
    echo "❌ Response file not found"
fi

echo ""
echo "🧪 Test 4: Tool usage query result"
echo "----------------------------------"
if [ -f response4.json ]; then
    cat response4.json | jq '.' 2>/dev/null || cat response4.json
else
    echo "❌ Response file not found"
fi

# Clean up response files
rm -f response1.json response2.json response3.json response4.json

echo ""
echo "🔥 STRESS TEST: 10 simultaneous requests..."
echo "=========================================="

# Start 10 simultaneous requests
for i in {1..10}; do
    echo "🚀 Starting stress test request #$i in background..."
    curl -s -X POST http://localhost:8081/api/query \
      -H "Content-Type: application/json" \
      -d "{
        \"query\": \"Stress test request #$i - What is $i + $i?\",
        \"conversation_id\": \"stress_test_$i\"
      }" > "stress_response_$i.json" &
    STRESS_PIDS[$i]=$!
done

echo ""
echo "⏳ All 10 stress test requests started. Waiting for completion..."
wait ${STRESS_PIDS[@]}

echo ""
echo "✅ All stress test requests completed!"
echo ""

# Quick summary of stress test results
echo "📊 Stress Test Summary:"
echo "======================="
for i in {1..10}; do
    if [ -f "stress_response_$i.json" ]; then
        echo "✅ Request #$i: Completed successfully"
    else
        echo "❌ Request #$i: Failed or missing response"
    fi
done

# Clean up stress test files
rm -f stress_response_*.json

# Check final server stats
echo ""
echo "📊 Final Server Statistics"
echo "-------------------------"
stats=$(curl -s http://localhost:8081/api/stats)
echo "$stats"

echo ""
echo "📋 Server Logs (last 20 lines):"
echo "-------------------------------"
tail -20 server.log

echo ""
echo "🔍 Event Analysis:"
echo "-----------------"

# Check if we captured the critical events
if grep -q "tool_call_start" server.log; then
    echo "✅ tool_call_start events captured"
else
    echo "❌ tool_call_start events MISSING"
fi

if grep -q "token_usage" server.log; then
    echo "✅ token_usage events captured"
else
    echo "❌ token_usage events MISSING"
fi

if grep -q "llm_generation_start" server.log; then
    echo "✅ llm_generation_start events captured"
else
    echo "❌ llm_generation_start events MISSING"
fi

if grep -q "llm_generation_end" server.log; then
    echo "✅ llm_generation_end events captured"
else
    echo "❌ llm_generation_end events MISSING"
fi

if grep -q "conversation_end" server.log; then
    echo "✅ conversation_end events captured"
else
    echo "❌ conversation_end events MISSING"
fi

echo ""
echo "🎯 Test completed! Now killing server..."

# Kill the server
kill $SERVER_PID 2>/dev/null || true
sleep 2

# Force kill if needed
if kill -0 $SERVER_PID 2>/dev/null; then
    echo "🔪 Force killing server..."
    kill -9 $SERVER_PID 2>/dev/null || true
fi

echo "✅ Server killed. Test complete!"
echo ""
echo "💡 To analyze results:"
echo "   - Check server.log for detailed event capture"
echo "   - Look for missing events (race condition indicators)"
echo "   - Compare event counts between requests"
echo ""
echo "🎉 Parallel testing script completed successfully!"
