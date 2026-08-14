package server

import (
	"strings"
	"testing"
)

// A product surface has its own workspace vocabulary (Video Studio's prompt
// names uploads/, work/ and outputs/) and does not carry the org-level Pulse
// tools at all, so AgentWorks' plan/pulse placement rules must not be appended
// to the shell tool description it reads on every get_api_spec call.
func TestMultiAgentToolDescriptionOmitsAgentWorksPlacementForProducts(t *testing.T) {
	const folder = "_users/default/Chats/Video Studio/projects/demo"

	product := enhanceToolDescriptionForMultiAgentMode("execute_shell_command", "base.", folder, true)
	for _, leaked := range []string{"pulse/", "goals.html", "org-pulse.html", "{plan_id}"} {
		if strings.Contains(product, leaked) {
			t.Fatalf("product profile description leaked AgentWorks guidance %q:\n%s", leaked, product)
		}
	}
	// The access restriction itself is true everywhere and must survive.
	if !strings.Contains(product, folder) || !strings.Contains(product, "read-only unless explicitly allowed") {
		t.Fatalf("product profile description dropped the write-scope restriction:\n%s", product)
	}

	// AgentWorks keeps the guidance it actually uses.
	agentworks := enhanceToolDescriptionForMultiAgentMode("execute_shell_command", "base.", folder, false)
	for _, want := range []string{"pulse/goals.html", "{plan_id}"} {
		if !strings.Contains(agentworks, want) {
			t.Fatalf("AgentWorks description lost %q:\n%s", want, agentworks)
		}
	}

	// Read-only tools: same split, and the product variant must not advertise a
	// pulse/ write scope it does not have.
	readOnlyProduct := enhanceToolDescriptionForMultiAgentMode("read_workspace_file", "base.", folder, true)
	if strings.Contains(readOnlyProduct, "pulse/") {
		t.Fatalf("product read-only description advertises pulse/ write access:\n%s", readOnlyProduct)
	}
	if readOnlyAgentWorks := enhanceToolDescriptionForMultiAgentMode("read_workspace_file", "base.", folder, false); !strings.Contains(readOnlyAgentWorks, "pulse/") {
		t.Fatalf("AgentWorks read-only description lost pulse/:\n%s", readOnlyAgentWorks)
	}
}
