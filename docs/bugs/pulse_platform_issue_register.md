# Pulse Platform-Issue Register

## Status

Triage snapshot captured 2026-08-03 from the workflow-local Pulse databases.
This is the canonical cross-workflow register for defects that a workflow-level
Pulse Fixer cannot repair. It is not evidence that every historical finding is
still reproducible on the latest binary: items marked **reverify** have an
implementation change but no post-change producing run yet.

The 2026-08-04 RTS Latency run supplied the first broad post-change runtime
reverification. It proved several fixes, but also reopened PLAT-009, showed that
PLAT-010 is incomplete at the projected lifecycle boundary, and exposed two new
platform issues in finalization authority and Pulse-agent cost accounting.
Follow-up UI/runtime inspection added PLAT-020 (scheduled-session continuation)
and PLAT-021 (proposal/decision projection). See the dated sections below.
The subsequent tool-error review added PLAT-022 through PLAT-026 and assigned
that independent batch to Claude Code.
The later Social Media runtime investigation expanded PLAT-003 to uniform
workflow-step DB read/write and added PLAT-027 (live child visibility) and
PLAT-028 (CDP tab argument normalization).
Electron runtime verification then added and closed PLAT-034: completed Raw
tmux panes were replacing their recorded stream with a final-screen capture,
which destroyed scrollback at the live-to-settled boundary.

This document records platform ownership and deduplication. The authoritative
per-workflow lifecycle remains `db/db.sqlite`; detailed single-defect incident
documents remain authoritative where linked.

## Why this register exists

Pulse currently stores a platform defect inside the workflow that observed it.
The same harness root cause can therefore appear as several workflow findings,
consume reviewer slots, recur forever, and invite a Fixer that has no authority
to change the platform. A cross-workflow register is needed until the lifecycle
has a first-class platform issue and linkage table.

A finding belongs here only when the failed boundary is owned by the workflow
runtime, scheduler, bridge, tool contract, shared persistence, or shared UI—not
by the workflow plan or its data.

## Two-agent repair board

This file is the read-mostly platform index. Each issue has a canonical ticket
fragment under `docs/bugs/pulse_platform/` so **Codex** and **Claude Code** can
repair different boundaries concurrently without editing one large document.

Rules:

1. Every implementation ticket has one platform key, one narrow acceptance
   boundary, and exactly one assigned agent: `Codex`, `Claude Code`, or
   `Unassigned`.
2. An agent changes the assigned agent and state in the ticket fragment to
   `in_progress` before editing code and lists the main files it expects to
   touch. The other agent may review that work but does not implement the same
   ticket unless ownership is explicitly handed off. The shared index does not
   need editing at claim time.
3. A broad platform issue may have multiple suffix tickets (`PLAT-003-A`,
   `PLAT-003-B`) when its API, runtime, UI, or migration boundaries can be fixed
   independently. Do not combine unrelated PLAT keys into one coding task.
4. `implemented` means focused tests pass. `runtime_reverify` means the code is
   complete but a real producing workflow/Pulse run is still required. `done`
   requires both where runtime evidence is part of acceptance.
5. Each fragment records its evidence, acceptance boundary, and test command.
   The index is synchronized once at handoff, review, or completion—not on every
   working update. Create another incident document only when large raw
   evidence would make the ticket fragment unreadable.

### Active ownership lanes

