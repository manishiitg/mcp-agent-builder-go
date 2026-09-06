// Synchronous (blocking) delegation: spawning a sub-agent for a delegated
// task, the workshop sub-agent tracking notifier, and the delegation
// start/end UI events. Relocated verbatim from server.go.
// (Async/background delegation lives in background_agents.go.)
package server

import (
	"context"
	"fmt"
	"github.com/manishiitg/coding-agent-loop/agent_go/internal/events"
	todo_creation_human "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
	orchEvents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	unifiedevents "github.com/manishiitg/mcpagent/events"
)

func safeDelegationRuntimeID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Sprintf("delegation-%d", time.Now().UnixNano())
	}
	var b strings.Builder
	b.Grow(len(id))
	lastDash := false
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	clean := strings.Trim(b.String(), "-")
	if clean == "" {
		return fmt.Sprintf("delegation-%d", time.Now().UnixNano())
	}
	if len(clean) > 96 {
		clean = strings.Trim(clean[:96], "-")
		if clean == "" {
			return fmt.Sprintf("delegation-%d", time.Now().UnixNano())
		}
	}
	return clean
}

func delegatedCodingAgentRuntimeFolder(userID, runtimeID string) string {
	return strings.TrimSuffix(perUserChatsFolderFor(userID), "/") + "/.agents/" + safeDelegationRuntimeID(runtimeID)
}

func scopeDelegatedHumanInputExecutor(workflowPath string, executor func(context.Context, map[string]interface{}) (string, error)) func(context.Context, map[string]interface{}) (string, error) {
	return func(ctx context.Context, args map[string]interface{}) (string, error) {
		requested, _ := args["workspace_path"].(string)
		normalized, normalizeErr := normalizeReportHumanInputWorkspacePath(requested)
		if normalizeErr != nil {
			return "", normalizeErr
		}
		if normalized != workflowPath {
			return "", fmt.Errorf("background workflow agent may create human-input requests only for %s", workflowPath)
		}
		return executor(ctx, args)
	}
}

// --- Background Agent Infrastructure for Async Delegation ---

// workshopExecutionBgNotifier implements WorkshopExecutionNotifier by registering
// workshop step/background executions in bgAgentRegistry so that HasRunningAgents()
// returns true and the frontend keeps polling for events.
type workshopExecutionBgNotifier struct {
	api           *StreamingAPI
	sessionID     string
	workspacePath string
	presetQueryID string
	userID        string
}

