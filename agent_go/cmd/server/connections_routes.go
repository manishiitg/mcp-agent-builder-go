package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/manishiitg/mcpagent/mcpclient"
	"github.com/manishiitg/mcpagent/oauth"
)

// Connections is the user-facing layer over MCP servers. A "connection" is an
// approved account (Notion, Linear, Stripe); MCP is the transport underneath
// it. The catalog turns each integration into a one-click Connect so users
// never edit mcp.json by hand.

// Auth strategies. Every catalog entry uses DCR; the other two are only ever
// inferred for custom servers, which the user configures by hand.
const (
	// authDCR: the remote server supports Dynamic Client Registration, so the
	// OAuth client registers itself. Zero setup for both admin and user, and the
	// only strategy the catalog offers.
	authDCR = "dcr"
	// authToken: no OAuth flow; the credential is already in the server config.
	authToken = "token"
	// authNone: the server needs no credential at all.
	authNone = "none"
)

// How a server is reached. Surfaced as the "Type" column so users can tell a
// hosted integration from one running on their own machine.
const (
	transportWeb   = "web"
	transportLocal = "local"
)

// Connection health states surfaced to the UI.
const (
	healthConnected      = "connected"
	healthNeedsReconnect = "needs_reconnect"
	healthNotConnected   = "not_connected"
)

// CatalogEntry is one integration in the curated catalog.
type CatalogEntry struct {
	ID         string `json:"id"`
	ServerName string `json:"server_name"`
	Name       string `json:"name"`
	Tagline    string `json:"tagline,omitempty"`
	Icon       string `json:"icon,omitempty"`
	DocsURL    string `json:"docs_url,omitempty"`
	// "available" or "coming_soon". coming_soon renders disabled.
	Status string `json:"status,omitempty"`
	// Always "dcr". Kept explicit so a future strategy has somewhere to declare
	// itself rather than being inferred from the shape of the entry.
	Auth string `json:"auth"`

	// Transport
	URL     string   `json:"url,omitempty"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`

	// Scopes requested during dynamic client registration.
	Scopes []string `json:"scopes,omitempty"`

	// Computed per-request, never read from the catalog file.
	Transport string `json:"transport"`
}

type connectionsCatalog struct {
	Version      int            `json:"version"`
	Integrations []CatalogEntry `json:"integrations"`
}

