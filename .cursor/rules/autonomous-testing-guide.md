# Autonomous Testing Guide

## 🚀 **Essential Testing Workflow**

### **1. Test Execution Pattern**
```bash
# Always run from agent_go directory
cd agent_go

# 🚨 CRITICAL: Use same log file and truncate for repeated testing
LOG_FILE="logs/[TEST_NAME]-test.log"

# Step 1: Truncate log file before test
echo "" > $LOG_FILE

# Step 2: Run test with same file
go run main.go test [TEST_TYPE] --provider [PROVIDER] --log-file $LOG_FILE

# Example: Run comprehensive React test (no verbose flag)
LOG_FILE="logs/react-test.log"
echo "" > $LOG_FILE
go run main.go test comprehensive-react --provider bedrock --log-file $LOG_FILE

# For next test run, just truncate and reuse
echo "" > $LOG_FILE
go run main.go test comprehensive-react --provider openai --log-file $LOG_FILE
```

### **2. Critical Testing Rules**
- **❌ NEVER use --verbose flag** - Always use log files for output
- **✅ ALWAYS test with ALL servers** - Never limit to subset of servers
- **✅ Use go run main.go** - No need to build binary every time
- **✅ Always use timestamped log files** - For proper tracking and debugging
- **🚨 CRITICAL: Use same log file and truncate for repeated testing** - Avoid log file proliferation and maintain consistent debugging

### **3. 🚨 IMPORTANT: Log File Truncation Commands**
```bash
# ✅ RECOMMENDED: Use echo to truncate (works reliably)
echo "" > $LOG_FILE

# ✅ ALTERNATIVE: Use truncate command if available
truncate -s 0 $LOG_FILE

# ❌ AVOID: Direct redirection can get stuck in terminal
> $LOG_FILE  # This can hang the terminal!

# ❌ AVOID: Using cat with /dev/null can be verbose
cat /dev/null > $LOG_FILE
```

### **3. Why Use go run main.go Instead of Building**
```bash
# ✅ RECOMMENDED: Use go run for testing
go run main.go test [TEST_TYPE] --provider [PROVIDER] --log-file logs/[TEST_NAME]-$(date +%Y%m%d-%H%M%S).log

# ❌ NOT RECOMMENDED: Building binary every time
go build -o ../bin/orchestrator . && ../bin/orchestrator test [TEST_TYPE] --provider [PROVIDER] --log-file logs/[TEST_NAME]-$(date +%Y%m%d-%H%M%S).log
```

**Benefits of go run main.go:**
- **Faster iteration** - No build time between changes
- **Automatic dependency resolution** - Go handles module updates
- **Consistent with development workflow** - Same command for testing and development
- **No binary management** - No need to track binary locations or versions

### **4. Why ALWAYS Test with ALL Servers**
```bash
# ✅ ALWAYS: Test with complete MCP ecosystem
go run main.go test orchestrator-planning-only --provider openai --log-file logs/orchestrator-planning-$(date +%Y%m%d-%H%M%S).log

# ❌ NEVER: Limit servers for production testing
go run main.go test orchestrator-planning-only --provider openai --servers "citymall-aws-mcp,citymall-scripts-mcp" --log-file logs/limited-test.log
```

### **5. 🚨 CRITICAL: Use Same Log File and Truncate for Repeated Testing**
```bash
# ✅ RECOMMENDED: Use same log file for repeated testing
# Step 1: Create log file once
LOG_FILE="logs/orchestrator-planning-test.log"

# Step 2: Truncate before each test run
echo "" > $LOG_FILE

# Step 3: Run test with same file
go run main.go test orchestrator-planning-only --provider bedrock --log-file $LOG_FILE

# Step 2: Truncate and run test
echo "" > $LOG_FILE
go run main.go test orchestrator-planning-only --provider openai --log-file $LOG_FILE
```