func (n *workshopExecutionBgNotifier) OnExecutionStart(start todo_creation_human.WorkshopExecutionStart) {
	if n.api.autoNotificationSessionUnreachable(n.sessionID) {
		log.Printf("[BG AGENT] OnExecutionStart ignored for stopped session %s (exec=%s)", n.sessionID, start.ID)
		if start.Cancel != nil {
			start.Cancel()
		}
		return
	}
	// A declared kind always wins. The name/id prefix sniffing below predates
	// declared kinds and exists only to classify executions that never set one.
	kind := strings.TrimSpace(start.Kind)
	if kind == "" {
		if isWorkflowStepTrackingExecution(start.ID, start.Name, start.Metadata) {
			kind = "workflow_step"
		} else {
			kind = "workshop_background"
		}
	}
	parentExecutionID := strings.TrimSpace(start.ParentExecutionID)
	if parentExecutionID == "" {
		// Workshop sessions deliberately detach long-running work from the
		// initiating HTTP context. If an older/custom launch path did not copy
		// the query root before detaching, recover the exact currently-running
		// conversation turn here rather than registering an orphan execution.
		parentExecutionID = n.api.currentConversationTurnExecutionID(n.sessionID)
	}
	metadata := map[string]string{
		"workflow_path":    n.workspacePath,
		"preset_query_id":  n.presetQueryID,
		"execution_source": trackedExecutionSourceWorkshopBackground,
	}
	for k, v := range start.Metadata {
		if strings.TrimSpace(k) == "" {
			continue
		}
		metadata[k] = v
	}
	bgAgent := &BackgroundAgent{
		ID:                start.ID,
		ParentExecutionID: parentExecutionID,
		Name:              start.Name,
		SessionID:         n.sessionID,
		Kind:              kind,
		Status:            BGAgentRunning,
		CreatedAt:         time.Now(),
		cancel:            start.Cancel,
		Metadata:          metadata,
	}
	n.api.bgAgentRegistry.Register(n.sessionID, bgAgent)
	n.api.trackWorkshopExecutionStart(n.sessionID, n.workspacePath, n.presetQueryID, n.userID, start.ID, start.Name, parentExecutionID, metadata)
	if bgAgent.GetStatus() == BGAgentCanceled {
		n.api.completeTrackedExecution(start.ID, trackedExecutionStatusCanceled, "parent execution canceled", metadata)
		if bgAgent.MarkTerminalNotified() {
			n.api.emitBackgroundAgentTerminated(n.sessionID, start.ID, start.Name, "")
		}
		return
	}

	// Pre-create the channel so NotifyCompletion never drops a completion
	n.api.bgAgentRegistry.GetNotificationChannel(n.sessionID)

	// Ensure background completion loop is running
	n.api.completionLoopStartedMu.Lock()
	if !n.api.completionLoopStarted[n.sessionID] {
		n.api.completionLoopStarted[n.sessionID] = true
		go n.api.backgroundCompletionLoop(n.sessionID)
	}
	n.api.completionLoopStartedMu.Unlock()

	// Emit background_agent_started so the execution remains visible and follows
	// the same parent-notification lifecycle as workflow steps. Long coding-CLI
	// tool calls may detach from the foreground even though their HTTP request is
	// still active, so generic/reviewer children must notify the parent rather than
	// relying only on the synchronous response path.
	// Forward the kind resolved above (creator's declaration, or the
	// workshop_background default, or the workflow_step override) so the
	// terminal store never has to re-infer it from the execution id.
	n.api.emitBackgroundAgentStarted(n.sessionID, start.ID, start.Name, "", parentExecutionID, orchEvents.ParseExecutionKind(kind))
	if metadata["suppress_auto_notification"] != "true" {
		n.api.notifyBackgroundAgentStarted(n.sessionID, start.ID)
	}
}

func isWorkflowStepTrackingExecution(id, name string, meta map[string]string) bool {
	if meta != nil && strings.TrimSpace(meta["execution_type"]) == "workflow-step" {
		return true
	}
	trimmedName := strings.TrimSpace(name)
	if strings.HasPrefix(trimmedName, "Step ->") || strings.HasPrefix(trimmedName, "Workflow step ->") {
		return true
	}
	trimmedID := strings.TrimSpace(id)
	return strings.HasPrefix(trimmedID, "workflow-step-") ||
		(strings.HasPrefix(trimmedID, "workflow-full-") && strings.Contains(trimmedID, "-step-"))
}

// suppressRepeatedChildFailureNotification marks an enclosing execution silent
// when it failed only because a direct message-sequence child already reported
// the same root error. The child remains the authoritative notification; the
// parent is still recorded and emitted as a terminal execution event, but it
// must not create a second synthetic user turn containing the same failure.
// Keeping this at the backend notification boundary makes the behavior
// consistent for every product and every client.
func suppressRepeatedChildFailureNotification(registry *BackgroundAgentRegistry, sessionID string, parent *BackgroundAgent) bool {
	if registry == nil || parent == nil {
		return false
	}
	parentSnapshot := parent.GetSnapshot()
	if parentSnapshot.Status != BGAgentFailed || strings.TrimSpace(parentSnapshot.Error) == "" {
		return false
	}
	for _, candidate := range registry.GetAll(sessionID) {
		if candidate == nil || candidate == parent {
			continue
		}
		child := candidate.GetSnapshot()
		if strings.TrimSpace(child.ParentExecutionID) != parentSnapshot.ID || child.Status != BGAgentFailed {
			continue
		}
		isMessageSequenceItem := child.Kind == "message_sequence_item" ||
			(child.Metadata != nil && child.Metadata["execution_type"] == "message-sequence-item")
		childError := strings.TrimSpace(child.Error)
		if !isMessageSequenceItem || childError == "" || !strings.Contains(parentSnapshot.Error, childError) {
			continue
		}
		parent.SetMetadata(map[string]string{
			"suppress_auto_notification": "true",
			"notification_suppression":   "repeated-message-sequence-child-failure",
		})
		return true
	}
	return false
}

