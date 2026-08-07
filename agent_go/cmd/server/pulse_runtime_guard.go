package server

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	step_based_workflow "github.com/manishiitg/coding-agent-loop/agent_go/pkg/orchestrator/agents/workflow/step_based_workflow"
	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"
	mcpexecutor "github.com/manishiitg/mcpagent/executor"
)

type trustedPulseSession struct {
	runID         string
	token         uint64
	expiresAt     time.Time
	reviewRunID   string
	reviewModules map[string]bool
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

func bindTrustedPulseReviewSession(childSessionID, reviewRunID string, modules []string) error {
	childSessionID = strings.TrimSpace(childSessionID)
	reviewRunID = strings.TrimSpace(reviewRunID)
	if childSessionID == "" || reviewRunID == "" || len(modules) == 0 {
		return fmt.Errorf("child session, review_run_id, and modules are required")
	}
	trustedPulseSessionRegistry.Lock()
	defer trustedPulseSessionRegistry.Unlock()
	trusted, ok := trustedPulseSessionRegistry.sessions[childSessionID]
	if !ok {
		return fmt.Errorf("child session %q has no delegated Pulse authority", childSessionID)
	}
	trusted.reviewRunID = reviewRunID
	trusted.reviewModules = map[string]bool{}
	for _, module := range modules {
		if canonical := pulsemodules.Normalize(module); canonical != "" {
			trusted.reviewModules[canonical] = true
		}
	}
	if len(trusted.reviewModules) == 0 {
		return fmt.Errorf("review authority has no valid modules")
	}
	trustedPulseSessionRegistry.sessions[childSessionID] = trusted
	return nil
}

func validateTrustedPulseReviewIdentity(ctx context.Context, pulseRunID, reviewRunID, module string) error {
	if err := validateTrustedPulseToolRunID(ctx, pulseRunID); err != nil {
		return err
	}
	sessionID := strings.TrimSpace(mcpexecutor.SessionIDFromContext(ctx))
	trustedPulseSessionRegistry.RLock()
	trusted := trustedPulseSessionRegistry.sessions[sessionID]
	trustedPulseSessionRegistry.RUnlock()
	module = pulsemodules.Normalize(module)
	if trusted.reviewRunID == "" || trusted.reviewRunID != strings.TrimSpace(reviewRunID) {
		return fmt.Errorf("review_run_id %q does not match this reviewer session's identity %q", reviewRunID, trusted.reviewRunID)
	}
	if !trusted.reviewModules[module] {
		return fmt.Errorf("module %q is not selected for reviewer session %q", module, sessionID)
	}
	return nil
}

// init installs the delegator on the orchestrator side. Authority lives here,
// but children are spawned in step_based_workflow, which cannot import this
// package — cmd/server imports it, so the dependency runs one way only.
func init() {
	step_based_workflow.SetPulseWriteAuthorityDelegator(DelegateTrustedPulseSessionToChild)
	step_based_workflow.SetPulseReviewAuthorityBinder(bindTrustedPulseReviewSession)
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
// The parent session id is passed in rather than read from a context. Tool
// executors here run on a context derived from the long-lived workshop session,
// not from the MCP request, so the request's session id is not present on it —
// deriving it would silently find nothing and refuse every delegation.
func DelegateTrustedPulseSessionToChild(parentSessionID, childSessionID, pulseRunID string) (func(), error) {
	parentSessionID = strings.TrimSpace(parentSessionID)
	childSessionID = strings.TrimSpace(childSessionID)
	pulseRunID = strings.TrimSpace(pulseRunID)
	if parentSessionID == "" {
		return nil, fmt.Errorf("delegating Pulse write authority requires the calling session's id")
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
