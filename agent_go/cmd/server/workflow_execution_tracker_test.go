package server

import (
	"testing"
	"time"
)

func TestRunningWorkflowListIncludesWorkflowBuilderTask(t *testing.T) {
	startedAt := time.Now().UTC()
	api := &StreamingAPI{
		trackedWorkflowExecutions: map[string]*TrackedWorkflowExecution{
			"builder-1": {
				ExecutionID:   "builder-1",
				SessionID:     "session-builder",
				Source:        trackedExecutionSourceWorkshopBackground,
				Kind:          "workflow_builder_task",
				Name:          "Review plan drift",
				Title:         "Review plan drift",
				PresetQueryID: "preset-1",
				WorkspacePath: "Workflow/rts-video",
				PhaseID:       "workflow-builder",
				PhaseName:     "Workflow Builder",
				Status:        trackedExecutionStatusRunning,
				UserID:        "user-1",
				StartedAt:     startedAt,
			},
		},
	}

	running := api.listRunningWorkflowExecutions("user-1")
	if len(running) != 1 {
		t.Fatalf("running len = %d, want 1", len(running))
	}
	if running[0].SessionID != "session-builder" {
		t.Fatalf("session_id = %q, want session-builder", running[0].SessionID)
	}
	if running[0].Kind != "workflow_builder_task" {
		t.Fatalf("kind = %q, want workflow_builder_task", running[0].Kind)
	}

	api.trackedWorkflowExecutionsMux.RLock()
	found := api.runningWorkflowListExecutionBySessionLocked("session-builder")
	api.trackedWorkflowExecutionsMux.RUnlock()
	if found == nil {
		t.Fatal("runningWorkflowListExecutionBySessionLocked did not find builder task")
	}
}

func TestRunningWorkflowListKeepsInternalWorkflowStepsOut(t *testing.T) {
	api := &StreamingAPI{
		trackedWorkflowExecutions: map[string]*TrackedWorkflowExecution{
			"step-1": {
				ExecutionID:   "step-1",
				SessionID:     "session-builder",
				Source:        trackedExecutionSourceWorkshopBackground,
				Kind:          "workflow_step",
				Name:          "Step -> collect data",
				WorkspacePath: "Workflow/rts-video",
				PhaseID:       "workflow-builder",
				Status:        trackedExecutionStatusRunning,
				UserID:        "user-1",
				StartedAt:     time.Now().UTC(),
			},
		},
	}

	running := api.listRunningWorkflowExecutions("user-1")
	if len(running) != 0 {
		t.Fatalf("running len = %d, want 0 for internal workflow step", len(running))
	}
}

func TestFindRunningTrackedExecutionForWorkspaceWhereDoesNotLetNewerScheduleHideBuilder(t *testing.T) {
	now := time.Now().UTC()
	api := &StreamingAPI{
		trackedWorkflowExecutions: map[string]*TrackedWorkflowExecution{
			"builder": {
				ExecutionID:   "builder",
				SessionID:     "chat-session",
				WorkspacePath: "Workflow/demo",
				PhaseID:       "workflow-builder",
				Status:        trackedExecutionStatusRunning,
				StartedAt:     now.Add(-time.Minute),
			},
			"schedule": {
				ExecutionID:   "schedule",
				SessionID:     "schedule-cron--demo_1",
				WorkspacePath: "Workflow/demo",
				PhaseID:       "execute-workflow",
				Status:        trackedExecutionStatusRunning,
				TriggeredBy:   "cron",
				StartedAt:     now,
			},
		},
	}

	found := api.findRunningTrackedExecutionForWorkspaceWhere("Workflow/demo", func(exec *TrackedWorkflowExecution) bool {
		return trackedExecutionBlocksNewWorkflowBuilderChat(exec)
	})
	if found == nil || found.SessionID != "chat-session" {
		t.Fatalf("builder lookup = %#v, want chat-session", found)
	}
}

func TestScheduledWorkflowBuilderPhaseDoesNotBlockInteractiveBuilderChat(t *testing.T) {
	now := time.Now().UTC()
	api := &StreamingAPI{
		trackedWorkflowExecutions: map[string]*TrackedWorkflowExecution{
			"message-sequence": {
				ExecutionID:   "msgseq-execute-allocate-execute-and-verify-1",
				SessionID:     "schedule-cron--5227790a_1784694634941870000",
				Source:        trackedExecutionSourceWorkshopBackground,
				Kind:          "workflow_builder_task",
				Name:          "Message sequence item -> [Execute] Allocator",
				Title:         "Message sequence item -> [Execute] Allocator",
				WorkspacePath: "Workflow/social-media",
				PhaseID:       "workflow-builder",
				PhaseName:     "Workflow Builder",
				Status:        trackedExecutionStatusRunning,
				TriggeredBy:   "workflow_builder",
				StartedAt:     now,
			},
		},
	}

	found := api.findRunningTrackedExecutionForWorkspaceWhere("Workflow/social-media", func(exec *TrackedWorkflowExecution) bool {
		return trackedExecutionBlocksNewWorkflowBuilderChat(exec)
	})
	if found != nil {
		t.Fatalf("builder lookup = %#v, want nil for scheduled message-sequence work", found)
	}
}

