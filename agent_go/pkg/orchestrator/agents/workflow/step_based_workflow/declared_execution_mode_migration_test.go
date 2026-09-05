package step_based_workflow

import (
	"context"
	"strings"
	"testing"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

func runDeclaredModeMigration(t *testing.T, readFile func(context.Context, string) (string, error), writeFile func(context.Context, string, string) error) (string, error) {
	t.Helper()
	exec := createMigrateDeclaredExecutionModeExecutor(changeStepTypeTestWorkspace, loggerv2.NewNoop(), readFile, writeFile)
	return exec(context.Background(), map[string]interface{}{})
}

func TestMigrateDeclaredExecutionModeMakesEveryPlanTypeExplicitAndStripsTheField(t *testing.T) {
	plan := &PlanningResponse{Steps: []PlanStepInterface{
		// Legacy agentic regular: no declared mode -> ran as a sequence; becomes one.
		&RegularPlanStep{Type: StepTypeRegular, CommonStepFields: CommonStepFields{ID: "legacy", Title: "Legacy", Description: "Legacy agentic work."}, NextStepID: "scripted"},
		// True scripted step with its script: stays regular, field stripped.
		&RegularPlanStep{Type: StepTypeRegular, CommonStepFields: CommonStepFields{ID: "scripted", Title: "Scripted", Description: "Deterministic."}},
		// Plain sequence: untouched.
		testSequenceStep("talk", "Talk"),
	}}
	configs := []StepConfig{
		{ID: "scripted", AgentConfigs: &AgentConfigs{DeclaredExecutionMode: StepModeScripted, DeclaredExecutionModeReason: "pure API call", ExecutionTier: "low"}},
		{ID: "legacy", AgentConfigs: &AgentConfigs{DeclaredExecutionMode: "agentic", DeclaredExecutionModeReason: "needs judgment"}},
	}
	files, readFile, writeFile := changeStepTypeHarness(t, plan, configs)
	files[normalizePathForWorkspaceAPI("learnings/scripted/main.py", changeStepTypeTestWorkspace)] = "print('ok')"

	out, err := runDeclaredModeMigration(t, readFile, writeFile)
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	if !strings.Contains(out, `"status":"migrated"`) {
		t.Fatalf("expected a migrated status, got %s", out)
	}

	updated, updatedConfigs := readTestPlanAndConfigs(t, readFile)
	legacy, _, _ := findStepByID(updated.Steps, "legacy")
	seq, ok := legacy.(*MessageSequencePlanStep)
	if !ok || seq.NextStepID != "scripted" || seq.Description != "Legacy agentic work." || len(seq.Items) != 1 {
		t.Fatalf("legacy agentic regular step must become an explicit sequence with its fields kept, got %T %+v", legacy, legacy)
	}
	if scripted, _, _ := findStepByID(updated.Steps, "scripted"); scripted.StepType() != StepTypeRegular {
		t.Fatalf("a true scripted step must stay regular, got %s", scripted.StepType())
	}
	if talk, _, _ := findStepByID(updated.Steps, "talk"); talk.StepType() != StepTypeMessageSeq {
		t.Fatalf("a plain sequence must be untouched, got %s", talk.StepType())
	}
	for _, id := range []string{"scripted", "legacy"} {
		cfg := MatchStepConfigByID(id, updatedConfigs)
		if cfg == nil || cfg.DeclaredExecutionMode != "" || cfg.DeclaredExecutionModeReason != "" {
			t.Fatalf("declared_execution_mode must be stripped from %q, got %+v", id, cfg)
		}
	}
	if cfg := MatchStepConfigByID("scripted", updatedConfigs); cfg.ExecutionTier != "low" {
		t.Fatalf("other step_config fields must survive, got %+v", cfg)
	}

	entry := findChangelogEntry(t, changeStepTypeTestWorkspace, files, "migrate_declared_execution_mode")
	sawReason := false
	sawType := false
	for _, change := range entry.Changes {
		if change.Field == "declared_execution_mode_reason" && change.OldValue == "pure API call" {
			sawReason = true
		}
		if change.StepID == "legacy" && change.Field == "type" && change.NewValue == string(StepTypeMessageSeq) {
			sawType = true
		}
	}
	if !sawReason || !sawType {
		t.Fatalf("changelog must preserve the removed reasons and record the type change, got %+v", entry.Changes)
	}
	if len(entry.DeletedSteps) != 1 || len(entry.AddedSteps) != 1 {
		t.Fatalf("changelog must carry the old/new step JSON for the one type change, got deleted=%d added=%d", len(entry.DeletedSteps), len(entry.AddedSteps))
	}
}

func TestMigrateDeclaredExecutionModeRepairsADeclaredScriptedSequence(t *testing.T) {
	seq := testSequenceStep("drifted", "Drifted")
	plan := &PlanningResponse{Steps: []PlanStepInterface{seq}}
	files, readFile, writeFile := changeStepTypeHarness(t, plan, []StepConfig{{ID: "drifted", AgentConfigs: &AgentConfigs{DeclaredExecutionMode: StepModeScripted}}})
	files[normalizePathForWorkspaceAPI("learnings/drifted/main.py", changeStepTypeTestWorkspace)] = "print('ok')"

	if _, err := runDeclaredModeMigration(t, readFile, writeFile); err != nil {
		t.Fatalf("migration failed: %v", err)
	}
	updated, _ := readTestPlanAndConfigs(t, readFile)
	if step, _, _ := findStepByID(updated.Steps, "drifted"); step.StepType() != StepTypeRegular {
		t.Fatalf("a declared-scripted sequence must become regular (PLAT-280 drift), got %s", step.StepType())
	}
}

func TestMigrateDeclaredExecutionModeRefusesADeclaredScriptedStepWithoutAScript(t *testing.T) {
	plan := &PlanningResponse{Steps: []PlanStepInterface{
		&RegularPlanStep{Type: StepTypeRegular, CommonStepFields: CommonStepFields{ID: "legacy", Title: "Legacy", Description: "Would be converted."}},
		&RegularPlanStep{Type: StepTypeRegular, CommonStepFields: CommonStepFields{ID: "no-script", Title: "No script", Description: "Declared scripted, no main.py."}},
	}}
	files, readFile, writeFile := changeStepTypeHarness(t, plan, []StepConfig{{ID: "no-script", AgentConfigs: &AgentConfigs{DeclaredExecutionMode: StepModeScripted}}})
	planBefore := files[normalizePathForWorkspaceAPI("planning/plan.json", changeStepTypeTestWorkspace)]
	configBefore := files[normalizePathForWorkspaceAPI("planning/step_config.json", changeStepTypeTestWorkspace)]

	_, err := runDeclaredModeMigration(t, readFile, writeFile)
	if err == nil || !strings.Contains(err.Error(), "no-script") {
		t.Fatalf("expected a refusal naming the broken step, got err=%v", err)
	}
	if files[normalizePathForWorkspaceAPI("planning/plan.json", changeStepTypeTestWorkspace)] != planBefore ||
		files[normalizePathForWorkspaceAPI("planning/step_config.json", changeStepTypeTestWorkspace)] != configBefore {
		t.Fatal("a refusal must leave plan.json and step_config.json untouched -- including the legacy step it would otherwise have converted")
	}
}

func TestMigrateDeclaredExecutionModeIsANoOpOnACleanWorkflow(t *testing.T) {
	plan := &PlanningResponse{Steps: []PlanStepInterface{testSequenceStep("talk", "Talk")}}
	files, readFile, writeFile := changeStepTypeHarness(t, plan, []StepConfig{{ID: "talk", AgentConfigs: &AgentConfigs{ExecutionTier: "high"}}})
	before := len(files)

	out, err := runDeclaredModeMigration(t, readFile, writeFile)
	if err != nil {
		t.Fatalf("no-op migration errored: %v", err)
	}
	if !strings.Contains(out, `"status":"no_op"`) || len(files) != before {
		t.Fatalf("a clean workflow must be a no-op with no writes, got %s with %d files", out, len(files))
	}
}

func TestMigrateDeclaredExecutionModeIsAPlanModificationTool(t *testing.T) {
	if !IsPlanModificationTool("migrate_declared_execution_mode") {
		t.Fatal("migrate_declared_execution_mode must count as a plan mutation")
	}
}
