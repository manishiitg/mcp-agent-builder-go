package server

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/manishiitg/mcpagent/mcpclient"
	"github.com/manishiitg/mcpagent/oauth"
)

func TestClassifyConnectionFailureNoOAuthStaysError(t *testing.T) {
	srvCfg := mcpclient.MCPServerConfig{}
	status := classifyConnectionFailure("open-server", srvCfg, errors.New("connection refused"))

	if status.Status != "error" || status.RequiresOAuth {
		t.Fatalf("status = %#v, want a plain error (no OAuth configured)", status)
	}
	if status.Error != "connection refused" {
		t.Fatalf("error = %q, want the original connection error preserved", status.Error)
	}
}

func TestClassifyConnectionFailureNoTokenFileNeedsOAuth(t *testing.T) {
	srvCfg := mcpclient.MCPServerConfig{
		OAuth: &oauth.OAuthConfig{
			TokenFile: filepath.Join(t.TempDir(), "does-not-exist.json"),
		},
	}
	status := classifyConnectionFailure("oauth-server", srvCfg, errors.New("connection refused"))

	if status.Status != "not_connected" || !status.RequiresOAuth {
		t.Fatalf("status = %#v, want not_connected/RequiresOAuth when no token file exists", status)
	}
}

// This is the regression this fix addresses: a connected OAuth server hitting
// a transient failure must not get silently relabeled as "needs OAuth" and
// permanently marked will-not-retry by runBackgroundDiscovery's
// strings.Contains(errMsg, "OAuth") heuristic.
func TestClassifyConnectionFailureExistingTokenFileStaysError(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token.json")
	if err := os.WriteFile(tokenFile, []byte(`{"access_token":"x"}`), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	srvCfg := mcpclient.MCPServerConfig{
		OAuth: &oauth.OAuthConfig{TokenFile: tokenFile},
	}
	status := classifyConnectionFailure("oauth-server", srvCfg, errors.New("dial tcp: connection refused"))

	if status.Status != "error" || status.RequiresOAuth {
		t.Fatalf("status = %#v, want a genuine error preserved for an already-connected server's transient failure", status)
	}
	if status.Error != "dial tcp: connection refused" {
		t.Fatalf("error = %q, want the original connection error preserved, not rewritten to \"OAuth authentication required\"", status.Error)
	}
}