| Ticket | Boundary | Assigned agent | State | Primary files |
|---|---|---|---|---|
| [PLAT-001-A](pulse_platform/plat-001.md) | Reverify keyed human-input propagation | Claude Code | `runtime_reverify` | workflow orchestration/handoff |
| [PLAT-002-A](pulse_platform/plat-002.md) | Reverify canonical nested tool-failure status | Claude Code | `runtime_reverify` | tool bridge and terminal status |
| [PLAT-003-A](pulse_platform/plat-003.md) | Reverify uniform workflow-step DB read/write | Codex | `runtime_reverify` | DB capability materialization |
| [PLAT-005-A](pulse_platform/plat-005.md) | Reverify multi-name API-spec lookup | Claude Code | `runtime_reverify` | mcpagent API bridge |
| [PLAT-006-A](pulse_platform/plat-006.md) | Reverify workflow-step shell cwd | Claude Code | `runtime_reverify` | step session/bridge cwd |
| [PLAT-007-A](pulse_platform/plat-007.md) | Exercise workflow image verification E2E | Claude Code | `runtime_e2e_reverify` | media tool path/model routing |
| [PLAT-009-A](pulse_platform/plat-009.md) | Merge grouped cost shards for an iteration-only query | Codex | `runtime_reverify` | `token_usage_store.go` |
| [PLAT-010-A](pulse_platform/plat-010.md) | Finish partially migrated finding-event identities | Codex | `runtime_reverify` | `pulse_finding_lifecycle.go` |
| [PLAT-011-A](pulse_platform/plat-011.md) | Complete non-tier LLM-role UI acceptance | Claude Code | `ui_acceptance_pending` | LLM configuration API/UI |
| [PLAT-012-A](pulse_platform/plat-012.md) | Reverify dependent-artifact changelog coverage | Claude Code | `runtime_reverify` | managed mutation changelog |
| [PLAT-014-A](pulse_platform/plat-014.md) | Reverify reviewer skill delivery on Tectonicus | Claude Code | `partial_runtime_reverify` | guidance and skill delivery |
| [PLAT-015-A](pulse_platform/plat-015.md) | Persist an explicit skipped-evaluation sentinel | Codex | `runtime_reverify` | `evaluation_execution.go`, `evaluation_types.go` |
| [PLAT-016-A](pulse_platform/plat-016.md) | Preserve legitimate numeric zero evaluation scores | Codex | `runtime_reverify` | `evaluation_types.go` |
| [PLAT-017-A](pulse_platform/plat-017.md) | Reproduce scheduler-success versus run-metadata mismatch | Unassigned | `blocked_on_reproduction` | scheduler and run-metadata finalization |
| [PLAT-018-A](pulse_platform/plat-018.md) | Use validated dashboard artifact as final-command proof | Codex | `runtime_reverify` | `pulse_final_commands.go` |
| [PLAT-019-A](pulse_platform/plat-019.md) | Price only unpriced Pulse-agent usage | Codex | `runtime_reverify` | `pulse_agent_metrics.go`, `costledger/ledger.go` |
| [PLAT-020-A](pulse_platform/plat-020.md) | Keep converted scheduled chat on the same session/tmux | Codex | `runtime_reverify` | `WorkflowChatTabs.tsx`, retained-input routing |
| [PLAT-021-A](pulse_platform/plat-021.md) | Separate proposals from answerable user decisions | Codex | `ui_reverify` | `pulseFindingPresentation.ts`, `PulseWorkspace.tsx` |
| [PLAT-022-A](pulse_platform/plat-022.md) | Restore `get_api_spec` registration in the affected message-sequence session | Claude Code | `assigned` | registration path / workflow-phase tool setup |
| [PLAT-023-A](pulse_platform/plat-023.md) | Make large-file diff context failure recoverable without unsafe fuzzy apply | Claude Code + Codex follow-up | `implemented` | `workspace/handlers/diff_patch.go` |
| [PLAT-026-A](pulse_platform/plat-026.md) | Keep selected running workflow visible exactly once | Claude Code | `implemented` | `GlobalActivityMonitor.tsx` |
| [PLAT-024-A](pulse_platform/plat-024.md) | Populate missing tool names in canonical error markers | Claude Code + Codex follow-up | `implemented` | mcpagent/provider logging |
| [PLAT-025-A](pulse_platform/plat-025.md) | Bound workspace-shell stdout memory without corrupting scripted JSON | Claude Code | `queued` | `workspace/handlers/shell.go` |
| [PLAT-027-A](pulse_platform/plat-027.md) | Keep a live asynchronous child visible after parent completion | Codex | `implemented` | terminal execution-tree projection |
| [PLAT-028-A](pulse_platform/plat-028.md) | Remove a recovered CDP tab from final page-action arguments | Codex | `implemented` | browser executor argument normalization |
| [PLAT-029-A](pulse_platform/plat-029.md) | Close stale live metadata before attaching to a missing tmux | Codex | `implemented` | terminal live-attach lifecycle |
| [PLAT-031-A](pulse_platform/plat-031.md) | Persist one immutable execution identity across cost-ledger date shards | Claude Code | `implemented` | execution-cost persistence and attribution |
| [PLAT-032-A](pulse_platform/plat-032.md) | Include child-agent calls in parent step telemetry | Claude Code | `implemented` | child dispatch telemetry and usage aggregation |
| [PLAT-033-A](pulse_platform/plat-033.md) | Replace placeholder changelog refs with truthful artifact evidence | Claude Code | `implemented` | managed mutation/changelog writer |
| [PLAT-034-A](pulse_platform/plat-034.md) | Retain Raw tmux scrollback after process completion | Codex | `done` | terminal live-attach and chat-history persistence |

