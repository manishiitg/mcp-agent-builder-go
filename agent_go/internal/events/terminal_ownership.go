package events

import (
	"fmt"
	"strings"

	orchestratorevents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
)

var childTranscriptDetailEventTypes = map[string]bool{
	"tool_call_start":      true,
	"tool_call_end":        true,
	"tool_call_error":      true,
	"llm_generation_start": true,
	"llm_generation_end":   true,
	"llm_generation_error": true,
	// Streaming events stay on SSE for live terminal/progress rendering. The
	// frontend consumes them ephemerally and no longer retains per-child screen
	// snapshots. Cursor pages omit them through shouldReturnEvent.
	"status_line":   true,
	"token_usage":   true,
	"system_prompt": true,
	"user_message":  true,
}

// ResolveTerminalOwnerID is the single backend contract that maps one event to
// the terminal transcript that owns it. New events are resolved once when they
// enter EventStore; terminal snapshot construction calls the same function as
// a compatibility fallback for older restored events.
//
// Ownership is intentionally singular. Execution containers and internal
// turns may fold into a workflow-step terminal, but one event must never be
// copied into several selectable transcripts.
func ResolveTerminalOwnerID(sessionID string, event Event, suppliedMetadata map[string]interface{}) string {
	sessionID = strings.TrimSpace(firstNonEmptyString(sessionID, event.SessionID))
	if sessionID == "" {
		return ""
	}

	metadata := resolvedTerminalMetadata(&event, suppliedMetadata)
	if ownerID := resolveWorkflowStepOwner(event, metadata); validResolvedTerminalOwner(ownerID, sessionID) {
		return ownerID
	}

	// A goroutine-local execution owner is more authoritative than a provider
	// event that inherited its parent's main-agent label.
	if ownerID := firstValidResolvedTerminalOwner(sessionID,
		stringField(metadata, "execution_owner_id"),
		stringField(metadata, "owner_execution_id"),
	); ownerID != "" {
		return ownerID
	}

	// Child identity is more specific than provider-level execution kind.
	if ownerID := firstValidResolvedTerminalOwner(sessionID,
		stringField(metadata, "background_agent_id"),
		stringField(metadata, "delegation_id"),
		stringField(metadata, "agent_id"),
	); ownerID != "" {
		return ownerID
	}

	if resolvedEventIsMainAgent(sessionID, event, metadata) {
		return "main:" + sessionID
	}

	if ownerID := firstValidResolvedTerminalOwner(sessionID,
		stringField(metadata, "execution_id"),
		event.ExecutionID,
	); ownerID != "" {
		return ownerID
	}

	return firstValidResolvedTerminalOwner(sessionID,
		stringField(metadata, "current_step_id"),
		stringField(metadata, "workflow_step_id"),
		stringField(metadata, "step_id"),
		stringField(metadata, "correlation_id"),
	)
}

// TerminalIDForOwner returns the exact identity used by terminal snapshots and
// by the per-terminal event API.
func TerminalIDForOwner(sessionID, ownerID string) string {
	sessionID = strings.TrimSpace(sessionID)
	ownerID = strings.TrimSpace(ownerID)
	if sessionID == "" {
		return ""
	}
	if ownerID == "" {
		return sessionID
	}
	return fmt.Sprintf("%s:%s", sessionID, ownerID)
}

// RetainEventInSessionWorkingSet reports whether an event belongs in the
// lightweight session channel. Child transcript detail is available from the
// per-terminal cursor endpoint and does not need to cross the wire twice.
// Legacy events without a resolved terminal fail open for compatibility.
func RetainEventInSessionWorkingSet(sessionID string, event Event) bool {
	sessionID = strings.TrimSpace(sessionID)
	terminalID := strings.TrimSpace(event.TerminalID)
	if sessionID == "" || terminalID == "" {
		return true
	}
	ownerID := strings.TrimSpace(event.TerminalOwnerID)
	if ownerID == "main:"+sessionID || terminalID == TerminalIDForOwner(sessionID, "main:"+sessionID) {
		return true
	}
	return !childTranscriptDetailEventTypes[event.Type]
}

func FilterSessionWorkingSet(sessionID string, source []Event) []Event {
	filtered := make([]Event, 0, len(source))
	for _, event := range source {
		if RetainEventInSessionWorkingSet(sessionID, event) {
			filtered = append(filtered, event)
		}
	}
	return filtered
}

func resolvedTerminalMetadata(event *Event, supplied map[string]interface{}) map[string]interface{} {
	metadata := make(map[string]interface{})
	payload := eventPayloadMap(event)
	for key, value := range payload {
		metadata[key] = value
	}
	for key, value := range mapField(payload, "metadata") {
		metadata[key] = value
	}
	for key, value := range supplied {
		metadata[key] = value
	}
	if event != nil {
		if event.ExecutionID != "" {
			if _, exists := metadata["execution_id"]; !exists {
				metadata["execution_id"] = event.ExecutionID
			}
		}
		if event.ParentExecutionID != "" {
			if _, exists := metadata["parent_execution_id"]; !exists {
				metadata["parent_execution_id"] = event.ParentExecutionID
			}
		}
		if event.ExecutionKind != "" {
			if _, exists := metadata["execution_kind"]; !exists {
				metadata["execution_kind"] = event.ExecutionKind
			}
		}
	}
	return metadata
}

