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
	// Video Studio names its surface rather than blocking a list. Anything not
	// named never reaches the agent, so a new AgentWorks tool cannot widen this
	// product by default — the failure mode a deny list has.
	if !manifest.Profile.ToolPolicy.IsAllowlist() {
		t.Fatalf("Video Studio should declare an allow list: %+v", manifest.Profile.ToolPolicy)
	}
	if len(manifest.Profile.ToolPolicy.Disabled) != 0 {
		t.Fatalf("an allow list is the complete surface; a deny list beside it is a second source: %+v", manifest.Profile.ToolPolicy)
	}
	enabled := map[string]bool{}
	for _, name := range manifest.Profile.ToolPolicy.Enabled {
		enabled[name] = true
	}
	// The product controls and the secret CRUD its ui.secrets surface needs.
	// set_workflow_secret is here because it was registered-but-invisible once.
	for _, name := range []string{
		"show_video", "agent_browser",
		"list_secrets", "set_workflow_secret", "set_user_secret",
		"run_full_workflow", "execute_step",
	} {
		if !enabled[name] {
			t.Fatalf("Video Studio needs %q: %+v", name, manifest.Profile.ToolPolicy)
		}
	}
	// AgentWorks-wide administration, the shared media/LLM bridge, sub-agent
	// orchestration, and scheduling are not this product's business.
	for _, name := range []string{
		"set_provider_auth", "install_skill", "add_mcp_server",
		"execute_shell_command", "diff_patch_workspace_file", "generate_video",
		"delegate", "query_agent", "create_workflow_schedule", "notify_user",
	} {
		if enabled[name] {
			t.Fatalf("Video Studio should not carry %q: %+v", name, manifest.Profile.ToolPolicy)
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
