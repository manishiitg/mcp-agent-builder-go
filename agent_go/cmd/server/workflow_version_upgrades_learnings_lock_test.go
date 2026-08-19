package server

import (
	"strings"
	"testing"
)

// PLAT-055 / J. The learnings-lock audit remains its own 1.0.22 boundary even
// after later contract upgrades are added.
// the learnings-lock audit as a mandatory final preflight. The regression this
// guards against is real: workflowContractArtifactPurityVersion ("1.0.21")
// sits at rank 20 in the known-version list, one below the new audit's rank
// 21 — an off-by-one there would silently re-run the already-completed 1.0.21
// purification turn for every workflow that had already passed it.

func TestWorkflowVersionUpgradePlanSkipsArtifactPurityAlreadyReached(t *testing.T) {
	plan := workflowVersionUpgradePlan(&WorkflowManifest{Version: "1.0.21"})
	if len(plan) != 4 {
		t.Fatalf("plan from 1.0.21 = %d steps, want audit, direct-report, scheduled-route, then periodic-pulse-review migration: %+v", len(plan), plan)
	}
	if plan[0].label != "upgrade-learnings-lock-audit" {
		t.Fatalf("plan[0].label = %q, want upgrade-learnings-lock-audit", plan[0].label)
	}
	if plan[0].to != workflowContractLearningsLockAuditVersion {
		t.Fatalf("plan[0].to = %q, want %q", plan[0].to, workflowContractLearningsLockAuditVersion)
	}
	if plan[1].label != "upgrade-direct-html-reports" || plan[1].to != workflowContractDirectHTMLReportsVersion {
		t.Fatalf("plan[1] = %+v, want direct-report migration", plan[1])
	}
	if plan[2].label != "upgrade-schedule-execution-model" || plan[2].to != workflowContractScheduleExecutionModelVersion {
		t.Fatalf("plan[2] = %+v, want scheduled-route migration to %s", plan[2], workflowContractScheduleExecutionModelVersion)
	}
	if plan[3].label != "upgrade-dedicated-pulse-schedule" || plan[3].to != WorkflowContractCurrentVersion {
		t.Fatalf("plan[3] = %+v, want periodic-pulse-review migration reaching current version", plan[3])
	}
	for _, label := range []string{"upgrade-current-artifact-contract"} {
		for _, step := range plan {
			if step.label == label {
				t.Fatalf("a workflow already at 1.0.21 was asked to repeat %q", label)
			}
		}
	}
}

func TestWorkflowVersionUpgradePlanOlderWorkflowGetsBothFinalSteps(t *testing.T) {
	plan := workflowVersionUpgradePlan(&WorkflowManifest{Version: "1.0.20"})
	if len(plan) != 5 {
		t.Fatalf("plan from 1.0.20 = %d steps, want artifact-purity, audit, report, scheduled-route, then periodic-pulse-review migration: %+v", len(plan), plan)
	}
	if plan[0].label != "upgrade-current-artifact-contract" || plan[0].to != workflowContractArtifactPurityVersion {
		t.Fatalf("plan[0] = %+v, want the 1.0.21 purification step first", plan[0])
	}
	if plan[1].label != "upgrade-learnings-lock-audit" || plan[1].to != workflowContractLearningsLockAuditVersion {
		t.Fatalf("plan[1] = %+v, want the learnings-lock audit reaching 1.0.22", plan[1])
	}
	if plan[2].label != "upgrade-direct-html-reports" || plan[2].to != workflowContractDirectHTMLReportsVersion {
		t.Fatalf("plan[2] = %+v, want direct-report migration", plan[2])
	}
	if plan[3].label != "upgrade-schedule-execution-model" || plan[3].to != workflowContractScheduleExecutionModelVersion {
		t.Fatalf("plan[3] = %+v, want scheduled-route migration to %s", plan[3], workflowContractScheduleExecutionModelVersion)
	}
	if plan[4].label != "upgrade-dedicated-pulse-schedule" || plan[4].to != WorkflowContractCurrentVersion {
		t.Fatalf("plan[4] = %+v, want periodic-pulse-review migration reaching current version", plan[4])
	}
}

func TestWorkflowVersionUpgradePlanNoStepsAtCurrentVersion(t *testing.T) {
	plan := workflowVersionUpgradePlan(&WorkflowManifest{Version: WorkflowContractCurrentVersion})
	if plan != nil {
		t.Fatalf("plan at current version = %+v, want nil", plan)
	}
}

func TestUpgradeLearningsLockAuditPromptShape(t *testing.T) {
	// The prompt must not resurrect the routing-quality check Phase 2 made
	// structural, must report rather than unlock, and must name the concrete
	// tool + boundary an implementer would otherwise have to guess.
	for _, want := range []string{
		"lock_learnings=true",
		"record_pulse_finding",
		"decision_required",
		"Do not call update_step_config to clear lock_learnings",
		`set_workflow_contract_version(version="1.0.22")`,
	} {
		if !strings.Contains(upgradeLearningsLockAudit, want) {
			t.Errorf("audit prompt missing %q", want)
		}
	}
	// The reason routing/file-naming checks are gone must be stated, not just
	// silently absent, or a future edit could plausibly re-add them.
	if !strings.Contains(upgradeLearningsLockAudit, "obsolete") {
		t.Error("audit prompt does not explain why the routing check was dropped")
	}
}
