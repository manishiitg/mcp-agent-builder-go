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
Tectonicus Pulse then supplied new shared cost-reconciliation and changelog
coverage evidence, and added PLAT-036 (context-usage saturation) and PLAT-037
(learning-freshness attribution).
Hetzner SSH then confirmed the remaining readable-diff gap in PLAT-033 and
added PLAT-038 (per-attempt pre-validation evidence retention). Its advisor
findings also exposed PLAT-039: a missing durable route made product
recommendations appear as engineering repairs.
The subsequent Hetzner manual-trigger investigation added PLAT-040: the UI
silently discarded a busy response, while manual Pulse and full schedules were
incorrectly sharing one durable workflow lease despite chat/schedule
concurrency being an explicit product contract.
The Confida QA review added three distinct shared boundaries: PLAT-041 gives
every expected cron occurrence durable identity before execution, PLAT-042
records provable human-answer attribution, and PLAT-043 permits bounded
read-only SQLite integrity PRAGMAs through the guarded database tool.
Local process inspection added PLAT-048: scheduled main agents inherited
interactive-chat tmux retention, completed-live terminals could not be stopped,
and shutdown cleanup missed the real Claude session-name prefix.
The Upwork review added PLAT-044 through PLAT-047 and PLAT-049: false
finalizer-ownership findings, linked-decision loop closure, typed verdict
boundary hardening, the open immutable artifact-identity design, and shared
platform mechanics leaking into workflow artifacts.
The scheduler audit then added PLAT-050: Pulse review selection, fixing,
recovery, and tmux polling were duplicated in Go instead of being owned by one
continuing main-agent conversation.
The Upwork v1.0.21 migration then exposed PLAT-051: upgrade guidance named an
internal helper instead of a registered agent tool. Its scheduled follow-up
also exposed PLAT-052: known consecutive scheduler turns unnecessarily closed
and resumed the native Claude session.
The next Upwork Pulse run exposed PLAT-053: `run_in_background` gave Pulse
review/fix children a workflow-step subset rather than the full workshop tool
surface, so they could diagnose but not persist or repair findings.
A 2026-08-10 RTS Latency cron trace then added PLAT-067: after a successful
background child, the parent Claude terminal disappeared; recovery timed out,
but the scheduler still sent the next turn into that unusable transport.
A cross-workflow scheduler audit then added PLAT-054, which gates this register
itself: the idle watchdog terminates scheduled runs whose child work is
demonstrably still running, so the producing runs that ~40 `runtime_reverify`
entries are waiting on keep being destroyed before they can supply evidence.
That audit also supplied the fresh reproduction PLAT-017 has been blocked on.
A cross-workflow learnings-store audit then added PLAT-055: a step's reflection
turn can only write `learnings/`, so facts, run results and harness bug reports
that belong to the database, knowledgebase or Pulse are filed there instead —
producing 110 KB and 89 KB "skill" files whose own contents warn readers not to
trust them.
A new `scripts/pulse_health.py` sweep of every workflow's
`external_action_required` findings (nothing had done this systematically
before) found 78 candidates across all 11 Pulse-enabled workflows. Triaging
Instagram's 11 collapsed to one root cause and added PLAT-056: the
`__automatic_final_validation__` repair loop's concern recorder files a durable
finding for a prevalidation failure even when a later iteration in the same
attempt already fixed it and the attempt's terminal result passed. The
remaining candidates (build-in-public, tectonicusadaytrading, linkedin,
rtslatency, social-media) are not yet triaged. That script's coherence check
also added PLAT-057.

