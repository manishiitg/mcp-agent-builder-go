#!/usr/bin/env python3
"""
Graphiti Knowledge Graph API
FastAPI-based service for memory management with Graphiti
"""

import asyncio
import logging
import os
from datetime import datetime, timezone
from typing import List, Optional, Dict, Any
from uuid import UUID

from fastapi import FastAPI, HTTPException, Depends, BackgroundTasks
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field
from dotenv import load_dotenv

from graphiti_core import Graphiti
from graphiti_core.nodes import EpisodeType, EpisodicNode
try:
    from graphiti_core.search.search_config_recipes import (
        EDGE_HYBRID_SEARCH_RRF,
        NODE_HYBRID_SEARCH_RRF,
        SearchConfig
    )
    SearchConfigRecipes = {
        'EDGE_HYBRID_SEARCH_RRF': EDGE_HYBRID_SEARCH_RRF,
        'NODE_HYBRID_SEARCH_RRF': NODE_HYBRID_SEARCH_RRF,
        'SearchConfig': SearchConfig
    }
except Exception:
    SearchConfigRecipes = None  # Older graphiti-core versions may not include recipes
from graphiti_core.driver.neo4j_driver import Neo4jDriver
from graphiti_core.llm_client.config import LLMConfig
from graphiti_core.llm_client.openai_generic_client import OpenAIGenericClient
from graphiti_core.embedder.openai import OpenAIEmbedder, OpenAIEmbedderConfig

# BGE Reranker imports (optional)
try:
    from graphiti_core.cross_encoder.bge_reranker_client import BGERerankerClient
    BGE_RERANKER_AVAILABLE = True
except ImportError:
    BGE_RERANKER_AVAILABLE = False
    BGERerankerClient = None

# Load environment variables
load_dotenv()

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Pydantic models for API requests/responses
class AddMemoryRequest(BaseModel):
    name: str = Field(..., description="Name/title of the episode")
    content: str = Field(..., description="Content to store in the knowledge graph")
    source_type: str = Field(default="text", description="Source type: 'text' or 'json'")
    source_description: Optional[str] = Field(None, description="Description of the source")
    reference_time: Optional[datetime] = Field(None, description="Reference time for the episode")

class SearchFactsRequest(BaseModel):
    query: str = Field(..., description="Search query for facts and relationships")
    limit: int = Field(default=10, description="Maximum number of results to return")
    center_node_uuid: Optional[str] = Field(None, description="Center node UUID for contextual search")

class SearchNodesRequest(BaseModel):
    query: str = Field(..., description="Search query for entity summaries")
    limit: int = Field(default=10, description="Maximum number of results to return")

class EpisodeResponse(BaseModel):
    uuid: str
    name: str
    content: str
    source_type: str
    source_description: Optional[str]
    reference_time: datetime
    created_at: datetime

class FactResponse(BaseModel):
    uuid: str
    fact: str
    valid_from: Optional[datetime]
    valid_until: Optional[datetime]
    source_node_uuid: str

class NodeResponse(BaseModel):
    uuid: str
    name: str
    content_summary: str
    labels: List[str]
    created_at: datetime

class DeleteEpisodeRequest(BaseModel):
    episode_uuid: str = Field(..., description="UUID of the episode to delete")

class CypherQueryRequest(BaseModel):
    query: str = Field(..., description="Cypher query to execute")
    limit: Optional[int] = Field(default=100, description="Maximum number of results to return")

class CypherQueryResponse(BaseModel):
    success: bool
    message: str
    data: Optional[Any] = None
    query: str
    execution_time_ms: Optional[float] = None

class APIResponse(BaseModel):
    success: bool
    message: str
    data: Optional[Any] = None

# Global Graphiti instance and Neo4j driver
graphiti_instance: Optional[Graphiti] = None
neo4j_driver_instance: Optional[Neo4jDriver] = None

