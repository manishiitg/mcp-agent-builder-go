package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestAgentTokenIsShortLivedAndSignedWithGatewaySecret(t *testing.T) {
	gateway := &gateway{secret: []byte("test-secret-that-is-long-enough")}
	raw, err := gateway.agentToken()
	if err != nil {
		t.Fatalf("agentToken: %v", err)
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("token parts = %d, want 3", len(parts))
	}
	mac := hmac.New(sha256.New, gateway.secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	wantSignature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(wantSignature)) {
		t.Fatal("token signature does not verify")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	claims := &gatewayClaims{}
	if err := json.Unmarshal(payload, claims); err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	if claims.UserID != "video-studio" || claims.Username != "video-studio" || claims.Provider != "gateway" {
		t.Fatalf("claims = %#v", claims)
	}
	untilExpiry := time.Until(time.Unix(claims.ExpiresAt, 0))
	if untilExpiry <= 0 || untilExpiry > 16*time.Minute {
		t.Fatalf("unexpected expiration: %d", claims.ExpiresAt)
	}
}

func TestServeAgentForwardsQueryTokenAsBearerCredential(t *testing.T) {
	var gotAuthorization string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthorization = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	gateway := &gateway{
		secret: []byte("test-secret-that-is-long-enough"),
		agent:  httputil.NewSingleHostReverseProxy(target),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/public/file?token=workspace-user-token", nil)
	response := httptest.NewRecorder()
	gateway.serveAgent(response, req)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
	if gotAuthorization != "Bearer workspace-user-token" {
		t.Fatalf("authorization = %q", gotAuthorization)
	}
}

func TestUnauthenticatedAPIRequestReturnsExplicitLoginSignal(t *testing.T) {
	gateway := &gateway{secret: []byte("test-secret-that-is-long-enough")}
	req := httptest.NewRequest(http.MethodGet, "/api/health?full=1", nil)
	req.Header.Set("Referer", "https://example.com/projects/123?tab=chat")
	response := httptest.NewRecorder()

	gateway.ServeHTTP(response, req)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
	if got := response.Header().Get(authRequiredHeader); got != "/login?next=%2Fprojects%2F123%3Ftab%3Dchat" {
		t.Fatalf("login header = %q", got)
	}
	if got := strings.TrimSpace(response.Body.String()); got != `{"error":"authentication_required"}` {
		t.Fatalf("body = %q", got)
	}
}

func TestUnauthenticatedAPIRequestIgnoresExternalReferer(t *testing.T) {
	gateway := &gateway{secret: []byte("test-secret-that-is-long-enough")}
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Header.Set("Referer", "https://attacker.example/steal")
	response := httptest.NewRecorder()

	gateway.ServeHTTP(response, req)

	if got := response.Header().Get(authRequiredHeader); got != "/login?next=%2F" {
		t.Fatalf("login header = %q", got)
	}
}

func TestUnauthenticatedFrontendRequestStillRedirectsToLogin(t *testing.T) {
	gateway := &gateway{secret: []byte("test-secret-that-is-long-enough")}
	req := httptest.NewRequest(http.MethodGet, "/projects/123", nil)
	response := httptest.NewRecorder()

	gateway.ServeHTTP(response, req)

	if response.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusSeeOther)
	}
	if got := response.Header().Get("Location"); got != "/login?next=%2Fprojects%2F123" {
		t.Fatalf("location = %q", got)
	}
}

func TestAuthenticatedRequestRefreshesSessionNearExpiry(t *testing.T) {
	gateway := &gateway{
		secret:      []byte("test-secret-that-is-long-enough"),
		frontendDir: t.TempDir(),
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookie, Value: gateway.signedSession(time.Now().Add(time.Hour))})
	response := httptest.NewRecorder()

	gateway.ServeHTTP(response, req)

	result := response.Result()
	defer result.Body.Close()
	var refreshed *http.Cookie
	for _, cookie := range result.Cookies() {
		if cookie.Name == sessionCookie {
			refreshed = cookie
			break
		}
	}
	if refreshed == nil {
		t.Fatal("expected a refreshed session cookie")
	}
	if remaining := time.Until(refreshed.Expires); remaining < 11*time.Hour || remaining > 13*time.Hour {
		t.Fatalf("refreshed session lifetime = %s", remaining)
	}
}
