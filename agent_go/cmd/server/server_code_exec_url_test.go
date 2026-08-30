package server

import "testing"

func TestGetCodeExecAPIURLUsesExplicitDeploymentOverride(t *testing.T) {
	t.Setenv("MCP_API_URL", "http://127.0.0.1:8000/")
	api := &StreamingAPI{config: ServerConfig{Host: "0.0.0.0", Port: 8000}}

	if got, want := api.GetCodeExecAPIURL(), "http://127.0.0.1:8000"; got != want {
		t.Fatalf("GetCodeExecAPIURL() = %q, want %q", got, want)
	}
}
