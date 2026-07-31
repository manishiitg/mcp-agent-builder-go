package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	mcpexecutor "github.com/manishiitg/mcpagent/executor"
)

type trustedPulseSession struct {
	runID     string
	token     uint64
	expiresAt time.Time
}

var trustedPulseSessionRegistry = struct {
	sync.RWMutex
	nextToken uint64
	sessions  map[string]trustedPulseSession
}{sessions: map[string]trustedPulseSession{}}

// pulseWorklistRecordMu serializes the short read-then-write window used to
// make a logical Pulse run's Gate decision immutable once fully recorded.
var pulseWorklistRecordMu sync.Mutex

// registerTrustedPulseSession binds a physical workshop session to the logical
// Pulse run it is allowed to update. Recovery sessions use the original run ID.
func registerTrustedPulseSession(sessionID, pulseRunID string) func() {
	return registerTrustedPulseSessionUntil(sessionID, pulseRunID, time.Time{})
}

// registerTemporaryTrustedPulseSession gives an explicit standalone
// /pulse-fixer turn a bounded lifecycle-writing window. Scheduled sessions use
// registerTrustedPulseSession and are released by the scheduler; a manual
// command cannot retain a release callback across tool calls, so its authority
// expires automatically.
func registerTemporaryTrustedPulseSession(sessionID, pulseRunID string, ttl time.Duration) {
	if ttl <= 0 {
		ttl = 2 * time.Hour
	}
	_ = registerTrustedPulseSessionUntil(sessionID, pulseRunID, time.Now().UTC().Add(ttl))
}

func registerTrustedPulseSessionUntil(sessionID, pulseRunID string, expiresAt time.Time) func() {
	sessionID = strings.TrimSpace(sessionID)
	pulseRunID = strings.TrimSpace(pulseRunID)
	if sessionID == "" || pulseRunID == "" {
		return func() {}
	}

	trustedPulseSessionRegistry.Lock()
	trustedPulseSessionRegistry.nextToken++
	token := trustedPulseSessionRegistry.nextToken
	trustedPulseSessionRegistry.sessions[sessionID] = trustedPulseSession{runID: pulseRunID, token: token, expiresAt: expiresAt}
	trustedPulseSessionRegistry.Unlock()

	return func() {
		trustedPulseSessionRegistry.Lock()
		if current, ok := trustedPulseSessionRegistry.sessions[sessionID]; ok && current.token == token {
			delete(trustedPulseSessionRegistry.sessions, sessionID)
		}
		trustedPulseSessionRegistry.Unlock()
	}
}

func validateTrustedPulseToolRunID(ctx context.Context, requestedRunID string) error {
	requestedRunID = strings.TrimSpace(requestedRunID)
	if requestedRunID == "" {
		return fmt.Errorf("pulse_run_id is required")
	}
	sessionID := strings.TrimSpace(mcpexecutor.SessionIDFromContext(ctx))
	if sessionID == "" {
		return fmt.Errorf("Pulse state writes require an active scheduler session")
	}

	trustedPulseSessionRegistry.RLock()
	trusted, ok := trustedPulseSessionRegistry.sessions[sessionID]
	trustedPulseSessionRegistry.RUnlock()
	if !ok {
		return fmt.Errorf("session %q is not authorized to update Pulse state", sessionID)
	}
	if !trusted.expiresAt.IsZero() && !time.Now().UTC().Before(trusted.expiresAt) {
		trustedPulseSessionRegistry.Lock()
		if current, exists := trustedPulseSessionRegistry.sessions[sessionID]; exists && current.token == trusted.token {
			delete(trustedPulseSessionRegistry.sessions, sessionID)
		}
		trustedPulseSessionRegistry.Unlock()
		return fmt.Errorf("session %q Pulse maintenance authorization expired", sessionID)
	}
	if trusted.runID != requestedRunID {
		return fmt.Errorf("pulse_run_id %q does not match this session's logical Pulse run %q", requestedRunID, trusted.runID)
	}
	return nil
}

