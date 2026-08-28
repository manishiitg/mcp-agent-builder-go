package step_based_workflow

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/pulsemodules"
)

// Pulse write authority is keyed by session id and owned by the server package,
// which cannot be imported here — cmd/server imports this package, so the
// dependency only runs one way. This is the seam: the server installs the
// delegation function at startup, and child-spawning code calls it through
// here.
//
// The authority itself is what structurally stops a read-only reviewer from
// writing Pulse state, rather than a prompt asking it not to. So a child that
// needs to write must be lent authority by a parent that already holds it, and
// a child that cannot be lent authority must not run as a writer at all.
var (
	pulseWriteAuthorityMu sync.RWMutex
	delegatePulseWriteFn  func(parentSessionID, childSessionID, pulseRunID string) (func(), error)
	pulseMaintenanceMu    sync.RWMutex
	pulseMaintenanceByID  = map[string]pulseMaintenancePhaseState{}
)

type pulseMaintenancePhaseState struct {
	PulseRunID     string
	Module         string
	RepairUnlocked bool
}

// BeginPulseMaintenanceReviewPhase locks one retained background sequence to
// review-safe tools. The same child session can be unlocked only after the
// runtime has observed its durable completed review receipt.
func BeginPulseMaintenanceReviewPhase(sessionID, pulseRunID, module string) (func(), error) {
	sessionID = strings.TrimSpace(sessionID)
	pulseRunID = strings.TrimSpace(pulseRunID)
	module = pulsemodules.Normalize(module)
	if sessionID == "" || pulseRunID == "" || module != pulsemodules.TechnicalReviewID {
		return nil, fmt.Errorf("Pulse technical maintenance requires session_id, pulse_run_id, and module=%q", pulsemodules.TechnicalReviewID)
	}
	pulseMaintenanceMu.Lock()
	if _, exists := pulseMaintenanceByID[sessionID]; exists {
		pulseMaintenanceMu.Unlock()
		return nil, fmt.Errorf("Pulse maintenance phase already exists for session %q", sessionID)
	}
	pulseMaintenanceByID[sessionID] = pulseMaintenancePhaseState{PulseRunID: pulseRunID, Module: module}
	pulseMaintenanceMu.Unlock()
	return func() {
		pulseMaintenanceMu.Lock()
		delete(pulseMaintenanceByID, sessionID)
		pulseMaintenanceMu.Unlock()
	}, nil
}

// UnlockPulseMaintenanceRepairPhase is called only by the background-sequence
// runtime after it has loaded a completed receipt for this exact child session.
func UnlockPulseMaintenanceRepairPhase(sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	pulseMaintenanceMu.Lock()
	defer pulseMaintenanceMu.Unlock()
	state, exists := pulseMaintenanceByID[sessionID]
	if !exists {
		return fmt.Errorf("Pulse maintenance review phase is not registered for session %q", sessionID)
	}
	state.RepairUnlocked = true
	pulseMaintenanceByID[sessionID] = state
	return nil
}

// PulseMaintenanceToolAllowed is the server/runtime backstop for both normal
// agent tool calls and session-scoped MCP bridge calls. Before the receipt
// barrier, only evidence reads plus typed review persistence are allowed.
func PulseMaintenanceToolAllowed(sessionID, toolName string) (bool, string) {
	sessionID = strings.TrimSpace(sessionID)
	toolName = strings.TrimSpace(toolName)
	pulseMaintenanceMu.RLock()
	state, exists := pulseMaintenanceByID[sessionID]
	pulseMaintenanceMu.RUnlock()
	if !exists || state.RepairUnlocked {
		return true, ""
	}
	if pulseReviewPhaseToolAllowed(toolName) {
		return true, ""
	}
	return false, fmt.Sprintf("tool %q is unavailable during the read-only Pulse Technical Review phase; complete_pulse_review must persist a completed %s receipt before the next sequence turn can repair", toolName, state.Module)
}

