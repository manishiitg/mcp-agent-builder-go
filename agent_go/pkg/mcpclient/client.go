package mcpclient

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"mcp-agent/agent_go/internal/utils"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// RetryConfig defines the retry behavior for MCP connections
type RetryConfig struct {
	MaxRetries     int           // Maximum number of retry attempts
	InitialDelay   time.Duration // Initial delay between retries
	MaxDelay       time.Duration // Maximum delay between retries
	BackoffFactor  float64       // Exponential backoff multiplier
	ConnectTimeout time.Duration // Timeout for each individual connection attempt
}

// DefaultRetryConfig returns a sensible default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:     3,
		InitialDelay:   1 * time.Second,
		MaxDelay:       30 * time.Second,
		BackoffFactor:  2.0,
		ConnectTimeout: 15 * time.Minute, // Increased from 10 minutes to 15 minutes for very slow npx commands
	}
}

// Client wraps the underlying MCP client with convenience methods
type Client struct {
	config        MCPServerConfig
	mcpClient     *client.Client
	serverInfo    *mcp.Implementation
	retryConfig   RetryConfig
	logger        utils.ExtendedLogger
	contextCancel context.CancelFunc // Store context cancel function for SSE connections
	context       context.Context    // Store context for SSE connections
	mu            sync.RWMutex       // Protect access to contextCancel and context
}

// New creates a new MCP client for the given server configuration
func New(config MCPServerConfig, logger utils.ExtendedLogger) *Client {
	return &Client{
		config:      config,
		retryConfig: DefaultRetryConfig(),
		logger:      logger,
	}
}

// NewWithRetryConfig creates a new MCP client with custom retry configuration
func NewWithRetryConfig(config MCPServerConfig, retryConfig RetryConfig, logger utils.ExtendedLogger) *Client {
	return &Client{
		config:      config,
		retryConfig: retryConfig,
		logger:      logger,
	}
}

// Connect establishes a connection to the MCP server with retry logic
func (c *Client) Connect(ctx context.Context) error {
	maxRetries := 3
	baseDelay := time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			delay := time.Duration(attempt-1) * baseDelay
			c.logger.Infof("🔄 Retrying MCP connection (attempt %d/%d) to server '%s' after %v delay...", attempt, maxRetries, c.getServerName(), delay)
			time.Sleep(delay)
		}

		protocol := c.config.GetProtocol()
		if protocol == ProtocolStdio {
			c.logger.Infof("🔌 Connecting to MCP server '%s' via %s (command: %s %v)...", c.getServerName(), protocol, c.config.Command, c.config.Args)
		} else {
			c.logger.Infof("🔌 Connecting to MCP server '%s' via %s (%s)...", c.getServerName(), protocol, c.config.URL)
		}

		err := c.connectOnce(ctx)
		if err == nil {
			if attempt > 1 {
				c.logger.Infof("✅ Successfully connected to MCP server after retry attempts: %s (retry_attempts: %d)", c.getServerName(), attempt-1)
			} else {
				c.logger.Infof("✅ Successfully connected to MCP server on first attempt: %s", c.getServerName())
			}
			return nil
		}

		c.logger.Errorf("❌ Connection attempt failed for server %s (attempt %d): %v", c.getServerName(), attempt, err)

		if attempt == maxRetries {
			return fmt.Errorf("failed to connect to MCP server '%s' after %d attempts: %v", c.getServerName(), maxRetries, err)
		}
	}

	return fmt.Errorf("unexpected error in retry loop for server '%s'", c.getServerName())
}

