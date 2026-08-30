package step_based_workflow

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/fsutil"
)

// PlanDriftCandidate is one step whose drift_review record is null (never
// reviewed since its last dependency-triggering plan edit), together with the
// deterministic Group 1/2 checks Go could run for it without an agent turn.
//
// This is deliberately the same "computed on read" shape as
// CollectPlanChangeBacklog: nothing is persisted, so a candidate can never be
// lost by anything failing to record, and the list always reflects the
// current step_config.json.
type PlanDriftCandidate struct {
	StepID string `json:"step_id"`
	// StepType is the plan.json step type ("routing", "branch", "regular",
	// ...), precomputed so the reviewer turn can tell which candidates are
	// routing steps (the "route"/major-fork concept, PLAT-259) without an
	// extra lookup -- routing steps get two additional judgment checks
	// (route_structural_isolation, route_eval_pairing) that branch and every
	// other step type do not. Empty if plan.json could not be read/parsed.
	StepType string           `json:"step_type,omitempty"`
	Checks   []StepDriftCheck `json:"checks"`
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

// CollectPlanDriftCandidates scans step_config.json for steps with a null
// drift_review record and runs the deterministic checks Go can run for each,
// so a plan_drift_review reviewer turn starts from pre-computed evidence
// instead of re-deriving it via tool calls. Returns nil when nothing is
// outstanding.
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
func CollectPlanDriftCandidates(ctx context.Context, workspacePath string) []PlanDriftCandidate {
	workspacePath = strings.Trim(strings.TrimSpace(workspacePath), "/")
	if workspacePath == "" {
		return nil
	}
	configPath := filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(workspacePath), PlanningFolderName, "step_config.json")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		return nil
	}
	configs, err := ParseStepConfigContent(string(raw))
	if err != nil {
		return nil
	}

	var pendingStepIDs []string
	byID := map[string]StepConfig{}
	for _, cfg := range configs {
		byID[cfg.ID] = cfg
		if cfg.AgentConfigs == nil || cfg.AgentConfigs.DriftReview == nil {
			pendingStepIDs = append(pendingStepIDs, cfg.ID)
		}
	}
	if len(pendingStepIDs) == 0 {
		return nil
	}

	// Precompute each candidate's plan.json step type -- lets the reviewer
	// turn tell routing steps (the "route"/major-fork concept) apart from
	// branch and everything else without an extra lookup. Best-effort: a
	// missing/unparseable plan.json just leaves StepType empty, it does not
	// fail the whole scan (the same tolerance readFile failures elsewhere in
	// this function already get).
	stepTypeByID := map[string]string{}
	planPath := filepath.Join(fsutil.WorkspaceDocsRoot(), filepath.FromSlash(workspacePath), PlanningFolderName, "plan.json")
	if planRaw, err := os.ReadFile(planPath); err == nil {
		var plan PlanningResponse
		if err := json.Unmarshal(planRaw, &plan); err == nil {
			for _, step := range plan.Steps {
				stepTypeByID[step.GetID()] = string(step.StepType())
			}
		}
	}

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

		candidates = append(candidates, PlanDriftCandidate{StepID: stepID, StepType: stepTypeByID[stepID], Checks: checks})
	}
	return candidates
}