def clean_embedding_data(value: Any) -> Any:
    """Recursively clean embedding data from any value"""
    # Utility: recursively strip embeddings and heavy fields
    def is_large_float_list(val: Any) -> bool:
        try:
            return (
                isinstance(val, list)
                and len(val) >= 32
                and all(isinstance(v, (int, float)) for v in val[:64])
            )
        except Exception:
            return False

    def should_drop_key(key: str) -> bool:
        lowered = key.lower()
        return (
            'embedding' in lowered
            or lowered in {'name_embedding', 'fact_embedding'}
        )

    def clean_value(val: Any) -> Any:
        # Drop obvious embedding arrays
        if is_large_float_list(val):
            return None
        # Dicts: clean keys recursively
        if isinstance(val, dict):
            cleaned: Dict[str, Any] = {}
            for k, v in val.items():
                if should_drop_key(k):
                    continue
                # Skip very heavy attributes collections
                if k in {'attributes'}:
                    continue
                cv = clean_value(v)
                if cv is not None:
                    cleaned[k] = cv
            return cleaned
        # Objects with __dict__
        if hasattr(val, '__dict__'):
            cleaned: Dict[str, Any] = {}
            for k, v in val.__dict__.items():
                if should_drop_key(k) or k in {'attributes'}:
                    continue
                cv = clean_value(v)
                if cv is not None:
                    cleaned[k] = cv
            return cleaned
        # Lists/Tuples
        if isinstance(val, (list, tuple)):
            cleaned_list = []
            for item in val:
                ci = clean_value(item)
                if ci is not None:
                    cleaned_list.append(ci)
            return cleaned_list
        return val

    return clean_value(value)

async def get_graphiti() -> Graphiti:
    """Get or create Graphiti instance"""
    global graphiti_instance, neo4j_driver_instance
    
    if graphiti_instance is None:
        logger.info("Initializing Graphiti instance...")
        
        # Initialize Neo4j AuraDB driver
        neo4j_uri = os.getenv('NEO4J_URI')
        neo4j_user = os.getenv('NEO4J_USERNAME')
        neo4j_password = os.getenv('NEO4J_PASSWORD')
        neo4j_database = os.getenv('NEO4J_DATABASE')
        
        # Validate required environment variables
        if not all([neo4j_uri, neo4j_user, neo4j_password, neo4j_database]):
            raise ValueError("Missing required Neo4j environment variables. Please set NEO4J_URI, NEO4J_USERNAME, NEO4J_PASSWORD, and NEO4J_DATABASE")
        
        logger.info(f"Connecting to Neo4j AuraDB: {neo4j_uri}")
        neo4j_driver = Neo4jDriver(
            uri=neo4j_uri,
            user=neo4j_user,
            password=neo4j_password,
            database=neo4j_database
        )
        neo4j_driver_instance = neo4j_driver  # Store globally for Cypher queries
        
        logger.info("Neo4j AuraDB driver initialized successfully")
        
        # Create OpenRouter LLM client
        llm_config = LLMConfig(
            api_key=os.getenv('OPENROUTER_API_KEY'),
            model=os.getenv('OPENROUTER_LLM_MODEL', 'openai/gpt-4.1-mini'),
            small_model=os.getenv('OPENROUTER_SMALL_MODEL', 'openai/gpt-4.1-mini'),
            base_url=os.getenv('OPENROUTER_BASE_URL', 'https://openrouter.ai/api/v1'),
        )
        
        llm_client = OpenAIGenericClient(config=llm_config)
        
        # Create Ollama embedding client
        embedder = OpenAIEmbedder(
            config=OpenAIEmbedderConfig(
                api_key=os.getenv('OLLAMA_API_KEY', 'ollama'),
                embedding_model=os.getenv('OLLAMA_EMBEDDING_MODEL', 'mxbai-embed-large'),
                embedding_dim=int(os.getenv('OLLAMA_EMBEDDING_DIM', '1024')),
                base_url=os.getenv('OLLAMA_BASE_URL', 'http://localhost:11434/v1'),
            )
        )
        
        # Configure BGE Reranker (default enabled)
        cross_encoder = None
        if os.getenv('USE_BGE_RERANKER', 'true').lower() == 'true' and BGE_RERANKER_AVAILABLE:
            try:
                logger.info("Initializing BGE Reranker for local cross-encoder reranking...")
                # Set HF_HOME to our custom models directory for BGE reranker
                bge_cache_dir = os.getenv('BGE_MODEL_CACHE_DIR', '/app/models/bge-reranker')
                os.environ['HF_HOME'] = bge_cache_dir
                logger.info(f"Set HF_HOME to {bge_cache_dir} for BGE reranker")
                cross_encoder = BGERerankerClient()
                logger.info("BGE Reranker initialized successfully")
            except Exception as e:
                logger.warning(f"Failed to initialize BGE Reranker: {e}. Continuing without reranker.")
                cross_encoder = None
        elif os.getenv('USE_BGE_RERANKER', 'true').lower() == 'true' and not BGE_RERANKER_AVAILABLE:
            logger.warning("BGE Reranker requested but not available. Install sentence-transformers to enable.")
        else:
            logger.info("Using OpenRouter reranking (BGE Reranker disabled via USE_BGE_RERANKER=false)")
        
        # Initialize Graphiti with Neo4j driver
        graphiti_instance = Graphiti(
            graph_driver=neo4j_driver,
            llm_client=llm_client,
            embedder=embedder,
            cross_encoder=cross_encoder,
        )
        
        # Initialize the graph database with graphiti's indices. This only needs to be done once.
        logger.info("Initializing Graphiti indices and constraints...")
        await graphiti_instance.build_indices_and_constraints()
        
        # Note: Kuzu DB doesn't support FTS indexes in the same way as other databases
        # Graphiti will handle text search through its own mechanisms
        
        logger.info("Graphiti instance initialized successfully")
    
    return graphiti_instance