// connectOnce performs a single connection attempt
func (c *Client) connectOnce(ctx context.Context) error {
	// Prepare environment variables
	var env []string
	for key, value := range c.config.Env {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	var mcpClient *client.Client
	var err error

	// Create MCP client based on protocol type (use smart detection)
	protocol := c.config.GetProtocol()
	switch protocol {
	case ProtocolSSE:
		// Use SSE transport
		sseManager := NewSSEManager(c.config.URL, c.config.Headers, c.logger)
		mcpClient, err = sseManager.Connect(ctx)
		if err != nil {
			return fmt.Errorf("failed to create SSE MCP client: %w", err)
		}

	case ProtocolHTTP:
		// Use HTTP transport
		httpManager := NewHTTPManager(c.config.URL, c.config.Headers, c.logger)
		mcpClient, err = httpManager.Connect(ctx)
		if err != nil {
			return fmt.Errorf("failed to create HTTP MCP client: %w", err)
		}

	case ProtocolStdio:
		fallthrough
	default:
		// Default to stdio for backward compatibility
		stdioManager := NewStdioManager(c.config.Command, c.config.Args, env, c.logger)
		mcpClient, err = stdioManager.Connect(ctx)
		if err != nil {
			return fmt.Errorf("failed to create MCP client: %w", err)
		}
	}

	c.mcpClient = mcpClient

	// For stdio clients, initialization is handled by the transport manager
	// For other protocols, we need to initialize here
	if protocol != ProtocolStdio {
		// Initialize connection
		initResult, err := c.mcpClient.Initialize(ctx, mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: "2024-11-05",
				Capabilities:    mcp.ClientCapabilities{},
				ClientInfo: mcp.Implementation{
					Name:    "mcp-agent-go",
					Version: "1.0.0",
				},
			},
		})
		if err != nil {
			c.mcpClient.Close()
			return fmt.Errorf("failed to initialize MCP connection: %w", err)
		}

		c.serverInfo = &initResult.ServerInfo
	} else {
		// For stdio, we need to get server info separately since initialization was already done
		// We'll get this from the first tool listing or other operation
		c.serverInfo = &mcp.Implementation{
			Name:    "stdio-server",
			Version: "1.0.0",
		}
	}

	return nil
}

// ConnectWithRetry establishes connection to the MCP server with retry logic
func (c *Client) ConnectWithRetry(ctx context.Context) error {
	var lastErr error

	for attempt := 0; attempt <= c.retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate delay with exponential backoff
			delay := time.Duration(float64(c.retryConfig.InitialDelay) * math.Pow(c.retryConfig.BackoffFactor, float64(attempt-1)))
			if delay > c.retryConfig.MaxDelay {
				delay = c.retryConfig.MaxDelay
			}

			c.logger.Infof("🔄 Retrying MCP connection (attempt %d/%d) to server '%s' after %v delay...", attempt+1, c.retryConfig.MaxRetries+1, c.getServerName(), delay)

			select {
			case <-time.After(delay):
				// Continue with retry
			case <-ctx.Done():
				return fmt.Errorf("context cancelled during retry delay: %w", ctx.Err())
			}
		}

		// Create context with timeout for this specific attempt
		connectCtx, cancel := context.WithTimeout(ctx, c.retryConfig.ConnectTimeout)

		// Log connection attempt
		if attempt == 0 {
			c.logger.Infof("🔌 Connecting to MCP server '%s' (command: %s %v)...", c.getServerName(), c.config.Command, c.config.Args)
		}

		// Attempt connection
		err := c.Connect(connectCtx)
		cancel()

		if err == nil {
			if attempt > 0 {
				c.logger.Infof("✅ Successfully connected to MCP server after retry attempts: %s (retry_attempts: %d)", c.getServerName(), attempt)
			} else {
				c.logger.Infof("✅ Successfully connected to MCP server on first attempt: %s", c.getServerName())
			}
			return nil
		}

		lastErr = err
		c.logger.Errorf("❌ Connection attempt failed for server %s (attempt %d): %v", c.getServerName(), attempt+1, err)

		// If this was the last attempt, don't sleep
		if attempt == c.retryConfig.MaxRetries {
			break
		}

		// Check if context was cancelled
		if ctx.Err() != nil {
			return fmt.Errorf("context cancelled during connection retry: %w", ctx.Err())
		}
	}

	return fmt.Errorf("failed to connect to MCP server '%s' after %d attempts: %w",
		c.getServerName(), c.retryConfig.MaxRetries+1, lastErr)
}

