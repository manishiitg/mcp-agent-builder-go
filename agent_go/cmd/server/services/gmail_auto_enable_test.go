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