# FastAPI app
app = FastAPI(
    title="Graphiti Knowledge Graph API",
    description="API for managing knowledge graphs with Graphiti",
    version="1.0.0"
)

# Add CORS middleware
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],  # Configure appropriately for production
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)

@app.on_event("startup")
async def startup_event():
    """Initialize Graphiti on startup"""
    try:
        await get_graphiti()
        logger.info("API startup completed successfully")
    except Exception as e:
        logger.error(f"Failed to initialize Graphiti: {e}")
        raise

@app.get("/")
async def root():
    """Root endpoint with API information"""
    return {
        "message": "Graphiti Knowledge Graph API",
        "version": "1.0.0",
        "endpoints": {
            "add_memory": "POST /add_memory",
            "search_facts": "POST /search_facts", 
            "search_nodes": "POST /search_nodes",
            "get_episodes": "GET /episodes",
            "delete_episode": "POST /delete_episode",
            "cypher_query": "POST /cypher_query",
            "health": "GET /health"
        }
    }

@app.get("/health")
async def health_check():
    """Health check endpoint"""
    try:
        graphiti = await get_graphiti()
        bge_reranker_enabled = os.getenv('USE_BGE_RERANKER', 'true').lower() == 'true'
        bge_reranker_available = BGE_RERANKER_AVAILABLE
        
        return {
            "status": "healthy", 
            "graphiti_initialized": graphiti is not None,
            "bge_reranker": {
                "enabled": bge_reranker_enabled,
                "available": bge_reranker_available,
                "active": bge_reranker_enabled and bge_reranker_available
            }
        }
    except Exception as e:
        return {"status": "unhealthy", "error": str(e)}

