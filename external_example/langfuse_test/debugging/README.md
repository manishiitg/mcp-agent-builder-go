# Langfuse Debug Tool

A read-only debugging tool for retrieving and inspecting existing Langfuse traces, spans, and sessions. This tool helps you analyze and debug existing traces without creating new ones.

## 🏗️ **Recent Architectural Improvements** ✅ **COMPLETED**

### **Environment Variable Dependency Removal** ✅ **COMPLETED**
**Task**: Remove all environment variable dependencies from the external package and implement optional tracer configuration using the `With()` pattern.

**What Was Accomplished**:
- ✅ **Removed `createTracer` function** from `llm.go` 
- ✅ **Eliminated all `os.Getenv` usage** in external package
- ✅ **Updated `NewAgent` function** to use tracer from config instead of creating one
- ✅ **Implemented optional tracer pattern** - tracer is now configurable via `WithTracer()` method
- ✅ **Maintained backward compatibility** - uses `NoopTracer` if no tracer is provided
- ✅ **Added `Tracer` field** to `Config` struct for direct tracer injection
- ✅ **Added `WithTracer()` method** to `Config` struct for fluent configuration
- ✅ **Updated main.go** to use `WithObservability()` for clean architecture

**New Architecture**:
```go
// Option 1: Use WithObservability for simple configuration
config := external.DefaultConfig().
    WithAgentMode(external.SimpleAgent).
    WithLLM("openai", "gpt-4.1", 0.7).
    WithObservability("langfuse", "https://cloud.langfuse.com")

// Option 2: Use WithTracer for direct tracer injection (advanced)
tracer, err := observability.NewLangfuseTracerWithLogger(logger)
config := external.DefaultConfig().
    WithAgentMode(external.SimpleAgent).
    WithLLM("openai", "gpt-4.1", 0.7).
    WithTracer(tracer)  // Direct tracer injection

// Create agent (tracer is optional)
agent, err := external.NewAgent(ctx, config)
```

**Benefits**:
- 🎯 **Cleaner Architecture**: No environment variable manipulation in external package
- 🔧 **Flexible Configuration**: Tracer is optional and configurable
- 🚫 **No Hidden Dependencies**: All configuration is explicit and visible
- 🔄 **Better Testability**: Easy to inject mock tracers for testing
- 📦 **Single Responsibility**: External package focuses on agent creation, not tracer setup
- 🚀 **Dual Approach**: Both `WithObservability()` and `WithTracer()` methods available

**Files Modified**:
- `agent_go/pkg/external/agent.go` - Updated `NewAgent` to use config.Tracer
- `agent_go/pkg/external/config.go` - Added `Tracer` field and `WithTracer()` method
- `agent_go/pkg/external/llm.go` - Removed `createTracer` function
- `external_example/langfuse_test/main.go` - Updated to use `WithObservability()`

**Testing Results**:
- ✅ **Code compiles successfully** with new architecture
- ✅ **Test runs successfully** using `WithObservability("langfuse", host)`
- ✅ **MCP servers connect** and tools execute properly
- ✅ **Langfuse tracing works** via the clean configuration approach
- ✅ **No environment variable dependencies** in external package

---

## 🚀 **Features**

- **Fetch Traces**: Retrieve traces by ID or fetch recent traces
- **Session-based Queries**: Fetch all traces for a specific session ID
- **Span Inspection**: View spans and their relationships within traces
- **Event Analysis**: Examine events and scores associated with traces
- **Debug Mode**: Enable detailed logging and API request information
- **Read-Only**: Safe to use in production environments

## 🔧 **Setup**

1. **Environment Variables**: Set your Langfuse credentials in `.env`:
   ```bash
   LANGFUSE_PUBLIC_KEY=your_public_key
   LANGFUSE_SECRET_KEY=your_secret_key
   LANGFUSE_HOST=https://cloud.langfuse.com  # Optional, defaults to cloud.langfuse.com
   ```

2. **Build the Tool**:
   ```bash
   cd debugging
   go build -o langfuse-debug .
   ```

## 📖 **Usage Examples**