**Why This Matters:**
- **🎯 Consistent Debugging**: Same file location for all test runs
- **📁 Avoid File Proliferation**: No timestamped files cluttering logs directory
- **🔄 Easy Comparison**: Compare results between different test runs
- **💾 Disk Space Management**: Prevent logs directory from growing indefinitely
- **🔍 Faster Analysis**: Know exactly where to look for logs

**Why All Servers Matter:**
- **Complete functionality testing** - Orchestrator needs full MCP ecosystem
- **Real-world conditions** - Production environment has all servers
- **Integration validation** - Tests server-to-server interactions
- **Performance testing** - Real load with all 13 servers and 122+ tools
- **Error detection** - Catch issues that only appear with full system

### **2. Log Analysis Workflow**
```bash
# Step 1: Set up log file for repeated testing
LOG_FILE="logs/[TEST_NAME]-test.log"

# Step 2: Truncate and run test
echo "" > $LOG_FILE
go run main.go test [TEST_TYPE] --provider bedrock --log-file $LOG_FILE

# Step 3: Check local logs immediately
tail -f $LOG_FILE

# Step 4: Get Langfuse traces for detailed analysis
go run main.go langfuse traces --filter "[TEST_TYPE]" --limit 10

# Step 5: For next test run, just truncate and reuse
echo "" > $LOG_FILE
go run main.go test [TEST_TYPE] --provider openai --log-file $LOG_FILE
```

## 🔍 **Test Types & Commands**

### **Agent Tests**
```bash
# Basic agent functionality
go run main.go test agent --simple --provider bedrock --log-file logs/simple-$(date +%Y%m%d-%H%M%S).log

# Multi-turn conversation testing
go run main.go test agent --multi-turn --provider bedrock --log-file logs/multi-turn-$(date +%Y%m%d-%H%M%S).log

# Streaming response testing
go run main.go test agent --streaming --provider bedrock --log-file logs/streaming-$(date +%Y%m%d-%H%M%S).log

# Complex multi-tool interactions
go run main.go test agent --complex --provider bedrock --log-file logs/complex-$(date +%Y%m%d-%H%M%S).log
```

### **Specialized Tests**
```bash
# ReAct agent with explicit reasoning
go run main.go test comprehensive-react --provider bedrock --log-file logs/react-$(date +%Y%m%d-%H%M%S).log
go run main.go test comprehensive-react --provider openrouter --log-file logs/react-openrouter-$(date +%Y%m%d-%H%M%S).log

# Large tool output handling
go run main.go test tool-output-handler --log-file logs/tool-output-$(date +%Y%m%d-%H%M%S).log

# Large output integration testing
go run main.go test large-output-integration --log-file logs/large-output-$(date +%Y%m%d-%H%M%S).log

# Structured output validation
go run main.go test structured-output --provider bedrock --log-file logs/structured-$(date +%Y%m%d-%H%M%S).log

# Token usage extraction testing (all providers: OpenAI, Bedrock, Anthropic, OpenRouter)
go run main.go test token-usage --log-file logs/token-usage-$(date +%Y%m%d-%H%M%S).log

### **OpenRouter Comprehensive ReAct Testing**
```bash
# Test OpenRouter with comprehensive ReAct reasoning
go run main.go test comprehensive-react --provider openrouter --log-file logs/react-openrouter-$(date +%Y%m%d-%H%M%S).log

# Test OpenRouter with specific servers (only when debugging specific issues)
go run main.go test comprehensive-react --provider openrouter --servers "citymall-aws-mcp,citymall-scripts-mcp" --log-file logs/react-openrouter-aws-scripts-$(date +%Y%m%d-%H%M%S).log

# Quick OpenRouter ReAct test script
./test_openrouter_comprehensive_react.sh
```
```

