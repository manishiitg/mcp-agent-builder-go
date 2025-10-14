#!/bin/bash

# Test Environment Setup for All 4 Providers
# This script sets up environment variables for testing

echo "🔧 Setting up test environment for all 4 providers..."

# Check if .env file exists and source it
if [ -f ".env" ]; then
    echo "📁 Found .env file, sourcing it..."
    export $(cat .env | grep -v '^#' | xargs)
    echo "✅ Loaded environment variables from .env"
else
    echo "⚠️  No .env file found, using current environment variables"
fi

echo ""
echo "🔍 Current Provider Configuration:"
echo "=================================="

# Check AWS/Bedrock
if [ -n "$AWS_REGION" ] && [ -n "$AWS_ACCESS_KEY_ID" ] && [ -n "$AWS_SECRET_ACCESS_KEY" ]; then
    echo "✅ AWS/Bedrock: Configured (Region: $AWS_REGION)"
else
    echo "❌ AWS/Bedrock: Not configured"
fi

# Check OpenAI
if [ -n "$OPENAI_API_KEY" ]; then
    echo "✅ OpenAI: Configured"
else
    echo "❌ OpenAI: Not configured"
fi

# Check Anthropic
if [ -n "$ANTHROPIC_API_KEY" ]; then
    echo "✅ Anthropic: Configured"
else
    echo "❌ Anthropic: Not configured"
fi

# Check OpenRouter
if [ -n "$OPEN_ROUTER_API_KEY" ]; then
    echo "✅ OpenRouter: Configured"
else
    echo "❌ OpenRouter: Not configured"
fi

echo ""
echo "🚀 Ready to test with available providers!"
echo "💡 Run './test_all_providers_simple.sh' to test all configured providers"
