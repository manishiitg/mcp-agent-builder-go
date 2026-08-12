package server

import (
	"strings"
	"testing"
)

// confida-login sat at 1.0.20 indefinitely: the 1.0.21 turn found 19 finding
// IDs in builder/improve-archive absent from the pulse_* tables, correctly
// declined to delete them, and every later trigger re-derived the same blocker
// while the QA work never ran. A recorded owner decision is the way out.

func TestUpgradeQueryCarriesTheOperatorDecisionForItsOwnTarget(t *testing.T) {
	manifest := &WorkflowManifest{
		Version: "1.0.20",
		ContractUpgradeDecisions: map[string]WorkflowContractUpgradeDecision{
			workflowContractArtifactPurityVersion: {
				Decision:  "The pre-2026-07-22 finding history in builder/improve-archive is not worth keeping. Delete it.",
				DecidedAt: "2026-08-12",
			},
		},
	}
	plan := workflowVersionUpgradePlan(manifest)
	if len(plan) == 0 {
		t.Fatal("no upgrade plan for a 1.0.20 workflow")
	}

	var artifactStep, auditStep string
	for _, step := range plan {
		switch step.label {
		case "upgrade-current-artifact-contract":
			artifactStep = step.query
		case "upgrade-learnings-lock-audit":
			auditStep = step.query
		}
	}
	if artifactStep == "" || auditStep == "" {
		t.Fatalf("plan is missing the steps under test: %+v", plan)
	}

	for _, want := range []string{
		"OPERATOR DECISION ON THIS UPGRADE (recorded 2026-08-12)",
		"not worth keeping",
		// The decision authorizes one blocker, not the migration generally.
		"nothing wider",
		"do not read this as general permission to proceed",
	} {
		if !strings.Contains(artifactStep, want) {
			t.Errorf("1.0.21 upgrade query missing %q", want)
		}
	}

	// A decision is scoped to the version it was recorded against. Leaking it
	// into the neighboring rung would authorize a migration nobody answered for.
	if strings.Contains(auditStep, "OPERATOR DECISION") {
		t.Errorf("the 1.0.21 decision leaked into the 1.0.22 upgrade query:\n%s", auditStep)
	}
}

func TestUpgradeQueryIsUnchangedWithoutADecision(t *testing.T) {
	without := workflowVersionUpgradePlan(&WorkflowManifest{Version: "1.0.20"})
	if len(without) == 0 {
		t.Fatal("no upgrade plan for a 1.0.20 workflow")
	}
	for _, step := range without {
		if strings.Contains(step.query, "OPERATOR DECISION") {
			t.Fatalf("%s carries a decision note with no decision recorded", step.label)
		}
	}
	if got := workflowContractUpgradeDecisionNote(nil, "1.0.21"); got != "" {
		t.Errorf("nil manifest note = %q, want empty", got)
	}
	blank := &WorkflowManifest{ContractUpgradeDecisions: map[string]WorkflowContractUpgradeDecision{"1.0.21": {Decision: "   "}}}
	if got := workflowContractUpgradeDecisionNote(blank, "1.0.21"); got != "" {
		t.Errorf("blank decision note = %q, want empty", got)
	}
}

func TestDecisionNoteOmitsTheTimestampWhenAbsent(t *testing.T) {
	manifest := &WorkflowManifest{
		ContractUpgradeDecisions: map[string]WorkflowContractUpgradeDecision{"1.0.21": {Decision: "Discard it."}},
	}
	note := workflowContractUpgradeDecisionNote(manifest, "1.0.21")
	if !strings.Contains(note, "OPERATOR DECISION ON THIS UPGRADE.") {
		t.Errorf("note should read cleanly with no recorded date: %s", note)
	}
	if strings.Contains(note, "recorded ") {
		t.Errorf("note invented a recorded date: %s", note)
	}
}

// The blocker wording is what makes a stalled turn diagnosable instead of
// looking like caution, and what tells it not to decide the owner's question.
func TestArtifactContractUpgradeStatesTheArchiveBlockerExplicitly(t *testing.T) {
	for _, want := range []string{
		"the SQLite pulse_* tables do not",
		"that is a blocker, not a judgement call for this turn",
		"Name the specific records",
		"workflow owner's decision",
	} {
		if !strings.Contains(upgradeCurrentArtifactContract, want) {
			t.Errorf("1.0.21 upgrade prompt missing %q", want)
		}
	}
}
