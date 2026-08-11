package server

import (
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
	if len(plan) != 1 || plan[0].label != "upgrade-schedule-execution-model" || plan[0].to != WorkflowContractCurrentVersion {
		t.Fatalf("unexpected upgrade plan: %+v", plan)
	}
}

func TestWorkflowVersionUpgradePlanReauditsEarlierRouteOnlyContract(t *testing.T) {
	plan := workflowVersionUpgradePlan(&WorkflowManifest{Version: workflowContractScheduledRouteVersion})
	if len(plan) != 1 || plan[0].label != "upgrade-schedule-execution-model" || plan[0].to != WorkflowContractCurrentVersion {
		t.Fatalf("1.0.24 workflow did not receive choice-aware schedule audit: %+v", plan)
	}
	for _, want := range []string{"EQUIVALENT ROUTE EXISTS", "DURABLE WORKFLOW BEHAVIOR", "GENUINELY SCHEDULE-SPECIFIC CONVERSATION", "direct_messages_reason"} {
		if !strings.Contains(plan[0].query, want) {
			t.Errorf("choice-aware migration prompt missing %q", want)
		}
	}
}
