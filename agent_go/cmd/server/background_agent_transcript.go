package server

import (
	"context"
	"log"
	"sync"
	"time"

	orchEvents "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/events"
)

// backgroundAgentTranscriptLocks serializes read-modify-write cycles against
// one transcript file within this process. It does not serialize across
// processes — this server and a separate orchestrator process may both write
// the same transcript (this server at start/terminal, the orchestrator's
// ContextAwareEventBridge for the granular event body) through the shared
// workspace API. That is the same accepted trade-off pkg/orchestrator's
// tokenFileMutex already makes for token usage: a rare lost update under
// true cross-process concurrency is a diagnostics gap, not a correctness bug
// in the run the transcript describes.
var backgroundAgentTranscriptLocks sync.Map // map[string]*sync.Mutex

func backgroundAgentTranscriptLock(path string) *sync.Mutex {
	v, _ := backgroundAgentTranscriptLocks.LoadOrStore(path, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// createBackgroundAgentTranscript writes the initial "running" transcript
// record before the child's first provider turn (PLAT-164 requirement 1).
// Called from emitBackgroundAgentStarted, which already fires unconditionally
// as soon as a background agent is registered — before any orchestrator-level
// construction/initialization that could itself fail — so a child that never
// gets far enough to run a turn still leaves this record once
// finalizeBackgroundAgentTranscript below marks it terminal.
func (api *StreamingAPI) createBackgroundAgentTranscript(sessionID, agentID, name, kind, parentExecutionID string, startedAt time.Time) {
	workspacePath := api.backgroundAgentLogWorkspacePath(sessionID)
	if workspacePath == "" {
		return
	}
	transcriptPath := orchEvents.BackgroundAgentTranscriptPath(workspacePath, sessionID, agentID)
	lock := backgroundAgentTranscriptLock(transcriptPath)
	lock.Lock()
	defer lock.Unlock()

	ctx := context.Background()
	status := "ok"
	if existingContent, exists, err := readFileFromWorkspace(ctx, transcriptPath); err != nil {
		log.Printf("[BG_AGENT_TRANSCRIPT] failed to read existing transcript %s: %v", transcriptPath, err)
	} else if exists && existingContent != "" {
		// A transcript already exists for this (session, agent) pair — an
		// agent ID collision or a duplicate start call. Do not clobber
		// whatever events it already has; just leave it in place.
		api.recordBackgroundAgentTranscriptPath(sessionID, agentID, transcriptPath, status)
		return
	}

	transcript := orchEvents.NewBackgroundAgentTranscript(sessionID, agentID, parentExecutionID, name, kind, startedAt)
	content, err := transcript.Marshal()
	if err != nil {
		log.Printf("[BG_AGENT_TRANSCRIPT] failed to marshal new transcript for session=%s agent=%s: %v", sessionID, agentID, err)
		status = "error: " + err.Error()
	} else if err := writeFileToWorkspace(ctx, transcriptPath, content); err != nil {
		log.Printf("[BG_AGENT_TRANSCRIPT] failed to write transcript %s: %v", transcriptPath, err)
		status = "error: " + err.Error()
	}
	api.recordBackgroundAgentTranscriptPath(sessionID, agentID, transcriptPath, status)
}

// finalizeBackgroundAgentTranscript marks a background agent's transcript
// terminal exactly once, at the same point its background_agent_log summary
// reaches a terminal state (PLAT-164 requirement 2). If no transcript exists
// yet — the start write failed, or setup never got as far as
// createBackgroundAgentTranscript's caller — this creates one now so the
// terminal diagnostic record still exists, per requirement 1's "a child that
// fails during setup still leaves a terminal diagnostic record".
func (api *StreamingAPI) finalizeBackgroundAgentTranscript(sessionID, agentID, status, errMsg string, completedAt time.Time) {
	workspacePath := api.backgroundAgentLogWorkspacePath(sessionID)
	if workspacePath == "" {
		return
	}
	transcriptPath := orchEvents.BackgroundAgentTranscriptPath(workspacePath, sessionID, agentID)
	lock := backgroundAgentTranscriptLock(transcriptPath)
	lock.Lock()
	defer lock.Unlock()

	ctx := context.Background()
	writeStatus := "ok"
	existingContent, exists, err := readFileFromWorkspace(ctx, transcriptPath)
	if err != nil {
		log.Printf("[BG_AGENT_TRANSCRIPT] failed to read transcript %s before finalizing: %v", transcriptPath, err)
		writeStatus = "error: " + err.Error()
	}

	var transcript *orchEvents.BackgroundAgentTranscript
	if exists && existingContent != "" {
		transcript, err = orchEvents.ParseBackgroundAgentTranscript(existingContent)
		if err != nil {
			log.Printf("[BG_AGENT_TRANSCRIPT] failed to parse transcript %s, recreating: %v", transcriptPath, err)
			transcript = nil
		}
	}
	if transcript == nil {
		transcript = orchEvents.NewBackgroundAgentTranscript(sessionID, agentID, "", "", "", completedAt)
	}
	transcript.MarkTerminal(status, errMsg, completedAt)

	content, marshalErr := transcript.Marshal()
	if marshalErr != nil {
		log.Printf("[BG_AGENT_TRANSCRIPT] failed to marshal transcript %s: %v", transcriptPath, marshalErr)
		writeStatus = "error: " + marshalErr.Error()
	} else if writeErr := writeFileToWorkspace(ctx, transcriptPath, content); writeErr != nil {
		log.Printf("[BG_AGENT_TRANSCRIPT] failed to write finalized transcript %s: %v", transcriptPath, writeErr)
		writeStatus = "error: " + writeErr.Error()
	}
	api.recordBackgroundAgentTranscriptPath(sessionID, agentID, transcriptPath, writeStatus)
}