Assignment reserves the lane; it does not claim that work has started. An agent
sets its fragment to `in_progress` when it actually begins. PLAT-004, PLAT-008,
and PLAT-013 remain unassigned because they are already runtime verified.
PLAT-017 remains unassigned because it needs a fresh reproduction before a
correct implementation boundary can be chosen.

For new work, create the smallest independent fragment and add one link here.
Claude Code should claim its currently active tickets in their fragment rather
than relying on chat-only coordination; that makes ownership survive restarts
and handoffs without creating an index-edit conflict with Codex.

## Evidence collection

The snapshot scanned all 11 databases under:

```text
workspace-docs/Workflow/*/db/db.sqlite
```

The structured set is the join of:

```sql
SELECT c.status, c.seen_count, d.finding_id, d.issue_kind,
       d.detail_json, c.step_id, c.text
FROM run_concerns c
JOIN pulse_finding_details d USING (fingerprint)
WHERE c.status NOT IN ('resolved', 'rejected')
  AND d.issue_kind = 'harness_issue';
```

That returned 12 active or acknowledged records across Build-in-public,
LinkedIn, RTS Latency, and Upwork. A second compatibility scan included older
records whose text describes a scheduler, bridge, Folder Guard, working
directory, tool-registration, or media-tool failure but predates
`issue_kind=harness_issue`.

## Priority summary

