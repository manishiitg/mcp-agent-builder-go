[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-115 — decouple Gate/Review+Fix/Finalize from every run's own session

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `implemented` — platform capability shipped and tested; not yet adopted by any real workflow |
| Last synchronized | `2026-08-16` |

- **Priority:** P2 — not a bug fix, a structural option. Adopting it on a
  frequently-run workflow is what actually prevents recurrence of two real
  incidents this register already tracks.
- **Owner:** scheduler Pulse orchestration (`runPostRunMonitor`,
  `scheduler.go`), workflow manifest (`workflow_manifest.go`), schedule
  tools (`interactive_workshop_manager.go`, `workflow_schedule_tools.go`)

## Why

Every scheduled workflow run has always been followed by one continuing
session that runs Gate → Review+Fix → Finalize before the next thing can
fire. For a workflow run frequently enough that sessions get reused across
many hours, this directly caused two real, separately-investigated bugs:

- **PLAT-113**: a Pulse-adjacent session stayed open for ~5 hours and
  deadlocked on its own input lane, blocking the next scheduled run.
- **PLAT-114**: the same kind of long, reused session evicted its own
  background agents' completion records once a 200-event UI cache filled
  from later activity in the same session — verified live while checking the
  quality of a completed Pulse pass, where the only way to reconstruct what
  actually happened was reading the git diff by hand.

Both trace to the same root shape: heavy review work sharing a session with
the run it's reviewing. The fix is a genuine split: every run keeps a fast,
reliable backup+notify pass; the full review runs separately, on its own
cadence, in its own session.

## What shipped

**A workflow-level toggle** — `post_run_monitor_mode` on `WorkflowManifest`
(`"per_run"`, the default and today's exact unchanged behavior, or
`"periodic"`). Under `"periodic"`, every run's own pass runs only backup +
a short factual notification (`postRunMonitorLightweightFinalizeStep`) — no
Gate, no reviewers, no Fixer, and no findings narrative, since none ran.

**A schedule-level marker** — `PulseReviewOnly` on `WorkflowSchedule`, which
reuses the exact plumbing the manual "Run Pulse now" trigger
(`TriggerPulseNow`) already exercises: skip workflow execution, set
`sctx.PulseOnly`, run the full Gate/Review+Fix/Finalize chain. Unlike the
manual trigger, it never pins Gate to one run folder.

**Gate reviews a backlog, reasoning about what's new itself** — no new
cursor or queue table, by deliberate design decision made in conversation
before implementation. `postRunMonitorBacklogGateStep` hands Gate a listing
of every currently-existing `runs/iteration-N/` folder (name, status,
started_at, completed_at) — `iteration-0` only when its own metadata is
terminal, since it's the live/reused slot, never a stable identity across
time. Gate compares this listing against `get_pulse_state`'s own
`last_checked_at` per module to decide what's genuinely new, the same kind
of judgment call it already makes for mode selection — Go's only job is
handing over what currently exists on disk. `pulse-gate.md` also has Gate
check its own review interval against `run_retention_count`: rotated run
folders beyond that count are permanently deleted by the executor
(`controller_run_manager.go`), so a periodic pass reviewing less often than
folders get deleted from under it would silently see a partial sample.

**Both schedule tools gained `pulse_review_only`** — `create_schedule` and
`update_schedule`, on both the workshop-facing (`interactive_workshop_manager.go`)
and chat-facing (`workflow_schedule_tools.go`) tool surfaces, threaded with
the same `setX`/pointer pattern PLAT-097 established for `messages`. A
`PulseReviewOnly` schedule never runs the workflow, so `group_names` is
exempted at all three places that otherwise require it —
`create_schedule`'s own check, `update_schedule`'s final unconditional
revalidation, and `ValidateManifest` itself; a test written against the
real implementation (not just the tool-layer guard) caught the third one
before it shipped.

**`update_workflow_config` gained `post_run_monitor_mode`** — the write side
needed to exist for any of the above to be reachable through a normal turn,
not just by hand-editing `workflow.json`.

**Rollout reuses the existing contract-upgrade mechanism, not a new one.**
`/pulse-setup` now considers periodic mode at setup time for new workflows.
For existing workflows, a new upgrade rung (`upgrade-periodic-pulse-review`,
contract version 1.0.26) reads real run history (not nominal cron alone),
and only if a workflow runs frequently enough for the split to matter,
creates the review schedule and switches `post_run_monitor_mode` in the
same turn — never one without the other, since a workflow left in
`"periodic"` mode with no review schedule yet gets a lightweight pass every
run and a full review from nothing, ever. A workflow that reviews cheaply
today is expected to say so and stay on `per_run`, the same "keep the
current model with a stated reason" pattern every other contract upgrade in
this register already allows.

## Not fixed here

- **No real workflow has adopted `"periodic"` mode yet.** This ships the
  capability; nothing has exercised it under a real, frequent schedule.
- **Retention and cadence are not automatically kept in sync.** Gate's
  self-check surfaces a mismatch as a `workflow_review`/`llm_ops_review`
  finding for the normal Fixer path to correct; nothing enforces it
  directly.
- **A workflow's own `post_run_monitor_mode` and its schedules'
  `PulseReviewOnly` markers can be set independently and inconsistently** —
  e.g. `"periodic"` with no `PulseReviewOnly` schedule, or vice versa.
  Nothing currently detects or flags that specific combination outside of
  the upgrade rung's own atomic pairing at setup time.
- **`update_workflow_config`'s new field has no dedicated unit test** — it
  follows the exact same manual-JSON-manipulation pattern as its
  already-untested sibling `post_run_monitor` in the same file; the
  underlying read-side logic it writes (`PostRunMonitorIsPeriodic`) is
  thoroughly tested instead.

## Verification

- `go build ./...` clean throughout.
- New tests for every decision point: `TestPostRunMonitorIsPeriodicFailsSafeToPerRun`
  (unknown/empty values never accidentally opt out of review),
  `TestPostRunMonitorUsesLightweightFinalizeRequiresRealEvidence` (the
  periodic review pass itself must never take the lightweight path),
  `TestLightweightFinalizeStepNeverRunsGateOrPublishesFindings`,
  `TestPulseReviewOnlyScheduleContractMirrorsManualPulseTrigger`,
  `TestPulseReviewBacklogSummaryExcludesNonTerminalIterationZero`,
  `TestBacklogGateStepDefersWhatsNewReasoningToGate`,
  `TestCreateAndUpdatePulseReviewOnlyScheduleSkipsGroupNamesRequirement`
  (against the real `CreateSchedule`/`UpdateSchedule` implementations, not
  just the tool-layer guard — this is what caught the `ValidateManifest`
  gap), `TestUpgradePeriodicPulseReviewPromptShape`, and
  `TestUpgradeQueriesNeverNamePlatTickets`.
- Every existing `workflowVersionUpgradePlan`-dependent test updated for the
  new terminal rung and reverified passing.
- Full suite reverified after every part: 26 failures throughout, byte-for-byte
  identical to the baseline captured before this work started — zero new
  failures introduced by any of six incremental changes.
- **Caught mid-implementation, not shipped**: an early test draft for the
  durable background-agent log (a related PLAT-114 follow-up surfaced during
  this same investigation) wrote a real row into social-media's actual
  production database before proper sandboxing was added — caught by
  inspection, deleted, and the test rewritten. Recorded on PLAT-114, not
  repeated here, but the same class of risk (a Go-side path resolution that
  walks up from cwd to a real `workspace-docs` directory when a workspace
  env var isn't set) is worth remembering for any future test in this file.
- Internal ticket numbers were caught and removed from every live,
  agent-facing prompt string added by this change (`workflow-tools.md`, the
  new upgrade rung) after explicit review — this codebase runs on
  individual users' own machines, and an internal tracking number in a
  prompt those users' agents execute is meaningless noise to them, not
  context. One pre-existing violation in an unrelated, already-shipped
  upgrade rung (`upgradeLearningsLockAudit`, PLAT-055) was found by the new
  regression test and fixed alongside it.

## Acceptance

- An unmodified workflow (no `post_run_monitor_mode` set) is byte-for-byte
  unaffected — proven by the fail-safe test on the mode-reading accessor.
- A `"periodic"` workflow's normal run schedule never runs Gate, reviewers,
  or Fixer, in any session.
- The periodic review pass reviews the real current backlog, decided by
  Gate's own reasoning against durable state, never a Go-selected single
  folder.
- Creating or updating a `PulseReviewOnly` schedule never requires
  `group_names`, at the tool layer, the Go implementation, and manifest
  validation alike.
- The upgrade rung never leaves a workflow in `"periodic"` mode without a
  paired review schedule, or vice versa.
