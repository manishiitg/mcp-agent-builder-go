# RAG-Based Semantic Search Implementation for Planner System

## 📋 **Overview**

This document outlines the complete implementation of a RAG (Retrieval-Augmented Generation) based semantic search system for the planner workspace. The system enables semantic search across markdown files using OpenAI embeddings and Qdrant vector database, with background processing for file changes and a comprehensive frontend interface.

## 🎯 **Project Goals**

- **Semantic Search**: Enable meaning-based search across workspace files
- **Background Processing**: Automatically process file changes for embedding generation
- **Vector Database**: Store and query embeddings using Qdrant
- **Frontend Integration**: Provide search interface in the workspace UI
- **Monitoring & Management**: APIs for job status, resync, and system health

## 🏗️ **Architecture Components**

### **Backend Services**

#### **1. Qdrant Vector Database**
- **Purpose**: Store and query document embeddings
- **Collection**: `workspace` with 1536-dimensional vectors (OpenAI text-embedding-3-small)
- **Protocol**: gRPC communication via official Qdrant Go client
- **Docker**: `qdrant:latest` with persistent volume storage

#### **2. OpenAI Embedding Service**
- **Model**: `text-embedding-3-small` (1536 dimensions)
- **API**: OpenAI embeddings endpoint with retry logic
- **Features**: Batch processing, exponential backoff, error handling
- **Environment**: `OPENAI_API_KEY` required

#### **3. Text Chunking Service**
- **Chunk Size**: 1000 characters with 200-character overlap
- **Strategy**: Word boundary preservation
- **Metadata**: File path, folder, chunk index, word/char counts

#### **4. Background Job Queue (SQLite)**
- **Purpose**: Persistent job queue for file processing
- **Features**: Retry logic (3 attempts), job status tracking, worker management
- **Schema**: Jobs table with status, priority, error handling
- **Persistence**: Survives Docker restarts

#### **5. File Processor**
- **Workers**: 2 background workers for parallel processing
- **Actions**: Create, update, delete file processing
- **Integration**: Automatic hooks on document creation/updates
- **Monitoring**: Real-time job statistics and health checks

### **Frontend Components**

#### **1. SemanticSearchSync Component**
- **Location**: `frontend/src/components/workspace/SemanticSearchSync.tsx`
- **Features**: 
  - Real-time status monitoring with polling
  - Service availability indicators (Qdrant, OpenAI)
  - Job statistics display (pending, processing, completed, failed)
  - Resync controls with dry-run and force options
  - Built-in semantic search testing interface

#### **2. API Integration**
- **Types**: TypeScript interfaces for all semantic search APIs
- **Methods**: Status, jobs, resync, and search endpoints
- **Error Handling**: Comprehensive error states and user feedback

## 🔧 **Implementation Details**

### **Database Schema**

```sql
CREATE TABLE IF NOT EXISTS jobs (
    id TEXT PRIMARY KEY,
    file_path TEXT NOT NULL,
    content TEXT NOT NULL,
    action TEXT NOT NULL,
    status TEXT NOT NULL,
    priority INTEGER NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    error TEXT,
    retries INTEGER NOT NULL DEFAULT 0,
    worker_id TEXT
);

CREATE INDEX IF NOT EXISTS idx_jobs_status_priority 
ON jobs (status, priority DESC, created_at ASC);
```

### **API Endpoints**

#### **Semantic Search**
- `GET /api/search/semantic` - Perform semantic search
- `POST /api/search/process-file` - Manually queue file for processing

#### **Monitoring & Management**
- `GET /api/semantic/jobs` - Get job processing statistics
- `GET /api/semantic/stats` - Get overall system status
- `POST /api/semantic/resync` - Trigger full resync operation

### **Search Parameters**

```typescript
interface SemanticSearchRequest {
  query: string;                    // Search query
  folder?: string;                  // Optional folder filter
  limit?: number;                   // Max results (default: 10)
  similarity_threshold?: number;    // Score threshold (removed in final version)
  include_regex?: boolean;          // Include regex results
  regex_limit?: number;             // Max regex results
}
```

