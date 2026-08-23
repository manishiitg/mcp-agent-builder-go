[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-182 — a step's re-read result was selected by max attempt/iteration number, letting a stale dispatch from weeks earlier outrank today's real result

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `fixed` — live reverify pending |
| Last synchronized | `2026-08-23` |

- **Priority:** P1 — a completion notification can report a stale, unrelated
  failure from weeks earlier as if it were the current run's result, with no
  indication anything is wrong; whoever reads it has no way to tell without
  independently checking the actual workspace files.
- **Owner:** step result re-read path
  (`pkg/orchestrator/agents/workflow/step_based_workflow/controller_execution.go`,
  `controller_workshop.go`).
- **Related:** [PLAT-176](plat-176.md) (same root shape — evidence filenames
  built from counters local to one dispatch — but a different consumer of
  that shape: PLAT-176 was about overwriting; this is about selecting the
  wrong survivor once files from multiple dispatches coexist).

## Symptom

A background agent labeled "Master Reel Orchestrator" (actually just the
plan step's own `Title`, re-run via the workshop `execute_step` tool) sent
an `[AUTO-NOTIFICATION]` for the "instagram" workflow (2026-08-23) reporting:

```
CONCERNS: Validation history: attempt 1 failed - ... content_package.json
must exist but was not found ...; attempt 2 failed - [same error]; attempt 3
failed - [same error]; unresolved after 3 attempt(s).
```

The referenced file genuinely existed, written today at 00:51. Today's
dispatch had in fact succeeded on its first try.

## Root cause, confirmed by direct code read and file inspection

`loadSingleStepResultFromLogs`
(`controller_execution.go:1018-1037`, before this fix) scans every
`execution-attempt-<N>-iteration-<M>.json` file present in a step's log
folder and keeps whichever has the numerically **highest `(attempt,
iteration)` pair**:

```go
if attempt > latestAttempt || (attempt == latestAttempt && iteration > latestIteration) {
    latestExecutionResult = execResult
    latestAttempt = attempt
    latestIteration = iteration
}
```

`attempt`/`iteration` are counters **local to one dispatch** — both reset to
1/0 every time the step is dispatched again (the exact same shape PLAT-176
already documented for the sibling overwrite bug). Across dispatches they
carry no time ordering whatsoever; nothing archives, clears, or otherwise
retires a prior dispatch's files once a step is re-run weeks later.

Live, for `step-create-reel` under `Workflow/instagram/runs/iteration-0/test-run`:
- `execution-attempt-1-iteration-0.json` — **2026-08-23 00:52:04**, today's
  real, successful dispatch (`execution_result: "STATUS: COMPLETED"`, read
  directly).
- `execution-attempt-2-iteration-0.json` / `execution-attempt-3-iteration-0.json`
  — both **2026-07-17**, a genuinely different, genuinely-failed dispatch
  from over a month earlier. Its `execution_result` field, read directly,
  contains the exact text later reported as today's failure, byte-for-byte:
  `"CONCERNS: Validation history: attempt 1 failed - ... content_package.json
  must exist but was not found ...; attempt 2 failed - ...; unresolved after
  3 attempt(s)."`

Because `3 > 1`, the July file won the comparison even though it is 37 days
older and describes an entirely different, already-resolved outcome.

**Forward path to the notification** (traced and confirmed):
`controller_workshop.go:305-317` deliberately discards the correct in-memory
result from `runExecutionPhase` and re-reads from logs via
`loadSingleStepResultFromLogs` — even when the in-memory execution itself
returned no error — because, per the existing code comment, "runExecutionPhase
writes results to log files." That re-read is what returned the stale July
text. From there: `interactive_workshop_manager.go:3246` →
`OnExecutionComplete(execID, stepDisplayName, result, ...)` →
`delegation.go:283` → `agent.SetResult(result)` →
`background_agents.go:1892`'s `buildAutoNotificationMessage` uses
`snap.Result` verbatim as the notification's `Result:` text.

**Explicitly ruled out**, with citation, so this isn't left as a
plausible-sounding but unverified theory: `RecordRunConcerns`'s durable
cross-run concern aggregation (`run_concerns.go`) is never folded into this
text — its only consumer that formats a history-style line is
`run_concerns.go:571-572` ("seen on N runs"), a different string shape
entirely, used by Pulse worklist/loop-closure code, not this notification
path. `controller_execution.go`'s own live retry loop
(`withValidationFailureConcern`) genuinely produced nothing for today's
dispatch — confirmed by the fact only one `pre_validation_final-gate_*`
file exists for today, proving that loop succeeded on its first live
iteration.

## Fix implemented

`loadSingleStepResultFromLogs` now compares each candidate file's
`completed_at` field (present in every execution-attempt file; wall-clock
time, genuinely comparable across dispatches, unlike the local counters)
and keeps whichever completed most recently. The `(attempt, iteration)`
comparison is retained only as: (a) a tie-breaker when two files share the
identical `completed_at`, and (b) the sole ordering signal for legacy files
that predate `completed_at` being recorded, so no existing evidence becomes
unreadable.

## Not implemented

- Nothing archives or clears a step's prior-dispatch `execution-attempt-*`
  files once a step is re-run — this fix changes which survivor gets
  *selected*, not whether stale ones keep accumulating on disk indefinitely.
  A future pass could extend PLAT-176's `superseded/` archival to this read
  path's source files too, but that's a larger change than fixing the
  selection bug itself and wasn't attempted here.

## Acceptance tests

Covered by `controller_execution_stale_result_test.go`, both proven
fail-before/pass-after against the actual prior comparison logic:

1. `TestLoadSingleStepResultFromLogsPrefersRecentCompletionOverHigherAttemptNumber`
   — three fixture files reproducing exactly this incident's shape (today's
   attempt-1 success alongside a July attempt-2/attempt-3 failure); asserts
   the result returned is today's, not July's. Confirmed failing against the
   prior max-`(attempt, iteration)` comparison, passing against the fix.
2. `TestLoadSingleStepResultFromLogsFallsBackToAttemptOrderingWithoutTimestamps`
   — legacy fixture files with no `completed_at` field; asserts the
   attempt/iteration fallback ordering still applies unchanged.

## Verification

- `go build ./...` clean.
- `go test ./pkg/orchestrator/agents/workflow/step_based_workflow/...`: both
  new tests pass; the two pre-existing failures
  (`TestRunInBackgroundPassesBuilderSkillSnapshotToBothAgentKinds`,
  `TestWorkshopPromptShellExamplesUseAbsolutePaths`) confirmed present on a
  clean `origin/main` checkout before this change, unrelated to this fix.
- Live reverify pending: no confirmation yet against a real re-dispatched
  step whose log folder already holds files from an earlier, unrelated
  dispatch.
