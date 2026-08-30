## Pulse Gate / Worklist

Use only for the scheduler's Gate stage. Gate performs a progressive evidence
scan and records which of the canonical review perspectives is due. It does
not audit deeply, launch reviewers, fix anything, write workflow artifacts,
publish, back up, notify, or render the dashboard.
Gate must not launch reviewers.

The canonical modules are:

- `technical_review`: one retained technical reviewer sequence. Engineering
  correctness, Stores Health, runtime operations, orchestration fitness,
  model/tier fitness, cost attribution, tool reliability, and execution
  efficiency are selectable focus lenses inside this module—not separate
  durable queues or separate default agents.
- `strategic_review`: one retained strategic reviewer sequence. It audits the
  current strategy and measurement system, and conditionally explores
  materially different approaches. It is never folded into Technical Review.
- `plan_drift_review`: event-triggered, not cadenced — due whenever
  `get_pulse_state(view="module")`'s `plan_drift_candidates` is non-empty
  (a canonical plan step has no `drift_review` record, or has one flagged
  `needs_review: true`). This is a plain fact, not a judgment call: the
  backend rejects the worklist if a non-empty candidate list is not marked
  due, so always record its true state rather than guessing.

Do not emit retired module names such as `workflow_review`, `llm_ops_review`,
`strategy_auditor`, or `goal_advisor`. Historical rows using those names are
migration inputs only.

## Progressive evidence scan

Read `soul/soul.md`, compact schedules from `workflow.json`,
`get_pulse_state(view="module")`, compact lifecycle/focus agendas, relevant
human inputs, and the smallest retained run summaries needed to judge whether
material evidence exists. Compare run timestamps with each module's
`last_checked_at`; do not treat every retained folder as new.

Use returned open concerns, plan-change backlog, loop-closure state,
`deterministic_intake`, module history, and focus history as selectors. Do not inject or mechanically parse
the full plan or full review history. The later reviewer reads authoritative
files and tools selectively.

Use `get_plan_prompt_health` once when deciding whether a materially changed
plan needs Technical Review. It returns only compact deterministic description
size and long verbatim-duplication metrics; it is specifically the safe
alternative to dumping `planning/plan.json`. A triggered report is evidence
that a **plan-orchestration-integrity** review may be useful, not an automatic
finding and not authority for Gate to rewrite the workflow. The later Technical
Review must inspect the affected descriptions, schemas, and shared references
before it decides whether prompt-contract consolidation is safe.

Treat explicit `CONCERNS:` records as selectors, not automatic findings. A
later reviewer decides whether they are a correctness issue, an efficiency or
coaching opportunity, a non-issue, or insufficient evidence.

If retention no longer covers the period since the last check, record that
coverage gap honestly. Do not treat a partial sample as complete.

Compare exact pins against `list_provider_models` and
`default_tier_models`. Provider-profile defaults auto-update, while exact pins
do not. Never infer freshness by name or silently rewrite an exact pin.

## Decide whether Technical Review is due

Technical Review is due when evidence can support useful verification, repair,
or a bounded new diagnosis. Examples include:

- a failed or suspiciously successful production run;
- a verified deterministic runtime signal (`run_not_completed`,
  `runtime_status_disagreement`, or `tool_success_with_structured_failure`);
- matured verification for a prior repair;
- an answered technical decision that remains unapplied;
- a material plan, artifact, report/evaluation, DB, knowledgebase, or learnings
  change;
- material cost, latency, model, tool, retry, timeout, completion, schedule, or
  capacity behavior;
- a cadence-threatening run with repeated context reconstruction, large
  retained tool output, duplicated discovery/validation, or an unnecessary
  container/sequence boundary.
- a triggered compact prompt-health report: one step over 20k description
  characters, at least 30% of described steps over 5k, or at least 10k
  extractable verbatim duplicate-description characters. These are objective
  triage thresholds, not a conclusion that long work is wrong.

The worklist reason proposes the best current technical focus. Use these stable
focus keys when applicable:

- `execution_health` — correctness, efficiency, tool/runtime reliability, and schedule recovery. For schedule-fire/capacity-wait history (did a schedule fire on time, was a run suspended for provider capacity, did recent scheduled fires error), read this workflow's own `schedule-runs.json` at the workflow root — it carries status/error/duration per fire that individual run folders don't.
- `plan_orchestration_integrity` — plan contracts, dependencies, context/handoffs, step types, scripted-vs-agentic choices, and sequence/todo orchestration fitness
- `store_integrity`
- `report_quality_truth` — report accuracy plus reporting UI, accessibility, and performance practices
- `evaluation_quality_truth` — evaluator truth, rubrics, thresholds, negative tests, and reproducibility
- `model_cost_fitness` — model/tier/reasoning/fallback choices, quality-cost fit, and cost attribution. Read this workflow's own per-run/per-step/per-item cost and token breakdown with `query_workflow_costs` (PLAT-184) — do not rely on the global Cost Analysis dashboard, which this workflow's own agents cannot reach at all.

This is agentic selection, not a Go threshold or semantic classifier. A large
run can be justified by adaptive research, browser dwell, or independent
verification. Cite exact step/item IDs and compact evidence paths, and explain
whether the evidence supports review now or needs a named future boundary.

## Deterministic-intake boundary

Treat a verified deterministic signal as a focused evidence lead, never as an
automatic finding. The collector proves only an objective fact — for example,
that a completed outer run contains an errored child call or a success-labelled
tool result with a non-zero exit code. The Technical reviewer must decide
whether the failure was essential, safely recovered, already explained by a
canonical issue, or needs a new durable finding. Do not keyword-scan ordinary
output for words such as "error" and do not launch a Fixer directly from the
signal.

