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
| [PLAT-090](pulse_platform/plat-090.md) | No surface reports Pulse time/cost against workflow time/cost, so "is Pulse worth it?" cannot be answered | unassigned | `open` (designed, not built) | Pulse measurement surface, cost ledger read path |
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
| [PLAT-105](pulse_platform/plat-105.md) | mcpagent already implements transport-neutral delivery, but only through a live `*Agent`; that object is deleted at streaming-loop end while the provider tmux stays live, so AgentWorks re-implements provider routing. Retained direct turns also lost structured tool receipts. The main wrapper now owns a durable Session across turns, the acknowledgement reports the actual transport, retained tool receipts do not duplicate ordinary turns, and IC-11 is enforced by the live P0 runner; cold-restart rehydration and the first authenticated per-provider IC-11 run remain | Codex | `in_progress` (warm main-chat path and P0 harness implemented; live provider evidence and cold-restart migration pending) | mcpagent session lifetime + delivery acknowledgement + retained-turn ownership |

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
| PLAT-017 Scheduler success leaves workflow metadata running | P1 | Social Media, Upwork | **reproduced 2026-08-09: a second projection (`pulse_review_log`) leaked 3 stranded `running` rows across 3 Upwork runs in one day; startup sweep extended under PLAT-054. The `run_metadata` half remains open** |
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
| [PLAT-070](pulse_platform/plat-070.md) | [PLAT-071](pulse_platform/plat-071.md) | [PLAT-072](pulse_platform/plat-072.md) | [PLAT-074](pulse_platform/plat-074.md) |
| [PLAT-075](pulse_platform/plat-075.md) | [PLAT-076](pulse_platform/plat-076.md) | [PLAT-077](pulse_platform/plat-077.md) | [PLAT-078](pulse_platform/plat-078.md) |
| [PLAT-080](pulse_platform/plat-080.md) | [PLAT-081](pulse_platform/plat-081.md) | [PLAT-082](pulse_platform/plat-082.md) | [PLAT-083](pulse_platform/plat-083.md) |
| [PLAT-084](pulse_platform/plat-084.md) | [PLAT-085](pulse_platform/plat-085.md) |  |  |
| [PLAT-086](pulse_platform/plat-086.md) |  |  |  |
| [PLAT-087](pulse_platform/plat-087.md) | [PLAT-088](pulse_platform/plat-088.md) | [PLAT-089](pulse_platform/plat-089.md) | [PLAT-090](pulse_platform/plat-090.md) |
| [PLAT-091](pulse_platform/plat-091.md) | [PLAT-092](pulse_platform/plat-092.md) | [PLAT-093](pulse_platform/plat-093.md) | [PLAT-094](pulse_platform/plat-094.md) |
| [PLAT-095](pulse_platform/plat-095.md) | [PLAT-096](pulse_platform/plat-096.md) | [PLAT-097](pulse_platform/plat-097.md) | [PLAT-098](pulse_platform/plat-098.md) |
| [PLAT-099](pulse_platform/plat-099.md) | [PLAT-100](pulse_platform/plat-100.md) | [PLAT-101](pulse_platform/plat-101.md) |  |
| [PLAT-102](pulse_platform/plat-102.md) | [PLAT-103](pulse_platform/plat-103.md) | [PLAT-104](pulse_platform/plat-104.md) |  |
| [PLAT-105](pulse_platform/plat-105.md) |  |  |  |
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
