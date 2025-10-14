#!/bin/bash

# Test script for the polling API system with multiple observers
echo "🧪 Testing Polling API System with Multiple Observers"
echo "====================================================="

BASE_URL="http://localhost:8000"

echo ""
echo "1. 📝 Registering multiple observers..."
OBSERVER1_RESPONSE=$(curl -s -X POST "$BASE_URL/api/observer/register" \
  -H "Content-Type: application/json" \
  -d '{"session_id": "test_session_observer1"}')

OBSERVER2_RESPONSE=$(curl -s -X POST "$BASE_URL/api/observer/register" \
  -H "Content-Type: application/json" \
  -d '{"session_id": "test_session_observer2"}')

echo "Observer 1 Response: $OBSERVER1_RESPONSE"
echo "Observer 2 Response: $OBSERVER2_RESPONSE"

# Extract observer IDs
if command -v jq &> /dev/null; then
  OBSERVER1_ID=$(echo $OBSERVER1_RESPONSE | jq -r '.observer_id')
  OBSERVER2_ID=$(echo $OBSERVER2_RESPONSE | jq -r '.observer_id')
else
  OBSERVER1_ID=$(echo $OBSERVER1_RESPONSE | grep -o '"observer_id":"[^"]*"' | cut -d'"' -f4)
  OBSERVER2_ID=$(echo $OBSERVER2_RESPONSE | grep -o '"observer_id":"[^"]*"' | cut -d'"' -f4)
fi

echo "Observer 1 ID: $OBSERVER1_ID"
echo "Observer 2 ID: $OBSERVER2_ID"

echo ""
echo "2. 📊 Checking observer statuses..."
STATUS1_RESPONSE=$(curl -s -X GET "$BASE_URL/api/observer/$OBSERVER1_ID/status" \
  -H "Content-Type: application/json")
STATUS2_RESPONSE=$(curl -s -X GET "$BASE_URL/api/observer/$OBSERVER2_ID/status" \
  -H "Content-Type: application/json")
echo "Observer 1 Status: $STATUS1_RESPONSE"
echo "Observer 2 Status: $STATUS2_RESPONSE"

echo ""
echo "3. 🤖 Starting complex AWS cost analysis queries for both observers..."
echo "⚠️  These complex queries may take 10-15 minutes to complete..."

# Start query for observer 1 (React agent)
QUERY1_RESPONSE=$(curl -s -X POST "$BASE_URL/api/query" \
  -H "Content-Type: application/json" \
  -H "X-Observer-ID: $OBSERVER1_ID" \
  -H "X-Session-ID: test_session_observer1" \
  -d '{
    "query": "Analyze costs across all AWS services and provide detailed breakdown. Get cost data for EC2, RDS, S3, Lambda, and other major services. Also check for any unusual spending patterns, identify cost optimization opportunities, and provide recommendations for reducing AWS costs. Include historical data for the last 3 months and forecast for the next month. Generate a comprehensive report with charts and detailed analysis.",
    "provider": "bedrock",
    "servers": ["all"],
    "enabled_tools": ["aws"],
    "max_turns": 5,
    "agent_mode": "react"
  }')

# Start query for observer 2 (Simple agent)
QUERY2_RESPONSE=$(curl -s -X POST "$BASE_URL/api/query" \
  -H "Content-Type: application/json" \
  -H "X-Observer-ID: $OBSERVER2_ID" \
  -H "X-Session-ID: test_session_observer2" \
  -d '{
    "query": "Analyze costs across all AWS services and provide detailed breakdown. Get cost data for EC2, RDS, S3, Lambda, and other major services. Also check for any unusual spending patterns, identify cost optimization opportunities, and provide recommendations for reducing AWS costs. Include historical data for the last 3 months and forecast for the next month. Generate a comprehensive report with charts and detailed analysis.",
    "provider": "bedrock",
    "servers": ["all"],
    "enabled_tools": ["aws"],
    "max_turns": 5,
    "agent_mode": "simple"
  }')

echo "Query 1 Response: $QUERY1_RESPONSE"
echo "Query 2 Response: $QUERY2_RESPONSE"

echo ""
echo "4. ⏳ Polling for events from both observers..."
echo "⏰ Expected completion time: 10-15 minutes"

# Poll both observers simultaneously
LAST_INDEX1=0
LAST_INDEX2=0
POLL_COUNT=0
MAX_POLLS=900  # 15 minutes with 1-second intervals
COMPLETED1=false
COMPLETED2=false

