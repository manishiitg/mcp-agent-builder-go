package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	virtualtools "github.com/manishiitg/coding-agent-loop/agent_go/cmd/server/virtual-tools"
	todo_creation_human "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
	orchestratorevents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
)

func lifecycleTestAPI() *StreamingAPI {
	return &StreamingAPI{
		trackedWorkflowExecutions: map[string]*TrackedWorkflowExecution{},
		bgAgentRegistry:           NewBackgroundAgentRegistry(),
	}
}

func TestConversationTurnExecutionIDIsAvailableToToolLaunches(t *testing.T) {
	ctx := withConversationTurnExecutionID(context.Background(), "scheduled-message-root")
	if got := virtualtools.SubAgentSpecFromContext(ctx).BackgroundAgentID; got != "scheduled-message-root" {
		t.Fatalf("background execution id = %q, want scheduled-message-root", got)
	}
	if got, _ := ctx.Value(orchestratorevents.ParentExecutionIDKey).(string); got != "scheduled-message-root" {
		t.Fatalf("orchestrator parent execution id = %q, want scheduled-message-root", got)
	}
}

func TestConversationTurnTreeIgnoresUnrelatedSessionWork(t *testing.T) {
	api := lifecycleTestAPI()
	now := time.Now().UTC()
	api.trackedWorkflowExecutions["turn-a"] = &TrackedWorkflowExecution{
		ExecutionID: "turn-a", SessionID: "shared-session", Source: trackedExecutionSourceConversationTurn,
		Status: trackedExecutionStatusRunning, StartedAt: now,
	}
	api.trackedWorkflowExecutions["unrelated"] = &TrackedWorkflowExecution{
		ExecutionID: "unrelated", SessionID: "shared-session", Source: trackedExecutionSourceConversationTurn,
		Status: trackedExecutionStatusCompleted, StartedAt: now.Add(time.Second),
	}

	state := api.conversationTurnTreeSnapshot("turn-a")
	if state.terminal() {
		t.Fatal("an unrelated completed turn in the same session completed turn-a")
	}
	if state.RootStatus != trackedExecutionStatusRunning {
		t.Fatalf("root status = %q, want running", state.RootStatus)
	}
}

func TestConversationTurnTreeWaitsForRecursiveDescendants(t *testing.T) {
	api := lifecycleTestAPI()
	now := time.Now().UTC()
	rootDone := now.Add(time.Second)
	api.trackedWorkflowExecutions["root"] = &TrackedWorkflowExecution{
		ExecutionID: "root", SessionID: "session-tree", Source: trackedExecutionSourceConversationTurn,
		Status: trackedExecutionStatusCompleted, StartedAt: now, CompletedAt: &rootDone,
	}
	api.trackedWorkflowExecutions["synthetic"] = &TrackedWorkflowExecution{
		ExecutionID: "synthetic", SessionID: "session-tree", Source: trackedExecutionSourceConversationTurn,
		Status: trackedExecutionStatusRunning, StartedAt: now,
		Metadata: map[string]string{"parent_execution_id": "root"},
	}
	api.trackedWorkflowExecutions["grandchild"] = &TrackedWorkflowExecution{
		ExecutionID: "grandchild", SessionID: "session-tree", Status: trackedExecutionStatusRunning,
		StartedAt: now, Metadata: map[string]string{"parent_execution_id": "synthetic"},
	}
	api.completeTrackedExecution("synthetic", trackedExecutionStatusCompleted, "", map[string]string{"result": "processed"})
	if parent := trackedExecutionParentID(api.trackedWorkflowExecutions["synthetic"]); parent != "root" {
		t.Fatalf("completion metadata detached synthetic turn from root: parent=%q", parent)
	}

	state := api.conversationTurnTreeSnapshot("root")
	if state.RunningChildren != 1 || state.terminal() {
		t.Fatalf("state = %+v, want one running recursive child", state)
	}
	api.completeTrackedExecution("grandchild", trackedExecutionStatusCompleted, "", nil)
	state = api.conversationTurnTreeSnapshot("root")
	if !state.terminal() {
		t.Fatalf("state = %+v, want terminal after the recursive child completed", state)
	}
}

func TestConversationTurnTreeHoldsCompletedChildUntilParentNotificationDispatches(t *testing.T) {
	api := lifecycleTestAPI()
	now := time.Now().UTC()
	doneAt := now.Add(time.Second)
	api.trackedWorkflowExecutions["root"] = &TrackedWorkflowExecution{
		ExecutionID: "root", SessionID: "session-notify", Source: trackedExecutionSourceConversationTurn,
		Status: trackedExecutionStatusCompleted, StartedAt: now, CompletedAt: &doneAt,
	}
	agent := &BackgroundAgent{
		ID: "child", ParentExecutionID: "root", SessionID: "session-notify",
		Status: BGAgentCompleted, CreatedAt: now, CompletedAt: &doneAt,
	}
	api.bgAgentRegistry.Register("session-notify", agent)

	if state := api.conversationTurnTreeSnapshot("root"); state.RunningChildren != 1 || state.terminal() {
		t.Fatalf("completed-but-unnotified child must hold the root open: %+v", state)
	}
	agent.finishCompletionNotification(true)
	if state := api.conversationTurnTreeSnapshot("root"); !state.terminal() {
		t.Fatalf("notified child should release the root: %+v", state)
	}
}

