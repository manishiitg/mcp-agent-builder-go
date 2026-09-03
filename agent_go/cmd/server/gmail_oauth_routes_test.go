package server

import "testing"

// Google redirects the user's browser to the callback as a plain navigation,
// which cannot carry our JWT, so the path must stay on the auth middleware's
// exemption list. This pins the coupling between the route constant and that
// list: if either side changes, this fails first.
func TestGmailOAuthCallbackIsExemptFromAuth(t *testing.T) {
	if !shouldSkipAuth(gmailOAuthCallbackPath) {
		t.Fatalf("%s must be exempt from auth (Google's redirect carries no JWT)", gmailOAuthCallbackPath)
	}
	// The exemption is for that one path, not the whole Gmail surface.
	if shouldSkipAuth("/api/human-feedback/gmail/connections") {
		t.Fatal("connection management routes must stay behind auth")
	}
}