// DelegateTrustedPulseSessionToChild lends one child execution the parent's
// writing authority for a single Pulse run, so the Fixer can run as a tracked
// background agent instead of occupying the operator's foreground turn.
//
// Authority is keyed by session id, and a background child runs under its own
// session, so without this it simply cannot record module results. That guard
// is load-bearing: it is what structurally prevents a read-only reviewer from
// writing state, rather than only a prompt telling it not to. So delegation is
// never self-service — the parent must already hold authority for exactly this
// run, which a reviewer never does.
//
// Returns a release function. Delegation does not outlive the parent's own
// grant: the child inherits the parent's expiry when it has one.
func DelegateTrustedPulseSessionToChild(parentCtx context.Context, childSessionID, pulseRunID string) (func(), error) {
	parentSessionID := strings.TrimSpace(mcpexecutor.SessionIDFromContext(parentCtx))
	childSessionID = strings.TrimSpace(childSessionID)
	pulseRunID = strings.TrimSpace(pulseRunID)
	if parentSessionID == "" {
		return nil, fmt.Errorf("delegating Pulse write authority requires an active parent session")
	}
	if childSessionID == "" || pulseRunID == "" {
		return nil, fmt.Errorf("child session id and pulse_run_id are required")
	}
	if childSessionID == parentSessionID {
		return nil, fmt.Errorf("child session must differ from the delegating parent session")
	}

	trustedPulseSessionRegistry.RLock()
	parent, ok := trustedPulseSessionRegistry.sessions[parentSessionID]
	trustedPulseSessionRegistry.RUnlock()
	if !ok || parent.runID != pulseRunID {
		return nil, fmt.Errorf("session %q does not hold Pulse write authority for run %q", parentSessionID, pulseRunID)
	}
	if !parent.expiresAt.IsZero() && !time.Now().UTC().Before(parent.expiresAt) {
		return nil, fmt.Errorf("session %q Pulse maintenance authorization expired", parentSessionID)
	}

	return registerTrustedPulseSessionUntil(childSessionID, pulseRunID, parent.expiresAt), nil
}

// isTrustedPulseRunLive reports whether any session still holds writing
// authority for pulseRunID.
//
// A module is claimed by recording it `due` with an empty result, and that row
// outlives the process that wrote it. When a Pulse dies mid-pass the claim
// stays behind forever, so a state-only check cannot tell "a pass is working on
// this" from "a pass abandoned this". social-media stranded five modules that
// way: its 2026-07-31 manual pass timed out under the old consolidated stage,
// whose label matched no module, so nothing was ever marked terminal and
// /pulse-fixer then refused every one of them with no way to recover.
//
// Authority is the honest liveness signal: it is granted when a pass starts and
// released when it ends, expires, or the process restarts.
func isTrustedPulseRunLive(pulseRunID string) bool {
	pulseRunID = strings.TrimSpace(pulseRunID)
	if pulseRunID == "" {
		return false
	}
	now := time.Now().UTC()
	trustedPulseSessionRegistry.RLock()
	defer trustedPulseSessionRegistry.RUnlock()
	for _, trusted := range trustedPulseSessionRegistry.sessions {
		if trusted.runID != pulseRunID {
			continue
		}
		if !trusted.expiresAt.IsZero() && !now.Before(trusted.expiresAt) {
			continue
		}
		return true
	}
	return false
}

func releaseTrustedPulseSessionForRun(ctx context.Context, pulseRunID string) {
	sessionID := strings.TrimSpace(mcpexecutor.SessionIDFromContext(ctx))
	pulseRunID = strings.TrimSpace(pulseRunID)
	if sessionID == "" || pulseRunID == "" {
		return
	}
	trustedPulseSessionRegistry.Lock()
	if current, ok := trustedPulseSessionRegistry.sessions[sessionID]; ok && current.runID == pulseRunID {
		delete(trustedPulseSessionRegistry.sessions, sessionID)
	}
	trustedPulseSessionRegistry.Unlock()
}
