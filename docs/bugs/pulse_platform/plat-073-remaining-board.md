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

## D. Eval scoring ambiguity (5) — one likely one-line bug, rest are workflow-level

- `28e793fd852b3b43` (social-media) — `evaluation_report.json` omits the
  `score` key entirely when a step scores 0. Check for a Go struct field with
  `omitempty` on a numeric score — that silently drops legitimate zeros. High
  confidence, cheap to verify.
- `8e266151da4a0e25` (social-media) — route-gated skipped steps stored as
  `score=0/max_score=0`, indistinguishable from a genuine zero without
  filtering by a `skipped` column. Schema/design issue, needs a decision on
  NULL vs. a status flag, not just a patch.
- `c85dfe0208e94aef`, `53316f02fa209f61` (build-in-public) — these are
  `evaluation_plan.json` content issues (a weak CHECK, a stale rule
  superseded by the actual `main.py`), not platform code. Workflow-level
  fixes, arguably misfiled as `external_action_required` — flag for
  `workflow_review` rather than a code fix.
- `9495aef3dab65c42` (linkedin) — evaluation runtime starts before
  `run_metadata.json` is finalized, so evaluators bind evidence to an
  unfinished run. Real sequencing bug, not yet traced to a code location.

## E. Schedules skipped / mis-reported (3) — not started

`7c4ac152` (build-in-public) — *"the host demonstrably ran two schedules"* on
one date. `3565d07c`, `3e42ae71` (linkedin) — `list_schedules` reports a
completed run as still running with `next_run` in the past, and a weekly
schedule skipped a fire with no record of why.

## F. Tools unavailable / limits (8) — triage before fixing, not started

Real: `dd9ede3c` (upwork, `agent_browser` snapshot overflows the tool-result
size limit — recurring, `PLAT-062`-adjacent), `ad5c92dd` (rtslatency,
`get_api_spec` silently treats an array-of-strings `tool_name` as one literal
name). **Not bugs — the sandbox correctly denying access**: `90348ad2`,
`22fa5102` (tectonicusadaytrading, direct `sqlite3` blocked by the folder
guard, working as designed). Reclassify those before this cluster is "8 bugs"
— it's closer to 2–3.

## G. Learnings/KB metadata wrong (6) — not started

`03f6ed72` (build-in-public) — `script_metadata.json` attributes a scripted
fast-path run to the `agentic` bucket. `9d1c0fe4` — `.learning_metadata.json`
misreports step identity on 7 of 9 steps. `10eb995c` —
`has_new_learning` is derived from "the turn reported changed files" rather
than actual content diffing. `dd2a4804` (social-media) —
`global_skill_objective` retains dead shared-session mechanics (same shape as
the PLAT-061 field-audit findings; check whether it's already covered).

## H. External sites blocking (3) — RECLASSIFY, not a platform bug

`23cfc840`, `0d39debd`, `659fd419` (tectonicusadaytrading) — Reddit 403s,
LunarCrush failures. The outside world, correctly reported. Close as
not-a-defect rather than leaving them to age; they will never resolve as
platform work.

## I. Human-input / backlog lifecycle (5) — not started

`bed0388b` (upwork) — `complete_pulse_review` persists `status=completed`
with an empty verdict string. `cf457bdd` — answered human inputs never
transition out of `answered` even once applied/superseded. `f2cbf9a1`
(rtslatency) — one harness finding exists as two rows under the same
`finding_id` with different fingerprints, so closing one leaves the other
open. `7602e2ac` (social-media) — `loop_closure` reports `answer_not_applied`
findings that don't check out against `report_human_inputs` directly.

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