### **MCP Server Tests**
```bash
# Test all MCP servers (ALWAYS use all servers)
go run main.go test aws-test --config configs/mcp_server_actual.json

# Test specific server types (ONLY for debugging specific issues)
go run main.go test comprehensive-react --servers "citymall-aws-mcp,citymall-scripts-mcp" --log-file logs/aws-scripts-$(date +%Y%m%d-%H%M%S).log
```

### **Orchestrator Planning Agent Tests**
```bash
# Test orchestrator planning agent with ALL servers (critical for full functionality)
go run main.go test orchestrator-planning-only --provider openai --log-file logs/orchestrator-planning-$(date +%Y%m%d-%H%M%S).log

# Test orchestrator planning agent with Bedrock
go run main.go test orchestrator-planning-only --provider bedrock --log-file logs/orchestrator-planning-bedrock-$(date +%Y%m%d-%H%M%S).log

# IMPORTANT: Never limit servers for orchestrator tests - they need full MCP ecosystem
```

## 📊 **Log Analysis Commands**

### **Real-time Log Monitoring**
```bash
# Watch logs during test execution
tail -f logs/[TEST_NAME]-*.log

# Follow specific test log
tail -f logs/comprehensive-react-*.log

# Monitor multiple log files
tail -f logs/*.log
```

### **Log Pattern Analysis**
```bash
# Check for ReAct patterns
grep -i "Let me think about this step by step" logs/react-*.log
grep -i "Final Answer:" logs/react-*.log

# Check for tool execution
grep -i "tool_call_start" logs/*.log
grep -i "tool_call_end" logs/*.log

# Check for errors
grep -i "error\|failed\|timeout" logs/*.log

# Check for large tool outputs
grep -i "large tool output detected" logs/*.log
```

### **Log Summary Commands**
```bash
# Count tool calls in log
grep -c "tool_call_start" logs/[TEST_NAME]-*.log

# Count errors in log
grep -c -i "error\|failed" logs/[TEST_NAME]-*.log

# Show test duration
grep "Test completed" logs/[TEST_NAME]-*.log

# Show token usage
grep "token_usage" logs/[TEST_NAME]-*.log

# Check token usage from LangChain
grep -A 5 "Token Usage Analysis" logs/[TEST_NAME]-*.log
grep "Reasoning tokens" logs/[TEST_NAME]-*.log
```

## 🔍 **Langfuse Trace Analysis**

### **Trace Retrieval Commands**
```bash
# Get recent traces
go run main.go langfuse traces --limit 10

# Get traces for specific test
go run main.go langfuse traces --filter "comprehensive_react_test" --limit 5

# Get traces by span type
go run main.go langfuse traces --filter "agent-conversation" --limit 10

# Get error traces
go run main.go langfuse traces --filter "error" --limit 5
```

### **Trace Analysis Workflow**
```bash
# Step 1: Get trace IDs from recent test
go run main.go langfuse traces --filter "[TEST_TYPE]" --limit 1

# Step 2: Analyze specific trace (replace TRACE_ID)
go run main.go langfuse traces --id [TRACE_ID]

# Step 3: Check for specific events
go run main.go langfuse traces --filter "tool_execution_timeout" --limit 5
```

## ✅ **Test Success Criteria**

### **ReAct Test Validation**
```bash
# Check for explicit reasoning
grep -i "Let me think about this step by step" logs/react-*.log

# Check for completion pattern
grep -i "Final Answer:" logs/react-*.log

# Check for tool timeout handling
grep -i "timed out\|timeout" logs/react-*.log

# Check for cross-provider fallback
grep -i "fallback\|switching provider" logs/react-*.log

# Check for OpenRouter ReAct patterns
grep -i "openrouter.*initialized" logs/react-openrouter-*.log
grep -i "provider: openrouter" logs/react-openrouter-*.log
```

### **Large Tool Output Test Validation**
```bash
# Check for large output detection
grep -i "large tool output detected" logs/tool-output-*.log

# Check for file writing
grep -i "large tool output written to file" logs/tool-output-*.log

# Check for virtual tools
grep -i "virtual tools enabled" logs/tool-output-*.log
```

