package step_based_workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
)

// PlanDriftCandidate is one step with no drift_review record at all, or one
// whose record has needs_review==true (flagged stale by a dependency-
// triggering plan edit since its last completed review), together with the
// deterministic Group 1/2 checks Go could run for it without an agent turn.
//
// This is deliberately the same "computed on read" shape as
// CollectPlanChangeBacklog: nothing is persisted, so a candidate can never be
// lost by anything failing to record, and the list always reflects the
// current step_config.json.
type PlanDriftCandidate struct {
	StepID string           `json:"step_id"`
	Checks []StepDriftCheck `json:"checks"`
}

// planDriftPlainFileReader adapts the workspace-relative paths that
// normalizePathForWorkspaceAPI produces to a plain filesystem read, for
// callers outside a tool-call context (no readFile callback available). This
// is read-only evidence gathering, never a mutation path.
func planDriftPlainFileReader(_ context.Context, workspaceRelativePath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(workspaceRelativePath)))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// planStepIDsFromPlanJSON returns every step id declared in plan.json's
// top-level "steps" array, recursing into routing/branch sub-agent steps the
// same way collectRawPlanStepIDs already does for the message-sequence code
// migration. Deliberately excludes "orphan_steps" — those are detached,
// non-executing steps, not live plan surface a drift review needs to cover.
func planStepIDsFromPlanJSON(planContent string) (map[string]bool, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal([]byte(planContent), &document); err != nil {
		return nil, fmt.Errorf("parse planning/plan.json: %w", err)
	}
	var rawSteps []json.RawMessage
	if err := json.Unmarshal(document["steps"], &rawSteps); err != nil {
		return nil, fmt.Errorf("parse planning/plan.json steps: %w", err)
	}
	ids := map[string]bool{}
	for _, raw := range rawSteps {
		collectRawPlanStepIDs(raw, ids)
	}
	return ids, nil
}

// CollectPlanDriftCandidates scans planning/plan.json for the plan's real
// step set, left-joins it against planning/step_config.json's drift_review
// records, and runs the deterministic checks Go can run for each step still
// pending review — so a plan_drift_review reviewer turn starts from
// pre-computed evidence instead of re-deriving it via tool calls.
//
// A step with no step_config.json row at all is exactly as pending as one
// with a row but a flagged (needs_review==true) or missing drift_review: the
// invariant is "has this step been reviewed since it last existed in this
// configured form," and a missing config row has never been reviewed.
// Deriving the step set from step_config.json alone (an earlier version of
// this function did) silently drops any step that never got a config row
// written for an unrelated reason — plan.json is the source of truth for
// which steps exist.
//
// Returns a non-nil error when plan.json or step_config.json exists but
// cannot be read or parsed — the caller must treat that as evidence
// unavailable, never as "nothing is due." A missing plan.json (no plan
// authored yet) or a missing step_config.json (no step has ever been
// configured) are not errors: both legitimately mean "review step_config.json
// coverage from scratch," which the join already handles as 100% pending.
//
// Coverage is intentionally partial for this first pass: Check 2
// (validation_schema DB rules) only covers a step_config.json override, not a
// plan.json-declared schema with no override present (step_config.json's own
// contract is that its validation_schema takes precedence over plan.json's,
// so a step relying purely on the plan.json-declared schema is silently out
// of scope here). Checks 5 (validation_schema file rules, needs the step's
// most recent real run output — no run-folder resolver exists yet) and 13
// (orphaned tables, needs every step's SQL references aggregated across the
// whole plan) are deliberately omitted; the reviewer turn checks both
// directly, the same way it does the Group 3 judgment checks.
func CollectPlanDriftCandidates(ctx context.Context, workspacePath string) ([]PlanDriftCandidate, error) {
	workspacePath = strings.Trim(strings.TrimSpace(workspacePath), "/")
	if workspacePath == "" {
		return nil, nil
	}

	planPath := filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(workspacePath), PlanningFolderName, "plan.json")
	planRaw, err := os.ReadFile(planPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read planning/plan.json: %w", err)
	}
	stepIDs, err := planStepIDsFromPlanJSON(string(planRaw))
	if err != nil {
		return nil, err
	}
	if len(stepIDs) == 0 {
		return nil, nil
	}

	configPath := filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(workspacePath), PlanningFolderName, "step_config.json")
	byID := map[string]StepConfig{}
	configRaw, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("read planning/step_config.json: %w", err)
		}
		// No step_config.json at all: every plan step is pending, byID stays empty.
	} else {
		configs, parseErr := ParseStepConfigContent(string(configRaw))
		if parseErr != nil {
			return nil, fmt.Errorf("parse planning/step_config.json: %w", parseErr)
		}
		for _, cfg := range configs {
			byID[cfg.ID] = cfg
		}
	}

	var pendingStepIDs []string
	for id := range stepIDs {
		cfg, ok := byID[id]
		if !ok || cfg.AgentConfigs == nil || cfg.AgentConfigs.DriftReview == nil || cfg.AgentConfigs.DriftReview.NeedsReview {
			pendingStepIDs = append(pendingStepIDs, id)
		}
	}
	if len(pendingStepIDs) == 0 {
		return nil, nil
	}
	sort.Strings(pendingStepIDs)

	// Workflow-wide checks run once and are attached to every candidate step:
	// a report query or db/README contract break isn't owned by one step, but
	// every step with an unreviewed record should see it as evidence.
	reportCheck, _ := CheckReportQueryCompatibility(ctx, workspacePath, planDriftPlainFileReader)
	readmeCheck, _ := CheckDBReadmeContract(ctx, workspacePath, planDriftPlainFileReader)

	candidates := make([]PlanDriftCandidate, 0, len(pendingStepIDs))
	for _, stepID := range pendingStepIDs {
		checks := []StepDriftCheck{reportCheck, readmeCheck}

		scriptedCheck, _ := CheckScriptedCodeDBQueries(ctx, workspacePath, stepID, planDriftPlainFileReader)
		checks = append(checks, scriptedCheck)

		if cfg, ok := byID[stepID]; ok && cfg.ValidationSchema != nil && len(cfg.ValidationSchema.DB) > 0 {
			dbCheck, _ := CheckValidationSchemaDBRules(ctx, workspacePath, cfg.ValidationSchema.DB)
			checks = append(checks, dbCheck)
		}

		candidates = append(candidates, PlanDriftCandidate{StepID: stepID, Checks: checks})
	}
	return candidates, nil
}
