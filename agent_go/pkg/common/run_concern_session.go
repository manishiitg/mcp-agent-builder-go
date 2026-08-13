package common

import "sync"

// PLAT-055. record_run_concern must file against the step that is actually
// running, not against whatever identity the model types into its arguments.
// The workflow-DB tools already resolve their workspace from trusted
// per-session state for the same reason; this is the equivalent registry for
// concern attribution.
//
// Getting this wrong is not cosmetic: run_concerns dedups by a fingerprint of
// (step_id, concern text) and Pulse Gate weighs seen_count when deciding
// whether a root cause is worth repairing. A misattributed concern splits one
// recurring defect into two rows that each look like a one-off.

// RunConcernSessionContext is the trusted step identity for one agent session.
type RunConcernSessionContext struct {
	WorkspacePath string
	RunFolder     string
	GroupName     string
	StepID        string
	// Phase distinguishes a concern raised by the task itself from one raised
	// by a closing turn. It is the only field that legitimately changes during
	// a session, because the reflection turn reuses the execution session.
	Phase string
}

var (
	runConcernSessionsMu sync.RWMutex
	runConcernSessions   = map[string]RunConcernSessionContext{}
)

// SetRunConcernSessionContext records the trusted step identity for a session.
func SetRunConcernSessionContext(sessionID string, ctx RunConcernSessionContext) {
	if sessionID == "" {
		return
	}
	runConcernSessionsMu.Lock()
	defer runConcernSessionsMu.Unlock()
	runConcernSessions[sessionID] = ctx
}

// SetRunConcernSessionPhase moves an existing session to a new phase, leaving
// the rest of the identity intact. Used when a step's closing turn begins.
func SetRunConcernSessionPhase(sessionID, phase string) {
	if sessionID == "" || phase == "" {
		return
	}
	runConcernSessionsMu.Lock()
	defer runConcernSessionsMu.Unlock()
	existing, ok := runConcernSessions[sessionID]
	if !ok {
		return
	}
	existing.Phase = phase
	runConcernSessions[sessionID] = existing
}

// GetRunConcernSessionContext returns the trusted step identity for a session.
func GetRunConcernSessionContext(sessionID string) (RunConcernSessionContext, bool) {
	if sessionID == "" {
		return RunConcernSessionContext{}, false
	}
	runConcernSessionsMu.RLock()
	defer runConcernSessionsMu.RUnlock()
	ctx, ok := runConcernSessions[sessionID]
	return ctx, ok
}

// ClearRunConcernSessionContext drops a session's identity when it ends.
func ClearRunConcernSessionContext(sessionID string) {
	if sessionID == "" {
		return
	}
	runConcernSessionsMu.Lock()
	defer runConcernSessionsMu.Unlock()
	delete(runConcernSessions, sessionID)
}
