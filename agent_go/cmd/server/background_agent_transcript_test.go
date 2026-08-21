package server

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	orchEvents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
)

// TestBackgroundAgentTranscriptCreatedAtStartAndFinalizedAtCompletion pins
// PLAT-164's core lifecycle contract: emitBackgroundAgentStarted creates a
// "running" transcript before the child's first provider turn, and
// emitBackgroundAgentCompleted marks it terminal exactly once. It also
// checks the reference + status recorded into background_agent_log
// (requirements 1, 2, 4, 5).
func TestBackgroundAgentTranscriptCreatedAtStartAndFinalizedAtCompletion(t *testing.T) {
	const workspacePath = "Workflow/test-workspace"
	const sessionID = "schedule-cron--test_1786545010990380000"
	const agentID = "workshop-background-task-1787245962052297000"

	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspace := &mockWorkspaceAPI{files: map[string]string{}}
	server := httptest.NewServer(workspace)
	defer server.Close()
	t.Setenv("WORKSPACE_API_URL", server.URL)

	api := &StreamingAPI{
		activeSessions: map[string]*ActiveSessionInfo{
			sessionID: {SessionID: sessionID, WorkspacePath: workspacePath},
		},
	}

	startedAt := time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
	api.emitBackgroundAgentStarted(sessionID, agentID, "Measurement Validator", "validate last week's metrics",
		"parent-exec-1", orchEvents.ExecutionKindSubAgent)

	transcriptPath := orchEvents.BackgroundAgentTranscriptPath(workspacePath, sessionID, agentID)
	content, exists, err := readFileFromWorkspace(context.Background(), transcriptPath)
	if err != nil {
		t.Fatalf("read transcript after start: %v", err)
	}
	if !exists {
		t.Fatalf("transcript was not created at %s", transcriptPath)
	}
	transcript, err := orchEvents.ParseBackgroundAgentTranscript(content)
	if err != nil {
		t.Fatalf("parse transcript after start: %v", err)
	}
	if transcript.Status != "running" {
		t.Fatalf("status after start = %q, want running", transcript.Status)
	}
	if transcript.Name != "Measurement Validator" {
		t.Fatalf("name = %q, unexpected", transcript.Name)
	}
	if transcript.ParentExecutionID != "parent-exec-1" {
		t.Fatalf("parent_execution_id = %q, want parent-exec-1", transcript.ParentExecutionID)
	}

	entries, err := backgroundAgentLogForSession(context.Background(), workspacePath, sessionID)
	if err != nil {
		t.Fatalf("backgroundAgentLogForSession after start: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("log entries after start = %d, want 1", len(entries))
	}
	if entries[0].TranscriptPath != transcriptPath {
		t.Fatalf("transcript_path = %q, want %q", entries[0].TranscriptPath, transcriptPath)
	}
	if entries[0].TranscriptStatus != "ok" {
		t.Fatalf("transcript_status after start = %q, want ok", entries[0].TranscriptStatus)
	}

	completedAt := startedAt.Add(3 * time.Minute)
	api.emitBackgroundAgentCompleted(sessionID, agentID, "Measurement Validator", "failed", "", "provider timeout after 3 retries", "3m0s")

	content, exists, err = readFileFromWorkspace(context.Background(), transcriptPath)
	if err != nil || !exists {
		t.Fatalf("read transcript after completion: exists=%v err=%v", exists, err)
	}
	transcript, err = orchEvents.ParseBackgroundAgentTranscript(content)
	if err != nil {
		t.Fatalf("parse transcript after completion: %v", err)
	}
	if transcript.Status != "failed" {
		t.Fatalf("status after completion = %q, want failed", transcript.Status)
	}
	if transcript.Error != "provider timeout after 3 retries" {
		t.Fatalf("error = %q, unexpected", transcript.Error)
	}
	if transcript.CompletedAt.IsZero() {
		t.Fatal("completed_at was not set")
	}
	if transcript.Name != "Measurement Validator" {
		t.Fatalf("name after completion = %q, want it preserved from start", transcript.Name)
	}

	entries, err = backgroundAgentLogForSession(context.Background(), workspacePath, sessionID)
	if err != nil {
		t.Fatalf("backgroundAgentLogForSession after completion: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("log entries after completion = %d, want 1", len(entries))
	}
	if entries[0].TranscriptStatus != "ok" {
		t.Fatalf("transcript_status after completion = %q, want ok", entries[0].TranscriptStatus)
	}
	_ = completedAt
}

// TestBackgroundAgentTranscriptTerminatedPathMarksTranscriptTerminal proves
// the cancel/terminate path (which never calls emitBackgroundAgentCompleted)
// still finalizes the transcript — requirement 2 explicitly names
// cancellation as a terminal case a transcript must not be left "running" for.
func TestBackgroundAgentTranscriptTerminatedPathMarksTranscriptTerminal(t *testing.T) {
	const workspacePath = "Workflow/test-workspace"
	const sessionID = "session-1"
	const agentID = "workshop-background-task-terminated"

	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	workspace := &mockWorkspaceAPI{files: map[string]string{}}
	server := httptest.NewServer(workspace)
	defer server.Close()
	t.Setenv("WORKSPACE_API_URL", server.URL)

	api := &StreamingAPI{
		activeSessions: map[string]*ActiveSessionInfo{
			sessionID: {SessionID: sessionID, WorkspacePath: workspacePath},
		},
	}

	api.emitBackgroundAgentStarted(sessionID, agentID, "Reviewer", "", "", orchEvents.ExecutionKindSubAgent)
	api.emitBackgroundAgentTerminated(sessionID, agentID, "Reviewer", "canceled")

	transcriptPath := orchEvents.BackgroundAgentTranscriptPath(workspacePath, sessionID, agentID)
	content, exists, err := readFileFromWorkspace(context.Background(), transcriptPath)
	if err != nil || !exists {
		t.Fatalf("read transcript after terminate: exists=%v err=%v", exists, err)
	}
	transcript, err := orchEvents.ParseBackgroundAgentTranscript(content)
	if err != nil {
		t.Fatalf("parse transcript after terminate: %v", err)
	}
	if transcript.Status != "canceled" {
		t.Fatalf("status after terminate = %q, want canceled", transcript.Status)
	}
	if transcript.CompletedAt.IsZero() {
		t.Fatal("completed_at was not set on terminate")
	}
}

// TestBackgroundAgentTranscriptWriteFailureIsVisibleNotSilent pins
// requirement 5: a storage write failure must show up as a visible
// persistence failure, not silently imply a transcript exists just because
// the agent's own start/completion event was emitted successfully.
func TestBackgroundAgentTranscriptWriteFailureIsVisibleNotSilent(t *testing.T) {
	const workspacePath = "Workflow/test-workspace"
	const sessionID = "session-1"
	const agentID = "agent-1"

	t.Setenv("WORKSPACE_DOCS_PATH", t.TempDir())
	// No WORKSPACE_API_URL configured — every readFileFromWorkspace/
	// writeFileToWorkspace call fails, simulating a workspace API outage.
	t.Setenv("WORKSPACE_API_URL", "")

	api := &StreamingAPI{
		activeSessions: map[string]*ActiveSessionInfo{
			sessionID: {SessionID: sessionID, WorkspacePath: workspacePath},
		},
	}

	api.emitBackgroundAgentStarted(sessionID, agentID, "Agent", "", "", orchEvents.ExecutionKindSubAgent)

	entries, err := backgroundAgentLogForSession(context.Background(), workspacePath, sessionID)
	if err != nil {
		t.Fatalf("backgroundAgentLogForSession: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	if entries[0].TranscriptStatus == "" || entries[0].TranscriptStatus == "ok" {
		t.Fatalf("transcript_status = %q, want a visible error — a write failure must not read as success", entries[0].TranscriptStatus)
	}
}