| Platform key | Priority | Current workflow evidence | State |
|---|---:|---|---|
| PLAT-001 Human-input propagation | P0 | Upwork | **implementation fixed; runtime reverify** |
| PLAT-002 Tool-failure status precedence | P0 | Upwork, Build-in-public, Social Media | **canonical CLI/runtime fix implemented; runtime reverify** |
| PLAT-003 Workflow DB tool exposure | P0 | Build-in-public, Instagram, RTS Latency, Social Media | **expanded to uniform workflow-step read/write; runtime reverify** |
| PLAT-004 Scheduler completion detection | P0 | RTS Latency | **runtime verified 2026-08-04** |
| PLAT-005 `get_api_spec` multi-name contract | P1 | RTS Latency | **fixed in mcpagent; runtime reverify** |
| PLAT-006 Workflow-step shell cwd contract | P1 | RTS Latency | **reverify** implemented fix |
| PLAT-007 Workflow image verification | P1 | Instagram | implemented; runtime/E2E reverify |
| PLAT-008 Phase cost pricing | P1 | Build-in-public, RTS Latency | **core pricing runtime verified; see PLAT-019** |
| PLAT-009 `get_cost_summary` run resolution | P1 | Build-in-public, Social Media, RTS Latency | **implementation repaired; runtime reverify** |
| PLAT-010 Finding identity split | P1 | RTS Latency | **implementation completed; runtime reverify** |
| PLAT-011 LLM role visibility | P2 | Build-in-public, RTS Latency | **runtime evidence positive; full UI acceptance pending** |
| PLAT-012 Changelog mutation coverage | P2 | LinkedIn, RTS Latency | **plan mutation verified; learning-tree boundary still reverify** |
| PLAT-013 Legacy regular-step editing | P1 | RTS Latency | **runtime verified 2026-08-04** |
| PLAT-014 Reviewer reference loading | P1 | RTS Latency, Tectonicus | **RTS runtime verified; Tectonicus reverify remains** |
| PLAT-015 Evaluation skipped-sentinel handling | P1 | Social Media | **implementation fixed; runtime reverify** |
| PLAT-016 Evaluation report drops real zero scores | P1 | Social Media | **implementation fixed; runtime reverify** |
| PLAT-017 Scheduler success leaves workflow metadata running | P1 | Social Media | **open; distinct from PLAT-004** |
| PLAT-018 Pulse finalizer cannot record dashboard completion | P1 | RTS Latency | **implementation fixed; runtime reverify** |
| PLAT-019 Pulse agent metrics remain unpriced | P1 | RTS Latency | **implementation fixed; runtime reverify** |
| PLAT-020 Converted scheduled chat must retain its session/tmux | P0 | RTS Latency | **implementation corrected; UI/runtime reverify** |
| PLAT-021 Proposals masquerade as pending user decisions | P1 | RTS Latency | **implementation fixed; UI reverify** |
| PLAT-022 `get_api_spec` absent from one workflow-phase session | P1 | Job Search | **assigned to Claude Code** |
| PLAT-023 Diff patch context recovery on large files | P1 | Tool-error logs | **implemented 2026-08-04 (large-file fixtures + corrected-retry round trip); nearest-candidate counting is accurate and tied near-matches refuse an arbitrary hint; runtime reverify remains** |
| PLAT-024 Tool-error marker omits tool name | P2 | Cross-workflow logs | **implemented 2026-08-04 (mcpagent d1eca1f + Codex follow-up); identity is recovered before per-tool failure classification, with a narrow envelope fallback and explicit "unknown"; runtime reverify remains** |
| PLAT-025 Workspace shell stdout buffer is unbounded | P1 | Platform availability | **queued for Claude Code** |
| PLAT-026 Selected running workflow hidden from global activity | P1 | RTS Latency | **implemented 2026-08-04 (a first pass missed the same-workflow-sibling case per Codex review; corrected — see ticket); runtime reverify remains** |
| PLAT-027 Async todo-task turn falsely completes its parent and hides the live child | P0 | Social Media | **completion gate runtime verified; placeholder 404 fixed and tested 2026-08-04; rebuilt UI reverify remains** |
| PLAT-028 Recovered CDP tab forwarded as action argument | P1 | Social Media | **implemented and executor-tested 2026-08-04; runtime reverify remains** |
| PLAT-029 Missing tmux remains live and reconnects forever | P0 | Social Media | **implemented and regression-tested 2026-08-04; rebuilt runtime reverify remains** |
| PLAT-031 Cost ledger loses run identity across UTC midnight | P1 | RTS Latency | **writer-side fix implemented 2026-08-05 (`mcp-agent-builder-go` 1bfa745d5): sticky-first-write ExecutionID survives a UTC date-shard rotation, numeric schedule-message indices no longer bucket under a step-shaped phase; full per-execution aggregate separation and the query layer remain PLAT-009's; runtime reverify remains** |
| PLAT-032 Child-agent calls omitted from parent telemetry | P1 | Social Media | **root cause fixed 2026-08-05 (`mcp-agent-builder-go` cdc3d1a76): async sub-agent dispatch now propagates the parent step's timing-capture ID to the child context; parent/child/total breakdown and failed-child/E2E tests not built; the separate failed-child status claim remains unreproduced; runtime reverify remains** |
| PLAT-033 Managed changelog contains placeholder refs | P1 | Social Media | **implemented 2026-08-05 (`mcp-agent-builder-go` cdc3d1a76) for the two reproduced offenders (update_step_config, write_workflow_manifest); shared mechanism now prefers real before/after snapshots over sha256("[]"); fail-closed on unsupported fields and per-caller audit of the other ~13 changelog call sites not done; runtime reverify remains** |
| PLAT-034 Completed Raw tmux terminal loses scrollback | P1 | Social Media / Electron | **fixed and runtime verified 2026-08-05 (`mcp-agent-builder-go` b984e6c5c); retained stream survives completion and remains scrollable** |

### Social Media classification correction — 2026-08-05

The three cards shown in the Platform queue do not represent three proven
platform repairs:

