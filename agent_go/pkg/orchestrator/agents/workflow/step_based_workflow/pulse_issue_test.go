package step_based_workflow

import (
	"strings"
	"testing"
)

func TestNewPulseIssueKeepsTechnicalLifecycleOutOfCoreIssue(t *testing.T) {
	finding := PulseFindingLifecycle{
		Fingerprint: "0123456789abcdef",
		Module:      "bug_review",
		Text:        "The selector keeps targeting the same accounts instead of finding new ones.",
		Status:      ConcernStatusOpen,
		FirstSeenAt: "2026-07-30T10:00:00Z",
		LastSeenAt:  "2026-07-31T10:00:00Z",
		SeenCount:   3,
	}

	issue := NewPulseIssue(finding)
	if issue.ID != "PUL-01234567" || strings.Contains(issue.ID, finding.Fingerprint) {
		t.Fatalf("human issue id = %q, want short stable address", issue.ID)
	}
	if issue.Title != finding.Text || issue.Description != "" {
		t.Fatalf("unexpected title/description: %+v", issue)
	}
	if issue.Status != "backlog" || issue.Priority != "none" || issue.SeenCount != 3 {
		t.Fatalf("unexpected issue projection: %+v", issue)
	}
}

func TestNewPulseIssueMapsStructuredSummaryPriorityAndStatus(t *testing.T) {
	finding := PulseFindingLifecycle{
		Fingerprint: "fedcba9876543210",
		FindingID:   "HARNESS-17",
		Text:        "The scheduler records success before the final agent message completes.",
		Status:      ConcernStatusAcknowledged,
		SeenCount:   2,
		Details: &PulseFindingDetails{
			Severity: "critical",
			Summary:  "Scheduler marks incomplete runs successful",
			Impact:   "Downstream reviews trust a truncated result.",
		},
		Events: []PulseFindingEvent{{EventType: FindingDispositionBlocked}},
	}

	issue := NewPulseIssue(finding)
	if issue.ID != "HARNESS-17" || issue.Status != "blocked" || issue.Priority != "urgent" {
		t.Fatalf("unexpected identity/status/priority: %+v", issue)
	}
	if issue.Title != "Scheduler marks incomplete runs successful" || !strings.Contains(issue.Description, "Impact:") {
		t.Fatalf("unexpected content: %+v", issue)
	}
}

func TestNewPulseIssueMapsFixLifecycleToWorkflowStatuses(t *testing.T) {
	tests := map[string]string{
		ConcernStatusFixing:                 "in_progress",
		ConcernStatusAwaitingVerification:   "in_review",
		ConcernStatusQueuedForEngineering:   "backlog",
		ConcernStatusResolved:               "done",
		ConcernStatusRejected:               "canceled",
		ConcernStatusExternalActionRequired: "external",
	}
	for findingStatus, issueStatus := range tests {
		t.Run(findingStatus, func(t *testing.T) {
			issue := NewPulseIssue(PulseFindingLifecycle{Fingerprint: "abcd", Text: "Issue", Status: findingStatus})
			if issue.Status != issueStatus {
				t.Fatalf("status = %q, want %q", issue.Status, issueStatus)
			}
		})
	}
}
