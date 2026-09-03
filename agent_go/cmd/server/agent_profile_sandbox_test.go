package server

import (
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

func TestAgentProfileReadOnlyFolders(t *testing.T) {
	workflowRO := []string{"Workflow/demo/planning/"}
	got := agentProfileReadOnlyFolders(agentprofiles.SandboxPolicy{}, workflowRO)
	if strings.Join(got, ",") != "skills/,subagents/,Downloads/,Workflow/demo/planning/" {
		t.Fatalf("default read-only set = %v", got)
	}
	got = agentProfileReadOnlyFolders(agentprofiles.SandboxPolicy{ReadOnly: []string{"Downloads", " reports/ ", ""}}, workflowRO)
	if strings.Join(got, ",") != "Downloads/,reports/" {
		t.Fatalf("declared read-only set = %v", got)
	}
	got = agentProfileReadOnlyFolders(agentprofiles.SandboxPolicy{ReadOnly: []string{}}, workflowRO)
	if len(got) != 0 {
		t.Fatalf("an explicit empty list must grant nothing extra: %v", got)
	}
}