### **Fetch Recent Traces**
```bash
./langfuse-debug langfuse
```

This fetches the 10 most recent traces from your Langfuse dashboard.

### **Fetch with Debug Mode**
```bash
./langfuse-debug langfuse --debug
```

This fetches recent traces with detailed API request information.

### **Fetch Traces by Session ID**
```bash
./langfuse-debug langfuse --session-id "my-session-123"
```

This fetches all traces associated with the specified session ID.

### **Fetch Specific Trace by ID**
```bash
./langfuse-debug langfuse --trace-id "trace_id_here"
```

This fetches a specific trace by its ID.

### **Show Help**
```bash
./langfuse-debug --help
```

This displays all available commands and options.

## 🎯 **Use Cases**

1. **Debugging Existing Traces**: Inspect traces that were created by your applications
2. **Session Analysis**: Analyze all traces within a specific session
3. **Performance Monitoring**: Review trace timing and performance metrics
4. **Error Investigation**: Examine failed traces and their associated spans
5. **Production Monitoring**: Safely inspect traces in production without creating new data

## 🔍 **What Gets Retrieved**

### **Recent Traces**
- Up to 10 most recent traces
- Basic trace information (ID, name, timestamp)
- Observation and score counts

### **Session-based Traces**
- All traces for a specific session ID
- Up to 50 traces per session
- Detailed trace metadata

### **Specific Trace**
- Complete trace details
- All associated spans and observations
- Scores and metadata
- Input/output data

## 📊 **Session ID Benefits**

- **Group Related Operations**: All traces with the same session ID are grouped together
- **Cross-Request Tracking**: Track operations across multiple API calls
- **User Session Management**: Associate traces with user sessions
- **Debugging**: Easily find all traces for a specific session
- **Production Safety**: Read-only access prevents accidental data creation

## 🚨 **Troubleshooting**

- **Missing Credentials**: Ensure `LANGFUSE_PUBLIC_KEY` and `LANGFUSE_SECRET_KEY` are set
- **Network Issues**: Check your internet connection and Langfuse host accessibility
- **API Limits**: Be aware of Langfuse API rate limits
- **Debug Mode**: Use `--debug` flag for detailed error information
- **No Traces Found**: Ensure your applications are creating traces before trying to fetch them

## 🔗 **Integration with Main Test**

This debugging tool complements the main `main.go` test by:
- Providing read-only access to existing traces
- Allowing inspection of traces created by your applications
- Supporting session-based trace analysis
- Offering safe debugging capabilities

Use this tool when you need to:
- Debug existing traces without creating new ones
- Analyze trace performance and structure
- Investigate errors in production traces
- Verify session ID grouping functionality
- Safely monitor production Langfuse data

## 🆕 **New Tracer Architecture Status**

### **What Was Fixed** ✅ **COMPLETED**
- **Environment Variable Dependencies**: Removed all `os.Setenv` calls from external examples
- **Tracer Configuration**: Added `Tracer` field to `Config` struct
- **Fluent API**: Added `WithTracer()` method for direct tracer injection
- **Clean Configuration**: Updated main.go to use `WithObservability()` method
- **Backward Compatibility**: Maintained existing `WithObservability()` functionality

### **Current Working Approach**
```go
// Clean, simple configuration using WithObservability
agentConfig := external.DefaultConfig().
    WithAgentMode(external.SimpleAgent).
    WithLLM("openai", "gpt-4.1", 0.7).
    WithObservability("langfuse", host)  // ✅ Working approach

// Alternative: Direct tracer injection (advanced users)
// config := external.DefaultConfig().
//     WithAgentMode(external.SimpleAgent).
//     WithLLM("openai", "gpt-4.1", 0.7).
//     WithTracer(customTracer)
```

### **Benefits of New Architecture**
- 🎯 **No Environment Pollution**: External examples don't modify system environment
- 🔧 **Clean Configuration**: All settings passed via config methods
- 🚫 **No Hidden Dependencies**: Configuration is explicit and visible
- 🔄 **Flexible**: Both simple and advanced tracer configuration options
- 📦 **Maintainable**: Clear separation of concerns between external package and examples
