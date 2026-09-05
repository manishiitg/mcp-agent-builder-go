package services

import "testing"

// An authenticated gws with the Gmail scope switches the channel on by itself;
// a deliberate disable in the settings form is the one thing that stops it.
func TestShouldAutoEnableGmail(t *testing.T) {
	ready := GmailAuthStatus{GwsInstalled: true, Authenticated: true, HasGmailScope: true, Email: "a@b.c"}
	cases := []struct {
		name string
		cfg  *GmailConfig
		st   GmailAuthStatus
		want bool
	}{
		{"authenticated host enables a fresh config", &GmailConfig{}, ready, true},
		{"already enabled is a no-op", &GmailConfig{Enabled: true}, ready, false},
		{"operator disabled it on purpose", &GmailConfig{ManuallyDisabled: true}, ready, false},
		{"gws missing", &GmailConfig{}, GmailAuthStatus{Authenticated: true, HasGmailScope: true}, false},
		{"not authenticated", &GmailConfig{}, GmailAuthStatus{GwsInstalled: true, HasGmailScope: true}, false},
		{"no gmail scope", &GmailConfig{}, GmailAuthStatus{GwsInstalled: true, Authenticated: true}, false},
		{"nil config", nil, ready, false},
	}
	for _, tc := range cases {
		if got := shouldAutoEnableGmail(tc.cfg, tc.st); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A gws login already names one Google account; the recipient defaults to
// that same address the first time, and never again once a value is set —
// including an operator who deliberately blanked it back out.
func TestAutoDefaultGmailRecipient(t *testing.T) {
	authenticated := GmailAuthStatus{Authenticated: true, HasGmailScope: true, Email: "trader@example.com"}
	cases := []struct {
		name      string
		cfg       *GmailConfig
		st        GmailAuthStatus
		wantEmail string
		wantOK    bool
	}{
		{"fresh config with no recipient defaults to the authenticated address", &GmailConfig{}, authenticated, "trader@example.com", true},
		{"an existing recipient is never overridden", &GmailConfig{DefaultTo: "ops@example.com"}, authenticated, "", false},
		{"not authenticated", &GmailConfig{}, GmailAuthStatus{HasGmailScope: true}, "", false},
		{"no gmail scope", &GmailConfig{}, GmailAuthStatus{Authenticated: true}, "", false},
		{"authenticated but no address known", &GmailConfig{}, GmailAuthStatus{Authenticated: true, HasGmailScope: true}, "", false},
		{"nil config", nil, authenticated, "", false},
	}
	for _, tc := range cases {
		email, ok := autoDefaultGmailRecipient(tc.cfg, tc.st)
		if ok != tc.wantOK || email != tc.wantEmail {
			t.Errorf("%s: got (%q, %v), want (%q, %v)", tc.name, email, ok, tc.wantEmail, tc.wantOK)
		}
	}
}
