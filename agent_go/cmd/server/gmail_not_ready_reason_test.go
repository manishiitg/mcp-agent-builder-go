package server

import (
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/services"
)

func TestGmailNotReadyReasonNamesTheMissingPiece(t *testing.T) {
	ok := services.GmailAuthStatus{GwsInstalled: true, Authenticated: true, HasGmailScope: true}
	cases := []struct {
		name string
		cfg  *services.GmailConfig
		auth services.GmailAuthStatus
		want string
	}{
		{"no gws", &services.GmailConfig{Enabled: true, DefaultTo: "a@b.c"}, services.GmailAuthStatus{}, "gws is not installed on the server."},
		{"not signed in", &services.GmailConfig{Enabled: true, DefaultTo: "a@b.c"}, services.GmailAuthStatus{GwsInstalled: true}, "No Gmail account is signed in."},
		{"no scope", &services.GmailConfig{Enabled: true, DefaultTo: "a@b.c"}, services.GmailAuthStatus{GwsInstalled: true, Authenticated: true}, "The signed-in account has no Gmail send scope."},
		{"switched off", &services.GmailConfig{DefaultTo: "a@b.c"}, ok, "The Gmail channel is switched off."},
	}
	for _, tc := range cases {
		if got := gmailNotReadyReason(tc.cfg, tc.auth); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
