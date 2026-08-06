package orchestrator

import "testing"

// TestStampExecutionIDIsStickyFirstWriteAndTracksDisplacedIDs locks in the
// PLAT-031 identity rule: the first execution to touch a run folder's
// aggregate claims ExecutionID, repeat writes from the same execution are a
// no-op, and a different execution reusing the run folder displaces the ID
// while preserving the earlier one so the two remain distinguishable.
func TestStampExecutionIDIsStickyFirstWriteAndTracksDisplacedIDs(t *testing.T) {
	tokenFile := &TokenUsageFile{}

	stampExecutionID(tokenFile, "exec-A")
	if tokenFile.ExecutionID != "exec-A" {
		t.Fatalf("ExecutionID = %q, want exec-A", tokenFile.ExecutionID)
	}
	if len(tokenFile.PriorExecutionIDs) != 0 {
		t.Fatalf("PriorExecutionIDs = %v, want empty after first write", tokenFile.PriorExecutionIDs)
	}

	// Second write from the SAME execution (e.g. a later turn) must be a no-op.
	stampExecutionID(tokenFile, "exec-A")
	if tokenFile.ExecutionID != "exec-A" || len(tokenFile.PriorExecutionIDs) != 0 {
		t.Fatalf("repeat write from same execution changed state: id=%q prior=%v", tokenFile.ExecutionID, tokenFile.PriorExecutionIDs)
	}

	// A different execution reuses the same run folder.
	stampExecutionID(tokenFile, "exec-B")
	if tokenFile.ExecutionID != "exec-B" {
		t.Fatalf("ExecutionID = %q, want exec-B after a different execution wrote", tokenFile.ExecutionID)
	}
	if len(tokenFile.PriorExecutionIDs) != 1 || tokenFile.PriorExecutionIDs[0] != "exec-A" {
		t.Fatalf("PriorExecutionIDs = %v, want [exec-A]", tokenFile.PriorExecutionIDs)
	}

	// An empty ExecutionID (caller didn't have one) must never erase an
	// already-claimed identity.
	stampExecutionID(tokenFile, "")
	if tokenFile.ExecutionID != "exec-B" {
		t.Fatalf("empty ExecutionID must be a no-op, got %q", tokenFile.ExecutionID)
	}

	// A nil tokenFile must not panic (defensive — callers may not have
	// initialized the aggregate yet on an error path).
	stampExecutionID(nil, "exec-C")
}

// TestStampExecutionIDGivesOneIdentityAcrossTwoIndependentDateShardFiles
// models the reported incident directly: a run beginning before UTC
// midnight and finishing after it writes into TWO separate date-shard
// files (each date's file starts this run folder's aggregate fresh). Both
// calls come from the same bridge instance, so they carry the same
// ExecutionID — proving the two shards can be recognized as one execution
// without needing to merge them at write time.
func TestStampExecutionIDGivesOneIdentityAcrossTwoIndependentDateShardFiles(t *testing.T) {
	const executionID = "run-crossing-midnight"

	aug4 := &TokenUsageFile{ByModel: map[string]*ModelTokenUsage{"claude-opus-5": {InputTokens: 100}}}
	stampExecutionID(aug4, executionID)

	// A new UTC date means a brand new daily file — its run-folder entry
	// starts empty, exactly like ResolveDailyGroupTokenUsagePath resolving
	// to 2026-08-05.json instead of 2026-08-04.json.
	aug5 := &TokenUsageFile{ByModel: map[string]*ModelTokenUsage{"claude-opus-5": {InputTokens: 5}}}
	stampExecutionID(aug5, executionID)

	if aug4.ExecutionID != executionID || aug5.ExecutionID != executionID {
		t.Fatalf("shards disagree on execution identity: aug4=%q aug5=%q, want both %q",
			aug4.ExecutionID, aug5.ExecutionID, executionID)
	}
}

// TestStepAggregationKeyRejectsBareNumericIndexAsStepID is the writer-side
// half of PLAT-031: a StepTokenData with no validated StepID must never
// produce a key like "execution_only:10" that looks like a real plan step.
// A real StepID keeps the caller's phase untouched.
func TestStepAggregationKeyRejectsBareNumericIndexAsStepID(t *testing.T) {
	numericOnly := &StepTokenData{Phase: "execution_only", Step: 10, StepID: ""}
	if got, want := stepAggregationKey(numericOnly), "schedule_message:10"; got != want {
		t.Fatalf("stepAggregationKey(numeric-only) = %q, want %q", got, want)
	}

	validated := &StepTokenData{Phase: "execution_only", Step: 10, StepID: "fetch-data"}
	if got, want := stepAggregationKey(validated), "execution_only:fetch-data"; got != want {
		t.Fatalf("stepAggregationKey(validated) = %q, want %q", got, want)
	}
}
