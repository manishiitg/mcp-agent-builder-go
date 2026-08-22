package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
	"github.com/manishiitg/mcpagent/mcpclient"
	"github.com/manishiitg/mcpagent/oauth"
)

const testCatalogJSON = `{
  "version": 1,
  "integrations": [
    {
      "id": "notion",
      "name": "Notion",
      "tagline": "Pages and databases",
      "icon": "notion",
      "status": "available",
      "auth": "dcr",
      "url": "https://mcp.notion.com/mcp"
    },
    {
      "id": "linear",
      "server_name": "linear",
      "name": "Linear",
      "tagline": "Issues and projects",
      "icon": "linear",
      "auth": "dcr",
      "url": "https://mcp.linear.app/mcp"
    },
    {
      "id": "not-yet",
      "name": "Not Yet",
      "status": "coming_soon",
      "auth": "dcr",
      "url": "https://example.com/mcp"
    }
  ]
}`

// newTestAPI builds a StreamingAPI backed by temp config files, plus a router
// wired exactly like production so path variables resolve.
func newTestAPI(t *testing.T) (*StreamingAPI, *mux.Router) {
	t.Helper()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp_servers.json")

	if err := os.WriteFile(configPath, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatalf("write base config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "connections_catalog.json"), []byte(testCatalogJSON), 0o600); err != nil {
		t.Fatalf("write catalog: %v", err)
	}

	api := &StreamingAPI{
		mcpConfigPath: configPath,
		logger:        loggerv2.NewNoop(),
		mcpConfig:     &mcpclient.MCPConfig{MCPServers: map[string]mcpclient.MCPServerConfig{}},
		// Mirrors production construction; appendServerLog writes into this map.
		serverLogs: map[string][]ServerLogEntry{},
	}

	// Background discovery needs a fully constructed server; stub it so these
	// handler tests stay hermetic and do not leak goroutines across tests.
	prevTrigger := triggerDiscovery
	triggerDiscovery = func(*StreamingAPI) {}
	t.Cleanup(func() { triggerDiscovery = prevTrigger })

	r := mux.NewRouter()
	r.HandleFunc("/api/connections/catalog", api.handleGetConnectionsCatalog).Methods("GET")
	r.HandleFunc("/api/connections", api.handleGetConnections).Methods("GET")
	r.HandleFunc("/api/connections/{id}/connect", api.handleConnectIntegration).Methods("POST")
	r.HandleFunc("/api/connections/{id}/disconnect", api.handleDisconnectConnection).Methods("POST")

	return api, r
}

func doJSON(t *testing.T, r *mux.Router, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var parsed map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
	return rec, parsed
}

func TestCatalogEndpointFillsDefaults(t *testing.T) {
	_, router := newTestAPI(t)
	rec, body := doJSON(t, router, "GET", "/api/connections/catalog", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rec.Code, rec.Body.String())
	}

	integrations, _ := body["integrations"].([]any)
	if len(integrations) != 3 {
		t.Fatalf("got %d integrations, want 3", len(integrations))
	}

	byID := map[string]map[string]any{}
	for _, raw := range integrations {
		e := raw.(map[string]any)
		byID[e["id"].(string)] = e
	}

	// server_name defaults to the id so config keys stay stable.
	if byID["notion"]["server_name"] != "notion" {
		t.Errorf("server_name = %v, want %q", byID["notion"]["server_name"], "notion")
	}
	// An entry that declares no status is available.
	if byID["linear"]["status"] != "available" {
		t.Errorf("status = %v, want %q", byID["linear"]["status"], "available")
	}
	// Transport is computed per request, never read from the file.
	if byID["notion"]["transport"] != "web" {
		t.Errorf("transport = %v, want %q", byID["notion"]["transport"], "web")
	}
}

