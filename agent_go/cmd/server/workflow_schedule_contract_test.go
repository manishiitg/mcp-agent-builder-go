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
	if len(plan) != 2 || plan[0].label != "upgrade-schedule-execution-model" || plan[0].to != workflowContractScheduleExecutionModelVersion {
		t.Fatalf("unexpected upgrade plan: %+v", plan)
	}
	if plan[1].label != "upgrade-periodic-pulse-review" || plan[1].to != WorkflowContractCurrentVersion {
		t.Fatalf("unexpected final upgrade step: %+v", plan[1])
	}
}

func TestWorkflowVersionUpgradePlanReauditsEarlierRouteOnlyContract(t *testing.T) {
	plan := workflowVersionUpgradePlan(&WorkflowManifest{Version: workflowContractScheduledRouteVersion})
	if len(plan) != 2 || plan[0].label != "upgrade-schedule-execution-model" || plan[0].to != workflowContractScheduleExecutionModelVersion {
		t.Fatalf("1.0.24 workflow did not receive choice-aware schedule audit: %+v", plan)
	}
	for _, want := range []string{"EQUIVALENT ROUTE EXISTS", "DURABLE WORKFLOW BEHAVIOR", "GENUINELY SCHEDULE-SPECIFIC CONVERSATION", "direct_messages_reason"} {
		if !strings.Contains(plan[0].query, want) {
			t.Errorf("choice-aware migration prompt missing %q", want)
		}
	}
	if plan[1].label != "upgrade-periodic-pulse-review" || plan[1].to != WorkflowContractCurrentVersion {
		t.Fatalf("unexpected final upgrade step: %+v", plan[1])
	}
}

// TestUpgradePeriodicPulseReviewPromptShape pins the required content of the
// new upgrade rung: it must instruct reading real run frequency (not nominal
// cron alone), require the review schedule and the workflow-level mode
// switch to land in the same turn (never one without the other — a workflow
// left in "periodic" mode with no review schedule gets a lightweight pass
// every run and a full review from nothing), require a retention check, and
// allow "stay on per_run" as a legitimate outcome.
func TestUpgradePeriodicPulseReviewPromptShape(t *testing.T) {
	normalized := strings.Join(strings.Fields(upgradePeriodicPulseReview), " ")
	for _, want := range []string{
		"pulse_review_only=true",
		"get_schedule_runs",
		"in the SAME turn, never one without the other",
		"run_retention_count",
		"Leave post_run_monitor_mode unset",
		`set_workflow_contract_version(version="1.0.26")`,
	} {
		if !strings.Contains(normalized, want) {
			t.Errorf("periodic-pulse-review prompt missing %q", want)
		}
	}
}

// TestUpgradeQueriesNeverNamePlatTickets guards a real mistake made while
// writing upgradePeriodicPulseReview and workflow-tools.md's periodic-Pulse
// guidance: this text runs live on operators' own workflows, on their own
// machines — an internal ticket number in it is meaningless noise to them,
// not useful context. Scoped to the upgrade query constants specifically,
// not the whole package, since a Go doc comment referencing a ticket for
// engineers reading this source is a different, legitimate thing.
func TestUpgradeQueriesNeverNamePlatTickets(t *testing.T) {
	platTicket := regexp.MustCompile(`PLAT-\d+`)
	queries := map[string]string{
		"upgradeMessageSequenceCode":     upgradeMessageSequenceCode,
		"upgradeCurrentArtifactContract": upgradeCurrentArtifactContract,
		"upgradeLearningsLockAudit":      upgradeLearningsLockAudit,
		"upgradeDirectHTMLReports":       upgradeDirectHTMLReports,
		"upgradePeriodicPulseReview":     upgradePeriodicPulseReview,
	}
	for name, query := range queries {
		if match := platTicket.FindString(query); match != "" {
			t.Errorf("%s references %q — an internal ticket number in a live upgrade prompt is meaningless to the operator running it on their own machine", name, match)
		}
	}
}
