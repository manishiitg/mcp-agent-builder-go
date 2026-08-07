package step_based_workflow

import (
	"fmt"
	"sync"
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
	bindPulseReviewFn     func(childSessionID, reviewRunID string, modules []string) error
)

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

// SetPulseReviewAuthorityBinder installs the server-side identity binder used
// after a reviewer child has received run authority.
func SetPulseReviewAuthorityBinder(bind func(childSessionID, reviewRunID string, modules []string) error) {
	pulseWriteAuthorityMu.Lock()
	defer pulseWriteAuthorityMu.Unlock()
	bindPulseReviewFn = bind
}

func bindPulseReviewAuthority(childSessionID, reviewRunID string, modules []string) error {
	pulseWriteAuthorityMu.RLock()
	bind := bindPulseReviewFn
	pulseWriteAuthorityMu.RUnlock()
	if bind == nil {
		return fmt.Errorf("Pulse review authority binding is not installed")
	}
	return bind(childSessionID, reviewRunID, modules)
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