### **Structured Output Test Validation**
```bash
# Check for validation events
grep -i "structured output validation" logs/structured-*.log

# Check for schema compliance
grep -i "schema validation\|type safety" logs/structured-*.log

# Check for LLM validation
grep -i "llm validation" logs/structured-*.log
```

### **Token Usage Test Validation**
```bash
# Check for token usage data availability
grep -i "token usage data is available" logs/token-usage-*.log

# Check for reasoning tokens (o3 models)
grep -i "reasoning tokens" logs/token-usage-*.log

# Check for OpenAI vs Bedrock field names
grep -i "prompt tokens\|completion tokens" logs/token-usage-*.log
grep -i "input tokens\|output tokens" logs/token-usage-*.log

# Check for OpenRouter integration
grep -i "openrouter.*initialized" logs/token-usage-*.log
grep -i "provider: openrouter" logs/token-usage-*.log

# Validate token counts
grep -A 3 "Token Usage Analysis" logs/token-usage-*.log
```

## 🔍 **Langfuse Integration Testing**

### **Langfuse Test Commands**
```bash
# Test basic Langfuse functionality
go run main.go test langfuse

# Test specific Langfuse features
go run main.go test langfuse --basic
go run main.go test langfuse --spans
go run main.go test langfuse --llm
go run main.go test langfuse --error
go run main.go test langfuse --complex

# Test all Langfuse functionality
go run main.go test langfuse --all
```

### **Langfuse Trace Retrieval**
```bash
# Get recent traces (limit 10)
go run main.go test langfuse get

# Get specific trace by ID
go run main.go test langfuse get --trace-id [TRACE_ID]

# Get traces with specific filters
go run main.go test langfuse get --filter "[TEST_TYPE]" --limit 5
```

### **Langfuse Test Validation**
```bash
# Check for successful trace creation
grep -i "✅ Langfuse: Started trace" logs/langfuse-*.log

# Check for span creation
grep -i "📊 Langfuse: Started span" logs/langfuse-*.log

# Check for trace completion
grep -i "🏁 Langfuse: Ended trace" logs/langfuse-*.log

# Check for authentication success
grep -i "✅ Langfuse: Authentication successful" logs/*.log

# Check for automatic Langfuse setup
grep -i "🔧 Automatic Langfuse Setup" logs/*.log
```

### **Token Usage in Langfuse Traces**
```bash
# Check if traces contain token usage
go run main.go test langfuse get --trace-id [TRACE_ID] | grep -i "token"

# Look for token counts in trace details
grep -A 5 "input_tokens\|output_tokens\|total_tokens" logs/*.log

# Check for token usage in generation spans
grep -i "LLM Generation.*tokens" logs/*.log
```

### **Langfuse Integration Workflow**
```bash
# Step 1: Run agent test with Langfuse enabled
go run main.go test agent --simple --provider bedrock --log-file logs/agent-langfuse-$(date +%Y%m%d-%H%M%S).log

# Step 2: Get trace ID from logs
grep "trace_id:" logs/agent-langfuse-*.log | tail -1

# Step 3: Retrieve detailed trace from Langfuse
go run main.go test langfuse get --trace-id [TRACE_ID]

# Step 4: Verify token usage in trace
go run main.go test langfuse get --trace-id [TRACE_ID] | grep -i "token"
```

### **Langfuse Troubleshooting**
```bash
# Check environment variables
echo "LANGFUSE_PUBLIC_KEY: $LANGFUSE_PUBLIC_KEY"
echo "LANGFUSE_SECRET_KEY: $LANGFUSE_SECRET_KEY"
echo "TRACING_PROVIDER: $TRACING_PROVIDER"

# Test Langfuse connection
go run main.go test langfuse --basic

# Check for authentication errors
grep -i "authentication\|unauthorized\|forbidden" logs/*.log

# Verify trace creation
grep -i "started trace\|ended trace" logs/*.log
```

