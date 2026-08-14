package videoproduct

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
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
	// The bridge shell is how product HTTP APIs are reached. Reaching them from
	// each CLI's own shell instead (api_transport.native_shell) is implemented
	// but deliberately not enabled — every provider can call the bridge as an
	// MCP tool, and Codex can ONLY do that. See
	// docs/design/product_api_transport_for_coding_agents.md.
	if !enabled["execute_shell_command"] {
		t.Fatalf("Video Studio reaches product APIs through the bridge shell; removing it leaves Codex with no path at all: %+v", manifest.Profile.ToolPolicy)
	}
	if mode := strings.TrimSpace(manifest.Profile.Runtime.APITransport.Mode); mode == "native_shell" {
		t.Fatalf("native_shell is intentionally off for now (see docs/design/product_api_transport_for_coding_agents.md); enabling it needs the bridge-shell entry above reconsidered too")
	}
	// video.show-video must declare what it presents. Without this the
	// factory refuses every call (see TestShowVideoRequiresADeclaredPresentationKind)
	// and the frontend's kind-keyed renderer registry has nothing to dispatch on.
	var showVideoBinding *agentprofiles.ToolBinding
	for i := range manifest.Profile.Tools {
		if manifest.Profile.Tools[i].ID == "video.show-video" {
			showVideoBinding = &manifest.Profile.Tools[i]
		}
	}
	if showVideoBinding == nil {
		t.Fatal("video.show-video is not in manifest.Profile.Tools")
	}
	if showVideoBinding.Presentation == nil || showVideoBinding.Presentation.Kind != "media.video" {
		t.Fatalf("video.show-video must declare presentation.kind=media.video, got %+v", showVideoBinding.Presentation)
	}
	// Under mcp_only the CLI's own Read/Write are denied, so the bridge has to
	// supply the file editor or the agent is left rewriting files with shell
	// heredocs. This is mode-dependent, not a property of the tool — the full
	// rule (and the hybrid direction, where it must be absent) is pinned by
	// TestProductToolGateGovernsTheCodingAgentBridgeCatalog.
	if manifest.Profile.Runtime.AgentTools.Mode != "hybrid" && !enabled["diff_patch_workspace_file"] {
		t.Fatalf("agent_tools.mode=%q denies native edits; the bridge must carry diff_patch_workspace_file: %+v",
			manifest.Profile.Runtime.AgentTools.Mode, manifest.Profile.ToolPolicy)
	}
	// AgentWorks-wide administration, the shared media/LLM bridge, sub-agent
	// orchestration, and scheduling are not this product's business.
	for _, name := range []string{
		"set_provider_auth", "install_skill", "add_mcp_server",
		"generate_video",
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
	if manifest.Profile.Runtime.AgentTools.Mode != "mcp_only" || manifest.Profile.Runtime.Approvals.Mode != "provider_auto" {
		t.Fatalf("Video Studio native-tool policy = %+v %+v, want mcp_only/provider_auto", manifest.Profile.Runtime.AgentTools, manifest.Profile.Runtime.Approvals)
	}

	if manifest.Workflows.BrowserMode != "auto" || len(manifest.Workflows.SelectedSkills) == 0 {
		t.Fatalf("unexpected workflow definition: %+v", manifest.Workflows)
	}

	// Every tool that can write declares where this product's artifacts belong.
	// Without a declaration the tool description says nothing about layout —
	// and the shared AgentWorks default it used to inherit (plan folders,
	// pulse/goals.html) names concepts Video Studio does not have and
	// contradicts the uploads//work//outputs/ rules in its own system prompt.
	for _, writer := range []string{"execute_shell_command", "diff_patch_workspace_file"} {
		if !enabled[writer] {
			continue
		}
		if len(manifest.Profile.Runtime.Workspace.Placement[writer]) == 0 {
			t.Fatalf("%s is exposed but declares no runtime.workspace.placement: %+v", writer, manifest.Profile.Runtime.Workspace)
		}
	}
}

// Plan-authoring guidance (frontend/src/commands/builtin-commands.tsx): use
// message_sequence "for every conversational, judgment-heavy, browser-driven, or
// adaptive step, even when it needs only one message. Non-scripted regular steps
// are unsupported." Every production stage here is exactly that — writing a
// brief, a storyboard, a design, a critique — and each declares
// declared_execution_mode: agentic, so none qualifies for `regular`.
//
// They previously authored as `regular` and ran only because the runtime
// rewrites non-scripted regular steps into this same shape
// (normalizeRegularStepToMessageSequence), which left the stored plan
// describing an execution model that no longer exists and forced the plan UI to
// reconstruct the real one.
func TestGeneratedPlanAuthorsStagesAsMessageSequences(t *testing.T) {
	raw, err := json.Marshal(planForAll(pipelineRegistry))
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Steps []struct {
			ID    string `json:"type_id"`
			Type  string `json:"type"`
			Items []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"items"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	stages := 0
	for _, step := range plan.Steps {
		if step.Type == "routing" {
			continue // a router branches; it runs no stage turn
		}
		stages++
		if step.Type != "message_sequence" {
			t.Fatalf("stage step has type %q; non-scripted stages must author as message_sequence", step.Type)
		}
		if len(step.Items) == 0 {
			t.Fatal("message_sequence stage declares no items; the runtime would have to synthesize the turn again")
		}
		if step.Items[0].Type != "user_message" {
			t.Fatalf("stage's first item is %q, want user_message", step.Items[0].Type)
		}
	}
	if stages == 0 {
		t.Fatal("no stage steps generated")
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
