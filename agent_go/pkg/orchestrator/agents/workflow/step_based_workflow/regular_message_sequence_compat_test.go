package step_based_workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

func TestScriptedToolNamesArePlanMutationsAndLegacyNamesAreNot(t *testing.T) {
	for _, name := range []string{"add_scripted_step", "update_scripted_step"} {
		if !IsPlanModificationTool(name) {
			t.Fatalf("expected %q to be recognized as a plan mutation", name)
		}
	}
	for _, name := range []string{"add_regular_step", "update_regular_step"} {
		if IsPlanModificationTool(name) {
			t.Fatalf("legacy authoring tool %q must not remain callable", name)
		}
	}
}

func TestScriptedToolSchemasExposeNextStepID(t *testing.T) {
	for name, schema := range map[string]string{
		"add_scripted_step":    getAddRegularStepSchema(),
		"update_scripted_step": getUpdateRegularStepSchema(),
	} {
		var decoded struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal([]byte(schema), &decoded); err != nil {
			t.Fatalf("%s schema is invalid JSON: %v", name, err)
		}
		if _, ok := decoded.Properties["next_step_id"]; !ok {
			t.Fatalf("%s schema does not expose next_step_id", name)
		}
	}
}

func TestScriptedStepNextStepIDRoundTripAndUpdate(t *testing.T) {
	original := &RegularPlanStep{
		Type:             StepTypeRegular,
		CommonStepFields: CommonStepFields{ID: "script-a"},
		NextStepID:       "script-b",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RegularPlanStep
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.NextStepID != "script-b" {
		t.Fatalf("round-trip next_step_id = %q", decoded.NextStepID)
	}

	updated := mergePartialStepUpdate(original, PartialPlanStep{NextStepID: "shared"}).(*RegularPlanStep)
	if updated.NextStepID != "shared" {
		t.Fatalf("updated next_step_id = %q, want shared", updated.NextStepID)
	}
}

func TestValidateScriptedStepUpdateTarget(t *testing.T) {
	plan := &PlanningResponse{Steps: []PlanStepInterface{
		&RegularPlanStep{CommonStepFields: CommonStepFields{ID: "scripted"}},
		&RegularPlanStep{CommonStepFields: CommonStepFields{ID: "legacy-agentic"}},
		&MessageSequencePlanStep{CommonStepFields: CommonStepFields{ID: "sequence"}},
	}}
	configs := []StepConfig{{
		ID:           "scripted",
		AgentConfigs: &AgentConfigs{DeclaredExecutionMode: StepModeScripted},
	}}

	if err := validateScriptedStepUpdateTarget(plan, configs, "scripted"); err != nil {
		t.Fatalf("declared scripted step was rejected: %v", err)
	}
	if err := validateScriptedStepUpdateTarget(plan, configs, "legacy-agentic"); err == nil {
		t.Fatal("legacy agentic regular step must not be editable through update_scripted_step")
	} else if !strings.Contains(err.Error(), "update_message_sequence_step") {
		t.Fatalf("legacy rejection must identify the working compatibility path: %v", err)
	}
	if err := validateScriptedStepUpdateTarget(plan, configs, "sequence"); err == nil {
		t.Fatal("message_sequence must use its type-specific update tool")
	}
}

func TestPrepareMessageSequenceUpdateTargetUpgradesLegacyAgenticRegular(t *testing.T) {
	legacy := &RegularPlanStep{
		Type: StepTypeRegular,
		CommonStepFields: CommonStepFields{
			ID:          "legacy-agentic",
			Title:       "Collect latency",
			Description: "Collect the latency evidence.",
		},
		NextStepID: "end",
	}
	plan := &PlanningResponse{Steps: []PlanStepInterface{legacy}}
	configs := []StepConfig{{
		ID:           legacy.ID,
		AgentConfigs: &AgentConfigs{DeclaredExecutionMode: StepModeAgentic},
	}}

	upgraded, err := prepareMessageSequenceUpdateTarget(plan, configs, legacy.ID)
	if err != nil {
		t.Fatalf("legacy compatibility upgrade failed: %v", err)
	}
	if !upgraded {
		t.Fatal("expected legacy agentic regular step to be upgraded")
	}
	sequence, ok := plan.Steps[0].(*MessageSequencePlanStep)
	if !ok {
		t.Fatalf("upgraded step type = %T, want *MessageSequencePlanStep", plan.Steps[0])
	}
	if sequence.ID != legacy.ID || sequence.Description != legacy.Description || sequence.NextStepID != "end" {
		t.Fatalf("compatibility upgrade lost existing fields: %#v", sequence)
	}
	if len(sequence.Items) != 1 || sequence.Items[0].ID != normalizedRegularSequenceItemID {
		t.Fatalf("compatibility upgrade did not preserve effective runtime queue: %#v", sequence.Items)
	}
}

func TestPrepareMessageSequenceUpdateTargetRejectsScriptedRegular(t *testing.T) {
	plan := &PlanningResponse{Steps: []PlanStepInterface{
		&RegularPlanStep{
			Type:             StepTypeRegular,
			CommonStepFields: CommonStepFields{ID: "scripted"},
		},
	}}
	configs := []StepConfig{{
		ID:           "scripted",
		AgentConfigs: &AgentConfigs{DeclaredExecutionMode: StepModeScripted},
	}}

	upgraded, err := prepareMessageSequenceUpdateTarget(plan, configs, "scripted")
	if err == nil || !strings.Contains(err.Error(), "update_scripted_step") {
		t.Fatalf("scripted regular rejection = %v, want update_scripted_step guidance", err)
	}
	if upgraded {
		t.Fatal("declared scripted step must never be converted")
	}
	if _, ok := plan.Steps[0].(*RegularPlanStep); !ok {
		t.Fatalf("rejected scripted step was mutated to %T", plan.Steps[0])
	}
}

func TestUpdateMessageSequenceExecutorAtomicallyPersistsLegacyUpgrade(t *testing.T) {
	legacyPlan := &PlanningResponse{Steps: []PlanStepInterface{
		&RegularPlanStep{
			Type: StepTypeRegular,
			CommonStepFields: CommonStepFields{
				ID:                  "voice-latency-collector",
				Title:               "Voice latency collector",
				Description:         "Collect voice latency.",
				ContextDependencies: []string{"input.json"},
				ContextOutput:       FlexibleContextOutput("latency.json"),
			},
			NextStepID: "end",
		},
	}}
	planJSON, err := json.Marshal(legacyPlan)
	if err != nil {
		t.Fatalf("marshal legacy plan: %v", err)
	}
	stepConfigJSON := `{"steps":[{"id":"voice-latency-collector","agent_configs":{"declared_execution_mode":"agentic"}}]}`
	var writtenPlan string
	readFile := func(_ context.Context, path string) (string, error) {
		switch {
		case strings.HasSuffix(path, "planning/plan.json"):
			return string(planJSON), nil
		case strings.HasSuffix(path, "planning/step_config.json"):
			return stepConfigJSON, nil
		default:
			return "", errors.New("not found")
		}
	}
	writeFile := func(_ context.Context, path, content string) error {
		if strings.HasSuffix(path, "planning/plan.json") {
			writtenPlan = content
		}
		return nil
	}
	executor := createUpdateMessageSequenceStepExecutor("workflow", loggerv2.NewNoop(), readFile, writeFile)

	result, err := executor(context.Background(), map[string]interface{}{
		"existing_step_id": "voice-latency-collector",
		"description":      "Collect voice latency and verify per-language sample coverage.",
		"items": []interface{}{
			map[string]interface{}{
				"id":      "verify-language-coverage",
				"type":    "user_message",
				"message": "Verify every supported language has non-zero samples and repair missing evidence.",
			},
		},
		"reason": "repair the recurring missing per-language measurement evidence",
	})
	if err != nil {
		t.Fatalf("legacy message-sequence update failed: %v", err)
	}
	if !strings.Contains(result, "upgraded its saved legacy regular type to message_sequence") {
		t.Fatalf("update result did not disclose compatibility upgrade: %s", result)
	}
	if writtenPlan == "" {
		t.Fatal("updated plan was not persisted")
	}

	var persisted PlanningResponse
	if err := json.Unmarshal([]byte(writtenPlan), &persisted); err != nil {
		t.Fatalf("decode persisted plan: %v", err)
	}
	sequence, ok := persisted.Steps[0].(*MessageSequencePlanStep)
	if !ok {
		t.Fatalf("persisted step type = %T, want *MessageSequencePlanStep", persisted.Steps[0])
	}
	if sequence.Description != "Collect voice latency and verify per-language sample coverage." {
		t.Fatalf("description was not updated: %q", sequence.Description)
	}
	if len(sequence.Items) != 1 || sequence.Items[0].ID != "verify-language-coverage" {
		t.Fatalf("items were not updated: %#v", sequence.Items)
	}
	if sequence.ContextOutput.String() != "latency.json" || sequence.NextStepID != "end" {
		t.Fatalf("legacy fields were not preserved: %#v", sequence)
	}
}

func TestNonScriptedRegularStepNormalizesToMessageSequence(t *testing.T) {
	validation := &ValidationSchema{}
	config := &AgentConfigs{DeclaredExecutionMode: StepModeAgentic}
	regular := &RegularPlanStep{
		Type: StepTypeRegular,
		CommonStepFields: CommonStepFields{
			ID:                  "analyze-results",
			Title:               "Analyze results",
			Description:         "Analyze the saved evidence and explain the result.",
			ContextDependencies: []string{"fetch.json"},
			ContextOutput:       FlexibleContextOutput("analysis.json"),
			ValidationSchema:    validation,
		},
		NextStepID:   "shared",
		AgentConfigs: config,
	}

	if !shouldNormalizeRegularStepToMessageSequence(regular) {
		t.Fatal("expected a non-scripted regular step to use message-sequence normalization")
	}
	sequence := normalizeRegularStepToMessageSequence(regular)
	if sequence == nil || sequence.StepType() != StepTypeMessageSeq {
		t.Fatalf("expected message_sequence normalization, got %#v", sequence)
	}
	if sequence.ID != regular.ID || sequence.Title != regular.Title || sequence.Description != regular.Description {
		t.Fatalf("normalization changed step identity or instructions: %#v", sequence)
	}
	if sequence.ContextOutput != regular.ContextOutput || sequence.ValidationSchema != validation || sequence.AgentConfigs != config {
		t.Fatal("normalization did not preserve output, validation, or agent configuration")
	}
	if sequence.NextStepID != "shared" {
		t.Fatalf("normalization lost next_step_id: %q", sequence.NextStepID)
	}
	if len(sequence.Items) != 1 || sequence.Items[0].ID != normalizedRegularSequenceItemID || sequence.Items[0].Type != "user_message" {
		t.Fatalf("expected one normalized work turn, got %#v", sequence.Items)
	}
	if got := effectiveRuntimeStepType(regular); got != string(StepTypeMessageSeq) {
		t.Fatalf("effective runtime step type = %q, want %q", got, StepTypeMessageSeq)
	}
}

func TestScriptedRegularStepDoesNotNormalizeToMessageSequence(t *testing.T) {
	regular := &RegularPlanStep{
		CommonStepFields: CommonStepFields{ID: "fetch-data"},
		AgentConfigs:     &AgentConfigs{DeclaredExecutionMode: StepModeScripted},
	}
	if shouldNormalizeRegularStepToMessageSequence(regular) {
		t.Fatal("scripted regular step must retain the saved-script execution path")
	}
	if got := effectiveRuntimeStepType(regular); got != string(StepTypeRegular) {
		t.Fatalf("effective runtime step type = %q, want %q", got, StepTypeRegular)
	}
}

func TestUpsertNewScriptedRegularStepConfig(t *testing.T) {
	configs := []StepConfig{{ID: "fetch-data", AgentConfigs: nil}}
	configs = upsertNewScriptedRegularStepConfig(configs, "fetch-data", "Fetch data")
	if len(configs) != 1 {
		t.Fatalf("expected existing config to be updated, got %d entries", len(configs))
	}
	cfg := configs[0]
	if cfg.Title != "Fetch data" || cfg.AgentConfigs == nil {
		t.Fatalf("missing updated scripted config: %#v", cfg)
	}
	if cfg.AgentConfigs.DeclaredExecutionMode != StepModeScripted {
		t.Fatalf("expected scripted mode, got %q", cfg.AgentConfigs.DeclaredExecutionMode)
	}
	if cfg.AgentConfigs.UseCodeExecutionMode == nil || !*cfg.AgentConfigs.UseCodeExecutionMode {
		t.Fatal("scripted mode did not enable code execution")
	}

	configs = upsertNewScriptedRegularStepConfig(configs, "normalize-data", "Normalize data")
	if len(configs) != 2 || configs[1].ID != "normalize-data" || configs[1].AgentConfigs == nil {
		t.Fatalf("expected a new scripted config entry, got %#v", configs)
	}
}

func TestCollectRegularPlanStepsIncludesNestedTodoRoutes(t *testing.T) {
	regular := &RegularPlanStep{CommonStepFields: CommonStepFields{ID: "fetch-data", Title: "Fetch data"}}
	sequence := &MessageSequencePlanStep{CommonStepFields: CommonStepFields{ID: "analyze-data"}}
	nested := &OrchestratorPlanStep{
		CommonStepFields: CommonStepFields{ID: "nested"},
		PredefinedRoutes: []PlanOrchestrationRoute{
			{RouteID: "fetch-data", SubAgentStep: regular},
			{RouteID: "analyze-data", SubAgentStep: sequence},
		},
	}
	root := &OrchestratorPlanStep{
		CommonStepFields: CommonStepFields{ID: "root"},
		PredefinedRoutes: []PlanOrchestrationRoute{{RouteID: "nested", SubAgentStep: nested}},
	}

	got := collectRegularPlanSteps(root)
	if len(got) != 1 || got[0] != regular {
		t.Fatalf("expected only the nested regular boundary, got %#v", got)
	}
}

func TestOrchestratorRejectsIncompleteMessageSequenceRoute(t *testing.T) {
	step := &OrchestratorPlanStep{
		CommonStepFields: CommonStepFields{
			ID:          "orchestrate",
			Title:       "Orchestrate",
			Description: "Delegate work.",
		},
		NextStepID: "end",
		PredefinedRoutes: []PlanOrchestrationRoute{{
			RouteID:   "analyze",
			RouteName: "Analyze",
			SubAgentStep: &MessageSequencePlanStep{CommonStepFields: CommonStepFields{
				ID:          "analyze",
				Title:       "Analyze",
				Description: "Analyze the evidence.",
			}},
		}},
	}

	if err := validateOrchestratorStepFieldsTyped(step); err == nil {
		t.Fatal("expected an empty message_sequence route to fail validation")
	}
	step.PredefinedRoutes[0].SubAgentStep.(*MessageSequencePlanStep).Items = []MessageSequenceItem{{
		ID:      "analyze",
		Type:    "user_message",
		Message: "Analyze the evidence and save the result.",
	}}
	if err := validateOrchestratorStepFieldsTyped(step); err != nil {
		t.Fatalf("expected a complete message_sequence route to pass validation: %v", err)
	}
}
