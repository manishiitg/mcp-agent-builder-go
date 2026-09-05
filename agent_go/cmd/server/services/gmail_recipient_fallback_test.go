package services

import "testing"

func TestGmailSelfRecipientFallback(t *testing.T) {
	g := &GmailService{config: &GmailConfig{
		DefaultConnectionID: "self",
		Connections:         []GmailConnection{{ID: "self", Email: "self@example.com", Enabled: true, Status: GmailConnectionConnected}},
	}}
	if got := g.pickRecipient(nil); got != "self@example.com" {
		t.Fatalf("fallback=%q", got)
	}
	explicit := &NotificationDestination{Gmail: &GmailDest{Email: "team@example.com"}}
	if got := g.pickRecipient(explicit); got != "team@example.com" {
		t.Fatalf("explicit=%q", got)
	}
	g.config.BlockedRecipients = []string{"team@example.com"}
	if got := g.pickRecipient(explicit); got != "" {
		t.Fatalf("blocked recipient was rerouted to %q", got)
	}
	g.config.BlockedRecipients = []string{"self@example.com"}
	if got := g.pickRecipient(nil); got != "" {
		t.Fatalf("self denylist ignored: %q", got)
	}
	g.config.DefaultConnectionID = ""
	if got := g.pickRecipient(nil); got != "" {
		t.Fatalf("arbitrary sender chosen: %q", got)
	}
}
