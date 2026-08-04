package guidance

import (
	"strings"
	"testing"
)

// workflow-tools.md's opening paragraph told any reader to call get_api_spec
// to confirm a parameter shape, without saying that tool is registered only
// for a coding CLI reaching tools through the api-bridge. Workshop mode
// explicitly supports both CLI and native tool-calling providers
// (common.IsCLIProvider gates this exact distinction elsewhere in the
// runtime), so a native-provider workshop session reading this doc got told
// to call a tool the runtime never registered for it — mcpagent's
// agent.go:1790 excludes get_api_spec from a session's virtual-tool
// executors whenever the agent is not in code-execution mode, and the
// lookup then fails with "custom tool get_api_spec is not registered for
// session <id>".
//
// A native session never needs it anyway: native tool-calling already
// declares every tool's full schema directly to the model.
func TestWorkflowToolsScopesGetAPISpecToCodeExecutionMode(t *testing.T) {
	rendered, err := renderReferenceKind("workflow-tools", tmplData{})
	if err != nil {
		t.Fatalf("render workflow-tools: %v", err)
	}

	firstMention := strings.Index(rendered, "get_api_spec")
	if firstMention < 0 {
		t.Fatal("workflow-tools no longer mentions get_api_spec at all")
	}
	// Only inspect the sentence around the first mention: that is the
	// standalone top-level instruction a reader hits before reaching the
	// "Coding-CLI bridge routing" section, which already scopes correctly.
	window := rendered[:min(firstMention+400, len(rendered))]

	for _, want := range []string{
		"coding CLI",
		"api-bridge",
		"native tool-calling session",
		"not registered",
	} {
		if !strings.Contains(window, want) {
			t.Errorf("get_api_spec instruction is missing %q — an unscoped instruction reads as available to every session: %s", want, window)
		}
	}
}
