[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-073 — working list of the remaining external_action_required board

| Coordination | Value |
|---|---|
| Assigned agent | unassigned — working list, not a single defect |
| Ticket state | `open` — tracking document for the post-triage backlog |
| Last synchronized | `2026-08-10` |

Not a bug report. This is the durable list PLAT-072's triage sweep produced,
so the remaining work is assignable — to another session, a parallel agent, or
a future pass — without re-deriving it from `pulse_close_stale.py --list`
output again. Regenerate the raw list with:

```
python3 scripts/pulse_close_stale.py --list
```

## Standing instruction for anyone picking up a cluster

Every cluster below was grouped by keyword pattern, not verified line by line.
**Read the full finding text before fixing anything** — three separate
clusters today turned out to be materially different from their first
impression on inspection (A: a classifier gap became a missing-handler-wiring
bug with 10x the blast radius; B: a "cost telemetry" bug turned out to be a
display-only field driving no behavior, removed rather than fixed; several D
items are workflow eval-spec edits, not platform code). Use
`pulse_close_stale.py --close --ticket <id> --evidence "<why>" --fingerprint
<fp>` to close a finding once its fix is live — never before, and never
without a fingerprint-specific reason.

## A. Telemetry lies about success/failure (3) — FIXED, not yet live

`mcpagent` commit `8c07adf`. `HandleCustomExecute` — the HTTP layer serving
every workflow step's `execute_shell_command` and other bridge tools — never
applied the canonical-failure check, so `Success` was unconditionally `true`
regardless of payload content. Close these three once the fix is live:
`e3dcf327c98828be` (build-in-public), `1cbf04498d1fa813` (social-media),
`431b0ce8eaf41d34` (upwork).

## B. Cost/context telemetry (6) — 4 closed/fixed, 2 remain open by design

Closed by removal (`agent_go` `e0e0494bf`): `context_usage_percent` findings —
the field drove no behavior, was actively misleading, removed rather than
fixed.

See **PLAT-081** for the remaining three, triaged as one real bug plus two
documentation gaps:

- `e6be98dfd6f4d639` (build-in-public) — **real bug, fixed**:
  `ApplyModelUsageToPhaseTokenUsageFile` merged the coding agent's
  session-*cumulative* usage onto the ledger every chat turn instead of
  overwriting it (the call site's own comment already said "overwrites... with
  the full cumulative history" — the implementation just didn't match), so the
  persisted total compounded turn over turn and got mispriced whenever the
  session's active model changed mid-session. PLAT-081 now persists one
  cumulative checkpoint per chat and adds only each turn's delta, preserving
  totals from other chats using the same model.
  The finding's second claim ("`input_cost_usd` zeroed for 2.8M tokens") is
  **not a bug** — those tokens are cache-served and correctly priced under
  `cache_cost_usd` instead; documented in `file-layout.md`.
- `43b988fe2ef952f3` (tectonicusadaytrading) — **not a math bug**.
  `costs/phase/daily` and `costs/execution` cover different, non-overlapping
  call sets by design (`costs/phase` = planning phase + workflow-builder chat
  only); there was simply no documentation saying so. Now documented in
  `file-layout.md` rather than left to look like a 3.1x undercount.
- `a8ab091308579946`, `e717a5e1a962a81f` (upwork) — **real gap, left open,
  design-only**. Not explained by `e0e0494bf` (checked; unrelated). The data
  model to fix this already exists (`pkg/costledger`'s per-call
  `SummarizeExecution`) but isn't exposed via any tool/endpoint — needs a
  design decision on the right surface, not a mechanical patch.

## C. Changelog / provenance not recorded (9) — FIXED (4/6 sites), not yet live

See **PLAT-074**. Confirmed PLAT-056 shape: one shared writer
(`completePlanChangelogEntry`), 6 of 16 mutation call sites never fed it real
diff/snapshot data. 4 of those 6 (`delete_plan_steps`, the 5-tool `add_*_step`
adder, `delete_todo_task_route`, `migrate_message_sequence_code_items`)
already had the real content in scope and just weren't wiring it through —
fixed via one fallback in the writer plus one caller change. The other 2
(`add_todo_task_route`, `update_todo_task_route`) needed new before/after
route capture — also fixed. Tests added, 22-failure baseline unchanged.

**Not yet reverified live** — needs a restart, then a run that exercises one
of the five fixed tools before closing fingerprints. Fingerprints to close
once verified: `02bbf615` `cb2bf4b1` `b5c2bfa8` (build-in-public), `7607952e`
`17e6f19a` (linkedin), `db76bb3a` (rtslatency), `ae0b8a1b` (social-media),
`69f4a8a7` (tectonicusadaytrading), `f4468936` (upwork) — verify each against
which specific tool call produced it before closing with
`pulse_close_stale.py`, since the fix is mechanism-level, not per-finding.

## D. Eval scoring ambiguity (5) — triaged; platform items implemented

- `28e793fd852b3b43` (social-media) — already fixed by PLAT-016. Current Go has
  `json:"score"` without `omitempty`, its captured-zero regression test passes,
  and the current report preserves `"score": 0`.
- `8e266151da4a0e25` (social-media) — the design decision is already implemented
  by PLAT-015: skipped rows and report entries carry `skipped=true`, while a
  genuine zero has `skipped=false`, `score_captured=true`, and a real max score.
- `c85dfe0208e94aef`, `53316f02fa209f61` (build-in-public) — these are
  `evaluation_plan.json` content issues (a weak CHECK, a stale rule
  superseded by the actual `main.py`), not platform code. Workflow-level
  fixes, arguably misfiled as `external_action_required` — flag for
  `workflow_review` rather than a code fix.
- `9495aef3dab65c42` (linkedin) — implemented as PLAT-075. The batch controller
  finalized target metadata after auto-evaluation; it now finalizes the target
  execution before evaluation starts. Runtime reverify pending.

## E. Schedules skipped / mis-reported (3) — triaged; one platform bug fixed

`7c4ac152` (build-in-public) mixed explained daily skips with two genuinely
silent Sunday omissions. The daily occurrences have durable `skipped_paused`
or `skipped_busy` decisions. The Sunday schedules had older persisted tracking
windows but no fire-decision rows; on restart `LoadSchedule` reset their cursor
to `now-30s` and advanced directly to next Sunday. That bootstrap defect is
[PLAT-080](plat-080.md), implemented with runtime reverify pending.

`3e42ae71` (linkedin) is explained by the durable ledger (`skipped_paused` on
2026-08-04), which the reviewer did not consult. `3565d07c` is a stale
projection claim, not supported by current schedule state; reverify through
`list_schedules` and the ledger rather than the launch-only history file.

## F. Tools unavailable / limits (8) — triaged, closer to 1 real open bug

- `ad5c92dd` (rtslatency, `get_api_spec` array-of-strings `tool_name`) —
  **already fixed** (`mcpagent` commit `ea60eb2`, predates the finding).
  `handleGetAPISpec` (`mcpagent/agent/code_execution_tools.go:24-55`) already
  handles both a JSON-array string and a real `[]interface{}`. Reverify live,
  then close.
- `dd9ede3c` (upwork, `agent_browser` snapshot overflow) — **real bug, not
  PLAT-062-adjacent** (that ticket is an unrelated prompt-text defect). See
  **PLAT-078**: two contributing mechanisms, one fixed. `setupExecutionFolderGuard`
  now grants read access to `<workflow-root>/tool_output_folder`
  (mcpagent's own spill target for any bridge tool result over its 128 KiB
  inline cap), closing the "spilled copy is structurally unreadable" half.
  Still open: `agent_browser`'s `snapshot` command itself has no output-size
  cap — a snapshot that overflows Claude Code's own (smaller) MCP result cap
  can still spill to a location this fix doesn't cover. Do not close this
  fingerprint on the folder-guard fix alone.
- `90348ad2`, `22fa5102` (tectonicusadaytrading, direct `sqlite3` blocked) —
  confirmed working-as-designed. `90348ad2` is already merged/resolved into
  `22fa5102` as the canonical survivor; close `22fa5102` via
  `resolve_run_concern(status="rejected")` (see cluster H).
- The remaining ~4 members of this 8-count cluster have no per-fingerprint
  detail recorded on this board — not triaged, not invented.

## G. Learnings/KB metadata wrong (6) — implemented / deduplicated

PLAT-076 fixes `03f6ed727fb67c9a`, `9d1c0fe414871e37`, and
`10eb995c50fb52c0`: saved-script runs are attributed to `scripted`, current
step identity is refreshed on every metadata write, learning changes use real
artifact-tree hashes, and byte-identical scripts no longer increment version
or relearn counters. `dd2a48047c4d7993` is already covered by PLAT-061 and the
v1.0.22 migration; the dead field is absent from the current workflow. Runtime
reverify remains before closing any workflow-local row.

## H. External sites blocking (3) — RECLASSIFY, not a platform bug — mechanism documented

`23cfc840`, `0d39debd`, `659fd419` (tectonicusadaytrading) — Reddit 403s,
LunarCrush failures. The outside world, correctly reported. Close as
not-a-defect rather than leaving them to age; they will never resolve as
platform work.

**`pulse_close_stale.py` is the wrong tool for this** — it only ever writes
`status='resolved'` ("a platform fix shipped"), which is deliberately outside
its scope for "this was never a platform defect" (see its updated docstring).
The correct mechanism is `resolve_run_concern(status="rejected", note=...)`,
called from within a live Pulse/workshop session — confirmed live in the DB:
`0d39debd`/`659fd419` are already `resolved` via a dedup/consolidation merge
into `23cfc840`, and `23cfc840` itself carries a `resolution_note` explaining
the Reddit 403 is the outside world, not a platform bug, but is still sitting
at `status='external_action_required'` pending the actual `rejected` call.

## I. Human-input / backlog lifecycle (5) — FIXED (2 of 3 code items), not yet live

See **PLAT-077**.

- `bed0388b` (upwork, `complete_pulse_review` empty verdict) — already fixed
  (commit `b541b520d`, predates the finding). Reverify live, then close.
- `cf457bdd` + `7602e2ac` (upwork / social-media) — same root cause, fixed
  together: `answerReportHumanInput`/`dismissReportHumanInput`
  (`cmd/server/report_human_inputs.go`) wrote their `UPDATE` with no status
  guard, so a concurrent writer (the documented chat/schedule concurrency
  contract, not just an in-process race — the package mutex only serializes
  goroutines in this one process) could silently revert an already-consumed
  row back to `answered` while leaving its `consumed_at`/`outcome_summary` in
  place. That's exactly the impossible state `7602e2ac` observed live
  through `loop_closure`. Added a `NOT IN (...)` status guard plus a
  `RowsAffected` check (erroring instead of silently no-op'ing) to all three
  transition functions (`answer`/`dismiss`/`consume`). Tests added.
- `f2cbf9a1` (rtslatency, two fingerprints under one harness finding) —
  fixed: `migrateDuplicatePulseFindingIdentities`
  (`pulse_finding_lifecycle.go`) only ever grouped duplicates by a
  human-assigned `finding_id`; a harness finding split before either row
  acquired one (this exact case) was invisible to it. Now also groups by
  `target_key` when `finding_id` is empty, scoped to `IssueKindHarness` rows
  only. Tests added (including a negative-scope guard so a coincidental
  target_key match on non-harness findings is never merged).
- Not yet reverified live — needs a restart, then exercising
  answer/dismiss/consume and a harness-finding split before closing
  fingerprints.

## J. Routing / run identity (3) — triaged; no new shared-code patch

`05a81ca02...` (hetznerssh) remains linked to PLAT-066. The incident is real,
but a later 2026-08-10 LinkedIn run on the current binary logged the supplied
route map, seeded `route_selection.json`, and consumed that exact route. This
is runtime-reverify evidence, not proof of the Hetzner incident's root cause;
keep PLAT-066 open until the same Hetzner route is reproduced after restart.

`b3ac1ae3` (build-in-public) is workflow/worktree state: `git status` reports
`D CLAUDE.md` while the tracked HEAD blob exists. Restoring or accepting that
user-owned deletion belongs to workflow review, not the platform runtime.

`5bf5513f` (rtslatency) is a settled workflow schema migration. The workflow's
own `db/README.md` records the approved 2026-08-05 decision to change
`latency_baselines` from `PRIMARY KEY(env)` to a history-bearing key and notes
that readers and writers must move atomically. Route it to a migration-capable
workflow/version upgrade; it is not a scheduler or shared persistence defect.

## K. Unclustered (3) — triaged; one real bug fixed

See **PLAT-082**.

- `ff9832cc` (build-in-public) — already fixed in
  `multi-llm-provider-go` `233f3c6`: Claude `execution_low` is Haiku 4.5 at
  medium effort. Runtime reverify remains.
- `0b7d9d4a` (social-media) — real and distinct from cluster A. Sync child
  execution swallowed its Go error into an `ERROR:` string before the async
  owner saw it, so the owner truthfully classified the false nil-error signal
  as completed. Fixed by preserving the typed error through the internal
  boundary; synchronous tool callers still receive a readable
  `success:false` envelope.
- `5e248d9e` (social-media) — not a defect. Full-run rotation moves the entire
  prior `iteration-0` to a retained `iteration-N`; the finding inspected only
  the fresh active folder and missed the backup tree.

## Suggested split for parallel work

Independent, non-overlapping code areas — safe to run concurrently:

1. **C (changelog writer)** — one root-cause hunt, `agent_go` plan-mutation path.
2. **D's real item + G** — `agent_go`, eval report serialization + learnings metadata, unrelated files to C.
3. **F triage + H + I** — no code changes for H; F needs per-item classification first; I is `pulse_finding_lifecycle`-adjacent.
4. **E + J** — scheduler-adjacent, likely touches `scheduler.go` again — hold until the PLAT-065/066/067/070/071 fixes are confirmed live, since another concurrent change to that file risks a conflict with restart-pending work already there.
