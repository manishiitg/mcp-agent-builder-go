package step_based_workflow

import (
	"strings"
	"testing"
)

// The scripted fast path builds its own HTTP request rather than going through
// workspace.Client, so it must attach the workspace token itself. Without it,
// every scripted run failed before the interpreter started.
func TestScriptedFastPathAttachesWorkspaceToken(t *testing.T) {
	src := readSourceFile(t, "controller_scripted.go")
	idx := strings.Index(src, `apiURL := getWorkspaceAPIURL() + "/api/execute"`)
	if idx < 0 {
		t.Fatal("could not locate the scripted execute request")
	}
	// Look at the request construction that follows, up to the Do() call.
	end := strings.Index(src[idx:], "http.DefaultClient.Do(req)")
	if end < 0 {
		t.Fatal("could not locate the request dispatch")
	}
	block := src[idx : idx+end]
	if !strings.Contains(block, "X-Workspace-Token") {
		t.Fatal("scripted /api/execute request must set X-Workspace-Token; /api/execute is token-protected and this path bypasses workspace.Client.doRequest")
	}
	if !strings.Contains(block, "WORKSPACE_API_TOKEN") {
		t.Fatal("the token must come from WORKSPACE_API_TOKEN, matching workspace.Client")
	}
}

// A rejected request means the script never ran. The error text is handed
// verbatim to the step agent as "Previous Script (Failed)", and without this
// framing the agent rewrites a script that was never executed.
func TestHarnessRejectionTellsAgentTheScriptNeverRan(t *testing.T) {
	src := readSourceFile(t, "controller_scripted.go")
	idx := strings.Index(src, `execErr = fmt.Errorf(`)
	if idx < 0 {
		t.Fatal("could not locate the api-error construction")
	}
	block := src[idx:min(idx+1200, len(src))]
	for _, want := range []string{
		"harness failure, not a script failure",
		"never ran",
		"Do NOT rewrite the script",
		"CONCERNS:",
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("harness-rejection error must tell the agent %q:\n%s", want, block)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