while [ $POLL_COUNT -lt $MAX_POLLS ] && ([ "$COMPLETED1" = false ] || [ "$COMPLETED2" = false ]); do
  POLL_COUNT=$((POLL_COUNT + 1))
  echo ""
  echo "📡 Poll #$POLL_COUNT (${POLL_COUNT}s elapsed)..."
  
  # Poll observer 1
  if [ "$COMPLETED1" = false ]; then
    echo "  Observer 1 (React agent, since index $LAST_INDEX1)..."
    EVENTS1_RESPONSE=$(curl -s -X GET "$BASE_URL/api/observer/$OBSERVER1_ID/events?since=$LAST_INDEX1" \
      -H "Content-Type: application/json")
    
    if command -v jq &> /dev/null; then
      NEW_LAST_INDEX1=$(echo $EVENTS1_RESPONSE | jq -r '.last_event_index')
      EVENT_COUNT1=$(echo $EVENTS1_RESPONSE | jq -r '.events | length')
    else
      NEW_LAST_INDEX1=$(echo $EVENTS1_RESPONSE | grep -o '"last_event_index":[0-9]*' | cut -d':' -f2)
      EVENT_COUNT1=$(echo $EVENTS1_RESPONSE | grep -o '"events":\[.*\]' | grep -o '\[.*\]' | grep -o '{' | wc -l)
    fi
    
    echo "    Received $EVENT_COUNT1 new events, last_event_index: $NEW_LAST_INDEX1"
    
    # Check for completion events (handle both simple and react agents)
    if echo $EVENTS1_RESPONSE | grep -q '"type":"agent_end"\|"type":"conversation_end"\|"type":"query_completed"\|"type":"error"'; then
      echo "    ✅ Observer 1 (React) query completed!"
      COMPLETED1=true
    fi
    
    LAST_INDEX1=$NEW_LAST_INDEX1
  fi
  
  # Poll observer 2
  if [ "$COMPLETED2" = false ]; then
    echo "  Observer 2 (Simple agent, since index $LAST_INDEX2)..."
    EVENTS2_RESPONSE=$(curl -s -X GET "$BASE_URL/api/observer/$OBSERVER2_ID/events?since=$LAST_INDEX2" \
      -H "Content-Type: application/json")
    
    if command -v jq &> /dev/null; then
      NEW_LAST_INDEX2=$(echo $EVENTS2_RESPONSE | jq -r '.last_event_index')
      EVENT_COUNT2=$(echo $EVENTS2_RESPONSE | jq -r '.events | length')
    else
      NEW_LAST_INDEX2=$(echo $EVENTS2_RESPONSE | grep -o '"last_event_index":[0-9]*' | cut -d':' -f2)
      EVENT_COUNT2=$(echo $EVENTS2_RESPONSE | grep -o '"events":\[.*\]' | grep -o '\[.*\]' | grep -o '{' | wc -l)
    fi
    
    echo "    Received $EVENT_COUNT2 new events, last_event_index: $NEW_LAST_INDEX2"
    
    # Check for completion events (handle both simple and react agents)
    if echo $EVENTS2_RESPONSE | grep -q '"type":"agent_end"\|"type":"conversation_end"\|"type":"query_completed"\|"type":"error"'; then
      echo "    ✅ Observer 2 (Simple) query completed!"
      COMPLETED2=true
    fi
    
    LAST_INDEX2=$NEW_LAST_INDEX2
  fi
  
  # Wait before next poll
  sleep 1
done

if [ "$COMPLETED1" = false ]; then
  echo "⚠️ Observer 1 (React) query did not complete within $MAX_POLLS polls (15 minutes)"
  echo "💡 The complex AWS cost analysis may still be running..."
fi
if [ "$COMPLETED2" = false ]; then
  echo "⚠️ Observer 2 (Simple) query did not complete within $MAX_POLLS polls (15 minutes)"
  echo "💡 The complex AWS cost analysis may still be running..."
fi

echo ""
echo "5. 📊 Final analysis for both observers..."

# Get final events for observer 1
FINAL_EVENTS1=$(curl -s -X GET "$BASE_URL/api/observer/$OBSERVER1_ID/events?since=0" \
  -H "Content-Type: application/json")
TOTAL_EVENTS1=$(echo $FINAL_EVENTS1 | jq -r '.events | length')

# Get final events for observer 2
FINAL_EVENTS2=$(curl -s -X GET "$BASE_URL/api/observer/$OBSERVER2_ID/events?since=0" \
  -H "Content-Type: application/json")
TOTAL_EVENTS2=$(echo $FINAL_EVENTS2 | jq -r '.events | length')

echo "Observer 1 (React) total events: $TOTAL_EVENTS1"
echo "Observer 2 (Simple) total events: $TOTAL_EVENTS2"

echo ""
echo "6. 🔍 Checking for event isolation..."
# Check if observers have different events (they should)
EVENT_TYPES1=$(echo $FINAL_EVENTS1 | jq -r '.events[] | .type' | sort | uniq)
EVENT_TYPES2=$(echo $FINAL_EVENTS2 | jq -r '.events[] | .type' | sort | uniq)

echo "Observer 1 (React) event types: $EVENT_TYPES1"
echo "Observer 2 (Simple) event types: $EVENT_TYPES2"

echo ""
echo "7. 📈 Event statistics comparison..."
STATS1=$(echo $FINAL_EVENTS1 | jq -r '.events | group_by(.type) | map({type: .[0].type, count: length})')
STATS2=$(echo $FINAL_EVENTS2 | jq -r '.events | group_by(.type) | map({type: .[0].type, count: length})')

echo "Observer 1 (React) statistics: $STATS1"
echo "Observer 2 (Simple) statistics: $STATS2"

echo ""
echo "8. 🗑️ Removing both observers..."
REMOVE1_RESPONSE=$(curl -s -X DELETE "$BASE_URL/api/observer/$OBSERVER1_ID" \
  -H "Content-Type: application/json")
REMOVE2_RESPONSE=$(curl -s -X DELETE "$BASE_URL/api/observer/$OBSERVER2_ID" \
  -H "Content-Type: application/json")
echo "Remove Observer 1 Response: $REMOVE1_RESPONSE"
echo "Remove Observer 2 Response: $REMOVE2_RESPONSE"

echo ""
echo "9. ✅ Verifying observer removal..."
VERIFY1_RESPONSE=$(curl -s -X GET "$BASE_URL/api/observer/$OBSERVER1_ID/status" \
  -H "Content-Type: application/json")
VERIFY2_RESPONSE=$(curl -s -X GET "$BASE_URL/api/observer/$OBSERVER2_ID/status" \
  -H "Content-Type: application/json")
echo "Verify Observer 1 Response: $VERIFY1_RESPONSE"
echo "Verify Observer 2 Response: $VERIFY2_RESPONSE"

echo ""
echo "🎉 Multiple Observer Polling API test completed!" 