1. `HARNESS-SUBAGENT-DISPATCH-STATUS` contains one proven platform defect and
   one historical claim. The proven telemetry undercount is now PLAT-032. Its
   claim that a failed child is reported as completed was not reproduced in the
   cited run and remains evidence to reproduce, not code to change.
2. `HARNESS-CHANGELOG-REF-PLACEHOLDER` is a current, independently reproduced
   platform defect and is now PLAT-033.
3. `PUL-93AE14C6` is **not presently a platform ticket**. It reports that reuse
   of `runs/iteration-0/default` erased the prior occupant even though the
   workflow declares `always_use_same_run=false`. The Fixer marked it
   `blocked` only because it did not reach the item in that pass, while its own
   note says the finding remains actionable next pass. That disposition routed
   it to Platform incorrectly. Keep the finding open and Pulse-owned until a
   repair attempt diagnoses a platform boundary or fixes the workflow/runtime
   configuration.

PLAT-031 is already implemented on its documented writer boundary and is not
one of these new Social Media repairs.

The two tool-error findings below are one family, not two independent repair
projects. The database-tool symptoms across three workflows are also one shared
capability defect until evidence proves otherwise.

### Why fixed platform findings appeared again in Social Media

Social Media run `schedule-cron--4128e261_1785724255412842000` began at
2026-08-03T02:30:55Z and its Pulse finalizer backed up at
2026-08-03T07:13:51Z (12:43 IST). The platform-reliability bundle
`46bf02dff` and canonical mcpagent tool-error change `7820b74` landed at
18:56 IST. Therefore the Social Media observations for nested tool-error status
and cost/run-folder attribution occurred before those implementations existed;
they are verification evidence for PLAT-002 and PLAT-009, not proof that the
new fixes regressed. This remains true even if the server restarted before the
Social Media run.

The scheduler evidence is different. PLAT-004 fixed premature success while
required child work was still in flight. Social Media's discovery children did
finish, but scheduler success and durable `run_metadata.status=running`
disagreed afterward. That is a terminal metadata/provenance reconciliation
defect and is tracked separately as PLAT-017 rather than being treated as a
PLAT-004 regression.

### RTS Latency runtime reverification — 2026-08-04

Run identity:

```text
workflow run: 33798848-0ed7-4132-821c-e05592e3ec4e
pulse_run_id: schedule-cron--42eca39a_1785810615371091000
workflow:     08:00:15–09:37:00 IST (96.75 minutes)
reviewers:    09:40:06–10:14:05 IST
fixer:        10:14:05–10:48:22 IST
```

**The new reviewer/Fixer topology worked.** Gate started exactly two independent
reviewers with concurrency enabled: `workflow_review` and `strategy_auditor`.
Their executions overlapped. `workflow_review` used the new multi-turn sequence
(7 LLM calls); Strategy Auditor remained an independent lens. Goal Advisor was
skipped on its own recorded boundary. The reviewer barrier waited for both, then
one consolidated Fixer ran. `pulse_agent_metrics.role` correctly distinguishes
`reviewer` from `fixer`.

**Verification-before-discovery worked.** The workflow reviewer processed 14
allowlisted attempt/finding tuples across 12 awaiting-verification findings:
13 passed, 1 failed, and 0 were inconclusive. The failed finding was reopened
and the Fixer repaired the plan-level source it named. Runtime evidence closed
the prior DB-persistence, DB-probe, variable-read, KB reachability, language/
`stt_routing`, digest-freshness, and scheduler-completion findings. In
particular, both collectors persisted 2026-08-04 rows through the sanctioned
`$MCP_CUSTOM` DB route, and the full scheduled workflow reached every required
step before success was recorded.

**The Fixer made real, audited changes.** It corrected the `audio_status`
contract conflict, added the mandatory `source=voice` gate, repaired workflow
root derivation, made declared production log-group configuration observable,
and changed the report's DB instructions from direct `sqlite3` access to the
sanctioned route. Ten managed plan mutations appear in
`planning/changelog/changelog-2026-08-04-04-18-12.json`. It also recorded three
impact interventions and ten goal observations. Five behavioral changes remain
correctly `awaiting_verification`; only the next producing workflow run can
settle them.

