package server

import (
	"regexp"
	"strings"
	"testing"
)

func TestScheduledWorkshopMessagesPassesDeclaredRouteSelections(t *testing.T) {
	got := scheduledWorkshopMessages(&ScheduleContext{Schedule: WorkflowSchedule{
		GroupNames:      []string{"draft"},
		RouteSelections: map[string]string{"step-router": "draft-only"},
	}})
	if len(got) != 1 {
		t.Fatalf("messages = %d, want 1", len(got))
	}
	for _, want := range []string{"[\"draft\"]", `"step-router":"draft-only"`, "run_full_workflow"} {
		if !strings.Contains(got[0], want) {
			t.Fatalf("generated trigger missing %q: %s", want, got[0])
		}
	}
}

func TestValidateScheduleMessagesRequiresReasonForInlineProcedure(t *testing.T) {
	procedure := []string{"Run sqlite3 db/db.sqlite SELECT * FROM queue"}
	if err := validateScheduleMessages(procedure, ""); err == nil {
		t.Fatal("inline SQL procedure without rationale unexpectedly accepted")
	}
	if err := validateScheduleMessages(procedure, "This is a one-off schedule-specific reconciliation conversation; it is not reusable plan behavior."); err != nil {
		t.Fatalf("intentional direct procedure rejected: %v", err)
	}
	if err := validateScheduleMessages([]string{"Run the selected workflow route."}, ""); err != nil {
		t.Fatalf("compact trigger rejected: %v", err)
	}
}

func TestValidateScheduleMessagesRequiresReasonForSequentialConversation(t *testing.T) {
	messages := []string{"Collect the current evidence.", "Challenge the result and summarize it."}
	if err := validateScheduleMessages(messages, ""); err == nil {
		t.Fatal("multi-message direct conversation without rationale unexpectedly accepted")
	}
	reason := "The second turn intentionally critiques transient context from the first and is unique to this calendar event."
	if err := validateScheduleMessages(messages, reason); err != nil {
		t.Fatalf("direct conversation with rationale rejected: %v", err)
	}
	if advisory := scheduleMessagesAdvisory(messages, reason); !strings.Contains(advisory, "not canonical plan steps") {
		t.Fatalf("advisory does not disclose lifecycle tradeoff: %q", advisory)
	}
}

func TestWorkflowVersionUpgradePlanAddsScheduledRoutesAfterDirectReports(t *testing.T) {
	plan := workflowVersionUpgradePlan(&WorkflowManifest{Version: workflowContractDirectHTMLReportsVersion})
	if len(plan) != 3 || plan[0].label != "upgrade-schedule-execution-model" || plan[0].to != workflowContractScheduleExecutionModelVersion {
		t.Fatalf("unexpected upgrade plan: %+v", plan)
	}
	if plan[1].label != "upgrade-dedicated-pulse-schedule" || plan[1].to != workflowContractDedicatedPulseScheduleVersion {
		t.Fatalf("unexpected dedicated Pulse step: %+v", plan[1])
	}
	if plan[2].label != "upgrade-schedule-prompt-contract" || plan[2].to != WorkflowContractCurrentVersion {
		t.Fatalf("unexpected final upgrade step: %+v", plan[2])
	}
}

func TestWorkflowVersionUpgradePlanReauditsEarlierRouteOnlyContract(t *testing.T) {
	plan := workflowVersionUpgradePlan(&WorkflowManifest{Version: workflowContractScheduledRouteVersion})
	if len(plan) != 3 || plan[0].label != "upgrade-schedule-execution-model" || plan[0].to != workflowContractScheduleExecutionModelVersion {
		t.Fatalf("1.0.24 workflow did not receive choice-aware schedule audit: %+v", plan)
	}
	for _, want := range []string{"EQUIVALENT ROUTE EXISTS", "DURABLE WORKFLOW BEHAVIOR", "GENUINELY SCHEDULE-SPECIFIC CONVERSATION", "direct_messages_reason"} {
		if !strings.Contains(plan[0].query, want) {
			t.Errorf("choice-aware migration prompt missing %q", want)
		}
	}
	if plan[1].label != "upgrade-dedicated-pulse-schedule" || plan[1].to != workflowContractDedicatedPulseScheduleVersion {
		t.Fatalf("unexpected dedicated Pulse step: %+v", plan[1])
	}
	if plan[2].label != "upgrade-schedule-prompt-contract" || plan[2].to != WorkflowContractCurrentVersion {
		t.Fatalf("unexpected final upgrade step: %+v", plan[2])
	}
}