A failed verified deterministic intake cannot be cooled down or skipped: set
`technical_review.due=true` and route it to the smallest matching technical
focus. For `plan_change_dependencies`, this means a current-contract change
with a durable `change_id` lacks a complete receipt across downstream steps,
validation, evaluation, reporting, database, and learnings/knowledge. Select
`plan_orchestration_integrity`. The failure proves missing coverage, not that
all six surfaces need edits; the reviewer must inspect each surface and record
an evidence-backed disposition. Legacy reviewed entries without `change_id`
are not reopened by this check.

The same boundary applies to `plan_drift_candidates`: a non-empty list means
`plan_drift_review.due=true` is required, not agentic judgment. It is a
different fact from `plan_change_dependencies` — the latter is about a plan
edit's blast radius never having been traced; `plan_drift_candidates` is about
a step's per-check drift record specifically never having been recorded.

When DB, knowledgebase, or learnings integrity is selected, explicitly name the
Stores Health scope in the reason. Stores Health remains a technical lens, not
a separate module.

## Decide whether Strategic Review is due

Strategic Review needs evidence, not a free slot. Select it when accumulated
outcomes can settle a real product/goal question, for example:

- the goal metric is flat, unmeasurable, or contradicted by outcomes;
- activity and outcomes diverge—for example activity is high but useful
  outcomes are not improving;
- selection bias, feedback loops, concentration, saturation, proxy
  optimization, or observation contamination may be shaping the plan;
- an experiment or approved strategic decision has reached its assessment or
  application boundary;
- enough evidence exists to compare the current approach with materially
  different alternatives.

Useful strategic focus keys include:

- `goal_measurement_validity`
- `strategy_effectiveness`
- `feedback_loops_bias`
- `concentration_saturation`
- `alternative_headroom`
- `experiment_impact`

Do not select Strategic Review merely because Technical Review is skipped.
Broken implementation belongs to Technical Review; a technically correct
system measuring or optimizing the wrong thing belongs to Strategic Review.
Missing telemetry is a coverage gap, not evidence of health or zero impact.
Never make one reviewer due merely because another reviewer was skipped.
Strategic Review combines the former Strategy Auditor and Goal Advisor into
one sequence: its opportunity phase runs only when the evidence supports
Strategic Review for business usefulness or strategic headroom, including
materially different approaches.

## Focus priority and rotation

For each selected module, use the compact focus agenda and reason within the
highest applicable lifecycle class:

1. new critical regression, security issue, data loss, or widespread failure;
2. matured verification;
3. answered but unapplied decision;
4. materially changed or never-reviewed focus;
5. overdue focus;
6. oldest remaining focus.

This is not rigid round-robin and not a numeric relevance score. A lightweight
safety scan may preempt normal rotation. Map evidence to stable
route/group/sub-workflow scopes first, then let the reviewer choose the smallest
sufficient focus set. A small route often needs one focus; materially distinct
large routes may justify several. Every additional focus must have distinct
evidence, risk, decision, or repair value. Record what won, what it inspected,
and what it deferred. Repeated selection of the same unchanged focus while
eligible never-reviewed or overdue work exists requires an explicit urgent or
verification reason.

It is valid to skip Technical Review, Strategic Review, or both. Every skip
must record why no useful review is currently possible and include evidence
plus `next_check_at`, a positive `cooldown_runs`, or
`next_check_after_run_id`. Skipping is a durable decision, not an assertion
that the module is healthy forever.

## Choose the pass shape

Choose one mode and give a concrete `mode_reason`:

- `backlog_drain`: retained active issues, matured verification, or answered
  decisions already provide the useful work. Do not add broad discovery.
- `discovery`: materially new technical or strategic evidence may reveal a
  root cause not explained by retained work.
- `strategy`: a product/goal question warrants Strategic Review. Do not use it
  as a disguised correctness pass.
- `observe`: neither review has actionable or mature evidence. Wait for the
  named next boundary.

An old backlog must not hide a new production failure. Conversely, unchanged
backlog must not force another expensive discovery pass. A cooldown or focus
rotation cannot suppress a measured miss or a new critical regression.

Select **at most two** agentically-judged due modules from `technical_review`
and `strategic_review`. `plan_drift_review` is never part of that judgment
call: its due state is the plain fact reported in `plan_drift_candidates` (see
Deterministic-intake boundary above), so record it as due exactly when that
list is non-empty, whether or not you also select two other modules. Call
`record_pulse_worklist` exactly once with the mode, mode reason, and one
decision for every canonical module: `technical_review`, `strategic_review`,
and `plan_drift_review`. On recovery, if this Pulse run already has a complete
worklist, verify it and stop rather than recording it again.

### Worklist payload boundary (required)

`record_pulse_worklist.decisions[]` is deliberately a **small scheduling
receipt**, not a review plan. Each decision may contain only `module`, `due`,
`reason`, `evidence`, `next_check_at`, `next_check_after_run_id`, and
`cooldown_runs`. Do **not** put `focuses`, `route_scope`, `issue_ids`,
`deferred_focuses`, `decision`, or any review-plan field in this call. Explain
the selected scope in `reason` and `evidence`; the later reviewer records its
actual focus coverage with `record_pulse_review_focus` after inspecting the
evidence. An unknown field rejects the whole worklist and prevents all review
and repair work from starting.

After the worklist, optionally record trustworthy comparable success-criterion
measurements with `record_pulse_impact`. Use stable criterion IDs, producing run
IDs, route/environment, exact evidence provenance, and an honest value or
qualitative status. If trustworthy evidence does not exist, record nothing;
missing evidence is not zero or healthy. Gate never creates interventions or
impact assessments.

The later retained Review+Fix task owns selected technical work,
strategic work, lifecycle updates, verification, and terminal receipts. Stop
after recording the worklist and any honest impact observations.
