[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-214 — `record_pulse_finding` intermittently reported a false-negative "could not be reloaded" error caused by a separate-connection reload race

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `fixed` |
| Last synchronized | `2026-08-29` |

- **Priority:** P2 — an agent cannot trust the tool's own success/failure
  signal for issue-updating calls, per the finding's own impact note: *"This
  risks either duplicate/redundant reattempts... or, worse, an agent
  concluding a real repair/finding was never recorded and skipping
  downstream steps that depended on it."*
- **Owner:** `agent_go/pkg/orchestrator/agents/workflow/step_based_workflow/pulse_finding_details.go`
  (`RecordPulseReviewFinding`).
- **Related:** `harness:record_pulse_finding` (`Workflow/ICICI-BANK-PARSING`,
  medium) — the finding this fixes.

## The finding

`record_pulse_finding` calls updating an **existing** `issue_id`
intermittently returned `success:false` with *"recorded Pulse finding could
not be reloaded by its internal lifecycle identity,"* even though the
underlying write had actually succeeded — confirmed durably visible in a
`get_pulse_state(view="backlog")` read moments later. 3 of 7 structurally
identical calls in the same session failed this way; the other 4 succeeded
cleanly with no error.

## Root cause — confirmed via code read

`RecordPulseReviewFinding` opens one database handle (`db`) at the top of
the function and performs every write on it. Its final step — reloading the
just-written finding to confirm the write landed and return the current
`issue_id`/status — called the *public* `LoadPulseFindingLifecycles`, which
opens its **own, separate** database connection (`openRunConcernsDB`) rather
than reusing `db`. A fresh connection opened immediately after a write on a
different connection is exactly the shape of a cross-connection SQLite
visibility/locking race: no fixed reproduction trigger, intermittent by
timing, and exactly what the finding's own limitations section named as the
likely mechanism (*"recommend the platform owner check the reload-after-write
path for a race or serialization inconsistency"*).

## Fix

Replaced the `LoadPulseFindingLifecycles(ctx, workspacePath, "", -1)` call
(a full, sorted, cross-joined backlog scan on a brand-new connection — far
more than this check actually needs) with a direct, targeted
`SELECT issue_id, status FROM run_concerns WHERE fingerprint=?` on the
**same** `db` handle the write itself used. Reading back on the exact
connection that performed the write removes the cross-connection race
window by construction — SQLite guarantees a connection sees its own
committed writes — and is simpler and cheaper than the query it replaced,
since this check only ever needed the one row it was about to confirm, not
the whole backlog.

## Explicitly not done

- Did not change `LoadPulseFindingLifecycles` itself, or any of its other
  call sites — it remains correct for its actual purpose (a sorted,
  cross-joined backlog view for reviewers/UI), just not reused here where a
  same-connection point lookup is both more correct and cheaper.
- Did not write a new test attempting to force the cross-connection race
  directly — it is fundamentally a timing race between separate SQLite
  connections, not something a sequential single-goroutine unit test can
  reliably reproduce. Relied instead on the existing
  `TestPulseFindingIssueIDUpdateReloadsExistingStepFindingAcrossReviewerModule`
  (which already exercises the exact "update an existing issue_id" shape
  end-to-end) continuing to pass as confirmation the replacement query is
  behaviorally correct.

## Verification

- `go build ./...` clean.
- `TestPulseFindingIssueIDUpdateReloadsExistingStepFindingAcrossReviewerModule`,
  `TestPulseFindingIssueIDUpdatesOneRootCauseAndMergePreservesDuplicateHistory`,
  `TestWorkflowObservationBecomesIssueOnlyWhenReviewerPromotesIt`, and the
  two `TestPulseFindingLifecycleCloses*` tests all pass unchanged — the
  exact suite most likely to catch a regression in this reload path.
- Full `pkg/orchestrator/agents/workflow/step_based_workflow` suite passes.
- Full suite: 3 pre-existing failures before and after (`cmd/server/guidance`,
  unrelated content), no regression.
- Not yet reverified live — this is a timing race; a real recurrence (or
  its absence) on ICICI-BANK-PARSING's next Pulse pass is the eventual
  confirmation, not something obtainable from this vantage point.
