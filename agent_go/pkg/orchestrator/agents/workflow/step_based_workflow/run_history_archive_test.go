package step_based_workflow

import "testing"

func run(folder, status, startedAt string) map[string]interface{} {
	return map[string]interface{}{"run_folder": folder, "status": status, "started_at": startedAt}
}

// TestRunHistoryRepointsOnlyTheRunThatOwnedTheFolder is the hetznerssh case.
//
// Every run executes in iteration-0 and records it, but iteration-0 is only the
// live slot — the next run rotates it to a permanent iteration-N. Nothing
// repointed the history entry, so 24 entries all claimed iteration-0 while the
// folders on disk were iteration-21..25.
//
// The visible damage was in the schedule popup: cost is looked up per run
// folder, and rotation already archives cost records to the new name correctly,
// so every history row resolved to the one folder still called iteration-0 and
// showed the CURRENT run's spend — identical on every row.
//
// Only the newest finished claimant is repointed. The older ones are from runs
// whose folders were pruned long ago; their real names are unrecoverable.
func TestRunHistoryRepointsOnlyTheRunThatOwnedTheFolder(t *testing.T) {
	runs := []map[string]interface{}{
		run("iteration-0", "success", "2026-08-05T18:13:00Z"),
		run("iteration-0", "error", "2026-08-10T09:00:00Z"), // newest finished — this one produced the folder
		run("iteration-0", "success", "2026-08-09T14:28:00Z"),
		run("iteration-24", "success", "2026-08-01T09:00:00Z"),
	}

	got := scheduleRunIndexOwningFolder(runs, "iteration-0")
	if got != 1 {
		t.Fatalf("index = %d, want 1 (the newest finished run claiming the folder)", got)
	}
}

// TestRunHistoryLeavesTheLiveSlotAloneWhileARunStillHoldsIt.
//
// Rotation happens at the start of the next run, and the new run's entry has no
// folder recorded yet — but a suspended run is the case that actually bites. A
// capacity-suspended run resumes into the SAME folder when the window reopens
// (PLAT-101); repointing it at an archived name would send the resume looking
// for evidence under a name it never wrote to.
func TestRunHistoryLeavesTheLiveSlotAloneWhileARunStillHoldsIt(t *testing.T) {
	for _, status := range []string{"running", "waiting_for_capacity"} {
		runs := []map[string]interface{}{
			run("iteration-0", status, "2026-08-18T12:12:00Z"),
		}
		if got := scheduleRunIndexOwningFolder(runs, "iteration-0"); got != -1 {
			t.Errorf("status %q: index = %d, want -1 — the run has not finished with the folder", status, got)
		}
	}

	// A suspended run must not shield an older finished run from being repointed.
	runs := []map[string]interface{}{
		run("iteration-0", "waiting_for_capacity", "2026-08-18T12:12:00Z"),
		run("iteration-0", "success", "2026-08-10T09:00:00Z"),
	}
	if got := scheduleRunIndexOwningFolder(runs, "iteration-0"); got != 1 {
		t.Errorf("index = %d, want 1 — the finished run still needs repointing", got)
	}
}

// TestRunHistoryFindsNothingToRepointWhenNoRunClaimsTheFolder keeps rotation
// silent in the ordinary cases: a workflow run started from chat writes no run
// history at all, and a folder nothing claims must not repoint some other run.
func TestRunHistoryFindsNothingToRepointWhenNoRunClaimsTheFolder(t *testing.T) {
	if got := scheduleRunIndexOwningFolder(nil, "iteration-0"); got != -1 {
		t.Errorf("empty history: index = %d, want -1", got)
	}
	runs := []map[string]interface{}{run("iteration-24", "success", "2026-08-01T09:00:00Z")}
	if got := scheduleRunIndexOwningFolder(runs, "iteration-0"); got != -1 {
		t.Errorf("index = %d, want -1 — no entry claims iteration-0", got)
	}
	if got := scheduleRunIndexOwningFolder(runs, "  "); got != -1 {
		t.Errorf("blank folder: index = %d, want -1", got)
	}
}
