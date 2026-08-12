package step_based_workflow

import (
	"strings"
	"testing"

	"github.com/manishiitg/coding-agent-loop/agent_go/pkg/contractupgrade"
)

func TestStampIsAllowedInsideItsOwnUpgradeTurn(t *testing.T) {
	t.Cleanup(func() { contractupgrade.Revoke("schedule-session") })

	contractupgrade.Mint("schedule-session", "1.0.21")
	refusal, ok := authorizeContractVersionStamp("schedule-session", "1.0.21")
	if !ok {
		t.Fatalf("stamp refused inside its own turn: %s", refusal)
	}
	if refusal != "" {
		t.Fatalf("permitted stamp returned a refusal: %s", refusal)
	}
}

// The confida-login 2026-08-12 case: the scheduler adjudicated the 1.0.21 turn
// as failed at 08:01:35 and the same session stamped 1.0.21 at 08:11:10 from an
// unrelated Pulse turn, via a shell curl to the MCP bridge.
func TestStampFromAClosedTurnIsRefused(t *testing.T) {
	t.Cleanup(func() { contractupgrade.Revoke("schedule-session") })

	contractupgrade.Mint("schedule-session", "1.0.21")
	contractupgrade.Revoke("schedule-session") // scheduler adjudicates the turn

	refusal, ok := authorizeContractVersionStamp("schedule-session", "1.0.21")
	if ok {
		t.Fatal("a closed turn was allowed to stamp the version it had declined")
	}
	// The refusal has to tell the agent what actually resolves this, or it will
	// read as a platform fault and get retried or worked around.
	for _, want := range []string{"no contract upgrade turn is open", "resolve the blocker", "makes the scheduler skip it"} {
		if !strings.Contains(refusal, want) {
			t.Fatalf("refusal missing %q: %s", want, refusal)
		}
	}
}

func TestStampOfAnotherVersionNamesWhatThisTurnMayStamp(t *testing.T) {
	t.Cleanup(func() { contractupgrade.Revoke("schedule-session") })

	contractupgrade.Mint("schedule-session", "1.0.21")
	refusal, ok := authorizeContractVersionStamp("schedule-session", "1.0.25")
	if ok {
		t.Fatal("a turn stamped a version other than its own target")
	}
	if !strings.Contains(refusal, "1.0.21") || !strings.Contains(refusal, "1.0.25") {
		t.Fatalf("refusal should name both the granted and the attempted version: %s", refusal)
	}
}

func TestStampIsSpentOnce(t *testing.T) {
	t.Cleanup(func() { contractupgrade.Revoke("schedule-session") })

	contractupgrade.Mint("schedule-session", "1.0.21")
	if _, ok := authorizeContractVersionStamp("schedule-session", "1.0.21"); !ok {
		t.Fatal("first stamp refused")
	}
	if _, ok := authorizeContractVersionStamp("schedule-session", "1.0.21"); ok {
		t.Fatal("the same authorization was spent twice")
	}
}

// A session with no controller-scoped HTTP session ID must not fall through to
// an unbounded stamp.
func TestStampWithoutASessionIsRefused(t *testing.T) {
	if _, ok := authorizeContractVersionStamp("", "1.0.21"); ok {
		t.Fatal("a blank session was allowed to stamp")
	}
}
