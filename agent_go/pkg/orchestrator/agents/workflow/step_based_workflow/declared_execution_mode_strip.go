package step_based_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

// workflowContractDeclaredExecutionModeStrippedVersionLabel names the
// contract version the v1.0.39 migration stamps. cmd/server owns the ladder
// (workflowContractDeclaredExecutionModeStrippedVersion); keep them in step.
const workflowContractDeclaredExecutionModeStrippedVersionLabel = "1.0.39"

// strippedDeclaredMode is one removed declared_execution_mode entry.
type strippedDeclaredMode struct {
	StepID string
	Scope  string // "planning" or "evaluation"
	Mode   string
	Reason string
}

// stripDeclaredExecutionModeFromConfigs clears the retired keys on every
// config that still carries them and reports what was removed.
func stripDeclaredExecutionModeFromConfigs(scope string, configs []StepConfig) []strippedDeclaredMode {
	var stripped []strippedDeclaredMode
	for i := range configs {
		cfg := configs[i].AgentConfigs
		if cfg == nil || (cfg.LegacyDeclaredExecutionMode == "" && cfg.LegacyDeclaredExecutionModeReason == "") {
			continue
		}
		stripped = append(stripped, strippedDeclaredMode{
			StepID: configs[i].ID,
			Scope:  scope,
			Mode:   cfg.legacyDeclared(),
			Reason: strings.TrimSpace(cfg.LegacyDeclaredExecutionModeReason),
		})
		cfg.LegacyDeclaredExecutionMode = ""
		cfg.LegacyDeclaredExecutionModeReason = ""
	}
	return stripped
}

// legacyAgenticRegularStepIDs lists the regular steps that still run as a
// message_sequence only because of the retired key. Stripping it would flip
// them to scripted, so the v1.0.38 migration has to have converted them first.
func legacyAgenticRegularStepIDs(plan *PlanningResponse, configs []StepConfig) []string {
	var ids []string
	for _, info := range append(collectAllSteps(plan.Steps), collectAllSteps(plan.OrphanSteps)...) {
		if info.Step == nil {
			continue
		}
		if isLegacyAgenticRegularStep(info.Step, MatchStepConfigByID(info.Step.GetID(), configs)) {
			ids = append(ids, info.Step.GetID())
		}
	}
	return ids
}

