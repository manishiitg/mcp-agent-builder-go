#!/bin/bash

echo "🧪 Testing Conversation History with Follow-up Questions"
echo "======================================================="

BASE_URL="http://localhost:8000"

# Test 1: Initial query
echo "📝 Test 1: Initial query"
SESSION_ID=$(curl -s -X POST "$BASE_URL/api/observer/register" \
  -H "Content-Type: application/json" \
  -d '{"session_id": "test_conversation_history"}' | jq -r '.observer_id')

echo "Session ID: $SESSION_ID"

# Submit first query
echo "Submitting first query..."
QUERY_RESPONSE=$(curl -s -X POST "$BASE_URL/api/query" \
  -H "Content-Type: application/json" \
  -H "X-Session-ID: test_conversation_history" \
  -H "X-Observer-ID: $SESSION_ID" \
  -d '{
    "query": "What is 2 + 2?",
    "provider": "bedrock",
    "agent_mode": "simple"
  }')

echo "Query response: $QUERY_RESPONSE"

# Wait for first query to complete
echo "Waiting for first query to complete..."
sleep 15

# Submit follow-up query
echo "📝 Test 2: Follow-up query"
FOLLOW_UP_RESPONSE=$(curl -s -X POST "$BASE_URL/api/query" \
  -H "Content-Type: application/json" \
  -H "X-Session-ID: test_conversation_history" \
  -H "X-Observer-ID: $SESSION_ID" \
  -d '{
    "query": "Now multiply that result by 3",
    "provider": "bedrock",
    "agent_mode": "simple"
  }')

echo "Follow-up response: $FOLLOW_UP_RESPONSE"

# Wait for follow-up to complete
echo "Waiting for follow-up to complete..."
sleep 15

# Submit another follow-up query
echo "📝 Test 3: Another follow-up query"
FINAL_RESPONSE=$(curl -s -X POST "$BASE_URL/api/query" \
  -H "Content-Type: application/json" \
  -H "X-Session-ID: test_conversation_history" \
  -H "X-Observer-ID: $SESSION_ID" \
  -d '{
    "query": "What was the original question and what is the final result?",
    "provider": "bedrock",
    "agent_mode": "simple"
  }')

echo "Final response: $FINAL_RESPONSE"

echo "✅ Conversation history test completed!"
echo "Check the responses above to verify that the agent remembers previous context."
echo ""
echo "Expected behavior:"
echo "- First query should answer '2 + 2 = 4'"
echo "- Follow-up should answer '4 * 3 = 12'"
echo "- Final query should reference both previous questions and give the complete calculation" 