// suppressParentOwnedMessageSequenceSuccess keeps successful completion at the
// workflow-step boundary. A message-sequence item is an implementation detail
// of that step; publishing both the child success and the parent success creates
// two identical "Step complete" turns. Failures deliberately take the opposite
// path above so the specific child error remains visible. A standalone item is
// never suppressed because there is no registered parent that can report its
// completion.
func suppressParentOwnedMessageSequenceSuccess(registry *BackgroundAgentRegistry, sessionID string, child *BackgroundAgent) bool {
	if registry == nil || child == nil {
		return false
	}
	snapshot := child.GetSnapshot()
	isMessageSequenceItem := snapshot.Kind == "message_sequence_item" ||
		(snapshot.Metadata != nil && snapshot.Metadata["execution_type"] == "message-sequence-item")
	parentID := strings.TrimSpace(snapshot.ParentExecutionID)
	if snapshot.Status != BGAgentCompleted || !isMessageSequenceItem || parentID == "" || registry.Get(sessionID, parentID) == nil {
		return false
	}
	child.SetMetadata(map[string]string{
		"suppress_auto_notification": "true",
		"notification_suppression":   "parent-owned-message-sequence-success",
	})
	return true
}

func (n *workshopExecutionBgNotifier) OnExecutionComplete(execID, name, result string, meta map[string]string, err error) {
	if n.api.autoNotificationSessionUnreachable(n.sessionID) {
		n.api.completeTrackedExecution(execID, trackedExecutionStatusCanceled, "session stopped", meta)
		log.Printf("[BG AGENT] OnExecutionComplete ignored for stopped session %s (exec=%s)", n.sessionID, execID)
		return
	}
	agent := n.api.bgAgentRegistry.Get(n.sessionID, execID)
	if agent == nil {
		return
	}

	// An agent already marked canceled was explicitly stopped by stop_step,
	// stop_all, or session shutdown. That path owns the terminal event and must not
	// be converted into a failure when the worker later unwinds with context.Canceled.
	if agent.GetStatus() == BGAgentCanceled {
		log.Printf("[BG AGENT] OnExecutionComplete skipped for already-canceled agent %s", execID)
		return
	}

	// Context-canceled / deadline-exceeded while the registry still says running
	// is a runtime timeout or unexpected parent-context loss, not an explicit user
	// cancellation. Treat it as failed and notify the waiting main agent. Marking
	// it canceled would make processBackgroundAgentCompletion deliberately suppress
	// the synthetic turn, which strands Pulse after a timed-out maintenance agent.
	if err != nil && (strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context deadline exceeded")) {
		if !agent.SetError(err.Error()) {
			return
		}
		n.api.completeTrackedExecution(execID, trackedExecutionStatusFailed, err.Error(), meta)
		duration := time.Since(agent.CreatedAt)
		n.api.emitBackgroundAgentCompleted(n.sessionID, execID, name, "failed", "", err.Error(), duration.Truncate(time.Second).String())
		suppressNotification := agent.GetSnapshot().Metadata["suppress_auto_notification"] == "true"
		log.Printf("[BG AGENT] Background execution %s ended from context loss while still running (suppress_auto_notification=%t): %v", execID, suppressNotification, err)
		if !suppressNotification {
			n.api.bgAgentRegistry.NotifyCompletion(n.sessionID, execID)
		}
		return
	}

	duration := time.Since(agent.CreatedAt)
	if len(meta) > 0 {
		agent.SetMetadata(meta)
	}
	if err != nil {
		if !agent.SetError(err.Error()) {
			return
		}
		n.api.completeTrackedExecution(execID, trackedExecutionStatusFailed, err.Error(), meta)
		n.api.emitBackgroundAgentCompleted(n.sessionID, execID, name, "failed", "", err.Error(), duration.Truncate(time.Second).String())
	} else {
		if !agent.SetResult(result) {
			return
		} // A concurrent stop owns the terminal outcome.
		n.api.completeTrackedExecution(execID, trackedExecutionStatusCompleted, "", meta)
		displayResult := workshopCompletionDisplayResult(n.workspacePath, result, meta)
		n.api.emitBackgroundAgentCompleted(n.sessionID, execID, name, "completed", displayResult, "", duration.Truncate(time.Second).String())
	}

	if err == nil && suppressParentOwnedMessageSequenceSuccess(n.api.bgAgentRegistry, n.sessionID, agent) {
		log.Printf("[BG AGENT] Suppressed child success notification for %s; enclosing execution %s owns the completion", execID, agent.GetSnapshot().ParentExecutionID)
	}
	if err != nil && suppressRepeatedChildFailureNotification(n.api.bgAgentRegistry, n.sessionID, agent) {
		log.Printf("[BG AGENT] Suppressed repeated parent failure notification for %s; direct message-sequence child already owns the same error", execID)
	}

	// A finished parent cannot still have live progress children. Settle any it
	// left running, so an end event that never arrived cannot pin the session
	// busy forever and stall the scheduler's drain-wait (PLAT-091).
	if orphans := n.api.bgAgentRegistry.ReconcileOrphanedProgressChildren(
		n.sessionID, execID,
		fmt.Sprintf("parent execution %s finished without an end event for this step", execID),
	); len(orphans) > 0 {
		log.Printf("[BG AGENT] Settled %d orphaned progress child(ren) of finished execution %s in session %s: %v",
			len(orphans), execID, n.sessionID, orphans)
		// The registry cannot reach the durable log, so emit each settlement
		// here (PLAT-117). Without this, a settled orphan stays stored
		// status='running', completed_at=NULL in PLAT-114's background_agent_log
		// forever — social-media 2026-08-16 left two such rows, which read as
		// live work to anyone auditing that table later.
		reason := fmt.Sprintf("parent execution %s finished without an end event for this step", execID)
		for _, orphanID := range orphans {
			// ReconcileOrphanedProgressChildren owns the background-agent
			// registry, while Global Monitor reads the unified execution
			// tracker. Settle both copies of the same progress mirror. Leaving
			// the tracker running makes the authoritative runtime snapshot busy
			// even though the foreground turn, terminal, parent execution, and
			// durable schedule run have all finished (PLAT-117).
			n.api.completeTrackedExecution(orphanID, trackedExecutionStatusFailed, reason, nil)
			name := orphanID
			if orphan := n.api.bgAgentRegistry.Get(n.sessionID, orphanID); orphan != nil {
				name = orphan.GetSnapshot().Name
			}
			n.api.emitBackgroundAgentCompleted(n.sessionID, orphanID, name, "failed", "", reason, "")
		}
	}

	// Signal completion to the notification loop unless the parent is already
	// synchronously awaiting this execution's direct tool result.
	if agent.GetSnapshot().Metadata["suppress_auto_notification"] != "true" {
		n.api.bgAgentRegistry.NotifyCompletion(n.sessionID, execID)
	}
}

