package server

import (
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/agentprofiles"
)

func TestRegisterChiefOfStaffToolFactoriesBuildsBothTools(t *testing.T) {
	registry := agentprofiles.NewRegistry()
	api := &StreamingAPI{}
	if err := api.registerChiefOfStaffToolFactories(registry); err != nil {
		t.Fatalf("registerChiefOfStaffToolFactories failed: %v", err)
	}

	cases := []struct {
		factoryID string
		toolName  string
		category  string
	}{
		{ChiefOfStaffToolFactoryActivityStatus, "get_activity_status", "activity_status"},
		{ChiefOfStaffToolFactoryUpdateNotifications, "update_chief_of_staff_notifications", "notification_tools"},
	}
	for _, tc := range cases {
		spec, err := registry.BuildTool(
			agentprofiles.ToolBinding{ID: tc.factoryID},
			agentprofiles.ToolRuntimeContext{UserID: "user-1", SessionID: "session-1"},
		)
		if err != nil {
			t.Fatalf("BuildTool(%q) failed: %v", tc.factoryID, err)
		}
		if spec.Name != tc.toolName {
			t.Fatalf("BuildTool(%q).Name = %q, want %q -- the callable tool name must be unchanged from the manual registration", tc.factoryID, spec.Name, tc.toolName)
		}
		if spec.Category != tc.category {
			t.Fatalf("BuildTool(%q).Category = %q, want %q", tc.factoryID, spec.Category, tc.category)
		}
		if spec.Execute == nil {
			t.Fatalf("BuildTool(%q).Execute is nil", tc.factoryID)
		}
		if spec.Parameters == nil {
			t.Fatalf("BuildTool(%q).Parameters is nil", tc.factoryID)
		}
	}
}

func TestChiefOfStaffToolFactoriesRegisterOnlyOnce(t *testing.T) {
	registry := agentprofiles.NewRegistry()
	api := &StreamingAPI{}
	if err := api.registerChiefOfStaffToolFactories(registry); err != nil {
		t.Fatalf("first registration failed: %v", err)
	}
	if err := api.registerChiefOfStaffToolFactories(registry); err == nil {
		t.Fatal("expected a second registration on the same registry to fail (duplicate factory id)")
	}
}
