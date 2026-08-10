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

## B. Cost/context telemetry (6) — 3 closed, 3 remain

Closed by removal (`agent_go` `e0e0494bf`): `context_usage_percent` findings —
the field drove no behavior, was actively misleading, removed rather than
fixed. Remaining, genuine pricing/attribution bugs, not yet investigated:

- `e6be98dfd6f4d639` (build-in-public) — one daily cost file priced
  claude-opus-5 at sonnet-5 rates; every phase ledger zeroes/omits
  `input_cost_usd` (charging $0.000003 for 2.8M tokens).
- `43b988fe2ef952f3` (tectonicusadaytrading) — `costs/phase/daily` reports
  $7.08/6 calls for a date where `costs/execution` records $21.98/22 calls, a
  3.1x understatement, with no file declaring whether the two ledgers overlap.
- `a8ab091308579946`, `e717a5e1a962a81f` (upwork) — date-wide overhead ledgers
  can't isolate current orchestrator/builder/Pulse cost or diagnose the
  cached-input workload item-by-item. Worth re-checking after `af0345a0` closes
  whether these were partly downstream of the same removed field.

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

## E. Schedules skipped / mis-reported (3) — not started

`7c4ac152` (build-in-public) — *"the host demonstrably ran two schedules"* on
one date. `3565d07c`, `3e42ae71` (linkedin) — `list_schedules` reports a
completed run as still running with `next_run` in the past, and a weekly
schedule skipped a fire with no record of why.

## F. Tools unavailable / limits (8) — triaged, closer to 1 real open bug

- `ad5c92dd` (rtslatency, `get_api_spec` array-of-strings `tool_name`) —
  **already fixed** (`mcpagent` commit `ea60eb2`, predates the finding).
  `handleGetAPISpec` (`mcpagent/agent/code_execution_tools.go:24-55`) already
  handles both a JSON-array string and a real `[]interface{}`. Reverify live,
  then close.
- `dd9ede3c` (upwork, `agent_browser` snapshot overflow) — **real bug, not
  PLAT-062-adjacent** (that ticket is an unrelated prompt-text defect). Root
  cause: `agent_browser`'s `snapshot` command has no output-size cap (unlike
  `read_skill`'s `maxReadSkillBatchSize=1`, added for the identical failure
  shape per `mcpagent/agent/skill.go:21-33`), so a large snapshot exceeds
  Claude Code's own MCP result cap and gets spilled to
  `MCP_TOOL_OUTPUT_DIR`. That dir resolves to `<workflow-root>/tool_output_folder`
  (`pkg/orchestrator/base_orchestrator_agent_factory.go:142`,
  `mcpagent/agent/coding_agents_bridge.go:198-200`) — a sibling of `runs/`
  that `setupExecutionFolderGuard` (`controller_agent_factory.go:427`) never
  grants read access to, so the spilled copy is structurally unreadable.
  Confirmed against the live filesystem: `workspace-docs/Workflow/upwork/tool_output_folder/`
  exists with spilled files up to 16 MB. Two independent fix points, neither
  implemented yet: (1) cap/paginate `agent_browser` snapshot output
  (`pkg/browser/executor.go`/`tools.go`), (2) resolve
  `MCP_TOOL_OUTPUT_DIR` inside the guard's granted read tree, or add
  `tool_output_folder` to `setupExecutionFolderGuard`'s read paths.
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

## J. Routing / run identity (3) — not started

`05a81ca02...` (hetznerssh) — the live PLAT-066 recurrence found earlier
today; still open, root cause not yet isolated (see PLAT-066). `b3ac1ae3`
(build-in-public) — `CLAUDE.md`, tracked at HEAD, is missing from the working
tree. `5bf5513f` (rtslatency) — `latency_baselines` uses `PRIMARY KEY (env)`
with no day/version dimension, so each collector run irreversibly destroys the
previous day's baseline.

## K. Unclustered (3)

`ff9832cc` (build-in-public) — `execution_low` resolves to a higher-reasoning
model than the tier name implies. `0b7d9d4a` (social-media) — sub-agent
dispatch envelopes report `status: "completed"` for workers that returned
`success:false`, likely the same shape as A, worth checking against the
`HandleCustomExecute` fix once it's live. `5e248d9e` (social-media) — the
PLAT-072 artifact-deletion claim left open pending verification that rotation
archives workflow artifacts, not just cost/eval paths.

## Suggested split for parallel work

Independent, non-overlapping code areas — safe to run concurrently:

1. **C (changelog writer)** — one root-cause hunt, `agent_go` plan-mutation path.
2. **D's real item + G** — `agent_go`, eval report serialization + learnings metadata, unrelated files to C.
3. **F triage + H + I** — no code changes for H; F needs per-item classification first; I is `pulse_finding_lifecycle`-adjacent.
4. **E + J** — scheduler-adjacent, likely touches `scheduler.go` again — hold until the PLAT-065/066/067/070/071 fixes are confirmed live, since another concurrent change to that file risks a conflict with restart-pending work already there.