// getServerName returns a human-readable name for the server (used for logging)
func (c *Client) getServerName() string {
	if c.config.Description != "" {
		return c.config.Description
	}
	return fmt.Sprintf("%s %v", c.config.Command, c.config.Args)
}

// Close closes the connection to the MCP server
func (c *Client) Close() error {
	// For SSE connections, cancel the stored context first
	if c.contextCancel != nil {
		c.logger.Infof("🔍 Canceling SSE context before closing client")
		c.contextCancel()
	}

	// Clear the stored context and cancel function
	c.mu.Lock()
	c.context = nil
	c.contextCancel = nil
	c.mu.Unlock()

	if c.mcpClient != nil {
		return c.mcpClient.Close()
	}
	return nil
}

// GetServerInfo returns information about the connected server
func (c *Client) GetServerInfo() *mcp.Implementation {
	return c.serverInfo
}

// GetMCPClient returns the underlying MCP client (for pooled client usage)
func (c *Client) GetMCPClient() *client.Client {
	return c.mcpClient
}

// ListTools returns all available tools from the server
func (c *Client) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	c.logger.Infof("🔧 [LISTTOOLS DEBUG] Starting ListTools call...")

	if c.mcpClient == nil {
		c.logger.Infof("❌ [LISTTOOLS DEBUG] Client not connected")
		return nil, fmt.Errorf("client not connected")
	}

	c.logger.Infof("🔧 [LISTTOOLS DEBUG] About to call underlying mcpClient.ListTools...")
	deadline, hasDeadline := ctx.Deadline()
	c.logger.Infof("🔧 [LISTTOOLS DEBUG] Context info: has_deadline=%v, deadline=%v, done=%v", hasDeadline, deadline, ctx.Done())

	listStartTime := time.Now()

	// Call ListTools directly without goroutine wrapper
	c.logger.Infof("🔧 [LISTTOOLS DEBUG] About to make the actual ListTools call...")

	// Add a timeout wrapper to see if it's the call itself
	callCtx, callCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer callCancel()

	c.logger.Infof("🔧 [LISTTOOLS DEBUG] Making ListTools call with 5m timeout...")
	result, err := c.mcpClient.ListTools(callCtx, mcp.ListToolsRequest{})

	c.logger.Infof("🔧 [LISTTOOLS DEBUG] ListTools call returned: error=%v", err)

	listDuration := time.Since(listStartTime)
	c.logger.Infof("🔧 [LISTTOOLS DEBUG] ListTools call completed: duration=%s", listDuration.String())

	if err != nil {
		c.logger.Infof("❌ [LISTTOOLS DEBUG] Failed to list tools: %v", err)
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	c.logger.Infof("✅ [LISTTOOLS DEBUG] Successfully listed tools: tool_count=%d", len(result.Tools))
	return result.Tools, nil
}

// CallTool invokes a tool with the given arguments
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
	if c.mcpClient == nil {
		return nil, fmt.Errorf("client not connected")
	}

	result, err := c.mcpClient.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: arguments,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to call tool %s: %w", name, err)
	}

	return result, nil
}

// ListResources lists all available resources from the server
func (c *Client) ListResources(ctx context.Context) ([]mcp.Resource, error) {
	if c.mcpClient == nil {
		return nil, fmt.Errorf("client not connected")
	}

	result, err := c.mcpClient.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	return result.Resources, nil
}

// GetResource gets a specific resource by URI
func (c *Client) GetResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	if c.mcpClient == nil {
		return nil, fmt.Errorf("client not connected")
	}

	result, err := c.mcpClient.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: uri,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get resource %s: %w", uri, err)
	}

	return result, nil
}

