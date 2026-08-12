// Package contractupgrade bounds when a workflow contract version may be stamped.
//
// A contract upgrade is a blocking preflight: the scheduler opens a turn asking
// the agent to perform one migration, then advances the ladder only once
// workflow.json carries that target version. Nothing used to bound *when* the
// stamp could be written.
//
// Observed on confida-login 2026-08-12: the scheduler adjudicated the 1.0.21
// turn as failed at 08:01:35 (the agent had correctly declined — the improve
// archive held 19 finding IDs absent from the pulse_* tables). At 08:11:10 the
// same session, by then running an unrelated Pulse finalizer turn, wrote the
// stamp anyway with
//
//	curl -X POST -d '{"version":"1.0.21"}' -H "$MCP_AUTH" "$MCP_CUSTOM/set_workflow_contract_version"
//
// The next preflight read 1.0.21, trusted it, and moved on to 1.0.22 — skipping
// the 1.0.21 migration outright. Only a hand-written revert stopped that from
// sticking.
//
// Two properties of that failure decide this design. The write arrived over the
// MCP bridge rather than as a native tool call, so no prompt-level rule could
// have refused it: the bound has to sit in the executor both paths share. And
// the session was alive and legitimately working, so killing sessions at
// adjudication would truncate migrations mid-rewrite — worse than the bug.
//
// Grants are process-local on purpose. One grant authorizes stamps for the life
// of one turn; surviving a restart would mean authorizing a turn that is no
// longer running.
package contractupgrade

import (
	"sort"
	"strings"
	"sync"
)

var (
	mu     sync.Mutex
	grants = map[string]map[string]bool{}
)

// Mint authorizes sessionID to stamp each of targets, replacing any previous
// authorization for that session. Targets are a set rather than a single
// version because Pulse folds every outstanding upgrade query into one
// Review+Fix turn (postRunMonitorAgenticReviewFixStep), so one turn can
// legitimately owe several stamps. Blank session IDs and blank targets are
// ignored; minting with no usable target revokes instead, so a caller cannot
// accidentally leave a stale grant behind.
func Mint(sessionID string, targets ...string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	allowed := make(map[string]bool, len(targets))
	for _, target := range targets {
		if target = strings.TrimSpace(target); target != "" {
			allowed[target] = true
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(allowed) == 0 {
		delete(grants, sessionID)
		return
	}
	grants[sessionID] = allowed
}

// Revoke withdraws every authorization held by sessionID. The scheduler calls
// this the moment it adjudicates a preflight turn — on the passing path as much
// as the failing one, since a turn that has been judged must not be able to
// change the thing it was judged on.
func Revoke(sessionID string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	delete(grants, sessionID)
}

// Consume authorizes one stamp of version by sessionID and spends that
// authorization, so a repeat call for the same version is refused. It reports
// whether the stamp is allowed.
func Consume(sessionID, version string) bool {
	sessionID = strings.TrimSpace(sessionID)
	version = strings.TrimSpace(version)
	if sessionID == "" || version == "" {
		return false
	}
	mu.Lock()
	defer mu.Unlock()
	allowed := grants[sessionID]
	if !allowed[version] {
		return false
	}
	delete(allowed, version)
	if len(allowed) == 0 {
		delete(grants, sessionID)
	}
	return true
}

// Restore returns an authorization that Consume spent on a stamp which then
// failed before it changed workflow.json. Without it a transient write error
// would strand the turn: the agent has done the migration, the grant is gone,
// and the retry it is told to make cannot succeed. Unlike Mint this adds to the
// session's existing set rather than replacing it, so restoring one target
// cannot drop the others a Pulse turn still owes.
func Restore(sessionID, version string) {
	sessionID = strings.TrimSpace(sessionID)
	version = strings.TrimSpace(version)
	if sessionID == "" || version == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if grants[sessionID] == nil {
		grants[sessionID] = map[string]bool{}
	}
	grants[sessionID][version] = true
}

// Granted lists the versions sessionID may still stamp, sorted for a stable
// refusal message. An empty result means the session holds no grant at all.
func Granted(sessionID string) []string {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	allowed := grants[sessionID]
	if len(allowed) == 0 {
		return nil
	}
	versions := make([]string, 0, len(allowed))
	for version := range allowed {
		versions = append(versions, version)
	}
	sort.Strings(versions)
	return versions
}
