# Plan-change impact analysis

A workflow is a web of **implicit dependencies**: a step's output feeds downstream steps, evals score it, the report dashboard queries its data, db stores it, and learnings/KB describe its behavior. Change a step and those silently rot — a downstream step reads a field you renamed, a report query returns nothing, an eval scores the wrong thing, a learning teaches the old behavior.

**Whenever you change a plan** — add / remove / reorder a step, or change a step's output contract, behavior, or description — you are not done until you have checked and reconciled the blast radius. This applies whether the change came from an active Workshop edit, an approved Goal Advisor proposal, or a bounded Pulse Fixer repair.

## 1. Name the change surface
First, pin down exactly what other artifacts key off — the **surface** of the change:
- the **step id**,
- the **output file / artifact** it produces (path + the JSON fields / shape),
- the **db tables / columns** it writes,
- its **topic / behavior** (what learnings & KB describe).

If you renamed a field, removed an output, changed a file path, changed what's written to db, or changed what the step does — that's the surface to trace. A change with no surface change (see Scope note) has no blast radius.

## 2. Trace each dimension — search, don't guess
For each dimension, **search the workspace** for references to the surface and reconcile every hit. Don't reason about ripple effects in the abstract — grep for the *actual* references (step id, output file/field, db table/column, topic).

- **Downstream steps** — search `planning/plan.json` and step descriptions for the step id / output file / changed field. A later step that consumes a field you changed must have its expectation **and** its `validation_schema` updated. (`read_skill(skills=[{"name":"builder-reference","path":"references/step-config.md"}])`)
- **Evals** — search `evaluation/` for the step id / output path. An eval that reads the changed output must be updated so it still resolves and scores the right thing. (`read_skill(skills=[{"name":"builder-reference","path":"references/evaluation-plan.md"}])`)
- **Report dashboard** — search `db/reports/index.html` and its `window.report.query` SQL for the db tables/columns and output fields. A query that reads changed data must be fixed. (`read_skill(skills=[{"name":"builder-reference","path":"references/reporting-policy.md"}])`)
- **db** — read `db/README.md` (it already lists each table's writers + shape). If this step writes db and the shape changed, update the schema, the README contract, and any readers. (`read_skill(skills=[{"name":"builder-reference","path":"references/stores.md"}])`)
- **Learnings** — the step's `learnings/{step-id}/` and `learnings/_global/SKILL.md` were generated against the old behavior. If behavior changed, clear `lock_learnings`/`lock_code` so they regenerate, and prune any now-wrong notes.
- **KB** — search `knowledgebase/notes/` for the step's topic. Notes describing the old behavior must be updated or flagged.

It is tractable **because the contracts already exist** — `db/README.md` lists writers and shape, report HTML declares its queries, the eval plan declares scope, the plan holds the step handoffs. You are cross-linking what is already written, not inventing a dependency graph.

## 3. Reconcile or flag
For each affected dependent:
- **Fix it now** if the fix is clear and contained — update the consumer's schema, fix the report query, repair the eval source, update the db contract, clear a learning lock. (Back up first, or rely on the surrounding pass's backup.)
- **Flag an open finding** if the reconciliation needs judgment — a downstream step needs real redesign, a metric definition is now ambiguous. Record it through typed Pulse tools so it isn't lost.

**Never leave a plan change with a silently broken dependent.**

## 4. Record structured dependency closure and impact
After tracing, call `mark_changelog_artifact_reviewed` with one evidence-backed disposition for every required surface: `downstream_steps`, `validation`, `evaluation`, `reporting`, `database`, and `learnings_and_knowledge`. Use only `updated`, `already_compatible`, `not_applicable`, `blocked`, or `broken`. A cosmetic change can honestly use `not_applicable`, but it still needs evidence explaining why the surface is unaffected. Every `blocked` or `broken` surface must include the durable Pulse `issue_ids` that own the unresolved repair. A bare `artifact_review.done=true` is no longer sufficient.

When the change has a defensible measurable effect, also record a typed Pulse impact intervention: what should change, the comparable baseline, and the future checkpoint. Link it back to each changelog entry with `sources=[{"source_type":"review","source_id":"<change_id>"}]`; this existing impact ledger is the effects log. Do not claim the change worked until later runs bound to the expected plan revision provide enough evidence.

## The changelog is your work-list — keep it lean
Every plan-mod tool call is auto-written to `planning/changelog/changelog-*.json` (tool, `reason`, affected step ids, old/new values). Treat entries without `artifact_review.done=true` as the **ledger of changes whose blast radius may not be reconciled yet** — your work-list. When you reconcile a change (steps 1–3), record its typed Pulse impact summary.

Do **not** edit or delete changelog files directly. The read-only Artifact Review agent returns exact inspected entries plus all six surface dispositions and evidence; the parent writer records typed findings/dispositions and then marks those entries through the dedicated `mark_changelog_artifact_reviewed` tool. Pulse uses that metadata to skip future no-op review turns.

This proactive check is one end of a loop; the **`review-artifact-drift` audit** is the read-only checklist passed to a generic reviewer that sweeps the changelog and catches anything the proactive pass missed. Its parent writer uses changelog review metadata as the cursor and calls `mark_changelog_artifact_reviewed` only for exact inspected entries. Pulse handles the same concern inside its independent Artifact Review module stage.

## Scope note
The discipline **scales to the change**. A change that is purely internal to a step (same output contract, same db writes, same described behavior) has no blast radius — confirm that quickly and move on. A renamed output field touches downstream + report + eval + db; a reworded description that keeps the same contract may only touch learnings. Trace what the surface actually reaches, not a fixed checklist for its own sake.
