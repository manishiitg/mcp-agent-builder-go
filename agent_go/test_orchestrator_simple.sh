#!/bin/bash

echo "🧪 Simple Orchestrator Test"
echo "==========================="

BASE_URL="http://localhost:8000"

echo ""
echo "1. 📝 Registering observer..."
OBSERVER_RESPONSE=$(curl -s -X POST "$BASE_URL/api/observer/register" \
  -H "Content-Type: application/json" \
  -d '{"session_id": "test_orch_simple"}')

echo "Observer Response: $OBSERVER_RESPONSE"

# Extract observer ID
if command -v jq &> /dev/null; then
  OBSERVER_ID=$(echo $OBSERVER_RESPONSE | jq -r '.observer_id')
else
  OBSERVER_ID=$(echo $OBSERVER_RESPONSE | grep -o '"observer_id":"[^"]*"' | cut -d'"' -f4)
fi

echo "Observer ID: $OBSERVER_ID"

echo ""
echo "2. 🤖 Testing Orchestrator Mode..."
QUERY_RESPONSE=$(curl -s -X POST "$BASE_URL/api/query" \
  -H "Content-Type: application/json" \
  -H "X-Observer-ID: $OBSERVER_ID" \
  -H "X-Session-ID: test_orch_simple" \
  -d '{
    "query": "Create a simple plan",
    "provider": "bedrock",
    "agent_mode": "orchestrator",
    "max_turns": 3
  }')

echo "Query Response: $QUERY_RESPONSE"

echo ""
echo "3. ⏳ Waiting for events..."
sleep 10

echo ""
echo "4. 📊 Checking events..."
EVENTS_RESPONSE=$(curl -s "http://localhost:8000/api/observer/$OBSERVER_ID/events?since=0")
echo "Events Response: $EVENTS_RESPONSE"

echo ""
echo "5. 🗑️ Cleaning up..."
REMOVE_RESPONSE=$(curl -s -X DELETE "$BASE_URL/api/observer/$OBSERVER_ID" \
  -H "Content-Type: application/json")
echo "Remove Response: $REMOVE_RESPONSE"

echo ""
echo "🎉 Simple test completed!" 