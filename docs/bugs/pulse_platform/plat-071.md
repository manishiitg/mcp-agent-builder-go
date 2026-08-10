[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-071 — an idle-wait timeout is treated as proof the workflow never ran

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `implemented` — both the record corruption and the stall's proximate cause; underlying phase-staleness still open |
| Last synchronized | `2026-08-10` |

- **Priority:** P1 — Pulse's durable history understates successful runs, and terminal command states are immutable
- **Owner:** scheduler workshop turn loop (`scheduler.go`)
- **Found on:** social-media, 2026-08-10, session `schedule-cron--5227790a_1786354253197815000`
- **Reported by:** the workflow's own Pulse finalizer, which escalated rather than acting — correctly, since this is scheduler behaviour outside its authority

## What happened

```
09:30:53Z  scheduler starts the session
09:34:45Z  workflow run starts        (runs/iteration-0/default)
12:15:18Z  workflow run COMPLETES     run_metadata.json status: "completed"
12:49:35Z  scheduler declares "workshop idle wait timed out ... (live_child_work=false)"
```

The run **succeeded**: 19 actions landed (1 post, 8 replies, 6 likes, 4 follows), 18/19 independently verified, evaluation scored 17/40, 238 artifact files written. The scheduler recorded the whole invocation as an error **34 minutes after the work had finished**, and Pulse was then told the workflow had not run at all — so `publish` was `skipped: no Pulse Gate/reviewer/Fixer...` and `notify` stalled at `waiting`.

The workflow's own backup record states it plainly:

> the Pulse Finalizer for this run_id was invoked with the premise 'workflow did not run', which is factually incorrect — the execution route completed in this session... The scheduler's 10m idle-wait watchdog fired because the builder turn was waiting on step notifications, not because the workflow failed to start.

## Root cause of the record corruption

`ProducedRunEvidence` is initialized `false` (`scheduler.go:2796`) and only computed near the end of the turn loop (`:2939`). The idle-wait failure returns at `:2875`, **before** that computation. So the flag keeps its initialized value and `reviewEvidenceAvailable` (`:2270`) reads false — not because anything examined the run, but because the wait expired.

The durable record said `completed` the entire time. Nothing consulted it.

## The watchdog itself was not wrong

The reporting agent framed this as *"the watchdog is misdiagnosing healthy runs as non-starts."* The harm is real and its evidence is sound, but the mechanism is different and worth stating precisely, because it changes the fix.

`live_child_work=false` was **accurate**. The workflow finished at 12:15:18; by the time the wait expired there genuinely was no live child. PLAT-054's liveness predicate did its job. Raising the timeout would not help either — the session sat idle for 34 minutes after completing, so a longer window only delays the same wrong conclusion while making genuine stalls slower to detect.

Two distinct defects are involved:

1. **The parent turn never resumed after the workflow completed.** Its own note says it *"was waiting on step notifications"*. This is PLAT-067's family — a child finishes, the parent never advances — and remains **open**.
2. **A timeout is allowed to overwrite a durable success record.** Independent of (1), and the more damaging of the two, because Pulse's finalized command states are immutable: a false "did not run" is permanent. Fixed here.

## Fix shipped (defect 2)

The idle-wait failure path now consults the durable record before returning, and preserves run evidence when a run actually started during this invocation. The run still fails — the session genuinely stalled, and that should stay visible — but Pulse is no longer told the workflow never ran.

It uses a new baseline-free helper, `workshopRunStartedDuringInvocation`, rather than the existing `workshopRunProducedEvidence`. That distinction matters: the existing helper asks "is this folder new?" first, which is right when it holds a real baseline but inverts when it does not — an empty baseline from a failed listing (PLAT-070, same day) makes the first folder look new and returns true unconditionally. On this path the narrower question — *did a run actually start?* — has a trustworthy answer needing no baseline. A record with no usable timestamp counts as nothing, so an unreadable record can never manufacture evidence.

Tests pin both directions: a pre-existing `iteration-0` reused by this invocation counts as evidence with no baseline at all, and older or unstamped runs never do.

## Fix shipped (defect 1)

Traced from the log. The turn did not hang — it **finished**, and the watchdog could not tell:

```
18:08:36  eval completion steered into the session
18:09:30  [BG AGENT] Synthetic turn completed (211 messages)
18:09:30  [ACTIVE_SESSION] status -> completed
          ...ten minutes of nothing at all...
18:19:35  idle wait expired (live_child_work=false)
```

A genuinely idle session is detected in about six seconds (two consecutive not-busy polls). Timing out instead means `sessionIsBusy` kept returning **true** for ten minutes after the turn's own completion was recorded.

There are two independent notions of busy and they disagreed. `sessionIsBusy` reads the runtime snapshot `Phase`; `isSessionBusy` is the explicit per-turn flag set at turn start and cleared at turn end. The snapshot phase stayed busy while the session's own lifecycle status said `completed`.

**This is the same signature as PLAT-065** — there too, `sessionIsBusy` stayed true after the Gate turn's tool call had durably succeeded, causing `abortIfTurnStillBusy` to kill the remaining Pulse stages. Both are very likely one underlying defect: the runtime snapshot phase not returning to a non-busy state when a turn ends.

The fix does not touch that state machine. At the point where the loop is already about to declare a timeout — nothing running, nothing progressed for the whole inactivity window — it now also consults the explicit per-turn flag. If that flag is clear, the only signal still claiming work is the snapshot phase, contradicted by everything else, and the turn is treated as finished. A genuine stall keeps its flag set and still times out. A diagnostic line records both signals when it does, so the next real stall is fully explained.

## Still open

- **The underlying phase staleness**: why the runtime snapshot `Phase` fails to leave busy when a turn completes. This fix reconciles the symptom at one call site; PLAT-065's `abortIfTurnStillBusy` reads the same stale signal and is not covered by it.
- **Historical records stay wrong.** Terminal Pulse command states are immutable by design, so the two runs already recorded as non-runs cannot be corrected. This ticket stops new ones; it does not repair the history. The reporting agent noted an identical earlier occurrence, making this at least the second.
- **Recurrence count.** The reporting agent called this "the eighth idle-timeout in this series". Not independently verified here — the log was truncated by the 12:26 restart — but the two corroborated occurrences are enough to treat it as recurring rather than incidental.

## Acceptance

- A run that completes and then stalls is still reported as an error, but Pulse receives run evidence and reviews it instead of skipping on "did not run".
- Evidence is never manufactured when the run record is missing or unstamped.
- Defect 1 tracked separately; closing this ticket does not close the stall.