## 🚨 **Troubleshooting Commands**

### **Common Issues & Solutions**
```bash
# Test connection to MCP servers
go run main.go test aws-test --config configs/mcp_server_actual.json

# Check server health
go run main.go test connection-pool --log-file logs/connection-test.log

# Test specific provider
go run main.go test agent --simple --provider openai --log-file logs/openai-test.log

# Test with verbose logging
go run main.go test [TEST_TYPE] --provider bedrock --log-file logs/debug-$(date +%Y%m%d-%H%M%S).log --verbose
```

### **Error Analysis**
```bash
# Check for specific error types
grep -i "connection.*failed" logs/*.log
grep -i "tool.*timeout" logs/*.log
grep -i "llm.*error" logs/*.log

# Check for provider fallbacks
grep -i "fallback.*provider" logs/*.log
grep -i "switching.*provider" logs/*.log
```

## 📋 **Testing Checklist**

### **Pre-Test Setup**
- [ ] Ensure in `agent_go` directory
- [ ] Check environment variables (AWS, OpenAI keys)
- [ ] Verify MCP servers are running
- [ ] Create logs directory if needed

### **Test Execution**
- [ ] Use timestamped log file names
- [ ] Include `--provider` flag
- [ ] Add `--verbose` for debugging
- [ ] Use `--servers` for specific server testing

### **Post-Test Analysis**
- [ ] Check local log file immediately
- [ ] Look for success/failure patterns
- [ ] Get Langfuse traces for detailed analysis
- [ ] Verify expected events occurred
- [ ] Check for errors or timeouts

### **Validation Steps**
- [ ] Confirm test completed successfully
- [ ] Verify tool calls executed as expected
- [ ] Check for ReAct patterns (if applicable)
- [ ] Validate large tool output handling (if applicable)
- [ ] Confirm structured output validation (if applicable)

## 🎯 **Quick Reference Commands**

### **Most Common Tests**
```bash
# Quick agent test
go run main.go test agent --simple --provider bedrock --log-file logs/quick-$(date +%Y%m%d-%H%M%S).log

# ReAct agent test
go run main.go test comprehensive-react --provider bedrock --log-file logs/react-$(date +%Y%m%d-%H%M%S).log --verbose
go run main.go test comprehensive-react --provider openrouter --log-file logs/react-openrouter-$(date +%Y%m%d-%H%M%S).log --verbose

# All servers test
go run main.go test comprehensive-react --provider bedrock --servers all --log-file logs/all-servers-$(date +%Y%m%d-%H%M%S).log --verbose

# Token usage test (all providers: OpenAI gpt-4.1, o3-mini, Bedrock Claude, Anthropic Claude, OpenRouter)
go run main.go test token-usage --log-file logs/token-usage-$(date +%Y%m%d-%H%M%S).log

# OpenRouter comprehensive ReAct testing
go run main.go test comprehensive-react --provider openrouter --log-file logs/react-openrouter-$(date +%Y%m%d-%H%M%S).log --verbose

# Langfuse integration test
go run main.go test langfuse --all --log-file logs/langfuse-$(date +%Y%m%d-%H%M%S).log
```

### **Log Analysis**
```bash
# Watch logs in real-time
tail -f logs/*.log

# Check for patterns
grep -i "Final Answer:" logs/*.log

# Check for Langfuse traces
grep -i "✅ Langfuse: Started trace" logs/*.log
grep -i "🏁 Langfuse: Ended trace" logs/*.log

# Get traces from Langfuse
go run main.go test langfuse get
```

### **Troubleshooting**
```bash
# Test connections
go run main.go test aws-test

# Check server health
go run main.go test connection-pool

# Debug mode
go run main.go test [TEST_TYPE] --verbose --log-file logs/debug.log

# Langfuse troubleshooting
go run main.go test langfuse --basic
go run main.go test langfuse get
``` 