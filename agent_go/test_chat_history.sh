#!/bin/bash

# Test script for chat history functionality
echo "🧪 Testing Chat History API"

# Test 1: Health check
echo "1. Testing health check..."
curl -s http://localhost:8000/api/chat-history/health | jq .

# Test 2: Create a chat session
echo -e "\n2. Creating a chat session..."
SESSION_RESPONSE=$(curl -s -X POST http://localhost:8000/api/chat-history/sessions \
  -H "Content-Type: application/json" \
  -d '{"session_id": "test-session-123", "title": "Test Chat Session"}')
echo $SESSION_RESPONSE | jq .

# Extract session ID for further tests
SESSION_ID=$(echo $SESSION_RESPONSE | jq -r '.session_id')

# Test 3: List chat sessions
echo -e "\n3. Listing chat sessions..."
curl -s http://localhost:8000/api/chat-history/sessions | jq .

# Test 4: Get specific session
echo -e "\n4. Getting specific session..."
curl -s http://localhost:8000/api/chat-history/sessions/$SESSION_ID | jq .

# Test 5: Update session
echo -e "\n5. Updating session..."
curl -s -X PUT http://localhost:8000/api/chat-history/sessions/$SESSION_ID \
  -H "Content-Type: application/json" \
  -d '{"title": "Updated Test Session", "status": "completed"}' | jq .

# Test 6: Get session events (should be empty initially)
echo -e "\n6. Getting session events..."
curl -s http://localhost:8000/api/chat-history/sessions/$SESSION_ID/events | jq .

# Test 7: Search events
echo -e "\n7. Searching events..."
curl -s "http://localhost:8000/api/chat-history/events?session_id=$SESSION_ID" | jq .


echo -e "\n✅ Chat history API tests completed!"
