#!/bin/bash

# Test script for single observer focusing on missing message types
echo "🧪 Testing Single Observer - Missing Message Types"
echo "=================================================="

BASE_URL="http://localhost:8000"

echo ""
echo "1. 📝 Registering observer..."
OBSERVER_RESPONSE=$(curl -s -X POST "$BASE_URL/api/observer/register" \
  -H "Content-Type: application/json" \
  -d '{"session_id": "test_session_single"}')

echo "Observer Response: $OBSERVER_RESPONSE"

# Extract observer ID
if command -v jq &> /dev/null; then
  OBSERVER_ID=$(echo $OBSERVER_RESPONSE | jq -r '.observer_id')
else
  OBSERVER_ID=$(echo $OBSERVER_RESPONSE | grep -o '"observer_id":"[^"]*"' | cut -d'"' -f4)
fi

echo "Observer ID: $OBSERVER_ID"

echo ""
echo "2. 🤖 Starting complex AWS cost analysis query..."
echo "⚠️  This complex query may take 10-15 minutes to complete..."
QUERY_RESPONSE=$(curl -s -X POST "$BASE_URL/api/query" \
  -H "Content-Type: application/json" \
  -H "X-Observer-ID: $OBSERVER_ID" \
  -H "X-Session-ID: test_session_single" \
  -d '{
    "query": "Analyze costs across all AWS services and provide detailed breakdown. Get cost data for EC2, RDS, S3, Lambda, and other major services. Also check for any unusual spending patterns, identify cost optimization opportunities, and provide recommendations for reducing AWS costs. Include historical data for the last 3 months and forecast for the next month. Generate a comprehensive report with charts and detailed analysis.",
    "provider": "bedrock",
    "servers": ["all"],
    "enabled_tools": ["aws"],
    "max_turns": 5,
    "agent_mode": "react"
  }')

echo "Query Response: $QUERY_RESPONSE"

echo ""
echo "3. ⏳ Polling for events and checking message types..."
echo "⏰ Expected completion time: 10-15 minutes"

LAST_INDEX=0
POLL_COUNT=0
MAX_POLLS=900  # 15 minutes with 1-second intervals
COMPLETED=false

# Track missing message types
MISSING_TYPES=("system_prompt" "user_message" "tool_output" "tool_response" "throttling_detected" "fallback_model_used" "token_usage")
FOUND_TYPES=()

while [ $POLL_COUNT -lt $MAX_POLLS ] && [ "$COMPLETED" = false ]; do
  POLL_COUNT=$((POLL_COUNT + 1))
  echo ""
  echo "📡 Poll #$POLL_COUNT (${POLL_COUNT}s elapsed)..."
  
  EVENTS_RESPONSE=$(curl -s -X GET "$BASE_URL/api/observer/$OBSERVER_ID/events?since=$LAST_INDEX" \
    -H "Content-Type: application/json")
  
  if command -v jq &> /dev/null; then
    NEW_LAST_INDEX=$(echo $EVENTS_RESPONSE | jq -r '.last_event_index')
    EVENT_COUNT=$(echo $EVENTS_RESPONSE | jq -r '.events | length')
    EVENT_TYPES=$(echo $EVENTS_RESPONSE | jq -r '.events[] | .type' | sort | uniq)
  else
    NEW_LAST_INDEX=$(echo $EVENTS_RESPONSE | grep -o '"last_event_index":[0-9]*' | cut -d':' -f2)
    EVENT_COUNT=$(echo $EVENTS_RESPONSE | grep -o '"events":\[.*\]' | grep -o '\[.*\]' | grep -o '{' | wc -l)
    EVENT_TYPES=$(echo $EVENTS_RESPONSE | grep -o '"type":"[^"]*"' | cut -d'"' -f4 | sort | uniq)
  fi
  
  echo "    Received $EVENT_COUNT new events, last_event_index: $NEW_LAST_INDEX"
  echo "    Event types: $EVENT_TYPES"
  
  # Check for missing message types
  for type in "${MISSING_TYPES[@]}"; do
    if echo "$EVENT_TYPES" | grep -q "$type"; then
      if [[ ! " ${FOUND_TYPES[@]} " =~ " ${type} " ]]; then
        echo "    ✅ FOUND MISSING TYPE: $type"
        FOUND_TYPES+=("$type")
      fi
    fi
  done
  
  # Check for completion events (handle both simple and react agents)
  if echo $EVENTS_RESPONSE | grep -q '"type":"agent_end"\|"type":"conversation_end"\|"type":"query_completed"\|"type":"error"'; then
    echo "    ✅ Query completed!"
    COMPLETED=true
  fi
  
  LAST_INDEX=$NEW_LAST_INDEX
  sleep 1
done

if [ "$COMPLETED" = false ]; then
  echo "⚠️ Query did not complete within $MAX_POLLS polls (15 minutes)"
  echo "💡 The complex AWS cost analysis may still be running..."
fi

echo ""
echo "4. 📊 Final analysis..."

# Get all events
FINAL_EVENTS=$(curl -s -X GET "$BASE_URL/api/observer/$OBSERVER_ID/events?since=0" \
  -H "Content-Type: application/json")

if command -v jq &> /dev/null; then
  TOTAL_EVENTS=$(echo $FINAL_EVENTS | jq -r '.events | length')
  ALL_EVENT_TYPES=$(echo $FINAL_EVENTS | jq -r '.events[] | .type' | sort | uniq)
  EVENT_STATS=$(echo $FINAL_EVENTS | jq -r '.events | group_by(.type) | map({type: .[0].type, count: length})')
else
  TOTAL_EVENTS=$(echo $FINAL_EVENTS | grep -o '"events":\[.*\]' | grep -o '\[.*\]' | grep -o '{' | wc -l)
  ALL_EVENT_TYPES=$(echo $FINAL_EVENTS | grep -o '"type":"[^"]*"' | cut -d'"' -f4 | sort | uniq)
fi

echo "Total events: $TOTAL_EVENTS"
echo "All event types: $ALL_EVENT_TYPES"
echo "Event statistics: $EVENT_STATS"

echo ""
echo "5. 🔍 Missing message types analysis..."
echo "Looking for: system_prompt, user_message, tool_output, tool_response, throttling_detected, fallback_model_used, token_usage"

for type in "${MISSING_TYPES[@]}"; do
  if echo "$ALL_EVENT_TYPES" | grep -q "$type"; then
    echo "    ✅ $type - FOUND"
  else
    echo "    ❌ $type - MISSING"
  fi
done

echo ""
echo "6. 🗑️ Removing observer..."
REMOVE_RESPONSE=$(curl -s -X DELETE "$BASE_URL/api/observer/$OBSERVER_ID" \
  -H "Content-Type: application/json")
echo "Remove Response: $REMOVE_RESPONSE"

echo ""
echo "🎉 Single Observer Message Types test completed!" 