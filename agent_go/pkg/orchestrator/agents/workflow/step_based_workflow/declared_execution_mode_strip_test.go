package step_based_workflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

func runStripDeclaredMode(t *testing.T, readFile func(context.Context, string) (string, error), writeFile func(context.Context, string, string) error) (string, error) {
	t.Helper()
	exec := createStripDeclaredExecutionModeExecutor(changeStepTypeTestWorkspace, loggerv2.NewNoop(), readFile, writeFile)
	return exec(context.Background(), map[string]interface{}{})
}

func TestStripDeclaredExecutionModeRemovesTheKeysAndKeepsReasonsInTheChangelog(t *testing.T) {
	plan := &PlanningResponse{Steps: []PlanStepInterface{
		&RegularPlanStep{Type: StepTypeRegular, CommonStepFields: CommonStepFields{ID: "scripted", Title: "Scripted", Description: "Deterministic."}},
		testSequenceStep("talk", "Talk"),
	}}
	files, readFile, writeFile := changeStepTypeHarness(t, plan, []StepConfig{
		{ID: "scripted", AgentConfigs: &AgentConfigs{LegacyDeclaredExecutionMode: StepModeScripted, LegacyDeclaredExecutionModeReason: "pure API call", ExecutionTier: "low"}},
		{ID: "talk", AgentConfigs: &AgentConfigs{ExecutionTier: "high"}},
	})
	configPath := normalizePathForWorkspaceAPI("planning/step_config.json", changeStepTypeTestWorkspace)
	if !strings.Contains(files[configPath], "declared_execution_mode") {
		t.Fatal("fixture must start with the retired key on disk")
	}

	out, err := runStripDeclaredMode(t, readFile, writeFile)
	if err != nil {
		t.Fatalf("strip failed: %v", err)
	}
	if !strings.Contains(out, `"status":"migrated"`) || !strings.Contains(out, `"planning_stripped":1`) {
		t.Fatalf("expected one planning entry stripped, got %s", out)
	}
	if strings.Contains(files[configPath], "declared_execution_mode") {
		t.Fatalf("the retired key must be gone from step_config.json, got %s", files[configPath])
	}
	_, configs := readTestPlanAndConfigs(t, readFile)
	if cfg := MatchStepConfigByID("scripted", configs); cfg == nil || cfg.ExecutionTier != "low" || cfg.LegacyDeclaredExecutionMode != "" {
		t.Fatalf("other fields must survive and the key must be cleared, got %+v", cfg)
	}
	if cfg := MatchStepConfigByID("talk", configs); cfg == nil || cfg.ExecutionTier != "high" {
		t.Fatalf("an entry without the key must be untouched, got %+v", cfg)
	}

	entry := findChangelogEntry(t, changeStepTypeTestWorkspace, files, "strip_declared_execution_mode")
	sawMode, sawReason := false, false
	for _, change := range entry.Changes {
		if change.StepID == "scripted" && change.Field == "planning.declared_execution_mode" && change.OldValue == StepModeScripted && change.NewValue == "" {
			sawMode = true
		}
		if change.StepID == "scripted" && change.Field == "planning.declared_execution_mode_reason" && change.OldValue == "pure API call" {
			sawReason = true
		}
	}
	if !sawMode || !sawReason {
		t.Fatalf("changelog must preserve the removed mode and its reason, got %+v", entry.Changes)
	}

	// Idempotent.
	out, err = runStripDeclaredMode(t, readFile, writeFile)
	if err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	if !strings.Contains(out, `"status":"no_op"`) {
		t.Fatalf("second run must be a no-op, got %s", out)
	}
}

func TestStripDeclaredExecutionModeRefusesWhileALegacyAgenticRegularStepRemains(t *testing.T) {
	plan := &PlanningResponse{Steps: []PlanStepInterface{
		&RegularPlanStep{Type: StepTypeRegular, CommonStepFields: CommonStepFields{ID: "legacy", Title: "Legacy", Description: "Ran as a sequence."}},
	}}
	files, readFile, writeFile := changeStepTypeHarness(t, plan, []StepConfig{
		{ID: "legacy", AgentConfigs: &AgentConfigs{LegacyDeclaredExecutionMode: StepModeAgentic, LegacyDeclaredExecutionModeReason: "needs judgment"}},
	})
	configPath := normalizePathForWorkspaceAPI("planning/step_config.json", changeStepTypeTestWorkspace)
	before := files[configPath]

	_, err := runStripDeclaredMode(t, readFile, writeFile)
	if err == nil || !strings.Contains(err.Error(), "legacy") || !strings.Contains(err.Error(), "migrate_declared_execution_mode") {
		t.Fatalf("expected a refusal naming the step and the v1.0.38 migration, got err=%v", err)
	}
	if files[configPath] != before {
		t.Fatal("a refusal must leave step_config.json untouched")
	}
}