func TestUpgradeDedicatedPulseSchedulePromptShape(t *testing.T) {
	normalized := strings.Join(strings.Fields(upgradeDedicatedPulseSchedule), " ")
	for _, want := range []string{
		"enabled pulse_review_only schedule",
		"single source of truth",
		"Normal workflow schedules never run Gate/Review+Fix inline",
		`set_workflow_contract_version(version="1.0.27")`,
	} {
		if !strings.Contains(normalized, want) {
			t.Errorf("periodic-pulse-review handoff prompt missing %q", want)
		}
	}
	for _, mustNotContain := range []string{
		"Leave post_run_monitor_mode unset",
		"leave it on per_run",
	} {
		if strings.Contains(normalized, mustNotContain) {
			t.Errorf("periodic-pulse-review handoff prompt should not re-implement the migration Gate now owns: %q", mustNotContain)
		}
	}
}

func TestVersion126ReceivesDedicatedPulseScheduleMigration(t *testing.T) {
	plan := workflowVersionUpgradePlan(&WorkflowManifest{Version: workflowContractPeriodicPulseReviewVersion})
	if len(plan) != 2 {
		t.Fatalf("1.0.26 upgrade plan = %+v, want dedicated Pulse then schedule prompt migrations", plan)
	}
	if plan[0].label != "upgrade-dedicated-pulse-schedule" || plan[0].to != workflowContractDedicatedPulseScheduleVersion {
		t.Fatalf("1.0.26 upgrade = %+v, want dedicated Pulse schedule migration", plan[0])
	}
	if plan[1].label != "upgrade-schedule-prompt-contract" || plan[1].to != WorkflowContractCurrentVersion {
		t.Fatalf("1.0.26 final upgrade = %+v, want schedule prompt migration", plan[1])
	}
}

func TestVersion127ReceivesSchedulePromptContractMigration(t *testing.T) {
	plan := workflowVersionUpgradePlan(&WorkflowManifest{Version: workflowContractDedicatedPulseScheduleVersion})
	if len(plan) != 1 || plan[0].label != "upgrade-schedule-prompt-contract" || plan[0].to != WorkflowContractCurrentVersion {
		t.Fatalf("1.0.27 upgrade plan = %+v, want the schedule prompt migration", plan)
	}
	for _, want := range []string{
		"dated incidents",
		"Evaluation ownership must remain correct",
		"Do not weaken concrete backup behavior",
		`set_workflow_contract_version(version="1.0.28")`,
	} {
		if !strings.Contains(plan[0].query, want) {
			t.Errorf("schedule prompt migration missing %q", want)
		}
	}
}

// TestUpgradeQueriesNeverNamePlatTickets guards a real mistake made while
// writing upgradeDedicatedPulseSchedule and workflow-tools.md's
// periodic-Pulse guidance: this text runs live on operators' own workflows,
// on their own machines — an internal ticket number in it is meaningless
// noise to them, not useful context. Scoped to the upgrade query constants
// specifically, not the whole package, since a Go doc comment referencing a
// ticket for engineers reading this source is a different, legitimate thing.
func TestUpgradeQueriesNeverNamePlatTickets(t *testing.T) {
	platTicket := regexp.MustCompile(`PLAT-\d+`)
	queries := map[string]string{
		"upgradeMessageSequenceCode":     upgradeMessageSequenceCode,
		"upgradeCurrentArtifactContract": upgradeCurrentArtifactContract,
		"upgradeLearningsLockAudit":      upgradeLearningsLockAudit,
		"upgradeDirectHTMLReports":       upgradeDirectHTMLReports,
		"upgradeDedicatedPulseSchedule":  upgradeDedicatedPulseSchedule,
		"upgradeSchedulePromptContract":  upgradeSchedulePromptContract,
	}
	for name, query := range queries {
		if match := platTicket.FindString(query); match != "" {
			t.Errorf("%s references %q — an internal ticket number in a live upgrade prompt is meaningless to the operator running it on their own machine", name, match)
		}
	}
}
