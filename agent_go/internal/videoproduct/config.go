package videoproduct

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultAddr           = "127.0.0.1:8200"
	DefaultFrontendOrigin = "http://127.0.0.1:3200"
	DefaultWorkspaceURL   = "http://127.0.0.1:8201"
	DefaultClaudeModel    = "claude-sonnet-5"
)

type Config struct {
	Addr            string
	DataDir         string
	FrontendOrigin  string
	WorkspaceAPIURL string
	MCPConfigPath   string
	Runner          AgentRunner
}

func DefaultConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("find home directory: %w", err)
	}
	dataDir := strings.TrimSpace(os.Getenv("VIDEO_STUDIO_DATA_DIR"))
	if dataDir == "" {
		dataDir = filepath.Join(home, "VideoStudio")
	}
	addr := strings.TrimSpace(os.Getenv("VIDEO_STUDIO_ADDR"))
	if addr == "" {
		addr = DefaultAddr
	}
	origin := strings.TrimSpace(os.Getenv("VIDEO_STUDIO_FRONTEND_ORIGIN"))
	if origin == "" {
		origin = DefaultFrontendOrigin
	}
	workspaceURL := strings.TrimSpace(os.Getenv("VIDEO_STUDIO_WORKSPACE_API_URL"))
	if workspaceURL == "" {
		workspaceURL = DefaultWorkspaceURL
	}
	mcpConfigPath := strings.TrimSpace(os.Getenv("VIDEO_STUDIO_MCP_CONFIG"))
	if mcpConfigPath == "" {
		mcpConfigPath, _ = filepath.Abs(filepath.Join("configs", "mcp_servers_clean.json"))
	}
	return Config{Addr: addr, DataDir: dataDir, FrontendOrigin: origin, WorkspaceAPIURL: workspaceURL, MCPConfigPath: mcpConfigPath}, nil
}
