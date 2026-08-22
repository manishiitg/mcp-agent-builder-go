package server

import (
	"net/http"
	"strings"
	"testing"
)

func TestFriendlyError401MapsToReconnect(t *testing.T) {
	fe := friendlyError("Google Workspace", http.StatusUnauthorized, "401 Unauthorized: token expired")

	if fe.Code != "unauthorized" {
		t.Errorf("code = %q, want %q", fe.Code, "unauthorized")
	}
	if fe.Title != "Google Workspace needs reconnection" {
		t.Errorf("title = %q, want %q", fe.Title, "Google Workspace needs reconnection")
	}
	if fe.Action != "reconnect" {
		t.Errorf("action = %q, want %q", fe.Action, "reconnect")
	}
	// The raw transport text must survive so the error UI can show it directly.
	if !strings.Contains(fe.Raw, "token expired") {
		t.Errorf("raw = %q, want it to preserve the original error", fe.Raw)
	}
}

func TestFriendlyErrorDetects401InBodyWithoutStatus(t *testing.T) {
	// The delegated OAuth handler often reports failures as text with no
	// meaningful HTTP status, so the body itself has to be classified.
	fe := friendlyError("Notion", 0, "failed to connect: server returned 401")
	if fe.Code != "unauthorized" {
		t.Errorf("code = %q, want %q", fe.Code, "unauthorized")
	}
}

func TestFriendlyErrorMappings(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		raw      string
		wantCode string
	}{
		{"403 forbidden", http.StatusForbidden, "forbidden", "forbidden"},
		{"404 not found", http.StatusNotFound, "not found", "not_found"},
		{"429 rate limited", http.StatusTooManyRequests, "slow down", "rate_limited"},
		{"timeout", 0, "context deadline exceeded", "timeout"},
		{"dns failure", 0, "dial tcp: no such host", "unreachable"},
		{"500 upstream", http.StatusInternalServerError, "internal error", "service_error"},
		{"unclassified", 0, "something odd happened", "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fe := friendlyError("Slack", tc.status, tc.raw)
			if fe.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", fe.Code, tc.wantCode)
			}
			if fe.Title == "" || fe.Message == "" {
				t.Error("friendly errors must always carry a title and message")
			}
			// A raw transport string must never be the user-facing message.
			if fe.Message == tc.raw {
				t.Errorf("message leaked the raw error: %q", fe.Message)
			}
		})
	}
}

func TestBuildServerConfigUsesAutoDiscovery(t *testing.T) {
	entry := &CatalogEntry{
		ID:      "notion",
		Auth:    authDCR,
		Name:    "Notion",
		Tagline: "Pages and databases",
		URL:     "https://mcp.notion.com/mcp",
	}

	cfg := buildServerConfig(entry, "~/.config/mcpagent/tokens/u1/notion.json")

	if cfg.OAuth == nil {
		t.Fatal("a catalog entry must produce an OAuth config")
	}
	if !cfg.OAuth.AutoDiscover {
		t.Error("dynamic client registration requires auto-discovery")
	}
	if !cfg.OAuth.UsePKCE {
		t.Error("PKCE must be on")
	}
	if cfg.OAuth.ClientID != "" || cfg.OAuth.ClientSecret != "" {
		t.Error("the client registers itself; no credential may be baked in")
	}
	if cfg.OAuth.TokenFile != "~/.config/mcpagent/tokens/u1/notion.json" {
		t.Errorf("token file = %q, want the per-user path", cfg.OAuth.TokenFile)
	}
	if cfg.URL != entry.URL {
		t.Errorf("url = %q, want %q", cfg.URL, entry.URL)
	}
	// The tagline is the only description the config carries.
	if cfg.Description != "Pages and databases" {
		t.Errorf("description = %q, want the tagline", cfg.Description)
	}
	if len(cfg.Env) > 0 {
		t.Error("nothing is supplied up front, so no env may be written")
	}
}

func TestCaptureWriterRecordsStatusAndBody(t *testing.T) {
	rec := newCaptureWriter()
	rec.Header().Set("X-Test", "1")
	rec.WriteHeader(http.StatusBadRequest)
	rec.Write([]byte("boom"))

	if rec.status != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.status, http.StatusBadRequest)
	}
	if rec.body.String() != "boom" {
		t.Errorf("body = %q, want %q", rec.body.String(), "boom")
	}
	if rec.Header().Get("X-Test") != "1" {
		t.Error("headers must be recorded so they can be replayed")
	}
}

func TestCaptureWriterDefaultsTo200(t *testing.T) {
	// A handler that writes a body without calling WriteHeader implies 200;
	// if this defaulted to 0 the connect handler would treat success as failure.
	rec := newCaptureWriter()
	rec.Write([]byte(`{"auth_url":"https://example.com"}`))
	if rec.status != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.status, http.StatusOK)
	}
}
