// connection.go
//
// This file contains logic for establishing MCP client connections, discovering tools, and preparing all connection artifacts needed for agent construction.
//
// Exported:
//   - NewAgentConnection: Handles config loading, server connection, tool discovery, and returns clients, toolToServer map, tools, servers, and system prompt.

package mcpagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mcp-agent/agent_go/internal/observability"
	"mcp-agent/agent_go/internal/utils"
	"mcp-agent/agent_go/pkg/events"
	"mcp-agent/agent_go/pkg/mcpcache"
	"mcp-agent/agent_go/pkg/mcpclient"

	"mcp-agent/agent_go/internal/llmtypes"

	"github.com/mark3labs/mcp-go/mcp"
)

// NewAgentConnection handles MCP config loading, server connection, tool discovery, and returns all connection artifacts for agent construction.
// Now uses caching to avoid redundant connections and discoveries.
func NewAgentConnection(ctx context.Context, llm llmtypes.Model, serverName, configPath, traceID string, tracers []observability.Tracer, logger utils.ExtendedLogger, cacheOnly bool) (map[string]mcpclient.ClientInterface, map[string]string, []llmtypes.Tool, []string, map[string][]mcp.Prompt, map[string][]mcp.Resource, string, error) {

	// Start timing the entire connection process
	connectionStartTime := time.Now()

	// Emit detailed connection start event
	if len(tracers) > 0 {
		eventData := events.NewMCPServerConnectionEvent(serverName, "started", 0, 0, "")
		eventData.ConfigPath = configPath
		eventData.Operation = "connection_start_with_cache"
		eventData.Timestamp = connectionStartTime

		event := events.NewAgentEvent(eventData)
		event.Type = events.MCPServerConnectionStart // Use correct unified event type
		event.Timestamp = connectionStartTime        // Preserve the actual connection start time
		event.TraceID = traceID
		event.CorrelationID = fmt.Sprintf("conn-start-%s", traceID)

		for _, tracer := range tracers {
			if err := tracer.EmitEvent(event); err != nil {
				logger.Warnf("Failed to emit connection start event to tracer: %v", err)
			}
		}
	}

	logger.Info("🔍 NewAgentConnection started (with caching)", map[string]interface{}{
		"server_name": serverName,
		"config_path": configPath,
		"trace_id":    traceID,
		"start_time":  connectionStartTime.Format(time.RFC3339),
	})

	// Try to get cached or fresh connection data
	result, err := mcpcache.GetCachedOrFreshConnection(ctx, llm, serverName, configPath, tracers, logger, cacheOnly)
	if err != nil {
		connectionDuration := time.Since(connectionStartTime)

		// Emit connection failure event
		if len(tracers) > 0 {
			failureTime := time.Now()
			eventData := events.NewMCPServerConnectionEvent(serverName, "failed", 0, connectionDuration, err.Error())
			eventData.ConfigPath = configPath
			eventData.Operation = "connection_failed"
			eventData.Timestamp = failureTime

			event := events.NewAgentEvent(eventData)
			event.Type = events.MCPServerConnectionError // Use correct unified event type
			event.Timestamp = failureTime                // Preserve the failure time
			event.TraceID = traceID
			event.CorrelationID = fmt.Sprintf("conn-failed-%s", traceID)

			for _, tracer := range tracers {
				if err := tracer.EmitEvent(event); err != nil {
					logger.Warnf("Failed to emit connection failure event to tracer: %v", err)
				}
			}
		}

		logger.Errorf("❌ Connection failed: %v", err)
		return nil, nil, nil, nil, nil, nil, "", fmt.Errorf("connection failed: %w", err)
	}

	// Determine servers list
	var servers []string
	if serverName == "all" || serverName == "" {
		servers = make([]string, 0, len(result.Clients))
		for srvName := range result.Clients {
			servers = append(servers, srvName)
		}
	} else {
		servers = strings.Split(serverName, ",")
		for i, s := range servers {
			servers[i] = strings.TrimSpace(s)
		}
	}

	// Calculate total connection duration
	connectionDuration := time.Since(connectionStartTime)

	// Count total prompts and resources across all servers
	totalPrompts := 0
	totalResources := 0
	for _, prompts := range result.Prompts {
		totalPrompts += len(prompts)
	}
	for _, resources := range result.Resources {
		totalResources += len(resources)
	}

	// Log comprehensive connection statistics
	if result.CacheUsed {
		logger.Info("✅ Using cached connection data", map[string]interface{}{
			"cache_used":      result.CacheUsed,
			"fresh_fallback":  result.FreshFallback,
			"servers_count":   len(result.Clients),
			"tools_count":     len(result.Tools),
			"prompts_count":   totalPrompts,
			"resources_count": totalResources,
			"cache_key":       result.CacheKey,
			"connection_time": connectionDuration.String(),
			"connection_ms":   connectionDuration.Milliseconds(),
		})
	} else {
		logger.Info("🔄 Using fresh connection data", map[string]interface{}{
			"cache_used":      false,
			"fresh_fallback":  true,
			"servers_count":   len(result.Clients),
			"tools_count":     len(result.Tools),
			"prompts_count":   totalPrompts,
			"resources_count": totalResources,
			"connection_time": connectionDuration.String(),
			"connection_ms":   connectionDuration.Milliseconds(),
		})
	}

	// Emit detailed connection completion event
	if len(tracers) > 0 {
		status := "completed_with_cache"
		if !result.CacheUsed {
			status = "completed_fresh"
		}

		eventData := events.NewMCPServerConnectionEvent(serverName, status, len(result.Tools), connectionDuration, "")
		eventData.ConfigPath = configPath
		eventData.Operation = "connection_complete"
		eventData.ServerInfo = map[string]interface{}{
			"cache_used":          result.CacheUsed,
			"fresh_fallback":      result.FreshFallback,
			"servers_count":       len(result.Clients),
			"tools_count":         len(result.Tools),
			"prompts_count":       totalPrompts,
			"resources_count":     totalResources,
			"connection_time_ms":  connectionDuration.Milliseconds(),
			"cache_key":           result.CacheKey,
			"discovery_completed": true,
		}

		completionTime := time.Now()
		eventData.Timestamp = completionTime

		event := events.NewAgentEvent(eventData)
		event.Type = events.MCPServerConnectionEnd // Use correct unified event type
		event.Timestamp = completionTime           // Preserve the actual completion time
		event.TraceID = traceID
		event.CorrelationID = fmt.Sprintf("conn-complete-%s", traceID)

		for _, tracer := range tracers {
			if err := tracer.EmitEvent(event); err != nil {
				logger.Warnf("Failed to emit connection complete event to tracer: %v", err)
			}
		}

		// Emit discovery event for detailed discovery statistics
		discoveryEventData := events.NewMCPServerDiscoveryEvent(len(result.Clients), len(result.Clients), 0, connectionDuration)
		discoveryEventData.ServerName = serverName
		discoveryEventData.Operation = "mcp_discovery_complete"
		discoveryEventData.ToolCount = len(result.Tools)
		discoveryEventData.Timestamp = completionTime

		discoveryEvent := events.NewAgentEvent(discoveryEventData)
		discoveryEvent.Type = events.MCPServerDiscovery // Use correct unified event type
		discoveryEvent.Timestamp = completionTime       // Use same completion time
		discoveryEvent.TraceID = traceID
		discoveryEvent.CorrelationID = fmt.Sprintf("discovery-complete-%s", traceID)

		for _, tracer := range tracers {
			if err := tracer.EmitEvent(discoveryEvent); err != nil {
				logger.Warnf("Failed to emit discovery complete event to tracer: %v", err)
			}
		}
	}

	return result.Clients, result.ToolToServer, result.Tools, servers, result.Prompts, result.Resources, result.SystemPrompt, nil
}