// PulseMaintenanceToolAllowedWithArgs narrows the one review-phase writer to
// the run-scoped Markdown checkpoint. Session-scoped HTTP calls are separately
// constrained by the dynamic folder guard; this check covers in-process tool
// execution whose folder snapshot is frozen when the retained agent is built.
func PulseMaintenanceToolAllowedWithArgs(sessionID, toolName string, args map[string]interface{}) (bool, string) {
	allowed, reason := PulseMaintenanceToolAllowed(sessionID, toolName)
	if !allowed || toolName != "diff_patch_workspace_file" {
		return allowed, reason
	}
	pulseMaintenanceMu.RLock()
	state, exists := pulseMaintenanceByID[strings.TrimSpace(sessionID)]
	pulseMaintenanceMu.RUnlock()
	if !exists || state.RepairUnlocked {
		return true, ""
	}
	path, _ := args["filepath"].(string)
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	checkpointSegment := "/runs/pulse/" + state.PulseRunID + "/"
	if strings.Contains("/"+strings.TrimPrefix(clean, "/"), checkpointSegment) {
		return true, ""
	}
	return false, fmt.Sprintf("diff_patch_workspace_file may write only runs/pulse/%s/ during the read-only Pulse Technical Review phase", state.PulseRunID)
}

func pulseReviewPhaseToolAllowed(toolName string) bool {
	for _, prefix := range []string{"get_", "list_", "query_", "read_", "search_", "review_", "validate_", "debug_"} {
		if strings.HasPrefix(toolName, prefix) {
			return true
		}
	}
	switch toolName {
	case "generate_text_llm", "read_image", "diff_patch_workspace_file",
		"record_pulse_finding", "record_pulse_review_focus", "record_pulse_verification",
		"complete_pulse_review", "merge_pulse_issues", "resolve_run_concern",
		"record_pulse_impact", "create_human_input_request":
		return true
	default:
		return false
	}
}

// SetPulseWriteAuthorityDelegator installs the server-side delegator. Passing
// nil uninstalls it, which disables writer children rather than silently
// letting them run unauthorized.
func SetPulseWriteAuthorityDelegator(
	delegate func(parentSessionID, childSessionID, pulseRunID string) (func(), error),
) {
	pulseWriteAuthorityMu.Lock()
	defer pulseWriteAuthorityMu.Unlock()
	delegatePulseWriteFn = delegate
}

// pulseWriteAuthorityDelegator returns the installed delegator, if any.
func pulseWriteAuthorityDelegator() func(string, string, string) (func(), error) {
	pulseWriteAuthorityMu.RLock()
	defer pulseWriteAuthorityMu.RUnlock()
	return delegatePulseWriteFn
}

// LendPulseWriteAuthorityForTest exercises the installed delegator from
// packages that can import this one, so the cross-package seam is covered by a
// test that fails when the wiring is dropped rather than only at runtime.
func LendPulseWriteAuthorityForTest(parentSessionID, childSessionID, pulseRunID string) (func(), error) {
	return lendPulseWriteAuthority(parentSessionID, childSessionID, pulseRunID)
}

// lendPulseWriteAuthority gives childSessionID the parent's authority for
// pulseRunID and returns the release function.
//
// It fails closed. A writer child that starts without authority would run its
// whole analysis and then fail on its first state write, having already spent
// the work and possibly mutated files — worse than not starting.
func lendPulseWriteAuthority(parentSessionID, childSessionID, pulseRunID string) (func(), error) {
	delegate := pulseWriteAuthorityDelegator()
	if delegate == nil {
		return nil, fmt.Errorf("Pulse write authority delegation is not installed; cannot start a writer child")
	}
	release, err := delegate(parentSessionID, childSessionID, pulseRunID)
	if err != nil {
		return nil, err
	}
	if release == nil {
		return func() {}, nil
	}
	return release, nil
}
