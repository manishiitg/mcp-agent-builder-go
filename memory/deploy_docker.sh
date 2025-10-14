#!/bin/bash

# Graphiti Knowledge Graph API Docker Deployment Script

set -e

echo "🐳 Deploying Graphiti Knowledge Graph API with Docker..."

# Check if Docker is running
if ! docker info > /dev/null 2>&1; then
    echo "❌ Docker is not running. Please start Docker first."
    exit 1
fi

# Create necessary directories
echo "📁 Creating data and logs directories..."
mkdir -p data logs

# Copy Docker environment configuration
echo "⚙️  Setting up environment configuration..."
if [ ! -f ".env" ]; then
    cp env.docker .env
    echo "✅ Created .env from env.docker template"
else
    echo "⚠️  .env already exists. Please ensure it's configured for Docker deployment."
    echo "   Key settings for Docker:"
    echo "   - OLLAMA_BASE_URL=http://ollama:11434/v1"
    echo "   - KUZU_DB=/app/data/graphiti.kuzu"
fi

# Build and start services
echo "🔨 Building and starting Docker services..."
docker-compose -f docker-compose.api.yml down --remove-orphans
docker-compose -f docker-compose.api.yml build --no-cache
docker-compose -f docker-compose.api.yml up -d

# Wait for services to be healthy
echo "⏳ Waiting for services to be healthy..."
sleep 10

# Check service health
echo "🏥 Checking service health..."
docker-compose -f docker-compose.api.yml ps

# Wait for Ollama to pull the embedding model
echo "📥 Ensuring Ollama has the required embedding model..."
docker exec graphiti-ollama ollama pull mxbai-embed-large || echo "⚠️  Model pull failed, but continuing..."

# Test API health endpoint
echo "🔍 Testing API health endpoint..."
sleep 5
if curl -f http://localhost:8000/health > /dev/null 2>&1; then
    echo "✅ API is healthy and responding"
else
    echo "❌ API health check failed"
    echo "📋 Checking logs..."
    docker-compose -f docker-compose.api.yml logs api
    exit 1
fi

echo ""
echo "🎉 Deployment completed successfully!"
echo ""
echo "📚 API Documentation: http://localhost:8000/docs"
echo "🔍 Health Check: http://localhost:8000/health"
echo "📊 API Root: http://localhost:8000/"
echo ""
echo "🐳 Docker Services:"
echo "   - Ollama (Embeddings): http://localhost:11434"
echo "   - Graphiti API: http://localhost:8000"
echo ""
echo "📋 Useful Commands:"
echo "   View logs: docker-compose -f docker-compose.api.yml logs -f"
echo "   Stop services: docker-compose -f docker-compose.api.yml down"
echo "   Restart API: docker-compose -f docker-compose.api.yml restart api"
echo "   Scale API: docker-compose -f docker-compose.api.yml up -d --scale api=3"
echo ""
echo "🗂️  Data Persistence:"
echo "   - Database: ./data/graphiti.kuzu"
echo "   - Logs: ./logs/"
echo "   - Ollama models: Docker volume 'graphiti_ollama_data'"
