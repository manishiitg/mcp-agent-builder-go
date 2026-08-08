package videoproduct

import (
	"strings"
	"testing"
)

func TestVideoStudioManifestDrivesProfileAndWorkflowCapabilities(t *testing.T) {
	manifest, err := VideoStudioManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Profile.ID != "video-studio" || manifest.Profile.Runtime.Capabilities.Browser != "required" || manifest.Profile.Runtime.Capabilities.Secrets != "required" || manifest.Profile.Runtime.Capabilities.LiveInput != "disabled" {
		t.Fatalf("unexpected declarative profile: %+v", manifest.Profile)
	}
	disabled := map[string]bool{}
	for _, name := range manifest.Profile.ToolPolicy.Disabled {
		disabled[name] = true
	}
	for _, name := range []string{
		"set_provider_auth", "install_skill", "add_mcp_server",
		"execute_shell_command", "diff_patch_workspace_file", "generate_video",
	} {
		if !disabled[name] {
			t.Fatalf("expected Video Studio to disable %q: %+v", name, manifest.Profile.ToolPolicy)
		}
	}
	if manifest.UI.Surface != "video-studio" || !manifest.UI.Secrets || manifest.Branding.Favicon != "/video-studio-favicon.svg" {
		t.Fatalf("unexpected UI/branding definition: %+v %+v", manifest.UI, manifest.Branding)
	}
	if manifest.Profile.Runtime.Transport != "structured" {
		t.Fatalf("Video Studio runtime transport = %q, want structured", manifest.Profile.Runtime.Transport)
	}
	if manifest.Profile.Runtime.AgentTools.Mode != "hybrid" || manifest.Profile.Runtime.Approvals.Mode != "provider_auto" {
		t.Fatalf("Video Studio native-tool policy = %+v %+v, want hybrid/provider_auto", manifest.Profile.Runtime.AgentTools, manifest.Profile.Runtime.Approvals)
	}
	if manifest.Workflows.BrowserMode != "auto" || len(manifest.Workflows.SelectedSkills) == 0 {
		t.Fatalf("unexpected workflow definition: %+v", manifest.Workflows)
	}
}

func TestVideoStudioPromptExpandsAllowListedVariables(t *testing.T) {
	prompt := renderProductPrompt("AgentWorks launch", "Friday at 2:30 PM IST")
	for _, required := range []string{"You are Video Studio", "Friday at 2:30 PM IST"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("rendered prompt missing %q: %s", required, prompt)
		}
	}
	if strings.Contains(prompt, "AgentWorks launch") || strings.Contains(prompt, "{{TIME}}") || strings.Contains(prompt, "{{PRODUCT_NAME}}") {
		t.Fatalf("prompt variables were not expanded: %s", prompt)
	}
}
