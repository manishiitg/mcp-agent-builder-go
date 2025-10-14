# 🚨 Deprecation Notice: Config Struct Approach

## ⚠️ **Important: Config Struct is Deprecated**

The `Config` struct and related functions in the external package are now **DEPRECATED** and will be removed in a future version.

## 🔄 **Migration Path: Use New With() Method Pattern**

### **❌ Old Approach (Deprecated)**
```go
// DEPRECATED: This approach will be removed
config := external.Config{
    AgentMode:     external.ReActAgent,
    ServerName:    "obsidian",
    ConfigPath:    "configs/mcp_servers.json",
    Provider:      "openai",
    ModelID:       "gpt-4.1",
    Temperature:   0.7,
    MaxTurns:      20,
    TraceProvider: "langfuse",
    LangfuseHost:  "https://cloud.langfuse.com",
    Logger:        customLogger,
    ToolChoice:    "auto",
    ToolTimeout:   30 * time.Second,
}

agent, err := external.NewAgent(ctx, config)
```

### **✅ New Approach (Recommended)**
```go
// NEW: Fluent builder pattern - cleaner and more intuitive
agent, err := external.NewAgentBuilder().
    WithAgentMode(external.ReActAgent).
    WithServer("obsidian", "configs/mcp_servers.json").
    WithLLM("openai", "gpt-4.1", 0.7).
    WithMaxTurns(20).
    WithObservability("langfuse", "https://cloud.langfuse.com").
    WithLogger(customLogger).
    WithToolChoice("auto").
    WithToolTimeout(5 * time.Minute).
    Create(ctx)
```

## 🔧 **Available With() Methods**

| Old Config Field | New With() Method | Description |
|------------------|-------------------|-------------|
| `AgentMode` | `WithAgentMode(mode)` | Set agent mode (Simple/ReAct) |
| `ServerName` + `ConfigPath` | `WithServer(name, configPath)` | Set MCP server configuration |
| `Provider` + `ModelID` + `Temperature` | `WithLLM(provider, modelID, temp)` | Set LLM configuration |
| `MaxTurns` | `WithMaxTurns(turns)` | Set maximum conversation turns |
| `TraceProvider` + `LangfuseHost` | `WithObservability(provider, host)` | Set tracing configuration |
| `Logger` | `WithLogger(logger)` | Set custom logger |
| `ToolChoice` | `WithToolChoice(choice)` | Set tool choice strategy |
| `ToolTimeout` | `WithToolTimeout(timeout)` | Set tool execution timeout |
| `SystemPrompt.CustomTemplate` | `WithCustomSystemPrompt(template)` | Set custom system prompt |
| `SystemPrompt.AdditionalInstructions` | `WithAdditionalInstructions(instructions)` | Add instructions to prompt |

## 🎯 **Benefits of New Approach**

1. **🔄 Fluent Interface**: More readable and intuitive
2. **🔒 Immutability**: Each With() call returns a new builder, preventing mutations
3. **📝 Self-Documenting**: Clear what each setting does
4. ** Composable**: Easy to build configs from base templates
5. **❌ Error Prevention**: Harder to forget required fields
6. ** Modern Go Style**: Follows current Go best practices

## 🚀 **Quick Migration Examples**

### **Simple Agent Creation**
```go
// OLD (Deprecated)
config := external.Config{
    AgentMode:  external.SimpleAgent,
    ServerName: "filesystem",
    ConfigPath: "configs/mcp_servers.json",
    Provider:   "openai",
    ModelID:    "gpt-4.1",
    Temperature: 0.5,
}

// NEW (Recommended)
agent, err := external.NewAgentBuilder().
    WithAgentMode(external.SimpleAgent).
    WithServer("filesystem", "configs/mcp_servers.json").
    WithLLM("openai", "gpt-4.1", 0.5).
    Create(ctx)
```

### **ReAct Agent with Custom Prompt**
```go
// OLD (Deprecated)
config := external.Config{
    AgentMode: external.ReActAgent,
    ServerName: "obsidian",
    ConfigPath: "configs/mcp_servers.json",
    Provider:  "bedrock",
    ModelID:   "us.anthropic.claude-sonnet-4-20250514-v1:0",
    Temperature: 0.2,
    MaxTurns:  15,
    SystemPrompt: external.SystemPromptConfig{
        CustomTemplate: "You are a specialized AI assistant...",
        AdditionalInstructions: "Remember to check permissions...",
    },
}

// NEW (Recommended)
agent, err := external.NewAgentBuilder().
    WithAgentMode(external.ReActAgent).
    WithServer("obsidian", "configs/mcp_servers.json").
    WithLLM("bedrock", "us.anthropic.claude-sonnet-4-20250514-v1:0", 0.2).
    WithMaxTurns(15).
    WithCustomSystemPrompt("You are a specialized AI assistant...").
    WithAdditionalInstructions("Remember to check permissions...").
    Create(ctx)
```

### **Agent with Langfuse Tracing**
```go
// OLD (Deprecated)
config := external.Config{
    AgentMode:     external.SimpleAgent,
    ServerName:    "all",
    ConfigPath:    "configs/mcp_servers.json",
    Provider:      "openai",
    ModelID:       "gpt-4.1",
    Temperature:   0.7,
    TraceProvider: "langfuse",
    LangfuseHost:  "https://cloud.langfuse.com",
    Logger:        customLogger,
}

// NEW (Recommended)
agent, err := external.NewAgentBuilder().
    WithAgentMode(external.SimpleAgent).
    WithServer("all", "configs/mcp_servers.json").
    WithLLM("openai", "gpt-4.1", 0.7).
    WithObservability("langfuse", "https://cloud.langfuse.com").
    WithLogger(customLogger).
    Create(ctx)
```

## 📅 **Timeline**

- **Current**: Config struct is deprecated with warnings
- **Next Release**: Config struct will show deprecation warnings
- **Future Release**: Config struct will be removed entirely

## 🆘 **Need Help?**

If you need assistance migrating your code to the new With() method pattern:

1. **Check Examples**: Look at `external_example/` directory for working examples
2. **Read Documentation**: See the main README for detailed usage
3. **Report Issues**: Open an issue if you encounter problems during migration

## 🎉 **Why This Change?**

The new With() method pattern provides:
- **Better Developer Experience**: More intuitive and readable
- **Improved Safety**: Immutable configuration building
- **Modern Go Patterns**: Follows current best practices
- **Easier Maintenance**: Cleaner, more maintainable code
