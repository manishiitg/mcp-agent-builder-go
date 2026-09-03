package sparkquillproduct

import (
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

func TestManifestDeclaresParentAndChild(t *testing.T) {
	profiles := BuiltinAgentProfiles()
	if len(profiles) != 2 || profiles[0].ID != ParentProfileID || profiles[1].ID != ChildProfileID {
		t.Fatalf("profiles = %+v", profiles)
	}
	parent, child := profiles[0], profiles[1]
	if parent.Runtime.Conversation.Mode != agentprofiles.ConversationModeSingleton || len(parent.Schedules) != 1 || parent.Schedules[0].ID != "pulse" {
		t.Fatalf("parent shape wrong: %+v", parent.Runtime.Conversation)
	}
	if !child.Runtime.Sandbox.IsStrict() || !child.Runtime.Sandbox.NetworkDisabled() || child.Runtime.Capabilities.Secrets != agentprofiles.CapabilityDisabled {
		t.Fatalf("child must be strict, offline and secret-free: %+v", child.Runtime)
	}
	if child.Runtime.Conversation.Mode != agentprofiles.ConversationModeKeyed || child.Runtime.Workspace.ProjectsRoot != "Chats/SparkQuill/activities" {
		t.Fatalf("child must be one conversation per activity: %+v", child.Runtime)
	}
	if !parent.ToolPolicy.IsAllowlist() || !child.ToolPolicy.IsAllowlist() {
		t.Fatal("both profiles must allowlist their tools")
	}
	for _, p := range profiles {
		if err := agentprofiles.Validate(p); err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
	}
}

func TestProfilesRegisterOnThePlatformRegistry(t *testing.T) {
	registry := agentprofiles.NewRegistry()
	for _, profile := range BuiltinAgentProfiles() {
		profile.Product = ProductName
		if err := registry.RegisterProfile(profile); err != nil {
			t.Fatalf("register %s: %v", profile.ID, err)
		}
	}
	if err := RegisterAgentProfileRuntime(registry, ""); err != nil {
		t.Fatal(err)
	}
	if err := RegisterProductSkills(); err != nil {
		t.Fatal(err)
	}
	if got, err := registry.Resolve(ChildProfileID, 0, "anyone"); err != nil || got.Product != ProductName {
		t.Fatalf("child profile should resolve as a built-in product profile: %+v %v", got, err)
	}
	names := SkillNames()
	if len(names) < 10 || names[0] != "backup" {
		t.Fatalf("skills = %v", names)
	}
	parent, _ := registry.Resolve(ParentProfileID, 0, "anyone")
	for _, skill := range parent.Skills {
		found := false
		for _, n := range names {
			if n == skill {
				found = true
			}
		}
		if !found {
			t.Fatalf("parent declares skill %q that is not embedded", skill)
		}
	}
}

func TestPromptsRenderForNewAndKnownFamilies(t *testing.T) {
	profiles := BuiltinAgentProfiles()
	parent, child := profiles[0], profiles[1]

	// A brand-new family: every nudge fires, nothing is known.
	rendered, err := agentprofiles.RenderPrompt(parent, agentprofiles.PromptContext{ProjectTitle: "SparkQuill", LocalDateTime: "Monday", Product: ParentPromptVariables(FamilyState{})})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"about their child: your child.", "do not yet know the child's name, grade, board", "what to call the parent", "recurring weekly schedule"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("new-family parent prompt missing %q", want)
		}
	}
	if strings.Contains(rendered, "{{") {
		t.Fatal("unrendered template action left in the parent prompt")
	}

	// A known family: nudges gone, facts in.
	known := FamilyState{Child: &Child{Name: "Maya", Grade: "6", Board: "CBSE"}, ParentLabel: "mom", WatchSites: []string{"https://school.example"}, Schedule: []ScheduleEntry{{Day: "Monday", Start: "08:00", End: "14:00", Label: "School"}}}
	rendered, err = agentprofiles.RenderPrompt(parent, agentprofiles.PromptContext{ProjectTitle: "SparkQuill", LocalDateTime: "Monday", Product: ParentPromptVariables(known)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "Maya, Grade 6 (CBSE)") || strings.Contains(rendered, "do not yet know") || !strings.Contains(rendered, "school.example") {
		t.Fatalf("known-family parent prompt wrong:\n%s", rendered[:600])
	}

	childVars := ChildPromptVariables(known, "Chats/SparkQuill/activities/2026-09-03-fractions", "Loves Harry Potter and football.")
	rendered, err = agentprofiles.RenderPrompt(child, agentprofiles.PromptContext{ProjectTitle: "Fractions", LocalDateTime: "Monday", Product: childVars})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"talking directly with Maya (Grade 6)", "your mom set up", "exactly ONE folder, Chats/SparkQuill/activities/2026-09-03-fractions;", "Harry Potter", "in Grade 6"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("child prompt missing %q", want)
		}
	}
	if familyRootFromActivity("_users/u1/Chats/SparkQuill/activities/2026-09-03-fractions") != "_users/u1/Chats/SparkQuill" {
		t.Fatal("family root derivation wrong")
	}
}