func workshopCompletionDisplayResult(workspacePath, result string, meta map[string]string) string {
	const defaultLimit = 500
	if strings.TrimSpace(meta["execution_type"]) != "pulse-reviewer" {
		return truncateForToolResponse(result, defaultLimit)
	}

	resultPath := strings.TrimSpace(meta["review_result_path"])
	workspacePath = strings.TrimSpace(workspacePath)
	if resultPath == "" || workspacePath == "" || filepath.IsAbs(resultPath) {
		return truncateForToolResponse(result, defaultLimit)
	}
	root, err := filepath.Abs(workspacePath)
	if err != nil {
		return truncateForToolResponse(result, defaultLimit)
	}
	candidate, err := filepath.Abs(filepath.Join(root, filepath.Clean(resultPath)))
	if err != nil {
		return truncateForToolResponse(result, defaultLimit)
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return truncateForToolResponse(result, defaultLimit)
	}
	content, err := os.ReadFile(candidate)
	if err != nil {
		return truncateForToolResponse(result, defaultLimit)
	}

	findings := strings.TrimSpace(string(content))
	if marker := "\n## Findings\n"; strings.Contains(findings, marker) {
		findings = strings.TrimSpace(strings.SplitN(findings, marker, 2)[1])
	}
	if findings == "" {
		return truncateForToolResponse(result, defaultLimit)
	}
	// Keep the complete review in SQLite. Only this immediate UI preview is
	// bounded (and explicitly marked as truncated); this is not a finding or
	// storage cap.
	return truncateForToolResponse(findings, 6000)
}

