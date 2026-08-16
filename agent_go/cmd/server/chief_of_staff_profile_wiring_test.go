package server

import (
	"context"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/internal/chiefofstaffproduct"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

// TestChiefOfStaffProfileResolvesAndBuildsItsTools is the end-to-end
// wiring check: register the real chief-of-staff profile AND its tool
// factories on one registry (mirroring exactly what server.go's startup
// does), resolve a query against it, and confirm every tool the profile
// declares actually builds. This is what would have caught profile.tools[]
// being left empty -- each half (profile parses; factories build in
// isolation) passed on its own without it.
func TestChiefOfStaffProfileResolvesAndBuildsItsTools(t *testing.T) {
	registry := agentprofiles.NewRegistry()
	for _, profile := range chiefofstaffproduct.BuiltinAgentProfiles() {
		if err := registry.RegisterProfile(profile); err != nil {
			t.Fatalf("RegisterProfile failed: %v", err)
		}
	}
	api := &StreamingAPI{agentProfiles: registry}
	if err := api.registerChiefOfStaffToolFactories(registry); err != nil {
		t.Fatalf("registerChiefOfStaffToolFactories failed: %v", err)
	}

	req := QueryRequest{AgentMode: "multi-agent", AgentProfileID: "chief-of-staff"}
	resolved, err := api.resolveAgentProfileForQuery(context.Background(), &req, "user-1", "session-1")
	if err != nil {
		t.Fatalf("resolveAgentProfileForQuery failed: %v", err)
	}
	if resolved == nil {
		t.Fatal("expected a resolved profile")
	}
	if !isGlobalScopedProfile(resolved) {
		t.Fatal("expected the resolved chief-of-staff profile to be global-scoped")
	}
	if len(resolved.Definition.Tools) != 2 {
		t.Fatalf("expected 2 declared tool bindings, got %d: %+v", len(resolved.Definition.Tools), resolved.Definition.Tools)
	}

	wantToolNames := map[string]bool{
		"get_activity_status":                 false,
		"update_chief_of_staff_notifications": false,
	}
	for _, binding := range resolved.Definition.Tools {
		spec, buildErr := registry.BuildTool(binding, agentprofiles.ToolRuntimeContext{UserID: "user-1", SessionID: "session-1"})
		if buildErr != nil {
			t.Fatalf("BuildTool(%q) failed: %v", binding.ID, buildErr)
		}
		if _, expected := wantToolNames[spec.Name]; !expected {
			t.Fatalf("unexpected tool name %q from binding %q", spec.Name, binding.ID)
		}
		wantToolNames[spec.Name] = true
	}
	for name, found := range wantToolNames {
		if !found {
			t.Fatalf("tool %q never built from the profile's declared bindings", name)
		}
	}
}