**PLAT-009 failed its runtime acceptance and is reopened.** At 09:51:07 IST,
`get_cost_summary(workspace_path="Workflow/rtslatency")` first tried the retired
`runs/iteration-0/token_usage.json`, then enumerated only
`costs/execution/__ungrouped__`. It did not select the authoritative
`costs/execution/dev/2026-08-04.json` shard. The reviewer consequently reported
that all nine scheduled step rows and `claude-sonnet-5` were absent, while the
authoritative dev ledger contains them. This is post-change current-binary
evidence, not a stale pre-fix observation.

**PLAT-010 is only partially repaired.** Migration has produced one canonical
`pulse_finding_details` row and one `run_concerns` row for
`HARNESS-REFDOC-REVIEW-ARTIFACT-DRIFT`, but `pulse_finding_events` still carries
ordinary `external_action_required` events under both the old and canonical
fingerprints. The projected backlog shown to the reviewer therefore still
presents two lifecycle items, and the Fixer explicitly reconciled both halves.
Preserving history is correct; preserving the old events as live lifecycle
events rather than explicit identity-merge history is not.

**PLAT-008's core arithmetic worked, but a separate accounting boundary did
not.** The phase and execution ledgers now stamp `pricing_model_id`,
`pricing_version`, fresh/cache token components, and non-zero totals for both
`claude-opus-5` and `claude-sonnet-5`. However, the three corresponding
`pulse_agent_metrics` rows have `usage_status=captured`, model usage populated,
and `total_cost_usd=0`; their embedded model objects report
`unpriced_call_count` for every call. That inconsistency is PLAT-019 rather than
a reopening of the phase-ledger arithmetic in PLAT-008.

**PLAT-012 is only partly exercised.** Every managed plan edit emitted a typed
changelog entry with target, actor, dependency class, and before/after refs.
The run also changed `learnings/_global/references/*.md`, but this pass does not
prove the runtime-learning-turn tree-hash path because those edits were made by
the Fixer rather than the dedicated learning turn. Keep that half in reverify.

**Run-quality note, not yet separate platform keys.** The Fixer recovered from
several avoidable tool failures: one absolute-path Folder Guard denial, one
failed assertion while rewriting, one protected evaluation-plan diff attempt,
and several schema-probing failures against `record_pulse_result` /
`record_pulse_impact`. These did not invalidate its terminal results, but they
are evidence that the prompt/schema ergonomics still waste calls. Do not count
them as workflow defects without first proving a shared contract problem.

### Platform repair pass — 2026-08-04

This pass repaired the current-binary defects above without restarting the
backend. Verification is focused/unit-level until the next real Pulse or
schedule-to-chat producer exercises each boundary:

- PLAT-009 now treats an iteration-only run such as `iteration-0` as the parent
  of its grouped ledger shards, enumerates every group/date shard, and merges
  only matching child run keys. Explicit `iteration-0/dev` queries remain
  exact. Tests prove dev+production merge and reject another iteration.
- PLAT-010 now finishes interrupted historical migrations: events left under
  an old fingerprint are moved to the canonical finding ID/fingerprint, with
  colliding tuples retained as explicit `identity_merge` history rather than a
  second live case.
- PLAT-015 persists a skipped step as `skipped: true` with explicit
  reason/evidence through JSON and SQLite. The PLAT-016 audit found the earlier
  repair incomplete beyond the workflow serializer: server projection could
  still omit zero, fractional values were truncated, and missing scores lacked
  an explicit presence bit. Workflow, SQLite, API, and UI now use
  `score_captured`; preserve fractional values; and display genuine zero.
- PLAT-018 now uses the scheduler's successful dashboard artifact validation
  as the command's proof and marks only `dashboard=done`. It does not grant the
  agent general DB write authority and does not infer success for backup,
  publish, or notify.
- PLAT-019 reprices only the unpriced slice of each captured model aggregate
  with the same immutable model rate card used by phase/execution ledgers.
  Mixed priced/unpriced aggregates cannot double-charge; genuinely unknown
  models remain explicit as `captured_unpriced` with a reason.
