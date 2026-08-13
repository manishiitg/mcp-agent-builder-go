Define what success means for this workflow before optimization.

The stable Goal contract belongs only in `soul/soul.md`. Time-based findings,
decisions, evidence, and follow-up work belong in typed Pulse records rendered
in the Pulse popup. Do not create or update a Pulse HTML document.{{if .Focus}}

Focus / hints from user: {{.Focus}}{{end}}

## Discovery (read-only)

1. Read `workflow.json`, `planning/plan.json`, `planning/step_config.json`,
   `soul/soul.md`, current reports, evaluation plans/results, and the typed
   Pulse backlog/review history relevant to the request.
2. State the current objective, success criteria, measurable signals, and any
   gap between the Goal and what the workflow actually measures.
3. Separate confirmed user constraints from assumptions inferred from artifacts.

## Confirm the Goal

Propose a concise Objective and checkable Success Criteria. Each criterion must
name an outcome, a signal/source, a comparison or threshold where appropriate,
and the cadence at which it can be observed. Do not turn implementation details,
provider choices, or temporary tactics into Goal constraints.

Ask for confirmation when changing the objective, metric meaning, threshold, or
any material trade-off. Once confirmed, update `soul/soul.md` through the
approved workflow tool. Do not duplicate the Goal in another artifact.

## Measurement review

Check whether reports, evaluation, and durable DB data can measure each success
criterion honestly. Missing coverage, broken measurements, or ambiguous metric
semantics must be filed as typed Pulse findings with evidence, recommended fix,
and a verification boundary. Use a human-input request only for a real
user/business decision, not a deterministic repair.

## Close-out

Return a compact summary: confirmed Goal, criteria, evidence sources, gaps,
decisions requested, and next action. Persist only typed Pulse records needed
for decisions or follow-up; do not create a presentation artifact.