// ListPrompts lists all available prompts from the server
func (c *Client) ListPrompts(ctx context.Context) ([]mcp.Prompt, error) {
	if c.mcpClient == nil {
		return nil, fmt.Errorf("client not connected")
	}

	result, err := c.mcpClient.ListPrompts(ctx, mcp.ListPromptsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list prompts: %w", err)
	}

	return result.Prompts, nil
}

// GetPrompt gets a specific prompt by name
func (c *Client) GetPrompt(ctx context.Context, name string) (*mcp.GetPromptResult, error) {
	if c.mcpClient == nil {
		return nil, fmt.Errorf("client not connected")
	}

	// Create the MCP request
	request := mcp.GetPromptRequest{
		Params: mcp.GetPromptParams{
			Name: name,
		},
	}

	// Send the request
	response, err := c.mcpClient.GetPrompt(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to get prompt: %w", err)
	}

	return response, nil
}

// SetContextCancel stores the context cancel function for later cleanup (used for SSE connections)
func (c *Client) SetContextCancel(cancel context.CancelFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contextCancel = cancel
}

// GetContextCancel retrieves the stored context cancel function
func (c *Client) GetContextCancel() context.CancelFunc {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.contextCancel
}

// SetContext stores the context for SSE connections
func (c *Client) SetContext(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.context = ctx
}

// GetContext retrieves the stored context
func (c *Client) GetContext() context.Context {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.context
}

// ConnectWithTimeout is a convenience method that connects with a default timeout
func (c *Client) ConnectWithTimeout(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return c.ConnectWithRetry(ctx)
}

// ParallelToolDiscoveryResult represents the result of discovering tools from a single server
type ParallelToolDiscoveryResult struct {
	ServerName string
	Tools      []mcp.Tool
	Error      error
	Client     ClientInterface // Add client to the result so it can be reused
}

