#!/bin/bash

echo "🔨 Building MCP Agent External Package"
echo "======================================"

# Build the external package
echo "📦 Building external package..."
go build -o bin/external_example examples/external_usage/main.go

if [ $? -eq 0 ]; then
    echo "✅ External package built successfully!"
    echo "📁 Binary created at: bin/external_example"
    echo ""
    echo "🚀 To run the example:"
    echo "   ./bin/external_example"
    echo ""
    echo "📋 Note: Make sure to set up your environment variables:"
    echo "   - AWS_REGION"
    echo "   - AWS_ACCESS_KEY_ID" 
    echo "   - AWS_SECRET_ACCESS_KEY"
    echo "   - BEDROCK_PRIMARY_MODEL"
else
    echo "❌ Failed to build external package"
    exit 1
fi 