func TestConnectComingSoonIsBlocked(t *testing.T) {
	_, router := newTestAPI(t)

	rec, body := doJSON(t, router, "POST", "/api/connections/not-yet/connect", `{}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "coming_soon" {
		t.Errorf("code = %v, want %q", errObj["code"], "coming_soon")
	}
}

func TestConnectUnknownIntegrationPointsToCustomMCP(t *testing.T) {
	_, router := newTestAPI(t)

	rec, body := doJSON(t, router, "POST", "/api/connections/does-not-exist/connect", `{}`)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "not_in_catalog" {
		t.Errorf("code = %v, want %q", errObj["code"], "not_in_catalog")
	}
	// There is no action to offer — the message is what points at Custom MCP.
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "Custom MCP") {
		t.Errorf("message = %q, want it to point at Custom MCP", msg)
	}
}

func TestConnectReconnectsConfiguredServerNotInCatalog(t *testing.T) {
	api, router := newTestAPI(t)

	// A custom OAuth server added through the JSON editor. Its row shows a
	// Reconnect button, which must reach the OAuth flow rather than being
	// refused as an unknown integration.
	if err := api.saveUserServer("legacy-oauth-server", mcpclient.MCPServerConfig{
		URL:   "https://127.0.0.1:1/mcp", // unreachable on purpose; discovery will fail
		OAuth: &oauth.OAuthConfig{AutoDiscover: true, UsePKCE: true},
	}); err != nil {
		t.Fatalf("save custom server: %v", err)
	}

	rec, body := doJSON(t, router, "POST", "/api/connections/legacy-oauth-server/connect", `{}`)

	// The endpoint is unreachable, so this cannot succeed — but it must fail as
	// a connection problem, never as "not in catalog".
	errObj, _ := body["error"].(map[string]any)
	if errObj != nil && errObj["code"] == "not_in_catalog" {
		t.Errorf("a configured server must not be rejected as not_in_catalog: %v", errObj)
	}
	if rec.Code == http.StatusNotFound {
		t.Errorf("status = 404, want the OAuth flow to be attempted. body: %s", rec.Body.String())
	}

	// The config must survive a failed reconnect — it was not provisioned here,
	// so there is nothing to roll back.
	userCfg, err := mcpclient.LoadConfig(api.getUserConfigPath(), api.logger)
	if err != nil {
		t.Fatalf("load user config: %v", err)
	}
	if _, ok := userCfg.MCPServers["legacy-oauth-server"]; !ok {
		t.Error("a failed reconnect must not delete the existing server configuration")
	}
}

func TestConnectStillRejectsTrulyUnknownIntegration(t *testing.T) {
	_, router := newTestAPI(t)

	rec, body := doJSON(t, router, "POST", "/api/connections/never-heard-of-it/connect", `{}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "not_in_catalog" {
		t.Errorf("code = %v, want %q", errObj["code"], "not_in_catalog")
	}
}

func TestListJoinsCatalogMetadataAndSummarises(t *testing.T) {
	api, router := newTestAPI(t)

	// Provisioned but never authorised: no token file exists yet.
	if err := api.saveUserServer("notion", mcpclient.MCPServerConfig{
		URL:   "https://mcp.notion.com/mcp",
		OAuth: &oauth.OAuthConfig{AutoDiscover: true, UsePKCE: true},
	}); err != nil {
		t.Fatalf("save server: %v", err)
	}

	rec, body := doJSON(t, router, "GET", "/api/connections", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rec.Code, rec.Body.String())
	}

	conns, _ := body["connections"].([]any)
	if len(conns) != 1 {
		t.Fatalf("got %d connections, want 1", len(conns))
	}
	c := conns[0].(map[string]any)
	if c["id"] != "notion" {
		t.Errorf("id = %v, want %q", c["id"], "notion")
	}
	// Catalog metadata must be joined onto the connection for the row.
	if c["name"] != "Notion" {
		t.Errorf("name = %v, want %q", c["name"], "Notion")
	}
	if c["custom"] != false {
		t.Error("a catalog-backed connection must not be marked custom")
	}
	// Provisioned without a stored token, so it needs reconnecting.
	if c["health"] != "needs_reconnect" {
		t.Errorf("health = %v, want %q", c["health"], "needs_reconnect")
	}

	summary, _ := body["summary"].(map[string]any)
	if summary["needs_attention"] != float64(1) || summary["total"] != float64(1) {
		t.Errorf("summary = %v, want 1 needing attention of 1 total", summary)
	}
}

