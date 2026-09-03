package server

import (
	"path/filepath"
	"testing"
)

// Per-user MCP token paths must follow XDG_CONFIG_HOME on hosts where the
// service user cannot write ~/.config, and stay on the historical path
// otherwise. The mcpagent library resolves the same variable, so the
// server's writes and the runtime's reads agree.
func TestMCPTokenPathsHonourXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/srv/state/xdg")
	if got := getUserTokenFilePath("u1", "notion"); got != filepath.Join("/srv/state/xdg", "mcpagent", "tokens", "u1", "notion.json") {
		t.Fatalf("with XDG: %q", got)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/svc")
	if got := getUserTokenFilePath("u1", "notion"); got != "/home/svc/.config/mcpagent/tokens/u1/notion.json" {
		t.Fatalf("without XDG: %q", got)
	}
}
