package step_based_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

// workflowContractDeclaredExecutionModeRetiredVersionLabel names the contract
// version this migration belongs to in its changelog reason. The
// authoritative constant is cmd/server's
// workflowContractDeclaredExecutionModeRetiredVersion; keep them in step.
const workflowContractDeclaredExecutionModeRetiredVersionLabel = "1.0.38"

// declaredModeTypeChange records one plan-type rewrite the migration made.
type declaredModeTypeChange struct {
	StepID string `json:"step_id"`
	From   string `json:"from"`
	To     string `json:"to"`
	before PlanStepInterface
	after  PlanStepInterface
}

// declaredModeRemoval records one step_config entry the field was stripped from.
type declaredModeRemoval struct {
	StepID string `json:"step_id"`
	Mode   string `json:"mode"`
	Reason string `json:"reason,omitempty"`
}

type declaredModeMigrationResult struct {
	Configs     []StepConfig             `json:"-"`
	TypeChanges []declaredModeTypeChange `json:"type_changes"`
	Removals    []declaredModeRemoval    `json:"removals"`
}

func (r *declaredModeMigrationResult) noOp() bool {
	return len(r.TypeChanges) == 0 && len(r.Removals) == 0
}

// migrateDeclaredExecutionModeInPlan makes the plan say explicitly what the
// runtime already did (PLAT-287): a regular step without a declared scripted
// mode was the legacy agentic shape that ran as a message_sequence, so it
// becomes one; a message_sequence still declared scripted (PLAT-280 drift)
// becomes regular; then declared_execution_mode and its reason are stripped
// from every step_config entry. Two passes: the first only looks for the one
// case this refuses to guess about -- a step declared scripted with no
// learnings/<id>/main.py, which is already broken -- so a refusal leaves
// plan and configs untouched.
func migrateDeclaredExecutionModeInPlan(plan *PlanningResponse, configs []StepConfig, scriptExists func(stepID string) bool) (*declaredModeMigrationResult, error) {
	if plan == nil {
		return nil, fmt.Errorf("plan is unavailable")
	}
	infos := append(collectAllSteps(plan.Steps), collectAllSteps(plan.OrphanSteps)...)

	declaredScripted := func(stepID string) bool {
		return isScriptedExecutionModeConfig(MatchStepConfigByID(stepID, configs))
	}

	// Pass 1: refuse before touching anything.
	var broken []string
	for _, info := range infos {
		step := info.Step
		if step == nil {
			continue
		}
		switch step.(type) {
		case *RegularPlanStep, *MessageSequencePlanStep:
			if declaredScripted(step.GetID()) && !scriptExists(step.GetID()) {
				broken = append(broken, step.GetID())
			}
		}
	}
	if len(broken) > 0 {
		return nil, fmt.Errorf("refusing to migrate: step(s) %s are declared scripted but have no learnings/<step-id>/main.py -- they are already broken, and this migration will not guess whether each should become a message_sequence (change_step_type target_type=message_sequence) or get its script written (update_scripted_step code=...); fix them first, then re-run", strings.Join(broken, ", "))
	}

	// Pass 2: rewrite plan types.
	result := &declaredModeMigrationResult{Configs: configs}
	replace := func(stepID string, replacement PlanStepInterface) error {
		replaced, _ := replaceStepRecursively(plan.Steps, stepID, replacement)
		if !replaced {
			replaced, _ = replaceStepRecursively(plan.OrphanSteps, stepID, replacement)
		}
		if !replaced {
			return fmt.Errorf("failed to replace step %q in the plan", stepID)
		}
		return nil
	}
	for _, info := range infos {
		switch step := info.Step.(type) {
		case *RegularPlanStep:
			if declaredScripted(step.GetID()) {
				continue // a true scripted step: type already says so
			}
			replacement := normalizeRegularStepToMessageSequence(step)
			if err := replace(step.GetID(), replacement); err != nil {
				return nil, err
			}
			result.TypeChanges = append(result.TypeChanges, declaredModeTypeChange{
				StepID: step.GetID(), From: string(StepTypeRegular), To: string(StepTypeMessageSeq), before: step, after: replacement,
			})
		case *MessageSequencePlanStep:
			if !declaredScripted(step.GetID()) {
				continue
			}
			replacement := normalizeMessageSequenceStepToRegular(step)
			if err := replace(step.GetID(), replacement); err != nil {
				return nil, err
			}
			result.TypeChanges = append(result.TypeChanges, declaredModeTypeChange{
				StepID: step.GetID(), From: string(StepTypeMessageSeq), To: string(StepTypeRegular), before: step, after: replacement,
			})
		}
	}

	// Pass 3: strip the field everywhere, preserving what it said.
	for i := range result.Configs {
		cfg := result.Configs[i].AgentConfigs
		if cfg == nil || (cfg.DeclaredExecutionMode == "" && cfg.DeclaredExecutionModeReason == "") {
			continue
		}
		result.Removals = append(result.Removals, declaredModeRemoval{
			StepID: result.Configs[i].ID,
			Mode:   canonicalDeclaredExecutionMode(cfg.DeclaredExecutionMode),
			Reason: strings.TrimSpace(cfg.DeclaredExecutionModeReason),
		})
		cfg.DeclaredExecutionMode = ""
		cfg.DeclaredExecutionModeReason = ""
	}
	return result, nil
}

