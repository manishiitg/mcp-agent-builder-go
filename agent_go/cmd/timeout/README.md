# MCP Timeout Server

A simple MCP server designed for testing connection timeouts and long-running operations.

## Purpose

This server provides a single tool that intentionally takes 10 seconds to complete, making it useful for:
- Testing MCP client timeout handling
- Validating connection management under long operations
- Debugging timeout-related issues in MCP implementations

## Features

- **Single Tool**: `mock_timeout` - sleeps for 10 seconds then returns success
- **Transport Support**: Both stdio (default) and SSE protocols
- **Simple Logging**: Basic logging during execution
- **Lightweight**: Minimal implementation focused on testing

## Usage

### As a Standalone Binary

```bash
# Build the timeout server
cd agent_go/cmd/timeout/timeout-bin
go build -o timeout-server .

# Run with stdio transport (default)
./timeout-server

# Run with SSE transport on custom port
./timeout-server --transport sse --port 7088
```

### From the Main Project

```bash
# Build the timeout server
cd agent_go
go build -o ../bin/timeout-server ./cmd/timeout/timeout-bin

# Run with stdio transport
../bin/timeout-server

# Run with SSE transport
../bin/timeout-server --transport sse --port 9091
```

## Transport Options

### Stdio Transport (Default)
- **Command**: `timeout-server` or `timeout-server --transport stdio`
- **Use Case**: Direct process communication, testing with stdio clients

### SSE Transport
- **Command**: `timeout-server --transport sse --port 9091`
- **Use Case**: HTTP-based communication, testing with SSE clients
- **Default Port**: 9091 (configurable)

## Tool Details

### `mock_timeout`
- **Description**: Mock tool that sleeps for 10 seconds to test connection timeouts
- **Parameters**: None
- **Behavior**: 
  1. Logs start of timeout operation
  2. Sleeps for exactly 10 seconds
  3. Logs completion
  4. Returns success message
- **Output**: "Mock timeout completed successfully after 10 seconds"

## Testing Scenarios

1. **Connection Timeout Testing**: Verify clients handle long-running operations
2. **SSE Connection Management**: Test SSE client behavior during long operations
3. **Client Resilience**: Validate client recovery after timeout scenarios
4. **Performance Testing**: Measure client performance under slow server conditions

## Configuration

The server supports minimal configuration:
- `--transport`: Protocol type (`stdio` or `sse`)
- `--port`: Port number for SSE transport (default: 9091)

## Example MCP Client Usage

```go
// Example: Call the mock_timeout tool
result, err := client.CallTool(ctx, "mock_timeout", map[string]interface{}{})
if err != nil {
    log.Printf("Tool call failed: %v", err)
    return
}

log.Printf("Tool result: %s", result.GetText())
```

## Logging

The server provides basic logging to stderr:
- Server startup messages
- Tool execution start/completion
- Transport-specific information
- Error messages

## Dependencies

- `github.com/mark3labs/mcp-go` - MCP server implementation
- `github.com/spf13/cobra` - Command-line interface
- `github.com/spf13/viper` - Configuration management

## Building

```bash
# Build timeout server binary
cd agent_go/cmd/timeout/timeout-bin
go build -o timeout-server .

# Build from main project
cd agent_go
go build -o ../bin/timeout-server ./cmd/timeout/timeout-bin
```
