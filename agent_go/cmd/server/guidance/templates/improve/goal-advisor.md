Run the opportunity phase of Strategic Review as a fresh, strategy-first
message after the current-strategy audit checkpoint exists. It is not routine
Pulse maintenance: challenge the audit and search for a materially different
approach only when evidence indicates strategic headroom or a ceiling.{{if .Focus}}

Focus especially on: {{.Focus}}{{end}}

Pulse is SQLite-backed and the Pulse popup is the only UI. Read and write
advisor proposals, experiments, findings, evidence, and outcomes through typed
Pulse and human-input tools. Do not create, read, or update an HTML journal,
cards, timeline anchors, CSS, or dashboard fragments.

## Evidence first

1. Read `soul/soul.md` for objective, success criteria, and explicit approved
   constraints.
2. Read typed Pulse findings, review history, prior advisor proposals and
   outcomes, pending/answered human inputs, retained runs/evals, costs, reports,
   planning changelog, plan/config, and relevant DB evidence.
3. Distinguish verified facts, explicit constraints, and revisable assumptions.
   Never treat the current plan as evidence that its strategy is correct.

For a claimed opportunity trend, prior experiment outcome, or regression, start
with the current run and compare up to three comparable retained runs (same
route/group and materially equivalent configuration). Read compact measurements,
typed history, and outcomes first. Open raw conversation or tool traces only for
the precise unexplained difference; never bulk-read every retained conversation.
State an evidence limitation when fewer comparable runs remain.

## Strategy review

State the current strategy ceiling, then generate materially different
alternatives before choosing one. Examine causal stages from acquisition/input
through execution, measurement, decision, action, and verified outcome. Look
for concentration, saturation, proxy optimization, missing causal stages,
unmeasured downside, stale evidence, and opportunities outside the current
plan.

For the highest-leverage thesis, specify: baseline, intended change, primary
success metric, evidence source, guardrails, review checkpoint, rollback/stop
condition, and what would disprove it. Do not propose a tactic merely because
it is novel.

## Proposal and experiment lifecycle

Experiments are optional. Multiple experiments may be `running` or `measuring`
only when their declared interference domains do not overlap. Proposed or
approved-but-not-started experiments do not consume an active slot. Each typed
record must retain a stable id, status (`proposed`, `deferred`, `approved`, `running`,
`measuring`, `blocked`, `adopted`, `rejected`, or `retired`), baseline, metric,
guardrails, evidence checkpoint, interference domains, and terminal outcome.

Persist that record with `record_pulse_impact(interventions=[...])`, using
`kind="strategy_experiment"`, `impact_type="direct_goal"`, baseline_window,
checkpoint, guardrails, and rollback_condition. Link human_input_id whenever
you ask for approval. A running/measuring experiment must declare stable
interference domains for every applicable goal criterion, control surface,
channel/cohort, metric stream, shared resource, and contamination boundary.
Do not use an ordinary fix-bundle intervention as a
substitute for an experiment.

If user/business judgment is required, create one `create_human_input_request`
with approve/reject/defer options, exact intended edits, expected impact, risk,
and evidence. Link the typed proposal/experiment to that request. Do not alter
the plan until an approved answer is available. Use a typed finding or
recommendation for evidence-wait or technical prerequisites instead of creating
a fake decision.

## Read-only and parent behavior

When instructed `READ-ONLY REVIEW`, return a compact evidence packet only; do
not edit files, consume answers, mutate plan/config/report/eval, or launch more
agents. The parent validates the result, applies only permitted approved work,
records typed dispositions, and marks consumed human input with the real
outcome.

## Close-out

Return concise plain language: strategy ceiling, thesis considered, conclusion,
evidence, proposal/experiment status, decision needed (if any), and next
evidence boundary. Persist only typed records; presentation is handled by the
Pulse popup.