**The measured shape of the backlog, 2026-08-09 (baseline snapshot committed
alongside this register).** Across 8 workflows: 406 active findings, overall
closure 54.7%, and **verification debt 69.7% — 354 of 508 repair attempts could
not be proven.** That ratio is the loop: a fix reaches `awaiting_verification`,
only a producing run can settle it, the run dies (PLAT-054), and the next pass
re-examines the same finding. Of the 406 active, **115 are run-gated** (a
producing run closes them — PLAT-054's direct signal), 213 sit in the
capacity-limited engineering queue, and 78 need platform tickets and will never
close from runs. Reverification of this register's ~40 `runtime_reverify`
entries and the drainage of those 115 are the same event.

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

## Finding new candidates — `scripts/pulse_health.py`

Nothing sweeps workflow Pulse state into this register automatically — every
entry above was added by someone manually querying a workflow's `db.sqlite`
after noticing `external_action_required` findings. `scripts/pulse_health.py`
(repo root, read-only, all workflows at once) automates the discovery half and
three related diagnostics:

| Section | Answers |
|---|---|
| `--section loop` | Is Pulse converging or looping? Closure rate, **verification debt** (share of repair work that could not be proven), recurrence, and the run-gated / engineering-queue / platform split |
| `--section category` | Which classification+step clusters look like one shared root cause — this is what made **PLAT-056** (Instagram's 11 findings, one cause) obvious |
| `--section coverage` | **Whether a finding can be verified at all.** Routes nest and compose, so end-to-end paths are not enumerable (LinkedIn: 6 routers, 16 branches, 256 combinations — and its runs executed 2, 6, and 1 of 24 steps). Coverage therefore models *step execution*, not paths: a finding whose step has not run is **untested**, not failed. It also reports findings frozen by a reviewer lens disabled at the Gate, which likewise cannot close |
| `--section coherence` | Contradictory states the Go invariant cannot retroactively repair (today: `harness_issue` + `queued_for_engineering`) |
| `--section untriaged` | `external_action_required` findings citing no `PLAT-NNN` — the candidate feed for this register |

An `UNTRIAGED` row is not automatically a new ticket; it may be a fresh instance
of an existing one in different words. Read it against the tickets below before
filing. The script never files tickets — matching a finding to an existing
PLAT-NNN, or deciding it is genuinely new, stays a judgment call.

**Before/after measurement.** `--json <path>` writes a snapshot;
`--baseline <path>` diffs against one and reports how many of the *specific
findings open at baseline* actually closed. Use that rather than total counts:
disabling a reviewer lens at the Gate lowers the finding count on its own, which
is suppression, not improvement. The pre-restart baseline for the
PLAT-054/PLAT-055 verification lives at
`docs/bugs/pulse_platform/baseline-2026-08-09-pre-restart.json`.

The cohort diff separates **`stuck`** (same status, and its step did run — the
fix genuinely did not work) from **`untested`** (same status, but its step never
ran — no verdict is available). Those two look identical in a raw status count,
and conflating them is how a working fix gets blamed for a route that was never
taken. There are consequently three reasons an open finding cannot close, and
only the first is a real failure:

1. its step ran and the fix did not work — `stuck`
2. its step never ran — `untested` (see `--section coverage`)
3. its owning reviewer lens is disabled at the Gate, so nothing re-reviews it —
   **90 of 406 active findings** at the 2026-08-09 baseline

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
6. When new evidence changes a ticket's diagnosis, scope, or implementation
   choice, preserve a **Decision history** in the ticket fragment. Each entry
   records the date, the decision made, the evidence/reason, and whether it
   supersedes an earlier belief. Do not silently rewrite the old conclusion:
   future reviewers need to distinguish an abandoned approach from a regression
   of the current one. Routine code edits that do not change the reasoning do
   not need another entry.

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
| [PLAT-017-A](pulse_platform/plat-017.md) | Reconcile proved terminal completion when the final schedule-run projection fails | Unassigned | `reproduced; implementation boundary open` | scheduler and run-metadata finalization |
| [PLAT-018-A](pulse_platform/plat-018.md) | Use validated dashboard artifact as final-command proof | Codex | `runtime_reverify` | `pulse_final_commands.go` |
| [PLAT-019-A](pulse_platform/plat-019.md) | Price only unpriced Pulse-agent usage | Codex | `runtime_reverify` | `pulse_agent_metrics.go`, `costledger/ledger.go` |
| [PLAT-020-A](pulse_platform/plat-020.md) | Keep converted scheduled chat on the same session/tmux and prevent runtime reconciliation from reverting it to read-only Schedule | Codex | `implementation_complete_runtime_reverify` (2026-08-15 recurrence fixed; live reverify pending) | `WorkflowChatTabs.tsx`, `workflowRuntimeTabProjection.ts`, retained-input routing |
| [PLAT-021-A](pulse_platform/plat-021.md) | Separate proposals from answerable user decisions | Codex | `ui_reverify` | `pulseFindingPresentation.ts`, `PulseWorkspace.tsx` |
| [PLAT-022-A](pulse_platform/plat-022.md) | Restore `get_api_spec` registration in the affected message-sequence session | Claude Code | `assigned` | registration path / workflow-phase tool setup |
| [PLAT-023-A](pulse_platform/plat-023.md) | Make large-file diff context failure recoverable without unsafe fuzzy apply | Claude Code + Codex follow-up | `implemented` | `workspace/handlers/diff_patch.go` |
| [PLAT-026-A](pulse_platform/plat-026.md) | Keep selected running workflow visible exactly once | Claude Code | `implemented` | `GlobalActivityMonitor.tsx` |
| [PLAT-024-A](pulse_platform/plat-024.md) | Populate missing tool names in canonical error markers | Claude Code + Codex follow-up | `implemented` | mcpagent/provider logging |
| [PLAT-025-A](pulse_platform/plat-025.md) | Bound workspace-shell stdout memory without corrupting scripted JSON | Claude Code | `queued` | `workspace/handlers/shell.go` |
| [PLAT-027-A](pulse_platform/plat-027.md) | Keep a live asynchronous child visible after parent completion | Codex | `implemented` | terminal execution-tree projection |
| [PLAT-028-A](pulse_platform/plat-028.md) | Remove a recovered CDP tab from final page-action arguments | Codex | `implemented` | browser executor argument normalization |
| [PLAT-029-A](pulse_platform/plat-029.md) | Close stale live metadata before attaching to a missing tmux | Codex | `implemented` | terminal live-attach lifecycle |
| [PLAT-031-A](pulse_platform/plat-031.md) | Persist one immutable execution identity across cost-ledger date shards | Codex | `runtime_reverify` | execution-keyed cost/evaluation persistence and projection |
| [PLAT-032-A](pulse_platform/plat-032.md) | Include child-agent calls in parent step telemetry | Claude Code | `implemented` | child dispatch telemetry and usage aggregation |
| [PLAT-033-A](pulse_platform/plat-033.md) | Replace placeholder changelog refs with truthful artifact evidence | Claude Code | `implemented` | managed mutation/changelog writer |
| [PLAT-034-A](pulse_platform/plat-034.md) | Retain Raw tmux scrollback after process completion | Codex | `done` | terminal live-attach and chat-history persistence |
| [PLAT-036-A](pulse_platform/plat-036.md) | Suppress context percentage when coding-CLI usage is aggregate rather than a context snapshot | Codex | `runtime_reverify` | coding-CLI adapters and cost telemetry writer |
| [PLAT-037-A](pulse_platform/plat-037.md) | Attribute learning freshness to the actual writer | Unassigned | `new` | learnings freshness ledger writer |
| [PLAT-038-A](pulse_platform/plat-038.md) | Retain complete pre-validation evidence for every attempt | Codex | `runtime_reverify` | shared validation artifact writer |
| [PLAT-039-A](pulse_platform/plat-039.md) | Preserve and enforce advisor recommendation routes | Codex | `runtime_reverify` | advisor artifact/lifecycle projection |
| [PLAT-040-A](pulse_platform/plat-040.md) | Let a chat/manual Pulse coexist with one full schedule and show trigger failures | Codex | `runtime_reverify` | scheduler lease lanes and Schedule UI |
| [PLAT-041-A](pulse_platform/plat-041.md) | Persist every expected cron occurrence before execution | Codex | `runtime_reverify` | scheduler tick and durable fire decisions |
| [PLAT-042-A](pulse_platform/plat-042.md) | Preserve human-answer actor kind and audit events | Codex | `runtime_reverify` | report human-input persistence |
| [PLAT-043-A](pulse_platform/plat-043.md) | Allow bounded read-only SQLite integrity PRAGMAs | Codex | `runtime_reverify` | guarded workflow DB query policy |
| [PLAT-044-A](pulse_platform/plat-044.md) | Reconcile false stage-ownership findings when finalization succeeds | Codex | `runtime_reverify` | review guidance and final-command lifecycle |
| [PLAT-045-A](pulse_platform/plat-045.md) | Consume an answered decision when its finding reaches an outcome | Codex | `runtime_reverify` | finding and human-input lifecycle |
| [PLAT-046-A](pulse_platform/plat-046.md) | Reject an empty reviewer verdict at the typed tool boundary | Codex | `runtime_reverify` | typed Pulse reviewer tools |
| [PLAT-047-A](pulse_platform/plat-047.md) | Design immutable physical identity for grouped run artifacts | Unassigned | `design_required` | run-folder retention and execution identity |
| [PLAT-048-A](pulse_platform/plat-048.md) | Bound retained tmux to interactive chats and close completed-live processes | Codex | `runtime_reverify` | coding-agent modes, terminal leases/routes, server shutdown |
| [PLAT-049-A](pulse_platform/plat-049.md) | Keep shared platform mechanics out of workflow artifacts | Codex | `runtime_reverify` | plan guards, review guidance, and version upgrade |
| [PLAT-050-A](pulse_platform/plat-050.md) | Keep Pulse reasoning in one continuing agent conversation | Codex | `runtime_reverify` | scheduler Pulse orchestration and event-driven completion |
| [PLAT-051-A](pulse_platform/plat-051.md) | Stamp contract upgrades through a real registered agent tool | Codex | `runtime_reverify` | version-upgrade guidance and Workshop tool surface |
| [PLAT-052-A](pulse_platform/plat-052.md) | Keep one native CLI alive across known scheduler turns | Codex | `runtime_reverify` | scheduler request lifecycle and coding-agent mode |
| [PLAT-053-A](pulse_platform/plat-053.md) | Give background workshop children the complete parent tool surface | Codex | `runtime_reverify` | background-agent construction and direct tool definitions |
| [PLAT-054-A](pulse_platform/plat-054.md) | Never expire a scheduler turn whose child work is still running | Claude Code | `runtime_reverify` | scheduler idle/turn-completion waits |
| [PLAT-055-A](pulse_platform/plat-055.md) | Give the reflection turn every store it must route to | Claude Code | `runtime_reverify` | step post-completion turns, learnings/KB contribution contract |
| [PLAT-056](pulse_platform/plat-056.md) | Stop the repair-loop recorder from filing durable concerns for same-attempt superseded iterations | unassigned | `open` | prevalidation / `__automatic_final_validation__` repair loop |
| [PLAT-057](pulse_platform/plat-057.md) | A harness_issue must not be parked in the workflow's own engineering queue | Claude Code | `runtime_reverify` | finding disposition/status coherence |
| [PLAT-058](pulse_platform/plat-058.md) | Keep learnings one topic-organised workflow skill, not per-step files | Claude Code | `runtime_reverify` | step reflection turn learnings target |
| [PLAT-059](pulse_platform/plat-059.md) | A learnings lock must state why | Claude Code | `implemented` | update_step_config lock path |
| [PLAT-060](pulse_platform/plat-060.md) | Ops-owned config decisions must carry their reason into step_config.json | Claude Code | `runtime_reverify` (deferred: llm_ops_review disabled) | update_step_config tier/model/mode path |
| [PLAT-061](pulse_platform/plat-061.md) | step_config field audit: dead field, orphans, phantom clears, incomplete merge | Claude Code | `implemented` | AgentConfigs surface |
| [PLAT-062](pulse_platform/plat-062.md) | Scripted prompt named a write target the folder guard forbids | Claude Code | `runtime_reverify` | scripted execution prompt |
| [PLAT-063](pulse_platform/plat-063.md) | Report pane flashed mid-run: a view flip or outer React render reloaded it | Claude Code + Codex | `done` (two independent frontend paths fixed; live reverified) | workflow canvas mounting and report iframe |
| [PLAT-064](pulse_platform/plat-064.md) | An entire workflow_* event family is dead; one completion check had no live fallback | Claude Code | `implemented` | frontend event-type consumers |
| [PLAT-065](pulse_platform/plat-065.md) | Gate recorded a due module and nothing ever resolved it | unassigned | `open` (detection shipped, proximate cause isolated) | scheduler Pulse orchestration (`abortIfTurnStillBusy`, `runPostRunMonitor`) |
| [PLAT-066](pulse_platform/plat-066.md) | route_selections correctly supplied, never seeded; router defaulted to the live-action route | unassigned | `open` (interim mitigation shipped) | step-based workflow orchestrator (`seedRouteSelectionsForRun`) |
| [PLAT-067](pulse_platform/plat-067.md) | Do not dispatch a scheduled continuation to a missing/unready parent coding-agent transport | unassigned | `implemented` (root cause of the transport loss still open) | scheduler parent-session recovery and background completion delivery |
| [PLAT-068](pulse_platform/plat-068.md) | The step-type checklist names an automated owner that never loads it | unassigned | `implemented` (verification blocked: `llm_ops_review` disabled at the Gate) | Pulse guidance (`review/ops-review.md`, `builder/design-plan.md`) |
| [PLAT-069](pulse_platform/plat-069.md) | Nothing measures whether a workflow gets cheaper/faster/more accurate over time | unassigned | `open` (design written) | Pulse trend measurement + Pulse popup |
| [PLAT-070](pulse_platform/plat-070.md) | A failed run-folder listing makes the scheduler blame an old failure on today's run | unassigned | `implemented` (runtime reverify pending) | scheduler run-outcome reconciliation, workspace state loading |
| [PLAT-071](pulse_platform/plat-071.md) | An idle-wait timeout is treated as proof the workflow never ran | unassigned | `implemented` (record corruption); session stall still open | scheduler workshop turn loop |
| [PLAT-072](pulse_platform/plat-072.md) | `external_action_required` has no exit path, so solved problems keep being re-reported as open | unassigned | `implemented` (sweep tool + version stamping; board 81→75) | Pulse finding lifecycle |
| [PLAT-073](pulse_platform/plat-073-remaining-board.md) | Working list of the remaining `external_action_required` board that PLAT-072's triage sweep produced — a durable tracking document, not a single defect, kept so the remaining items stay assignable without re-deriving them from `pulse_close_stale.py --list` | unassigned | `open` (tracking document for the post-triage backlog) | `pulse_close_stale.py` + the external_action_required backlog |
| [PLAT-074](pulse_platform/plat-074.md) | 6 of 16 plan-mutation call sites never fed the changelog writer real diff/snapshot data, collapsing before_ref/after_ref to a meaningless placeholder | unassigned | `implemented` (4 of 6 call sites fixed; runtime reverify pending) | plan changelog writer (`planning_agent.go`) |
| [PLAT-075](pulse_platform/plat-075.md) | Auto-evaluation starts before its target execution is finalized | Codex | `runtime_reverify` | batch execution / auto-evaluation boundary |
| [PLAT-076](pulse_platform/plat-076.md) | Learning and scripted metadata record claims instead of runtime facts | Codex | `runtime_reverify` | learning detection and scripted metadata persistence |
| [PLAT-077](pulse_platform/plat-077.md) | Human-input answer/dismiss had no concurrent-writer guard; harness findings could split invisibly across two fingerprints | unassigned | `implemented` (runtime reverify pending) | report-human-input lifecycle, pulse finding identity migration |
| [PLAT-078](pulse_platform/plat-078.md) | Spilled bridge tool output (large agent_browser snapshots) landed outside every granted read path | unassigned | `implemented` (folder-guard fix only; snapshot size cap still open) | step execution folder guard |
| [PLAT-080](pulse_platform/plat-080.md) | An old cron schedule with no durable fire-decision row restarted from “now” and silently lost earlier due occurrences | Codex | `implemented` (runtime reverify pending) | scheduler cron-cursor bootstrap |
| [PLAT-081](pulse_platform/plat-081.md) | Workflow-builder chat cost writer merged cumulative usage repeatedly; first replacement fix could erase other chats | Codex | `implemented` (per-chat delta fix tested, 2 findings documented, 1 left design-only) | phase token-usage cost ledger |
| [PLAT-082](pulse_platform/plat-082.md) | Failed async child agents were reported completed because the internal sync boundary erased their Go error | Codex | `implemented` (runtime reverify pending; 2 companion findings classified) | todo-task sub-agent execution boundary |
| [PLAT-083](pulse_platform/plat-083.md) | No-run Pulse Finalizer instructed the agent to record an invalid "dashboard" command, surfaced live once PLAT-073-A made the rejection visible | unassigned | `implemented` (runtime reverify pending) | scheduler Pulse Finalizer prompt |
| [PLAT-084](pulse_platform/plat-084.md) | Scheduled runs using execute_step directly had no Pulse evidence signal, so Gate/Review+Fix/Fixer/dashboard/publish were silently skipped | unassigned | `implemented` (runtime reverify pending) | scheduler Pulse evidence detection, execute_step registration |
| [PLAT-086](pulse_platform/plat-086.md) | Route-backed and direct-sequence schedules had different lifecycle guarantees but agents could not see or record that design choice | Codex | `implemented` (v1.0.25 migration; runtime reverify pending) | schedule execution-model contract |
| [PLAT-087](pulse_platform/plat-087.md) | Message-sequence children advertise MCP servers and tools that are not actually registered | unassigned | `open` | child-session AgentSpec/tool materialization |
| [PLAT-088](pulse_platform/plat-088.md) | Every scheduled workflow and Pulse turn was billed to `chat`, making Pulse-vs-goal cost unmeasurable | unassigned | `implemented` (runtime reverify pending) | cost scope attribution (`handleQuery`, `pkg/costobserver`) |
| [PLAT-089](pulse_platform/plat-089.md) | Grouped runs leave previous-run attempt logs in the active evidence folder | unassigned | `open` | grouped-run cleanup and execution-evidence identity |
| [PLAT-090](pulse_platform/plat-090.md) | Daily Pulse cost is ledgered, but no durable per-Pulse-run/stage cost and timing record exists, so reviewer/fixer spend cannot be tied to one pass and its outcome | unassigned | `open` (2026-08-16 live distinction verified; per-pass measurement not built) | Pulse run measurement surface + cost ledger read path |
| [PLAT-091](pulse_platform/plat-091.md) | Evaluation step children never complete, pinning the session busy so Pulse loses Review+Fix, Finalize, backup and notify | unassigned | `implemented` (runtime reverify pending) | background-agent completion for evaluation step executions |
| [PLAT-092](pulse_platform/plat-092.md) | Answered operator decisions are never applied or consumed; 26 stranded across 6 workflows, oldest 31 days | unassigned | `implemented` (drain contract shipped; historical backlog stranded) | Pulse Review+Fix decision drain |
| [PLAT-093](pulse_platform/plat-093.md) | Answered decisions were applied after the run, so the run they were meant to change had already happened | unassigned | `implemented` (runtime reverify pending) | scheduler pre-run turn sequence |
| [PLAT-094](pulse_platform/plat-094.md) | A stale busy signal aborted Pulse Finalize on a turn that had already finished, matching the PLAT-071 race at a different call site | unassigned | `implemented` (runtime reverify pending) | Pulse step-boundary busy check |
| [PLAT-095](pulse_platform/plat-095.md) | Scheduled messages had no exact lifecycle identity, so session-wide idle/event heuristics could advance early or stall forever. Replaced both waiters with one query-rooted recursive execution tree, linked child-result continuations, shared the runtime projection with Global Monitor, and removed Go-inferred Pulse final-command failures | Codex | `implemented` (live reverify pending) | query dispatch + scheduler + runtime activity lifecycle |
| [PLAT-096](pulse_platform/plat-096.md) | Contract upgrades had two delivery paths (Pulse still carried them four weeks after the pre-run preflight replaced it, bundling four unverified rungs), an unfenced stamp a closed turn could still write, and no way for an unattended turn to finish or escalate | Claude Code | `implemented` (runtime reverify pending: confida-login has not yet climbed 1.0.20 → 1.0.25) | contract-upgrade preflight, stamp authorization, Pulse dispatch |
| [PLAT-097](pulse_platform/plat-097.md) | `update_schedule(messages=[])` and `messages=null` both silently no-op instead of clearing, blocking any messages-based schedule from migrating to the route-backed model | unassigned | `implemented` (runtime reverify pending) | update_schedule argument parsing, SchedulerCallbacks.UpdateSchedule |
| [PLAT-098](pulse_platform/plat-098.md) | Contract upgrades were invisible to a workflow owner (no tool, no API, no UI — only a version and a counter in run history) and unrunnable by hand; PLAT-096's stamp fence then removed the one informal manual route. Adds get_contract_upgrades, scopes the fence to scheduler sessions, and guards an operator stamp against skipping the ladder | Claude Code | `done` | workshop tool surface, stamp authorization, operator visibility |
| [PLAT-099](pulse_platform/plat-099.md) | Updating a workflow from one coding-agent provider to another could leave continuation metadata on the old provider, causing both live-input paths to reject messages despite a healthy new-provider tmux | Codex | `implemented` (live reverify pending) | retained coding-agent live-input routing + continuation metadata |
| [PLAT-100](pulse_platform/plat-100.md) | Workshop executions and live-steered retry continuations could detach from the initiating query, allowing a schedule to advance while a full-workflow orchestrator tree was still running | Codex | `implemented` (live reverify pending) | workshop execution launch + live completion continuation + exact conversation-turn lifecycle |
| [PLAT-101](pulse_platform/plat-101.md) | Claude capacity exhaustion can strand a workflow as running; preserve structured reset timestamps, try fallbacks, then durably pause and resume the exact unfinished run | unassigned | `open` (design agreed; implementation pending) | Claude adapter + mcpagent typed errors + workflow continuation scheduler |
| [PLAT-102](pulse_platform/plat-102.md) | Warm retained messages are fast, but cold coding-agent startup and response observation mixed unrelated waits into perceived send latency | Codex | `partially implemented` (audited 2026-08-14: startup timing, snapshot and trailing-wait fixes landed; Pi latency instrumentation, total request latency, and the latency E2E outstanding) | retained input delivery + Codex interactive startup timing |
| [PLAT-103](pulse_platform/plat-103.md) | Retained turns completed without a structured final response; adjacent user/assistant event races could render duplicate chat messages | Codex | `implemented` (runtime reverify pending) | retained-turn output contract + formatted transcript reconciliation |
| [PLAT-104](pulse_platform/plat-104.md) | HTTP acknowledgement and SSE durable events act as competing frontend message producers instead of reconciling one client-generated identity | unassigned | `open` (design recorded; implementation deferred) | frontend chat transport + event-store reconciliation |
| [PLAT-105](pulse_platform/plat-105.md) | Retained delivery works through a durable mcpagent Session, but the 2026-08-15 Social Media run proved the per-turn lifecycle is still broken: Codex captured the final response and exited while AgentWorks remained busy and Formatted mode never received the answer. Every accepted turn needs one stable `turn_id`, exactly one canonical completion, host settlement from that event, and a P0 proving the final answer is visible, busy clears, and the same Session remains reusable. | Codex | `p0_blocking` (live regression reproduced; completion producer/bridge boundary must be traced and fixed) | mcpagent ↔ AgentWorks per-turn completion ownership |
| [PLAT-106](pulse_platform/plat-106.md) | Concurrent Chat and Schedule tabs for one workflow can render a Schedule event inside Chat, falsely pairing unrelated user and assistant messages | unassigned | `partially implemented` (2026-08-15: root cause was Codex retained-answer lookup resolving by working dir + newest mtime across two sessions sharing a workflow directory — now bound to the exact thread/rollout; frontend ownership guards and session-switch reset added as defence in depth; live verification pending) | codex retained-answer thread binding + frontend session-event ownership |
| [PLAT-107](pulse_platform/plat-107.md) | A sequential scheduled main-agent turn can be projected as its own child, auto-selected, and shown as a blank terminal placeholder | unassigned | `partially implemented` (2026-08-15: self-parent turns now collapsed out of the rail entirely; runtime verification pending) | execution-tree terminal projection + selection |
| [PLAT-108](pulse_platform/plat-108.md) | Coding-agent transcripts are located by working directory + recency instead of conversation identity; the same defect was fixed independently in Cursor and Codex and is still reachable in Codex completion detection and structured streaming | unassigned | `partially implemented` (2026-08-15: Codex interactive transport fully bound — retained, wrapped, completion detection, streaming; structured transport and the contract capability + certification pending) | coding-agent transcript identity contract + certification |
| [PLAT-113](pulse_platform/plat-113.md) | Session turn occupancy was decided by sessionBusy, a display flag never set for workflow turns, so every background-agent completion during a scheduled run skipped its queue and blocked on the input lane — 25 never-started synthetic turns piled behind one 5-hour turn until the idle-wait watchdog killed a healthy run | unassigned | `partially implemented; unverified at runtime` (2026-08-16: lane authoritative + register-after-acquire landed; sessionBusy demotion pending. Backend down since 10:08 IST, so neither fix has executed once — next check is the 15:00 IST slot) | session turn occupancy + auto-notification queueing |
| [PLAT-109](pulse_platform/plat-109.md) | Switching workflows can falsely show no chats, retain the previous workflow selection, or focus a runtime tab until refresh even though the destination workflow has durable Chat history | unassigned | `open` (live recurrence 2026-08-15; trace workflow-switch hydration and selection ordering before fixing) | frontend workflow switch + chat-index hydration + canonical Chat selection |
| [PLAT-110](pulse_platform/plat-110.md) | Codex verified/isolated launch can retain an undeclared allowlisted credential and expose declared secret values in process arguments | unassigned | `open_deferred` (not reachable in current compatibility mode; release gate before strict-mode rollout) | `multi-llm-provider-go/internal/clisandbox` + AgentWorks strict-mode E2E |
| [PLAT-111](pulse_platform/plat-111.md) | Cost Analysis blocks first paint on an unbounded all-history ledger/file scan and then serially loads logs for every historical run | platform | `implemented_pending_live_reverify` (bounded 30-day summary + exact SQL headlines + cursor pagination; initial log fan-out removed 2026-08-16) | cost API + ledger rollups + `CostsPopup` lazy detail loading |
| [PLAT-112](pulse_platform/plat-112.md) | Production workflow UI exposes the internal terminal/child-agent debug rail and continues terminal observation work even when nobody is debugging | platform | `implemented_pending_live_reverify` (default-off server/client diagnostic gates; normal Chat/Schedule now use session events without terminal/tree polling, 2026-08-16) | runtime projection + terminal observation APIs + workflow workspace UI |
| [PLAT-114](pulse_platform/plat-114.md) | Background agents (Pulse's Gate/reviewers/Fixer included) had no durable execution record — only a receipt a module's Fixer turn may never write, and a 200-event UI cache overwritten on session reuse | unassigned | `implemented` (durable log shipped and tested; no query surface yet) | background-agent lifecycle |
| [PLAT-115](pulse_platform/plat-115.md) | Gate/Review+Fix/Finalize always ran inside the same session as the run they reviewed. Shipped periodic mode as opt-in (frequency-gated), then that same-day migration revealed the frequency gate used the wrong signal — PLAT-113's original incident happened on an infrequent workflow too. Policy changed: periodic mode is now mandatory for every workflow, bootstrapped by Gate itself the first time it runs a normal `per_run` pass, not by a dedicated migration turn | unassigned | `implemented` (mandatory policy + Gate-owned bootstrap shipped and tested; no real workflow has actually been bootstrapped yet — happens on each workflow's next normal Gate pass) | scheduler Pulse orchestration, workflow manifest, schedule tools, pulse-gate.md |
| [PLAT-116](pulse_platform/plat-116.md) | A completed coding-CLI turn can leave the canonical formatted stream open even though the provider-native final answer is already durable and visible. The 2026-08-20 Social Media recurrence pinned the Codex instance to a cross-session lock inversion during rollout resolution and exposed the missing provider-neutral per-turn lifecycle contract. | unassigned | `implemented_pending_restarted_ui_reverify` — Codex lock inversion fixed; `mcpagent` now owns stable turn IDs and exactly-once unified completion; live retained-session P0 passed for Codex, Claude Code, Cursor, and Pi; live concurrent Codex and race tests passed. Remaining proof is one restarted AgentWorks Social Media run. | provider adapters + mcpagent canonical turn lifecycle + AgentWorks completion consumer + scheduler safety net |
| [PLAT-117](pulse_platform/plat-117.md) | Per-step workflow progress records are registered as background agents purely so the UI keeps polling, but the same registry is the authority for `RunningChildren` — so one dropped progress end-event made `terminal()` permanently unreachable and guaranteed an idle-wait timeout. This is the mechanism behind social-media's false "workflow did not start" emails: a post had landed and been verified, yet two orphaned step-0 mirrors held the turn open, burned the 3-hour live-child grace, and the operator was told nothing ran | unassigned | `implemented` (progress mirrors excluded from turn liveness but still counted for progress/display; orphan settling completed; PLAT-071 regression restored — fail-before/pass-after verified) | background-agent registry + conversation turn lifecycle + workflow progress bridge |
| [PLAT-118](pulse_platform/plat-118.md) | Codex | `implemented; core runtime verified` (Landlock rootless backend + capability-based selection + fail-closed `SANDBOX_UNAVAILABLE`; live-verified on the hardened EC2 host as uid 999. Code claims independently re-reviewed 2026-08-17 — all hold. Two items deliberately open: nested `BlockedWritePaths` precedence cannot be expressed in additive Landlock rules and fails closed instead, and a fal.ai auth-only probe was not run) | `new` (code claims independently verified 2026-08-17 — all hold; review added to the ticket noting the stderr/reporting question, the pre-existing unguarded exec path, and Landlock ABI/contract caveats) (live root cause reproduced 2026-08-17; rootless Landlock backend, capability detection, truthful preflight, and runtime reverify required) | shared workspace shell sandbox + Linux deployment health |
| [PLAT-119](pulse_platform/plat-119.md) | Every Pulse step opens with "load builder-reference and follow it exactly", but the Pulse session is built from the workflow's `selected_skills` and never gets that platform skill — so on a workflow that lacks it, Gate/Review+Fix/Finalize improvise. Nothing checks, nothing warns, and the pass is recorded as normal; the only reason this was found is that one Gate run volunteered it | `implemented` (root cause was narrower than first written: the reference surface was attached INSIDE `if workshopSession != nil`, and workshop creation is skipped for an already-stopped session — exactly the state a Pulse finalizer runs in after the run's transport dies. Moved outside the guard; fail-before/pass-after regression test pins the structure) | `new` (root cause confirmed in code: `selected_skills: sctx.Capabilities.SelectedSkills`; salesoutreach had `["agent-browser"]` only, exactly matching the agent's report) | scheduler Pulse orchestration + platform skill attachment |
| [PLAT-120](pulse_platform/plat-120.md) | Video Studio voice dictation (sherpa-onnx-go + Nemotron streaming STT, capability-gated via `agentprofiles.RuntimeCapabilities.Voice`, SparkQuill-parity UX) shipped and passed two independent non-mic proofs (WAV file, synthetic tone), but no real-speech end-to-end pass has succeeded on the one machine it's been tested on | unassigned | `implemented_pending_live_reverify` (blocked on PLAT-122) | `pkg/voicestt`, `cmd/server/voice_stt_routes.go`, `frontend/src/voice/*` |
| [PLAT-121](pulse_platform/plat-121.md) | SparkQuill's release process was already split into its own CI workflow with its own tag namespace and path-scoped triggers, contrary to how it was carried as open on the task list; 5 most recent `main` CI runs are green | unassigned | `implemented` (CI-verified; no further work identified) | `.github/workflows/sparkquill-desktop.yml` |
| [PLAT-122](pulse_platform/plat-122.md) | A dev machine's real microphone reads exact digital silence (`rms=0.0000`) through every app, not just this one — full elimination chain ruled out the STT pipeline, wrong device, browser-automation fake device, SparkQuill holding the device, and third-party audio HAL drivers | unassigned | `open` (environment-level, not code-owned; blocks PLAT-120's live reverify) | none identified yet — OS/driver layer; see `voice_dictation_mic_captures_silence.md` |
| [PLAT-123](pulse_platform/plat-123.md) | `record_run_concern`'s trusted session identity was wired up correctly, but three of four places that build a workflow step's tool list never added the tool itself to `SelectedTools`/`enabledTools` — so it silently no-oped for any step outside the one path (a custom `EnabledCustomTools` allowlist) that already force-included it. Confirmed live: confida-login's survey-app-and-refresh-knowledge step named the tool in its own reflection-turn prompt and got `tools_unavailable: unknown=[record_run_concern]` | unassigned | `implemented` (fixed all three call sites to match the one already-correct one; two new regression tests pin the default-branch gap; live reverify on a freshly-started run still pending) | `controller_agent_factory.go` (`applyStepConfigToAgentConfig`, `createTodoTaskOrchestratorAgent`, `prepareCustomTools`) |
| [PLAT-124](pulse_platform/plat-124.md) | An oversized agent_browser snapshot (>24000 runes) returned an error and DISCARDED the tree, so the agent had to pick a narrower --selector for a page it was never allowed to see; separately, the bridge's spill target (tool_output_folder) was granted by setupExecutionFolderGuard but not by the two parallel guard builders, so a step handed "full output saved to <path>" was forbidden to open it. Confirmed live on confida-login: 4 blind retries at ~30.4k runes, plus an "outside every workspace root" dead end | unassigned | `implemented` (truncated head + explicit incompleteness banner, new opt-in `--full-snapshot`, tool_output_folder granted in all three builders and pinned by a parity test; live reverify pending) | `pkg/browser/executor.go` + message_sequence/KB-update folder guards |
| [PLAT-125](pulse_platform/plat-125.md) | Every workflow step agent — orchestrator, message_sequence, routing, regular, sub_agent alike — is handed the workshop chat's 41-doc builder reference bundle, so a step holding 8 tools is instructed to call provider-configuration tools it does not have; failing that, it invented provider names, producing 19 further `search_web_llm` errors | unassigned | `implemented` — capability-derived selection shipped with regression tests; live reverify pending | `pkg/orchestrator/agents/workflow/step_based_workflow/supplementary_prompts.go`, `cmd/server/guidance` |
| [PLAT-126](pulse_platform/plat-126.md) | An unquoted JSON path to `json_extract` (`json_extract(col, $.field)` instead of `json_extract(col, '$.field')`) fails with "unrecognized token: \"$\"", which names the character SQLite choked on but never the fix — 22 identical failures on one workflow in one day, none self-corrected | unassigned | `implemented` — hint shipped with fail-before/pass-after tests against the real production path; live reverify pending | `cmd/server/virtual-tools/workflow_db_tools.go` |
| [PLAT-127](pulse_platform/plat-127.md) | The tool-error suspect scan flagged two documented successful outcomes as failures — agent_browser's `{"waited":"timeout"}` wait outcome (142 of 524 suspects on one workflow in one day) and get_route_description's route-catalog prose (12 of 12) | unassigned | `implemented` in [manishiitg/mcpagent@2200bad](https://github.com/manishiitg/mcpagent/commit/2200bad) — fail-before/pass-after tests; live reverify pending | `mcpagent/toolerr` (vendored via go.mod replace, no agent_go change) |
| [PLAT-128](pulse_platform/plat-128.md) | Guidance tests asserted content for review-improve-log and review-code, both deliberately deleted 8 days before ("simplify Pulse workflow reviews") — plus two unrelated test/production drift bugs (a missing DBGuidance template key, an impossible "must never mention the tool it explicitly forbids" check); 5 of a 24-failure baseline fixed | unassigned | `implemented`, 19 pre-existing failures remain (wording drift, out of scope) | `cmd/server/guidance/render_all_test.go`, `pkg/orchestrator/agents/workflow/step_based_workflow` prompt tests |
| [PLAT-129](pulse_platform/plat-129.md) | 13 more of PLAT-128's 19 remaining failures traced to specific renames/removals (call_generic_agent -> run_in_background, READ-ONLY REVIEW -> READ-ONLY STRATEGY AUDIT, finding_id -> "no invented identifier" for 3 specialists, 5 improve-* docs merged into shared Engineering Review lenses, goal-advisor's 538-line rewrite, builder/improve.html dashboard markup gone everywhere) | unassigned | `implemented`, 6 remain (2 trace to PLAT-080's scheduler mechanism, 2 to content that git history shows was never written, 1 to an unfamiliar persistence-model change, 1 untraced) | `cmd/server/guidance/render_all_test.go` |
| [PLAT-130](pulse_platform/plat-130.md) | Stopping a schedule marks it stopped but does not stop the work: cancelTrackedExecutionsForSession/cancelBackgroundAgents only flip an in-memory status flag read by watchers -- the goroutine actually driving a message_sequence item loop has no cancel-aware context and keeps executing queued items (including side-effecting ones) after the schedule is shown as "stopped" | unassigned | `implemented` — 2026-08-20: first live reverify reproduced it via a third mechanism (handleQuery's auto-resume/materialize-guard relaunching a killed coding-agent tmux after Stop); gated on isSessionMarkedStopped. Second live reverify same day is clean: no relaunch, in-flight turn cancelled honestly | `cmd/server/workflow_execution_tracker.go`, `cmd/server/session_lifecycle.go`, `cmd/server/server.go` |
| [PLAT-131](pulse_platform/plat-131.md) | Session enrichment overwrote preset_query_id/preset_name/workspace_path unconditionally from a tracked execution, so a running scheduled session whose only live execution was a workshop_background one (which carries no identity) had all three erased -- the frontend resolves a workflow by exactly those fields, so clicking the rtslatency activity pill opened a Schedule tab under whichever workflow was already on screen instead of switching | unassigned | `implemented` -- fail-before/pass-after against the exact live payload; live reverify pending a restart | `cmd/server/polling.go` |
| [PLAT-132](pulse_platform/plat-132.md) | A permanently-broken MCP server is re-attempted on every restart: startup tool-cache discovery walks every configured server (correctly — it feeds the `GET /api/tools` catalogue, which must list what is *available*, not what is used), but `discoveryFailedServers` only remembers **auth** failures. `MiniMax` dies on import (`ModuleNotFoundError: mcp.server.fastmcp`, no version pin — `google-sheets` next to it pins `mcp>=1.8,<2` and works), surfaces as `transport closed`, is never classified permanent, and burns ~30s + 4 error-level lines every restart for a server **zero** of 19 workflows select | unassigned | `open` (diagnosed, deliberately not fixed — needs a call on what counts as deterministic vs transient, and whether the skip should expire) | `agent_go/cmd/server/tools.go` (`initializeToolCache`, `runBackgroundDiscovery`, `discoveryFailedServers`) |
| [PLAT-133](pulse_platform/plat-133.md) | Claimed the P0 coding-agent contract enforces that a certification has a registered proof but never that the proof tests what the certification claims, citing the Pi hang as the example that slipped through | unassigned | `closed / not a defect` (2026-08-18: premise disproven on both halves. (1) There was no Pi hang — the `agent_settled`-never-fires claim was a logging artifact, refuted by live runs. (2) The coverage said to be missing is present: no provider is registered `Transport: structured` (it means *primary* transport), so all four get 16-17 required P0 certs incl. `structured_multi_turn`/`structured_streaming`. And a two-turn resume proof *does* test termination by construction — turn one must complete and exit or there is nothing to resume. All four P0 enforcement tests pass) | `multi-llm-provider-go` certification framework (`coding_agent_certification.go`, `coding_agent_contract.go`) |
| [PLAT-134](pulse_platform/plat-134.md) | Ordinary and product chat were assembled as a generic multi-agent orchestrator, constructing delegation, schedule and tier-selection machinery even when product profiles explicitly forbade those capabilities; the live path is now direct prompt + skills + tools + conversation, with compatibility naming/dead-code extraction still open | Codex | `partially_implemented` (runtime simplified and automated checks pass; live reverify plus compatibility cleanup pending) | direct/product chat request and runtime construction + chat UI model/config projection |
| [PLAT-135](pulse_platform/plat-135.md) | `TestDelegationUsesTheReadOnlyProfileLookup` asserted that a sub-agent resolves its parent profile through the read-only lookup, so it could never receive a wider tool surface than the product declared; PLAT-134 removed chat delegation and the assertion lost its subject, leaving the containment property unasserted on the workflow sub-agent path that still spawns them | unassigned | `open` | `cmd/server/agent_profile_runtime.go`, workflow sub-agent spawn path |
| [PLAT-136](pulse_platform/plat-136.md) | Every run executes in runs/iteration-0 and records it, but iteration-0 is only the live slot and the next run rotates it to a permanent iteration-N; run history was never repointed (24 of 25 hetznerssh entries said iteration-0 while disk held iteration-21..25), so the schedule popup — which looks cost up per folder — showed the CURRENT run's spend identically on every historical row | unassigned | `implemented` — rotation repoints run history; live reverify pending | `controller_run_manager.go` (rotatePairedIterationZero) |
| [PLAT-140](pulse_platform/plat-140.md) | A server restart cleared the in-memory state the app's chat record is built from; the resume rebuilt a partial history and persistence wrote it straight over the full one — salesoutreach went from 242 user turns (still intact in Claude Code's own transcript) to 2, with one copy on disk and no backup | unassigned | `implemented` — overwrite guard shipped; flattened records not recoverable | `chat_history_persistence.go`, `server.go` conversation write sites |
| [PLAT-141](pulse_platform/plat-141.md) | NO interactive (tmux) adapter emits a tool-call end on ANY provider — inherent, since a terminal pane carries no structured tool events — so a call that Claude Code's transcript shows completing in 41ms never produced a tool_call_end store event, so the chat showed a finished command as unresolved and the compensating settle displayed a fabricated 45.4s duration; tools dispatch under sub-session ids while open calls are tracked under the parent schedule session, but the pairing hypothesis is not yet proven | unassigned | `partially implemented` — Claude Code interactive recovery shipped, codexcli/cursorcli/picli interactive still share that gap. Separately (2026-08-20): a THIRD, provider/transport-agnostic mechanism found and fixed — `logToolCallTelemetry` had no case for `ToolCallErrorEvent`, so a tool call that failed with a real error (not silence) stayed "open" forever and was swept into the same synthetic settle as genuinely unreported calls, on a structured (non-tmux) pi turn. Fail-before/pass-after verified against the exact production log line | interactive adapters (all providers), `internal/events/event_store.go` |
| [PLAT-142](pulse_platform/plat-142.md) | Pulse's own review evidence has the same orphaned-tool-call gap PLAT-141 found in the chat UI — 15,601 tool calls across 745 execution logs, 2,563 (16.4%) with no recorded result, up to 88% on one workflow — because the two are independent consumers of the same incomplete event stream, not a pipeline; recovery shipped for regular/message_sequence steps on Claude Code, todo-task steps and the other three providers left as named follow-up | unassigned | `partially implemented` | `context_aware_bridge.go`, `controller_execution.go` |
| [PLAT-137](pulse_platform/plat-137.md) | Strategy Auditor and Goal Advisor duplicated one lifecycle; the manual Strategy slash wrapper also retained the old nested-reviewer path and could complete without the canonical `strategic_review` receipt | Codex | `implemented` — merged strategic sequence, direct standalone execution, exact-session receipt enforcement, and interference-domain experiment control shipped; live post-restart/compaction verification pending | Pulse module registry/Gate, strategic Review+Fix and standalone dispatch/guidance, typed persistence, experiment lifecycle, Pulse UI projection |
| [PLAT-138](pulse_platform/plat-138.md) | Engineering Review buried verification, discovery, Stores/Operations evidence, fixing and persistence inside one oversized contract; the first live backlog-drain then proved its “complete backlog” rule was also unbounded, assigning 198 roots to one child | Codex | `implemented` — compaction-safe sequence plus one agent-chosen coherent repair objective per pass; live post-restart verification pending | Pulse Review+Fix message-sequence dispatch/guidance, bounded progress, run-scoped checkpoints and typed persistence |
| [PLAT-139](pulse_platform/plat-139.md) | An `ICICI-BANK-PARSING-v2` workflow step finished its real work at 09:45 but held its caller ~65 minutes, never reporting completion | unassigned | `fixed` (2026-08-19: the original diagnosis in this ticket was WRONG and is corrected inline. It concluded "pi's process is gone, so cmd.Wait() is stuck on an internally-owned stderr pipe" — but the `ps` query used could not match pi, which runs as `COMM=pi`, not as a node process. Listing the server's real children during a live recurrence showed pi ALIVE (27m and 65m, elapsed matching the stall warnings, zero zombies), so cmd.Wait() was blocked correctly. Real cause: pi finishes its work and fails to EXIT — its MCP child keeps Node's event loop alive while waiting on a stdin pi never closes — and the `agent_settled` teardown that breaks this had been deleted from the pi adapter. Restored in `@3d9bcc6`; stall log now records pid + whether the terminal event was seen (`@000b917`) so the two cases cannot be confused again. The stderr-pipe fix `@6d8e4e9` is a real separate defect and is retained) | `multi-llm-provider-go` picli structured adapter |
| [PLAT-148](pulse_platform/plat-148.md) | PLAT-116 orphan cleanup closed the Build-in-Public Pulse conversation after Gate timed out, then the scheduler immediately sent Review+Fix into that closed session, stranding the remaining sequence and later schedules | unassigned | `implemented` — orphan cleanup is now turn-scoped and preserves the conversation | per-turn lifecycle + scheduled conversation continuation |
| [PLAT-143](pulse_platform/plat-143.md) | Workflow restoration is represented by one global frontend boolean, so hydration of another session can cover LinkedIn with a repeatedly re-armed `Restoring previous session…` screen while LinkedIn's own APIs are healthy | unassigned | `implemented` — active restore state is keyed by session | keyed frontend restore lifecycle |
| [PLAT-144](pulse_platform/plat-144.md) | Logically dependent schedules can only approximate ordering with cron offsets; Tectonic Daily Pulse fired 15 minutes after close, while close normally required 22+ minutes, and collided | unassigned | `implemented` — occurrence-linked dependency, terminal policy, delay, deadline, and restart-safe release shipped | schedule dependencies + overlap validation |
| [PLAT-145](pulse_platform/plat-145.md) | A busy workflow lease terminally discards every occurrence as `skipped_busy`; three Tectonic occurrences were durably observed and then permanently lost because no queue/coalesce/retry policy exists | unassigned | `implemented` — durable skip/queue-latest/retry/coalesce with atomic lease claim and visible waiting state shipped | durable scheduler collision policy |
| [PLAT-146](pulse_platform/plat-146.md) | Manual launch preserves schedule prose but no typed safety mode; the Tectonic “market close” schedule launched before cutoff could reach ordinary entry logic because close-only existed only as a time assumption | unassigned | `implemented` — runtime mode reaches and gates the script boundary | typed schedule inputs + side-effect enforcement |
| [PLAT-147](pulse_platform/plat-147.md) | The managed backup contract allowed a generated Git bundle to be tracked inside its own source repository; Tectonic reached a ~67 GB bundle and ~133 GB `.git` through recursive self-inclusion | unassigned | `contained` — canonical path guard, future-agent Git contract, bundle deletion, verified history rebuild, and external recovery bundle shipped; managed archive lifecycle/health remains separate work | backup destination validation + health |
| [PLAT-149](pulse_platform/plat-149.md) | Two independent mechanisms report the same bridge tool call under different identities — a toolcalllog-backed HTTP hook (reliable, provider-agnostic) and a second, unlocated mechanism carrying the model's own id that drops ~10% of results — both flowing to the same consumers unreconciled | unassigned | `implemented` — shared recovery shipped for chat UI + Pulse evidence; unreliable mechanism's construction site still unlocated | `pkg/toolcallrecovery`, `pkg/agentwrapper/llm_agent.go` |
| [PLAT-150](pulse_platform/plat-150.md) | live-attach started its tmux control client with `pty.StartWithSize` (an `exec.CommandContext`) and cleaned up with only `ptmx.Close()` + `cmd.Process.Kill()`. Kill terminates but does not REAP, so every attach left a `<defunct>` child of the server plus an os/exec `watchCtx` goroutine blocked forever on a send only `Wait` drains — one leaked PID and one leaked goroutine per attach, until restart | unassigned | `fixed` (2026-08-19: confirmed from a live production pprof dump — five zombies against exactly five `watchCtx` goroutines parked on `chan send`, four of whose Start-callers had already exited. Fixed with a deferred reap covering both the ctx-cancel and self-exit paths. Hard to find because a zombie is anonymous, the Start-vs-Wait heuristic reports clean, the cleanup code looks correct, and a simultaneous unrelated 64-minute `syscall.Wait4` hang masked it) | `agent_go/cmd/server/terminal_live_attach.go` |
| [PLAT-152](pulse_platform/plat-152.md) | Claimed Pi CLI's interactive adapter emits native tool-call chunks while Claude Code's and Codex CLI's do not, making the coding-agent adapters non-standardized | unassigned | `closed / not a defect` (2026-08-19: the asymmetry does not exist. Counting emission sites per PACKAGE rather than per file, all four providers emit `StreamChunkTypeToolCall*` — Claude at `claudecode_transcript_stream.go:211,220`, Codex at `codexcli_transcript_stream.go:142,152`, Cursor at `cursorcli_transcript_stream.go:258,261` — and all are live-tailed on a poll ticker, not end-of-turn. The original analysis grepped only `*_interactive_adapter.go` and inferred provider-level absence from file-level absence; Pi differs in file layout only, because its JS harness gives it a marker side-channel. Cursor, unexamined originally, has the identical structure. Acting on the finding would have duplicated every tool-call chunk) | `multi-llm-provider-go/pkg/adapters/*` |
| [PLAT-153](pulse_platform/plat-153.md) | A structured pi turn had no overall ceiling — TOOL_EXECUTION_TIMEOUT only wraps individual tool calls, and nothing bounded the whole pi process's lifetime. Confirmed live: a form-26as pi sat with a completely idle Node event loop (native `sample`, not a goroutine dump — main thread parked in kevent, no thread in any syscall) for over an hour with zero progress. Matches pi's own upstream issue (earendil-works/pi#8004: real 5.5h/8.7h freezes, "no general tool-call timeout") — not our bug, but our blast radius | unassigned | `fixed and confirmed live` (2026-08-19: piMaxTurnDuration adds a 45m default ceiling, PI_STRUCTURED_MAX_TURN_DURATION-overridable. A second real bug found proving it: exec.CommandContext's default cancellation only kills the tracked pid, not the process group Setpgid was set up for — a grandchild survived as a reparented orphan three times before the test was strengthened to actually check for it. Fixed with cmd.Cancel doing a proper group kill, matching procshutdown's existing correct pattern. Both fail-before/pass-after verified. Confirmed same-day: fired at exactly 45m0.029s on a real recurrence, group-killed cleanly, workflow recovered within 2 minutes via retry) | `multi-llm-provider-go` picli structured adapter |
| [PLAT-151](pulse_platform/plat-151.md) | Todo routes advertised `context_to_pass`, but runtime never delivered it to child sessions; descriptions, real file dependencies, and per-call instructions are now the only context channels | Codex | `implemented` — dead field removed from backend/frontend and mutation schemas; focused tests pass | todo-route plan contract |
| [PLAT-154](pulse_platform/plat-154.md) | `record_pulse_finding` successfully updated a step-owned issue, then reloaded through the observing reviewer's module filter, could not see the row, and falsely returned an internal-lifecycle error that triggered retries | Codex | `implemented` — unfiltered exact-identity reload and cross-module regression proof pass; live restart/retry verification pending | typed Pulse finding write/reload boundary |
| [PLAT-155](pulse_platform/plat-155.md) | Pulse flattened 98 raw workflow observations and 2 reviewer-confirmed issues into one “100 open” backlog, then let the reviewer sequence repair its own mixed queue | Codex | `implemented` — explicit observation/issue projection and independent Review→Fix stages; focused tests pass, live convergence verification pending | Pulse evidence projection, backlog/Gate contract, reviewer promotion, and Fixer dispatch |
| [PLAT-156](pulse_platform/plat-156.md) | `review_plan` injected a 553 KB plan plus a Go-authored step-config interpretation into the reviewer prompt instead of letting the agent inspect authoritative source files selectively | Codex | `implemented` — reviewer now reads soul/plan/step config itself with targeted queries; live reverify pending | `runReviewPlanAgent` + review-plan prompt |
| [PLAT-157](pulse_platform/plat-157.md) | `call_sub_agent` guidance made parents repeat contracts predefined children already receive, stateful routes replayed that combined contract on continuation, and conversation inspection used ambiguous `todo_id` instead of exact `execution_id` | Codex | `implemented` — contract simplified and focused tests pass; live reverify pending | workflow todo-task sub-agent contract |
| [PLAT-158](pulse_platform/plat-158.md) | Legacy `post_run_monitor` remained a second Pulse trigger beside a dedicated review schedule, so ordinary workflow runs still launched Gate/Review+Fix inline and could spend hours doing review work at the wrong cadence | Codex | `implemented` — dedicated `pulse_review_only` schedule is now the sole recurring-Pulse authority; restart/live reverify pending | manifest + scheduler Pulse lifecycle dispatch |
| [PLAT-159](pulse_platform/plat-159.md) | Scheduler/cron `workflow_phase` sessions set WorkflowLabel/PresetName from the raw workspace folder name instead of the workflow's configured `label`, so the Global Activity Monitor could show a session's own workflow as a different one running elsewhere (folder `social-media`, configured label `twitter-automation`) | unassigned | `implemented` — fail-before/pass-after; live reverify pending | `cmd/server/server.go`, `cmd/server/polling.go` |
| [PLAT-160](pulse_platform/plat-160.md) | Interactive tool-call completion is reconstructed by polling the CLI's transcript file, whose loop exits on turn-end without a final read and can lose the last event; PLAT-149's proposed reliable alternative (the bridge-execution hook) turns out to have a dead live-delivery path of its own (noop tracer, and unregistered on the workflow-step code path) | unassigned | `partially implemented` — settled-timestamp and shell-settle-presentation symptoms fixed; claudecode/codexcli transcript tailers now do a final read on cancel (matching cursorcli's already-correct pattern), closing the confirmed root cause with low risk. The larger architectural fix (promote toolcalllog to primary) remains unstarted, scoped larger than first filed | `multi-llm-provider-go` interactive transcript tailers, `agent_go/pkg/agentwrapper/llm_agent.go`, `agent_go/internal/events/event_store.go` |
| [PLAT-161](pulse_platform/plat-161.md) | bd184c1f6's report-frame height fix collapsed the platform iframe to 0px to measure content, which also collapsed any `vh`-sized content inside the report (a report's own nested iframe, in salesoutreach's case) for the duration -- its GTM-strategy tab measured 319px against a real 2052px, with nothing to scroll to reach the rest | unassigned | `implemented` — verified against a real Chromium browser loading the actual affected report | `frontend/src/components/workflow/reportWidgets/HtmlWidgetFrame.tsx` |
| [PLAT-164](pulse_platform/plat-164.md) | Background agents (`run_in_background`, todo-task delegates, scheduled/Pulse children) had only a durable lifecycle summary (PLAT-114), not a transcript — intermediate assistant messages and tool calls existed only in the capped/reused live UI-event cache, unrecoverable once trimmed or after a restart | unassigned | `implemented` — generic per-background-agent structured transcript, keyed by session+agent below `builder/conversation/background/` (corrected from this ticket's own original `sessions/` proposal, which had no other usage anywhere in this codebase); write-failures are visible via new `background_agent_log.transcript_status`. Scope-fix same day: the initial scoping check reused a pre-existing "workflow-step:" prefix classifier that matches nothing real in this codebase, so the bridge-side appender was fabricating spurious transcripts for plain workflow-step/todo-task step executions (which already have their own conversation+timing trace via `controller_execution.go`/`controller_todo_task.go`); it now only appends when cmd/server already registered a header. Build/test verified, live reverify pending | `cmd/server/background_agent_transcript.go`, `pkg/orchestrator/context_aware_bridge.go`, `pkg/orchestrator/base_orchestrator_background_transcript.go`, `pkg/orchestrator/events/background_transcript.go` |
| [PLAT-166](pulse_platform/plat-166.md) | The SQLite cost ledger (`pkg/costledger`, what Cost Analysis' `CostsPopup.tsx` actually reads) has no phase attribution, so a step's execution-turn and reflection-turn spend sum into one indistinguishable per-step row; a different, older token-usage-file system already split this (PLAT-068) but feeds a different consumer entirely | unassigned | `implemented` — `Entry.Phase` + `ExecutionAggregate.ByPhase` in the ledger, `costobserver.Observer.SetPhase` toggled around the reflection turn (a completely separate `AgentEventListener` from the bridge push the code had always assumed sufficed), `CostsPopup.tsx` renders a reflection sub-line when present. Scope-fix same day (caught by review): every observer had defaulted to `PhaseExecutionOnly`, so *every* execution — not just ones with a reflection turn — grew a redundant `by_phase.execution_only` duplicate of its own total in every API response; now defaults to untagged (`""`), which writes no `by_phase` entry at all. Build/test verified, live reverify pending | `pkg/costledger/ledger.go`, `pkg/costledger/sqlite.go`, `pkg/costobserver/observer.go`, `pkg/orchestrator/agents/workflow/step_based_workflow/reflection_turn_run.go`, `frontend/src/components/workflow/CostsPopup.tsx` |
| [PLAT-167](pulse_platform/plat-167.md) | PLAT-166's Phase/ByPhase mechanism is generic (any string, aggregated automatically) but only one caller (the reflection turn) ever used it; a `message_sequence` step's individual items (user_message, prevalidation repairs, foreach rows) all funnel through one call site and had no phase tag, so their cost/time stayed merged into one row | unassigned | `implemented` — `executeMessageSequenceUserMessage` tags every item's turn with `"item:<id>"` via the same `SetPhase` mechanism; `CostsPopup.tsx`'s render generalized from a hardcoded single reflection check to iterating every tagged phase. Build/test verified, live reverify pending | `pkg/orchestrator/agents/workflow/step_based_workflow/controller_message_sequence.go`, `frontend/src/utils/costActivityBreakdown.ts`, `frontend/src/components/workflow/CostsPopup.tsx` |
| [PLAT-168](pulse_platform/plat-168.md) | After a successful `query_workflow_db` call, an agent redundantly hand-rebuilt the identical HTTP request via inline Python inside `execute_shell_command`, hit `SyntaxError: unterminated triple-quoted string literal`, and retried 19+ variations of the same fragile shell-smuggled-script pattern for 45 minutes until PLAT-153's turn ceiling caught it — 212 tool calls, never silent, never converging. Also records a design decision reached the same day: a real-time consecutive-failure/loop detector was considered and explicitly rejected (no reliable live signal separates 'about to converge' from 'never will'; the ceiling stays a dumb resource cap, root-causing recurring patterns is Pulse's job, done after the fact with the full transcript, not a live control's) | unassigned | `filed` — root cause identified (redundant re-verification of an already-successful tool call); fix is a guidance addition to `stores.md` stating the tool result is authoritative, not attempted in this pass | `agent_go/cmd/server/guidance/templates/system/stores.md` |
| [PLAT-169](pulse_platform/plat-169.md) | `ToolSelectionSection.tsx`'s server checkbox compares `selectedServers` by exact string, so a manifest saved under a legacy MCP server spelling (`google_sheets`) renders the current spelling's checkbox (`google-sheets`) unchecked; toggling it appends the new spelling instead of replacing the old, and the backend's own hyphen/underscore duplicate validator then permanently blocks every future save of that workflow until the JSON is hand-edited | unassigned | `implemented` — alias-aware checkbox comparisons plus save-time self-heal (`dedupeServerNames`); build/test verified, live reverify pending | `frontend/src/components/ToolSelectionSection.tsx`, `frontend/src/components/ModePresetBar.tsx`, `frontend/src/utils/mcpServerAlias.ts` |
| [PLAT-170](pulse_platform/plat-170.md) | Product presentation tools validated files through canonical `_users/<id>/...` paths, then reused that server-internal path for `ui_presentations`; the user-scoped workspace DB API correctly rejected it, so valid videos, characters, and documents disappeared behind a generic tool-failure envelope | Codex | `implemented and deployed; direct tool retry pending` — shared path conversion, focused tests, production DB-path probe, and healthy rootless release complete | `agent_go/pkg/presentations/presentations.go` |
| [PLAT-171](pulse_platform/plat-171.md) | A chat session's own dying terminal cancels *any* work registered under its session id, including a `run_full_workflow`-launched group execution's tracked steps that are independently running and share nothing with the dead terminal but that one shared field — `BackgroundAgentRegistry.CancelAll`/`cancelTrackedExecutionsForSession` select by session-id equality with no owner-vs-watcher distinction. Live-observed: an unrelated "Rakesh Yadav" group run's Verification step was marked canceled and its LLM turn context-canceled mid-flight when the launching chat session's tmux pane died for an unconnected reason | unassigned | `open` — root cause fully traced to file:line; fix needs a design decision (separate ownership id vs. an explicit "detached launch" marker) before implementation | `cmd/server/session_lifecycle.go`, `cmd/server/background_agents.go`, `cmd/server/workflow_execution_tracker.go`, `cmd/server/delegation.go` |
| [PLAT-173](pulse_platform/plat-173.md) | The KB half of the merged reflection turn carried none of the anti-append discipline the learnings half states five different ways, so steps appended a fresh dated section every cycle instead of correcting the existing one — and `stores.md` compounded it by promising an automatic "notes compact themselves past 20KB / 30 sections" that has never been implemented anywhere, giving steps a positive reason to keep appending. Live: confida-login's `app-structure.md` grew past every stated threshold, and `page-agreement-workflows.md` accumulated an unreconciled contradiction (a 2026-07-19 "confirmed broken" entry sitting alongside later entries showing the same path working) | unassigned | `fixed` — KB-half guidance + corrected `stores.md`, fail-before/pass-after test; live reverify pending on a fresh survey cycle | `pkg/orchestrator/agents/workflow/step_based_workflow/reflection_turn.go`, `cmd/server/guidance/templates/system/stores.md` |
| [PLAT-174](pulse_platform/plat-174.md) | `step_config.json` was read exactly once per run (controller.go's `populateRuntimeFields` sweep at run start), so an `update_step_config` change made after the run started never reached a step that hadn't begun yet, silently, for every field, on every step — no error told the caller it didn't apply. Live: confida-login pinned `execute-browser-and-capture-apis` to pi-cli/gemini-3.7-flash 16 minutes before that step began; it ran three times in that run and used claude-code/claude-sonnet-5 every time, because "before this step started" was the wrong boundary — "before the run started" was the one that mattered, and the run's first step had already begun 15 minutes before the change | unassigned | `fixed` — `executeSingleStep` now refreshes a step's runtime config from disk immediately before dispatch, fail-open on a read error; fail-before/pass-after test against a real workspace-API stand-in; live reverify pending | `pkg/orchestrator/agents/workflow/step_based_workflow/controller_execution.go`, `controller.go` |
| [PLAT-175](pulse_platform/plat-175.md) | Commit `a960df20` correctly blocked raw `db.sqlite` filesystem access for message_sequence steps, but dropped the whole `db/` grant to do it — taking `db/assets/` down as collateral, the one durable location a step can write an arbitrary file to (`mutate_workflow_db` is SQL-only). A code comment mislabeled this "PLAT-169 follow-up"; PLAT-169 is a real, unrelated ticket (MCP server checkbox dedup) — corrected in code and here. Live: confida-login's `survey-app-and-refresh-knowledge` step is instructed every cycle to sync `db/assets/business-context/` via shell (compare `.source_sha`, add/remove/overwrite files, rewrite the manifest) and had no legal path to do so — its own reflection turn flagged it, silently, since nothing upstream had changed on the runs it was checked | unassigned | `fixed` — `setupMessageSequenceFolderGuard` now grants `db/assets/` specifically (a sibling of db.sqlite, not a child, so it doesn't reopen the Landlock conflict) to both read and write paths; existing regression test narrowed from a blanket `/db` substring check to specifically `db.sqlite`; new fail-before/pass-after test; live reverify pending | `pkg/orchestrator/agents/workflow/step_based_workflow/controller_message_sequence.go` |
| [PLAT-176](pulse_platform/plat-176.md) | Execution evidence is named from `retryAttempt`/`loopIteration`, both counters LOCAL to one dispatch of a step — both restart at 1 on every dispatch — so a re-dispatched step recomputes the identical path and `WriteWorkspaceFile` overwrites the previous dispatch's result, conversation, timing, and prompts in place. Live: confida-login's `execute-browser-and-capture-apis` was dispatched 5 times in one run and every dispatch clobbered the last; the same timing file read minutes apart returned two different executions (2675000ms/21 tool calls, then 104248ms/14 tool calls), and a diagnosis built on the stale read was wrong. A looping run therefore erases the evidence that it looped — defeating the after-the-fact review PLAT-168 makes the platform depend on, since the loop is already invisible to every live control (longest turn 18.5min, all steps COMPLETED, re-dispatches are not retries) | unassigned | `fixed` — prior dispatch's four evidence files are moved into a stamped `superseded/` subfolder before the write; canonical names still hold the newest dispatch so no reader changes behavior, and a subfolder stays off the `execution-attempt-*` globs readers Sscanf; best-effort, never fails a step; fail-before/pass-after tests; live reverify pending | `pkg/orchestrator/agents/workflow/step_based_workflow/controller_execution.go` |
| [PLAT-162](pulse_platform/plat-162.md) | HTTP-backed workflow tools published schemas through direct definitions/get_api_spec but executed unchecked argument maps, so guessed plan-tool fields reached handlers and caused repeated retry storms | Codex | `implemented` — canonical registration-boundary validation and corrected plan schemas; restart/runtime reverify pending | `mcpagent/agent`, AgentWorks plan-tool registration |
| [PLAT-163](pulse_platform/plat-163.md) | Technical and strategic reviews need durable route-aware focus rotation, one canonical technical identity, and visible coverage history | Codex | `implemented` — canonical technical migration, both focus catalogs, agentic route-aware focus counts, slash/scheduled parity, and visible selected/next-focus UI shipped; live Pulse verification remains | Pulse module registry/migrations, Gate, reviewer/Fixer contracts, focus persistence, and Pulse UI |
| [PLAT-165](pulse_platform/plat-165.md) | The Schedule history UI is a filtered chat archive, so it cannot show which durable schedule occurrences ran, were missed, failed, or can resume | unassigned | `partially implemented` — UI now projects durable job/run records; unified occurrence read model/stage aggregation remains | scheduler occurrence history + workflow Schedule activity UI |

Assignment reserves the lane; it does not claim that work has started. An agent
sets its fragment to `in_progress` when it actually begins. PLAT-004, PLAT-008,
and PLAT-013 remain unassigned because they are already runtime verified.
PLAT-017 remains unassigned, but is no longer blocked on reproduction: the
2026-08-09 Upwork evidence in its fragment reproduces the leak on the current
binary. Its `pulse_review_log` half shipped with PLAT-054; the `run_metadata`
implementation boundary is still the open decision.

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
| PLAT-008 Phase cost pricing | P1 | Build-in-public, RTS Latency, Tectonicus | **core pricing runtime verified; phase/daily-to-execution reconciliation remains open** |
| PLAT-009 `get_cost_summary` run resolution | P1 | Build-in-public, Social Media, RTS Latency | **implementation repaired; runtime reverify** |
| PLAT-010 Finding identity split | P1 | RTS Latency | **implementation completed; runtime reverify** |
| PLAT-011 LLM role visibility | P2 | Build-in-public, RTS Latency | **runtime evidence positive; full UI acceptance pending** |
| PLAT-012 Changelog mutation coverage | P2 | LinkedIn, RTS Latency | **plan mutation verified; learning-tree boundary still reverify** |
| PLAT-013 Legacy regular-step editing | P1 | RTS Latency | **runtime verified 2026-08-04** |
| PLAT-014 Reviewer reference loading | P1 | RTS Latency, Tectonicus | **RTS runtime verified; Tectonicus reverify remains** |
| PLAT-015 Evaluation skipped-sentinel handling | P1 | Social Media | **implementation fixed; runtime reverify** |
| PLAT-016 Evaluation report drops real zero scores | P1 | Social Media | **implementation fixed; runtime reverify** |
| PLAT-017 Scheduler success leaves workflow metadata running | P1 | Social Media, Upwork | **`pulse_review_log` boundary repaired 2026-08-15: scheduler now requires terminal typed receipts for every due module before Review+Fix completes; this prevents completed reviews being falsely relabelled interrupted at restart. The independent `run_metadata` half remains open** |
| PLAT-018 Pulse finalizer cannot record dashboard completion | P1 | RTS Latency | **implementation fixed; runtime reverify** |
| PLAT-019 Pulse agent metrics remain unpriced | P1 | RTS Latency | **implementation fixed; runtime reverify** |
| PLAT-020 Converted scheduled chat must retain its session/tmux | P0 | RTS Latency | **August 15 reconciliation regression fixed; UI/runtime reverify** |
| PLAT-021 Proposals masquerade as pending user decisions | P1 | RTS Latency | **implementation fixed; UI reverify** |
| PLAT-022 `get_api_spec` absent from one workflow-phase session | P1 | Job Search | **assigned to Claude Code** |
| PLAT-023 Diff patch context recovery on large files | P1 | Tool-error logs | **implemented 2026-08-04 (large-file fixtures + corrected-retry round trip); nearest-candidate counting is accurate and tied near-matches refuse an arbitrary hint; runtime reverify remains** |
| PLAT-024 Tool-error marker omits tool name | P2 | Cross-workflow logs | **implemented 2026-08-04 (mcpagent d1eca1f + Codex follow-up); identity is recovered before per-tool failure classification, with a narrow envelope fallback and explicit "unknown"; runtime reverify remains** |
| PLAT-025 Workspace shell stdout buffer is unbounded | P1 | Platform availability | **queued for Claude Code** |
| PLAT-026 Selected running workflow hidden from global activity | P1 | RTS Latency | **implemented 2026-08-04 (a first pass missed the same-workflow-sibling case per Codex review; corrected — see ticket); runtime reverify remains** |
| PLAT-027 Async todo-task turn falsely completes its parent and hides the live child | P0 | Social Media | **completion gate runtime verified; placeholder 404 fixed and tested 2026-08-04; rebuilt UI reverify remains** |
| PLAT-028 Recovered CDP tab forwarded as action argument | P1 | Social Media | **implemented and executor-tested 2026-08-04; runtime reverify remains** |
| PLAT-029 Missing tmux remains live and reconnects forever | P0 | Social Media | **implemented and regression-tested 2026-08-04; rebuilt runtime reverify remains** |
| PLAT-031 Cost ledger loses run identity across UTC midnight | P1 | RTS Latency, Hetzner SSH | **execution-keyed cost/evaluation ledger implemented 2026-08-06: date/group files are shards only; immutable execution/evaluation IDs own their records, rotation updates only archived path metadata, and server projections preserve that identity. Historical folder-only rows remain explicitly legacy; runtime reverify remains** |
| PLAT-032 Child-agent calls omitted from parent telemetry | P1 | Social Media | **root cause fixed 2026-08-05 (`mcp-agent-builder-go` cdc3d1a76): async sub-agent dispatch now propagates the parent step's timing-capture ID to the child context; parent/child/total breakdown and failed-child/E2E tests not built; the separate failed-child status claim remains unreproduced; runtime reverify remains** |
| PLAT-033 Managed changelog contains placeholder refs | P1 | Social Media, Tectonicus, Hetzner SSH | **partially implemented 2026-08-05: truthful target/snapshot refs for the two reproduced writers, plus deterministic value-free `workflow.json` field-path evidence for `write_workflow_manifest`; Tectonicus still shows broad coverage/provenance gaps across managed mutation history, so the complete caller audit remains open and runtime reverify is pending** |
| PLAT-034 Completed Raw tmux terminal loses scrollback | P1 | Social Media / Electron | **fixed and runtime verified 2026-08-05 (`mcp-agent-builder-go` b984e6c5c); retained stream survives completion and remains scrollable** |
| PLAT-035 Retained tmux follow-up stays globally busy after returning to its prompt | P0 | Social Media, LinkedIn | **stream-driven fix implemented and regression-tested 2026-08-05; runtime reverify after rebuild remains** |
| PLAT-036 Context-usage percentage always saturates | P2 | Tectonicus | **implemented 2026-08-05; coding-CLI aggregate usage now marks the percentage unavailable; runtime reverify remains** |
| PLAT-037 Learning freshness misattributes out-of-band writes | P2 | Tectonicus | **new; freshness ledger assigns edits to the next step rather than the writer** |
| PLAT-038 Pre-validation attempts overwrite each other | P1 | Cross-mode validation | **implemented 2026-08-05; every new validation invocation retains a run-local attempt artifact while the compatibility latest pointer remains** |
| PLAT-039 Advisor recommendations default to engineering repairs | P1 | Hetzner SSH | **implemented 2026-08-06; route persistence, backend validation, lifecycle compatibility, and legacy UI fallback complete; runtime reverify remains** |
| PLAT-040 Full schedule silently blocked by chat/Pulse | P1 | Hetzner SSH | **implemented 2026-08-06; manual Pulse and producing schedules now use separate durable lanes, only a real full workflow blocks another schedule, and trigger errors are visible; runtime reverify remains** |
| PLAT-041 Expected cron occurrence can disappear without a decision | P0 | Confida QA | **implemented 2026-08-07; occurrence identity, restart cursor, gap classification, and focused tests complete; runtime reverify remains** |
| PLAT-042 Human answers lack provable actor attribution | P1 | Confida QA | **implemented 2026-08-07; server-derived actor metadata and append-only events complete; runtime reverify remains** |
| PLAT-043 DB integrity PRAGMAs blocked by read policy | P2 | Confida QA | **implemented 2026-08-07; bounded allowlist and rejection tests complete; runtime reverify remains** |
| PLAT-044 Normal finalizer ownership filed as a platform defect | P1 | Upwork | **implemented; stable reason-code reconciliation and runtime reverify remain** |
| PLAT-045 Resolved finding leaves linked decision open | P1 | Upwork | **implemented transactionally; runtime reverify remains** |
| PLAT-046 Empty reviewer completion verdict | P2 | Upwork | **storage already rejected it; typed tool boundary hardened and tested; runtime reverify remains** |
| PLAT-047 Grouped run artifacts lack immutable physical identity | P1 | Upwork | **open design item; distinct from the completed PLAT-031 cost identity work** |
| PLAT-048 Scheduled/main tmux processes outlive their useful session | P0 | Local backend process audit | **implemented; only user-interactive main agents retain tmux, with a one-hour ceiling and live cleanup; runtime reverify remains** |
| PLAT-049 Shared platform mechanics leak into workflow artifacts | P1 | Upwork + cross-workflow scan | **Builder guard, Engineering Review audit, and idempotent workflow v1.0.21 purification preflight implemented; migration/runtime reverify remains** |
| PLAT-050 Pulse reasoning split between agent and Go | P1 | Upwork recovery-Fixer investigation | **implemented; one continuing main-agent conversation and event-driven ordering need runtime reverify** |
| PLAT-051 Upgrade prompt names an unregistered tool | P0 | Upwork | **implemented 2026-08-07: `set_workflow_contract_version` replaces the internal-helper instruction; restart and one v1.0.20 upgrade reverify remain** |
| PLAT-052 Scheduled turns visibly close/reopen Claude Code | P1 | Upwork | **implemented 2026-08-07: known consecutive scheduler turns retain their native CLI; restart and schedule lifecycle reverify remain** |
| PLAT-054 Idle watchdog kills live child work | P0 | Social Media, Tectonicus, Upwork, LinkedIn, Instagram, RTS Latency, Substack, Build-in-public | **implemented 2026-08-09: both wait paths consult a shared liveness predicate before expiring a turn, bounded by a 3 h ceiling; `pulse_review_log` added to the startup sweep. Runtime reverify is the gate for the ~40 other reverify entries** |
| PLAT-055 Reflection turn can only write learnings | P1 | Social Media, RTS Latency, Upwork, LinkedIn | **implemented 2026-08-09: KB and learnings merged into one reflection turn (regular and message_sequence paths) with `knowledgebase/notes` write, `db/` read and a structured `record_run_concern`; prompt now carries a routing rule, the workflow's real table names, per-step file ownership, a measured size signal and a compaction rule. Behavioural reverify required** |
| PLAT-056 Repair-loop recorder files durable concerns for self-healed iterations | P2 | Instagram (11 findings, one root cause) | **open — found via `scripts/pulse_health.py`; Pulse already correctly dispositioned all 11 as not-a-defect on 2026-08-04, only the recorder fix and closure remain** |
| PLAT-059 A learnings lock could be set with no stated reason | P2 | LinkedIn (6 of 6 steps locked, none justified) | **implemented 2026-08-09: `lock_learnings=true` requires `lock_learnings_reason`; unlocking never does; clearing the lock clears the reason. UI lock toggle removed. Steers no-contribution steps to `learnings_access="read"` instead** |
| PLAT-060 Ops config changes carried no reason into the config | P2 | cross-workflow | **implemented 2026-08-09: `execution_tier` / `execution_llm` / `declared_execution_mode` each require a paired reason at write time, each rejection naming its hidden consequence AND `create_human_input_request` as the escape hatch so the field cannot induce confabulation. UI tier setter removed. Objectives deliberately NOT gated — they get a yield loop instead. Reverify deferred: llm_ops_review is disabled at the Gate** |
| PLAT-062 Scripted prompt named a forbidden write target | P2 | hetznerssh | **implemented 2026-08-09: the MODE NOTE told the agent to save to `learnings/{step-id}/main.py`, which `setupExecutionFolderGuard` never opens for writes, while the same prompt's Code Execution section correctly said `code/main.py`. The step obeyed the wrong one, was denied, and filed a concern that persistence had failed — it had not, the platform saved it back 42 s later byte-identical. Surfaced only because PLAT-061 removed the `learn_code_max_fix_iterations: 0` artifact that had stopped these steps ever attempting a repair** |
| PLAT-065 Gate recorded a due module, session ended, nothing resolved it | P1 | social-media (new P0 dropped 6+h); 4 more found back to July 25 | **open 2026-08-10 — a Gate pass correctly found a real P0 (two graders disagree on public content) and recorded workflow_review=due; 42s later `abortIfTurnStillBusy` fired because the step's outcome wasn't `completed` and the session still read busy, so it stamped backup/publish/notify all `failed` with the same reason and returned — before the existing durable-worklist recovery logic (built for exactly this case) ever ran, since the busy-check is ordered ahead of it. Confirmed proximate cause via `pulse_final_command_state` rows, all three timestamped identically. Same-night cron run on upwork completed normally, so not universal. Still open: why `sessionIsBusy` read true 42s after Gate's tool call had already succeeded — needs a diagnostic log at the call site, not yet added; log window for this incident has rotated out. Fix designed (recover-before-abort reorder) but not shipped — needs a safety call on whether sending the next turn immediately once busy clears risks a real overlap, vs. needing a bounded wait. Detection shipped: `scripts/pulse_health.py --section stranded` found this instance plus 4 pre-existing ones, oldest 374h** |
| PLAT-064 Dead workflow_* event family; one completion check had no fallback | P3 | frontend-wide | **implemented 2026-08-09: tracing PLAT-063's dead entries found workflow_start/workflow_progress/workflow_end/batch_execution_end are ALL dead — orchestrator_end is the only real completion signal. checkTabCompletion (useChatStore + useWorkflowStore) had no orchestrator_end fallback for workflow mode, so a workflow completing without human feedback could not satisfy it — currently dead code itself, fixed as a landmine. EVENT_TYPES.COMPLETION trimmed to the one real signal. Display components and generated types deliberately left inert rather than deleted** |
| PLAT-066 route_selections correctly supplied, never seeded; router silently defaulted to the live-action route | P1 | social-media (Weekly Strategy Discovery run) | **open 2026-08-10 — proven with hard evidence: the schedule's `run_full_workflow` payload correctly carried `route_selections={"step-run-mode-router":"propose_new"}` (confirmed from the raw HTTP payload in the server log), yet `step-run-mode-router`'s own routing-evaluation.json says `"No route_selection.json found; using default_route_id \"execution\""`. `seedRouteSelectionsForRun`'s own success log line never fired for this session — the write path did not complete, exact reason not yet isolated (not called vs. early-return vs. write error vs. a path mismatch between the seed and read call sites). Consequence: `execute-allocate`/`step-execution-pipeline` (the live-action route) ran and failed prevalidation on an unrelated pre-existing bug, while the agent separately drove `propose-new-strategies-direct` by hand to still satisfy the schedule — leaving the wrongly-triggered execution job orphaned (`list_executions` still shows it `running` under a `cancelled` parent; `stop_step` and a full-cancel both gave contradictory answers about it, a related but separately unresolved bookkeeping defect). Interim mitigation shipped this session (commit `eaa089b`): removed `default_route_id` from `step-run-mode-router` in plan.json, so a future seeding gap fails loudly instead of silently running the live-action route — the seeding bug itself is still open** |
| PLAT-067 Scheduled parent continues after its coding-agent terminal disappears | P0 | RTS Latency cron run, 2026-08-10 | **implemented 2026-08-10 — the defect was a single swallowed error at `server.go:5850`, not a missing recovery system: `StartAgentTransportSession` already performs the verify-and-single-replacement (relaunch with `--resume`, wait for a verified ready prompt), and the 5-minute timeout was it correctly concluding the transport was unusable — but its error was logged and stepped over, so `StreamWithEvents` fired 57 lines later into a dead pane. Two consecutive ~32-minute turns burned on one run, killing every producing step after them. Now aborts with a named `parent_transport_unavailable` error using the same `sendError(..., true)` + `return` the same function already uses for a streaming failure, leaving any queued child completion intact so recovery never re-runs a successful child. Still open: why the parent tmux server disappeared at all.** |
| PLAT-063 Report pane flashed during runs | P3 | reported on the live UI | **fixed 2026-08-09 and extended 2026-08-11/12: view flips no longer unmount the viewer; ordinary renders no longer reapply iframe `srcDoc`; pending-decision background polls are visually silent and retain unchanged state; and the self-triggering scroll-repair loop captured live in browser logs was removed. The iframe now reloads only for a new document or explicit Refresh. Also records that `workflow_end` and `batch_execution_end` are dead entries in the frontend COMPLETION list** |
| PLAT-061 step_config carried a dead field and an incomplete merge | P2 | 10 of 12 workflows carried `db_access` | **implemented 2026-08-09: `db_access` removed (ignored by `resolveDBAccess`, yet documented and agent-settable — the `global_skill_objective` shape); `MergeAgentConfigFields` completed from 19/28 fields, three of the nine dropped ones gated writes; reflection-based guard added. 4 orphan fields deleted (incl. `disable_tier_optimization`, a loophole around PLAT-060, and `learn_code_max_fix_iterations`, whose every stored value was a migration artifact that silently disabled script repair); phantom clear-names now acknowledged no-ops rather than silent successes. `execution_max_turns` was misclassified — it carries the workflow-wide setting — and stays** |
| PLAT-058 Per-step learnings files fragmented one workflow skill | P1 | Social Media (5 forked files, first live PLAT-055 run) | **implemented 2026-08-09: supersedes PLAT-055 item D. Learnings are one topic-organised skill every step improves; step-named orphans are folded and deleted; frontmatter description kept accurate. Found by the steps themselves via `record_run_concern`, one classifying it `harness_issue`** |
| PLAT-057 harness_issue parkable in the workflow engineering queue | P2 | Upwork (2 of 48 harness findings) | **implemented 2026-08-09: `queued_for_engineering` is rejected for a `harness_issue` at disposition time, naming both exits; `issue_kind` promoted to shared constants; 4 regression tests incl. the untouched normal path. The 2 pre-existing rows are reported by the coherence check, not migrated** |

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
| [PLAT-034](pulse_platform/plat-034.md) | [PLAT-035](pulse_platform/plat-035.md) |  |  |
| [PLAT-036](pulse_platform/plat-036.md) | [PLAT-037](pulse_platform/plat-037.md) | [PLAT-038](pulse_platform/plat-038.md) | [PLAT-039](pulse_platform/plat-039.md) |
| [PLAT-040](pulse_platform/plat-040.md) | [PLAT-041](pulse_platform/plat-041.md) | [PLAT-042](pulse_platform/plat-042.md) | [PLAT-043](pulse_platform/plat-043.md) |
| [PLAT-044](pulse_platform/plat-044.md) | [PLAT-045](pulse_platform/plat-045.md) | [PLAT-046](pulse_platform/plat-046.md) | [PLAT-047](pulse_platform/plat-047.md) |
| [PLAT-048](pulse_platform/plat-048.md) | [PLAT-049](pulse_platform/plat-049.md) |  |  |
| [PLAT-050](pulse_platform/plat-050.md) | [PLAT-051](pulse_platform/plat-051.md) | [PLAT-052](pulse_platform/plat-052.md) | [PLAT-053](pulse_platform/plat-053.md) |
| [PLAT-054](pulse_platform/plat-054.md) | [PLAT-055](pulse_platform/plat-055.md) | [PLAT-056](pulse_platform/plat-056.md) | [PLAT-057](pulse_platform/plat-057.md) |
| [PLAT-058](pulse_platform/plat-058.md) | [PLAT-059](pulse_platform/plat-059.md) | [PLAT-060](pulse_platform/plat-060.md) | [PLAT-061](pulse_platform/plat-061.md) |
| [PLAT-062](pulse_platform/plat-062.md) | [PLAT-063](pulse_platform/plat-063.md) | [PLAT-064](pulse_platform/plat-064.md) | [PLAT-065](pulse_platform/plat-065.md) |
| [PLAT-066](pulse_platform/plat-066.md) | [PLAT-067](pulse_platform/plat-067.md) | [PLAT-068](pulse_platform/plat-068.md) | [PLAT-069](pulse_platform/plat-069.md) |
| [PLAT-070](pulse_platform/plat-070.md) | [PLAT-071](pulse_platform/plat-071.md) | [PLAT-072](pulse_platform/plat-072.md) | [PLAT-073](pulse_platform/plat-073-remaining-board.md) |
| [PLAT-074](pulse_platform/plat-074.md) |  |  |  |
| [PLAT-075](pulse_platform/plat-075.md) | [PLAT-076](pulse_platform/plat-076.md) | [PLAT-077](pulse_platform/plat-077.md) | [PLAT-078](pulse_platform/plat-078.md) |
| [PLAT-080](pulse_platform/plat-080.md) | [PLAT-081](pulse_platform/plat-081.md) | [PLAT-082](pulse_platform/plat-082.md) | [PLAT-083](pulse_platform/plat-083.md) |
| [PLAT-084](pulse_platform/plat-084.md) | [PLAT-085](pulse_platform/plat-085.md) |  |  |
| [PLAT-086](pulse_platform/plat-086.md) |  |  |  |
| [PLAT-087](pulse_platform/plat-087.md) | [PLAT-088](pulse_platform/plat-088.md) | [PLAT-089](pulse_platform/plat-089.md) | [PLAT-090](pulse_platform/plat-090.md) |
| [PLAT-091](pulse_platform/plat-091.md) | [PLAT-092](pulse_platform/plat-092.md) | [PLAT-093](pulse_platform/plat-093.md) | [PLAT-094](pulse_platform/plat-094.md) |
| [PLAT-095](pulse_platform/plat-095.md) | [PLAT-096](pulse_platform/plat-096.md) | [PLAT-097](pulse_platform/plat-097.md) | [PLAT-098](pulse_platform/plat-098.md) |
| [PLAT-099](pulse_platform/plat-099.md) | [PLAT-100](pulse_platform/plat-100.md) | [PLAT-101](pulse_platform/plat-101.md) |  |
| [PLAT-102](pulse_platform/plat-102.md) | [PLAT-103](pulse_platform/plat-103.md) | [PLAT-104](pulse_platform/plat-104.md) |  |
| [PLAT-105](pulse_platform/plat-105.md) | [PLAT-106](pulse_platform/plat-106.md) | [PLAT-107](pulse_platform/plat-107.md) | [PLAT-108](pulse_platform/plat-108.md) |
| [PLAT-109](pulse_platform/plat-109.md) | [PLAT-110](pulse_platform/plat-110.md) | [PLAT-111](pulse_platform/plat-111.md) |  |
| [PLAT-112](pulse_platform/plat-112.md) | [PLAT-113](pulse_platform/plat-113.md) | [PLAT-114](pulse_platform/plat-114.md) | [PLAT-115](pulse_platform/plat-115.md) |
| [PLAT-116](pulse_platform/plat-116.md) | [PLAT-117](pulse_platform/plat-117.md) | [PLAT-118](pulse_platform/plat-118.md) | [PLAT-119](pulse_platform/plat-119.md) |
| [PLAT-120](pulse_platform/plat-120.md) | [PLAT-121](pulse_platform/plat-121.md) | [PLAT-122](pulse_platform/plat-122.md) | [PLAT-123](pulse_platform/plat-123.md) |
| [PLAT-124](pulse_platform/plat-124.md) | [PLAT-125](pulse_platform/plat-125.md) | [PLAT-126](pulse_platform/plat-126.md) | [PLAT-127](pulse_platform/plat-127.md) |
| [PLAT-128](pulse_platform/plat-128.md) | [PLAT-129](pulse_platform/plat-129.md) | [PLAT-130](pulse_platform/plat-130.md) | [PLAT-131](pulse_platform/plat-131.md) |
| [PLAT-132](pulse_platform/plat-132.md) | [PLAT-133](pulse_platform/plat-133.md) |  |  |
| [PLAT-134](pulse_platform/plat-134.md) | [PLAT-135](pulse_platform/plat-135.md) | [PLAT-136](pulse_platform/plat-136.md) | [PLAT-137](pulse_platform/plat-137.md) |
| [PLAT-138](pulse_platform/plat-138.md) | [PLAT-139](pulse_platform/plat-139.md) | [PLAT-140](pulse_platform/plat-140.md) | [PLAT-141](pulse_platform/plat-141.md) |
| [PLAT-142](pulse_platform/plat-142.md) | [PLAT-143](pulse_platform/plat-143.md) | [PLAT-144](pulse_platform/plat-144.md) | [PLAT-145](pulse_platform/plat-145.md) |
| [PLAT-146](pulse_platform/plat-146.md) | [PLAT-147](pulse_platform/plat-147.md) | [PLAT-148](pulse_platform/plat-148.md) | [PLAT-149](pulse_platform/plat-149.md) |
| [PLAT-150](pulse_platform/plat-150.md) | [PLAT-151](pulse_platform/plat-151.md) | [PLAT-152](pulse_platform/plat-152.md) | [PLAT-153](pulse_platform/plat-153.md) |
| [PLAT-154](pulse_platform/plat-154.md) | [PLAT-155](pulse_platform/plat-155.md) | [PLAT-156](pulse_platform/plat-156.md) | [PLAT-157](pulse_platform/plat-157.md) |
| [PLAT-158](pulse_platform/plat-158.md) | [PLAT-159](pulse_platform/plat-159.md) | [PLAT-160](pulse_platform/plat-160.md) | [PLAT-161](pulse_platform/plat-161.md) |
| [PLAT-164](pulse_platform/plat-164.md) | [PLAT-166](pulse_platform/plat-166.md) | [PLAT-167](pulse_platform/plat-167.md) | [PLAT-168](pulse_platform/plat-168.md) |
| [PLAT-162](pulse_platform/plat-162.md) | [PLAT-163](pulse_platform/plat-163.md) | [PLAT-165](pulse_platform/plat-165.md) | [PLAT-169](pulse_platform/plat-169.md) |
| [PLAT-170](pulse_platform/plat-170.md) | [PLAT-171](pulse_platform/plat-171.md) | [PLAT-172](pulse_platform/plat-172.md) | [PLAT-173](pulse_platform/plat-173.md) |
| [PLAT-174](pulse_platform/plat-174.md) | [PLAT-175](pulse_platform/plat-175.md) | [PLAT-176](pulse_platform/plat-176.md) |  |

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
   PLAT-017 now has a current-binary Tectonic reproduction and can proceed to
   durable terminal-event/reconciliation design.
10. Repair/query-test the remaining P1 observability and media defects.
11. Address the P2 configuration and changelog completeness gaps.
12. Reverify PLAT-027 and PLAT-028 together on the next Social Media CDP run:
    the asynchronous child must stay visible and its browser action must not
    forward the tab ID as an element selector.

The first four are ordered ahead of cost and UI completeness because they can
change what a workflow does while still allowing it to report success.
