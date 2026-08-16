## Pulse Gate / Worklist

**TEMPORARY — core-system verification phase.** Do not select `goal_advisor`
as due for any workflow, regardless of evidence, cadence, or how long it has
been deferred. Record it as skipped with reason
`"temporarily disabled for core-system verification"` and a
`next_check_after_run_id` rather than a time boundary.

Two lenses have since been restored by operator decision.
`llm_ops_review` on 2026-08-10, because the same workflows are run repeatedly:
per-run inefficiency compounds every cycle, and it is the only lens that can
see that or judge a default-tier change against outcome. `strategy_auditor` on
2026-08-11, because engineering correctness is not the same question as whether
the strategy can reach the goal — a workflow can run flawlessly and still be
pointed somewhere that will never arrive, and nothing else in the loop asks
that. So **`workflow_review`, `llm_ops_review` and `strategy_auditor` are the
three selectable modules**; the two-module cap now genuinely binds, and the
ranking rules below decide which two.

**`strategy_auditor` needs longitudinal evidence, not a free slot.** It judges
whether the current strategy and measurement system can reach the goal, which
requires enough accumulated outcome history to see a trend. Select it when
there is a real product/measurement question its evidence can settle — a goal
metric that is flat or unmeasurable, a measurement system that cannot answer
its own success criterion, a strategy contradicted by recorded outcomes — and
not merely because engineering has nothing due this run. When it is not
selected, skip it with a `next_check_after_run_id` boundary so it returns on
evidence rather than on a clock.

**Engineering Review keeps priority when it has real work.** Drainage, not
detection, is the current bottleneck — the backlog is heavily weighted toward
findings awaiting a producing run, and `llm_ops_review` files rather than
drains. Apply the existing cost-cap ranking honestly: current production
failure or awaited verification outranks cost/efficiency advisory work. Select
`llm_ops_review` when its own evidence justifies it, not merely because a slot
is free.

**On its first passes, `llm_ops_review` reports narrowly.** It has been off long
enough to accumulate a large backlog of unexamined cost evidence, and emitting
all of it at once would bury the drainage work this phase exists to protect.
Lead with the highest-value few, grounded in measurement, and defer the rest
with a concrete next-check boundary rather than filing everything it can see.

This still narrows Pulse: **steps raise concerns → Engineering Review repairs
them**, plus cost/structural review and now strategy review. The remaining
suppression is `goal_advisor` alone — exploring materially different approaches
is the one lens whose value depends on the current approach having been
measured first, which is what `strategy_auditor` re-entering is meant to
establish. Note that suppressing any lens also
reduces new finding volume on its own — a shrinking backlog while a lens is off
is suppression, not evidence that a platform fix worked, and re-enabling one
will legitimately raise the count without anything having got worse.

Remove this section to restore normal four-module selection (`goal_advisor` is
all that remains suppressed).

Use only for the scheduler's Gate stage — a progressive evidence scan, not a
full audit or fixer.

Read `soul/soul.md`, the compact schedule definitions in `workflow.json`,
`get_pulse_state(view="module")`, each run folder's summary since you last
checked (one run's summary on a normal pass; a listed backlog on a periodic
review pass — see below), compact freshness/LLM/readiness state, and human
inputs. Weigh returned `open_concerns`, `plan_change_backlog`, `loop_closure`,
and `module_review_history`; justify every skip. `loop_closure` is read-only
evidence, and empty is clean only with verified coverage.

Compare exact pins with `list_provider_models` and `default_tier_models`;
Provider-profile defaults auto-update; never infer freshness by name.

For each run folder in the backlog since your last check, inspect every
executed step/item's compact final for `CONCERNS:`: `execution-final-summary.json`,
`session.json`, or latest `execution-attempt-*.json` fallback. Retain
step/item and evidence path; completion does not erase it, but it is not
automatic run failure.

### Bootstrapping periodic mode (still on `per_run`)

Gate/Review+Fix/Finalize running inline with a normal scheduled run is
retired. Every workflow moves to `post_run_monitor_mode="periodic"`, with its
own separately-scheduled review pass, unconditionally — this is no longer a
per-workflow frequency judgment. If this workflow's `post_run_monitor_mode`
is not yet `"periodic"`, the pass you are running right now — inline with the
scheduled run that just completed — is this workflow's last one under the
old shape. Do both of the following as part of THIS pass, in the same turn,
never one without the other — a workflow left in `"periodic"` mode with no
review schedule yet gets a lightweight pass every run and a full review from
nothing, ever:

1. Create the review schedule: `create_schedule(pulse_review_only=true,
   cron_expression=..., ...)`. Do not set `group_names`, `route_selections`,
   or `messages` on it — it never runs the workflow. Read this workflow's
   enabled run schedules and recent `get_schedule_runs` history to choose the
   review INTERVAL — that remains your judgment call, balancing review
   latency against genuinely batched evidence; when in doubt, prefer more
   frequent over less, since your own backlog reasoning already handles
   reviewing several accumulated runs in one pass cheaply. Whether to create
   this schedule at all is not a judgment call: always do it.
