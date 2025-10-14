#!/bin/bash

echo "🧪 Testing Cache Events Fix with Bedrock Provider"
echo "=================================================="

# Wait for server to start
echo "⏳ Waiting for server to start..."
sleep 5

# Test 1: Simple query that should trigger cache events
echo "🔍 Test 1: Making simple query with Bedrock provider..."
curl -X POST http://localhost:8000/api/query \
  -H "Content-Type: application/json" \
  -d '{"query": "What is 2+2?", "provider": "bedrock", "mode": "simple"}' \
  -s | jq .

echo ""
echo "⏳ Waiting for query to process..."
sleep 10

# Test 2: Query that should trigger tool execution and cache hit events
echo "🔍 Test 2: Making query that requires tools (should trigger cache hit events)..."
curl -X POST http://localhost:8000/api/query \
  -H "Content-Type: application/json" \
  -d '{"query": "Use the tavily search tool to find information about Go programming language", "provider": "bedrock", "mode": "react"}' \
  -s | jq .

echo ""
echo "⏳ Waiting for tool execution..."
sleep 15

echo ""
echo "📊 Checking logs for cache events..."
echo "=================================="

# Check for cache events in logs
echo "🔍 Cache operation start events:"
tail -50 logs/server_debug.log | grep -i "cache_operation_start" | tail -5

echo ""
echo "🔍 Cache hit events:"
tail -50 logs/server_debug.log | grep -i "cache_hit" | tail -5

echo ""
echo "🔍 Tool execution events:"
tail -50 logs/server_debug.log | grep -i "tool.*execution\|tool.*call" | tail -5

echo ""
echo "🔍 LLM generation events:"
tail -50 logs/server_debug.log | grep -i "llm.*generation\|bedrock.*response" | tail -5

echo ""
echo "✅ Test completed. Check the frontend to see if cache events are visible."
echo "💡 Expected: cache_operation_start events + cache_hit events during tool execution"
