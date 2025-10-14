# Graphiti Knowledge Graph API Dockerfile
FROM python:3.13-slim

# Set working directory
WORKDIR /app

# Set environment variables
ENV PYTHONUNBUFFERED=1
ENV PYTHONDONTWRITEBYTECODE=1
ENV PIP_NO_CACHE_DIR=1
ENV PIP_DISABLE_PIP_VERSION_CHECK=1

# Build argument for BGE reranker
ARG USE_BGE_RERANKER=false

# Install system dependencies
RUN apt-get update && apt-get install -y \
    gcc \
    g++ \
    make \
    curl \
    wget \
    && rm -rf /var/lib/apt/lists/*

# Note: FTS extension may not be available for ARM64, will be handled at runtime

# Copy requirements files
COPY requirements_api.txt ./

# Install Python dependencies
RUN pip install --no-cache-dir -r requirements_api.txt

# Install graphiti-core with sentence-transformers support for BGE reranker
RUN pip install --no-cache-dir "graphiti-core[sentence-transformers]"

# Copy application files
COPY api.py ./

# Create BGE model download script
RUN echo '#!/usr/bin/env python3\n\
import os\n\
import sys\n\
import shutil\n\
from sentence_transformers import SentenceTransformer\n\
from pathlib import Path\n\
\n\
def download_bge_model():\n\
    """Download BGE reranker model during build (only if not already cached)"""\n\
    try:\n\
        model_name = os.getenv("BGE_MODEL_NAME", "BAAI/bge-reranker-v2-m3")\n\
        cache_dir = os.getenv("BGE_MODEL_CACHE_DIR", "/app/models/bge-reranker")\n\
        \n\
        # Check if model already exists in our custom cache\n\
        model_path = Path(cache_dir) / model_name.replace("/", "--")\n\
        if model_path.exists() and any(model_path.iterdir()):\n\
            print(f"BGE model already cached at {cache_dir}, skipping download")\n\
            return\n\
        \n\
        print(f"Downloading BGE model: {model_name}")\n\
        print(f"Cache directory: {cache_dir}")\n\
        \n\
        # Set HF_HOME to our custom directory to force download there\n\
        os.environ["HF_HOME"] = cache_dir\n\
        \n\
        # Download and cache the model\n\
        model = SentenceTransformer(model_name)\n\
        print(f"BGE model downloaded successfully to {cache_dir}")\n\
        \n\
        # Verify the model was downloaded to the right place\n\
        if model_path.exists():\n\
            print(f"Model verified at {model_path}")\n\
        else:\n\
            print(f"Warning: Model not found at expected path {model_path}")\n\
            \n\
    except Exception as e:\n\
        print(f"Warning: Failed to download BGE model: {e}")\n\
        print("BGE reranker will be downloaded on first use")\n\
        sys.exit(0)  # Non-fatal error\n\
\n\
if __name__ == "__main__":\n\
    download_bge_model()\n\
' > download_bge_model.py

# Download BGE model only if USE_BGE_RERANKER=true
RUN if [ "$USE_BGE_RERANKER" = "true" ]; then \
        python download_bge_model.py || echo "BGE model download failed, will download on first use"; \
    else \
        echo "BGE reranker disabled, skipping model download"; \
    fi

# Create directories for data persistence and BGE model cache
RUN mkdir -p /app/data /app/logs /app/models/bge-reranker

# Create non-root user for security
RUN useradd --create-home --shell /bin/bash app && \
    chown -R app:app /app
USER app

# Expose port
EXPOSE 8000

# Health check
HEALTHCHECK --interval=30s --timeout=30s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8000/health || exit 1

# Default command
CMD ["python", "api.py"]
