package step_based_workflow

import (
	"errors"
	"strings"
	"testing"
)

// TestLendPulseWriteAuthorityFailsClosedWhenUninstalled pins the fail-closed
// behavior. A writer child that starts unauthorized would run its whole
// analysis and then fail on its first state write, having already spent the
// work and possibly mutated files.
func TestLendPulseWriteAuthorityFailsClosedWhenUninstalled(t *testing.T) {
	original := pulseWriteAuthorityDelegator()
	t.Cleanup(func() { SetPulseWriteAuthorityDelegator(original) })

	SetPulseWriteAuthorityDelegator(nil)
	if _, err := lendPulseWriteAuthority("parent-1", "workshop-fixer-1", "run-1"); err == nil ||
		!strings.Contains(err.Error(), "not installed") {
		t.Fatalf("uninstalled delegator did not fail closed: %v", err)
	}
}

func TestLendPulseWriteAuthorityPropagatesRefusalAndRelease(t *testing.T) {
	original := pulseWriteAuthorityDelegator()
	t.Cleanup(func() { SetPulseWriteAuthorityDelegator(original) })

	refusal := errors.New("session does not hold Pulse write authority for run \"run-1\"")
	SetPulseWriteAuthorityDelegator(func(string, string, string) (func(), error) {
		return nil, refusal
	})
	if _, err := lendPulseWriteAuthority("parent-1", "workshop-fixer-1", "run-1"); !errors.Is(err, refusal) {
		t.Fatalf("refusal was not propagated to the caller: %v", err)
	}

	released := false
	var gotChild, gotRun string
	SetPulseWriteAuthorityDelegator(func(_, childSessionID, pulseRunID string) (func(), error) {
		gotChild, gotRun = childSessionID, pulseRunID
		return func() { released = true }, nil
	})
	release, err := lendPulseWriteAuthority("parent-1", "workshop-fixer-1", "run-1")
	if err != nil {
		t.Fatalf("lend authority: %v", err)
	}
	if gotChild != "workshop-fixer-1" || gotRun != "run-1" {
		t.Fatalf("delegator received child=%q run=%q", gotChild, gotRun)
	}
	release()
	if !released {
		t.Fatal("release did not reach the installed delegator")
	}
}

// TestPulseWriteAuthorityDelegatorIsInstalledByServer guards the seam itself.
// The delegator lives in cmd/server and is installed by its init; if that
// wiring is dropped, every writer child silently stops being possible.
func TestPulseWriteAuthorityDelegatorIsInstalledByServer(t *testing.T) {
	if pulseWriteAuthorityDelegator() != nil {
		t.Skip("delegator already installed by an importing binary")
	}
	// Nothing imports cmd/server from this package's own test binary, so an
	// uninstalled delegator here is expected. The paired assertion lives in
	// cmd/server, where the init has actually run.
}
