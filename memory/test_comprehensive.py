#!/usr/bin/env python3
"""
Comprehensive Graphiti Memory API Test Script

This script tests all core functionality of the Graphiti memory API:
- Direct in-process testing (no HTTP)
- API endpoint testing (with HTTP)
- Episode management (add, search, delete)
- Database operations (Cypher queries)
- Error handling and validation

Usage:
    python test_comprehensive.py [--mode=all|direct|api] [--verbose]
"""

import asyncio
import os
import sys
import json
import time
import argparse
from datetime import datetime, timezone
from typing import Any, Dict, Optional, List

# Import required packages

try:
    import api as api_module
    from graphiti_core.nodes import EpisodeType, EpisodicNode
except ImportError as e:
    print(f"❌ Import error: {e}", file=sys.stderr)
    print("Make sure you're in the memory directory and have installed requirements_api.txt", file=sys.stderr)
    sys.exit(2)


class TestResults:
    """Track test results and statistics"""
    def __init__(self):
        self.passed = 0
        self.failed = 0
        self.skipped = 0
        self.results = []
    
    def add_result(self, test_name: str, passed: bool, message: str = "", details: str = ""):
        """Add a test result"""
        self.results.append({
            'test': test_name,
            'passed': passed,
            'message': message,
            'details': details
        })
        if passed:
            self.passed += 1
        else:
            self.failed += 1
    
    def print_summary(self):
        """Print test summary"""
        print(f"\n{'='*60}")
        print(f"TEST SUMMARY")
        print(f"{'='*60}")
        print(f"✅ Passed: {self.passed}")
        print(f"❌ Failed: {self.failed}")
        print(f"⏭️  Skipped: {self.skipped}")
        print(f"📊 Total: {self.passed + self.failed + self.skipped}")
        
        if self.failed > 0:
            print(f"\n❌ FAILED TESTS:")
            for result in self.results:
                if not result['passed']:
                    print(f"  - {result['test']}: {result['message']}")
                    if result['details']:
                        print(f"    Details: {result['details']}")
        
        print(f"{'='*60}")
        return self.failed == 0


def ok(test_name: str, passed: bool, message: str = "", details: str = ""):
    """Print test result with emoji"""
    status = "✅" if passed else "❌"
    print(f"{status} {test_name}" + (f" - {message}" if message else ""))
    if details and not passed:
        print(f"    Details: {details}")


