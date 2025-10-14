package timeout

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TimeoutCmd represents the timeout server command
var TimeoutCmd = &cobra.Command{
	Use:   "timeout",
	Short: "Start the MCP timeout server with stdio or SSE protocol",
	Long: `Start the MCP timeout server that provides timeout testing capabilities via stdio or SSE protocol.
	
This server provides:
- Mock timeout tool that sleeps for 10 seconds
- Useful for testing MCP server connection timeouts
- Stdio or SSE transport protocol
- Simple and lightweight implementation

Examples:
  mcp-agent timeout                    # Start timeout server with stdio transport
  mcp-agent timeout --transport sse   # Start timeout server with SSE transport
  mcp-agent timeout --transport sse --port 9091  # Start SSE server on port 9091`,
	Run: runTimeoutServer,
}

func init() {
	// Add timeout server command flags
	TimeoutCmd.Flags().String("transport", "stdio", "Transport protocol (stdio or sse)")
	TimeoutCmd.Flags().Int("port", 9091, "Port for SSE transport (default: 9091)")

	// Bind flags to viper
	viper.BindPFlags(TimeoutCmd.Flags())
}

func runTimeoutServer(cmd *cobra.Command, args []string) {
	// Create MCP server
	s := server.NewMCPServer(
		"Timeout Server",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Add mock timeout tool
	s.AddTool(
		mcp.NewTool(
			"mock_timeout",
			mcp.WithDescription("Mock tool that sleeps for 30 seconds to test connection timeouts"),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			// Log start of timeout
			fmt.Fprintf(os.Stderr, "Mock timeout tool called - starting 30 second sleep...\n")

			// Sleep for 30 seconds
			time.Sleep(30 * time.Second)

			// Log completion
			fmt.Fprintf(os.Stderr, "Mock timeout tool completed after 30 seconds\n")

			// Return success message
			return mcp.NewToolResultText("Mock timeout completed successfully after 30 seconds"), nil
		},
	)

	// Start server based on transport type
	transport := viper.GetString("transport")
	port := viper.GetInt("port")

	switch transport {
	case "stdio":
		fmt.Fprintf(os.Stderr, "Starting MCP Timeout Server with stdio transport...\n")
		if err := server.ServeStdio(s); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	case "sse":
		fmt.Fprintf(os.Stderr, "Starting MCP Timeout Server with SSE transport on port %d...\n", port)
		addr := fmt.Sprintf(":%d", port)
		if err := serveSSE(s, addr); err != nil {
			fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Invalid transport type: %s. Use 'stdio' or 'sse'\n", transport)
		os.Exit(1)
	}
}

// serveSSE starts an HTTP server with SSE support for the MCP server
func serveSSE(s *server.MCPServer, addr string) error {
	// Create HTTP server
	http.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

		// Create a channel to signal when the connection is closed
		notify := w.(http.CloseNotifier).CloseNotify()

		// Send initial connection event
		fmt.Fprintf(w, "event: connected\ndata: {\"status\": \"connected\"}\n\n")
		w.(http.Flusher).Flush()

		// Keep connection alive and handle MCP requests
		for {
			select {
			case <-notify:
				// Client disconnected
				return
			default:
				// Send heartbeat to keep connection alive
				fmt.Fprintf(w, "event: heartbeat\ndata: {\"timestamp\": \"%s\"}\n\n", time.Now().Format(time.RFC3339))
				w.(http.Flusher).Flush()
				time.Sleep(30 * time.Second)
			}
		}
	})

	// Start HTTP server
	fmt.Fprintf(os.Stderr, "SSE server listening on %s\n", addr)
	return http.ListenAndServe(addr, nil)
}
