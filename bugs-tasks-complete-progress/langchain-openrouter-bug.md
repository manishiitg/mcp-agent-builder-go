# LangChain Go OpenRouter Usage Parameter Bug

## 🐛 **Issue Summary**
LangChain Go library does not pass `CallOptions.Metadata` to the actual HTTP request body for OpenRouter requests, preventing the `usage: {include: true}` parameter from being sent to OpenRouter API.

## 🔍 **Problem Details**

### **Expected Behavior**
- OpenRouter requests should include `usage: {include: true}` parameter in request body
- This enables cache token information (cache_tokens, cache_discount, etc.) in responses
- Cache information helps with cost optimization and usage tracking

### **Actual Behavior**
- `CallOptions.Metadata` is set correctly but not passed to HTTP request
- OpenRouter returns basic token usage but no cache-specific fields
- Cache token information is missing from responses

## 🧪 **Debug Evidence**

### **What Works**
```go
// ✅ Metadata is being set correctly
opts.Metadata["usage"] = map[string]interface{}{
    "include": true,
}
// Debug output: map[usage:map[include:true]]
```

### **What Doesn't Work**
```json
// ❌ Cache fields missing from response
{
  "CompletionTokens": 10,
  "PromptTokens": 18, 
  "TotalTokens": 28,
  "ReasoningTokens": 0
  // Missing: cache_tokens, cache_discount, cache_write_cost, cache_read_cost
}
```

## 🔧 **Root Cause**
LangChain Go has two separate metadata fields:
- `CallOptions.Metadata` - Used internally by LangChain Go
- `ChatRequest.Metadata` - Sent in actual HTTP request body

Our implementation sets `CallOptions.Metadata` but there's no public API to set `ChatRequest.Metadata`.

## 📁 **Files Modified**
- `agent_go/internal/llm/providers.go` - Added `WithOpenRouterUsage()` function
- `agent_go/cmd/testing/token-usage-test.go` - Enhanced to detect cache fields

## 🎯 **Impact**
- **Low Priority**: Basic token usage still works
- **Missing Feature**: Cache token information for cost optimization
- **Workaround Available**: Manual OpenRouter client implementation

## 💡 **Potential Solutions**
1. **Custom OpenRouter Client**: Create direct HTTP client with usage parameter
2. **LangChain Go Fork**: Modify library to support `ChatRequest.Metadata`
3. **Alternative Approach**: Use different method to pass usage parameter

## 📊 **Current Status**
- ✅ Implementation complete but not functional
- ✅ Debugging shows metadata is set but not transmitted
- ❌ Cache token information not available
- 🔄 Ready for alternative implementation approach

## 🔗 **References**
- [OpenRouter Prompt Caching Documentation](https://openrouter.ai/docs/features/prompt-caching)
- LangChain Go v0.1.14-pre.2.0.20250822161313-dd61fd90f4d9
- Issue discovered: 2025-01-27
