[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-258 — Dedicated `plan_drift_review` Pulse module (design + phases 1-3 complete)

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `design complete; phase 1 implemented; phase 2 complete (6 of 9 deterministic checks built; 1 needed no new code, 2 correctly found not-buildable-as-designed — see below); phase 3 implemented (record_plan_drift_review tool); phases 4-6 not yet built` |
| Last synchronized | `2026-08-30` |

- **Type:** platform feature (multi-phase), not a single bug fix. Filed at
  the user's explicit request, to track a real, user-identified gap and
  the full design before further phases land.
- **Origin:** the user reported a recurring, significant problem — editing
  a plan step regularly leaves something silently broken over time: DB
  access, DB table shape, learnings, KB, step description, or
  validation_schema drift out of sync with what the step actually does
  and needs. They correctly recalled a `/review-artifact-drift` mechanism
  is supposed to catch this, but suspected it doesn't actually work.

## Investigation: the existing mechanism is real but effectively unexercised

Confirmed via direct code/log inspection (not assumption):
- `/review-artifact-drift` (`agent_go/cmd/server/guidance/templates/review/review-artifact-drift.md`, Workshop-mode only) is a genuine, specific checklist covering the reported concerns.
- It is **not automatically enforced** — every plan-step-edit tool appends a text nudge asking the agent to run it, pure LLM judgment with no verification.
- One real enforced backstop exists: `validateDeterministicIntakeRouting` (`pulse_worklist.go:1010-1041`) fails `record_pulse_worklist` if `CollectPlanChangeBacklog` shows unreviewed changelog entries, unless `technical_review` is due — but it only forces due=**true**, never guarantees the checklist itself actually runs, and is Workshop-mode-gated in a way that may not be reachable from Pulse's unattended/scheduled context.
- **Evidence it's not working:** grepped ~2 months of `server_debug.log`/`schedule.log` (178MB+) — the actual invocation and its completion message (`"Marked %d changelog ... artifact_review.done=true"`) appear **zero times**. Scaffolded, not exercised.
- An existing ticket, **PLAT-197**, already built real gating around this (per-surface `surface_reviews`, Gate backlog blocking) but its own "still open" section admits: no requirement that a fix is verified against a real run afterward. The user's suspicion that a prior fix wasn't fully effective is correct — PLAT-197 solved the *gating* shape, not the *verification* gap.
- User also recalled a per-step JSON `checks` field for this — checked exhaustively against both `plan.json`'s step schema (`CommonStepFields`) and `step_config.json`'s `AgentConfigs`: **no such field existed** before this ticket. (It does now — see Phase 1.)

## Design (agreed with the user across this session)

**Module, not a technical_review sub-check.** A new dedicated Pulse module, `plan_drift_review`, event-triggered by real plan-step changes rather than a time cadence — confirmed feasible: Pulse's module registry (`pulsemodules.go`) was already built as a real extensible registry (a ~15-line entry, no DB migration — `pulse_module_state.module` is plain `TEXT`). The harder, novel part is genuine event-triggering (see Phase 1 below) and reviewer-turn authoring (Phase 4) — both real work, not free.

**Trigger — the simplest possible "due" check.** Every step carries a `drift_review` record. It gets nulled by the *same* hook that already nulls `description_reviewed` on any dependency-triggering field change (`clearDescriptionReviewedAfterPlanUpdate`). Pulse's "is `plan_drift_review` due" check becomes: does any step in the plan have `drift_review == null`. No cadence math, no judgment call — a plain scan.

**Evidence-required, not a boolean.** `drift_review` holds a `reviewed_at`/`reviewed_by` plus a list of per-check records (`check_id`, `status`, `evidence`) — the review has to say *what it compared and what it found*, not just "reviewed: true". This directly targets the self-reported-with-no-proof failure mode found above.

**The 14 checks**, split by whether they need an LLM or are pure Go:

*Deterministic (Group 1 — downstream SQL-dependency drift, extract-and-dry-run):*
1. Report query compatibility — every `window.report.query(...)` in `db/reports/index.html` still runs against current schema.
2. `validation_schema.db[].checks[]` SQL still resolves.
3. `evaluation_plan.json` query compatibility, same treatment.
4. Other steps' scripted-code `query_workflow_db(...)` calls, same treatment.

*Deterministic (Group 2 — declared vs. observed):*
5. `validation_schema` non-DB JSONPath fields still resolve against real recent `context_output.json`.
6. KB access mode vs. actual tool-call history.
7. Learnings access mode vs. actual tool-call history.
8. ~~DB access declaration drift~~ — investigated and **dropped**: `db_access` was retired in PLAT-061 (every step gets managed read-write access; there's nothing left to compare against).
9. `db/README.md` documented table contract vs. live `PRAGMA table_info`.
13. **Orphaned/legacy tables** — a table in `db.sqlite` with zero references across every known consumer (step queries, report, eval, docs) because the step that wrote it changed/was deleted. Fix routes through the existing `apply_workflow_db_migration` tool (which already auto-snapshots via `VACUUM INTO` before any destructive statement) — never a raw `DROP`.

*Judgment-based (Group 3 — needs an LLM turn, evidence-required schema applies):*
10. Step description accuracy vs. actual configured behavior.
11. Learnings *content* staleness (not just access mode) — does `learnings/<step-id>/main.py` still describe the step's current behavior.
12. KB *content* relevance, same idea.
14. **DB schema normalization/design quality** — informed by mechanical schema introspection (`PRAGMA table_info` across all tables), judged by the reviewer.
+ **KB/learnings lock appropriateness** (folded into 6/7): is `lock_learnings`/the access mode the *right* choice given the step's current maturity, not just internally consistent.

**Trust for the judgment checks (10, 11, 12, 14):** evidence-required records (above) + periodic independent spot-check verification (a second pass re-checks a sample, same pattern as this session's own adversarial code-review flow) + outcome-based tracking (if a later mechanical check catches something a judgment review should have caught, that's a logged review-quality failure, not just a missed finding).

**Sequencing (agreed, reordered after phase 3):** Phase 1 (this ticket) →
Phase 2 (build the deterministic checks as real Go functions) → Phase 3 (a
`record_plan_drift_review` tool, since `planning/` is FolderGuard-blocked-
write for normal sessions — the reviewer needs a purpose-built recording
tool with the same privileged write path `update_step_config` already uses,
not a raw file write) → **Phase 4 (frontend — currently hardcoded to a
2-module layout; moved up from phase 5 at the user's request, so the module
is visible/inspectable in the UI before the reviewer-turn content is
authored)** → Phase 5 (module registration + reviewer turn content, the
real authoring work) → Phase 6 (cutover: remove the now-redundant
drift-handling folded into `technical_review`, once the new module is
proven working).

## Phase 1 — implemented in this ticket

- `AgentConfigs.DriftReview *StepDriftReview` (new field, `step_config.json`), with `StepDriftReview{ReviewedAt, ReviewedBy, Checks []StepDriftCheck}` and `StepDriftCheck{CheckID, Status, Evidence}` — evidence-required by construction.
- `clearDriftReviewAfterPlanUpdate` — exact mirror of `clearDescriptionReviewedAfterPlanUpdate`, wired into both existing call sites (`handlePlanStepDependentArtifactReview`, `handleTodoTaskRouteArtifactReview`) so any dependency-triggering step edit nulls the record, same trigger condition as the existing description-review clear.
- `clearStepConfigField`/`isKnownAgentConfigClearField` updated with a `drift_review` case.
- `MergeAgentConfigFields` updated — caught by the codebase's own `TestMergeAgentConfigFieldsCoversEveryField` completeness test, which correctly failed until this was added (a saved `drift_review` would otherwise never reach the runtime on the merge path).
- Notice text updated so an agent editing a step sees the drift-review clear alongside the description-review clear.

## Phase 2 — in progress: the deterministic checks

Design refinement made while starting this phase: the deterministic checks
(Groups 1/2) don't need an agent/LLM turn at all — they're plain Go functions,
following the exact precedent `run_concerns.go` already documents ("these rows
are written by Go..., never by an agent calling a tool. There is no call for
an agent to skip"). Only the judgment checks (Group 3, phase 5 — frontend was
reordered ahead of it to phase 4, see Sequencing below) genuinely need a
Pulse reviewer turn and the `record_plan_drift_review` tool (phase 3) to
persist their reasoning-based evidence.

**Built (`plan_drift_checks.go`):** `CheckReportQueryCompatibility` — Check 1.
Extracts every `window.report.query(...)` SQL string out of a workflow's
`db/reports/index.html` (three quote-style patterns, since Go's RE2 regexp
engine has no backreferences — one pattern per quote character instead of a
single backreference-based pattern), then dry-runs each against the live
`db.sqlite` opened `query_only` (never mutation-capable, verified by test). A
query that used to run and now errors — a step renamed/dropped a table or
column the report depends on — is drift, caught mechanically, no LLM needed.
This is the exact concrete case ("a step can affect report") that prompted
this whole investigation.

**Built:** `CheckValidationSchemaDBRules` — Check 2. Dry-runs every
`ValidationSchema.DB[]` rule's SQL against the live `db.sqlite` and applies
its `MinRows`/`MaxRows`/`Checks` assertions via the REAL `evaluateDBRule`
(`pre_validation_db.go`) — the exact pure function normal pre-validation
calls, reused as-is rather than reimplemented, so semantics can never drift
between what a live run checks and what this drift check checks. A rule that
used to pass and no longer does (renamed column, row-count assertion broken)
is drift.

**Built:** `CheckValidationSchemaFileRules` — Check 5. Re-runs a step's
`ValidationSchema.Files[].json_checks` against its most recent real output,
via the real `validateJSONCheck` (`pre_validation.go`) — same reuse
principle. Deliberately takes an injected `loadJSON(fileName)` resolver
rather than finding "the most recent run's output file" itself — that
run-folder resolution is orchestration-layer plumbing (phase 3), unrelated to
what this check itself asserts, so it stays a pure, directly-testable
function with synthetic data.

**Investigation, then built or resolved for the rest:**

- **Check 3** (evaluation_plan.json query compatibility) — investigated:
  `EvaluationStep` (`evaluation_types.go`) has no SQL field; it carries
  `PreValidation *ValidationSchema` — the SAME type Checks 2/5 already
  consume. **No new check needed** — Checks 2/5 already cover eval steps for
  free once phase 3 runs them against every step's effective schema, not
  only regular steps'.

- **Built: `CheckScriptedCodeDBQueries`** — Check 4. Confirmed real shape by
  surveying 27 real `learnings/<step-id>/main.py` files: 24 use
  `sqlite3.connect(os.environ["DB_PATH"])` + `cur.execute("SQL", (params,))`,
  standard `?`-placeholder parameterization (3 outliers shell out to the
  `sqlite3` CLI — not covered, reported honestly as "0 queries found" rather
  than a false pass). Extracts `.execute(...)` calls (triple- and single-
  quoted), then dry-runs each via `EXPLAIN <sql>` with every `?` bound to
  `NULL` — SQLite still must resolve every referenced table/column to build
  the bytecode program, so schema drift is caught identically to a real run,
  without executing the statement or needing real parameter values (verified
  against a throwaway local database before shipping).

- **Built: `CheckDBReadmeContract`** — Check 9. Confirmed real shape:
  surveyed 13 real `db/README.md` files — 12/13 contain a literal `CREATE
  TABLE` DDL string (two conventions: inline backtick-delimited, or a fenced
  ` ```sql ` block; the 13th is prose-only, honestly reported as "no DDL
  found" rather than guessed at). Extracts each documented `CREATE TABLE`
  statement and runs it against a throwaway **in-memory** SQLite database,
  reading back its column list via `PRAGMA table_info` on that scratch
  database — using SQLite's own real parser instead of a hand-written SQL
  column-list parser (a DDL's own `CHECK`/`FOREIGN KEY` clauses make regex
  column extraction unreliable). Compares that declared column set against
  the live table's real columns. Hit and fixed a genuine bug in review: a
  `:memory:` SQLite database is private to whichever pooled connection opens
  it, so without pinning the scratch connection to `SetMaxOpenConns(1)` (and
  fully closing each query before starting the next one on it), the `CREATE
  TABLE` and the `PRAGMA` read could silently land on two different databases
  (empty columns) or, worse, deadlock waiting for a connection the pool would
  never free. Both fixed, verified by test.

- **Checks 6/7** (KB/learnings access mode vs. actual tool-call history) —
  investigated: **not buildable as originally conceived**. No durable,
  step-keyed tool-call history exists — the event store
  (`agent_go/internal/events`) is purely in-memory, pruned after session
  inactivity; the one durable-shaped candidate (`persistedToolCallTiming`,
  `timing_persistence.go`) is confirmed dead/staged code with zero real
  callers. More importantly: the exact violation this check was meant to
  catch ("called a KB-write tool despite read-only access") is **already
  prevented live** by FolderGuard write-path scoping — when
  `knowledgebase_access` doesn't grant write, the write path is simply never
  opened, so the attempt fails in the moment rather than needing after-the-
  fact detection. No check built; the only genuinely open part of the user's
  original ask here — "is the CURRENT access mode/lock state the *right*
  choice given the step's maturity" — was already correctly categorized as a
  Group 3 judgment check (phase 5), not a mechanical one.

- **Built: `CheckOrphanedTables`** — Check 13. Now buildable with reasonable
  source coverage using Checks 1/2/4/9's own extraction: cross-references
  every live `db.sqlite` table against every table name referenced by report
  queries, `validation_schema.db[]` rule SQL, scripted `main.py` queries, and
  `db/README.md`'s own documented table names (via a lightweight
  `FROM`/`JOIN`/`UPDATE`/`INTO` regex scan — a heuristic, not a SQL parser,
  same tradeoff as the query extractors). A live table matching none of
  those, and not on the platform-reserved list (mirrors
  `workspace/handlers/query.go`'s denylist — different Go module, kept in
  sync by comment), is a real orphan *candidate*. Deliberately takes its
  reference lists as pre-aggregated inputs rather than assembling them
  itself — full coverage needs every step's validation_schema and scripted
  code, which means parsing the whole `plan.json` step-type union, which is
  phase-3 orchestration work (it already needs to iterate every step to
  schedule their own checks) — and its evidence explicitly states this is a
  heuristic scan of known sources, not a full-plan reference audit, so a
  finding reads as "candidate for manual review," not certainty. The fix
  path for a real orphan is `apply_workflow_db_migration` (already
  auto-snapshots before any destructive change), never a raw `DROP`.

## Phase 3 — implemented: `record_plan_drift_review`

`planning/step_config.json` is FolderGuard-blocked-write for a normal
session, so the reviewer (automatic or manual) needs a purpose-built
recording path, not a raw file write. Registered
`record_plan_drift_review` through `registerPlanModificationTools` — the
exact same function that already registers `update_step_config`,
`cleanup_orphan_step_configs`, etc., so it gets the identical privileged
`writeFile` (`withPlanMutationWriteAccess`) every other plan-mod tool
already has, no new plumbing.

- Takes `step_id` + `checks: [{check_id, status, evidence}]` (+ optional
  `reviewed_by`). Rejects an empty `checks` array, a missing `check_id`, an
  invalid `status` (must be `pass`/`fail`/`fixed`), and — the concrete
  enforcement of "evidence-required, not a boolean" — any `evidence` under
  15 characters, which is enough to reject a rubber-stamped "ok"/"fine"/
  "n/a" without being a real quality bar.
- Writes the full record atomically (reads `step_config.json`, replaces
  `drift_review` wholesale, writes back) — creates a new `step_config.json`
  entry if the step never had one rather than erroring, matching how
  `update_step_config` handles a first write.
- `reviewed_by` defaults to the calling session id if not explicitly
  provided, so an automatic Pulse pass and a manual
  `review-artifact-drift` invocation are both distinguishable in the
  record later.
- Caught and fixed a real gap in review: `agent_go/cmd/server/
  toolset_invariant_test.go` guards exactly the failure class this session
  already found for `/review-artifact-drift` (a tool that exists but is
  never reachable) — a new plan-mod tool has to be added to BOTH
  `GetToolsForWorkshopMode`'s allow-list (`interactive_workshop_manager.go`)
  and the invariant test's own tracking list, or it would have been
  registered but never actually exposed to any agent. Both updated;
  `TestToolSetInvariants` passes.

## Verification

Phase 1: `go build ./agent_go/... ./workspace/...` clean. `go test
./agent_go/pkg/orchestrator/agents/workflow/step_based_workflow/...` — full
package suite green, including 2 new tests
(`TestClearDriftReviewAfterPlanUpdate`,
`TestClearDriftReviewAfterPlanUpdateSkipsTitleOnly`) mirroring the existing
description-review test pair exactly, plus the pre-existing
`TestArtifactReviewNotices`/`TestMergeAgentConfigFieldsCoversEveryField`
updated and passing.

Phase 2 (Checks 1, 2, 4, 5, 9, 13): 34 new tests total across
`plan_drift_checks_test.go`. Check 1 — 4 for `extractReportQueries` (all
three quote styles, dedup-preserving-first-occurrence-position, escaped-quote
handling, no-match case) and 4 for `CheckReportQueryCompatibility` (pass on
matching schema, fail on a dropped column, pass when no report exists, and a
dedicated safety test proving a report embedding an `UPDATE` statement never
actually mutates the database — the `query_only` guard holds). Check 2 — 4
for `CheckValidationSchemaDBRules` (assertions hold, fail on a renamed
column, fail when a row-count assertion breaks, pass when no rules
declared). Check 5 — 4 for `CheckValidationSchemaFileRules` (fields resolve,
fail when a declared field is renamed away in real output, fail when a
`must_exist` file is missing, pass when no rules declared). Check 4 — 6:
extraction against a real-shaped multi-line scripted file,
`countSQLPlaceholders`, pass/fail/not-scripted/mutation-safety. Check 9 — 7:
extraction across both real README conventions, DDL-to-column-list parsing
via the in-memory scratch database, pass/fail-on-dropped-column/
fail-on-dropped-table/pass-on-prose-only/pass-on-no-readme. Check 13 — 5:
table-reference extraction across all four SQL clause kinds,
pass-when-referenced, fail-on-a-genuine-orphan, reserved-tables-never-
flagged, report-and-readme-references-recognized. `gofmt`/`go vet` clean, full package suite
still green.

Phase 3: 10 new tests across `plan_drift_review_tool_test.go` —
`validateStepDriftChecks` (rejects empty slice, missing check_id, invalid
status, and 5 different placeholder-evidence strings in one table test;
accepts real multi-check evidence) and `createRecordPlanDriftReviewExecutor`
end-to-end (writes a new record, creates a `step_config.json` entry when the
step had none, overwrites and fully replaces a prior record while honoring
an explicit `reviewed_by`, rejects invalid checks WITHOUT writing anything —
verified by diffing the file content before/after —, requires `step_id`).
Plus `TestToolSetInvariants` (`agent_go/cmd/server`) passing after adding
the new tool to both tracking lists. `gofmt`/`go vet` clean, full package
suite still green.

## Reverify

Once later phases land: confirm live that editing a step's description/context_dependencies/validation_schema nulls `drift_review` in `step_config.json`, and that a title-only edit does not. Also confirm `record_plan_drift_review` is actually callable from a live Workshop-mode session (not just present in the allow-list) and that a written record is visible in `step_config.json` on disk.