- PLAT-020 now retains the scheduled conversation's session ID when it becomes
  interactive. A live retained tmux receives the message directly; if the pane
  is gone, the backend resumes the same native coding-CLI conversation. It does
  not fork the chat or discard its context.
- PLAT-021 separates informational proposals from answer-blocked decisions.
  A finding appears under **Your decisions** only when it carries a linked
  `human_input_id`; a missing link is Pulse-owned repair work, and a
  `proposal_recorded` item appears under **Proposed improvements** instead.
- PLAT-017 was deliberately not changed. The retained Social Media
  `run_metadata.json` now has a terminal `failed` state, so the historical
  scheduler-success/metadata-running contradiction cannot be reproduced from
  the current artifact. Capture a fresh current-binary mismatch before
  changing shared scheduler terminal-state ownership.

Focused verification passed:

```text
go test ./agent_go/pkg/costledger ./agent_go/pkg/orchestrator ./agent_go/pkg/orchestrator/agents/workflow/step_based_workflow ./agent_go/cmd/server \
  -run 'TestReadRunAcrossDates|TestFindingIdentityMigration|TestReconcilePulseDashboard|TestPulseAgentMetrics|TestPhasePricingUses' -count=1
npm test -- --run src/components/workflow/pulseFindingPresentation.test.ts
npx tsc -b --pretty false
```

### Additional workflow-runtime repairs — 2026-08-04

Three related changes were implemented after the repair pass above. None
required a backend restart during development:

1. **PLAT-003 — uniform workflow DB access.** Every workflow execution step,
   including evaluation and message-sequence children, now gets the same
   managed read-write capability. Legacy `db_access=read` and item-level DB
   flags no longer create parent/child capability drift. Agentic access remains
   mediated by `query_workflow_db` and `mutate_workflow_db`; this does not
   reopen raw SQLite/WAL/SHM access.
2. **PLAT-027 — asynchronous-child visibility.** The terminal rail now projects
   live children directly from the session execution tree while their detailed
   terminal snapshot is pending. The placeholder reconciles with the real
   terminal by execution identity, so the child remains visible without a
   duplicate after its parent turn completes.
3. **PLAT-028 — CDP tab normalization.** A bare, unambiguous tab ID such as
   `t2` is now recovered before command planning and subprocess construction.
   `click t2 e64` therefore routes to tab `t2` and invokes `click e64` instead
   of treating the tab as an element selector.
4. **PLAT-003 — DB query argument compatibility.** The canonical read argument
   remains `sql`, but the natural `query` spelling is now a documented alias
   and reaches the same query-only backend. Conflicting aliases fail before a
   database request.
5. **PLAT-034 — Raw terminal retention.** The live terminal byte stream now has
   a distinct `tmux_stream` source. Settled detail and chat-history persistence
   preserve it instead of overwriting it with tmux's final visible screen. Raw
   remains the default; the Electron runtime recheck confirmed scrolling after
   completion.

Focused verification covered workflow-step capability/folder-guard routes,
terminal execution-tree projection, and the real browser executor boundary.
Each fragment records its exact acceptance and remaining runtime recheck.

## Ticket files

The fragment is canonical for current ownership, implementation notes,
verification evidence, and acceptance. This index supplies cross-ticket
priority and historical run context.

