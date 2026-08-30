[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-258 — Dedicated `plan_drift_review` Pulse module (design + phases 1-2 in progress)

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `design complete; phase 1 implemented; phase 2 in progress (1 of 9 deterministic checks built); phases 3-6 not yet built` |
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

**Sequencing (agreed):** Phase 1 (this ticket) → Phase 2 (build the deterministic checks as real Go functions) → Phase 3 (a `record_plan_drift_review` tool, since `planning/` is FolderGuard-blocked-write for normal sessions — the reviewer needs a purpose-built recording tool with the same privileged write path `update_step_config` already uses, not a raw file write) → Phase 4 (module registration + reviewer turn content, the real authoring work) → Phase 5 (frontend — currently hardcoded to a 2-module layout) → Phase 6 (cutover: remove the now-redundant drift-handling folded into `technical_review`, once the new module is proven working).

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
an agent to skip"). Only the judgment checks (Group 3, phase 4) genuinely need
a Pulse reviewer turn and the `record_plan_drift_review` tool (phase 3) to
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

Remaining Group 1/2 checks (2-9, minus the retired #8) are not yet built.

## Verification

Phase 1: `go build ./agent_go/... ./workspace/...` clean. `go test
./agent_go/pkg/orchestrator/agents/workflow/step_based_workflow/...` — full
package suite green, including 2 new tests
(`TestClearDriftReviewAfterPlanUpdate`,
`TestClearDriftReviewAfterPlanUpdateSkipsTitleOnly`) mirroring the existing
description-review test pair exactly, plus the pre-existing
`TestArtifactReviewNotices`/`TestMergeAgentConfigFieldsCoversEveryField`
updated and passing.

Phase 2 (Check 1): 8 new tests — 4 for `extractReportQueries` (all three quote
styles, dedup-preserving-first-occurrence-position, escaped-quote handling,
no-match case) and 4 for `CheckReportQueryCompatibility` (pass on matching
schema, fail on a dropped column, pass when no report exists, and a dedicated
safety test proving a report embedding an `UPDATE` statement never actually
mutates the database — the `query_only` guard holds). `gofmt`/`go vet` clean,
full package suite still green.

## Reverify

Once later phases land: confirm live that editing a step's description/context_dependencies/validation_schema nulls `drift_review` in `step_config.json`, and that a title-only edit does not.
