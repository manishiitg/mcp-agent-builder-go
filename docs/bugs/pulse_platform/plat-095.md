[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-095 — "is this message actually done" was answered by two different copies of the same rule, and a third mechanism the rule never reaches

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `implemented` — busy-signal reconciliation consolidated in Go, plus a follow-up fix for the global-monitor gap this investigation found; runtime reverify pending |
| Last synchronized | `2026-08-12` |

- **Priority:** P1 — every scheduled message in the platform depends on this
  decision; a wrong answer either drops real work (PLAT-071/094) or shows a
  stale "running" badge over a session that already finished.
- **Owner:** scheduler turn-completion checks (`scheduler.go`), runtime
  snapshot classification (`runtime_coordinator.go`), frontend runtime status
  (`frontend/src/utils/runtimeActivity.ts`)
- **Follows:** [PLAT-071](plat-071.md) (workshop idle-wait fix) and
  [PLAT-094](plat-094.md) (the same fix at the Pulse step boundary) — this
  ticket is what happens when you ask "why do we have this fix in two places
  and not one."

## What was actually duplicated

Auditing every place the platform decides "has this scheduled turn finished"
found **three distinct mechanisms**, not two shared ones:

1. **Workshop turns** (`waitForWorkshopIdleWithInactivityTimeout`,
   `scheduler.go:4052`) — a polling loop. Declares done from the runtime
   snapshot phase (`sessionIsBusy`), and only consulted the explicit per-turn
   flag (`isSessionBusy`) as a tie-breaker at timeout (PLAT-071). Used by
   schedule messages, contract-upgrade preflights, the PLAT-093 decision
   drain, step-based-workflow steps, and Chief-of-Staff task reports.
2. **Pulse turns** (`waitForPulseTurnCompletion`, `scheduler.go:3945`) — a
   different mechanism entirely: subscribes to the session's event stream and
   declares done when it goes quiet for a settle window with no live child
   work. It does not consult `sessionIsBusy`/`isSessionBusy` to decide the
   turn is over at all.
3. **Pulse's post-wait recheck** (`abortIfTurnStillBusy`, PLAT-094) — runs
   *after* #2 says done, and re-checks the same two busy signals before
   allowing the next message — using its own independent copy of the exact
   reconciliation rule already inline in #1.

So the reconciliation rule (PLAT-071's fix) existed twice, hand-written
separately, only because Pulse's completion check (#2) doesn't route through
the same code as everyone else's (#1).

## Fix shipped: one shared reconciliation function

`reconcileSessionBusySignal(snapshotBusy, explicitBusy bool) bool`
(`scheduler.go:2440`, renamed from PLAT-094's `reconcilePulseStepSessionBusy`)
is now the single implementation, called from both:

- `abortIfTurnStillBusy` (Pulse step boundary), unchanged call shape.
- `waitForWorkshopIdleWithInactivityTimeout`'s timeout branch, which
  previously hand-rolled `if !s.api.isSessionBusy(sessionID) { return nil }`
  inline — replaced with a call to the shared function.

Deliberately asymmetric, same as PLAT-071/094: the explicit flag only ever
corrects a snapshot *claiming* busy; an idle snapshot is never escalated to
busy. Two independent, hand-rolled copies of an asymmetric rule are exactly
the shape that drifts silently — the next person patching one copy has no
reason to know the other exists.

**Deliberately not merged**: mechanisms #1 (polling) and #2 (event
subscription) stay separate. That's a real implementation-strategy
difference — Pulse turns can run for hours and event-driven waiting avoids
polling overhead — not the same duplication as the reconciliation rule.
Collapsing them was scoped out explicitly: higher risk, touches every
scheduled message in the platform, and this file has multiple concurrent
editors at any given time.

## Follow-up fix shipped: `ActiveWorkflowExecution` was missing the collapsed status entirely

Chasing the frontend finding below turned up something more concrete than a
"same bug class, be careful" note. `ActiveSessionInfo` (the model behind
polling/SSE/the execution tree) has always computed and shipped a pre-collapsed
`DisplayStatus` (`busy`/`idle`/`stopped`) alongside its raw `RuntimeState`, from
one `snapshot` value in one call — `polling.go:440-443`:

```go
if snapshot, ok := api.authoritativeRuntimeSnapshot(session.SessionID); ok {
    enriched.RuntimeState = &snapshot
    enriched.DisplayStatus = sessionDisplayStatusFromRuntime(snapshot).Status
}
```

Because both come from the same value, they can never disagree — the
frontend's own `runtimeDisplayStatus()` re-deriving the identical mapping from
`runtime_state.phase` is redundant, but not a live bug (verified: the two
mappings agree today).

`ActiveWorkflowExecution` — the model behind `/api/workflow/running`, the
endpoint the Global Monitor actually reads for workflow-level busy state — had
**no `DisplayStatus` field at all** (`workflow.go:598-624`). All three call
sites that attach its `RuntimeState` shipped only the raw 7-state `Phase`,
with no collapsed answer available — the one API surface where a consumer is
*forced* to re-derive busy/idle/stopped from scratch, because nothing
authoritative was ever offered.

Fixed by adding the field and populating it the same way, at all three sites:
`workflow_execution_tracker.go:463` (`listRunningWorkflowExecutions`),
`workflow_running_routes.go:87` (`handleGetRunningWorkflow`), and
`workflow_running_routes.go:157` (`handleUpdateRunningWorkflow`). Mirrored
into the frontend type (`RunningWorkflowInfo` gained `runtime_state` — which
the Go struct had always sent but the TS type never declared — and
`display_status`) so it's actually usable from TypeScript.

## Open finding: the same bug class exists in the frontend, but the naive fix is wrong

The Global Activity Monitor (`frontend/src/utils/runtimeActivity.ts:5`,
`runtimeDisplayStatus`) classifies a session busy/idle purely from
`runtime.phase`, never consulting `runtime.foreground_turn.busy` — which is
already present in every payload
(`snapshot.ForegroundTurn.Busy = api.isSessionBusy(sessionID)`,
`runtime_coordinator.go:376`). So the UI can show "running" over a session
whose foreground turn has already ended: the same class of staleness as
PLAT-071/094, one layer further out.

**It cannot be fixed the same way.** `deriveRuntimePhase`
(`runtime_coordinator.go:447`) does not compare two views of one fact — it
ORs four independent live-evidence signals: foreground-turn busy, a running
child execution, a live background agent, or a busy terminal. Any one alone
is enough to call it "running", including states with **no foreground turn at
all** — "waiting for background agents" is a real, intentionally distinct
state the frontend already branches on elsewhere
(`hasLiveBackgroundAgents` in `globalActivityMonitorStatus.ts`). Reconciling
`phase` against `foreground_turn.busy` alone, the same two-signal way as the
Go fix, would misclassify every one of those as idle — trading a known
staleness bug for a real, immediate regression.

What actually explains a stuck "running" phase is more likely one of the
*other three* OR'd inputs going stale — `BackgroundLive` or `TerminalBusy`
staying true after the real work ended is exactly the shape PLAT-091 already
fixed for one specific input (orphaned evaluation-step background agents).
The generic fix for "is Phase trustworthy" is making each of the four inputs
self-correcting, the way PLAT-091 did for one of them — not adding a second
reconciliation layer on top of an OR, which would just be a third
reimplementation of the same rule in TypeScript, on top of the two already
merged into one in Go.

## Not fixed here

- Whether `collectRuntimeSnapshot`'s read of `RawSessionStatus`
  (`snapshot.RawSessionStatus = active.Status`, `runtime_coordinator.go:363`)
  can race a concurrent completion writer — i.e., whether a stale in-memory
  `ActiveSessionInfo.Status` read is what actually explained PLAT-094's
  incident, given `ForegroundTurn.Busy` (`isSessionBusy`) had *already*
  cleared by the time the abort fired, and per `deriveRuntimePhase`'s OR
  logic, that alone should not have kept Phase "running" unless one of the
  other three evidence fields was also stale at that instant. Not
  investigated this pass — the PLAT-094 fix is safe regardless of which input
  was actually stale, since it corrects at the point of use, but it does not
  explain the underlying cause any more than PLAT-065 or PLAT-094's own "not
  fixed here" sections did.
- `BackgroundLive`/`TerminalBusy` staleness beyond PLAT-091's specific fix
  (orphaned evaluation-step children). Whether other background-agent or
  terminal shapes can go stale the same way is unaudited.
- The frontend Global Activity Monitor itself. No code change shipped there;
  see the finding above for why a same-shape fix is unsafe.

## Verification

- `go build ./...` clean.
- `TestReconcileSessionBusySignal` (renamed from PLAT-094's
  `TestReconcilePulseStepSessionBusy`, same cases, doc comment updated to
  note it now backs both call sites) and
  `TestAbortIfTurnStillBusyReclassificationChangesTheOutcome` still pass
  unchanged.
- Full existing `waitForWorkshopIdle*` test suite (10 tests, including
  `TestWaitForWorkshopIdleTimesOutWhenSessionStaysBusy` and
  `TestWaitForWorkshopIdleAllowsLongRunningTmuxWithProgress`, which exercise
  the exact branch rewired to call the shared function) passes unchanged —
  proving the substitution is behavior-preserving, not just compiling.
- No new integration test was added for the workshop-idle-wait's use of the
  shared function specifically: the test harness couples `sessionIsBusy` and
  `isSessionBusy` together through `setSessionBusy` +
  `observeRuntimeSnapshot`, so forcing the two signals to disagree inside a
  full timing-loop test would need separate test infrastructure. Coverage
  instead comes from the pure-function table test (proves the rule) plus the
  unchanged integration suite (proves no regression in the common paths).
- `TestRunningWorkflowListCarriesTheCollapsedDisplayStatus` (new): pins that
  `listRunningWorkflowExecutions` populates `DisplayStatus` from the same
  snapshot as `RuntimeState`, and that the two can never disagree. Verified to
  fail (with a nil-vs-populated mismatch) when the assignment is neutered, and
  pass with it restored.
- `npx tsc --noEmit` clean after the `RunningWorkflowInfo` type additions.
- **Not yet reverified live.**

## Acceptance

- Exactly one function decides whether a stale runtime-snapshot busy signal
  should be trusted over the explicit per-turn flag, and both places that
  make this decision (the workshop idle-wait, the Pulse step boundary) call
  it.
- The frontend/global-monitor finding is documented with a stated reason it
  was not fixed the same way, not silently dropped.
