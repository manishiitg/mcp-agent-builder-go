package server

import (
	"context"
	"fmt"
	"strings"

	mcpexecutor "github.com/manishiitg/mcpagent/executor"
)

// Protect the parent writer, not the reviewer writing its own final receipt.
// Use the same exact-turn tree as the scheduler waiter; unrelated work in the
// session and the current caller itself must not masquerade as a running child.
func (api *StreamingAPI) guardPulseResultExecutor(executors map[string]interface{}, fallbackSessionID string) {
	execute, ok := executors["record_pulse_result"].(func(context.Context, map[string]interface{}) (string, error))
	if !ok {
		return
	}
	executors["record_pulse_result"] = func(ctx context.Context, args map[string]interface{}) (string, error) {
		if strings.TrimSpace(stringToolArg(args, "module")) != "" && strings.EqualFold(strings.TrimSpace(stringToolArg(args, "result")), "failed") {
			sessionID := strings.TrimSpace(mcpexecutor.SessionIDFromContext(ctx))
			if sessionID == "" {
				sessionID = fallbackSessionID
			}
			root, callerPath := api.pulseResultCallerTree(sessionID)
			if state := api.conversationTurnTreeSnapshot(root, callerPath...); state.RunningChildren > 0 {
				return "", fmt.Errorf("cannot record a failed Pulse review while this turn has %d active child execution(s); wait for their terminal results and recheck the run receipt. A missing checkpoint is not proof of failure", state.RunningChildren)
			}
		}
		return execute(ctx, args)
	}
}

// A completion-notification turn may be nested under the original dispatch
// while another reviewer is still running beside it. Walk back to that same
// session's tree, excluding only the writer's own ancestor chain from the live
// count. Never cross into a different child tool session or an unrelated turn.
func (api *StreamingAPI) pulseResultCallerTree(sessionID string) (string, []string) {
	current := api.currentConversationTurnExecutionID(sessionID)
	if current == "" {
		return "", nil
	}
	api.trackedWorkflowExecutionsMux.RLock()
	defer api.trackedWorkflowExecutionsMux.RUnlock()
	var callerPath []string
	seen := map[string]bool{}
	for !seen[current] {
		seen[current] = true
		callerPath = append(callerPath, current)
		execution := api.trackedWorkflowExecutions[current]
		parentID := trackedExecutionParentID(execution)
		parent := api.trackedWorkflowExecutions[parentID]
		if parent == nil || parent.SessionID != sessionID || seen[parentID] {
			break
		}
		current = parentID
	}
	return current, callerPath
}