// FriendlyError replaces raw transport failures with a recovery path. The raw
// text is preserved under Raw so the error UI can show the technical detail
// directly, without a disclosure step.
type FriendlyError struct {
	Code    string `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
	Raw     string `json:"raw,omitempty"`
}

// Connection is an integration the user has provisioned.
type Connection struct {
	ID         string         `json:"id"`
	ServerName string         `json:"server_name"`
	Name       string         `json:"name"`
	Icon       string         `json:"icon,omitempty"`
	Auth       string         `json:"auth"`
	Transport  string         `json:"transport"`
	Health     string         `json:"health"`
	ExpiresIn  string         `json:"expires_in,omitempty"`
	Custom     bool           `json:"custom"`
	Error      *FriendlyError `json:"error,omitempty"`
}

type connectionsListResponse struct {
	Connections []Connection `json:"connections"`
	Summary     struct {
		Connected      int `json:"connected"`
		NeedsAttention int `json:"needs_attention"`
		Total          int `json:"total"`
	} `json:"summary"`
}

// The catalog is a deploy-time file, so it is cached after the first read.
// Keyed by path so a changed CONNECTIONS_CATALOG_PATH (and each test) gets its
// own entry rather than reusing a stale one. Editing the file in place requires
// a restart.
var (
	catalogCache   = map[string]*connectionsCatalog{}
	catalogCacheMu sync.RWMutex
)

// connectionsCatalogPath resolves the catalog next to the MCP config, with an
// env override for deployments that mount configs elsewhere.
func (api *StreamingAPI) connectionsCatalogPath() string {
	if p := os.Getenv("CONNECTIONS_CATALOG_PATH"); p != "" {
		return p
	}
	return filepath.Join(filepath.Dir(api.mcpConfigPath), "connections_catalog.json")
}

func (api *StreamingAPI) loadCatalog() (*connectionsCatalog, error) {
	path := api.connectionsCatalogPath()

	catalogCacheMu.RLock()
	cached := catalogCache[path]
	catalogCacheMu.RUnlock()
	if cached != nil {
		return cached, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read connections catalog at %s: %w", path, err)
	}

	var cat connectionsCatalog
	if err := json.Unmarshal(data, &cat); err != nil {
		return nil, fmt.Errorf("failed to parse connections catalog: %w", err)
	}

	// Default the MCP server name to the catalog id so config keys stay stable.
	for i := range cat.Integrations {
		if cat.Integrations[i].ServerName == "" {
			cat.Integrations[i].ServerName = cat.Integrations[i].ID
		}
		if cat.Integrations[i].Status == "" {
			cat.Integrations[i].Status = "available"
		}
	}

	catalogCacheMu.Lock()
	catalogCache[path] = &cat
	catalogCacheMu.Unlock()
	return &cat, nil
}

func (api *StreamingAPI) findCatalogEntry(id string) (*CatalogEntry, error) {
	cat, err := api.loadCatalog()
	if err != nil {
		return nil, err
	}
	for i := range cat.Integrations {
		if cat.Integrations[i].ID == id {
			entry := cat.Integrations[i]
			return &entry, nil
		}
	}
	return nil, fmt.Errorf("integration %q not found in catalog", id)
}

// inferAuthKind decides which health check applies to a server. Catalog entries
// declare their strategy; custom servers added through the JSON editor have to
// be read off their own configuration, since assuming one strategy would either
// nag about credentials an open server never needed or silently pass an
// OAuth-backed server that has no token.
func inferAuthKind(entry *CatalogEntry, cfg mcpclient.MCPServerConfig) string {
	if entry != nil {
		return entry.Auth
	}
	switch {
	case cfg.OAuth != nil:
		return authDCR
	case len(cfg.Env) > 0 || len(cfg.Headers) > 0:
		return authToken
	default:
		return authNone
	}
}

// transportKind reports how a server is reached: a URL means a hosted service,
// a command means a process spawned locally.
func transportKind(url, command string) string {
	if url == "" && command != "" {
		return transportLocal
	}
	return transportWeb
}

// friendlyError maps a transport/auth failure onto a recovery path.
func friendlyError(serviceName string, status int, raw string) *FriendlyError {
	raw = strings.TrimSpace(raw)
	lower := strings.ToLower(raw)

	switch {
	case status == http.StatusUnauthorized || strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized"):
		return &FriendlyError{
			Code:    "unauthorized",
			Title:   fmt.Sprintf("%s needs reconnection", serviceName),
			Message: "Your sign-in has expired or was revoked. Reconnect to continue.",
			Action:  "reconnect",
			Raw:     raw,
		}
	case status == http.StatusForbidden || strings.Contains(lower, "403") || strings.Contains(lower, "forbidden"):
		return &FriendlyError{
			Code:    "forbidden",
			Title:   fmt.Sprintf("%s is missing a permission", serviceName),
			Message: "The connected account does not have access to this data. Reconnect and approve the requested access, or use an account with permission.",
			Action:  "reconnect",
			Raw:     raw,
		}
	case status == http.StatusNotFound || strings.Contains(lower, "404"):
		return &FriendlyError{
			Code:    "not_found",
			Title:   fmt.Sprintf("%s could not be found", serviceName),
			Message: "The service address is wrong or the integration has moved. Check the connection settings.",
			Raw:     raw,
		}
	case status == http.StatusTooManyRequests || strings.Contains(lower, "429"):
		return &FriendlyError{
			Code:    "rate_limited",
			Title:   fmt.Sprintf("%s is rate limiting requests", serviceName),
			Message: "Too many requests were sent. Wait a moment and try again.",
			Action:  "retry",
			Raw:     raw,
		}
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return &FriendlyError{
			Code:    "timeout",
			Title:   fmt.Sprintf("%s did not respond", serviceName),
			Message: "The service took too long to reply. It may be temporarily unavailable.",
			Action:  "retry",
			Raw:     raw,
		}
	case strings.Contains(lower, "no such host") || strings.Contains(lower, "connection refused") || strings.Contains(lower, "dial tcp"):
		return &FriendlyError{
			Code:    "unreachable",
			Title:   fmt.Sprintf("Could not reach %s", serviceName),
			Message: "Check your network connection and try again.",
			Action:  "retry",
			Raw:     raw,
		}
	case status >= 500:
		return &FriendlyError{
			Code:    "service_error",
			Title:   fmt.Sprintf("%s reported an error", serviceName),
			Message: "The service is having trouble right now. Try again shortly.",
			Action:  "retry",
			Raw:     raw,
		}
	default:
		return &FriendlyError{
			Code:    "unknown",
			Title:   fmt.Sprintf("Could not connect to %s", serviceName),
			Message: "Something went wrong while connecting. Try again.",
			Action:  "retry",
			Raw:     raw,
		}
	}
}

func writeFriendlyError(w http.ResponseWriter, status int, fe *FriendlyError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"error": fe})
}

// triggerDiscovery re-runs MCP tool discovery after a connection changes. It is
// indirected through a variable so tests can stub out the heavy background work
// that only a fully constructed server can perform.
var triggerDiscovery = func(api *StreamingAPI) {
	go api.triggerMCPDiscovery()
}

// handleGetConnectionsCatalog handles GET /api/connections/catalog
func (api *StreamingAPI) handleGetConnectionsCatalog(w http.ResponseWriter, r *http.Request) {
	cat, err := api.loadCatalog()
	if err != nil {
		api.logger.Error(fmt.Sprintf("Failed to load connections catalog: %v", err), err)
		writeFriendlyError(w, http.StatusInternalServerError, &FriendlyError{
			Code:    "catalog_unavailable",
			Title:   "Integration catalog unavailable",
			Message: "The list of available integrations could not be loaded. Custom MCP still works.",
			Action:  "retry",
			Raw:     err.Error(),
		})
		return
	}

	entries := make([]CatalogEntry, len(cat.Integrations))
	copy(entries, cat.Integrations)
	for i := range entries {
		entries[i].Transport = transportKind(entries[i].URL, entries[i].Command)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"version":      cat.Version,
		"integrations": entries,
	})
}

// tokenHealth reports OAuth token validity without the heavy discovery that
// /api/oauth/status performs, so listing many connections stays fast.
func tokenHealth(serverConfig mcpclient.MCPServerConfig, userTokenFile string) (bool, string) {
	oauthCfg := serverConfig.OAuth
	if oauthCfg == nil {
		oauthCfg = &oauth.OAuthConfig{UsePKCE: true}
	}
	// Copy so the caller's config is not mutated.
	cfg := *oauthCfg
	cfg.TokenFile = userTokenFile

	mgr := oauth.NewManager(&cfg, nil)
	valid, expiresIn, _ := mgr.GetTokenStatus()
	return valid, expiresIn
}

// handleGetConnections handles GET /api/connections
func (api *StreamingAPI) handleGetConnections(w http.ResponseWriter, r *http.Request) {
	userID := GetUserIDFromContext(r.Context())

	cat, err := api.loadCatalog()
	if err != nil {
		api.logger.Warn(fmt.Sprintf("Connections catalog unavailable, listing custom servers only: %v", err))
		cat = &connectionsCatalog{}
	}

	config, err := mcpclient.LoadMergedConfig(api.mcpConfigPath, api.logger)
	if err != nil {
		api.logger.Error(fmt.Sprintf("Failed to load MCP config: %v", err), err)
		writeFriendlyError(w, http.StatusInternalServerError, friendlyError("your workspace", http.StatusInternalServerError, err.Error()))
		return
	}

	// Map MCP server name -> catalog entry so custom servers can be told apart.
	byServer := make(map[string]*CatalogEntry, len(cat.Integrations))
	for i := range cat.Integrations {
		byServer[cat.Integrations[i].ServerName] = &cat.Integrations[i]
	}

	resp := connectionsListResponse{Connections: []Connection{}}

	names := make([]string, 0, len(config.MCPServers))
	for name := range config.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		serverConfig := config.MCPServers[name]
		entry := byServer[name]

		conn := Connection{
			ServerName: name,
			Name:       name,
			Custom:     entry == nil,
			Auth:       inferAuthKind(entry, serverConfig),
			Transport:  transportKind(serverConfig.URL, serverConfig.Command),
		}
		if entry != nil {
			conn.ID = entry.ID
			conn.Name = entry.Name
			conn.Icon = entry.Icon
		} else {
			conn.ID = name
		}

		switch {
		case conn.Auth == authNone:
			// Nothing to authenticate: an open server is healthy as soon as it
			// is configured. Reporting "needs attention" here would leave a
			// permanent false alarm in the header count.
			conn.Health = healthConnected
		case conn.Auth == authToken:
			// Only custom servers reach this: the credential is whatever the
			// user put in the config, so its presence is the whole check.
			if len(serverConfig.Env) > 0 || len(serverConfig.Headers) > 0 {
				conn.Health = healthConnected
			} else {
				conn.Health = healthNeedsReconnect
				conn.Error = friendlyError(conn.Name, http.StatusUnauthorized, "no credential stored for this connection")
			}
		default:
			userTokenFile := getUserTokenFilePath(userID, name)
			valid, expiresIn := tokenHealth(serverConfig, userTokenFile)
			if valid {
				conn.Health = healthConnected
				conn.ExpiresIn = expiresIn
			} else {
				conn.Health = healthNeedsReconnect
				conn.Error = friendlyError(conn.Name, http.StatusUnauthorized, "stored token is missing or expired")
			}
		}

		resp.Connections = append(resp.Connections, conn)
	}

	for _, c := range resp.Connections {
		resp.Summary.Total++
		if c.Health == healthConnected {
			resp.Summary.Connected++
		} else {
			resp.Summary.NeedsAttention++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// buildServerConfig turns a catalog entry into an MCP server config. Every
// entry authenticates through dynamic client registration, so the OAuth client
// registers itself and nothing has to be supplied up front.
func buildServerConfig(entry *CatalogEntry, userTokenFile string) mcpclient.MCPServerConfig {
	return mcpclient.MCPServerConfig{
		Description: entry.Tagline,
		URL:         entry.URL,
		Command:     entry.Command,
		Args:        entry.Args,
		OAuth: &oauth.OAuthConfig{
			AutoDiscover: true,
			UsePKCE:      true,
			Scopes:       entry.Scopes,
			TokenFile:    userTokenFile,
		},
	}
}

// saveUserServer adds or replaces one server in the user config, leaving every
// other user server untouched.
func (api *StreamingAPI) saveUserServer(name string, cfg mcpclient.MCPServerConfig) error {
	userConfigPath := api.getUserConfigPath()
	userConfig, err := mcpclient.LoadConfig(userConfigPath, api.logger)
	if err != nil || userConfig == nil {
		userConfig = &mcpclient.MCPConfig{MCPServers: make(map[string]mcpclient.MCPServerConfig)}
	}
	if userConfig.MCPServers == nil {
		userConfig.MCPServers = make(map[string]mcpclient.MCPServerConfig)
	}
	userConfig.MCPServers[name] = cfg
	return mcpclient.SaveConfig(userConfigPath, userConfig)
}

// removeUserServer deletes one server from the user config. Base servers are
// left alone — they are not the user's to remove.
func (api *StreamingAPI) removeUserServer(name string) error {
	userConfigPath := api.getUserConfigPath()
	userConfig, err := mcpclient.LoadConfig(userConfigPath, api.logger)
	if err != nil || userConfig == nil {
		return nil // nothing stored, nothing to remove
	}
	if userConfig.MCPServers == nil {
		return nil
	}
	delete(userConfig.MCPServers, name)
	return mcpclient.SaveConfig(userConfigPath, userConfig)
}

// captureWriter records a delegated handler's response so a raw error can be
// rewritten as a friendly one before it reaches the client.
type captureWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newCaptureWriter() *captureWriter {
	return &captureWriter{header: make(http.Header), status: http.StatusOK}
}

func (c *captureWriter) Header() http.Header         { return c.header }
func (c *captureWriter) WriteHeader(status int)      { c.status = status }
func (c *captureWriter) Write(b []byte) (int, error) { return c.body.Write(b) }

type connectRequest struct {
	// ClientID for the needs_client_id fallback, used when a server turns out
	// not to support dynamic client registration after all.
	ClientID string `json:"client_id,omitempty"`
}

// handleConnectIntegration handles POST /api/connections/{id}/connect.
// It provisions the MCP server config from the catalog, then delegates to the
// existing OAuth start flow so discovery/PKCE logic is not duplicated.
func (api *StreamingAPI) handleConnectIntegration(w http.ResponseWriter, r *http.Request) {
	if isMCPConfigLocked() {
		writeFriendlyError(w, http.StatusForbidden, &FriendlyError{
			Code:    "locked",
			Title:   "Connections are locked",
			Message: "An administrator has locked integration settings for this workspace.",
			Action:  "contact_admin",
		})
		return
	}

	id := mux.Vars(r)["id"]
	entry, err := api.findCatalogEntry(id)
	if err != nil {
		// Not a catalog integration. If a server by this name is already
		// configured — one added through Custom MCP — this is a reconnect, so
		// hand it to the OAuth flow rather than refusing it as unknown.
		if api.serverExists(id) {
			api.reconnectExistingServer(w, r, id)
			return
		}
		writeFriendlyError(w, http.StatusNotFound, &FriendlyError{
			Code:    "not_in_catalog",
			Title:   "Integration not available",
			Message: "This integration is not in the catalog. Use Custom MCP to add it manually.",
			Raw:     err.Error(),
		})
		return
	}

	if entry.Status == "coming_soon" {
		writeFriendlyError(w, http.StatusBadRequest, &FriendlyError{
			Code:    "coming_soon",
			Title:   fmt.Sprintf("%s is not available yet", entry.Name),
			Message: "This integration is still being set up.",
			Action:  "contact_admin",
		})
		return
	}

	var req connectRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // body is optional for DCR entries
	}

	userID := GetUserIDFromContext(r.Context())
	if _, err := ensureUserTokenDir(userID); err != nil {
		api.logger.Error(fmt.Sprintf("Failed to create token dir: %v", err), err)
		writeFriendlyError(w, http.StatusInternalServerError, friendlyError(entry.Name, http.StatusInternalServerError, err.Error()))
		return
	}

	userTokenFile := getUserTokenFilePath(userID, entry.ServerName)
	cfg := buildServerConfig(entry, userTokenFile)

	if err := api.saveUserServer(entry.ServerName, cfg); err != nil {
		api.logger.Error(fmt.Sprintf("Failed to save server config for %s: %v", entry.ServerName, err), err)
		writeFriendlyError(w, http.StatusInternalServerError, friendlyError(entry.Name, http.StatusInternalServerError, err.Error()))
		return
	}
	api.appendServerLog(entry.ServerName, "info", fmt.Sprintf("Connection provisioned from catalog entry %q", entry.ID))

	// Delegate to the existing OAuth start handler, preserving auth context and
	// forwarding headers so the redirect URI is derived the same way.
	body, _ := json.Marshal(OAuthLoginRequest{ServerName: entry.ServerName, ClientID: req.ClientID})
	delegated := r.Clone(r.Context())
	delegated.Body = io.NopCloser(bytes.NewReader(body))
	delegated.ContentLength = int64(len(body))

	rec := newCaptureWriter()
	api.handleOAuthStart(rec, delegated)

	if rec.status >= 400 {
		// Provisioning succeeded but auth could not start — roll the config back
		// so the card does not linger in a broken half-connected state.
		if err := api.removeUserServer(entry.ServerName); err != nil {
			api.logger.Warn(fmt.Sprintf("Failed to roll back server config for %s: %v", entry.ServerName, err))
		}
		writeFriendlyError(w, rec.status, friendlyError(entry.Name, rec.status, rec.body.String()))
		return
	}

	for k, vs := range rec.header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(rec.status)
	w.Write(rec.body.Bytes())
}

// serverExists reports whether a server by this name is configured, catalog
// entry or not.
func (api *StreamingAPI) serverExists(name string) bool {
	config, err := mcpclient.LoadMergedConfig(api.mcpConfigPath, api.logger)
	if err != nil {
		return false
	}
	_, err = config.GetServer(name)
	return err == nil
}

// reconnectExistingServer re-authenticates a server that is already configured
// but has no catalog entry to provision from. The config stays exactly as the
// user wrote it; only the OAuth flow runs.
func (api *StreamingAPI) reconnectExistingServer(w http.ResponseWriter, r *http.Request, name string) {
	var req connectRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	body, _ := json.Marshal(OAuthLoginRequest{ServerName: name, ClientID: req.ClientID})
	delegated := r.Clone(r.Context())
	delegated.Body = io.NopCloser(bytes.NewReader(body))
	delegated.ContentLength = int64(len(body))

	rec := newCaptureWriter()
	api.handleOAuthStart(rec, delegated)

	if rec.status >= 400 {
		// Nothing to roll back: this server was not provisioned by this call.
		writeFriendlyError(w, rec.status, friendlyError(name, rec.status, rec.body.String()))
		return
	}

	for k, vs := range rec.header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(rec.status)
	w.Write(rec.body.Bytes())
}

// handleDisconnectConnection handles POST /api/connections/{id}/disconnect.
// Removes the stored token but KEEPS the server config, so the card stays in
// the list showing "needs reconnection" with a one-click Reconnect.
func (api *StreamingAPI) handleDisconnectConnection(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	serverName := api.resolveServerName(id)
	userID := GetUserIDFromContext(r.Context())

	tokenFile := expandPath(getUserTokenFilePath(userID, serverName))
	if err := os.Remove(tokenFile); err != nil && !os.IsNotExist(err) {
		api.logger.Error(fmt.Sprintf("Failed to remove token for %s: %v", serverName, err), err)
		writeFriendlyError(w, http.StatusInternalServerError, friendlyError(serverName, http.StatusInternalServerError, err.Error()))
		return
	}

	api.appendServerLog(serverName, "info", "Disconnected — token removed, configuration kept")
	api.logger.Info(fmt.Sprintf("Disconnected user %s from %s (config retained)", userID, serverName))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":      "disconnected",
		"server_name": serverName,
		"message":     "Signed out. The connection is kept so you can reconnect in one click.",
	})
}

// handleTestConnection handles POST /api/connections/{id}/test — connects and
// lists tools so the user gets a concrete "it works" signal.
func (api *StreamingAPI) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	serverName := api.resolveServerName(id)
	displayName := serverName
	if entry, err := api.findCatalogEntry(id); err == nil {
		displayName = entry.Name
	}

	config, err := mcpclient.LoadMergedConfig(api.mcpConfigPath, api.logger)
	if err != nil {
		writeFriendlyError(w, http.StatusInternalServerError, friendlyError(displayName, http.StatusInternalServerError, err.Error()))
		return
	}

	serverConfig, err := config.GetServer(serverName)
	if err != nil {
		writeFriendlyError(w, http.StatusNotFound, &FriendlyError{
			Code:    "not_connected",
			Title:   fmt.Sprintf("%s is not connected", displayName),
			Message: "Connect the integration before testing it.",
			Action:  "connect",
			Raw:     err.Error(),
		})
		return
	}

	userID := GetUserIDFromContext(r.Context())
	if serverConfig.OAuth != nil {
		serverConfig.OAuth.TokenFile = getUserTokenFilePath(userID, serverName)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	client := mcpclient.New(serverConfig, api.logger)
	if err := client.Connect(ctx); err != nil {
		api.appendServerLog(serverName, "error", fmt.Sprintf("Test failed: %v", err))
		writeFriendlyError(w, http.StatusBadGateway, friendlyError(displayName, 0, err.Error()))
		return
	}
	defer client.Close()

	tools, err := client.ListTools(ctx)
	if err != nil {
		api.appendServerLog(serverName, "error", fmt.Sprintf("Test failed listing tools: %v", err))
		writeFriendlyError(w, http.StatusBadGateway, friendlyError(displayName, 0, err.Error()))
		return
	}

	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	api.appendServerLog(serverName, "info", fmt.Sprintf("Test succeeded: %d tools available", len(names)))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status":      "ok",
		"server_name": serverName,
		"tool_count":  len(names),
		"tools":       names,
		"message":     fmt.Sprintf("%s is working — %d tools available.", displayName, len(names)),
	})
}

// resolveServerName maps a catalog id to its MCP server name, falling back to
// the id itself so custom servers can use these endpoints too.
func (api *StreamingAPI) resolveServerName(id string) string {
	if entry, err := api.findCatalogEntry(id); err == nil {
		return entry.ServerName
	}
	return id
}
