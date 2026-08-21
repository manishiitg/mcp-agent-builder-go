package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkshopExecutionNotifierSuppressesRepeatedMessageSequenceParentFailure(t *testing.T) {
	registry := NewBackgroundAgentRegistry()
	api := &StreamingAPI{bgAgentRegistry: registry}
	const (
		sessionID = "message-sequence-parent-failure"
		parentID  = "workflow-full-script"
		childID   = "msgseq-script-execute"
		rootError = `message_sequence step "shortform-script" item "execute-and-verify" reported STATUS: FAILED: sandbox unavailable`
	)
	parent := &BackgroundAgent{
		ID: parentID, Name: "Script [default]", SessionID: sessionID,
		Status: BGAgentRunning, CreatedAt: time.Now(),
	}
	child := &BackgroundAgent{
		ID: childID, ParentExecutionID: parentID, Name: "Message sequence item -> Script / execute-and-verify (user_message)",
		SessionID: sessionID, Kind: "message_sequence_item", Status: BGAgentFailed, Error: rootError, CreatedAt: time.Now(),
		Metadata: map[string]string{"execution_type": "message-sequence-item"},
	}
	registry.Register(sessionID, parent)
	registry.Register(sessionID, child)
	completionCh := registry.GetNotificationChannel(sessionID)

	notifier := &workshopExecutionBgNotifier{api: api, sessionID: sessionID}
	notifier.OnExecutionComplete(parentID, parent.Name, "", nil, fmt.Errorf("message sequence step 13 execution failed: %s", rootError))

	if got := parent.GetSnapshot().Metadata["notification_suppression"]; got != "repeated-message-sequence-child-failure" {
		t.Fatalf("notification_suppression = %q, want repeated child failure marker", got)
	}
	select {
	case got := <-completionCh:
		t.Fatalf("repeated parent failure queued a second auto-notification for %q", got)
	default:
	}
}

func TestWorkshopExecutionNotifierKeepsDistinctParentFailureNotification(t *testing.T) {
	registry := NewBackgroundAgentRegistry()
	api := &StreamingAPI{bgAgentRegistry: registry}
	const (
		sessionID = "distinct-parent-failure"
		parentID  = "workflow-full-script"
	)
	parent := &BackgroundAgent{ID: parentID, Name: "Script [default]", SessionID: sessionID, Status: BGAgentRunning, CreatedAt: time.Now()}
	child := &BackgroundAgent{
		ID: "msgseq-child", ParentExecutionID: parentID, SessionID: sessionID, Kind: "message_sequence_item",
		Status: BGAgentFailed, Error: "child failed validation", CreatedAt: time.Now(),
	}
	registry.Register(sessionID, parent)
	registry.Register(sessionID, child)
	completionCh := registry.GetNotificationChannel(sessionID)

	notifier := &workshopExecutionBgNotifier{api: api, sessionID: sessionID}
	notifier.OnExecutionComplete(parentID, parent.Name, "", nil, fmt.Errorf("parent failed while persisting the run summary"))

	select {
	case got := <-completionCh:
		if got != parentID {
			t.Fatalf("completion id = %q, want %q", got, parentID)
		}
	default:
		t.Fatal("distinct parent failure notification was suppressed")
	}
}

func TestWorkshopExecutionNotifierReportsUnexpectedContextCancelAsFailure(t *testing.T) {
	registry := NewBackgroundAgentRegistry()
	api := &StreamingAPI{
		bgAgentRegistry: registry,
	}
	const (
		sessionID = "pulse-harden-session"
		execID    = "harden-20000"
	)
	agent := &BackgroundAgent{
		ID:        execID,
		Name:      "Harden Workflow",
		SessionID: sessionID,
		Status:    BGAgentRunning,
		CreatedAt: time.Now().Add(-10 * time.Minute),
	}
	registry.Register(sessionID, agent)
	completionCh := registry.GetNotificationChannel(sessionID)

	notifier := &workshopExecutionBgNotifier{api: api, sessionID: sessionID}
	notifier.OnExecutionComplete(execID, agent.Name, "", nil, context.Canceled)

	snap := agent.GetSnapshot()
	if snap.Status != BGAgentFailed {
		t.Fatalf("status = %q, want failed", snap.Status)
	}
	if snap.Error != context.Canceled.Error() {
		t.Fatalf("error = %q, want %q", snap.Error, context.Canceled.Error())
	}
	select {
	case got := <-completionCh:
		if got != execID {
			t.Fatalf("completion id = %q, want %q", got, execID)
		}
	default:
		t.Fatal("expected failed background execution to queue a parent auto-notification")
	}
}

