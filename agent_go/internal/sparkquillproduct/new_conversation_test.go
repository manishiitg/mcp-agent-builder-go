package sparkquillproduct

import (
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

// The parent can start a fresh chat with Quill; the child's conversation is
// bound to an activity and has its own "start fresh" flow, so it must not
// offer the generic control.
func TestOnlyTheParentProfileOffersANewConversation(t *testing.T) {
	got := map[string]agentprofiles.CapabilityRequirement{}
	for _, p := range BuiltinAgentProfiles() {
		got[p.ID] = p.Runtime.Capabilities.NewConversation
	}
	if got["sparkquill"] == "" || got["sparkquill"] == agentprofiles.CapabilityDisabled {
		t.Fatalf("parent profile new_conversation = %q, want enabled", got["sparkquill"])
	}
	if got["sparkquill-child"] != "" && got["sparkquill-child"] != agentprofiles.CapabilityDisabled {
		t.Fatalf("child profile new_conversation = %q, want absent or disabled", got["sparkquill-child"])
	}
}
