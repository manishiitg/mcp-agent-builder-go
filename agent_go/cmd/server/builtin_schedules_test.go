package server

import (
	"testing"
)

// Org Pulse itself is gone (DefaultBuiltinSchedules returns empty -- see
// builtin_schedules.go). What's left to test is the degradation path: an
// existing user whose _users/<id>/multiagent-schedules.json still has a
// builtin-org-pulse override (or a user-created duplicate) from before this
// removal must not error or panic -- NormalizeOrgPulseSchedule's
// FindDefaultBuiltinSchedule lookup now fails (ok=false), so it passes the
// stale entry through completely unchanged rather than refreshing it to
// content that no longer exists.

func TestMergeBuiltinSchedulesPassesThroughStaleOrgPulseOverrideUnchanged(t *testing.T) {
	resume := true
	stale := WorkflowSchedule{
		ID:             builtinOrgPulseID,
		Name:           "Custom Org Pulse",
		Description:    "User cadence override for org pulse",
		ScheduleType:   "calendar",
		CronExpression: "15 9 * * *",
		Timezone:       "Asia/Kolkata",
		Enabled:        true,
		Mode:           "multi-agent",
		Query:          "old org-pulse query",
		Messages:       []string{"old step"},
		ResumePrevious: &resume,
		CalendarItems: []CalendarScheduleItem{
			{ID: "one", Date: "2026-07-01", Time: "09:00", Messages: []string{"old item step"}},
		},
	}

	merged := MergeBuiltinSchedules([]WorkflowSchedule{stale})
	var got *WorkflowSchedule
	for i := range merged {
		if merged[i].ID == builtinOrgPulseID {
			got = &merged[i]
			break
		}
	}
	if got == nil {
		t.Fatal("merged schedules missing the stale org pulse override -- it should still pass through, not disappear")
	}
	if got.Query != stale.Query || len(got.Messages) != 1 || got.Messages[0] != stale.Messages[0] {
		t.Fatalf("stale org pulse content should pass through unchanged now that there is no builtin to refresh it against: query=%q messages=%v", got.Query, got.Messages)
	}
	if len(got.CalendarItems) != 1 || len(got.CalendarItems[0].Messages) != 1 {
		t.Fatalf("calendar item messages should also pass through unchanged: %#v", got.CalendarItems)
	}
}

func TestMergeBuiltinSchedulesPassesThroughStaleDuplicateOrgPulseUnchanged(t *testing.T) {
	duplicate := WorkflowSchedule{
		ID:             "user-created-org-pulse",
		Name:           "Org Pulse",
		Description:    "Daily org-pulse duplicate",
		CronExpression: "30 7 * * *",
		Timezone:       "UTC",
		Enabled:        true,
		Mode:           "multi-agent",
		Query:          "legacy org pulse",
		Messages:       []string{"old step"},
	}

	merged := MergeBuiltinSchedules([]WorkflowSchedule{duplicate})
	var got *WorkflowSchedule
	for i := range merged {
		if merged[i].ID == duplicate.ID {
			got = &merged[i]
			break
		}
	}
	if got == nil {
		t.Fatal("merged schedules missing the stale duplicate org pulse")
	}
	if got.Query != duplicate.Query || len(got.Messages) != 1 || got.Messages[0] != duplicate.Messages[0] {
		t.Fatalf("stale duplicate org pulse content should pass through unchanged: query=%q messages=%v", got.Query, got.Messages)
	}
}

func TestIsOrgPulseScheduleDoesNotMatchIncidentalText(t *testing.T) {
	schedule := WorkflowSchedule{
		ID:          "publish-org-report",
		Name:        "Publish reporting dashboard",
		Description: "Post the org-pulse.html summary after publishing.",
		Query:       "Read the Org Pulse output and publish it.",
		Mode:        "multi-agent",
	}
	if IsOrgPulseSchedule(schedule) {
		t.Fatal("schedule with incidental Org Pulse text was classified as the managed Org Pulse")
	}
}
