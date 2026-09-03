package agentsession

import (
	"os"
	"testing"
)

// The bridge's address and token must travel to the agent as explicit
// configuration, never via the process environment: a second executor in the
// same process (the main server's, if this package is hosted there) would
// otherwise overwrite it.
func TestSharedBridgeConfiguresAgentExplicitly(t *testing.T) {
	for _, key := range []string{"MCP_API_URL", "MCP_API_TOKEN", "MCP_BRIDGE_API_URL", "MCP_BRIDGE_BINARY"} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
	b := &sharedBridge{hostURL: "http://127.0.0.1:43210", apiToken: "tok-1", bridgePath: "/opt/mcpbridge"}
	cfg := b.runtimeMCPConfig("session-7")
	if cfg.SessionID != "session-7" || cfg.APIBaseURL != b.hostURL || cfg.BridgeAPIBaseURL != b.hostURL || cfg.APIToken != "tok-1" {
		t.Fatalf("explicit config not populated: %+v", cfg)
	}
	for _, key := range []string{"MCP_API_URL", "MCP_API_TOKEN", "MCP_BRIDGE_API_URL", "MCP_BRIDGE_BINARY"} {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			t.Fatalf("%s leaked into the process environment: %q", key, v)
		}
	}
}
