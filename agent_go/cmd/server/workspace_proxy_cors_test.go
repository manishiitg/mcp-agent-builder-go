package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The agent server's CORS middleware is the only thing allowed to speak
// CORS to the browser: a second Access-Control-Allow-Origin from the
// workspace server behind the proxy makes browsers reject the response.
func TestWorkspaceProxyStripsUpstreamCORSHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("X-Upstream", "kept")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer upstream.Close()
	t.Setenv("WORKSPACE_API_URL", upstream.URL)

	req := httptest.NewRequest(http.MethodGet, "/api/wp/api/documents/Chats/x.json", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserContextKey, &UserClaims{UserID: "u1"}))
	rec := httptest.NewRecorder()
	workspaceProxyHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Values("Access-Control-Allow-Origin"); len(got) != 0 {
		t.Fatalf("upstream CORS origin must be stripped, got %v", got)
	}
	if got := rec.Header().Values("Access-Control-Allow-Headers"); len(got) != 0 {
		t.Fatalf("upstream CORS headers must be stripped, got %v", got)
	}
	if rec.Header().Get("X-Upstream") != "kept" {
		t.Fatal("non-CORS upstream headers must pass through")
	}
}