func resolveWorkflowStepOwner(event Event, metadata map[string]interface{}) string {
	for _, candidate := range []string{
		event.ExecutionID,
		stringField(metadata, "execution_id"),
		stringField(metadata, "agent_execution_id"),
	} {
		candidate = strings.TrimSpace(candidate)
		if strings.HasPrefix(candidate, "workflow-step:") {
			return canonicalResolvedWorkflowStepOwner(candidate)
		}
	}

	agentID := firstNonEmptyString(
		stringField(metadata, "agent_id"),
		stringField(metadata, "background_agent_id"),
		event.ExecutionID,
	)
	parentExecutionID := firstNonEmptyString(
		stringField(metadata, "parent_execution_id"),
		stringField(metadata, "owner_execution_id"),
		event.ParentExecutionID,
	)
	stepID := firstNonEmptyString(
		resolvedWorkflowStepIDFromExecution(agentID),
		resolvedWorkflowStepIDFromExecution(parentExecutionID),
		stringField(metadata, "current_step_id"),
		stringField(metadata, "workflow_step_id"),
		stringField(metadata, "step_id"),
	)

	declaredKind := orchestratorevents.ParseExecutionKind(stringField(metadata, "execution_kind"))
	if declaredKind.FoldsIntoParent() && parentExecutionID != "" && stepID != "" {
		return "workflow-step:" + parentExecutionID + ":" + stepID
	}
	if strings.HasPrefix(agentID, "msgseq-") && stepID != "" &&
		(strings.HasPrefix(parentExecutionID, "exec-") || strings.HasPrefix(parentExecutionID, "workflow-full-")) {
		return "workflow-step:" + parentExecutionID + ":" + stepID
	}
	if !resolvedIsTerminalMetadata(metadata) && strings.HasPrefix(agentID, "exec-") && stepID != "" {
		return "workflow-step:" + agentID + ":" + stepID
	}

	workflowID := firstNonEmptyString(
		stringField(metadata, "workflow_execution_id"),
		stringField(metadata, "workflow_run_id"),
		stringField(metadata, "execution_owner_id"),
		stringField(metadata, "execution_id"),
		event.ExecutionID,
	)
	if strings.HasPrefix(workflowID, "workflow-full-") && stepID != "" {
		return "workflow-step:" + workflowID + ":" + stepID
	}
	return ""
}

func resolvedEventIsMainAgent(sessionID string, event Event, metadata map[string]interface{}) bool {
	kind := strings.ToLower(strings.TrimSpace(firstNonEmptyString(
		stringField(metadata, "execution_kind"),
		stringField(metadata, "scope"),
		event.ExecutionKind,
	)))
	if kind == "main_agent" || kind == "main" || kind == "chat" {
		return true
	}
	return event.ExecutionID == "main:"+sessionID
}

func resolvedIsTerminalMetadata(metadata map[string]interface{}) bool {
	kind := strings.ToLower(firstNonEmptyString(
		stringField(metadata, "kind"),
		stringField(metadata, "stream_kind"),
		stringField(metadata, "display_kind"),
		stringField(metadata, "mode"),
	))
	return kind == "terminal" || kind == "tmux" || kind == "tui"
}

func firstValidResolvedTerminalOwner(sessionID string, candidates ...string) string {
	for _, candidate := range candidates {
		if validResolvedTerminalOwner(candidate, sessionID) {
			return strings.TrimSpace(candidate)
		}
	}
	return ""
}

func validResolvedTerminalOwner(candidate, sessionID string) bool {
	candidate = strings.TrimSpace(candidate)
	return candidate != "" && candidate != strings.TrimSpace(sessionID)
}

func resolvedWorkflowStepIDFromExecution(executionID string) string {
	executionID = strings.TrimSpace(executionID)
	if !strings.HasPrefix(executionID, "exec-") {
		return ""
	}
	withoutPrefix := strings.TrimPrefix(executionID, "exec-")
	lastDash := strings.LastIndex(withoutPrefix, "-")
	if lastDash <= 0 {
		return ""
	}
	suffix := withoutPrefix[lastDash+1:]
	if len(suffix) < 12 {
		return ""
	}
	for _, r := range suffix {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return strings.TrimSpace(withoutPrefix[:lastDash])
}

func canonicalResolvedWorkflowStepOwner(ownerID string) string {
	ownerID = strings.TrimSpace(ownerID)
	if !strings.HasPrefix(ownerID, "workflow-step:") {
		return ownerID
	}
	parts := strings.Split(ownerID, ":")
	if len(parts) < 3 {
		return ownerID
	}
	executionID := strings.TrimSpace(strings.Join(parts[1:len(parts)-1], ":"))
	encodedStepID := resolvedWorkflowStepIDFromExecution(executionID)
	if executionID == "" || encodedStepID == "" {
		return ownerID
	}
	return "workflow-step:" + executionID + ":" + encodedStepID
}