func TestListMarksNonCatalogServersAsCustom(t *testing.T) {
	api, router := newTestAPI(t)

	// A server added by hand through Custom MCP / the JSON editor.
	if err := api.saveUserServer("my-internal-tool", mcpclient.MCPServerConfig{
		URL: "https://internal.example.com/mcp",
	}); err != nil {
		t.Fatalf("save custom server: %v", err)
	}

	_, body := doJSON(t, router, "GET", "/api/connections", "")
	conns, _ := body["connections"].([]any)
	if len(conns) != 1 {
		t.Fatalf("got %d connections, want 1", len(conns))
	}
	c := conns[0].(map[string]any)
	if c["custom"] != true {
		t.Error("a server with no catalog entry must be marked custom")
	}
	// A bare URL server needs no credential, so it is healthy as configured.
	// Reporting "needs attention" would be a permanent false alarm.
	if c["auth"] != "none" {
		t.Errorf("auth = %v, want %q", c["auth"], "none")
	}
	if c["health"] != "connected" {
		t.Errorf("health = %v, want %q", c["health"], "connected")
	}
	if c["error"] != nil {
		t.Errorf("an open server must not carry an error: %v", c["error"])
	}
}

func TestListInfersOAuthForCustomServerWithOAuthConfig(t *testing.T) {
	api, router := newTestAPI(t)

	// A custom server carrying an OAuth block must be judged on its token file,
	// not treated as a token/no-auth server.
	if err := api.saveUserServer("legacy-oauth-server", mcpclient.MCPServerConfig{
		URL:   "https://legacy.example.com/mcp",
		OAuth: &oauth.OAuthConfig{AutoDiscover: true, UsePKCE: true},
	}); err != nil {
		t.Fatalf("save custom server: %v", err)
	}

	_, body := doJSON(t, router, "GET", "/api/connections", "")
	conns, _ := body["connections"].([]any)
	if len(conns) != 1 {
		t.Fatalf("got %d connections, want 1", len(conns))
	}
	c := conns[0].(map[string]any)
	if c["auth"] == "token" || c["auth"] == "none" {
		t.Errorf("auth = %v, want an OAuth strategy", c["auth"])
	}
	// No token has been stored, so it genuinely does need reconnecting.
	if c["health"] != "needs_reconnect" {
		t.Errorf("health = %v, want %q", c["health"], "needs_reconnect")
	}
}

func TestListInfersTokenAuthForCustomServerWithEnv(t *testing.T) {
	api, router := newTestAPI(t)

	if err := api.saveUserServer("env-backed-server", mcpclient.MCPServerConfig{
		Command: "npx",
		Args:    []string{"-y", "some-server"},
		Env:     map[string]string{"SOME_API_KEY": "secret"},
	}); err != nil {
		t.Fatalf("save custom server: %v", err)
	}

	_, body := doJSON(t, router, "GET", "/api/connections", "")
	c := body["connections"].([]any)[0].(map[string]any)
	if c["auth"] != "token" {
		t.Errorf("auth = %v, want %q", c["auth"], "token")
	}
	if c["health"] != "connected" {
		t.Errorf("health = %v, want %q", c["health"], "connected")
	}
}

func TestDisconnectKeepsServerConfig(t *testing.T) {
	api, router := newTestAPI(t)

	if err := api.saveUserServer("notion", mcpclient.MCPServerConfig{
		URL:   "https://mcp.notion.com/mcp",
		OAuth: &oauth.OAuthConfig{AutoDiscover: true, UsePKCE: true},
	}); err != nil {
		t.Fatalf("save server: %v", err)
	}

	rec, _ := doJSON(t, router, "POST", "/api/connections/notion/disconnect", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rec.Code, rec.Body.String())
	}

	// This is the whole point of Disconnect vs Remove: the connection survives
	// so the card can offer a one-click Reconnect.
	userCfg, err := mcpclient.LoadConfig(api.getUserConfigPath(), api.logger)
	if err != nil {
		t.Fatalf("load user config: %v", err)
	}
	if _, ok := userCfg.MCPServers["notion"]; !ok {
		t.Error("disconnect must keep the server configuration")
	}
}

func TestConnectIsBlockedWhenMCPConfigLocked(t *testing.T) {
	t.Setenv("MCP_CONFIG_LOCKED", "true")
	_, router := newTestAPI(t)

	rec, body := doJSON(t, router, "POST", "/api/connections/notion/connect", `{}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["code"] != "locked" {
		t.Errorf("code = %v, want %q", errObj["code"], "locked")
	}
}