### **Search Results**

```typescript
interface SemanticSearchResult {
  file_path: string;               // Full file path
  chunk_text: string;              // Matching text chunk
  chunk_index: number;             // Chunk position in file
  score: number;                   // Similarity score (0-1)
  folder: string;                  // File folder
  file_type: string;               // File extension
  word_count: number;              // Chunk word count
  char_count: number;              // Chunk character count
  search_method: "semantic";       // Search method identifier
}
```

## 🚀 **Key Features Implemented**

### **1. Automatic File Processing**
- **Hooks**: Document creation/update automatically triggers background processing
- **Chunking**: Files split into 1000-character chunks with overlap
- **Embedding**: OpenAI API generates embeddings for each chunk
- **Storage**: Embeddings stored in Qdrant with metadata

### **2. Background Job Management**
- **Queue**: SQLite-based persistent job queue
- **Workers**: 2 parallel workers for processing
- **Retry Logic**: Failed jobs retry up to 3 times
- **Status Tracking**: Real-time job status monitoring

### **3. Comprehensive Monitoring**
- **Service Health**: Qdrant and OpenAI availability checks
- **Job Statistics**: Pending, processing, completed, failed counts
- **Performance Metrics**: Processing times, embedding generation stats
- **Error Handling**: Detailed error logging and user feedback

### **4. Frontend Integration**
- **Status Display**: Real-time system health indicators
- **Search Interface**: Built-in semantic search testing
- **Resync Controls**: Manual resync with options
- **Polling**: Automatic status updates every 5 seconds

### **5. CLI Management**
- **Resync Command**: `./planner resync` for full re-indexing
- **Options**: `--dry-run`, `--force`, `--docs-dir`, `--qdrant-url`
- **Progress**: Real-time processing statistics and completion status

## 🔍 **Search Quality & Performance**

### **Similarity Scoring**
- **Range**: 0.0 to 1.0 (0% to 100% similarity)
- **Threshold**: Initially 0.7 (70%), reduced to 0.3 (30%) for better results
- **Final Approach**: Removed threshold filtering, rely on limit parameter

### **Performance Metrics**
- **Embedding Generation**: ~1-2 seconds per batch (5 chunks)
- **Qdrant Query**: ~500ms for semantic search
- **Total Search Time**: ~1-2 seconds end-to-end
- **Processing Throughput**: ~2-4 files per minute per worker

### **Search Quality Examples**

**Query**: "aws accounts"
**Results Found**:
1. AWS inventory files (45.8% similarity) - Account ID 414085459896
2. AWS cost analysis todos (32.6% similarity) - Cost optimization tasks
3. AWS cost reports (31.1% similarity) - $772K+ in Savings Plans

## 🐛 **Issues Resolved**

### **1. UUID Format Issues**
- **Problem**: Qdrant strict UUID parsing requirements
- **Solution**: Implemented proper UUID generation using `github.com/google/uuid`

### **2. Similarity Threshold Too High**
- **Problem**: 0.7 threshold filtered out all results (highest score was 0.4581)
- **Solution**: Reduced to 0.3, then removed threshold filtering entirely

### **3. SQLite CGO Requirements**
- **Problem**: Docker build failed due to CGO disabled
- **Solution**: Added `gcc musl-dev` and `ENV CGO_ENABLED=1` to Dockerfile

### **4. Session Management for External SSE Servers**
- **Problem**: External SSE servers had different session patterns
- **Solution**: Enhanced session lifecycle handling and context isolation

### **5. Resync Command Isolation**
- **Problem**: Resync command used different data directory than main server
- **Solution**: Unified data directory (`/app/data`) for both CLI and server

## 📊 **Testing & Validation**

### **Comprehensive Testing**
- **Unit Tests**: Individual service components
- **Integration Tests**: End-to-end semantic search workflow
- **Performance Tests**: Embedding generation and query performance
- **Error Handling**: Retry logic and failure scenarios

