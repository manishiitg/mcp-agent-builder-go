[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-017 — scheduler success leaves durable workflow metadata running

| Coordination | Value |
|---|---|
| Assigned agent | `Unassigned` |
| Ticket state | `blocked_on_reproduction` |
| Last synchronized | `2026-08-04` |

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

