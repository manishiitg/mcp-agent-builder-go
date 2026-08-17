package chiefofstaffproduct

import (
	"reflect"
	"sort"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/skills"
)

func TestChiefOfStaffManifestDeclaresGlobalScopeAndPinnedModel(t *testing.T) {
	manifest, err := ChiefOfStaffManifest()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Profile.ID != "chief-of-staff" {
		t.Fatalf("unexpected profile id: %q", manifest.Profile.ID)
	}
	if manifest.Profile.Scope != agentprofiles.ProfileScopeGlobal {
		t.Fatalf("chief-of-staff must declare scope: global, got %q", manifest.Profile.Scope)
	}
	if manifest.Profile.Runtime.Provider != "claude-code" || manifest.Profile.Runtime.ModelID != "claude-sonnet-5" {
		t.Fatalf("expected a single pinned model (no tier routing), got provider=%q model_id=%q", manifest.Profile.Runtime.Provider, manifest.Profile.Runtime.ModelID)
	}
	// Deliberately no allowlist: Chief of Staff's tool surface is the
	// platform's broadest and still evolving, unlike Video Studio's named
	// surface. An empty tool_policy is the existing legacy (deny-list-only)
	// behavior, matching what a profile-less chat already gets.
	if manifest.Profile.ToolPolicy.IsAllowlist() {
		t.Fatalf("chief-of-staff should not declare a tool_policy allowlist: %+v", manifest.Profile.ToolPolicy)
	}
	wantTools := map[string]bool{
		"chief-of-staff.activity-status":      false,
		"chief-of-staff.update-notifications": false,
	}
	for _, binding := range manifest.Profile.Tools {
		if _, expected := wantTools[binding.ID]; !expected {
			t.Fatalf("unexpected tool binding %q", binding.ID)
		}
		wantTools[binding.ID] = true
	}
	for id, found := range wantTools {
		if !found {
			t.Fatalf("expected profile.tools[] to bind %q -- without this, registerAgentProfileTools registers nothing for a chief-of-staff chat even though the ToolFactory exists", id)
		}
	}
	if len(manifest.Workflows.Enabled) != 0 {
		t.Fatalf("Chief of Staff dispatches to workflows rather than shipping fixed pipelines; workflows.enabled should stay empty: %v", manifest.Workflows.Enabled)
	}
	if manifest.UI.Surface != "chief-of-staff" {
		t.Fatalf("unexpected ui.surface: %q", manifest.UI.Surface)
	}
	gotSkills := append([]string(nil), manifest.Profile.Skills...)
	sort.Strings(gotSkills)
	wantSkills := append([]string(nil), chiefOfStaffSkillNames...)
	sort.Strings(wantSkills)
	if !reflect.DeepEqual(gotSkills, wantSkills) {
		t.Fatalf("manifest skills %v do not match the registered set %v -- product.yaml and profile_definition.go's chiefOfStaffSkillNames must list the same names", gotSkills, wantSkills)
	}
	for _, excluded := range []string{"org-goals", "org-html", "org-pulse", "chief-task-report"} {
		for _, name := range manifest.Profile.Skills {
			if name == excluded {
				t.Fatalf("goal-related skill %q must not be declared -- goals feature is dropped", excluded)
			}
		}
	}
}

func TestRegisterProductSkillsRegistersEveryDeclaredSkill(t *testing.T) {
	if err := RegisterProductSkills(); err != nil {
		t.Fatalf("RegisterProductSkills failed: %v", err)
	}
	for _, name := range chiefOfStaffSkillNames {
		if !skills.IsBuiltinSkill(name) {
			t.Fatalf("expected %q to be registered as a builtin skill", name)
		}
	}
}

func TestBuiltinAgentProfileValidatesAndResolvesGlobalScope(t *testing.T) {
	profile := BuiltinAgentProfile()
	if err := agentprofiles.Validate(profile); err != nil {
		t.Fatalf("built-in profile failed validation: %v", err)
	}
	if profile.EffectiveScope() != agentprofiles.ProfileScopeGlobal {
		t.Fatalf("EffectiveScope() = %q, want global", profile.EffectiveScope())
	}
	if profile.SystemPromptTemplate == "" {
		t.Fatal("expected a non-empty rendered system prompt template")
	}
	if !profile.BuiltIn {
		t.Fatal("expected BuiltIn to be true")
	}
}

func TestBuiltinAgentProfilesReturnsExactlyOneVersion(t *testing.T) {
	// Unlike Video Studio, which keeps a legacy version-1 profile registered
	// alongside its current one, chief-of-staff has no prior shipped version
	// to stay resolvable for.
	profiles := BuiltinAgentProfiles()
	if len(profiles) != 1 {
		t.Fatalf("expected exactly one built-in profile, got %d", len(profiles))
	}
	if profiles[0].Version != 1 {
		t.Fatalf("expected version 1, got %d", profiles[0].Version)
	}
}

func TestRenderPromptSucceedsAgainstAPromptContext(t *testing.T) {
	// resolveAgentProfileForQuery calls agentprofiles.RenderPrompt for every
	// resolved profile regardless of scope, even though the rendered result
	// is never actually used for a global-scoped profile at the server.go
	// prompt-assembly step. If this fails, the whole query fails -- so the
	// placeholder prompt must render cleanly against a real PromptContext.
	profile := BuiltinAgentProfile()
	rendered, err := agentprofiles.RenderPrompt(profile, agentprofiles.PromptContext{
		ProjectTitle:  "Chief of Staff",
		LocalDateTime: "Monday, 1 January 2026 at 9:00 AM UTC",
	})
	if err != nil {
		t.Fatalf("RenderPrompt failed: %v", err)
	}
	if rendered == "" {
		t.Fatal("expected non-empty rendered prompt")
	}
}
