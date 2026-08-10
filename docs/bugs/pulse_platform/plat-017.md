[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-017 — scheduler success leaves durable workflow metadata running

| Coordination | Value |
|---|---|
| Assigned agent | `Unassigned` |
| Ticket state | `reproduced; implementation boundary open` |
| Last synchronized | `2026-08-09` |

> Claim this ticket in this file before implementation. During active work,
> update this fragment rather than the shared index; synchronize the index
> once at handoff, review, or completion.


- **Priority:** P1
- **Owner:** scheduler/workflow terminal-state persistence and reconciliation
- **Source findings:** `HARNESS-PULSE-RUN-STATUS-MISMATCH` and
  `HARNESS-SCHEDULER-CHILD-STATUS`
- **Source database:** `Workflow/social-media/db/db.sqlite`
- **Recorded state:** `external_action_required`; the two IDs describe one
  terminal-state boundary and must not become two repair projects
- **Problem:** the discovery children completed and the scheduler reported
  success, while `runs/iteration-0/default/run_metadata.json` remained
  `status=running`.
- **Distinction from PLAT-004:** PLAT-004 prevented success while required work
  was still running. PLAT-017 concerns stale durable workflow metadata after
  genuine completion.
- **Impact:** Pulse and later consumers cannot choose one authoritative run
  status; the same completed run can appear successful in schedule history and
  active/incomplete in workflow evidence.
- **Current state:** open. Reproduce on the current binary before choosing
  whether scheduler completion must finalize run metadata directly or a shared
  reconciler must atomically persist both terminal projections.
- **Acceptance:** after one completed, failed, canceled, and interrupted
  scheduled fixture, scheduler history and `run_metadata` agree on terminal
  state, completion time, and owning execution identity. A partial write is
  retried or surfaced as failure rather than leaving contradictory success.

## Reproduction on the current binary — 2026-08-09 (Upwork)

The blocking reproduction is supplied. A second durable projection has the same
defect: `pulse_review_log` rows are written **only** by an agent
(`recordPulseReviewOnDB`), so a pass that dies between "review started" and
"review recorded" strands its row at `status='running'` permanently.

Three stranded `workflow_review` rows were found in
`Workflow/upwork/db/db.sqlite`, from three different runs on a single day:

| `review_run_id` | recorded_at |
|---|---|
| `schedule-manual--78ba88d0_1786169154458106000` | 2026-08-08T07:55:21Z |
| `workshop-background-task-1786193263563776000` | 2026-08-08T12:59:19Z |
| `schedule-cron--78ba88d0_1786203048970002000` | 2026-08-08T17:35:52Z |

Historic distribution in that database: `completed` 42, `failed` 10, `running`
3, empty 1 — so this is a steady leak, not a one-off. The 17:35 row also
carried an empty `verdict` alongside 5 verifications, which is the same surface
PLAT-046 hardened at the tool boundary.

**Root cause of the leak, distinct from the metadata half:** `Start()` only ever
reconciled `pulse_final_command_state` (via
`finalizeAllUnresolvedPulseFinalCommands`). Nothing swept the reviewer
projection, so an interruption that was correctly reconciled on one table was
silently left inconsistent on the other.

### Partial fix shipped under PLAT-054

`finalizeAllRunningPulseReviewLogs` now runs in the same startup sweep. It is
deliberately narrow: only rows already `running` are touched, only
status/verdict are rewritten, and `finding_count` / `verification_count` the
dead pass genuinely recorded are preserved. Tests:
`TestFinalizeAllRunningPulseReviewLogsClosesStrandedRows` and
`...ToleratesMissingTable`. The three Upwork rows above were reconciled directly
because that server is intentionally stopped pending a rebuild.

**Still open, and still the core of this ticket:** the `run_metadata.json`
half. Startup reconciliation closes stranded rows *after the fact*; it does not
make scheduler history and workflow run metadata agree at completion time. The
implementation boundary question in *Current state* is unchanged.