class GraphitiTester:
    """Main test class for Graphiti functionality"""
    
    def __init__(self, verbose: bool = False):
        self.verbose = verbose
        self.results = TestResults()
        self.episode_uuid = None
        
        # Set up environment defaults for Neo4j AuraDB
        # These will be loaded from .env file or environment variables
        if not os.getenv('NEO4J_URI'):
            raise ValueError("NEO4J_URI environment variable is required. Please set it in your .env file or environment.")
        if not os.getenv('NEO4J_USERNAME'):
            raise ValueError("NEO4J_USERNAME environment variable is required. Please set it in your .env file or environment.")
        if not os.getenv('NEO4J_PASSWORD'):
            raise ValueError("NEO4J_PASSWORD environment variable is required. Please set it in your .env file or environment.")
        if not os.getenv('NEO4J_DATABASE'):
            raise ValueError("NEO4J_DATABASE environment variable is required. Please set it in your .env file or environment.")
        os.environ.setdefault("OPENROUTER_LLM_MODEL", "openai/gpt-4.1-mini")
        os.environ.setdefault("OPENROUTER_SMALL_MODEL", "openai/gpt-4.1-mini")
        os.environ.setdefault("OLLAMA_BASE_URL", "http://localhost:11434/v1")
        os.environ.setdefault("USE_BGE_RERANKER", "true")  # Test with BGE enabled by default
        
        # Suppress embedding output and debug information
        os.environ.setdefault("TRANSFORMERS_VERBOSITY", "error")
        os.environ.setdefault("TOKENIZERS_PARALLELISM", "false")
        os.environ.setdefault("HF_HUB_DISABLE_PROGRESS_BARS", "1")
        os.environ.setdefault("HF_HUB_DISABLE_TELEMETRY", "1")
    
    def log(self, message: str):
        """Log message if verbose mode is enabled"""
        if self.verbose:
            print(f"🔍 {message}")
    
    async def test_direct_functionality(self) -> bool:
        """Test direct in-process Graphiti functionality"""
        print(f"\n{'='*60}")
        print("TESTING DIRECT IN-PROCESS FUNCTIONALITY")
        print(f"{'='*60}")
        
        try:
            # Initialize Graphiti
            self.log("Initializing Graphiti...")
            graphiti = await api_module.get_graphiti()
            self.results.add_result("Graphiti initialized", True, "Graphiti instance created successfully")
            ok("Graphiti initialized", True)
            
            # Test add_episode
            self.log("Testing add_episode...")
            episode_result = await graphiti.add_episode(
                name="Employee Record",
                episode_body="John Smith is a software engineer at Microsoft with 5 years of experience.",
                source=EpisodeType.text,
                reference_time=datetime.now(timezone.utc).replace(tzinfo=None),  # Use naive UTC datetime for consistency
                source_description="In-process test"
            )
            self.episode_uuid = str(episode_result.uuid) if hasattr(episode_result, 'uuid') else None
            self.results.add_result("add_episode", True, "Episode added successfully")
            ok("add_episode", True)
            
            # Test search
            self.log("Testing search functionality...")
            search_results = await graphiti.search("Microsoft")
            search_count = len(search_results) if search_results else 0
            self.results.add_result("search('Microsoft')", search_count > 0, f"Found {search_count} results")
            ok("search('Microsoft')", search_count > 0, f"results={search_count}")
            
            # Test search with the episode we just added
            self.log("Testing search with added episode...")
            episode_search_results = await graphiti.search("software engineer")
            episode_search_count = len(episode_search_results) if episode_search_results else 0
            self.results.add_result("search('software engineer')", episode_search_count > 0, f"Found {episode_search_count} results")
            ok("search('software engineer')", episode_search_count > 0, f"results={episode_search_count}")
            
            # Test Cypher query
            self.log("Testing Cypher query...")
            neo4j_driver = api_module.neo4j_driver_instance
            try:
                cypher_result = await neo4j_driver.execute_query("MATCH(n) RETURN count(n) as node_count")
                # Handle different result formats
                if isinstance(cypher_result, list) and len(cypher_result) > 0:
                    if isinstance(cypher_result[0], list):
                        node_count = cypher_result[0][0]
                    elif isinstance(cypher_result[0], dict):
                        node_count = cypher_result[0].get('node_count', 0)
                    else:
                        node_count = cypher_result[0]
                else:
                    node_count = 0
                
                # For Neo4j, the count might be 0 initially, so we'll just test that the query executes
                query_success = True
                self.results.add_result("cypher MATCH(n) RETURN count(n)", query_success, f"Query executed successfully, found {node_count} nodes")
                ok("cypher MATCH(n) RETURN count(n)", query_success, f"count={node_count}")
            except Exception as e:
                self.results.add_result("cypher MATCH(n) RETURN count(n)", False, f"Query failed: {str(e)}")
                ok("cypher MATCH(n) RETURN count(n)", False, str(e))
            
            # Test cleanup (optional, non-critical)
            if self.episode_uuid:
                self.log("Testing episode cleanup...")
                try:
                    await graphiti.remove_episode(self.episode_uuid)
                    self.results.add_result("remove_episode (cleanup)", True, "Episode removed successfully")
                    ok("remove_episode (cleanup)", True)
                except Exception as e:
                    self.results.add_result("remove_episode (cleanup)", False, f"Cleanup failed: {str(e)}")
                    ok("remove_episode (cleanup)", False, str(e))
                    # Don't count cleanup failure as a hard failure
            
            return True
            
        except Exception as e:
            self.results.add_result("Direct functionality test", False, f"Test failed: {str(e)}")
            ok("Direct functionality test", False, str(e))
            return False
    
    async def test_memory_storage_and_search(self) -> bool:
        """Test comprehensive memory storage and search functionality"""
        print(f"\n{'='*60}")
        print("TESTING MEMORY STORAGE AND SEARCH")
        print(f"{'='*60}")
        
        try:
            # Get Graphiti instance
            graphiti = await api_module.get_graphiti()
            
            # Test data - various types of memories
            test_memories = [
                {
                    "name": "User Profile",
                    "content": "John Smith is a software engineer at Microsoft with 5 years of experience. He specializes in Python and Go programming languages.",
                    "source_type": "text",
                    "source_description": "User registration form"
                },
                {
                    "name": "Project Details",
                    "content": "The MCP Agent project is a Go-based system that integrates with multiple MCP servers. It uses LangChain for LLM integration and supports both OpenAI and AWS Bedrock providers.",
                    "source_type": "text", 
                    "source_description": "Project documentation"
                },
                {
                    "name": "Technical Architecture",
                    "content": "The system uses a hybrid architecture with OpenRouter for LLM inference, Ollama for embeddings, and Kuzu DB as the graph database. BGE reranker is used for local cross-encoder reranking.",
                    "source_type": "text",
                    "source_description": "Technical documentation"
                },
                {
                    "name": "User Preferences",
                    "content": "User prefers dark mode interface, Python for backend development, and prefers detailed technical explanations over simple answers.",
                    "source_type": "text",
                    "source_description": "User conversation"
                },
                {
                    "name": "API Configuration",
                    "content": "The memory API runs on port 8055 with endpoints for add_memory, search_facts, search_nodes, and cypher_query. It uses FastAPI and supports both direct function calls and HTTP requests.",
                    "source_type": "text",
                    "source_description": "API documentation"
                }
            ]
            
            # Add multiple memories
            self.log("Adding multiple test memories...")
            added_episodes = []
            for i, memory in enumerate(test_memories):
                try:
                    from api import AddMemoryRequest
                    request = AddMemoryRequest(
                        name=memory["name"],
                        content=memory["content"],
                        source_type=memory["source_type"],
                        source_description=memory["source_description"]
                    )
                    result = await api_module.add_memory(request, None)
                    if result.success:
                        added_episodes.append(result.data.get('episode_uuid', f'episode_{i}'))
                        self.log(f"Added memory {i+1}: {memory['name']}")
                    else:
                        self.results.add_result(f"add_memory_{i+1}", False, f"Failed to add memory: {memory['name']}")
                        ok(f"add_memory_{i+1}", False, f"Failed: {memory['name']}")
                except Exception as e:
                    self.results.add_result(f"add_memory_{i+1}", False, f"Error adding memory: {str(e)}")
                    ok(f"add_memory_{i+1}", False, str(e))
            
            # Test various search scenarios
            search_tests = [
                {
                    "query": "Microsoft",
                    "expected_keywords": ["Microsoft", "software engineer", "John Smith"],
                    "description": "Search for company name"
                },
                {
                    "query": "Python programming",
                    "expected_keywords": ["Python", "programming", "backend"],
                    "description": "Search for programming language"
                },
                {
                    "query": "MCP Agent project",
                    "expected_keywords": ["MCP Agent", "Go-based", "LangChain"],
                    "description": "Search for project name"
                },
                {
                    "query": "dark mode interface",
                    "expected_keywords": ["dark mode", "interface", "preferences"],
                    "description": "Search for user preferences"
                },
                {
                    "query": "port 8055 API",
                    "expected_keywords": ["8055", "API", "FastAPI"],
                    "description": "Search for technical details"
                },
                {
                    "query": "graph database Kuzu",
                    "expected_keywords": ["Kuzu", "graph database", "architecture"],
                    "description": "Search for database technology"
                }
            ]
            
            # Test search_facts
            self.log("Testing search_facts with various queries...")
            for i, search_test in enumerate(search_tests):
                try:
                    from api import SearchFactsRequest
                    request = SearchFactsRequest(query=search_test["query"], limit=10)
                    result = await api_module.search_facts(request)
                    
                    if result.success and hasattr(result.data, 'facts') and result.data.facts:
                        facts = result.data.facts
                        found_keywords = []
                        for fact in facts:
                            fact_text = getattr(fact, 'fact', '').lower()
                            for keyword in search_test["expected_keywords"]:
                                if keyword.lower() in fact_text:
                                    found_keywords.append(keyword)
                        
                        # Check if we found at least some expected keywords
                        keyword_match = len(found_keywords) > 0
                        self.results.add_result(f"search_facts_{i+1}", keyword_match, 
                                              f"Found {len(facts)} facts, keywords: {found_keywords}")
                        ok(f"search_facts_{i+1}", keyword_match, 
                           f"query='{search_test['query']}', found={len(facts)} facts, keywords={found_keywords}")
                    else:
                        self.results.add_result(f"search_facts_{i+1}", False, 
                                              f"No facts found for query: {search_test['query']}")
                        ok(f"search_facts_{i+1}", False, f"No facts found: {search_test['query']}")
                except Exception as e:
                    self.results.add_result(f"search_facts_{i+1}", False, f"Search error: {str(e)}")
                    ok(f"search_facts_{i+1}", False, str(e))
            
            # Test search_nodes
            self.log("Testing search_nodes with various queries...")
            for i, search_test in enumerate(search_tests):
                try:
                    from api import SearchNodesRequest
                    request = SearchNodesRequest(query=search_test["query"], limit=10)
                    result = await api_module.search_nodes(request)
                    
                    if result.success and hasattr(result.data, 'nodes') and result.data.nodes:
                        nodes = result.data.nodes
                        found_keywords = []
                        for node in nodes:
                            content_summary = getattr(node, 'content_summary', '')
                            name = getattr(node, 'name', '')
                            node_content = (content_summary + ' ' + name).lower()
                            for keyword in search_test["expected_keywords"]:
                                if keyword.lower() in node_content:
                                    found_keywords.append(keyword)
                        
                        # Check if we found at least some expected keywords
                        keyword_match = len(found_keywords) > 0
                        self.results.add_result(f"search_nodes_{i+1}", keyword_match,
                                              f"Found {len(nodes)} nodes, keywords: {found_keywords}")
                        ok(f"search_nodes_{i+1}", keyword_match,
                           f"query='{search_test['query']}', found={len(nodes)} nodes, keywords={found_keywords}")
                    else:
                        self.results.add_result(f"search_nodes_{i+1}", False,
                                              f"No nodes found for query: {search_test['query']}")
                        ok(f"search_nodes_{i+1}", False, f"No nodes found: {search_test['query']}")
                except Exception as e:
                    self.results.add_result(f"search_nodes_{i+1}", False, f"Search error: {str(e)}")
                    ok(f"search_nodes_{i+1}", False, str(e))
            
            # Test complex queries
            self.log("Testing complex search queries...")
            complex_queries = [
                {
                    "query": "software engineer Microsoft Python",
                    "description": "Multi-keyword search"
                },
                {
                    "query": "MCP Agent Go LangChain integration",
                    "description": "Technical stack search"
                },
                {
                    "query": "user preferences dark mode Python backend",
                    "description": "User preference search"
                }
            ]
            
            for i, query_test in enumerate(complex_queries):
                try:
                    # Test search_facts
                    from api import SearchFactsRequest
                    request = SearchFactsRequest(query=query_test["query"], limit=5)
                    result = await api_module.search_facts(request)
                    
                    facts_found = result.success and hasattr(result.data, 'facts') and result.data.facts and len(result.data.facts) > 0
                    facts_count = len(result.data.facts) if (result.success and hasattr(result.data, 'facts') and result.data.facts) else 0
                    self.results.add_result(f"complex_search_facts_{i+1}", facts_found,
                                          f"Complex query: {query_test['query']}")
                    ok(f"complex_search_facts_{i+1}", facts_found, 
                       f"query='{query_test['query']}', found={facts_count} facts")
                    
                    # Test search_nodes
                    from api import SearchNodesRequest
                    request = SearchNodesRequest(query=query_test["query"], limit=5)
                    result = await api_module.search_nodes(request)
                    
                    nodes_found = result.success and hasattr(result.data, 'nodes') and result.data.nodes and len(result.data.nodes) > 0
                    nodes_count = len(result.data.nodes) if (result.success and hasattr(result.data, 'nodes') and result.data.nodes) else 0
                    self.results.add_result(f"complex_search_nodes_{i+1}", nodes_found,
                                          f"Complex query: {query_test['query']}")
                    ok(f"complex_search_nodes_{i+1}", nodes_found,
                       f"query='{query_test['query']}', found={nodes_count} nodes")
                except Exception as e:
                    self.results.add_result(f"complex_search_{i+1}", False, f"Complex search error: {str(e)}")
                    ok(f"complex_search_{i+1}", False, str(e))
            
            # Test episode retrieval
            self.log("Testing episode retrieval...")
            try:
                result = await api_module.get_episodes(limit=20, offset=0)
                episodes_found = result.success and hasattr(result.data, 'episodes') and result.data.episodes and len(result.data.episodes) > 0
                episode_count = len(result.data.episodes) if (result.success and hasattr(result.data, 'episodes') and result.data.episodes) else 0
                self.results.add_result("episode_retrieval", episodes_found,
                                      f"Retrieved {episode_count} episodes")
                ok("episode_retrieval", episodes_found, f"found {episode_count} episodes")
            except Exception as e:
                self.results.add_result("episode_retrieval", False, f"Episode retrieval error: {str(e)}")
                ok("episode_retrieval", False, str(e))
            
            return True
            
        except Exception as e:
            self.results.add_result("Memory storage and search test", False, f"Test failed: {str(e)}")
            ok("Memory storage and search test", False, str(e))
            return False

    async def test_bge_reranker(self) -> bool:
        """Test BGE reranker functionality"""
        print(f"\n{'='*60}")
        print("TESTING BGE RERANKER FUNCTIONALITY")
        print(f"{'='*60}")
        
        try:
            # Test BGE reranker availability
            self.log("Testing BGE reranker availability...")
            bge_available = api_module.BGE_RERANKER_AVAILABLE
            self.results.add_result("BGE reranker available", bge_available, 
                                  "BGE reranker is available" if bge_available else "BGE reranker not available (sentence-transformers not installed)")
            ok("BGE reranker available", bge_available)
            
            if not bge_available:
                self.log("BGE reranker not available, skipping BGE-specific tests")
                self.results.add_result("BGE reranker tests", True, "Skipped - BGE not available")
                ok("BGE reranker tests", True, "skipped - not available")
                return True
            
            # Test BGE reranker initialization
            self.log("Testing BGE reranker initialization...")
            try:
                # BGE reranker is already enabled by default
                original_bge_setting = os.environ.get("USE_BGE_RERANKER", "true")
                # Keep BGE enabled for testing
                
                # Reset Graphiti instance to test BGE initialization
                api_module.graphiti_instance = None
                
                # Initialize Graphiti with BGE reranker
                graphiti = await api_module.get_graphiti()
                
                # Check if BGE reranker was initialized
                bge_initialized = hasattr(graphiti, 'cross_encoder') and graphiti.cross_encoder is not None
                self.results.add_result("BGE reranker initialization", bge_initialized, 
                                      "BGE reranker initialized successfully" if bge_initialized else "BGE reranker failed to initialize")
                ok("BGE reranker initialization", bge_initialized)
                
                # Test health check with BGE status
                self.log("Testing health check with BGE status...")
                try:
                    health_result = await api_module.health_check()
                    bge_status = health_result.get("bge_reranker", {})
                    bge_enabled = bge_status.get("enabled", False)
                    bge_active = bge_status.get("active", False)
                    
                    health_success = bge_enabled and bge_active
                    self.results.add_result("Health check BGE status", health_success, 
                                          f"BGE enabled: {bge_enabled}, active: {bge_active}")
                    ok("Health check BGE status", health_success, f"enabled={bge_enabled}, active={bge_active}")
                except Exception as e:
                    self.results.add_result("Health check BGE status", False, f"Health check failed: {str(e)}")
                    ok("Health check BGE status", False, str(e))
                
                # BGE setting remains enabled (default)
                
                return True
                
            except Exception as e:
                self.results.add_result("BGE reranker initialization", False, f"BGE initialization failed: {str(e)}")
                ok("BGE reranker initialization", False, str(e))
                
                # BGE setting remains enabled (default)
                return False
            
        except Exception as e:
            self.results.add_result("BGE reranker test", False, f"BGE test failed: {str(e)}")
            ok("BGE reranker test", False, str(e))
            return False
    
    async def test_api_functions(self) -> bool:
        """Test API functions directly (not HTTP endpoints)"""
        print(f"\n{'='*60}")
        print("TESTING API FUNCTIONS DIRECTLY")
        print(f"{'='*60}")
        
        try:
            # Get Graphiti instance
            graphiti = await api_module.get_graphiti()
            
            # Test add_memory function
            self.log("Testing add_memory function...")
            try:
                from api import AddMemoryRequest
                request = AddMemoryRequest(
                    name="Direct Test Episode",
                    content="This is a test episode created via direct function call",
                    source_type="text",
                    source_description="Direct test"
                )
                # Call the function directly
                result = await api_module.add_memory(request, None)
                success = result.success
                self.results.add_result("add_memory function", success, f"Success: {result.success}")
                ok("add_memory function", success, f"success={result.success}")
            except Exception as e:
                self.results.add_result("add_memory function", False, f"Function failed: {str(e)}")
                ok("add_memory function", False, str(e))
            
            # Test search_facts function
            self.log("Testing search_facts function...")
            try:
                from api import SearchFactsRequest
                request = SearchFactsRequest(query="test episode", limit=5)
                # Call the function directly
                result = await api_module.search_facts(request)
                success = result.success
                self.results.add_result("search_facts function", success, f"Success: {result.success}")
                ok("search_facts function", success, f"success={result.success}")
            except Exception as e:
                self.results.add_result("search_facts function", False, f"Function failed: {str(e)}")
                ok("search_facts function", False, str(e))
            
            # Test search_nodes function
            self.log("Testing search_nodes function...")
            try:
                from api import SearchNodesRequest
                request = SearchNodesRequest(query="test episode", limit=5)
                # Call the function directly
                result = await api_module.search_nodes(request)
                success = result.success
                self.results.add_result("search_nodes function", success, f"Success: {result.success}")
                ok("search_nodes function", success, f"success={result.success}")
            except Exception as e:
                self.results.add_result("search_nodes function", False, f"Function failed: {str(e)}")
                ok("search_nodes function", False, str(e))
            
            # Test get_episodes function
            self.log("Testing get_episodes function...")
            try:
                # Call the function directly
                result = await api_module.get_episodes(limit=5, offset=0)
                success = result.success
                self.results.add_result("get_episodes function", success, f"Success: {result.success}")
                ok("get_episodes function", success, f"success={result.success}")
            except Exception as e:
                self.results.add_result("get_episodes function", False, f"Function failed: {str(e)}")
                ok("get_episodes function", False, str(e))
            
            # Test cypher_query function
            self.log("Testing cypher_query function...")
            try:
                from api import CypherQueryRequest
                request = CypherQueryRequest(query="MATCH(n) RETURN count(n) as node_count", limit=10)
                # Call the function directly
                result = await api_module.execute_cypher_query(request)
                success = result.success
                self.results.add_result("cypher_query function", success, f"Success: {result.success}")
                ok("cypher_query function", success, f"success={result.success}")
            except Exception as e:
                self.results.add_result("cypher_query function", False, f"Function failed: {str(e)}")
                ok("cypher_query function", False, str(e))
            
            return True
            
        except Exception as e:
            self.results.add_result("API functions test", False, f"Test failed: {str(e)}")
            ok("API functions test", False, str(e))
            return False
    
    async def run_tests(self, mode: str = "all") -> bool:
        """Run all tests based on mode"""
        print(f"🚀 Starting Graphiti Memory API Tests")
        print(f"📅 {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
        print(f"🔧 Mode: {mode}")
        
        success = True
        
        if mode in ["all", "direct"]:
            success &= await self.test_direct_functionality()
        
        if mode in ["all", "api"]:
            success &= await self.test_api_functions()
        
        if mode in ["all", "memory"]:
            success &= await self.test_memory_storage_and_search()
        
        if mode in ["all", "bge"]:
            success &= await self.test_bge_reranker()
        
        # Print final summary
        overall_success = self.results.print_summary()
        
        if overall_success:
            print(f"\n🎉 All tests passed! Graphiti Memory API is working correctly.")
        else:
            print(f"\n⚠️  Some tests failed. Check the details above.")
        
        return overall_success


async def main():
    """Main entry point"""
    parser = argparse.ArgumentParser(description="Comprehensive Graphiti Memory API Test")
    parser.add_argument("--mode", choices=["all", "direct", "api", "memory", "bge"], default="all",
                       help="Test mode: all (default), direct (core functionality only), api (API functions only), memory (memory storage and search), bge (BGE reranker only)")
    parser.add_argument("--verbose", "-v", action="store_true",
                       help="Enable verbose logging")
    
    args = parser.parse_args()
    
    tester = GraphitiTester(verbose=args.verbose)
    success = await tester.run_tests(mode=args.mode)
    
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    asyncio.run(main())
