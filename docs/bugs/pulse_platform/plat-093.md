[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-093 — answered decisions were applied after the run, so the run they were meant to change had already happened

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `implemented` — pre-run decision drain shipped; runtime reverify pending |
| Last synchronized | `2026-08-12` |

- **Priority:** P1 — the operator's decision lands a full cycle late, and for a
  cadence decision that means the problem they asked to fix repeats every run
  until then
- **Owner:** scheduler pre-run turn sequence (`scheduledWorkshopTurns`,
  `executeWorkshopJob`)
- **Follows:** [PLAT-092](plat-092.md), which stopped answered decisions being
  silently re-read forever but drained them in the **post-run** Pulse pass

## The gap PLAT-092 left

PLAT-092 gave Review+Fix an explicit drain contract. That fixed *whether*
decisions get applied, but not *when*: Pulse runs after the workflow, so an
approved change lands on the next cycle at the earliest. The run that was
supposed to benefit had already executed on the old behavior.

The stranded backlog shows why that ordering is wrong. **24 of the 26 answered
decisions require an action, not an acknowledgement** — only 2 are pure
rejections. And what they change is what a run *does*:

- `ops-hourly-cadence-overrun-2026-08-05` → `evals-once-daily`. Tectonicus's
  hourly runs overrun the hour, so the operator gets one snapshot a day instead
  of six. Applied post-run, the overrun repeats every hour until the next Pulse.
- `plan-proposal-panel-label-and-benchmark-backfill` → `approve-both`. Switches
  the scoring unit from R-multiples to percent-vs-benchmark. Applied post-run,
  the run that just finished was still scored the way the operator said was
  "stacked against the very trades it is judging".
- Others: `activate`, `turn-on`, `provide_token`, `B-rank-based`,
  `graded_all_three`, `recent_anchor`, `approve-v2`, `approve_rebuild`.

## Why "try, don't classify"

There is no `apply_timing` field, and adding one — or inferring pre/post from
keywords — would be guesswork. Theme-based classification has already misfired
here: the PLAT-072 triage sweep matched 11 of 81 findings and misattributed
several.

It is not needed, because the asymmetry is one-sided. If a decision changes
what the run does, applying it before the run is strictly better. If it does
not, applying it before the run is harmless. There is no case where waiting is
better. So the drain attempts everything at the earliest opportunity, and the
exceptions identify themselves at attempt time: needs evidence from a run that
has not happened, premise went stale, or intent is ambiguous. **The attempt is
the classification.**

## Fix shipped

`scheduledDecisionDrainTurn` builds a blocking pre-run turn, inserted by
`executeWorkshopJob` **after** any contract upgrade and **before** the first
schedule message. It only exists when there are answered decisions, so an
ordinary run pays nothing.

Applying is an agent turn, not Go, for a concrete reason: the decision's
`context` field holds a prose "what happens next if you approve" section
written for the operator, not a machine-readable patch, and the typed plan,
config, eval and schedule tools it needs already exist. The contract-upgrade
preflight is the proven precedent for mutating plan artifacts before the first
schedule message.

Ordering is deliberate — an upgrade can rewrite the very artifacts a decision
edits, so upgrades go first.

Safety boundaries, all pinned by tests:

- **It cannot cost the run.** If the drain fails to start or fails to settle,
  the scheduler logs it and continues to the schedule message. The decisions
  stay answered-and-unapplied, exactly as before this turn existed, and
  PLAT-092's post-run drain still sees them.
- **It carries the operator's actual answer**, not just the decision id, so a
  rejection cannot be applied as an approval.
- **It must not consume what it did not apply** — consuming to tidy the list
  would erase a decision while recording that it was honored.
- **It must not run the workflow**, execute steps, back up, publish or notify.
- **Staleness is checked** against the decision's own `run_id` and
  `answered_at` versus the current plan, so a month-old proposal is not blindly
  applied to a plan that has moved. 21 of the 26 carry a `run_id`; the 5 that
  do not have no staleness anchor and should be read rather than auto-applied.

## Not fixed here

- The 26 existing stranded decisions are still stranded. The next scheduled run
  of each workflow will now attempt them, which is the intended path, but
  several are month-old proposals whose premise may not hold — expect some to
  be left with a reason rather than applied.
- Whether `goal_advisor` should be un-suppressed. It remains the designated
  handler for its own answered decisions and is still off.

## Verification

- `go build ./...` clean.
- Four new tests (`plat093_decision_drain_preflight_test.go`): no turn when
  nothing is pending; the operator's selected option travels with each id; the
  prompt forbids running the workflow and forbids consuming what was not
  applied; and the turn sits after upgrades and before schedule messages.
- Full suite: 24 failures, all accounted for (22 known baseline + 2 from
  another session's in-flight work), zero unexplained.
- **Not yet reverified live** — needs a restart, then a scheduled run on a
  workflow that has answered decisions waiting (tectonicusadaytrading has 8).

## Acceptance

- A decision the operator answered is applied before the next run of that
  workflow, not after it.
- A drain that cannot apply a decision leaves it with a stated reason and never
  fails the run.
- An ordinary run with no answered decisions runs no extra turn.