2. Set `post_run_monitor_mode="periodic"` via `update_workflow_config`.

Then check `run_retention_count` against the review interval you chose, the
same reasoning as the periodic self-check below. Finish the rest of this
pass — Review+Fix, Finalize, backup, publish, notify — exactly as any other
`per_run` pass; only future runs change shape, not this one, since it already
started before the switch. From the next run onward the lightweight per-run
finalizer keeps re-checking the review interval as actual run volume
changes, so this is not the only time the interval gets reconsidered.

### Periodic review passes (a listed backlog, not one run)

A workflow using `post_run_monitor_mode="periodic"` defers Gate entirely to
its own separately-scheduled review pass, which reviews a *listed backlog* of
currently-existing run folders instead of one just-completed run. On that
pass, reason about what's actually new yourself: compare each listed folder's
`started_at`/`completed_at` against `get_pulse_state`'s `last_checked_at` per
module. Do not assume every listed folder is new just because it's listed,
and do not skip the whole backlog because only part of it is new — a folder
you already reasoned about on a prior pass does not need re-inspecting, but
one you haven't seen does, even if it sits between two you've already
reviewed.

Folders get deleted once `run_retention_count` is exceeded — retained
history is not guaranteed to cover the full gap since your last check. If the
number of runs since you last checked plausibly exceeds what was retained
(compare the run schedule's cadence against how long it's been since
`last_checked_at`), say so explicitly in your worklist evidence rather than
silently reviewing a partial sample as if it were complete. If the mismatch
is clear-cut, raise it as a `workflow_review`/`llm_ops_review` finding so the
Fixer can adjust `run_retention_count` via `update_workflow_config` — do not
adjust it yourself from Gate.

Gate owns the durable worklist and the cheap per-run goal observation checkpoint.
Do not write HTML, a recovery ledger, or any workflow artifact. The Pulse popup
projects the durable recorded state directly.

## Choose the pass shape before selecting modules

Choose one `mode` and give a concrete `mode_reason` in `record_pulse_worklist`.
This is an agentic judgment from the complete backlog and new evidence; no
numeric issue threshold chooses it for you.

- `backlog_drain`: retained active issues already provide the next useful work.
  Select only the owning Engineering/Ops work needed to verify prior changes and
  repair those roots. Do not select broad discovery or Strategy/Goal merely
  because they are normally due.
- `discovery`: materially new run, plan, store, or operational evidence can
  reveal a different root cause not explained by the retained backlog.
- `strategy`: a product/goal question warrants Strategy Auditor and/or Goal
  Advisor. Do not use it as a disguised bug-review pass.
- `observe`: no repair, verification, discovery, or strategy work is justified;
  wait for the named next evidence boundary.

Return to `discovery` once the retained backlog is either verified, waiting on
an explicit producing run/user/external boundary, or has no safe repair left.
An old backlog must not suppress a new failed or suspicious production run.

Call `record_pulse_worklist` exactly once with the selected `mode`, its
`mode_reason`, and one decision for every canonical
module: `workflow_review`, `llm_ops_review`, `strategy_auditor`,
`goal_advisor`. These are perspectives, not artifact types. `workflow_review`
is Engineering Review: execution bugs, report/eval implementation bugs,
plan-change impact and artifact consistency, and DB/knowledgebase/learnings
integrity. `llm_ops_review` is operational efficiency and reliability.
`strategy_auditor` is the product/business review of whether the current
strategy and measurement system can achieve the goal. `goal_advisor` explores
materially different approaches. Do not emit retired artifact-named modules.
It is valid to skip every module when current evidence and recorded next-check
boundaries justify that choice. In that case no reviewer and no Fixer run; the
Finalizer still delivers the current durable state.

When `workflow_review` is due because of DB, knowledgebase, or learnings
integrity, say **Stores Health** and name the relevant store(s) in that
decision's `reason` and evidence. The later Engineering executor uses that
durable Gate evidence to run its distinct Stores Health turn before fixing; it
must not infer one from a generic Engineering due flag.
On recovery, if this Pulse run already has a complete worklist,
verify and stop; do not record it twice.

After recording the worklist, preserve longitudinal evidence without launching
another agent. If the just-completed producing run contains one or more
trustworthy comparable success-criterion measurements, call
`record_pulse_impact` exactly once with `observations` only. Use the stable
criterion id, metric, producing run id, route/environment, numeric value or an
honest qualitative status, timestamp, unit, and exact evidence provenance.
The observation identity is idempotent, so recovery may safely retry the same
run/criterion/metric/route. If no trustworthy observation exists, do not call
the tool and do not fabricate one. Gate never creates interventions or impact
assessments; the single Fixer links verified work and judges matured windows.

Every skip needs reason, evidence, and `next_check_at`, positive `cooldown_runs`,
or `next_check_after_run_id`. Name evidence that overrides cadence.
Missing baseline means `baseline pending`, not healthy. Use bounded adaptive cadence.