// createMigrateDeclaredExecutionModeExecutor is the trusted tool behind the
// v1.0.38 contract upgrade (see upgradeDeclaredExecutionModeRetired in
// cmd/server). It changes no behavior: it writes down the execution model
// each step already had, in the one place that will carry it from now on --
// the plan type -- and records the removed declarations in the changelog.
func createMigrateDeclaredExecutionModeExecutor(
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
		before, err := json.Marshal(plan)
		if err != nil {
			return "", fmt.Errorf("snapshot plan: %w", err)
		}
		scriptExists := func(stepID string) bool {
			content, readErr := readFile(ctx, normalizePathForWorkspaceAPI(filepath.Join("learnings", stepID, "main.py"), workspacePath))
			return readErr == nil && strings.TrimSpace(content) != ""
		}

		result, err := migrateDeclaredExecutionModeInPlan(plan, configs, scriptExists)
		if err != nil {
			return "", err
		}
		if result.noOp() {
			return `{"status":"no_op","message":"Every plan type already states its execution model and no step_config entry carries declared_execution_mode."}`, nil
		}

		if err := validatePlanStepIDs(plan.Steps); err != nil {
			return "", fmt.Errorf("validate migrated plan: %w", err)
		}
		if err := validateStepIDUniqueness(plan); err != nil {
			return "", fmt.Errorf("validate migrated plan: %w", err)
		}
		if err := validateCrossPlanStepIDUniqueness(ctx, workspacePath, readFile, plan); err != nil {
			return "", fmt.Errorf("validate migrated plan: %w", err)
		}
		if err := ValidatePlanStructure(plan); err != nil {
			return "", fmt.Errorf("validate migrated plan: %w", err)
		}

		if len(result.TypeChanges) > 0 {
			if err := writePlanToFile(ctx, workspacePath, plan, readFile, writeFile, logger); err != nil {
				return "", fmt.Errorf("write migrated planning/plan.json: %w", err)
			}
		}
		if len(result.Removals) > 0 {
			if err := writeStepConfigViaFileCallback(ctx, workspacePath, result.Configs, writeFile); err != nil {
				return "", fmt.Errorf("plan.json is migrated but writing planning/step_config.json failed: %w -- re-run migrate_declared_execution_mode; it is idempotent", err)
			}
		}
		after, _ := json.Marshal(plan)

		stepIDs := make([]string, 0, len(result.TypeChanges)+len(result.Removals))
		changes := make([]PlanFieldChange, 0, len(result.TypeChanges)+2*len(result.Removals))
		var deleted, added []json.RawMessage
		for _, change := range result.TypeChanges {
			stepIDs = append(stepIDs, change.StepID)
			changes = append(changes, PlanFieldChange{StepID: change.StepID, Field: "type", OldValue: change.From, NewValue: change.To})
			if raw, err := json.Marshal(change.before); err == nil {
				deleted = append(deleted, raw)
			}
			if raw, err := json.Marshal(change.after); err == nil {
				added = append(added, raw)
			}
		}
		for _, removal := range result.Removals {
			stepIDs = append(stepIDs, removal.StepID)
			changes = append(changes, PlanFieldChange{StepID: removal.StepID, Field: "declared_execution_mode", OldValue: removal.Mode, NewValue: ""})
			if removal.Reason != "" {
				// The reason is the one thing the field carried that the plan
				// type cannot: keep it here rather than lose it.
				changes = append(changes, PlanFieldChange{StepID: removal.StepID, Field: "declared_execution_mode_reason", OldValue: removal.Reason, NewValue: ""})
			}
		}
		toSequence, toRegular := 0, 0
		for _, change := range result.TypeChanges {
			if change.To == string(StepTypeMessageSeq) {
				toSequence++
			} else {
				toRegular++
			}
		}
		logPlanChange(ctx, workspacePath, PlanChangelogEntry{
			Tool: "migrate_declared_execution_mode",
			Reason: fmt.Sprintf(
				"Workflow contract v%s (PLAT-287): the plan type alone now states a step's execution model. %d legacy agentic regular step(s) made explicit as message_sequence, %d declared-scripted message_sequence step(s) made regular, declared_execution_mode removed from %d step_config entr(y/ies); behavior unchanged, removed reasons preserved in this entry.",
				workflowContractDeclaredExecutionModeRetiredVersionLabel, toSequence, toRegular, len(result.Removals),
			),
			StepIDs:        deduplicateStrings(stepIDs),
			Changes:        changes,
			DeletedSteps:   deleted,
			AddedSteps:     added,
			BeforeSnapshot: json.RawMessage(before),
			AfterSnapshot:  json.RawMessage(after),
		}, readFile, writeFile, logger)

		payload, _ := json.Marshal(result)
		logger.Info(fmt.Sprintf("✅ migrate_declared_execution_mode: %d type change(s), %d field removal(s)", len(result.TypeChanges), len(result.Removals)))
		return fmt.Sprintf(`{"status":"migrated","result":%s}`, payload), nil
	}
}

func deduplicateStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
