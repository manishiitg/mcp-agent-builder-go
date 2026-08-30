## Plan drift review (lean first version)

This module has no repair authority. It establishes ground truth per step and
hands off — it never edits plan/step_config/validation_schema, learnings, KB,
or db.sqlite in this turn. A later `technical_review` pass (or a human) does
the actual repair.

`plan_drift_review` is event-triggered, not cadenced: it becomes due whenever
any step's `drift_review` record in `step_config.json` is null — cleared by
the same hook that clears `description_reviewed` on any dependency-triggering
plan edit (a step's description, `validation_schema`, `context_dependencies`,
or store-access mode changed). It is not about time passing; it is about a
step's configuration having moved since it was last checked.

### 1. Read the precomputed evidence

Call `get_pulse_state(view="module", pulse_run_id=<this run's id>)`. Its
`plan_drift_candidates` array lists every step with a null `drift_review`
record, each carrying its plan.json `step_type` (empty if plan.json could
not be read) and already carrying Go-computed results for:

- `report_query_compatibility` — every `window.report.query(...)` in
  `db/reports/index.html` still resolves against the live schema.
- `validation_schema_db_rules` — only when the step's `step_config.json`
  carries a `validation_schema.db[]` override; a plan.json-only declared
  schema with no override is not covered here (check it yourself if the step
  has one).
- `scripted_code_db_queries` — the step's `main.py`, if scripted, still
  resolves its `sqlite3` queries against the live schema.
- `db_readme_contract` — `db/README.md`'s documented `CREATE TABLE` DDL still
  matches the live schema (workflow-wide, attached to every candidate step).

Read `plan_drift_candidates_note` for the exact coverage boundary. Treat a
precomputed `fail` as real evidence — do not re-derive it, and do not accept a
`pass` at face value without confirming it against the check's own explicit
scope (an empty rule set legitimately passes).

### 2. Fill the gaps Go could not precompute

For each candidate step, directly check what the deterministic pass skipped:

- **`validation_schema` file rules** — if the step declares
  `validation_schema.files[].json_checks` or `must_exist`, read that step's
  most recent real run output and verify those assertions still hold by hand.
- **Orphaned tables** — once per pass, not per step: list `db.sqlite`'s
  tables via `query_workflow_db`, cross-reference against report queries, any
  step's DB rules/scripted queries, and `db/README.md`. A table matching none
  of those and not a platform-reserved table is a candidate, not a certainty
  — say so in the evidence. Route a genuine orphan through
  `apply_workflow_db_migration` (it auto-snapshots) rather than filing it as
  a plain finding, but do not run that migration in this turn: this module
  has no repair authority, so file it via `record_pulse_finding` instead.

Then do the judgment checks a Go function cannot:

- **Step description accuracy** — does the description still match the
  step's actual configured behavior (prompt, tools, store access)?
- **Learnings / KB content staleness** — does `learnings/<step-id>/main.py`
  or its knowledgebase notes still describe what the step currently does?
- **Learnings / KB access appropriateness** — is the step's
  `knowledgebase_access` / learnings access mode and lock state still the
  right choice given the step's current maturity (not just internally
  consistent with itself)?
- **DB schema normalization** — once per pass, not per step: informed by
  `PRAGMA table_info` across every table, judge whether the schema stays
  reasonably normalized, not merely whether each table's own contract holds.

For a candidate whose `step_type` is `"routing"` only (never `"branch"` —
branch is deliberately the small in-flow decision, these two checks do not
apply to it; see `references/routing.md`/`references/branch.md`), also
judge:

- **`route_structural_isolation`** — read this step's `routes[]` and
  `next_step_id` chains directly from plan.json. Trace each route forward
  step by step. Two sibling routes legitimately reconverge at a shared
  step — routing.md's documented convergence pattern, where both routes'
  terminal steps point their `next_step_id` at the same downstream step (or
  both at `"end"`); that is not drift. Flag it when an *interior* step
  (reached before either route's own convergence point) is reachable from
  more than one route: that means the routes are silently sharing exclusive
  path segments they shouldn't, not deliberately rejoining.
- **`route_eval_pairing`** — check whether the workflow has an
  `evaluation/evaluation_plan.json` at all. If it does not, this check is
  out of scope for this step — record it `pass` with evidence saying the
  workflow has no eval plan, nothing to pair. If it does, look for at least
  one eval step whose `applies_to_routes` references this routing step's ID
  (e.g. `applies_to_routes: [{"routing_step_id":"<this step's id>",
  "route_ids":[...]}]` — see `references/evaluation-plan.md`). No matching
  eval step for a workflow that otherwise has an eval plan is the finding.

### 3. Persist the merged result per step

Call `record_plan_drift_review(step_id=..., checks=[...])` exactly once per
candidate step, merging the precomputed checks from step 1 with the ones you
ran directly in step 2. Every check needs a real `check_id`, a `status` of
`pass`/`fail`/`fixed`, and specific `evidence` — never a placeholder like
"ok" or "n/a" (the tool rejects evidence under 15 characters for exactly this
reason). Use `fixed` only for a check this same turn genuinely repaired and
verified; since this module has no repair authority, that should be rare to
never in this first version — default to `fail` plus a filed finding.

### 4. Hand off real failures

For any check that failed and was not itself fixed in this turn, call
`record_pulse_finding` with `module="plan_drift_review"`, a stable
`concern`/`target_key` naming the step and the broken surface, and
`recommended_route="fixer_handoff"` so it enters the normal `technical_review`
repair queue rather than waiting on this module to gain repair authority.
Reuse an existing issue's `issue_id` for the same semantic root cause instead
of filing a duplicate — check the compact backlog first.

### 5. Close out

Update the run-scoped checkpoint (`runs/pulse/<run>/plan-drift-review.md`)
with a compact per-step summary before ending. Call `record_pulse_result`
exactly once for the terminal `plan_drift_review` module result. Do not
render HTML, back up, publish, or notify — those belong to the finalizer
stage.
