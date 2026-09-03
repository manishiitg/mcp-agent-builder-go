package services

import (
	"path/filepath"
	"testing"
)

// The refresh-token directory follows XDG_CONFIG_HOME like the MCP connector
// tokens do: on RTS ~/.config is root-owned and only the XDG tree is writable.
func TestGmailOAuthTokenDirHonoursXDGConfigHome(t *testing.T) {
	t.Setenv("GMAIL_OAUTH_TOKEN_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "/srv/xdg")
	if got, want := gmailOAuthTokenDir(), filepath.Join("/srv/xdg", "agentworks", "gmail-oauth"); got != want {
		t.Fatalf("token dir = %q, want %q", got, want)
	}
	t.Setenv("GMAIL_OAUTH_TOKEN_DIR", "/explicit/dir")
	if got := gmailOAuthTokenDir(); got != "/explicit/dir" {
		t.Fatalf("explicit override must win, got %q", got)
	}
}