func TestWaitForConversationTurnTreeReturnsExactRootFailure(t *testing.T) {
	api := lifecycleTestAPI()
	now := time.Now().UTC()
	doneAt := now.Add(time.Second)
	api.trackedWorkflowExecutions["failed-turn"] = &TrackedWorkflowExecution{
		ExecutionID: "failed-turn", SessionID: "session-failed", Source: trackedExecutionSourceConversationTurn,
		Status: trackedExecutionStatusFailed, StartedAt: now, CompletedAt: &doneAt, LastError: "model failed",
	}

	err := api.waitForConversationTurnTree(context.Background(), "session-failed", "failed-turn", time.Second)
	if !errors.Is(err, errWorkshopSessionFailed) || err == nil {
		t.Fatalf("error = %v, want exact root failure", err)
	}
}

func TestWorkshopExecutionWithoutExplicitParentAttachesToActiveQueryRoot(t *testing.T) {
	api := lifecycleTestAPI()
	api.completionLoopStarted = make(map[string]bool)
	const sessionID = "session-workshop-parent"
	api.trackExecutionStart(&TrackedWorkflowExecution{
		ExecutionID: "query-root",
		SessionID:   sessionID,
		Source:      trackedExecutionSourceConversationTurn,
		Status:      trackedExecutionStatusRunning,
		StartedAt:   time.Now().UTC(),
	})

	notifier := &workshopExecutionBgNotifier{api: api, sessionID: sessionID}
	notifier.OnExecutionStart(todo_creation_human.WorkshopExecutionStart{
		ID:   "workflow-full-child",
		Name: "full-run",
		Metadata: map[string]string{
			"suppress_auto_notification": "true",
			"execution_type":             "full-workflow",
			"run_folder":                 "iteration-0/default",
		},
	})

	agent := api.bgAgentRegistry.Get(sessionID, "workflow-full-child")
	if agent == nil {
		t.Fatal("workshop execution was not registered")
	}
	if got := agent.GetSnapshot().ParentExecutionID; got != "query-root" {
		t.Fatalf("background parent = %q, want query-root", got)
	}
	tracked := api.trackedWorkflowExecutions["workflow-full-child"]
	if got := trackedExecutionParentID(tracked); got != "query-root" {
		t.Fatalf("tracked parent = %q, want query-root", got)
	}
	if tracked.RunFolder != "iteration-0/default" || tracked.Metadata["execution_type"] != "full-workflow" {
		t.Fatalf("tracked start metadata = %+v, run_folder=%q", tracked.Metadata, tracked.RunFolder)
	}

	api.completeTrackedExecution("query-root", trackedExecutionStatusCompleted, "", nil)
	if state := api.conversationTurnTreeSnapshot("query-root"); state.RunningChildren != 1 || state.terminal() {
		t.Fatalf("active workshop child must keep query root open: %+v", state)
	}

	notifier.OnExecutionComplete("workflow-full-child", "full-run", "completed", map[string]string{
		"suppress_auto_notification": "true",
	}, nil)
	if state := api.conversationTurnTreeSnapshot("query-root"); !state.terminal() {
		t.Fatalf("query tree should finish after workshop child: %+v", state)
	}
}

func TestDurableFailedWorkflowDescendantOverridesMissingCompletionCallback(t *testing.T) {
	const (
		workspacePath = "Workflow/salesoutreach"
		runFolder     = "iteration-0/dubai-real-estate"
	)
	workspace := &mockWorkspaceAPI{files: map[string]string{
		workspacePath + "/runs/" + runFolder + "/run_metadata.json": `{"status":"failed","completed_at":"2026-08-27T10:44:12Z"}`,
	}}
	server := httptest.NewServer(workspace)
	defer server.Close()
	t.Setenv("WORKSPACE_API_URL", server.URL)

	api := lifecycleTestAPI()
	now := time.Now().UTC()
	rootDone := now.Add(time.Second)
	api.trackedWorkflowExecutions["root"] = &TrackedWorkflowExecution{
		ExecutionID: "root", SessionID: "schedule", Status: trackedExecutionStatusCompleted,
		StartedAt: now, CompletedAt: &rootDone,
	}
	api.trackedWorkflowExecutions["full-run"] = &TrackedWorkflowExecution{
		ExecutionID: "full-run", SessionID: "schedule", WorkspacePath: workspacePath,
		Kind: string(orchestratorevents.ExecutionKindFullRun), RunFolder: runFolder,
		Status: trackedExecutionStatusRunning, StartedAt: now,
		Metadata: map[string]string{"parent_execution_id": "root", "execution_type": "full-workflow", "run_folder": runFolder},
	}

	childID, gotRunFolder, failed := api.durableFailedWorkflowDescendant(context.Background(), "root")
	if !failed || childID != "full-run" || gotRunFolder != runFolder {
		t.Fatalf("durable failure = child=%q run=%q failed=%t", childID, gotRunFolder, failed)
	}

	workspace.mu.Lock()
	workspace.files[workspacePath+"/runs/"+runFolder+"/run_metadata.json"] = `{"status":"completed","completed_at":"2026-08-27T10:44:12Z"}`
	workspace.mu.Unlock()
	if childID, gotRunFolder, failed := api.durableFailedWorkflowDescendant(context.Background(), "root"); failed {
		t.Fatalf("successful durable run was treated as failure: child=%q run=%q", childID, gotRunFolder)
	}
}