## Per-pass review cost cap

Select **at most two** due modules in one scheduled Pulse pass. When more than
two lenses independently qualify, rank them by the nearest actionable outcome:
current production failure or verification first, then aged open repair work,
then material new evidence, then discovery-only advisory work. Select the top
two; defer every other eligible lens with a plain reason that it lost this
capacity ranking and a concrete next-check boundary. A deferred lens is not
clean, completed, or forgotten — it remains eligible on its next boundary.
Never select a third module merely because it is independently due.

Make cadence proportional to the workflow's real schedule. An hourly workflow
must not buy the same unchanged deep review every hour: use material-change,
failure, awaiting-verification, answered-decision, and checkpoint evidence, then
set a meaningful run cooldown or time checkpoint for unchanged lanes. A workflow
that runs every three or four days must not be deferred for several additional
runs when the next producing run is the first chance to verify a repair. Consider
both elapsed time and completed producing-run count, and estimate when the next
scheduled evidence opportunity will arrive. Failures, suspicious success, newly
arrived verification evidence, and material plan/store/cost changes override
ordinary cadence. Never mark a lane due merely because Pulse itself ran, and
never use cadence to suppress a new high-severity signal.

Backlog drainage outranks broad discovery. When active findings, answered
decisions, unfinished fix attempts, or awaiting-verification work exist, select
the owning modules to drain that work and require them to use retained evidence
before launching another reviewer. Still select Workflow Review for a new failed or
suspicious run so a new P0 cannot hide behind the backlog. Defer discovery-only
health/advisory reviews whose artifacts and evidence did not materially change;
give each an exact backlog-drained, artifact-change, or next-valid-run check.
Do not repeatedly spend a pass rediscovering unchanged findings.

An answered `report_human_inputs` decision with source `pulse` and an id starting
`advisor-specialization-` is actionable configuration work. Select Strategy
Auditor as due (within the normal two-module cap) so the consolidated Fixer can
activate, revise, or reject it. This ownership choice is only a route to the
writer-capable Fixer; the Strategy Auditor must still perform its own independent
current-strategy review and must not treat the proposed lens as active before the
config tool succeeds.

Within the two-module cap, select the modules with actionable
repair/verification work or genuinely new trigger evidence. The next main-agent
Review+Fix turn owns every selected lens, any useful independent specialist
children, consolidation, repair, verification, and terminal receipts. There is
no scheduler-launched residual or recovery Fixer.
When work must wait for a real evidence/user/external boundary, record that
boundary instead of inventing a capacity cooldown.

For an off-track material goal, select independent lenses according to their
own evidence and cadence:

- **Engineering Review when implementation evidence changed.** It owns
  execution correctness, safe exploratory QA, report/eval implementation and
  truthfulness, plan-change blast radius and artifact consistency, and
  DB/knowledgebase/learnings integrity. A new plan changelog or store-integrity
  signal selects this perspective with the relevant evidence pack; it does not
  create another reviewer identity. Successful execution is never proof that
  behavior was correct.
- **LLM/Ops Review only for operational evidence.** Select it for a material
  cost/time/model/tool/runtime change, an Ops checkpoint, or retained Ops work.
- **Strategy Auditor more frequently than Goal Advisor.** Select it when the
  current strategy needs an independent completeness/effectiveness audit:
  activity and outcomes diverge, concentration, saturation, weak exploration,
  or absent target/source/outcome linkage. It asks from a product/business
  perspective whether the current plan, report, and evaluation system measure
  and create useful goal progress. A technically correct report/eval that
  measures the wrong thing is Strategy work; a broken implementation is
  Engineering work. Missing telemetry is `measurement_gap`, never healthy.
- **Goal Advisor selectively.** Select it for an independent blank-sheet
  opportunity review, an answered strategic decision, an experiment checkpoint,
  or planned healthy-headroom review. It explores materially different approaches
  outside the current plan; it is not a downstream handler for Auditor findings.

Engineering Review is also due for failures, suspicious success, an unreviewed
plan changelog, stale or internally inconsistent report/evaluation evidence,
wrong tool/source/route/decision evidence, store-integrity drift, or a reached
QA checkpoint. These triggers do not automatically make Ops or Strategy due.
Catalog changes override the LLM/Ops cooldown; never silently change models or
tiers.

Never make one reviewer due merely because another reviewer has or has not run.
When evidence is unreliable, select the highest-priority eligible lenses within
the two-module cap and let each return `execution_problem` or
`insufficient_evidence` within its own result. Strategy Auditor and Goal Advisor must not consume each other's conclusions; agreement is corroboration discovered only during later consolidation. A clean run or green eval cannot suppress a measured miss.
Implementation correctness stays Engineering Review work; efficiency, cost,
model, and runtime reliability stay LLM/Ops work; business usefulness stays
Strategy Auditor work.
Gate must not launch reviewers, mutate plan/config/artifacts, create the human-input request,
publish, back up, notify, or write HTML. Stop after recording the complete
worklist and any honest current-run goal observations.
