package videoproduct

import (
	"encoding/json"
	"reflect"
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
	// Under mcp_only the CLI's own Read/Write are denied, so the bridge supplies
	// the guarded editor. Mode-dependent, not a property of the tool — the
	// hybrid direction is pinned by
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
			ID    string `json:"id"`
			Type  string `json:"type"`
			Items []struct {
				Type string `json:"type"`
			} `json:"items"`
			Routes []struct {
				RouteID string `json:"route_id"`
				Sub     struct {
					ID    string `json:"id"`
					Type  string `json:"type"`
					Items []struct {
						Type string `json:"type"`
					} `json:"items"`
				} `json:"sub_agent_step"`
			} `json:"predefined_routes"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}

	assertStage := func(id, stepType string, items int, firstItem string) {
		t.Helper()
		if stepType != "message_sequence" {
			t.Fatalf("stage %q has type %q; non-scripted stages must author as message_sequence", id, stepType)
		}
		if items == 0 {
			t.Fatalf("stage %q declares no items; the runtime would have to synthesize the turn again", id)
		}
		if firstItem != "user_message" {
			t.Fatalf("stage %q first item is %q, want user_message", id, firstItem)
		}
	}

	stages := 0
	for _, step := range plan.Steps {
		switch step.Type {
		case "routing":
			continue // a router branches; it runs no stage turn
		case "todo_task":
			// An orchestrator runs no turn of its own — its work is its routes,
			// and each of those is still a stage and still a message_sequence.
			if len(step.Routes) == 0 {
				t.Fatalf("todo_task %q declares no predefined_routes", step.ID)
			}
			for _, route := range step.Routes {
				stages++
				first := ""
				if len(route.Sub.Items) > 0 {
					first = route.Sub.Items[0].Type
				}
				assertStage(route.Sub.ID, route.Sub.Type, len(route.Sub.Items), first)
				if route.RouteID != route.Sub.ID {
					t.Fatalf("route_id %q does not match its sub_agent_step id %q; the orchestrator re-dispatches by route_id", route.RouteID, route.Sub.ID)
				}
			}
			continue
		}
		stages++
		first := ""
		if len(step.Items) > 0 {
			first = step.Items[0].Type
		}
		assertStage(step.ID, step.Type, len(step.Items), first)
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

// The pre-production block is orchestrated because its critic is a pure
// reviewer: it faults BRIEF/STORYBOARD/SCRIPT/frame.md, owns none of them, and
// is told not to build. Linearly its `revise` verdict had no addressee — the
// author's step had finished, the next step did not own the artifact, and the
// platform removed step loops. This pins the wiring that gives a finding
// somewhere to go.
func TestPreProductionIsOrchestratedSoCritiqueFindingsHaveAnAddressee(t *testing.T) {
	block := infographicPipeline.Orchestrated
	if block == nil {
		t.Fatal("infographic pipeline declares no orchestrated block; a creative-critique verdict would again have nowhere to go")
	}

	// The critic must be inside the block. Outside it, the orchestrator never
	// sees the verdict and cannot re-dispatch anyone.
	var hasCritic bool
	for _, id := range block.StageIDs {
		if id == "infographic-creative-critique" {
			hasCritic = true
		}
	}
	if !hasCritic {
		t.Fatalf("creative critique is not one of the orchestrated routes: %v", block.StageIDs)
	}

	// It must also be able to reach the authors of what it faults, or a
	// `revise` verdict still has no addressee.
	for _, owner := range []string{"infographic-concept", "infographic-copy", "infographic-layout"} {
		var found bool
		for _, id := range block.StageIDs {
			if id == owner {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s authors an artifact the critique faults but is not a route the orchestrator can re-dispatch: %v", owner, block.StageIDs)
		}
	}

	// The instruction has to say what each verdict does. Without this the
	// orchestrator sees a scorecard and no mandate to act on it.
	for _, required := range []string{"revise", "blocked", "pass"} {
		if !strings.Contains(block.Description, required) {
			t.Fatalf("orchestrator description never handles verdict %q", required)
		}
	}
	// Bounded, or a failing critique loops forever.
	if !strings.Contains(block.Description, "three review rounds") {
		t.Fatalf("orchestrator description sets no bound on re-review rounds:\n%s", block.Description)
	}

	// The later critiques stay linear on purpose: they own the artifact they
	// judge and repair it in place, which needs no orchestrator.
	for _, id := range block.StageIDs {
		if id == "infographic-composition-critique" || id == "infographic-render-critique" {
			t.Fatalf("%s repairs the artifact it judges; orchestrating it adds nondeterminism for nothing", id)
		}
	}
}

// The creative critique writes "the exact acceptance criteria the builder must
// prove". The builder is the first stage after the orchestrated gate, so if its
// instructions never name the scorecard, those criteria are written and then
// read by nobody until the composition critique — one stage too late to steer
// the build they were written for.
func TestBuilderIsPointedAtTheCriteriaTheCritiqueWrote(t *testing.T) {
	var critique, design *PipelineStage
	for i := range infographicPipeline.Stages {
		switch infographicPipeline.Stages[i].ID {
		case "infographic-creative-critique":
			critique = &infographicPipeline.Stages[i]
		case "infographic-design":
			design = &infographicPipeline.Stages[i]
		}
	}
	if critique == nil || design == nil {
		t.Fatal("creative critique or design stage is missing from the infographic pipeline")
	}
	for _, artifact := range append([]string{critique.Output}, critique.Artifacts...) {
		if !strings.Contains(design.Description, artifact) {
			t.Fatalf("the build stage never mentions %q, so the critique's acceptance criteria reach no builder", artifact)
		}
	}
}

// Skills are attached per step id via step_config.steps[].agent_configs.
// enabled_skills, and the runtime resolves them by matching that id — including
// recursively into predefined_routes[].sub_agent_step (populateRuntimeFields).
// Moving a stage from a top-level step to a route therefore keeps its skills
// ONLY because its step_config entry is still emitted under the same id. This
// pins that: a stage that loses its entry would run with no skills at all,
// which fails as a vague result rather than an error.
func TestOrchestratedRoutesKeepTheirSkillEntries(t *testing.T) {
	raw, err := json.Marshal(stepConfigForAll(pipelineRegistry))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Steps []struct {
			ID     string `json:"id"`
			Agents struct {
				Skills []string `json:"enabled_skills"`
			} `json:"agent_configs"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	skillsByID := map[string][]string{}
	for _, step := range config.Steps {
		skillsByID[step.ID] = step.Agents.Skills
	}

	for _, p := range pipelineRegistry {
		if p.Orchestrated == nil {
			continue
		}
		if _, ok := skillsByID[p.Orchestrated.ID]; !ok {
			t.Fatalf("orchestrator %q has no step_config entry; it would fall back to platform defaults instead of this product's LLM", p.Orchestrated.ID)
		}
		for _, id := range p.Orchestrated.StageIDs {
			var stage *PipelineStage
			for i := range p.Stages {
				if p.Stages[i].ID == id {
					stage = &p.Stages[i]
				}
			}
			if stage == nil {
				t.Fatalf("orchestrated stage id %q matches no stage in pipeline %q", id, p.ID)
			}
			got, ok := skillsByID[id]
			if !ok {
				t.Fatalf("orchestrated route %q has no step_config entry; as a route it would run with no skills", id)
			}
			if !reflect.DeepEqual(got, stage.Skills) {
				t.Fatalf("route %q skills = %v, want %v", id, got, stage.Skills)
			}
			if len(stage.Skills) > 0 && len(got) == 0 {
				t.Fatalf("route %q declares skills but its config carries none", id)
			}
		}
	}
}

// The plan graph draws no sequential edge out of a todo_task — usePlanToFlow
// sets lastExitNodeId = null for it and follows next_step_id only. The runtime
// would fall through in array order regardless, so omitting it ran correctly
// while rendering the orchestrator as a dead end with no path to the build.
func TestOrchestratorNamesItsSuccessor(t *testing.T) {
	raw, err := json.Marshal(planForAll(pipelineRegistry))
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Steps []struct {
			ID   string `json:"id"`
			Type string `json:"type"`
			Next string `json:"next_step_id"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{"end": true}
	for _, s := range plan.Steps {
		ids[s.ID] = true
	}
	var seen int
	for _, s := range plan.Steps {
		if s.Type != "todo_task" {
			continue
		}
		seen++
		if s.Next == "" {
			t.Fatalf("todo_task %q names no next_step_id; the plan graph renders it as a dead end", s.ID)
		}
		if !ids[s.Next] {
			t.Fatalf("todo_task %q points at %q, which is not a step in the plan", s.ID, s.Next)
		}
	}
	if seen == 0 {
		t.Fatal("no todo_task step in the generated plan")
	}
}