func TestStripDeclaredExecutionModeMarksDeclaredScriptedEvaluationStepsOnThePlan(t *testing.T) {
	plan := &PlanningResponse{Steps: []PlanStepInterface{testSequenceStep("talk", "Talk")}}
	files, readFile, writeFile := changeStepTypeHarness(t, plan, nil)
	evalPlan := map[string]interface{}{
		"steps": []map[string]interface{}{
			{"id": "eval-scripted", "title": "Scripted eval", "description": "Checks output deterministically.", "max_score": 10},
			{"id": "eval-judge", "title": "Judge eval", "description": "LLM judgement.", "max_score": 10},
		},
	}
	evalPlanJSON, _ := json.Marshal(evalPlan)
	evalPlanPath := strings.Trim(changeStepTypeTestWorkspace, "/") + "/" + evaluationPlanRelPath
	files[evalPlanPath] = string(evalPlanJSON)
	evalConfigJSON, _ := json.Marshal(StepConfigFile{Steps: []StepConfig{
		{ID: "eval-scripted", AgentConfigs: &AgentConfigs{LegacyDeclaredExecutionMode: StepModeScripted, LegacyDeclaredExecutionModeReason: "deterministic check"}},
		{ID: "eval-judge", AgentConfigs: &AgentConfigs{LegacyDeclaredExecutionMode: StepModeAgentic}},
	}})
	evalConfigPath := normalizePathForWorkspaceAPI("evaluation/step_config.json", changeStepTypeTestWorkspace)
	files[evalConfigPath] = string(evalConfigJSON)

	out, err := runStripDeclaredMode(t, readFile, writeFile)
	if err != nil {
		t.Fatalf("strip failed: %v", err)
	}
	if !strings.Contains(out, `"evaluation_stripped":2`) || !strings.Contains(out, `"evaluation_marked_scripted":1`) {
		t.Fatalf("expected both eval entries stripped and one marked scripted, got %s", out)
	}
	if strings.Contains(files[evalConfigPath], "declared_execution_mode") {
		t.Fatalf("the retired key must be gone from evaluation/step_config.json, got %s", files[evalConfigPath])
	}
	var updatedPlan struct {
		Steps []map[string]interface{} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(files[evalPlanPath]), &updatedPlan); err != nil {
		t.Fatalf("parse updated evaluation plan: %v", err)
	}
	modes := map[string]interface{}{}
	for _, step := range updatedPlan.Steps {
		modes[step["id"].(string)] = step["execution_mode"]
	}
	if modes["eval-scripted"] != StepModeScripted {
		t.Fatalf("the declared-scripted eval step must be marked execution_mode=scripted on the plan, got %v", modes["eval-scripted"])
	}
	if _, set := modes["eval-judge"]; set && modes["eval-judge"] != nil {
		t.Fatalf("an agentic eval step must not get an execution_mode, got %v", modes["eval-judge"])
	}
	entry := findChangelogEntry(t, changeStepTypeTestWorkspace, files, "strip_declared_execution_mode")
	sawPlanMark := false
	for _, change := range entry.Changes {
		if change.StepID == "eval-scripted" && change.Field == "execution_mode" && change.NewValue == StepModeScripted {
			sawPlanMark = true
		}
	}
	if !sawPlanMark {
		t.Fatalf("changelog must record the execution_mode mark, got %+v", entry.Changes)
	}
}

func TestStripDeclaredExecutionModeIsAPlanModificationTool(t *testing.T) {
	if !IsPlanModificationTool("strip_declared_execution_mode") {
		t.Fatal("strip_declared_execution_mode must count as a plan mutation")
	}
}