func (n *workshopExecutionBgNotifier) OnExecutionTerminated(execID, name string) {
	if n.api.autoNotificationSessionUnreachable(n.sessionID) {
		n.api.completeTrackedExecution(execID, trackedExecutionStatusCanceled, "session stopped", nil)
		return
	}
	for _, agent := range n.api.bgAgentRegistry.CancelExecutionTree(n.sessionID, execID) {
		snapshot := agent.GetSnapshot()
		n.api.completeTrackedExecution(snapshot.ID, trackedExecutionStatusCanceled, "execution terminated", nil)
		if agent.MarkTerminalNotified() {
			n.api.emitBackgroundAgentTerminated(n.sessionID, snapshot.ID, snapshot.Name, "")
		}
	}

}

// workflowSubAgentTrackingNotifier tracks inner workshop sub-agents in the backend
// execution tree and triggers synthetic-turn notifications only when they finish.
type workflowSubAgentTrackingNotifier struct {
	api       *StreamingAPI
	sessionID string
}

func (n *workflowSubAgentTrackingNotifier) OnSubAgentStart(start todo_creation_human.WorkshopExecutionStart) {
	if n == nil || n.api == nil || strings.TrimSpace(start.ID) == "" {
		return
	}
	if n.api.autoNotificationSessionUnreachable(n.sessionID) {
		if start.Cancel != nil {
			start.Cancel()
		}
		return
	}
	kind := strings.TrimSpace(start.Kind)
	if kind == "" {
		kind = "workflow_sub_agent"
	}
	bgAgent := &BackgroundAgent{
		ID:                start.ID,
		ParentExecutionID: start.ParentExecutionID,
		Name:              start.Name,
		SessionID:         n.sessionID,
		Kind:              kind,
		Status:            BGAgentRunning,
		CreatedAt:         time.Now(),
		Metadata:          start.Metadata,
		cancel:            start.Cancel,
	}
	n.api.bgAgentRegistry.Register(n.sessionID, bgAgent)
	suppressAutoNotification := start.Metadata["suppress_auto_notification"] == "true"
	if !suppressAutoNotification {
		// Pre-create the completion channel and loop so a fast sub-agent completion
		// cannot drop its auto-notification. Parent-reconciled children skip this
		// path because their result returns to the owning orchestrator conversation.
		n.api.bgAgentRegistry.GetNotificationChannel(n.sessionID)
		n.api.completionLoopStartedMu.Lock()
		if n.api.completionLoopStarted == nil {
			n.api.completionLoopStarted = make(map[string]bool)
		}
		if !n.api.completionLoopStarted[n.sessionID] {
			n.api.completionLoopStarted[n.sessionID] = true
			go n.api.backgroundCompletionLoop(n.sessionID)
		}
		n.api.completionLoopStartedMu.Unlock()
	}

	n.api.emitBackgroundAgentStarted(n.sessionID, start.ID, start.Name, "", start.ParentExecutionID, orchEvents.ParseExecutionKind(kind))
	if !suppressAutoNotification {
		n.api.notifyBackgroundAgentStarted(n.sessionID, start.ID)
	}
}

