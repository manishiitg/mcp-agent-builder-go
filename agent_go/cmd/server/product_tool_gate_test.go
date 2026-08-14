package server

import (
	"reflect"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/videoproduct"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

func profileWithPolicy(id string, policy agentprofiles.ToolPolicy) *resolvedAgentProfile {
	return &resolvedAgentProfile{Definition: agentprofiles.Profile{ID: id, ToolPolicy: policy}}
}

// A profile without mode=allowlist must not filter. Products still on the
// legacy deny-list keep their current surface when the gate is installed.
func TestProductToolGateObserveModeAdmitsEverything(t *testing.T) {
	gate := newProductToolGate(profileWithPolicy("video-studio", agentprofiles.ToolPolicy{
		Disabled: []string{"execute_shell_command"},
	}))

	if gate.enforcing() {
		t.Fatal("a profile without mode=allowlist must be in observe mode")
	}
	for _, name := range []string{"execute_shell_command", "set_user_secret", "anything_at_all"} {
		if !gate.Admit(name) {
			t.Fatalf("observe mode declined %q", name)
		}
	}

	registered, filtered := gate.summary()
	want := []string{"anything_at_all", "execute_shell_command", "set_user_secret"}
	if !reflect.DeepEqual(registered, want) {
		t.Fatalf("registered = %v, want %v", registered, want)
	}
	if filtered != nil {
		t.Fatalf("observe mode filtered %v", filtered)
	}
}

// A nil profile is ordinary (non-product) chat: never filtered, never logged.
func TestProductToolGateNilProfileAdmitsEverything(t *testing.T) {
	gate := newProductToolGate(nil)
	if gate.enforcing() {
		t.Fatal("a nil profile must not enforce")
	}
	if !gate.Admit("delegate") {
		t.Fatal("a nil profile declined a tool")
	}
	if gate.profileID != "" {
		t.Fatalf("profileID = %q, want empty so logSurface stays quiet", gate.profileID)
	}
}

func TestProductToolGateAllowlistFiltersUnlistedTools(t *testing.T) {
	gate := newProductToolGate(profileWithPolicy("video-studio", agentprofiles.ToolPolicy{
		Mode:    agentprofiles.ToolPolicyModeAllowlist,
		Enabled: []string{"video.show-video", "set_workflow_secret", "run_full_workflow"},
	}))

	if !gate.enforcing() {
		t.Fatal("mode=allowlist must enforce")
	}
	if !gate.Admit("set_workflow_secret") {
		t.Fatal("an enabled tool was declined")
	}
	if gate.Admit("image_gen") {
		t.Fatal("an unlisted tool was admitted")
	}

	registered, filtered := gate.summary()
	if !reflect.DeepEqual(registered, []string{"set_workflow_secret"}) {
		t.Fatalf("registered = %v", registered)
	}
	// The filtered set is the diagnostic that makes a fail-closed allowlist
	// debuggable: a missing capability shows up here, not as agent confusion.
	if !reflect.DeepEqual(filtered, []string{"image_gen"}) {
		t.Fatalf("filtered = %v", filtered)
	}
}

// The gate is the one decision point, so it must not care which pool a tool
// arrived from. Secret, workflow, and platform tools are all just names.
func TestProductToolGateAppliesAcrossPools(t *testing.T) {
	gate := newProductToolGate(profileWithPolicy("video-studio", agentprofiles.ToolPolicy{
		Mode:    agentprofiles.ToolPolicyModeAllowlist,
		Enabled: []string{"list_secrets", "query_step"},
	}))

	cases := map[string]bool{
		"list_secrets":          true,  // secret tools
		"query_step":            true,  // workflow tools
		"set_user_secret":       false, // secret pool, not enabled
		"execute_step":          false, // workflow pool, not enabled
		"list_llm_capabilities": false, // platform pool, not enabled
	}
	for name, want := range cases {
		if got := gate.Admit(name); got != want {
			t.Errorf("Admit(%q) = %v, want %v", name, got, want)
		}
	}
}

// A delegated sub-agent builds its own gate from the same resolved profile
// rather than inheriting the parent's instance. That is only safe if one
// profile always yields one surface — otherwise delegating becomes a way around
// tool_policy, which is the parent/child divergence this design exists to stop.
func TestProductToolGateIsDerivedIdenticallyForParentAndChild(t *testing.T) {
	policy := agentprofiles.ToolPolicy{
		Mode:    agentprofiles.ToolPolicyModeAllowlist,
		Enabled: []string{"video.show-video", "query_step"},
	}
	parent := newProductToolGate(profileWithPolicy("video-studio", policy))
	child := newProductToolGate(profileWithPolicy("video-studio", policy))

	if parent.enforcing() != child.enforcing() {
		t.Fatalf("parent enforcing=%v but child enforcing=%v", parent.enforcing(), child.enforcing())
	}
	for _, name := range []string{"video.show-video", "query_step", "image_gen", "set_user_secret"} {
		if parent.Admit(name) != child.Admit(name) {
			t.Errorf("parent and child disagree on %q; a child could exceed its product's surface", name)
		}
	}
}

func TestProductToolGateTrimsAndDeduplicates(t *testing.T) {
	gate := newProductToolGate(profileWithPolicy("video-studio", agentprofiles.ToolPolicy{
		Mode:    agentprofiles.ToolPolicyModeAllowlist,
		Enabled: []string{"  query_step  ", ""},
	}))

	if !gate.Admit(" query_step ") {
		t.Fatal("whitespace around a registered name must not change the decision")
	}
	if !gate.Admit("query_step") {
		t.Fatal("second registration of the same name must still be admitted")
	}

	registered, _ := gate.summary()
	if !reflect.DeepEqual(registered, []string{"query_step"}) {
		t.Fatalf("registered = %v, want one de-duplicated entry", registered)
	}
}

// The gate decides what a session may execute AND what its coding-agent bridge
// advertises. Those were separate: the bridge's hardcoded core list
// (execute_shell_command, diff_patch_workspace_file, agent_browser) synthesized
// a definition for a tool the profile had removed, so the CLI was offered a
// shell whose own description says to use it for HTTP calls, called it, and was
// refused with "not registered for session" — Claude Code silently retried on
// its native Bash, Codex reported the product broken.
//
// agent_browser is the control: hybrid profiles genuinely want it, because no
// coding CLI has a native equivalent for driving the user's signed-in browser.
func TestProductToolGateGovernsTheCodingAgentBridgeCatalog(t *testing.T) {
	manifest, err := videoproduct.VideoStudioManifest()
	if err != nil {
		t.Fatalf("load Video Studio manifest: %v", err)
	}
	gate := newProductToolGate(&resolvedAgentProfile{Definition: manifest.Profile})
	if !gate.enforcing() {
		t.Fatal("Video Studio declares tool_policy.mode=allowlist; the gate must enforce")
	}

	// The bridge carries what the CLI cannot do natively, so a file editor's
	// place depends on the mode: under hybrid every supported CLI ships one and
	// diff_patch_workspace_file is redundant; under mcp_only they are denied and
	// it is the only guarded way to edit a file. Asserting one direction
	// unconditionally made the exclusion look like a property of the tool.
	//
	// execute_shell_command stays IN: it is how product HTTP APIs are
	// reached, and Codex can reach it only as an MCP tool (its JS code-mode
	// sandbox has no network and no env). See
	// docs/design/product_api_transport_for_coding_agents.md.
	nativeToolsDenied := manifest.Profile.Runtime.AgentTools.Mode != "hybrid"
	if got := gate.Admit("diff_patch_workspace_file"); got != nativeToolsDenied {
		if nativeToolsDenied {
			t.Fatalf("agent_tools.mode=%q denies the CLI's own editor, so the bridge must supply diff_patch_workspace_file", manifest.Profile.Runtime.AgentTools.Mode)
		}
		t.Fatal("diff_patch_workspace_file must stay out of a hybrid profile's surface: the CLI supplies its own")
	}
	for _, kept := range []string{"execute_shell_command", "agent_browser"} {
		if !gate.Admit(kept) {
			t.Fatalf("%q is allowlisted and has no usable native equivalent for every provider; it must survive", kept)
		}
	}
}
