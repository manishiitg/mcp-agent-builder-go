package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"sync"

	orchevents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
)

// backgroundAgentTranscriptFileLocks serializes read-modify-write cycles
// against one transcript file within this process. It does not serialize
// across processes — cmd/server may concurrently write the same file's
// "running"/terminal header through the workspace API. That is the same
// accepted trade-off tokenFileMutex already makes for token usage: a rare
// lost update under true cross-process concurrency is a diagnostics gap, not
// a correctness bug in the run the transcript describes.
var backgroundAgentTranscriptFileLocks sync.Map // map[string]*sync.Mutex

func backgroundAgentTranscriptFileLock(path string) *sync.Mutex {
	v, _ := backgroundAgentTranscriptFileLocks.LoadOrStore(path, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// AppendBackgroundAgentTranscriptEvent implements BackgroundAgentTranscriptWriter
// (PLAT-164). It is best-effort by design, matching every other
// background-agent observability write in this codebase (background_agent_log,
// token persistence above it in this file's package): a transcript-append
// failure must not fail the turn it is merely recording. The caller
// (ContextAwareEventBridge.persistTranscriptAsync) surfaces the returned
// error through WaitForBackgroundTranscriptPersistence rather than swallowing
// it silently.
func (bo *BaseOrchestrator) AppendBackgroundAgentTranscriptEvent(ctx context.Context, sessionID, agentID string, evt orchevents.BackgroundAgentTranscriptEvent) error {
	workspacePath := bo.GetWorkspacePath()
	if workspacePath == "" || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(agentID) == "" {
		return nil
	}
	filePath := orchevents.BackgroundAgentTranscriptPath(workspacePath, sessionID, agentID)
	lock := backgroundAgentTranscriptFileLock(filePath)
	lock.Lock()
	defer lock.Unlock()

	var transcript *orchevents.BackgroundAgentTranscript
	existingContent, err := bo.ReadWorkspaceFile(ctx, filePath)
	if err != nil {
		errStr := err.Error()
		if !strings.Contains(errStr, "not found") && !strings.Contains(errStr, "no such file") {
			return fmt.Errorf("read background agent transcript %s: %w", filePath, err)
		}
	} else {
		transcript, err = orchevents.ParseBackgroundAgentTranscript(existingContent)
		if err != nil {
			bo.GetLogger().Warn(fmt.Sprintf("⚠️ Failed to parse background agent transcript %s, recreating: %v", filePath, err))
			transcript = nil
		}
	}
	if transcript == nil {
		// cmd/server's createBackgroundAgentTranscript normally wins this
		// race by writing the "running" header first, but a granular event
		// can legitimately reach this bridge before that write lands — don't
		// drop it just because the header isn't there yet.
		transcript = orchevents.NewBackgroundAgentTranscript(sessionID, agentID, "", "", "", evt.Timestamp)
	}
	transcript.AppendEvent(evt)

	content, err := transcript.Marshal()
	if err != nil {
		return fmt.Errorf("marshal background agent transcript %s: %w", filePath, err)
	}
	if err := bo.WriteWorkspaceFile(ctx, filePath, content); err != nil {
		return fmt.Errorf("write background agent transcript %s: %w", filePath, err)
	}
	return nil
}
