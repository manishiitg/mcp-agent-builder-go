#!/bin/bash

echo "🚀 Langfuse Integration Test Runner"
echo "==================================="

# Check if .env file exists
if [ ! -f ".env" ]; then
    echo "❌ Error: .env file not found"
    echo "   Please create a .env file with your Langfuse credentials:"
    echo "   LANGFUSE_PUBLIC_KEY=your_public_key"
    echo "   LANGFUSE_SECRET_KEY=your_secret_key"
    echo "   LANGFUSE_HOST=https://cloud.langfuse.com (optional)"
    exit 1
fi

# Initialize Go modules if needed
if [ ! -f "go.sum" ]; then
    echo "📦 Initializing Go modules..."
    go mod tidy
fi

# Run the test
echo "🔧 Running Langfuse integration test with external agent..."
go run main.go

echo ""
echo "✅ Test completed! Check your Langfuse dashboard for traces."