func TestInteractiveWorkflowBuilderTaskBlocksNewBuilderChat(t *testing.T) {
	now := time.Now().UTC()
	exec := &TrackedWorkflowExecution{
		ExecutionID:   "builder-chat-1",
		SessionID:     "builder-chat-session",
		Source:        trackedExecutionSourceWorkshopBackground,
		Kind:          "workflow_builder_task",
		WorkspacePath: "Workflow/social-media",
		PhaseID:       "workflow-builder",
		Status:        trackedExecutionStatusRunning,
		TriggeredBy:   "workflow_builder",
		StartedAt:     now,
	}

	if !trackedExecutionBlocksNewWorkflowBuilderChat(exec) {
		t.Fatal("interactive workflow-builder task should block a second builder chat")
	}
}

// The chat/schedule pairing is symmetric, and only one half was implemented.
// trackedExecutionBlocksNewWorkflowBuilderChat already lets a user open a chat
// while scheduled work runs; the reverse — a schedule firing while a user has a
// chat open — was rejected with 409 workflow_busy in a few milliseconds, telling
// a cron job to "stop the running chat", which it cannot do.
func TestScheduledRequestBypassesWorkflowBusy(t *testing.T) {
	for name, tc := range map[string]struct {
		sessionID   string
		triggeredBy string
		want        bool
	}{
		"manual schedule trigger": {sessionID: "schedule-manual--44163d28_1785290407859979000", want: true},
		"cron schedule session":   {sessionID: "schedule-cron--8cfba228_1784606404126176000", want: true},
		"cron trigger field":      {sessionID: "b3c7f380-f405-4661-86d4-a9f984a151ce", triggeredBy: "cron", want: true},
		"scheduler trigger field": {sessionID: "b3c7f380-f405-4661-86d4-a9f984a151ce", triggeredBy: "scheduled", want: true},
		"ordinary user chat":      {sessionID: "d0afa8ad-5af3-4a3d-aeea-a6dfa7646975", triggeredBy: "workflow_builder", want: false},
		"bare uuid session":       {sessionID: "b3c7f380-f405-4661-86d4-a9f984a151ce", want: false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := scheduledRequestBypassesWorkflowBusy(tc.sessionID, tc.triggeredBy); got != tc.want {
				t.Fatalf("scheduledRequestBypassesWorkflowBusy(%q, %q) = %v, want %v",
					tc.sessionID, tc.triggeredBy, got, tc.want)
			}
		})
	}
}

// Both directions of the same rule, stated together so neither half can be
// removed without the other's intent being visible.
func TestChatAndScheduleDoNotBlockEachOther(t *testing.T) {
	userChat := &TrackedWorkflowExecution{
		Status:      trackedExecutionStatusRunning,
		PhaseID:     "workflow-builder",
		SessionID:   "d0afa8ad-5af3-4a3d-aeea-a6dfa7646975",
		TriggeredBy: "workflow_builder",
	}
	scheduled := &TrackedWorkflowExecution{
		Status:    trackedExecutionStatusRunning,
		PhaseID:   "workflow-builder",
		SessionID: "schedule-manual--44163d28_1785290407859979000",
	}

	// A running schedule must not block a user opening a chat.
	if trackedExecutionBlocksNewWorkflowBuilderChat(scheduled) {
		t.Fatal("scheduled work must not block a user's builder chat")
	}
	// A running user chat must not block a schedule firing.
	if !scheduledRequestBypassesWorkflowBusy(scheduled.SessionID, scheduled.TriggeredBy) {
		t.Fatal("a schedule must not be refused because a user chat is open")
	}
	// A second interactive chat is still refused — that guard is the point.
	if !trackedExecutionBlocksNewWorkflowBuilderChat(userChat) {
		t.Fatal("a running user chat must still block another interactive chat")
	}
	if scheduledRequestBypassesWorkflowBusy(userChat.SessionID, userChat.TriggeredBy) {
		t.Fatal("an ordinary chat must not claim the scheduled bypass")
	}
}
