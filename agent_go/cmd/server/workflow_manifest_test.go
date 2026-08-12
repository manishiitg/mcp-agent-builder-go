package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	step_based_workflow "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
)

func TestWorkflowManifestChangelogChangesIsStableAndValueFree(t *testing.T) {
	previous := `{
  "label": "Old label",
  "capabilities": {
    "selected_skills": ["browser"],
    "slack_webhook_secret_name": "old-secret-value"
  },
  "unchanged": true
}`
	current := `{
  "label": "New label",
  "capabilities": {
    "selected_skills": ["browser", "ffmpeg"],
    "slack_webhook_secret_name": "new-secret-value"
  },
  "new_setting": "enabled",
  "unchanged": true
}`

	got := workflowManifestChangelogChanges(previous, current)
	want := []step_based_workflow.PlanFieldChange{
		{Field: "workflow.json.capabilities.selected_skills", OldValue: "present", NewValue: "changed"},
		{Field: "workflow.json.capabilities.slack_webhook_secret_name", OldValue: "present", NewValue: "changed"},
		{Field: "workflow.json.label", OldValue: "present", NewValue: "changed"},
		{Field: "workflow.json.new_setting", OldValue: "absent", NewValue: "added"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %#v, want %#v", got, want)
	}

	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal changes: %v", err)
	}
	for _, value := range []string{"old-secret-value", "new-secret-value", "New label"} {
		if strings.Contains(string(encoded), value) {
			t.Fatalf("changelog evidence leaked manifest value %q: %s", value, encoded)
		}
	}
}

func TestWorkflowManifestChangelogChangesCreationAndCorruptPriorState(t *testing.T) {
	created := workflowManifestChangelogChanges("", `{"label":"New workflow","capabilities":{"enabled":true}}`)
	wantCreated := []step_based_workflow.PlanFieldChange{
		{Field: "workflow.json", OldValue: "absent", NewValue: "added"},
	}
	if !reflect.DeepEqual(created, wantCreated) {
		t.Fatalf("creation changes = %#v, want %#v", created, wantCreated)
	}

	corrupt := workflowManifestChangelogChanges("not-json", `{"label":"New workflow"}`)
	wantCorrupt := []step_based_workflow.PlanFieldChange{
		{Field: "workflow.json", OldValue: "present", NewValue: "changed"},
	}
	if !reflect.DeepEqual(corrupt, wantCorrupt) {
		t.Fatalf("corrupt prior changes = %#v, want %#v", corrupt, wantCorrupt)
	}
}

func TestValidateManifestCDPPorts(t *testing.T) {
	manifest := NewWorkflowManifest("Multi-profile browser")
	manifest.Capabilities.BrowserMode = "cdp"
	manifest.Capabilities.CDPPorts = []int{9222, 9333}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("valid multi-profile CDP ports rejected: %v", err)
	}

	manifest.Capabilities.CDPPorts = []int{9222, 9222}
	if err := ValidateManifest(manifest); err == nil {
		t.Fatal("duplicate CDP ports should be rejected")
	}

	manifest.Capabilities.CDPPorts = []int{9222, 9333, 9444, 9555, 9666}
	if err := ValidateManifest(manifest); err == nil {
		t.Fatal("more than four CDP ports should be rejected")
	}
}

func TestValidateManifestAdvisorSpecialization(t *testing.T) {
	manifest := NewWorkflowManifest("Specialized advisors")
	manifest.Pulse = &WorkflowPulseConfig{AdvisorSpecialization: &WorkflowAdvisorSpecialization{
		Version:         1,
		StrategyAuditor: "Inspect acquisition concentration within the current strategy.",
		GoalAdvisor:     "Explore credible channels outside the current strategy.",
	}}
	if err := ValidateManifest(manifest); err != nil {
		t.Fatalf("valid advisor specialization rejected: %v", err)
	}

	manifest.Pulse.AdvisorSpecialization.GoalAdvisor = ""
	if err := ValidateManifest(manifest); err == nil || !strings.Contains(err.Error(), "goal_advisor") {
		t.Fatalf("missing Goal Advisor specialization should be rejected, got %v", err)
	}
}

