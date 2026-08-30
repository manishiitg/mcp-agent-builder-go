package server

import (
	"os"
	"strings"
	"testing"
)

// Ordinary and product chat are direct sessions. Workflow-specific background
// execution has its own tools and must not leak back into this handler.
func TestChatHandlerDoesNotRegisterLegacyDelegationOrScheduleTools(t *testing.T) {
	source, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, forbidden := range []string{
		"virtualtools.CreateDelegationTools(",
		"virtualtools.CreateDelegationToolExecutors(",
		"createWorkflowScheduleTools()",
		"createWorkflowScheduleExecutors(",
		"GetMultiAgentDelegationInstructionsWithUser(",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("chat handler still contains legacy orchestration path %q", forbidden)
		}
	}
	if !strings.Contains(text, "GetAgentWorksChatInstructionsWithUser(") {
		t.Fatal("chat handler does not install the direct-chat prompt")
	}
}