func TestWorkshopExecutionNotifierPreservesExplicitCancellation(t *testing.T) {
	registry := NewBackgroundAgentRegistry()
	api := &StreamingAPI{
		bgAgentRegistry: registry,
	}
	const (
		sessionID = "explicit-stop-session"
		execID    = "harden-stopped"
	)
	agent := &BackgroundAgent{
		ID:        execID,
		Name:      "Harden Workflow",
		SessionID: sessionID,
		Status:    BGAgentCanceled,
		CreatedAt: time.Now(),
	}
	registry.Register(sessionID, agent)
	completionCh := registry.GetNotificationChannel(sessionID)

	notifier := &workshopExecutionBgNotifier{api: api, sessionID: sessionID}
	notifier.OnExecutionComplete(execID, agent.Name, "", nil, context.Canceled)

	if got := agent.GetStatus(); got != BGAgentCanceled {
		t.Fatalf("status = %q, want canceled", got)
	}
	select {
	case got := <-completionCh:
		t.Fatalf("unexpected completion notification for explicitly canceled execution: %q", got)
	default:
	}
}

func TestWorkshopExecutionNotifierQueuesReviewerCompletionForParent(t *testing.T) {
	registry := NewBackgroundAgentRegistry()
	api := &StreamingAPI{bgAgentRegistry: registry}
	const (
		sessionID = "pulse-review-session"
		execID    = "pulse-review-learning-health"
	)
	agent := &BackgroundAgent{
		ID:        execID,
		Name:      "Pulse reviewer: learning health",
		SessionID: sessionID,
		Status:    BGAgentRunning,
		CreatedAt: time.Now(),
		Metadata: map[string]string{
			"execution_type":     "pulse-reviewer",
			"pulse_reviewer":     "true",
			"module":             "learning_health",
			"review_result_path": "pulse/reviews/run-1/learning_health.md",
		},
	}
	registry.Register(sessionID, agent)
	completionCh := registry.GetNotificationChannel(sessionID)

	notifier := &workshopExecutionBgNotifier{api: api, sessionID: sessionID}
	notifier.OnExecutionComplete(execID, agent.Name, "review complete", agent.Metadata, nil)

	if got := agent.GetStatus(); got != BGAgentCompleted {
		t.Fatalf("status = %q, want completed", got)
	}
	select {
	case got := <-completionCh:
		if got != execID {
			t.Fatalf("completion id = %q, want %q", got, execID)
		}
	default:
		t.Fatal("expected reviewer completion to queue a parent auto-notification")
	}
}

func TestWorkshopCompletionDisplayResultUsesPulseReviewArtifact(t *testing.T) {
	workspacePath := t.TempDir()
	resultPath := filepath.Join("pulse", "reviews", "run-1", "eval_health.md")
	if err := os.MkdirAll(filepath.Dir(filepath.Join(workspacePath, resultPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspacePath, resultPath), []byte(`# Pulse reviewer result

- Module: eval_health

## Findings

The evaluation found a stale metric source.

**Next:** correct the source and verify the next run.
`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := workshopCompletionDisplayResult(workspacePath, "review saved at "+resultPath, map[string]string{
		"execution_type":     "pulse-reviewer",
		"review_result_path": resultPath,
	})

	want := "The evaluation found a stale metric source.\n\n**Next:** correct the source and verify the next run."
	if got != want {
		t.Fatalf("display result = %q, want %q", got, want)
	}
}

func TestWorkshopCompletionDisplayResultRejectsArtifactOutsideWorkspace(t *testing.T) {
	workspacePath := t.TempDir()
	got := workshopCompletionDisplayResult(workspacePath, "review saved", map[string]string{
		"execution_type":     "pulse-reviewer",
		"review_result_path": "../outside.md",
	})
	if got != "review saved" {
		t.Fatalf("display result = %q, want fallback", got)
	}
}