@app.post("/add_memory", response_model=APIResponse)
async def add_memory(request: AddMemoryRequest, background_tasks: BackgroundTasks):
    """Store episodes and interactions in the knowledge graph"""
    try:
        graphiti = await get_graphiti()
        
        # Convert source_type string to EpisodeType enum
        source_type = EpisodeType.text if request.source_type.lower() == "text" else EpisodeType.json
        
        # Normalize reference_time to naive UTC to avoid aware/naive compare errors
        if request.reference_time is not None:
            rt = request.reference_time
            if rt.tzinfo is not None:
                # Convert aware datetime to naive UTC
                rt = rt.astimezone(timezone.utc).replace(tzinfo=None)
            else:
                # Already naive, ensure it's treated as UTC
                rt = rt.replace(tzinfo=None)
            reference_time = rt
        else:
            # Use a simple, guaranteed naive datetime to avoid any timezone issues
            # This ensures we don't have any timezone-related problems
            reference_time = datetime(2024, 1, 1, 12, 0, 0)  # Simple naive datetime
        
        logger.info(f"Adding episode: {request.name}")
        logger.debug(f"Using reference_time: {reference_time} (type: {type(reference_time)}, tzinfo: {getattr(reference_time, 'tzinfo', 'N/A')})")
        
        try:
            # Add episode to Graphiti
            episode_uuid = await graphiti.add_episode(
                name=request.name,
                episode_body=request.content,
                source=source_type,
                source_description=request.source_description,
                reference_time=reference_time
            )
        except Exception as e:
            error_msg = str(e)
            logger.error(f"Error adding episode: {error_msg}")
            
            if "offset-naive and offset-aware" in error_msg or "timezone" in error_msg.lower():
                logger.error(f"Timezone error detected: {error_msg}")
                # Try with a completely naive datetime using a different approach
                try:
                    # Use a very simple naive datetime - same as default
                    reference_time = datetime(2024, 1, 1, 12, 0, 0)  # Simple naive datetime
                    logger.debug(f"Retrying with simple naive datetime: {reference_time}")
                    episode_uuid = await graphiti.add_episode(
                        name=request.name,
                        episode_body=request.content,
                        source=source_type,
                        source_description=request.source_description,
                        reference_time=reference_time
                    )
                    logger.info("Successfully added episode with fallback datetime")
                except Exception as e2:
                    logger.error(f"Fallback also failed: {e2}")
                    # Try one more time with None reference_time if Graphiti supports it
                    try:
                        logger.debug("Trying with None reference_time")
                        episode_uuid = await graphiti.add_episode(
                            name=request.name,
                            episode_body=request.content,
                            source=source_type,
                            source_description=request.source_description
                            # No reference_time parameter
                        )
                        logger.info("Successfully added episode without reference_time")
                    except Exception as e3:
                        logger.error(f"Final fallback also failed: {e3}")
                        raise e  # Raise original error
            else:
                raise
        
        logger.info(f"Successfully added episode: {episode_uuid}")
        
        # Ensure response only returns plain UUID, not full object repr
        try:
            if hasattr(episode_uuid, 'uuid'):
                episode_uuid_str = str(getattr(episode_uuid, 'uuid'))
            else:
                # If it's a UUID already, str() is fine; if it's a complex object string, fallback to getattr
                candidate = str(episode_uuid)
                # Best-effort: if it contains "uuid='...'", extract inner value
                if "uuid='" in candidate:
                    import re
                    match = re.search(r"uuid='([0-9a-fA-F-]+)'", candidate)
                    episode_uuid_str = match.group(1) if match else candidate
                else:
                    episode_uuid_str = candidate
        except Exception:
            episode_uuid_str = str(episode_uuid)
        
        return APIResponse(
            success=True,
            message=f"Episode '{request.name}' added successfully",
            data={"episode_uuid": episode_uuid_str}
        )
        
    except Exception as e:
        logger.error(f"Error adding episode: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to add episode: {str(e)}")

@app.post("/search_facts", response_model=APIResponse)
async def search_facts(request: SearchFactsRequest):
    """Find relevant facts and relationships"""
    try:
        graphiti = await get_graphiti()
        
        logger.info(f"Searching facts for query: {request.query}")
        
        # Use proper search configuration with search recipes
        results = []
        if SearchConfigRecipes is not None:
            try:
                # Use EDGE_HYBRID_SEARCH_RRF for fact/edge search
                config = SearchConfigRecipes['EDGE_HYBRID_SEARCH_RRF']
                search_results = await graphiti._search(request.query, config)
                
                # Extract edges from search results
                if hasattr(search_results, 'edges'):
                    results = search_results.edges
                elif hasattr(search_results, 'results'):
                    results = search_results.results
                else:
                    results = []
                    
                # Apply center node filtering if specified
                if request.center_node_uuid:
                    center_uuid = UUID(request.center_node_uuid)
                    # Filter results to those connected to the center node
                    filtered_results = []
                    for result in results:
                        if (hasattr(result, 'source_node_uuid') and str(result.source_node_uuid) == str(center_uuid)) or \
                           (hasattr(result, 'target_node_uuid') and str(result.target_node_uuid) == str(center_uuid)):
                            filtered_results.append(result)
                    results = filtered_results
                    
            except Exception as e:
                logger.warning(f"Search recipe failed, falling back to basic search: {e}")
                # Fallback to basic search if recipes fail
                if request.center_node_uuid:
                    center_uuid = UUID(request.center_node_uuid)
                    results = await graphiti.search(
                        request.query,
                        center_node_uuid=center_uuid
                    )
                else:
                    results = await graphiti.search(request.query)
        else:
            logger.warning("SearchConfigRecipes not available, using basic search")
            # Fallback to basic search if recipes not available
            if request.center_node_uuid:
                center_uuid = UUID(request.center_node_uuid)
                results = await graphiti.search(
                    request.query,
                    center_node_uuid=center_uuid
                )
            else:
                results = await graphiti.search(request.query)
        
        # Limit results after getting them from Graphiti
        if request.limit and len(results) > request.limit:
            results = results[:request.limit]
        
        # Convert results to response format
        facts = []
        for result in results:
            # Handle both EntityEdge and other result types
            if hasattr(result, 'fact'):
                # Clean any embedding data from the result
                cleaned_result = clean_embedding_data(result)
                
                # Handle both object and dictionary formats
                if isinstance(cleaned_result, dict):
                    facts.append(FactResponse(
                        uuid=str(cleaned_result.get('uuid', '')),
                        fact=cleaned_result.get('fact', ''),
                        valid_from=cleaned_result.get('valid_from'),
                        valid_until=cleaned_result.get('valid_until'),
                        source_node_uuid=str(cleaned_result.get('source_node_uuid', ''))
                    ))
                else:
                    facts.append(FactResponse(
                        uuid=str(cleaned_result.uuid),
                        fact=cleaned_result.fact,
                        valid_from=getattr(cleaned_result, 'valid_from', None),
                        valid_until=getattr(cleaned_result, 'valid_until', None),
                        source_node_uuid=str(getattr(cleaned_result, 'source_node_uuid', ''))
                    ))
        
        logger.info(f"Found {len(facts)} facts for query: {request.query}")
        
        return APIResponse(
            success=True,
            message=f"Found {len(facts)} facts",
            data={"facts": facts, "query": request.query}
        )
        
    except Exception as e:
        logger.error(f"Error searching facts: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to search facts: {str(e)}")

