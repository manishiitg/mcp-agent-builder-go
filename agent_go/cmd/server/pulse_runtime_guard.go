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