// DiscoverAllToolsParallel connects to all servers in the config in parallel, lists tools, and returns results per server.
func DiscoverAllToolsParallel(ctx context.Context, cfg *MCPConfig, logger utils.ExtendedLogger) []ParallelToolDiscoveryResult {
	servers := cfg.ListServers()
	if len(servers) == 0 {
		return []ParallelToolDiscoveryResult{}
	}

	logger.Infof("🚀 DiscoverAllToolsParallel started: server_count=%d, servers=%v", len(servers), servers)

	resultsCh := make(chan ParallelToolDiscoveryResult, len(servers))
	var wg sync.WaitGroup

	for _, name := range servers {
		srvCfg, _ := cfg.GetServer(name) // ignore error, will be caught below
		wg.Add(1)
		go func(name string, srvCfg MCPServerConfig) {
			defer wg.Done()

			client := New(srvCfg, logger)
			var cancel context.CancelFunc
			var connCtx context.Context

			if srvCfg.Protocol == ProtocolSSE {
				// For SSE, create a new background context with timeout to avoid parent cancellation
				// IMPORTANT: Do NOT defer cancel() here - we need the context to remain valid for the entire client lifecycle
				connCtx, cancel = context.WithTimeout(context.Background(), 15*time.Minute)
				logger.Infof("🔍 Using SSE protocol with isolated context: server_name=%s, timeout=15m", name)
			} else {
				// For stdio and other protocols, also use isolated context with longer timeout
				connCtx, cancel = context.WithTimeout(context.Background(), 15*time.Minute)
				defer cancel() // Safe to cancel immediately for non-SSE protocols
				logger.Infof("🔍 Using %s protocol with isolated context: server_name=%s, timeout=15m", srvCfg.Protocol, name)
			}

			logger.Infof("🔍 Attempting connection: server_name=%s", name)

			if err := client.ConnectWithRetry(connCtx); err != nil {
				logger.Errorf("❌ Connection failed: server_name=%s, error=%v", name, err)
				if cancel != nil {
					cancel() // Clean up context on connection failure
				}
				resultsCh <- ParallelToolDiscoveryResult{ServerName: name, Tools: nil, Error: err, Client: nil}
				return
			}

			logger.Infof("✅ Connection successful: server_name=%s", name)

			// For SSE connections, the SSE manager now uses background context for Start() automatically
			// For other protocols, no additional Start() call is needed
			logger.Infof("✅ Client ready for use: server_name=%s", name)

			// For SSE connections, use the same isolated context for tool listing
			// For other protocols, use the same isolated context
			var listCtx context.Context
			listCtx = connCtx // Use the same isolated context for all protocols

			logger.Infof("🔍 Listing tools: server_name=%s", name)

			logger.Infof("🔧 [PARALLEL DEBUG] About to call ListTools: server_name=%s", name)
			logger.Infof("🔧 [PARALLEL DEBUG] Context info before ListTools: server_name=%s, context_done=%v, context_err=%v", name, listCtx.Done(), listCtx.Err())
			listStartTime := time.Now()

			logger.Infof("🔧 [PARALLEL DEBUG] Calling client.ListTools NOW...: server_name=%s", name)
			tools, err := client.ListTools(listCtx)

			listDuration := time.Since(listStartTime)
			logger.Infof("🔧 [PARALLEL DEBUG] ListTools call completed: server_name=%s, duration=%s, error=%v", name, listDuration.String(), err)

			// Don't close the client here - we need to reuse it for agent creation
			// _ = client.Close()

			// For SSE connections, store the context and cancel function for later cleanup
			// Don't cancel the context here - it needs to remain valid for the client lifecycle
			if srvCfg.Protocol == ProtocolSSE {
				// Store the context and cancel function in the client for later cleanup
				// We'll cancel it when the client is actually closed
				client.SetContextCancel(cancel)
				client.SetContext(connCtx) // Store the context as well
				logger.Infof("🔍 Stored SSE context and cancel function for later cleanup: server_name=%s", name)
			}

			if err != nil {
				logger.Errorf("❌ Tool listing failed: server_name=%s, error=%v", name, err)
			} else {
				logger.Infof("✅ Tool listing successful: server_name=%s, tool_count=%d", name, len(tools))
			}

			resultsCh <- ParallelToolDiscoveryResult{ServerName: name, Tools: tools, Error: err, Client: client}
		}(name, srvCfg)
	}

	results := make([]ParallelToolDiscoveryResult, 0, len(servers))
	received := make(map[string]bool)
	total := len(servers)

	timeout := false
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	for receivedCount := 0; receivedCount < total; {
		select {
		case r := <-resultsCh:
			results = append(results, r)
			received[r.ServerName] = true
			receivedCount++
		case <-ctx.Done():
			timeout = true
			break
		case <-done:
			// All goroutines finished
			break
		}
		if timeout {
			break
		}
	}

	// If timeout, add missing servers as timeouts
	if timeout {
		for _, name := range servers {
			if !received[name] {
				results = append(results, ParallelToolDiscoveryResult{
					ServerName: name,
					Tools:      nil,
					Error:      fmt.Errorf("tool discovery timed out for this server"),
				})
			}
		}
	}

	// Drain any remaining results (if any)
	for len(results) < total {
		select {
		case r := <-resultsCh:
			results = append(results, r)
		default:
			break
		}
	}

	// Emit comprehensive cache event for all discovered servers
	// This ensures the frontend can see comprehensive cache status during active operations
	serverNames := make([]string, 0, len(servers))
	serverStatus := make(map[string]interface{})

	for _, result := range results {
		serverNames = append(serverNames, result.ServerName)
		status := "ok"
		if result.Error != nil {
			status = "error"
		}
		serverStatus[result.ServerName] = map[string]interface{}{
			"status":      status,
			"tools_count": len(result.Tools),
			"error":       result.Error,
		}
	}

	// Log comprehensive cache event for debugging
	logger.Infof("🔍 Comprehensive cache event for active tool discovery: servers_count=%d, servers=%v, total_tools=%d",
		len(serverNames), serverNames, len(results))

	// Note: To emit actual events, we would need to pass tracers to this function
	// For now, we log the information so it appears in the server logs

	return results
}