@app.post("/search_nodes", response_model=APIResponse)
async def search_nodes(request: SearchNodesRequest):
    """Search for entity summaries and information"""
    try:
        graphiti = await get_graphiti()
        
        logger.info(f"Searching nodes for query: {request.query}")
        
        # Perform node search using the configurable _search with a node recipe.
        node_results = []
        if SearchConfigRecipes is not None:
            try:
                config = SearchConfigRecipes['NODE_HYBRID_SEARCH_RRF']
                search_results = await graphiti._search(request.query, config)
                node_results = getattr(search_results, 'nodes', [])
            except Exception as e:
                logger.warning(f"_search with recipe failed, returning empty nodes: {e}")
        else:
            logger.warning("SearchConfigRecipes unavailable in this graphiti-core version; returning empty nodes")

        # Limit results
        if request.limit and len(node_results) > request.limit:
            node_results = node_results[:request.limit]

        # Convert results to response format
        nodes = []
        for result in node_results:
            try:
                # Extract attributes before cleaning (to preserve structure)
                node_uuid = str(getattr(result, 'uuid', ''))
                node_name = getattr(result, 'name', 'Unknown')
                node_summary = getattr(result, 'content_summary', getattr(result, 'summary', ''))
                node_labels = getattr(result, 'labels', [])
                node_created = getattr(result, 'created_at', datetime.now(timezone.utc).replace(tzinfo=None))
                
                nodes.append(NodeResponse(
                    uuid=node_uuid,
                    name=node_name,
                    content_summary=node_summary,
                    labels=node_labels,
                    created_at=node_created,
                ))
            except Exception as e:
                # Skip malformed entries but log the error
                logger.warning(f"Skipping malformed node entry: {e}")
                continue
        
        logger.info(f"Found {len(nodes)} nodes for query: {request.query}")
        
        return APIResponse(
            success=True,
            message=f"Found {len(nodes)} nodes",
            data={"nodes": nodes, "query": request.query}
        )
        
    except Exception as e:
        logger.error(f"Error searching nodes: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to search nodes: {str(e)}")

