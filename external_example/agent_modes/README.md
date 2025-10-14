# Agent Modes Example - Simple vs ReAct Comparison

This directory demonstrates the **key differences between Simple and ReAct agent modes** using the external MCP agent package. You can see how each mode behaves differently in terms of reasoning, tool usage, and conversation flow.

## ✅ **What We Built**

A comprehensive comparison system that demonstrates:
- **Simple Agent Mode**: Direct tool usage without explicit reasoning
- **ReAct Agent Mode**: Step-by-step reasoning with explicit thought processes
- **Mode Switching**: Easy configuration between different agent behaviors
- **Behavior Comparison**: Side-by-side analysis of how each mode works

## 🏗️ **Architecture Overview**

### **Agent Mode Differences**

#### **Simple Agent** (`simple`)
- **Behavior**: Direct tool usage without explicit reasoning
- **Conversation End**: Ends immediately when no tool calls are detected
- **Max Turns**: 10 (fewer turns for direct responses)
- **Best For**: Straightforward queries, quick responses, tool-heavy tasks
- **Event**: Emits `conversation_end` when no tools are called

#### **ReAct Agent** (`react`)
- **Behavior**: Explicit reasoning with step-by-step thinking
- **Conversation End**: Ends when "Final Answer:" pattern is detected
- **Max Turns**: 20 (more turns for reasoning process)
- **Best For**: Complex queries, multi-step problems, reasoning-heavy tasks
- **Event**: Emits `conversation_end` when completion pattern is found

### **Mode Configuration**
```go
// Simple Agent
config := external.DefaultConfig().
    WithAgentMode(external.SimpleAgent).
    WithMaxTurns(10)

// ReAct Agent  
config := external.DefaultConfig().
    WithAgentMode(external.ReActAgent).
    WithMaxTurns(20)
```

## 🚀 **How to Use Different Agent Modes**

### **Step 1: Choose Your Agent Mode**

```go
// For Simple Agent (fast, direct)
config := external.DefaultConfig().
    WithAgentMode(external.SimpleAgent).
    WithLLM("openai", "gpt-4o-mini", 0.1).
    WithMaxTurns(10)

// For ReAct Agent (reasoning, step-by-step)
config := external.DefaultConfig().
    WithAgentMode(external.ReActAgent).
    WithLLM("openai", "gpt-4o", 0.1).
    WithMaxTurns(20)
```

### **Step 2: Create Your Agent**

```go
// Create agent with chosen mode
agent, err := external.NewAgent(config)
if err != nil {
    log.Fatalf("Failed to create agent: %v", err)
}

// Use the agent - it will behave according to the selected mode
response, err := agent.Execute(ctx, "What's the weather like today?")
```

### **Step 3: Observe the Differences**

**Simple Agent Response:**
```
I'll check the weather for you.
[Tool Call: weather_tool]
The weather is sunny with a temperature of 75°F.
```

**ReAct Agent Response:**
```
Let me think about this step by step:

1. First, I need to understand what weather information the user is asking for
2. I should use a weather tool to get current conditions
3. Let me call the weather tool to get this information
4. Based on the tool response, I can provide a comprehensive answer

[Tool Call: weather_tool]

Now I have the weather information. Let me analyze it:
- Current temperature: 75°F
- Conditions: Sunny
- This is pleasant weather for outdoor activities

Final Answer: The weather is currently sunny with a temperature of 75°F, which is quite pleasant for outdoor activities.
```

## 📁 **File Structure**

```
external_example/agent_modes/
├── README.md                 # This guide
├── agent_modes.go           # Complete working example with both modes
├── run_simple.sh            # Test script for Simple agent
├── run_react.sh             # Test script for ReAct agent
├── compare_modes.sh         # Side-by-side comparison script
├── mcp_servers.json         # MCP server configuration
└── logs/                    # Generated log files
    ├── simple_agent.log     # Simple agent execution logs
    └── react_agent.log      # ReAct agent execution logs
```

## 🧪 **Testing Different Agent Modes**

### **Test Simple Agent**
```bash
cd external_example/agent_modes

# Test Simple agent mode
./run_simple.sh

# Check the logs
tail -f logs/simple_agent.log
```

### **Test ReAct Agent**
```bash
# Test ReAct agent mode
./run_react.sh

# Check the logs
tail -f logs/react_agent.log
```

### **Compare Both Modes**
```bash
# Run side-by-side comparison
./compare_modes.sh

# This will show you the differences in behavior
```

## 🔧 **Key Implementation Details**

### **Mode Selection**
1. **Configuration Time**: Set agent mode when creating the configuration
2. **Runtime Behavior**: Agent behavior is determined by the selected mode
3. **No Dynamic Switching**: Mode cannot be changed after agent creation
4. **Consistent Behavior**: Same mode applies to all conversations with that agent

### **Tool Usage Patterns**
- **Simple Agent**: Uses tools directly without explanation
- **ReAct Agent**: Explains reasoning before and after tool usage

### **Conversation Flow**
- **Simple Agent**: Shorter conversations, fewer turns
- **ReAct Agent**: Longer conversations, more detailed reasoning

## 🎯 **Use Cases**

### **Simple Agent Best For**
- **Quick Queries**: "What's the time?", "Get file info"
- **Tool-Heavy Tasks**: File operations, data retrieval
- **Production Systems**: Where speed is critical
- **Simple Workflows**: Straightforward, single-step operations

### **ReAct Agent Best For**
- **Complex Problems**: Multi-step reasoning required
- **Debugging**: Need to understand the thought process
- **Learning**: Want to see how the agent thinks
- **Research**: Complex queries requiring analysis

## ✅ **What's Working Now**

- ✅ **Mode Selection**: Easy configuration between Simple and ReAct
- ✅ **Behavior Differences**: Clear distinction in how each mode works
- ✅ **Tool Usage**: Different patterns for each mode
- ✅ **Conversation Flow**: Different turn limits and end detection
- ✅ **Event System**: Proper event emission for each mode
- ✅ **Performance**: Optimized settings for each mode

## 🚀 **Next Steps for Developers**

1. **Choose Your Mode**: Decide between Simple (fast) and ReAct (reasoning)
2. **Configure Accordingly**: Set appropriate max turns and other parameters
3. **Test Both Modes**: Run the comparison to see the differences
4. **Optimize for Use Case**: Use Simple for production, ReAct for complex tasks
5. **Monitor Performance**: Track which mode works better for your use case

## 🔍 **Troubleshooting**

### **Simple Agent Too Fast**
- Increase `WithMaxTurns()` for more complex queries
- Consider switching to ReAct mode for reasoning-heavy tasks

### **ReAct Agent Too Slow**
- Decrease `WithMaxTurns()` to limit conversation length
- Consider switching to Simple mode for straightforward tasks

### **Mode Not Working as Expected**
- Verify `WithAgentMode()` is called before `NewAgent()`
- Check that the mode constant is correct (`SimpleAgent` or `ReActAgent`)
- Ensure the LLM model supports the selected mode

---

**🎉 You now have a complete understanding of Simple vs ReAct agent modes!**