func (n *workflowSubAgentTrackingNotifier) OnSubAgentComplete(agentID, name string, result string, err error) {
	if n == nil || n.api == nil || strings.TrimSpace(agentID) == "" {
		return
	}
	agent := n.api.bgAgentRegistry.Get(n.sessionID, agentID)
	if agent == nil {
		return
	}
	if agent.GetStatus() == BGAgentCanceled {
		return
	}
	suppressAutoNotification := agent.GetSnapshot().Metadata["suppress_auto_notification"] == "true"
	if err != nil {
		if strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "context deadline exceeded") {
			agent.SetCanceled()
			// Emit a terminal event for context-canceled sub-agents so the
			// completion loop has a consistent signal — mirrors OnExecutionComplete
			// (finding-onsubagentcomplete-context-cancel-silent-drop fix).
			if !n.api.isSessionMarkedStopped(n.sessionID) {
				if agent.MarkTerminalNotified() {
					n.api.emitBackgroundAgentTerminated(n.sessionID, agentID, name, "canceled")
					if !suppressAutoNotification {
						n.api.bgAgentRegistry.NotifyCompletion(n.sessionID, agentID)
					}
				}
			}
			return
		}
		agent.SetError(err.Error())
		if agent.GetStatus() == BGAgentCanceled {
			return
		}
		duration := time.Since(agent.CreatedAt)
		n.api.emitBackgroundAgentCompleted(n.sessionID, agentID, name, "failed", "", err.Error(), duration.Truncate(time.Second).String())
		if !suppressAutoNotification {
			n.api.bgAgentRegistry.NotifyCompletion(n.sessionID, agentID)
		}
		return
	}
	agent.SetResult(result)
	if agent.GetStatus() == BGAgentCanceled {
		return
	}
	duration := time.Since(agent.CreatedAt)
	n.api.emitBackgroundAgentCompleted(n.sessionID, agentID, name, "completed", truncateForToolResponse(result, 500), "", duration.Truncate(time.Second).String())
	if !suppressAutoNotification {
		n.api.bgAgentRegistry.NotifyCompletion(n.sessionID, agentID)
	}
}

// truncateForToolResponse truncates a string for inclusion in tool responses
func truncateForToolResponse(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}

// emitDelegationStartEvent emits an event when delegation starts
// This event serves as the parent for all sub-agent events (via parent_id linking)
func (api *StreamingAPI) emitDelegationStartEvent(sessionID, delegationID string, depth int, instruction, reasoningLevel, modelID string, servers []string, backgroundAgentID, agentTemplate string) {
	now := time.Now()
	eventID := fmt.Sprintf("%s_delegation_start_%s", sessionID, delegationID)
	eventData := &events.DelegationStartEventData{
		DelegationID:      delegationID,
		Depth:             depth,
		Instruction:       instruction,
		ReasoningLevel:    reasoningLevel,
		ModelID:           modelID,
		Servers:           servers,
		BackgroundAgentID: backgroundAgentID,
		AgentTemplate:     agentTemplate,
		Timestamp:         now.Format(time.RFC3339),
	}
	event := events.Event{
		ID:        eventID,
		Type:      "delegation_start",
		Timestamp: now,
		SessionID: sessionID,
		Data: &unifiedevents.AgentEvent{
			Type:           unifiedevents.EventType("delegation_start"),
			Timestamp:      now,
			HierarchyLevel: depth,
			SessionID:      sessionID,
			Component:      fmt.Sprintf("delegation-%d", depth),
			CorrelationID:  delegationID, // Links all delegation events together
			ParentID: func() string {
				if strings.TrimSpace(backgroundAgentID) == "" {
					return ""
				}
				return fmt.Sprintf("%s_background_agent_started_%s", sessionID, strings.TrimSpace(backgroundAgentID))
			}(),
			Data: eventData,
		},
	}
	api.eventStore.AddEvent(sessionID, event)
	log.Printf("[DELEGATION] Emitted delegation_start event %s for %s at depth %d", eventID, delegationID, depth)
}