// createStripDeclaredExecutionModeExecutor is the trusted tool behind the
// v1.0.39 contract upgrade (see upgradeDeclaredExecutionModeStripped in
// cmd/server), the second half of PLAT-287. The runtime now decides a step's
// execution model from its plan type alone, so the retired
// declared_execution_mode / declared_execution_mode_reason keys are removed
// from planning/step_config.json and evaluation/step_config.json. An
// evaluation step that was declared scripted first gets
// execution_mode="scripted" on its evaluation_plan.json entry, so nothing
// changes how it runs. Every removed reason is kept in the changelog entry.
func createStripDeclaredExecutionModeExecutor(
	workspacePath string,
	logger loggerv2.Logger,
	readFile func(context.Context, string) (string, error),
	writeFile func(context.Context, string, string) error,
) func(context.Context, map[string]interface{}) (string, error) {
	return func(ctx context.Context, _ map[string]interface{}) (string, error) {
		plan, err := readPlanFromFile(ctx, workspacePath, readFile)
		if err != nil {
			return "", fmt.Errorf("read planning/plan.json: %w", err)
		}
		configs, err := readStepConfigViaFileCallback(ctx, workspacePath, readFile)
		if err != nil {
			return "", fmt.Errorf("read planning/step_config.json: %w", err)
		}
		if legacy := legacyAgenticRegularStepIDs(plan, configs); len(legacy) > 0 {
			return "", fmt.Errorf("refusing to strip declared_execution_mode: regular step(s) %s still carry declared_execution_mode=\"agentic\" and would flip to scripted without it; run migrate_declared_execution_mode (workflow contract v1.0.38) first so they become explicit message_sequence steps", strings.Join(legacy, ", "))
		}

		// Evaluation side: optional files. A declared-scripted evaluation step
		// keeps running its script only if the plan entry says so, so mark the
		// plan before the config loses the key.
		evalConfigPath := normalizePathForWorkspaceAPI("evaluation/step_config.json", workspacePath)
		var evalConfigs []StepConfig
		evalConfigExists := false
		if content, readErr := readFile(ctx, evalConfigPath); readErr == nil && strings.TrimSpace(content) != "" {
			evalConfigs, err = ParseStepConfigContent(content)
			if err != nil {
				return "", fmt.Errorf("read evaluation/step_config.json: %w", err)
			}
			evalConfigExists = true
		}
		scriptedEvalIDs := map[string]bool{}
		for _, sc := range evalConfigs {
			if sc.AgentConfigs.legacyDeclared() == StepModeScripted {
				scriptedEvalIDs[sc.ID] = true
			}
		}
		var evalPlanChanges []PlanFieldChange
		if len(scriptedEvalIDs) > 0 {
			evalPlanPath, document, stepsKey, rawSteps, loadErr := loadEvaluationPlanDocument(ctx, workspacePath, readFile)
			if loadErr != nil {
				return "", fmt.Errorf("evaluation step(s) declared scripted but the evaluation plan cannot be read to record execution_mode: %w", loadErr)
			}
			for _, entry := range rawSteps {
				step, isObject := entry.(map[string]interface{})
				if !isObject {
					continue
				}
				id, _ := step["id"].(string)
				if !scriptedEvalIDs[strings.TrimSpace(id)] {
					continue
				}
				current, _ := step["execution_mode"].(string)
				if canonicalDeclaredExecutionMode(current) == StepModeScripted {
					continue
				}
				step["execution_mode"] = StepModeScripted
				evalPlanChanges = append(evalPlanChanges, PlanFieldChange{StepID: strings.TrimSpace(id), Field: "execution_mode", OldValue: current, NewValue: StepModeScripted})
			}
			if len(evalPlanChanges) > 0 {
				document[stepsKey] = rawSteps
				out, marshalErr := json.MarshalIndent(document, "", "  ")
				if marshalErr != nil {
					return "", fmt.Errorf("marshal evaluation plan: %w", marshalErr)
				}
				if err := writeFile(ctx, evalPlanPath, string(out)); err != nil {
					return "", fmt.Errorf("write evaluation/evaluation_plan.json: %w", err)
				}
			}
		}

		planningStripped := stripDeclaredExecutionModeFromConfigs("planning", configs)
		evalStripped := stripDeclaredExecutionModeFromConfigs("evaluation", evalConfigs)
		if len(planningStripped) == 0 && len(evalStripped) == 0 && len(evalPlanChanges) == 0 {
			return `{"status":"no_op","message":"No step_config entry carries declared_execution_mode any more; the plan type alone states each step's execution model."}`, nil
		}

		if len(planningStripped) > 0 {
			if err := writeStepConfigViaFileCallback(ctx, workspacePath, configs, writeFile); err != nil {
				return "", fmt.Errorf("write planning/step_config.json: %w", err)
			}
		}
		if evalConfigExists && len(evalStripped) > 0 {
			out, marshalErr := json.MarshalIndent(StepConfigFile{Steps: evalConfigs}, "", "  ")
			if marshalErr != nil {
				return "", fmt.Errorf("marshal evaluation/step_config.json: %w", marshalErr)
			}
			if err := writeFile(ctx, evalConfigPath, string(out)); err != nil {
				return "", fmt.Errorf("write evaluation/step_config.json: %w", err)
			}
		}

		stepIDs := make([]string, 0, len(planningStripped)+len(evalStripped))
		changes := append([]PlanFieldChange{}, evalPlanChanges...)
		for _, item := range append(planningStripped, evalStripped...) {
			stepIDs = append(stepIDs, item.StepID)
			changes = append(changes, PlanFieldChange{StepID: item.StepID, Field: item.Scope + ".declared_execution_mode", OldValue: item.Mode, NewValue: ""})
			if item.Reason != "" {
				changes = append(changes, PlanFieldChange{StepID: item.StepID, Field: item.Scope + ".declared_execution_mode_reason", OldValue: item.Reason, NewValue: ""})
			}
		}
		logPlanChange(ctx, workspacePath, PlanChangelogEntry{
			Tool: "strip_declared_execution_mode",
			Reason: fmt.Sprintf(
				"Workflow contract v%s (PLAT-287, half 2): the plan type alone decides how a step runs, so the retired declared_execution_mode keys were removed from %d planning and %d evaluation step_config entries (original reasons preserved in the changes below); %d evaluation step(s) marked execution_mode=scripted in evaluation_plan.json so they keep running their script.",
				workflowContractDeclaredExecutionModeStrippedVersionLabel, len(planningStripped), len(evalStripped), len(evalPlanChanges),
			),
			StepIDs: stepIDs,
			Changes: changes,
		}, readFile, writeFile, logger)

		logger.Info(fmt.Sprintf("✅ strip_declared_execution_mode: planning=%d evaluation=%d eval_plan_marked=%d", len(planningStripped), len(evalStripped), len(evalPlanChanges)))
		return fmt.Sprintf(`{"status":"migrated","planning_stripped":%d,"evaluation_stripped":%d,"evaluation_marked_scripted":%d,"message":"declared_execution_mode removed; the plan type states each step's execution model. Call set_workflow_contract_version(version=%q)."}`,
			len(planningStripped), len(evalStripped), len(evalPlanChanges), workflowContractDeclaredExecutionModeStrippedVersionLabel), nil
	}
}
