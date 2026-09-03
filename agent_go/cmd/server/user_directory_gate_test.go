package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func directoryGateRequest(t *testing.T, userID, username string) *httptest.ResponseRecorder {
	t.Helper()
	token, err := GenerateJWT(userID, username, "")
	if err != nil {
		t.Fatalf("GenerateJWT: %v", err)
	}
	handler := AuthMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/terminals", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// A valid token for an identity the directory does not know -- the ghost
// "default" user a browser keeps after a deployment migrates to real
// accounts -- must be refused with the wording the frontend keys on to drop
// the token, while the known admin sails through.
func TestAuthMiddlewareRefusesTokenForUserUnknownToTheDirectory(t *testing.T) {
	t.Setenv("MULTI_USER_MODE", "true")
	t.Setenv("AUTH_SECRET", "test-auth-secret-with-enough-entropy")
	withMemoryUserDirectory(t, `{"users":[
	  {"id":"aa73da63e26b40a1bb701c2b4c024870","username":"admin","admin":true,"can_create":true,"products":[]}
	]}`)

	if rr := directoryGateRequest(t, "aa73da63e26b40a1bb701c2b4c024870", "admin"); rr.Code != http.StatusNoContent {
		t.Fatalf("known admin: status %d body %s", rr.Code, rr.Body.String())
	}
	rr := directoryGateRequest(t, "default", "default")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("ghost user: status %d, want 401 (body %s)", rr.Code, rr.Body.String())
	}
	if body := strings.ToLower(rr.Body.String()); !strings.Contains(body, "expired") {
		t.Fatalf("ghost user: body must say the session expired so the client drops the token, got %s", rr.Body.String())
	}
}

// The gate is deliberately narrow: no directory adopted yet (empty), or a
// single-user deployment, keeps today's behaviour.
func TestAuthMiddlewareDirectoryGateIsNarrow(t *testing.T) {
	t.Setenv("AUTH_SECRET", "test-auth-secret-with-enough-entropy")

	t.Setenv("MULTI_USER_MODE", "true")
	withMemoryUserDirectory(t, `{"users":[]}`)
	if rr := directoryGateRequest(t, "default", "default"); rr.Code != http.StatusNoContent {
		t.Fatalf("empty directory must not gate: status %d body %s", rr.Code, rr.Body.String())
	}

	t.Setenv("MULTI_USER_MODE", "false")
	withMemoryUserDirectory(t, `{"users":[{"id":"a1","username":"alice","admin":true,"can_create":true,"products":[]}]}`)
	if rr := directoryGateRequest(t, "default", "default"); rr.Code != http.StatusNoContent {
		t.Fatalf("single-user mode must not gate: status %d body %s", rr.Code, rr.Body.String())
	}
}