| Ticket | Ticket | Ticket | Ticket |
|---|---|---|---|
| [PLAT-001](pulse_platform/plat-001.md) | [PLAT-002](pulse_platform/plat-002.md) | [PLAT-003](pulse_platform/plat-003.md) | [PLAT-004](pulse_platform/plat-004.md) |
| [PLAT-005](pulse_platform/plat-005.md) | [PLAT-006](pulse_platform/plat-006.md) | [PLAT-007](pulse_platform/plat-007.md) | [PLAT-008](pulse_platform/plat-008.md) |
| [PLAT-009](pulse_platform/plat-009.md) | [PLAT-010](pulse_platform/plat-010.md) | [PLAT-011](pulse_platform/plat-011.md) | [PLAT-012](pulse_platform/plat-012.md) |
| [PLAT-013](pulse_platform/plat-013.md) | [PLAT-014](pulse_platform/plat-014.md) | [PLAT-015](pulse_platform/plat-015.md) | [PLAT-016](pulse_platform/plat-016.md) |
| [PLAT-017](pulse_platform/plat-017.md) | [PLAT-018](pulse_platform/plat-018.md) | [PLAT-019](pulse_platform/plat-019.md) | [PLAT-020](pulse_platform/plat-020.md) |
| [PLAT-021](pulse_platform/plat-021.md) |  |  |  |
| [PLAT-022](pulse_platform/plat-022.md) | [PLAT-023](pulse_platform/plat-023.md) | [PLAT-024](pulse_platform/plat-024.md) | [PLAT-025](pulse_platform/plat-025.md) |
| [PLAT-026](pulse_platform/plat-026.md) | [PLAT-027](pulse_platform/plat-027.md) | [PLAT-028](pulse_platform/plat-028.md) | [PLAT-029](pulse_platform/plat-029.md) |
| [PLAT-030](pulse_platform/plat-030.md) | [PLAT-031](pulse_platform/plat-031.md) | [PLAT-032](pulse_platform/plat-032.md) | [PLAT-033](pulse_platform/plat-033.md) |
| [PLAT-034](pulse_platform/plat-034.md) |  |  |  |
## Explicitly not platform issues

The following remain workflow-owned or evidence-state items even when they are
currently blocked:

- a collector catches an exception and ignores the loaded variables;
- a step hardcodes an incorrect log group or database probe result;
- a plan's editable Folder Guard configuration omits a required folder;
- a fix is waiting for the next producing run;
- an upstream service has not produced data;
- a strategy requires a user decision or deployment approval;
- a workflow writes through direct `sqlite3` despite having a documented,
  reachable sanctioned mutation tool.

If the configuration is not editable because the platform exposes no sanctioned
edit path, that missing edit path is the platform defect—not the workflow's
desired configuration.

## Lifecycle behavior required

Until the platform lifecycle is implemented, these records should be treated as
one of two states:

- **platform_open:** confirmed on the current binary and owned outside the
  workflow;
- **platform_reverify:** a platform change exists, and the next applicable real
  run must prove the observation is gone.

They must remain visible to the operator but must not consume normal workflow
Fixer capacity, count as recurring workflow-strategy evidence, or occupy the
reviewer's new-finding cap. A new observation should link to the platform key
above rather than filing another independent platform issue.

After a platform fix ships:

1. move every linked workflow finding to awaiting verification;
2. run the smallest real or E2E producer that exercises the boundary;
3. close all linked findings together when it passes;
4. reopen the single platform issue with the new evidence when it fails.

## Recommended implementation order

1. PLAT-001 human-input propagation — implementation complete; real full-run reverify pending.
2. PLAT-002 canonical tool-error precedence — implementation and focused tests complete; persisted runtime evidence pending.
3. PLAT-003 DB capability exposure — uniform workflow-step read/write is
   implemented and focused capability tests pass; source workflows need
   post-build reverify.
4. PLAT-004 scheduler completion barrier — runtime verified by the full
   2026-08-04 RTS scheduled run; retain regression coverage.
5. Reverify the corrected PLAT-020 same-session schedule-to-chat handoff first:
   the session ID must remain stable and the message must reach the retained or
   resumed tmux exactly once.
6. Reverify completed PLAT-009, PLAT-010, PLAT-018, PLAT-019, and PLAT-021 on
   the next applicable RTS Pulse pass/UI open.
7. Add the platform issue/link lifecycle so the backlog stops duplicating them.
8. Reverify PLAT-006 and the remaining Tectonicus half of PLAT-014; PLAT-013 is
   runtime verified on RTS Latency.
9. Reverify PLAT-015 and PLAT-016 with a producing Social Media evaluation;
   reproduce PLAT-017 on the current binary before changing terminal-state
   ownership.
10. Repair/query-test the remaining P1 observability and media defects.
11. Address the P2 configuration and changelog completeness gaps.
12. Reverify PLAT-027 and PLAT-028 together on the next Social Media CDP run:
    the asynchronous child must stay visible and its browser action must not
    forward the tab ID as an element selector.

The first four are ordered ahead of cost and UI completeness because they can
change what a workflow does while still allowing it to report success.
