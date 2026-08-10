package step_based_workflow

import (
	"strings"
	"testing"

	loggerv2 "github.com/manishiitg/mcpagent/logger/v2"
)

// PLAT-059. A lock now has to say why.
//
// Under the shared topic-organised skill (PLAT-058) a locked step still reads
// every other step's contributions and can never give anything back, so the
// freeze is a standing cost someone must be able to re-judge later. LinkedIn
// reached 6 of 6 steps locked with no recorded justification for any of them —
// nobody could tell whether that was deliberate or accumulated drift, and the
// pre-existing "include review_notes explaining why" convention was advisory
// and was therefore skipped.

func TestLockingLearningsRequiresAReason(t *testing.T) {
	err := validateLockLearningsChange(true, "")
	if err == nil {
		t.Fatal("a step was frozen with no stated reason")
	}
	// The rejection must name the field, say why the freeze is costly, and point
	// at the cheaper alternative — otherwise every "this step should not write"
	// case becomes a lock that then needs justifying.
	for _, want := range []string{"lock_learnings_reason", "can never contribute", `learnings_access="read"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("rejection missing %q: %v", want, err)
		}
	}

	if err := validateLockLearningsChange(true, "   "); err == nil {
		t.Error("whitespace was accepted as a justification")
	}
	if err := validateLockLearningsChange(true, "selectors stable across 12 runs since 2026-06"); err != nil {
		t.Errorf("a real reason was rejected: %v", err)
	}
}

func TestUnlockingNeverRequiresAReason(t *testing.T) {
	// Unlocking restores the default (contribute); it costs nothing and must not
	// be gated, or a step stays frozen because nobody could phrase a release note.
	if err := validateLockLearningsChange(false, ""); err != nil {
		t.Errorf("unlocking demanded a reason: %v", err)
	}
}

func TestClearingLockAlsoClearsItsReason(t *testing.T) {
	// A reason left behind would attach to a freeze that no longer exists, and
	// would then read as justification for a future re-lock nobody reviewed.
	locked := true
	sc := &StepConfig{AgentConfigs: &AgentConfigs{
		LockLearnings:       &locked,
		LockLearningsReason: "selectors stable across 12 runs",
	}}

	if !clearStepConfigField(sc, "lock_learnings") {
		t.Fatal("clearStepConfigField did not recognise lock_learnings")
	}
	if sc.AgentConfigs.LockLearnings != nil {
		t.Error("lock_learnings was not cleared")
	}
	if sc.AgentConfigs.LockLearningsReason != "" {
		t.Errorf("reason survived the lock being cleared: %q", sc.AgentConfigs.LockLearningsReason)
	}
}

func TestLockReasonSurvivesConfigMerge(t *testing.T) {
	// The reason has to travel with the lock through config merging, or the
	// upgrade audit sees a locked step with an empty reason and reports a gap
	// that does not exist.
	locked := true
	source := &AgentConfigs{LockLearnings: &locked, LockLearningsReason: "frozen pending a selector rewrite"}
	target := &AgentConfigs{}

	MergeAgentConfigFields(target, source, "step-x", loggerv2.NewNoop())

	if target.LockLearnings == nil || !*target.LockLearnings {
		t.Fatal("lock did not merge")
	}
	if target.LockLearningsReason != "frozen pending a selector rewrite" {
		t.Errorf("reason did not merge: %q", target.LockLearningsReason)
	}
}
