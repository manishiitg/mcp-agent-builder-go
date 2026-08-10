package orchestrator

import (
	"testing"
	"time"
)

// TestApplyModelUsageToPhaseTokenUsageFileOverwritesRatherThanAccumulates
// pins the PLAT-073 cluster B fix (e6be98dfd6f4d639): the caller (workflow
// builder chat) passes the coding agent's session-cumulative usage snapshot
// on every turn, not a per-turn delta. The old merge-based implementation
// added that growing cumulative total on top of what earlier turns already
// wrote, so a two-turn session's persisted total was roughly 3x the real
// spend (turn 1's cumulative, plus turn 2's larger cumulative added on top).
func TestApplyModelUsageToPhaseTokenUsageFileOverwritesRatherThanAccumulates(t *testing.T) {
	file := &PhaseTokenUsageFile{}
	now := time.Now()

	// Turn 1: session cumulative so far.
	turn1 := &ModelTokenUsage{InputTokens: 1000, OutputTokens: 500, TotalCost: 0.10}
	ApplyModelUsageToPhaseTokenUsageFile(file, "workflow-builder", "claude-sonnet-5", turn1, now)

	got := file.ByModel["claude-sonnet-5"]
	if got.InputTokens != 1000 || got.TotalCost != 0.10 {
		t.Fatalf("after turn 1: InputTokens=%d TotalCost=%.4f, want 1000/0.10", got.InputTokens, got.TotalCost)
	}

	// Turn 2: mcpagent reports the NEW session-cumulative total (which
	// already includes turn 1's tokens), not a delta for turn 2 alone.
	turn2Cumulative := &ModelTokenUsage{InputTokens: 2200, OutputTokens: 1100, TotalCost: 0.22}
	ApplyModelUsageToPhaseTokenUsageFile(file, "workflow-builder", "claude-sonnet-5", turn2Cumulative, now)

	got = file.ByModel["claude-sonnet-5"]
	if got.InputTokens != 2200 || got.TotalCost != 0.22 {
		t.Fatalf("after turn 2: InputTokens=%d TotalCost=%.4f, want 2200/0.22 (the fresh cumulative snapshot, not 1000+2200=3200)", got.InputTokens, got.TotalCost)
	}

	byPhase := file.ByPhaseAndModel["workflow-builder"]["claude-sonnet-5"]
	if byPhase.InputTokens != 2200 || byPhase.TotalCost != 0.22 {
		t.Fatalf("by_phase_and_model after turn 2: InputTokens=%d TotalCost=%.4f, want 2200/0.22", byPhase.InputTokens, byPhase.TotalCost)
	}
}

// TestApplyModelUsageToPhaseTokenUsageFileClonesRatherThanAliases guards
// against a regression where the stored bucket and the caller's usage
// pointer become the same object — a later in-place mutation of the
// caller's ModelTokenUsage (e.g. EnsureModelTokenUsagePricing re-run on the
// same pointer) must not silently rewrite an already-persisted snapshot.
func TestApplyModelUsageToPhaseTokenUsageFileClonesRatherThanAliases(t *testing.T) {
	file := &PhaseTokenUsageFile{}
	now := time.Now()

	usage := &ModelTokenUsage{InputTokens: 500, TotalCost: 0.05}
	ApplyModelUsageToPhaseTokenUsageFile(file, "workflow-builder", "claude-sonnet-5", usage, now)

	usage.InputTokens = 999999
	usage.TotalCost = 999.0

	got := file.ByModel["claude-sonnet-5"]
	if got.InputTokens != 500 || got.TotalCost != 0.05 {
		t.Fatalf("stored bucket was aliased to the caller's usage pointer: InputTokens=%d TotalCost=%.4f, want 500/0.05", got.InputTokens, got.TotalCost)
	}
}
