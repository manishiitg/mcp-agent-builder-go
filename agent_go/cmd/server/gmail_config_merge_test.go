package server

import (
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/services"
)

// Saving the settings form must not wipe the connections registry: the form
// only carries channel fields, and the handler used to write a fresh config
// built from them alone.
func TestMergeGmailConfigRequestKeepsRegistryState(t *testing.T) {
	current := &services.GmailConfig{
		Enabled:   false,
		DefaultTo: "old@example.com",
		Connections: []services.GmailConnection{
			{ID: "gmail_001", DisplayName: "host", Email: "host@example.com", Enabled: true},
		},
		DefaultConnectionID:  "gmail_001",
		HostAccountDismissed: false,
	}
	got := mergeGmailConfigRequest(current, GmailConfigRequest{Enabled: true, DefaultTo: "new@example.com"})
	if !got.Enabled || got.DefaultTo != "new@example.com" {
		t.Fatalf("form fields not applied: %+v", got)
	}
	if len(got.Connections) != 1 || got.Connections[0].ID != "gmail_001" || got.DefaultConnectionID != "gmail_001" {
		t.Fatalf("registry state lost on save: %+v", got)
	}
	if got.ManuallyDisabled {
		t.Fatal("enabling through the form must clear ManuallyDisabled")
	}
	if current.DefaultTo != "old@example.com" {
		t.Fatal("the stored config must not be mutated in place")
	}
}

func TestMergeGmailConfigRequestDisableIsDeliberate(t *testing.T) {
	got := mergeGmailConfigRequest(&services.GmailConfig{Enabled: true}, GmailConfigRequest{Enabled: false})
	if got.Enabled || !got.ManuallyDisabled {
		t.Fatalf("switching off through the form must set ManuallyDisabled: %+v", got)
	}
	if got := mergeGmailConfigRequest(nil, GmailConfigRequest{Enabled: true}); got == nil || !got.Enabled {
		t.Fatal("a nil stored config must still yield the requested settings")
	}
}
