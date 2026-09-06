[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-296 — Isolate workflow chat CLI prompts and skills across concurrent Owner and Run sessions

| Coordination | Value |
|---|---|
| Assigned agent | Unassigned (Codex code investigation) |
| Ticket state | `in progress — isolated Codex builder edits and targeted execution pass; completion notification and concurrent Owner/Run acceptance remain open` |
| Last synchronized | `2026-09-06` |

## Problem

Owner/Workshop and Run sessions can work on the same workflow, with different
prompts, tools and skills. The server selects these per session, but main
workflow chats give their CLI processes the same workflow working directory.
The CLI adapters project session-specific instructions and skills into fixed
paths in that directory. Separate conversations therefore do not guarantee
separate instruction files.

For example, an Owner session projects its instructions, then a Run session
projects different instructions into the same path. A subsequent file read,
startup or continuation can see the other session's content. Closing one
session can also remove or restore a file another session still needs.
Same-mode sessions can collide too; Owner versus Run makes the difference
in intended guidance particularly visible.

The adapter-level collision is now reproduced (see the test record below).
It is **not yet a reproduced live two-user incident or a demonstrated permission bypass**. Server-side tool grants are
separate from projected prompt text. The complete filesystem and user
isolation boundary still needs verification.

## Code evidence

- `agent_go/cmd/server/server.go`: workflow-phase chats set
  `chatWorkingFolder = workflowPhaseFolder`, derive `chatWorkingDir`, and pass
  it as `CodingAgentWorkingDir`. Read-only users are forced into Run mode.
- `agent_go/cmd/server/coding_agent_modes.go`:
  `codingAgentWorkspaceWorkingDir` resolves the workflow folder beneath the
  workspace documents root.
- `agent_go/cmd/server/workflow_phase_tools.go`: Workshop mode gates the
  available mutation tools and guidance. Guidance materialization in
  `guidance/materialize.go` attaches mode-specific content under stable skill
  identities, including `builder-reference`.
- The locally replaced `mcpagent` dependency, `agent/coding_agent_options.go`,
  uses the configured working directory unless isolated-session workspace
  mode is enabled. It requests project-instruction-only delivery, so a
  successful projection can be the CLI's sole system-instruction carrier.
- The locally replaced `multi-llm-provider-go` dependency's Codex adapter,
  `pkg/adapters/codexcli/codexcli_interactive_adapter.go`, writes `AGENTS.md`
  beneath that working directory. Its cleanup restores or removes the path
  without checking whether another session now owns its contents.
  `codexcli_skills.go` projects skills into `.agents/skills` beneath the same
  directory.
- The Claude adapter also projects project instructions and attached skills
  into the working directory. Audit each supported provider's actual paths,
  discovery rules and cleanup behavior during implementation.
- `mcpagent/agent/coding_agent_options.go` already provides an isolated
  temporary directory keyed by session ID for opted-in agents. This is a
  useful precedent, but it can fall back to the shared working directory if
  directory creation fails. It is not a complete main-chat migration.
- `mcpagent/agent/session_handle.go` prioritizes the persisted working
  directory when resolving continuations. Existing CLI sessions therefore
  need an explicit resume/migration strategy.

Dependency paths above refer to sibling repositories selected by
`agent_go/go.mod`; changes may span all three repositories.

## Implementation requirements

1. Give each chat a private runtime directory scoped to the authenticated
   user/tenant, workflow and session. Keep its identity stable across turns
   and supported server restarts. Prompt, skill and provider configuration
   writes must not affect another live session, including during mode changes.
2. Keep the workflow's plan, reports, database, outputs and learnings in the
   authoritative shared workflow workspace. Do not copy the entire workflow
   into independent per-user folders. Separate the CLI runtime directory
   from the workspace tools' workflow root.
3. Audit relative paths, shell execution, provider instruction discovery and
   skill references. Use explicit workflow-root resolution or a controlled
   mapping so moving the CLI runtime does not break workflow file access or
   accidentally discover another mode's generated instructions.
4. Make cleanup ownership-aware. Cancellation, failures and shutdown must
   remove only the terminating session's generated files. Preserve
   user-maintained project instructions.
5. Persist the runtime location needed by native CLI resume. Define migration
   for existing sessions: resume safely or start a new native conversation
   with application history preserved and an explicit handoff. Do not
   silently resume against a different workflow or mode.
6. Retain server-side authorization independently of prompt isolation. Run
   users must not acquire Owner tools or unauthorized filesystem access by
   changing a client mode value, path or prompt. Folder separation alone is
   not a permission boundary.
7. Fail with an actionable error if required isolation cannot be established;
   do not silently fall back to shared instruction files.
8. Check Codex, Claude and other supported CLI provider paths, plus main chats,
   scheduled execution and step agents. Reuse existing isolation where it
   meets this contract and avoid breaking provider session coordination.

Concurrent workflow edits remain a separate concern: verify compatibility
with [PLAT-290](plat-290.md)'s execution snapshots and retained run revisions.
Private instruction folders must not change which plan revision an active
run executes or widen workflow mutation permissions.

## Acceptance and regression checks

- Run two same-provider chats on one workflow as Owner and Run, with distinct
  prompt/skill markers. Test both startup orders, continued turns and skill
  reads. Each session consistently receives its own guidance.
- Repeat with two same-mode sessions and relevant cross-provider combinations.
- Close, cancel or fail one session while the other continues. Its instruction
  and skill files remain intact and usable.
- Switch one chat's mode while another is active; verify both content and
  actual server tool grants. Forged Run-to-Owner requests remain denied.
- Verify normal workflow file reads, permitted writes, report generation,
  database access and outputs resolve to the correct shared workspace after
  changing the runtime directory.
- Restart and resume chats, including a pre-migration session. Check native
  conversation identity, history, mode and working-directory association.
- Change a plan as Owner while a Run session executes; confirm the existing
  run-revision behavior remains correct.
- Force private-directory creation failure and assert an actionable failure
  with no shared projection fallback.
- Use a fixture workflow with no public side effects for concurrency tests.

## Delivery status

Workflow-chat isolation is implemented as the server default, with
`AGENTWORKS_ISOLATE_WORKFLOW_CLI=false` retained as an explicit rollback. The
separate-instance launcher also defaults to isolation. Both initial agent
construction and workflow-phase setup select private directories; the workspace
bridge retains the shared workflow root. Resume checks reject mismatched cwd and
Codex project-directory overrides. Read-only mode identity is pinned to Run.

The implementation is ready in mainline source; an already-running server needs
a restart before the new default applies. Full two-user authorization and the
real restart/native-resume acceptance matrix remain open, so the ticket remains
in progress.
The current policy replays application history into a fresh native conversation
when resuming into a different app chat; only the same private chat identity
retains native continuation. Runtime directory garbage collection is deferred.

## 2026-09-06 — Repeatable testing-workflow reproduction

Standard process: [Isolated workflow testing](../../getting-started/isolated-workflow-testing.md).
Runner: `scripts/test-workflow-isolation.py`; projection fixture:
`scripts/testdata/plat296_projection_test.go.tmpl`.

Used an allowlisted, sanitized copy of the existing `Workflow/testing`. Tested
real Codex and Claude prompt/skill projection functions through Go overlays,
without modifying the provider checkout or invoking any native CLI/model.
Both providers reproduced prompt, skill body, supporting-reference and cleanup
collisions with overlapping simulated Owner/Run lifetimes. Both startup orders
and both cleanup modes were exercised. Private-directory controls passed while
retaining a shared workflow plan. Source workflow input hashes were unchanged.

16 cases passed in reproduction mode (8 per provider, including controls).
The `--expect isolated --providers codexcli` negative check failed as expected
on the shared-directory cases. The existing isolated-instance launcher test
also passed using its fake runner.

Local retained evidence:
`.local/workflow-tests/plat-296-5wrfgb8_/receipt.json` and the provider logs.
The receipt records the source hashes and scope. No running AgentWorks server,
browser, workflow or native session was started/stopped/reconfigured. Zero model
calls. This does not yet verify the server's directory choice, real two-user
permissions, native continuation, restart or mode switching.

## 2026-09-06 — Opt-in implementation and offline verification

- Production selector: `agent_go/cmd/server/workflow_cli_isolation.go`, backed
  by `agent_go/pkg/cliruntime/workspace.go`. Private storage is required outside
  workspace documents and rejects symlinked managed directories and obstructions.
- Server restore tests cover matching private continuation, old/shared or
  other-chat rejection, provider/mode isolation, and both representations of
  Codex's cwd override. Claude, Pi and Cursor handle checks are exercised;
  their full native CLIs are not invoked.
- `python3 scripts/test-workflow-isolation.py --server-isolation` tests the
  production selector, then feeds its paths to the real Codex/Claude projection
  and cleanup functions. Both startup orders and cleanup modes pass, with shared
  workflow instructions/data retained. Shared-directory cases still reproduce
  the original collision, so the probe retains its negative control.
- `bash scripts/test-local-instance.sh` verifies flag forwarding with a fake
  runner. No existing server, browser, tmux pane or source workflow was changed.

Evidence: `.local/workflow-tests/plat-296-azko23d8/receipt.json`,
`server-isolation.log`, `server-runtime-directories.json`, `codexcli.log`, and
`claudecode.log`. These are local retained artifacts, not committed fixtures.

## 2026-09-06 — Live chat and shared file-read smoke check

The separate test instance at `http://127.0.0.1:52733/` was restarted with the
user's existing `/Users/mipl/.codex` login, as requested. The earlier fresh Codex
authentication directory had caused the CLI to wait at sign-in; it was a test
setup issue. No credentials were copied and the production backend remained
running as the same process.

The real Codex chat now completes a response, reads `builder-reference` and its
file-layout reference, and lists the shared testing workflow's `planning/plan.json`,
`planning/step_config.json`, `soul.md`, `workflow.json`, and conversation files.
The CLI launches in the private runtime selected before the restart, while
workspace shell commands still run in the shared testing workflow folder.

Listing the private CLI directory through the workspace shell is denied by its
workspace-root boundary. This is recorded separately, not counted as a successful
private-directory read. This smoke check does not complete concurrent Owner/Run,
cleanup, mode-change, two-user authorization, or native-resume acceptance.

Local evidence: `.local/agentworks-instances/plat296-live/live-file-access-retry.json`
and that instance's server log; chat `87999abb-c50e-4e8b-a26b-b58f3dc9ed3a`.

## 2026-09-06 — Actual builder-chat acceptance

Tested the existing Codex chat as a builder, using its normal workflow APIs and
file tools. The outer test driver only submitted chat requests and inspected
results; it did not implement the plan or Python changes for the builder.

Verified on disk:

- Read the existing plan and builder-reference skills.
- Renamed the existing step through `update_scripted_step`.
- Added `builder-write-probe` through `add_scripted_step`, including JSON output
  validation, and authored `learnings/builder-write-probe/main.py` through the
  workspace patch tool.
- Passed `validate_plan_change` with no issues.
- On a subsequent chat turn, updated both code and plan description and repaired
  the test fixture's file layout through the same normal tools.
- Executed only the new scripted step using `execute_step(fast_path_only=true)`.
  Execution `exec-builder-write-probe-1788679689541286000` finished successfully.
  Its real output at
  `runs/iteration-0/execution/builder-write-probe/builder-write-probe.json` contains
  `marker: PLAT296_BUILDER_OK`, `soul_readable: true`, and
  `shared_workflow_marker: PLAT296_SHARED_WORKFLOW`.

The first execution failed because the test fixture placed `soul.md` at the
workflow root. Execution steps are granted read access to the canonical `soul/`
folder. The builder copied the fixture content into `soul/soul.md` and corrected
its script and description; the retry passed without widening permissions.
The native CLI cwd remained the private per-chat runtime throughout these turns.
Production testing-workflow input hashes were unchanged.

**The entire automatic builder flow does not pass yet.** On both the failed and
successful step completions, the notification dispatcher cleared a supposedly
stale busy flag and attempted a synthetic turn while the retained Codex turn was
still active. `AskWithHistory` returned
`mcpagent: a turn is already in flight on this agent`. The dispatcher then logged
the synthetic turn as completed, and chat stayed at “waiting for automatic
completion.” The successful JSON was independently verified; automatic return
to the builder and readback were not credited as passing. Investigate the busy
check and notification retry handling in `background_agents.go` before claiming
end-to-end readiness. This evidence does not establish that directory isolation
caused the notification race.

Evidence: `.local/agentworks-instances/plat296-live/builder-acceptance/receipt.json`,
its `before/` and `after/` artifacts, and server log entries at 12:52:48 and
12:58:09 local time. Concurrent Owner/Run, mode-change, two-user permissions and
the full native-resume/restart matrix remain pending.

## 2026-09-06 — Fresh chat with ordinary user messages

Opened the blank Builder tab in the isolated test UI and verified a new backend
chat (`07fdef59-20d2-4e9d-8ead-fd76d88d00eb`) and private CLI runtime. The test
used only two ordinary messages, with no tool names, paths or acceptance markers:

1. “What does this workflow do right now?”
2. “Can you add a step at the end that writes a short, readable summary of what
   this workflow checks? Run that new step once and show me the summary.”

The builder independently read skills and canonical files, explained the existing
workflow, added `summarize-workflow-checks` as a message-sequence step, validated
the plan and launched it. The first execution failed because the new step lacked
read permission for `planning/plan.json`. Its failure notification reached the
builder, which added scoped `additional_read_paths`, validated and retried without
another user message. Execution `exec-summarize-workflow-checks-1788680544501472000`
succeeded. The success notification also reached the builder, which read and
displayed the actual `workflow-checks-summary.md`; the file was independently
verified on disk. Production testing-workflow input hashes were unchanged.

This fresh-chat builder flow passes after autonomous recovery. It does not erase
the earlier retained-chat notification race or establish concurrent Owner/Run,
two-user permissions, or restart/resume acceptance. Local receipt and artifact
copies: `.local/agentworks-instances/plat296-live/natural-chat-acceptance/`.

## 2026-09-06 — Concurrent Builder and Run chats

Added a `Run chat` action in the workflow composer controls. It creates an
independent workflow chat whose tab metadata pins `workshop_mode=run`; normal
Builder chats continue to use `workshop`. Submission and command discovery now
read the mode from the owning tab, so changing tabs cannot make two concurrent
chats inherit one workflow-global mode.

Live-tested both chats at the same time in the isolated testing instance:

- Run chat `5d413cc2-f374-4599-b93a-234d21606f81` received the `run` override,
  had 67 registered workflow tools, and used private runtime
  `cli-runtimes/v1/7c0acb...`.
- A Builder chat received the `workshop` override with 128 registered workflow
  tools while the Run chat was active. A second Builder conversation created
  directly from the permanent Builder launcher repeated that result as fresh
  session `926dba2c-f472-4b24-9549-c46947108154`, using private runtime
  `cli-runtimes/v1/d3e469db...`.
- Both read the same shared testing workflow while running concurrently.
- After switching away and back, the Run chat refused a requested plan type
  conversion and directed the user to Workshop. No workflow files changed.

This completes same-user concurrent Builder/Run prompt and tool separation.
Two-user authorization and restart/native-resume acceptance remain pending.

## 2026-09-06 — Scheduled Run isolation and smaller runtime surface

Normal cron and calendar schedule messages now execute with
`execution_options.workshop_mode=run`. This gives unattended work the same
constrained prompt and tool policy as an interactive Run chat and selects a
private CLI runtime directory by default, derived from the schedule session,
provider, user, workflow and Run mode.
Explicitly selected workflow business skills remain attached so the scheduled
job can perform its purpose.

The scheduler chooses authority per turn. Contract-upgrade and answered-decision
preflights use Workshop mode because those bounded turns may update workflow
artifacts. Pulse Gate, review/fix and finalization also use Workshop mode. The
request helper clones nested execution options, preventing one maintenance
turn's elevation from leaking into the normal scheduled run that follows.

Run's projected reference bundle no longer contains Workshop slash-command procedures, Pulse
repair/drift material, secret management, or backup/publish authoring guides.
Focused scheduler, tool-policy, guidance materialization and prompt tests pass.

Conversation and cost continuity were audited after introducing these per-turn
mode changes. The canonical conversation JSON continues to merge the full
pre-switch transcript with each new exchange while excluding the synthetic
conversation-file pointer used to ground the relaunched CLI. A disk fallback now
loads that prior transcript when the process-local history cache is empty, so the
overwrite guard does not preserve the old file at the expense of dropping the
new turn.

Cost attribution now uses `pulse_lifecycle_turn` as the authoritative Pulse
signal; Pulse can use the Builder model without the retired
`llm_config_source=scheduled_pulse` marker. Phase-token cumulative bookkeeping is
also keyed by native CLI session epoch. A Run→Workshop relaunch therefore records
the new process's complete first-turn usage instead of subtracting the prior
process's counters. Non-native phase turns use their query/turn id as the epoch.
Tests cover cumulative history across mode changes, disk-history recovery,
native-runtime counter resets, explicit Pulse scope, and the existing immutable
cost-ledger behavior.