@app.get("/episodes", response_model=APIResponse)
async def get_episodes(limit: int = 20, offset: int = 0):
    """Retrieve recent episodes via Graphiti search (no raw queries)."""
    try:
        graphiti = await get_graphiti()

        logger.info(f"Retrieving episodes via search: limit={limit}, offset={offset}")

        # Use a node search recipe to fetch recent nodes; Graphiti doesn't expose list episodes in some versions
        node_results = []
        if SearchConfigRecipes is not None:
            try:
                config = SearchConfigRecipes['NODE_HYBRID_SEARCH_RRF']
                search_results = await graphiti._search("", config)  # empty query to get recent nodes
                node_results = getattr(search_results, 'nodes', [])
            except Exception as e:
                logger.warning(f"Episode listing via _search failed: {e}")
        else:
            logger.warning("SearchConfigRecipes unavailable; cannot enumerate episodes via _search")

        # Filter to episodic nodes if label information is present
        episodic_nodes = []
        for n in node_results:
            labels = getattr(n, 'labels', []) or []
            if any(label.lower().startswith("episodic") for label in labels) or 'EpisodicNode' in type(n).__name__:
                episodic_nodes.append(n)

        # Apply offset/limit
        slicer = episodic_nodes[offset: offset + limit if limit else None]

        # Map to response
        episode_list = []
        for ep in slicer:
            try:
                # Clean any embedding data from the episode
                cleaned_ep = clean_embedding_data(ep)
                episode_list.append(EpisodeResponse(
                    uuid=str(getattr(cleaned_ep, 'uuid', '')),
                    name=getattr(cleaned_ep, 'name', ''),
                    content=getattr(cleaned_ep, 'episode_body', ''),
                    source_type=getattr(getattr(cleaned_ep, 'source', EpisodeType.text), 'value', 'text'),
                    source_description=getattr(cleaned_ep, 'source_description', None),
                    reference_time=getattr(cleaned_ep, 'reference_time', datetime.now(timezone.utc).replace(tzinfo=None)),
                    created_at=getattr(cleaned_ep, 'created_at', datetime.now(timezone.utc).replace(tzinfo=None)),
                ))
            except Exception:
                continue

        return APIResponse(
            success=True,
            message=f"Retrieved {len(episode_list)} episodes",
            data={"episodes": episode_list, "total": len(episodic_nodes)}
        )

    except Exception as e:
        logger.error(f"Error retrieving episodes: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to retrieve episodes: {str(e)}")

@app.post("/delete_episode", response_model=APIResponse)
async def delete_episode(request: DeleteEpisodeRequest):
    """Remove episodes via Graphiti CRUD (no raw queries)."""
    try:
        await get_graphiti()
        episode_uuid = str(UUID(request.episode_uuid))
        logger.info(f"Deleting episode via CRUD: {episode_uuid}")

        # Resolve episodic node and delete using Graphiti CRUD
        node = await EpisodicNode.get_by_uuid(neo4j_driver_instance, episode_uuid)
        await node.delete(neo4j_driver_instance)

        return APIResponse(
            success=True,
            message=f"Episode {episode_uuid} deleted successfully",
            data={"deleted_uuid": episode_uuid}
        )
    except Exception as e:
        logger.error(f"Error deleting episode: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to delete episode: {str(e)}")

@app.post("/cypher_query", response_model=CypherQueryResponse)
async def execute_cypher_query(request: CypherQueryRequest):
    """Execute Cypher queries directly against the Neo4j database"""
    import time
    
    try:
        start_time = time.time()
        
        # Get Graphiti instance (this will initialize the driver if needed)
        await get_graphiti()
        
        # Get the Neo4j driver
        neo4j_driver = neo4j_driver_instance
        
        # Execute the Cypher query
        logger.info(f"Executing Cypher query: {request.query}")
        
        # Execute query and get results
        result = await neo4j_driver.execute_query(request.query)
        
        execution_time = (time.time() - start_time) * 1000  # Convert to milliseconds
        
        # Process results
        if result:
            # Convert result to list of dictionaries for JSON serialization
            processed_results = []
            for row in result:
                cleaned_row = clean_embedding_data(row)
                processed_results.append(cleaned_row)
            
            # Apply limit if specified
            if request.limit and len(processed_results) > request.limit:
                processed_results = processed_results[:request.limit]
            
            return CypherQueryResponse(
                success=True,
                message=f"Query executed successfully. Found {len(processed_results)} results.",
                data={
                    "results": processed_results,
                    "total_results": len(processed_results),
                    "limited": request.limit and len(processed_results) == request.limit
                },
                query=request.query,
                execution_time_ms=execution_time
            )
        else:
            return CypherQueryResponse(
                success=True,
                message="Query executed successfully. No results returned.",
                data={"results": [], "total_results": 0},
                query=request.query,
                execution_time_ms=execution_time
            )
        
    except Exception as e:
        logger.error(f"Error executing Cypher query: {e}")
        raise HTTPException(
            status_code=400, 
            detail=f"Failed to execute Cypher query: {str(e)}"
        )

if __name__ == "__main__":
    import uvicorn
    
    # Get configuration from environment
    host = os.getenv("API_HOST", "0.0.0.0")
    port = int(os.getenv("API_PORT", "8000"))
    debug = os.getenv("API_DEBUG", "false").lower() == "true"
    
    logger.info(f"Starting Graphiti API server on {host}:{port}")
    
    uvicorn.run(
        "api:app",
        host=host,
        port=port,
        reload=debug,
        log_level="info"
    )
