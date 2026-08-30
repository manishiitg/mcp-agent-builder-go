## Plan drift review

`plan_drift_review` is a review-**and**-fix module, the same shape as
`technical_review`: in one retained turn it establishes ground truth per due
step, applies safe workflow-owned repairs directly, verifies each fix, and
only routes to a human decision or a platform-owned boundary what it
genuinely cannot resolve itself. It does not hand routine drift off to
`technical_review` to redo — a check this module already fixed and verified
must never reappear as a separate `technical_review` finding for the same
root cause.

`plan_drift_review` is event-triggered, not cadenced: it becomes due whenever
any canonical plan step has no `drift_review` record at all, or has one with
`needs_review == true` — flagged by the same hook that flags
`description_reviewed` stale on any persisted plan-step field change (this
includes a title-only edit; nothing is classified as cosmetic). It is not
about time passing; it is about a step's configuration having moved since it
was last checked.

This is a **stale flag**, not a null-and-rebuild: a flagged step's prior
review (`checks`, `reviewed_at`, `reviewed_by`, `reviewed_through_change_id`)
stays exactly as it was until you replace it with a completed review. Use
that prior evidence — read it, and read only the `planning/changelog/`
entries for this step **after** `reviewed_through_change_id` (if the step had
no prior review, or no `reviewed_through_change_id`, treat every relevant
changelog entry as new) to understand exactly what changed since the last
check, rather than re-auditing the whole step from scratch.

### 1. Read the precomputed evidence

Call `get_pulse_state(view="module", pulse_run_id=<this run's id>)`. Its
`plan_drift_candidates` array lists every step with no `drift_review` record
or `needs_review == true`, each carrying its plan.json `step_type` (empty if
plan.json could not be read) and already carrying Go-computed results for:

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
  — say so in the evidence.

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
  workflow has no eval plan, nothing to pair. If it does, this check is
  about **coverage of every route this step declares, not just any one
  eval reference** (a single eval covering 1 of this step's 5 routes must
  not pass): collect every eval step whose `applies_to_routes` names this
  routing step's ID (e.g. `applies_to_routes:
  [{"routing_step_id":"<this step's id>", "route_ids":[...]}]` — see
  `references/evaluation-plan.md`), union their `route_ids`, and compare
  that union against this step's own declared `routes[].route_id` set.
  Missing routes are the finding — name them by `route_id` in the
  evidence. Two carve-outs, both judgment calls, not mechanical: (1) an
  eval step with no `applies_to_routes` at all runs for every execution
  regardless of route, so if it genuinely evaluates something the routing
  decision itself doesn't affect (e.g. a global output-format or
  cost-discipline check), it can count as intentionally route-agnostic
  coverage for routes that have no route-specific eval step — but if it
  only evaluates the behavior of the specific branch the run happened to
  take, it does not count as covering the routes it never sees. (2) a
  route whose destination is a trivial no-op (e.g. `next_step_id` goes
  straight to `"end"` with nothing produced) may legitimately have nothing
  worth evaluating — judge whether that's really true before treating it
  as covered.

### 3. Apply safe workflow-owned fixes and verify each one

For every check that failed, fix it now if it is a bounded, safe,
workflow-owned repair — the same standard `technical_review` applies to its
own repair batches. This is the normal path, not the exception:

- A broken `window.report.query(...)`, `validation_schema.db[]` rule, or
  scripted `main.py` query usually means a report/schema/script still
  references a renamed or dropped column/table — update the referencing
  artifact (the report SQL, the schema rule, or the script) to match the live
  schema. Route a genuine orphaned table through
  `apply_workflow_db_migration` (it auto-snapshots before any destructive
  statement) rather than a raw `DROP`.
- A stale description, stale learnings/KB content, or a wrong learnings/KB
  access mode is a direct edit through the normal Workflow Builder tools
  (`update_step_config` and friends).
- A `db/README.md` contract mismatch is a doc edit reconciling the
  documented DDL with the live schema (or the reverse, if the doc is right
  and the schema drifted — decide which side is authoritative from the
  step's actual current behavior, not merely which was easier to edit).

After each fix, verify it the same way `technical_review` verifies a repair:
re-run the specific check (re-derive the report query result, re-check the
schema rule, re-read the file) rather than assuming the edit worked. Record
that check `status: "fixed"` with evidence describing both what was wrong and
what you changed and confirmed.

### 4. Route what you cannot safely fix in this turn

Not every drift check is a same-turn repair. Classify anything you did not
fix using the same routes `technical_review` uses — do not invent a
different scheme. Every `record_pulse_finding` call in this step must carry
`step_id=<this step's id>`: `record_plan_drift_review`'s finding-verification
requires each fail-status check's linked finding to be filed against the
exact step under review, not merely to exist somewhere in the backlog.

