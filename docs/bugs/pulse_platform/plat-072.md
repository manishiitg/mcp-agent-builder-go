[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-072 — `external_action_required` has no exit path, so solved problems keep being re-reported as open

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — the defect this ticket originally described was already fixed; the reason it looked open is the real defect |
| Last synchronized | `2026-08-10` |

- **Priority:** P1 — stale findings actively mislead, and the count only grows
- **Owner:** Pulse finding lifecycle (`external_action_required` disposition)
- **Found on:** cross-workflow, while triaging the 81 open platform findings

## What this ticket was, and why it changed

It was originally filed as *"reused `iteration-0` makes per-run cost unattributable and overwrites run provenance"*, on the strength of six workflows independently reporting it: social-media (07-29), build-in-public (08-03), upwork (08-04), hetznerssh and rtslatency (08-05), linkedin (08-06). Six independent reports of the same defect is normally strong evidence.

**It was already fixed.** Commit `0f6519640`, *"Fix cost ledger run identity across rotation"*, **2026-08-06**, and comprehensively — not only the storage format but the readers and the rotation path:

```
cost_storage.go (server + orchestrator)   execution_id as immutable record identity
virtual-tools/tool_costs.go               the cost reader
evaluation_score_storage.go               the same fix for eval scores
controller_run_manager.go                 ArchiveRunCostPaths on rotation
review/ops-review.md                      the reviewer contract
execution_identity_cost_test.go           regression test
```

Rotation archives rather than deletes — social-media's `iteration-254` / `iteration-253` folders are those archives — and the code states the intent directly: *"Cost records keep the UUID as their immutable identity. Rotation only updates their human-readable archived path, so the next iteration-0 run cannot inherit historical spend."*

**Every one of the six findings is dated on or before that commit.** They are frozen descriptions of a solved problem.

## The real defect

`external_action_required` is a state with **no exit in either direction**:

- **It cannot close.** Nothing sweeps these findings when a platform fix ships. `resolve_run_concern` is limited by contract to *"acknowledgment or rejection"*, and Pulse is correctly read-only on platform matters.
- **It cannot reopen or self-correct.** The concern upsert flips a stale row back to `open` only for `resolved`, `awaiting_verification` and `awaiting_run`:
  ```sql
  status = CASE WHEN run_concerns.status IN (?, ?, ?) THEN ? ELSE run_concerns.status END
  ```
  `external_action_required` is deliberately absent, so re-observation leaves it untouched — the "suppresses unchanged rediscovery" property. But the same property means a finding whose problem was *fixed* is equally frozen.

So the board only grows: 13 findings from July, 68 from August, and the two newest both duplicate platform issues already known.

## Why this is worse than an untidy list

This ticket is its own evidence. Working with a full day of context on this codebase, six corroborating reports were enough to file a P1 for work completed four days earlier. The findings read as current because nothing about them says otherwise. The operator caught it — *"we already solved this before right, we had created a format for this, we maintain a uuid?"* — from memory, which is not a control that scales.

Every stale finding on that board is a live trap for the next reviewer, human or agent. And the compounding cost is real: a triage sweep attempted here matched only 11 of 81 by theme, and misattributed several of those, so bulk closure by pattern is not available either — the stale ones have to be read.

## Suggested fix

1. **Give the state an exit.** When a platform ticket reaches `implemented`, sweep findings whose `reason_code` / `external_owner` matches and dispose of them with the ticket id and commit as evidence. Not a bulk clear — an evidence-stamped closure, so the trail survives.
2. **Let it self-correct like every other state.** Adding `external_action_required` to the reopen list is a one-line change and makes closure safe: if the fix did not hold, the next observation reopens it. That safety net is exactly why closing these is low-risk, and it is the only reason bulk closure would be defensible at all.
3. **Record what a finding was reported against.** These carry no platform-version or commit context, so nothing distinguishes "still true" from "was true in July". A stamp at filing time would let a sweep answer this mechanically instead of by reading.

## Not doing

Closing the six cost-attribution findings by hand right now. They are stale and can be closed, but doing it one-off leaves the mechanism unbuilt and the other 75 untouched — and the sweep is the thing worth having.

## Acceptance

- A platform finding whose owning ticket is implemented can be closed with evidence, without a human remembering the fix happened.
- A wrongly-closed finding reopens on next observation rather than staying closed.
- The 81 currently on the board are triaged against the PLAT register, with stale ones closed and genuinely open ones left visible.
