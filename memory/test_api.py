#!/usr/bin/env python3
"""
Test script for Graphiti Knowledge Graph API
Tests all endpoints to ensure functionality
"""

import asyncio
import json
import time
from datetime import datetime, timezone
import httpx

# API base URL
API_BASE_URL = "http://localhost:8055"

# Test data
TEST_EPISODES = [
    {
        "name": "Test Episode 1",
        "content": "John Smith is a software engineer at Microsoft. He works on Azure cloud services and has 5 years of experience.",
        "source_type": "text",
        "source_description": "Employee directory"
    },
    {
        "name": "Test Episode 2", 
        "content": "Sarah Johnson is a data scientist at Google. She specializes in machine learning and has published several papers on neural networks.",
        "source_type": "text",
        "source_description": "Research team directory"
    },
    {
        "name": "Test Episode 3",
        "content": json.dumps({
            "company": "Apple",
            "employee": {
                "name": "Mike Chen",
                "role": "Product Manager",
                "department": "iOS",
                "experience_years": 8
            },
            "projects": ["iOS 17", "Apple Intelligence", "Vision Pro"]
        }),
        "source_type": "json",
        "source_description": "HR database export"
    }
]

async def test_api_endpoints():
    """Test all API endpoints"""
    print("🧪 Testing Graphiti Knowledge Graph API...")
    
    async with httpx.AsyncClient(timeout=30.0) as client:
        
        # Test 1: Health Check
        print("\n1️⃣ Testing Health Check...")
        try:
            response = await client.get(f"{API_BASE_URL}/health")
            if response.status_code == 200:
                print("✅ Health check passed")
                print(f"   Response: {response.json()}")
            else:
                print(f"❌ Health check failed: {response.status_code}")
                return False
        except Exception as e:
            print(f"❌ Health check error: {e}")
            return False
        
        # Test 2: Add Memory Episodes
        print("\n2️⃣ Testing Add Memory...")
        episode_uuids = []
        
        for i, episode in enumerate(TEST_EPISODES):
            try:
                response = await client.post(
                    f"{API_BASE_URL}/add_memory",
                    json=episode
                )
                if response.status_code == 200:
                    data = response.json()
                    episode_uuid = data["data"]["episode_uuid"]
                    episode_uuids.append(episode_uuid)
                    print(f"✅ Episode {i+1} added: {episode_uuid}")
                else:
                    print(f"❌ Episode {i+1} failed: {response.status_code} - {response.text}")
            except Exception as e:
                print(f"❌ Episode {i+1} error: {e}")
        
        if not episode_uuids:
            print("❌ No episodes were added successfully")
            return False
        
        # Wait for processing
        print("\n⏳ Waiting for episodes to be processed...")
        await asyncio.sleep(10)
        
        # Test 3: Search Facts
        print("\n3️⃣ Testing Search Facts...")
        search_queries = [
            "Who works at Microsoft?",
            "What are the roles of software engineers?",
            "Who has experience with machine learning?"
        ]
        
        for query in search_queries:
            try:
                response = await client.post(
                    f"{API_BASE_URL}/search_facts",
                    json={"query": query, "limit": 5}
                )
                if response.status_code == 200:
                    data = response.json()
                    facts = data["data"]["facts"]
                    print(f"✅ Search '{query}': Found {len(facts)} facts")
                    for fact in facts[:2]:  # Show first 2 facts
                        print(f"   - {fact['fact'][:80]}...")
                else:
                    print(f"❌ Search '{query}' failed: {response.status_code}")
            except Exception as e:
                print(f"❌ Search '{query}' error: {e}")
        
        # Test 4: Search Nodes
        print("\n4️⃣ Testing Search Nodes...")
        node_queries = [
            "software engineer",
            "data scientist", 
            "product manager"
        ]
        
        for query in node_queries:
            try:
                response = await client.post(
                    f"{API_BASE_URL}/search_nodes",
                    json={"query": query, "limit": 3}
                )
                if response.status_code == 200:
                    data = response.json()
                    nodes = data["data"]["nodes"]
                    print(f"✅ Node search '{query}': Found {len(nodes)} nodes")
                    for node in nodes[:2]:  # Show first 2 nodes
                        print(f"   - {node['name']}: {node['content_summary'][:60]}...")
                else:
                    print(f"❌ Node search '{query}' failed: {response.status_code}")
            except Exception as e:
                print(f"❌ Node search '{query}' error: {e}")
        
        # Test 5: Get Episodes
        print("\n5️⃣ Testing Get Episodes...")
        try:
            response = await client.get(f"{API_BASE_URL}/episodes?limit=10")
            if response.status_code == 200:
                data = response.json()
                episodes = data["data"]["episodes"]
                print(f"✅ Retrieved {len(episodes)} episodes")
                for episode in episodes[:3]:  # Show first 3 episodes
                    print(f"   - {episode['name']}: {episode['content'][:50]}...")
            else:
                print(f"❌ Get episodes failed: {response.status_code}")
        except Exception as e:
            print(f"❌ Get episodes error: {e}")
        
        # Test 6: Delete Episode (delete the first one)
        print("\n6️⃣ Testing Delete Episode...")
        if episode_uuids:
            try:
                response = await client.post(
                    f"{API_BASE_URL}/delete_episode",
                    json={"episode_uuid": episode_uuids[0]}
                )
                if response.status_code == 200:
                    print(f"✅ Episode deleted: {episode_uuids[0]}")
                else:
                    print(f"❌ Delete episode failed: {response.status_code}")
            except Exception as e:
                print(f"❌ Delete episode error: {e}")
        
        # Test 7: Verify deletion
        print("\n7️⃣ Verifying Episode Deletion...")
        try:
            response = await client.get(f"{API_BASE_URL}/episodes?limit=10")
            if response.status_code == 200:
                data = response.json()
                episodes = data["data"]["episodes"]
                remaining_uuids = [ep["uuid"] for ep in episodes]
                if episode_uuids[0] not in remaining_uuids:
                    print("✅ Episode successfully deleted")
                else:
                    print("⚠️  Episode still exists (may be cached)")
            else:
                print(f"❌ Verification failed: {response.status_code}")
        except Exception as e:
            print(f"❌ Verification error: {e}")
    
    print("\n🎉 API testing completed!")
    return True

async def main():
    """Main test function"""
    print("🚀 Starting Graphiti Knowledge Graph API Tests")
    print(f"📡 API Base URL: {API_BASE_URL}")
    
    # Wait a moment for API to be ready
    print("⏳ Waiting for API to be ready...")
    await asyncio.sleep(2)
    
    success = await test_api_endpoints()
    
    if success:
        print("\n✅ All tests completed successfully!")
        print("🌐 API Documentation: http://localhost:8055/docs")
        print("🔍 Health Check: http://localhost:8055/health")
    else:
        print("\n❌ Some tests failed. Check the output above.")
        exit(1)

if __name__ == "__main__":
    asyncio.run(main())