- **A genuine user decision** (e.g. two materially different ways to
  reconcile a contract, and only the operator can say which is intended):
  call `create_human_input_request` first, then `record_pulse_finding` with
  `step_id`, `recommended_route="decision_required"`, and that
  `human_input_id`.
- **A platform-owned boundary** (a runtime/harness/bridge limitation, not
  this workflow's own plan, config, code, or data): `record_pulse_finding`
  with `step_id` (`recommended_route` may be omitted — it is not a valid
  route for this case). Then, in the same turn's step 6 close-out, give that
  finding's `finding_dispositions[]` entry on `record_pulse_result`
  `disposition="external_action_required"` with a `reason_code`, an
  `external_owner`, and a `reopen_condition` — those three fields belong to
  the disposition, not to `record_pulse_finding` itself.
- **Insufficient evidence to fix safely right now** (e.g. a real fix needs a
  future run's output to confirm): `record_pulse_finding` with `step_id`,
  `recommended_route="evidence_wait"`, and an exact `next_check`.
- Only as a last resort — a fix that is real, workflow-owned, and safe in
  principle, but too large or cross-cutting for this focused pass to
  complete and verify on its own — fall back to `record_pulse_finding` with
  `step_id` and `recommended_route="fixer_handoff"` so `technical_review`
  picks it up. Keep this rare: a `fixer_handoff` finding for something this
  module could safely have fixed itself is exactly the extra Pulse cycle
  this design exists to remove.

Reuse an existing issue's `issue_id` for the same semantic root cause instead
of filing a duplicate — check the compact backlog first (an existing issue
keeps its original `step_id`; a later `step_id` argument on an `issue_id`
update does not move it to a different step). File findings **before**
persisting the review: `record_plan_drift_review` rejects a fail-status check
with no `finding_id`, and rejects a `finding_id` that does not resolve to a
real, active, already-filed Pulse finding for this exact step — this is what
keeps a routed check from ever being persisted as "reviewed" with no
corresponding tracked item.

### 5. Persist the merged result per step

Call `record_plan_drift_review(step_id=..., checks=[...], reviewed_through_change_id=...)`
exactly once per candidate step, merging the precomputed checks from step 1
with the ones you ran directly in step 2, using `status: "fixed"` for
step-3 repairs and `status: "fail"` plus the linked `finding_id` for
step-4 routes. Every check needs a real `check_id`, a `status` of
`pass`/`fail`/`fixed`, and specific `evidence` — never a placeholder like
"ok" or "n/a" (the tool rejects evidence under 15 characters for exactly this
reason). Pass `reviewed_through_change_id` as the latest
`planning/changelog/` `change_id` you actually read for this step, so the
next review resumes exactly where this one left off. This call always fully
replaces the step's prior evidence and clears `needs_review` — there is no
partial update.

### 6. Close out

Update the run-scoped checkpoint (`runs/pulse/<run>/plan-drift-review.md`)
with a compact per-step summary before ending. Call `record_pulse_result`
exactly once for the terminal `plan_drift_review` module result, with a
`finding_dispositions[]` entry for every finding filed this turn — including
`disposition="external_action_required"` (with `reason_code`,
`external_owner`, `reopen_condition`) for step 4's platform-owned findings.
Do not render HTML, back up, publish, or notify — those belong to the
finalizer stage.