func TestValidateManifestRejectsEquivalentMCPServerAliases(t *testing.T) {
	manifest := NewWorkflowManifest("Duplicate MCP aliases")
	manifest.Capabilities.SelectedServers = []string{"google_sheets", "google-sheets"}

	err := ValidateManifest(manifest)
	if err == nil {
		t.Fatal("equivalent MCP server aliases should be rejected")
	}
	if !strings.Contains(err.Error(), "resolve to the same MCP server") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestNewWorkflowManifestDefaultsGlobalSecretsToNone(t *testing.T) {
	manifest := NewWorkflowManifest("Test workflow")
	if manifest.Version != WorkflowContractCurrentVersion {
		t.Fatalf("Version = %q, want %q", manifest.Version, WorkflowContractCurrentVersion)
	}
	if manifest.Capabilities.SelectedGlobalSecretNames == nil {
		t.Fatal("SelectedGlobalSecretNames = nil, want empty selection")
	}
	if got := len(*manifest.Capabilities.SelectedGlobalSecretNames); got != 0 {
		t.Fatalf("SelectedGlobalSecretNames length = %d, want 0", got)
	}
}

func TestWorkflowCreatorDefaultsGlobalSecretsToNone(t *testing.T) {
	cases := []struct {
		name        string
		workflowMap map[string]interface{}
	}{
		{
			name:        "missing capabilities",
			workflowMap: map[string]interface{}{},
		},
		{
			name: "null global secrets",
			workflowMap: map[string]interface{}{
				"capabilities": map[string]interface{}{
					"selected_global_secret_names": nil,
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defaultWorkflowCreatorGlobalSecretsToNone(tc.workflowMap)

			caps := tc.workflowMap["capabilities"].(map[string]interface{})
			names, ok := caps["selected_global_secret_names"].([]interface{})
			if !ok {
				t.Fatalf("selected_global_secret_names = %T, want []interface{}", caps["selected_global_secret_names"])
			}
			if len(names) != 0 {
				t.Fatalf("selected_global_secret_names length = %d, want 0", len(names))
			}
		})
	}
}

func TestReadWorkflowManifestMigratesMissingLabelFromWorkspacePath(t *testing.T) {
	const workspacePath = "Workflow/instagram"
	manifestJSON, err := json.Marshal(map[string]interface{}{
		"schema_version": 1,
		"id":             "wf_instagram",
		"version":        "1.0.9",
		"capabilities":   map[string]interface{}{},
		"schedules":      []interface{}{},
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	workspace := &mockWorkspaceAPI{files: map[string]string{
		workspacePath + "/workflow.json": string(manifestJSON),
	}}
	server := httptest.NewServer(workspace)
	defer server.Close()
	t.Setenv("WORKSPACE_API_URL", server.URL)

	manifest, found, err := ReadWorkflowManifest(context.Background(), workspacePath)
	if err != nil || !found {
		t.Fatalf("ReadWorkflowManifest() found=%v err=%v", found, err)
	}
	if manifest.Label != "instagram" {
		t.Fatalf("Label = %q, want instagram", manifest.Label)
	}

	workspace.mu.Lock()
	persistedJSON := workspace.files[workspacePath+"/workflow.json"]
	workspace.mu.Unlock()
	var persisted WorkflowManifest
	if err := json.Unmarshal([]byte(persistedJSON), &persisted); err != nil {
		t.Fatalf("unmarshal persisted manifest: %v", err)
	}
	if persisted.Label != "instagram" {
		t.Fatalf("persisted Label = %q, want instagram", persisted.Label)
	}
}

func TestReadWorkflowManifestPrunesRetiredExecutionDefaultsField(t *testing.T) {
	const workspacePath = "Workflow/linkedin"
	manifestJSON, err := json.Marshal(map[string]interface{}{
		"schema_version": 1,
		"id":             "wf_linkedin",
		"version":        "1.0.20",
		"label":          "linkedin",
		"capabilities":   map[string]interface{}{},
		"execution_defaults": map[string]interface{}{
			"always_use_same_run":    true,
			"global_skill_objective": "retired field text",
			"workshop_mode":          "optimizer",
		},
		"schedules": []interface{}{},
	})
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	workspace := &mockWorkspaceAPI{files: map[string]string{
		workspacePath + "/workflow.json": string(manifestJSON),
	}}
	server := httptest.NewServer(workspace)
	defer server.Close()
	t.Setenv("WORKSPACE_API_URL", server.URL)

	manifest, found, err := ReadWorkflowManifest(context.Background(), workspacePath)
	if err != nil || !found {
		t.Fatalf("ReadWorkflowManifest() found=%v err=%v", found, err)
	}
	if manifest.ExecutionDefs.WorkshopMode != "optimizer" || !manifest.ExecutionDefs.AlwaysUseSameRun {
		t.Fatalf("known execution_defaults fields not preserved: %+v", manifest.ExecutionDefs)
	}

	workspace.mu.Lock()
	persistedJSON := workspace.files[workspacePath+"/workflow.json"]
	workspace.mu.Unlock()
	if strings.Contains(persistedJSON, "global_skill_objective") {
		t.Fatalf("persisted manifest still contains retired field global_skill_objective: %s", persistedJSON)
	}
	if !strings.Contains(persistedJSON, "\"workshop_mode\": \"optimizer\"") {
		t.Fatalf("persisted manifest lost known field workshop_mode: %s", persistedJSON)
	}
}
