[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-258 — Dedicated `plan_drift_review` Pulse module (corrective contract implemented; parity follow-up open)

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `open — combined review-and-fix contract agreed` — the corrective scan/stale-evidence foundation is implemented, but `plan_drift_review` must now own safe workflow repair and verification in the same retained turn, matching Technical Review's combined maintenance pattern; slash/scheduled parity, deleted-step coverage, mutation atomicity, and the remaining independent-review gaps also remain open |
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

**Trigger — the simplest possible "due" check.** Every reviewed step carries
a `drift_review` record containing its evidence plus a `needs_review` flag.
Every persisted step update sets that flag to `true` while preserving the
previous review. Pulse's due check becomes: does any canonical plan step lack
a review record, or have `drift_review.needs_review == true`. No cadence math,
compatibility-check trigger, or judgment call — a plain scan.

**Evidence-required, not merely a boolean.** `drift_review` holds
`needs_review`, `reviewed_at`/`reviewed_by`,
`reviewed_through_change_id`, and a list of per-check records (`check_id`,
`status`, `evidence`) — the flag controls deterministic due-ness, while the
record must still say *what it compared and what it found*. This directly
targets the self-reported-with-no-proof failure mode found above.

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

## Phase 4 — implemented: frontend, module visible ahead of reviewer-turn authoring

Reordered ahead of phase 5 at the user's explicit request, so the module is
inspectable in the UI before its reviewer-turn content is written. A prior
research pass (this ticket) confirmed the frontend's Pulse module list
(`pulseSections.ts`'s `PULSE_MODULE_COMMANDS`) was a hand-maintained 2-entry
array, independently restated (not derived) a second time inside
`PulseWorkspace.tsx`'s "Work areas" card grid, with a literal `lg:grid-cols-2`
Tailwind class — the exact drift-prone-restatement shape `pulsemodules.go`'s
own doc comment already warns about, just on the frontend side of the fence.

**Changed:**
- `pulseSections.ts` — added the `plan_drift_review` entry (id/label/
  description) to `PULSE_MODULE_COMMANDS`. Everything that already derives
  from this array generically (`WorkflowToolbar.tsx`'s recorded/total pulse
  overview count, `buildPulseWorkspaceModuleSummaries`,
  `normalizePulseWorkspaceModule`) picked up the third module for free — no
  count assumptions found there.
- `PulseWorkspace.tsx` — added the third "Plan drift review" card to the
  Work-areas inline array (its own icon/description/tone, matching the
  existing two literal entries' shape) and widened `lg:grid-cols-2` →
  `lg:grid-cols-3`. Deliberately did **not** add `plan_drift_review` to the
  `strategic` advisor-style branch (`area.id === 'strategic_review'`) or to
  `pulseFindingPresentation.ts`'s advisor-module list: this module's findings
  are deterministic pass/fail/fixed checks with a real fix path, the same
  shape as Technical Review's Gate findings, not strategy recommendations a
  user accepts/declines — so it correctly falls into the existing
  non-strategic branch, which is already generic over lifecycle counts and
  needed no new bespoke rendering logic.
- `ReportHumanInputPanel.tsx`, `reportHumanInputChat.ts`,
  `reportHumanInputFormatting.ts`, `api-types.ts` — added `plan_drift_review`
  to the human-input `source` label maps ("Plan Drift Review" /
  "Waiting for Plan Drift Review" / "Plan Drift Review is working"), matching
  the existing pattern for the other two modules. Inert until phase 5 makes
  the module actually post human-input requests, but avoids a second,
  later, easy-to-forget cross-file update once it does.
- Test updates: `pulseSections.test.ts`'s exact-array assertion now expects
  all three module ids; `PulseWorkspace.test.tsx` asserts the third card's
  label renders.

**Consequence understood and intended, not a bug:** until phase 5 registers
`plan_drift_review` in the Go `pulsemodules.go` registry and wires scheduling,
the new card will show "No stored review yet" with all-zero counts, and
`WorkflowToolbar.tsx`'s pulse overview denominator becomes 3 while the
numerator can only reach 2 — a visibly-incomplete-looking state. This is
exactly what phase 4 being moved ahead of phase 5 was for: the module visible
and inspectable before its content is authored, not a functional module yet.

**Verification:** `npx vitest run` — 730 passed, 2 pre-existing failures in
`sessionRestore.productFallback.test.ts` (unrelated Video Studio
session-restore work from a concurrent session, confirmed by file/topic, not
touched by this change). `npx tsc --noEmit` clean.

## Phase 5 — implemented: module registration, scheduling, reviewer-turn content

Note on authorship: this phase's core implementation (module registration,
`CollectPlanDriftCandidates`, the `plan-drift-review.md` reviewer-turn
content, scheduler/Gate wiring) was built directly in the shared primary
working directory by a concurrent session while this ticket's own Phase 5
investigation was still in flight. Found via `git status` before starting —
reviewed line-by-line, built, and tested rather than re-implemented; this
section documents what was verified and the two gaps closed on top of it.

**Registration (`pulsemodules.go`):** `PlanDriftReviewID = "plan_drift_review"`
added as a third `Module{}` in `All`, `StepLabel: "plan-drift-review"`,
aliases `drift_review`/`plan_drift`. No scheduler step-label collision.

**`CollectPlanDriftCandidates`** (new file `plan_drift_candidates.go`): the
orchestration-layer Go function Phase 2's design always needed — scans
`step_config.json` for steps with a null `drift_review` record and runs the
deterministic checks Go can precompute for each: Check 1 (report query
compatibility) and Check 9 (`db/README.md` contract) once per pass
(workflow-wide, attached to every candidate), Check 4 (scripted-code queries)
per step, and Check 2 (`validation_schema.db[]` rules) per step **only when
`step_config.json` itself carries the override** — a plan.json-only declared
schema with no override is documented as out of scope for this precompute
pass. Checks 5 (validation_schema file rules) and 13 (orphaned tables) are
deliberately not precomputed — no run-folder resolver exists yet for 5, and
13 needs every step's references aggregated across the whole plan — both are
routed to the reviewer turn's own direct-check step instead, exactly Phase
2's original design for what the deterministic pass could and couldn't cover.

**Due-ness enforcement (`pulse_worklist.go`):** `validatePlanDriftRouting`
mirrors `validateDeterministicIntakeRouting`'s treatment of `technical_review`
exactly — plan_drift_review's due state is a plain fact (any candidate from
`CollectPlanDriftCandidates`), not a judgment call, so `record_pulse_worklist`
is rejected outright if a non-empty candidate list isn't marked due. Wired
into `recordPulseWorklistWithMode` alongside the existing intake check.
`get_pulse_state(view="module")` now also returns `plan_drift_candidates` (the
same precomputed list) plus a `plan_drift_candidates_note` explaining the
coverage boundary, so the reviewer turn starts from evidence instead of
re-deriving it.

**Reviewer-turn content (`plan-drift-review.md`, new guidance template,
registered in `guidance.go`):** explicitly scoped as a **lean first version
with no repair authority** — it establishes ground truth per step and hands
off real failures via `record_pulse_finding(recommended_route="fixer_handoff")`
into the existing `technical_review` repair queue, rather than repairing
anything itself. Five steps: read the precomputed evidence, fill the two
checks Go couldn't precompute plus the Group 3 judgment checks, call
`record_plan_drift_review` once per step merging both, file a finding for
anything unresolved, checkpoint and call `record_pulse_result` once. This
matches Phase 2's original trust design for judgment checks (10/11/12/14)
exactly, and correctly folds the KB/learnings access-mode-appropriateness
question (originally Checks 6/7) into the judgment pass rather than a dead
mechanical check, as Phase 2 had already concluded.

**Scheduling (`scheduler.go`, `pulse-gate.md`):** `plan_drift_review` gets its
own `run_in_background` launch block in the review-fix lifecycle step,
parallel to `technical_review`/`strategic_review`'s existing blocks. Gate's
worklist prompt updated: `plan_drift_review` is explicitly carved out of the
"select at most two" agentic judgment call — it is always recorded as due
exactly when `plan_drift_candidates` is non-empty, never selected or skipped
by discretion.

**Findings visibility:** `record_pulse_finding(module="plan_drift_review")`
means failures become real `PulseFindingLifecycle` rows tagged with this
module, so Phase 4's frontend card will show non-zero counts once this ships
live — the two phases connect as designed.

**Gaps closed in review (this ticket's own contribution to phase 5):** no
test file existed for either new function. Added
`plan_drift_candidates_test.go` (7 tests: nil on missing/malformed/fully-
reviewed step_config.json, correct candidate list on a mix of reviewed/
unreviewed steps, workflow-wide + scripted checks always present, the DB-rule
check's override-only condition, blank-workspace-path handling) and
`TestPulseWorklistRequiresPlanDriftReviewWhenCandidatesExist` in
`pulse_worklist_test.go` (mirrors the existing Technical-Review intake-routing
tests: a candidate forces rejection until marked due, then succeeds).
Confirmed no other test asserts a stale 2-module set — the one hit
(`TestValidatePulseDueModuleResultsRequiresTerminalModuleResults`'s
`"technical_review, strategic_review"` substring check) remains correct
because that test never marks `plan_drift_review` due and sets up no
step_config.json, so `CollectPlanDriftCandidates` returns nil there.

**Verification:** `go build ./...` clean (`agent_go`, `workspace`).
`gofmt`/`go vet` clean for every touched/new file (two pre-existing `go vet`
findings elsewhere in the package, unrelated to this change, left as found).
`go test ./cmd/server/... ./pkg/orchestrator/agents/workflow/step_based_workflow/...`
full packages green, 8 new tests total (7 phase 5 candidate tests + 1 routing
test) plus the full pre-existing suite for both packages.

## Phase 6 — implemented: technical_review cutover

Scoped narrowly and deliberately, based on a dedicated investigation into
exactly what in `technical_review`'s guidance chain is genuinely redundant
with `plan_drift_review` versus a distinct concern that must not be touched —
getting this wrong (removing something still load-bearing) was the single
highest risk in this whole ticket.

**What is genuinely redundant and was cut over:**
`review-artifact-drift.md` (the checklist `technical_review` conditionally
loads as an evidence pack, also the standalone `/review-artifact-drift`
command) previously told an agent to manually re-derive DB/report/
validation_schema/scripted-code drift by hand for every affected step — work
`plan_drift_review` now does mechanically, via real Go dry-runs against the
live schema, which is strictly more rigorous than a manual read. Added a
deferral note in its checklist: before re-deriving `report_query_
compatibility`, `validation_schema_db_rules`, `validation_schema_file_rules`,
`scripted_code_db_queries`, `db_readme_contract`, or `orphaned_tables` by
hand, read the step's `agent_configs.drift_review` record and treat it as
authoritative when present and current; only fall back to a manual check
when the record is absent, stale, or its evidence looks insufficient. Also
fixed a line that PLAT-258 made factually false ("Artifact drift is a
technical-review focus, not a third Pulse module" — there now is one).

**What looked redundant but is not, and was deliberately left alone (per
investigation):**
- `validateDeterministicIntakeRouting` (forces `technical_review` due on an
  unreconciled changelog entry, tracked via `artifact_review.done`) and
  `validatePlanDriftRouting` (forces `plan_drift_review` due on a null
  `drift_review` record) enforce two independent completion flags for two
  independent processes — a plan edit's six-surface blast-radius
  reconciliation vs. one step's per-check drift state. Both can legitimately
  fire on the same edit. Neither Go function was touched.
- `review-artifact-drift.md`'s non-overlapping coverage (schedule cron/
  timezone/queue drift, eval/success-criteria coverage gaps, downstream-step
  field consumption, dead step/schedule references, cross-step writer/
  consumer semantic disagreement) is real, distinct work `plan_drift_review`
  does not do — the file was trimmed at the overlap, not gutted or deleted,
  and the standalone `/review-artifact-drift` command it also serves keeps
  working unchanged.
- `pulse-bug-review.md`'s "drift" mentions (`shadow_store_drift`, schema/
  description drift found via actual execution-trace evidence) are a
  different mechanism — real-run bug detection, not `plan_drift_review`'s
  static/schema-level checks — and were left untouched.
- `pulse-fixer-practices.md`, `workflow-tools.md`, `optimize-playbook.md` —
  confirmed either generic process guidance or a different consumer
  (Workshop-facing manual tool catalog, not the Pulse-automated turn), left
  untouched.

Also added a one-line disambiguation in `plan-change-impact.md` (which
already used the informal term "Artifact Review module stage" for
`technical_review`'s internal changelog-reconciliation stage, predating this
ticket): clarified it is not the same thing as the new, separately-scheduled
`plan_drift_review` Pulse module, since the two now share adjacent
terminology.

**Verification:** `go build`/`go test`/`gofmt` clean (no Go code changed in
this phase — only guidance-template Markdown). Guidance rendering tests
re-run explicitly (`TestArtifactDriftAuditsTheSchedule`,
`TestStandalone*`, `TestMaterialize*`) — all pass; none asserted the exact
text this phase removed or added, since they check for presence of specific
unrelated substrings that remain in place.

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

After the corrective contract lands, confirm live that updating **any persisted
field** of a step—including a title-only edit—preserves the existing review,
appends the change to the changelog, and sets
`drift_review.needs_review = true`. Also confirm `record_plan_drift_review` is
actually callable from a live Workshop-mode session (not just present in the
allow-list), that only a completed turn replaces the evidence and clears the
flag, and that an interrupted turn leaves the flag set for retry.

## Independent review — 2026-08-30

The six implementation phases are present, but the ticket is **not
acceptance-complete**. The review found four lifecycle defects that can make
Pulse report no work or postpone a repair even though plan drift remains:

1. **The due scan does not derive its step set from the canonical plan.**
   `CollectPlanDriftCandidates` scans only `planning/step_config.json`.
   A plan step with no config row is therefore invisible, even though the
   corrective invariant says every plan step with no `drift_review`, or with
   `drift_review.needs_review == true`, is due; a deleted step is represented
   by the workflow-level deletion-review flag described below. A
   workspace audit found 31 top-level plan steps across 11 workflows with no
   corresponding config row. The collector must parse `planning/plan.json`,
   enumerate its steps, and left-join `step_config.json` by step ID; a missing
   config row, missing review, or flagged review must produce a candidate.
2. **Read and parse failures are converted into a false clean result.** The
   collector returns `nil` for an unreadable or malformed
   `step_config.json`, which is indistinguishable from "nothing is due" to
   Gate. Change the API to return `(candidates, error)` and route the error as
   visible technical/platform work. Inaccessible review state must never
   suppress the module.
3. **A failed review can be marked done before its repair issue exists.**
   `record_plan_drift_review` persists a non-null record even when a check has
   `status: "fail"`, while candidate collection treats every non-null record
   as complete. The reviewer creates the corresponding
   `record_pulse_finding` only afterward. If that second call fails or the
   turn ends between calls, the failed step is no longer due and no repair
   item is guaranteed. Persist the failed result and linked finding
   atomically, or keep failed/unlinked reviews due.
4. **Review and repair are unnecessarily split across Pulse cycles.** The
   scheduler launches `plan_drift_review` in parallel with
   `technical_review`, while the drift reviewer has explicitly been given no
   repair authority. Technical review can read the backlog before the drift
   reviewer adds its handoff, postponing a safe repair to the next run. Run
   drift intake before technical maintenance, or let the same maintenance
   worker apply safe fixes in the same pass.

Acceptance requires regression tests for missing config rows, malformed
config, failed-review handoff atomicity, and same-pass safe-repair routing.
Until those pass, the earlier "all 6 phases complete" wording describes code
landing, not a completed operational contract.

## Agreed corrective contract — 2026-08-30

The review trigger is intentionally reduced to one deterministic condition:

```text
Any canonical plan step has no drift_review record
OR drift_review.needs_review == true
OR workflow_drift_review.needs_review == true
→ plan_drift_review is due
```

No compatibility check, changelog classification, cadence, or LLM judgment
may independently decide whether the module is due. Deterministic SQL, JSON,
schema, and reference checks remain evidence available *inside* the review;
they are not additional triggers.

The lifecycle is:

1. A newly-created step has no review record yet, so it is due.
2. **Every persisted update to any field of a plan step** sets
   `drift_review.needs_review = true` while preserving the previous evidence,
   reasoning, reviewer, and timestamp. Do not attempt to classify an update
   as material or cosmetic in Go; a description or title change can still
   alter meaning, and classification would create a new false-negative path.
   UI state that is not persisted in the plan naturally does not participate.
3. **Deleting a step** cannot flag the removed step, so the delete mutation
   appends a `step_deleted` changelog entry with the deleted step definition
   and its final `drift_review` snapshot, then sets
   `workflow_drift_review.needs_review = true`. This is the only case where
   copying the prior review into the changelog is necessary: the source record
   is about to be removed. It lets the agent inspect dangling routes,
   dependencies, reports, evaluations, learnings, and database artifacts.
4. Gate enumerates the canonical step set from `planning/plan.json` and
   left-joins `planning/step_config.json`. A missing config row, missing
   review, or `needs_review: true` all mean due; it also checks the one
   workflow-level deletion-review flag.
5. The same plan-mutation operation appends an immutable changelog entry
   containing the change ID, step ID, timestamp, actor, reason, and changed
   fields. It does **not** duplicate the previous review into each changelog
   row because that review remains in `step_config.json` until replacement.
   Appending the change and setting the flag must be one mutation contract.
6. The agentic reviewer reads the current step, its dependencies and
   artifacts, the preserved review, and only changelog entries after its
   `reviewed_through_change_id`. It uses this evidence to determine downstream
   effects and apply safe fixes.
7. Only a completed review replaces the evidence, sets
   `reviewed_through_change_id` to the latest consumed change, and sets
   `needs_review = false`. If the reviewer turn or required persistence
   fails, the flag stays true and the next Pulse run retries it. If a
   completed review creates an unresolved human/platform/fixer item, the
   review record and linked item must be committed atomically.
   A completed deletion review likewise advances the workflow-level
   `reviewed_through_change_id` and clears its flag. Failed/interrupted work
   leaves the appropriate flag set.
8. Scheduled Pulse and the standalone artifact-review slash command must
   call the **same** candidate collector, reviewer contract, safe-fix path,
   and completion writer. The slash command is a manual entry point into the
   module, not a separate checklist implementation. It selects the same
   canonical steps whose review is missing or flagged, consumes the same
   changelog evidence, and updates the same record only after completion. If
   no canonical step is due, it reports `no plan drift review due` and
   performs no duplicate review.

This preserves review history without duplicating it in the changelog or
making the changelog part of due-ness: a missing review or a step/workflow
`needs_review: true` flag triggers the work; the changelog explains what
changed and helps the agent determine its effects.

Acceptance must exercise both entry points against the same fixture: first
verify that scheduled and slash dispatch choose the same missing/flagged
steps and deletion-review work, and produce the same durable result. Cover a
new step, an ordinary update, and a deletion. Then rerun the slash command
and verify that it cleanly reports no work. The current standalone
`/review-artifact-drift` checklist does not yet provide this parity and must
be routed through the shared `plan_drift_review` implementation rather than
maintained as an independent behavior.

## Corrective contract — implemented (2026-08-30)

Fixed and tested all four independent-review lifecycle findings, plus the
"stale flag" redesign from the agreed corrective contract:

**Finding 1 (candidate scan omits steps without a config row):**
`CollectPlanDriftCandidates` now derives the canonical step set from
`planning/plan.json` (via `planStepIDsFromPlanJSON`, recursing into routing
sub-agent steps), left-joined against `planning/step_config.json` by step ID.
A missing config row is exactly as pending as a flagged one.

**Finding 2 (read/parse failure reads as "nothing is due"):**
`CollectPlanDriftCandidates` now returns `([]PlanDriftCandidate, error)`.
`validatePlanDriftRouting` requires `plan_drift_review` or `technical_review`
due when the scan itself fails (routing the failure as visible technical/
platform work, exactly like a failed deterministic intake signal);
`get_pulse_state(view="module")` surfaces the failure via a new
`plan_drift_candidates_error` field so Gate can see it.

**Finding 3 (a failed check can be marked reviewed with no repair item):**
`StepDriftCheck` gained an optional `finding_id`, required whenever
`status == "fail"`. `record_plan_drift_review` rejects a fail-status check
with no `finding_id`, and separately verifies the `finding_id` resolves to a
real, already-filed Pulse finding (`verifyStepDriftCheckFindingsExist`) — not
merely a non-empty string. The reviewer-turn guidance was reordered so
`record_pulse_finding` runs *before* `record_plan_drift_review` for every
failing check.

**Finding 4 (parallel dispatch races technical_review's backlog read):**
`plan_drift_review` is now its own preceding Pulse lifecycle step
(`pulseLifecyclePlanDriftReviewStep`), run and fully completed (`runStep`
blocks until the turn and its registered background child finish) before
`pulseLifecycleAgenticReviewStep` dispatches `technical_review`/
`strategic_review`. A new `pulseWorklistModulesDue` helper scopes the
due-check to specific modules instead of "any module due," and
`validatePulseDueModuleResultsFor` scopes receipt validation the same way —
needed so the new preceding step doesn't wrongly flag `technical_review` as
"missing a receipt" when it simply hasn't run yet.

**Stale flag model (supersedes the earlier null-on-edit design):**
`StepDriftReview` gained `NeedsReview bool` and `ReviewedThroughChangeID
string`. `clearDriftReviewAfterPlanUpdate` no longer nulls the record or
shares `description_reviewed`'s field classifier — it sets
`NeedsReview = true` on *any* persisted field change (title included; no
field is classified as cosmetic), while leaving `Checks`/`ReviewedAt`/
`ReviewedBy`/`ReviewedThroughChangeID` untouched, so a step's last real
review stays available as evidence even while stale.
`record_plan_drift_review` accepts an optional `reviewed_through_change_id`
and always fully replaces the evidence while explicitly clearing
`NeedsReview`. `CollectPlanDriftCandidates`'s pending check became "no record,
or `NeedsReview == true`." The reviewer-turn guidance (`plan-drift-review.md`)
was rewritten to read the preserved prior evidence plus only the changelog
entries after `reviewed_through_change_id`, instead of re-auditing from
scratch.

**Merged alongside a concurrent session's PLAT-259 work** (routing/branch
step types) landing on `plan_drift_candidates.go`/`plan-drift-review.md` at
the same time: preserved the `StepType` precompute and the routing-only
`route_structural_isolation`/`route_eval_pairing` judgment checks through a
manual 3-way merge, verified line-by-line rather than taking either side
wholesale.

**Regression found and fixed during merge verification:** this ticket's own
earlier Phase 5 push had reworded `pulse-gate.md`'s "Select **at most two**
due modules" line to add an agentically-judged/technical_review/
strategic_review clarification, which silently broke
`TestStrategyAuditorGuidanceRequiresLongitudinalEvidenceAndReadOnlyHandoff`'s
exact-substring check (pre-existing, predates this session's corrective
work). Reworded to `"Select **at most two** due modules, chosen agentically
from..."`, preserving the literal expected substring while keeping the
`plan_drift_review` carve-out clarification.

**Verification:** `GOWORK=off go build ./...` clean (the worktree's own
`go.mod` replace directives resolve correctly without the machine-level
`go.work`, which does not list this worktree's path). Full
`go test ./cmd/server/... ./pkg/orchestrator/agents/workflow/step_based_workflow/...`
green except two **pre-existing, confirmed-unrelated** failures left
untouched: `TestEveryPulsePlatformTicketIsLinkedFromTheRegister` (the register
links `plat-248.md`/`plat-249.md`, neither of which exists on disk — a gap in
someone else's ticket filing, unrelated ticket range) and
`TestUpgradeDedicatedPulseSchedulePromptShape` (fails against
`workflow_version_upgrades.go`, confirmed byte-identical to `origin/main`,
i.e. not touched by this merge at all). `gofmt` clean on every touched file.

**Follow-up not built in this pass** (explicitly out of scope, per the
user's own "finish 1-6, treat 7 as its own follow-up" direction given while
this work was in flight — should be filed as a fresh, clearly-scoped ticket
rather than folded into another PLAT-258 mega-push):
- Point 7/8 of the agreed corrective contract: routing the standalone
  `/review-artifact-drift` slash command through the same candidate
  collector/reviewer contract/completion writer scheduled Pulse uses, so the
  two entry points can never diverge.
- The workflow-level `workflow_drift_review` flag for covering deleted steps
  (added to the ticket's design after this implementation pass was already
  underway): a `step_deleted` changelog entry carrying the deleted step's
  final `drift_review` snapshot, plus a workflow-level `needs_review`/
  `reviewed_through_change_id` pair Gate's due-check also has to consult.

## Review decision — combine plan-drift review and repair (2026-08-30)

The handoff-only first version is superseded. `plan_drift_review` must follow
the same combined maintenance model already used by Technical Review: one
retained agent reviews its due scope, applies every safe workflow-owned fix,
verifies the result, and records the terminal evidence in the same turn.
Message-sequence continuations are allowed only when later reasoning genuinely
needs another turn; they are not a mandatory boundary between review and fix.

The module owns plan-change drift end to end:

1. Read the shared candidate collector, preserved prior review, and changelog
   entries after `reviewed_through_change_id`.
2. Run the deterministic and judgment checks against the current step and its
   affected dependencies.
3. Apply coherent, safe workflow-owned repairs immediately across the exact
   affected surfaces: plan/config, downstream contracts, validation, DB/report,
   evaluation, learnings, KB, scripts, and route wiring as warranted by the
   evidence.
4. Verify each repair proportionally and record the check as `fixed`; a
   same-turn repair does not need an intermediate Pulse issue merely to hand
   work to another agent.
5. Route only genuine human decisions, platform-owned defects, external
   actions, or unsafe/ambiguous redesigns to durable issues. A failing check
   may clear `needs_review` only after that durable handoff exists.
6. Clear the step/workflow `needs_review` flag only after the complete review,
   safe-fix, verification, and required-handoff set is durably recorded.
   Interrupted or failed work stays due.

Ownership must remain non-overlapping. Plan Drift owns repairs caused by a
plan change and its dependent artifacts. Technical Review owns unrelated
runtime/tool/store/report/evaluation/cost defects and must not re-review a
Plan Drift root already fixed or durably routed in the same Pulse run.

This decision also removes the scheduler workaround that depended on
Technical Review already being due at Gate time. A Plan Drift-only Pulse run
must be capable of reaching zero safe plan-drift debt without waiting for a
second Pulse cycle.

Acceptance must prove, in one scheduled run and through the shared manual
entry point: review discovers a repairable drift; the same retained agent
changes the relevant files; verification passes; the drift review is recorded
as fixed and cleared; no duplicate Technical Review issue is created; and a
failed/interrupted repair leaves the candidate due for retry.