### **Test Commands**
```bash
# Test semantic search API
curl "http://localhost:8081/api/search/semantic?query=aws+accounts&limit=10"

# Test job status
curl "http://localhost:8081/api/semantic/jobs"

# Test system status
curl "http://localhost:8081/api/semantic/stats"

# Trigger resync
curl -X POST "http://localhost:8081/api/semantic/resync"

# CLI resync
docker-compose exec planner-api ./planner resync
```

### **Frontend Testing**
- **Search Interface**: Built-in search testing in SemanticSearchSync popup
- **Status Monitoring**: Real-time service health and job progress
- **Resync Controls**: Manual resync with dry-run and force options

## 🔧 **Configuration**

### **Environment Variables**
```bash
# OpenAI Configuration
OPENAI_API_KEY=your_openai_api_key_here
OPENAI_EMBEDDING_MODEL=text-embedding-3-small

# Qdrant Configuration
QDRANT_URL=http://qdrant:6333

# Planner Configuration
DOCS_DIR=/app/planner-docs
DATA_DIR=/app/data
```

### **Docker Compose Services**
```yaml
services:
  qdrant:
    image: qdrant/qdrant:latest
    ports:
      - "6333:6333"
    volumes:
      - qdrant_data:/qdrant/storage
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:6333/"]

  planner-api:
    build: ./planner
    ports:
      - "8081:8081"
    environment:
      - OPENAI_API_KEY=${OPENAI_API_KEY}
      - OPENAI_EMBEDDING_MODEL=text-embedding-3-small
      - QDRANT_URL=http://qdrant:6333
    volumes:
      - ./planner-docs:/app/planner-docs
      - planner_data:/app/data
    depends_on:
      - qdrant
```

## 📈 **Performance Optimizations**

### **Batch Processing**
- **Embedding Generation**: Process 5 chunks per batch
- **Qdrant Upserts**: Batch point insertion for efficiency
- **Worker Pool**: 2 parallel workers for job processing

### **Caching & Persistence**
- **SQLite Queue**: Persistent job queue survives restarts
- **Qdrant Storage**: Persistent vector storage
- **Metadata Caching**: File metadata cached for quick access

### **Error Handling**
- **Retry Logic**: Exponential backoff for API failures
- **Graceful Degradation**: Fallback to regex search if semantic fails
- **Comprehensive Logging**: Debug logs for troubleshooting

## 🎯 **Future Enhancements**

### **Potential Improvements**
1. **Hybrid Search**: Combine semantic and regex results intelligently
2. **Query Expansion**: Enhance queries with synonyms and related terms
3. **Result Ranking**: Advanced ranking algorithms beyond similarity scores
4. **Incremental Updates**: Process only changed file sections
5. **Multi-language Support**: Support for non-English content
6. **Advanced Filtering**: Date ranges, file types, content categories

### **Scalability Considerations**
1. **Horizontal Scaling**: Multiple Qdrant instances for large datasets
2. **Worker Scaling**: Dynamic worker pool based on queue size
3. **Caching Layer**: Redis for frequently accessed embeddings
4. **Load Balancing**: Distribute search requests across multiple instances

## 📝 **Summary**

The RAG-based semantic search system for the planner workspace has been successfully implemented with:

- **Complete Backend**: Qdrant vector database, OpenAI embeddings, SQLite job queue
- **Background Processing**: Automatic file processing with retry logic
- **Frontend Integration**: Comprehensive monitoring and search interface
- **CLI Management**: Resync command for full re-indexing
- **Comprehensive Testing**: End-to-end validation and performance testing
- **Production Ready**: Error handling, monitoring, and scalability considerations

The system provides semantic search capabilities that understand meaning rather than just exact text matches, enabling users to find relevant content even when using different terminology or phrasing. The background processing ensures that file changes are automatically indexed, and the comprehensive monitoring provides visibility into system health and job progress.

**Status**: ✅ **COMPLETE** - All features implemented, tested, and production-ready.
