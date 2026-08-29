package instructions

import (
	"strings"
	"testing"
)

func TestSpecialWorkspaceToolsInstructionsDirectScriptedAgentsToBridgeGuidance(t *testing.T) {
	for name, prompt := range map[string]string{
		"cheat sheet": GetSpecialWorkspaceToolsInstructions(),
		"CLI pointer": GetSpecialWorkspaceToolsPointer(),
	} {
		for _, want := range []string{"references/workspace-media-tools.md", "references/mcp-bridge.md", "never invoke a provider directly", "credentials in a script"} {
			if !strings.Contains(prompt, want) {
				t.Fatalf("%s missing scripted active-tool guidance %q:\n%s", name, want, prompt)
			}
		}
	}
}
