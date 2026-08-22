[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-177 — resumed Claude Code session claimed it lost tool access it demonstrably had; three plausible platform explanations investigated and ruled out

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — investigated, no platform root cause confirmed |
| Last synchronized | `2026-08-22` |

- **Priority:** P2 — disruptive when it happens (the agent stops using tools
  it has and asks the user to re-grant access that was never withdrawn), but
  not reproduced on demand and not traced to a concrete code defect after a
  thorough investigation.
- **Owner:** native session resume/relaunch path
  (`cmd/server/server.go`'s `seedCodingAgentRuntimeFromRestoredConversation`
  and the FIX-B materialize guard in `handleQuery`), coding-agent
  code-execution-mode plumbing (`mcpagent/agent`).
- **Related:** none — this is the first ticket investigating the native
  resume path specifically.

## Symptom

Mid-conversation on workflow "substack" (session
`b5e39872-4e4e-4645-8059-6d6e7a1231db`, Claude Code CLI provider), the agent
told the user it no longer had access to plan-editing tools
(`update_message_sequence_step`, `delete_schedule`, `add_todo_task_route`,
`create_human_input_request`, etc.) — tools it had used successfully earlier
in the same conversation — and described only having "general-purpose tools"
available. The user asked whether this correlates with a session resume;
this ticket confirms it does, but does not confirm *why* the resume causes
the agent to make this claim.

## What's confirmed

- The session underwent a native-runtime resume + relaunch at `13:47:47-50`
  on 2026-08-22 (`[claude-code] Restored native runtime from chat history
  for session b5e39872-...`), followed by the FIX-B "Materialize guard"
  block inside `handleQuery` (`cmd/server/server.go` ~6030-6107) relaunching
  the CLI process via `mcpagent.StartAgentTransportSession` →
  `executeLLMForCodingAgentTransportLaunch`
  (`mcpagent/agent/llm_generation.go:1059-1073`).
- The agent's tool-access complaint follows this relaunch. Before it (e.g.
  `09:19:00`, `09:19:21`), the same tools worked: two successful
  `update_message_sequence_step` calls logged as direct
  `[SESSION_ROUTE_DEBUG] ... url=/s/b5e39872.../tools/custom/update_message_sequence_step`
  hits.

## Three hypotheses investigated and ruled out

**1. Native MCP tool list going stale across `--resume`.** Ruled out at the
design level: this platform's CLI-bridge providers (claude-code, codex,
cursor, pi) never register these ~107 tools as a native MCP tool list in the
first place. `buildBridgeMCPConfig`
(`multi-llm-provider-go/coding_agents_bridge.go:115-193`) only ever emits a
fixed 4-tool native set (`execute_shell_command`,
`diff_patch_workspace_file`, `agent_browser`, `get_api_spec`); every other
tool is discovered via `get_api_spec(tool_name=...)` and invoked as an HTTP
call issued through `execute_shell_command`. There is no native list to go
stale.

**2. The `launchOnly` relaunch path omits the system prompt.** Ruled out by
direct read of the actual live prompt log for the exact relaunch —
`logs/agent_prompts/b5e39872-4e4e-4645-8059-6d6e7a1231db/263_13-47-50_chat-agent_code-exec_claude-code_claude-sonnet-5.md`
(99077 bytes). The system prompt is fully present (complete "Workflow
Builder Agent" instructions, plan steps, phase-detection logic) and its
`<available_tools>` block (lines 186-361) explicitly lists every tool the
agent claimed it lacked — `create_human_input_request` (line 208),
`add_todo_task_route` (line 253), `delete_schedule` (line 263),
`update_message_sequence_step` (line 309) — plus prose (line 168) teaching
the exact `get_api_spec(tool_name=...)` + HTTP invocation convention. This
also directly contradicts an initial subagent investigation report that
claimed the `launchOnly` branch sends no system prompt; re-reading
`executeLLMForCodingAgentTransportLaunch`
(`mcpagent/agent/llm_generation.go:1059-1073`) shows it explicitly appends
`WithCodingProviderLaunchSystemPrompt` before calling
`executeLLMInner(..., launchOnly=true)`, with its own comment explaining
exactly why ("Without this, launch-only sends nil messages... adapter's
splitSystemPrompt returns empty..."). The subagent had examined
`executeLLMInner` in isolation without tracing back to this caller.

**3. Resume silently drops/overrides the workflow's `use_code_execution_mode`
manifest setting, changing the tool-invocation convention mid-conversation.**
This looked promising: the substack workflow's manifest
(`workspace-docs/Workflow/substack/workflow.json`) sets
`"use_code_execution_mode": false`, and the working 09:19 call hit a direct
named route rather than routing through `execute_shell_command`, while the
13:47:50 prompt log is explicitly tagged `_code-exec` and teaches the
shell+`get_api_spec` convention. Ruled out on closer trace: `useCodeExecutionMode`
is force-set `true` for every CLI-bridge provider (claude-code, codex,
cursor, pi) at *every* agent-construction site, unconditionally, regardless
of the manifest — both at original session creation and at resume alike,
with explicit comments stating this is intentional:
  - `mcpagent/agent/agent.go:825-828,1746-1755` (`isCodingCLIBridgeProvider`) —
    "CLI providers always need code execution mode (tools accessed via HTTP
    bridge)."
  - `cmd/server/server.go:4446-4451` — the generic chat-agent construction
    path (the one actually used here, matching "chat-agent" in the log
    filename) — "Multi-agent chat / generic agent always runs in
    code-execution mode regardless of provider... Tool-search and
    simple-agent paths have been retired."
  - `cmd/server/server.go:3685-3687` — same for the workflow orchestrator
    path.
  - `pkg/orchestrator/agents/workflow/step_based_workflow/controller_agent_factory.go:741-758` —
    checks `common.IsCLIProvider` *before* honoring any per-step override.
  - `controller.go:1351-1369` — "every agent now runs in code-execution mode
    for HTTP-bridge tool routing."

  The resume path (`server.go:5962/5972/6059/7507`) operates on
  `underlyingAgent := llmAgent.GetUnderlyingAgent()`, the *same* agent object
  built earlier in the identical `handleQuery` call by the chat-agent
  construction path above — it does not re-read or drop the manifest
  setting on resume, because that setting was never read for CLI providers
  at any point, including the original 09:19 launch. `useCodeExecutionMode`
  was almost certainly already `true` at 09:19 too (only one prompt-log file
  exists for this session, tagged `_code-exec`, and the filename suffix is
  data-driven off the live flag — `mcpagent/agent/conversation.go:2255-2257`).
  The most likely explanation for the 09:19 direct route is that Claude Code
  CLI's own built-in Bash tool issued the HTTP call directly (`curl`), which
  is the code-execution convention working as intended — it just doesn't
  produce a preceding `execute_shell_command`-dispatch log line the way
  agent_go's own instrumented custom tool does, so it looked superficially
  different from the 13:47:50 traffic without actually being a different
  mode.

## What remains unexplained

All three concrete, code-level explanations for "the platform told the model
something different, or gave it less, after the resume" are ruled out by
direct evidence — the system prompt, tool catalog, and invocation convention
were identical and complete both before and after the resume. The remaining
gap is genuinely unclear and was not resolved in this pass: either (a) the
model itself made an incorrect self-assessment of its own capabilities
despite having complete, correct instructions in front of it — an
LLM-response failure rather than a platform defect — possibly triggered by
something about reconciling resumed conversation history against the
system prompt; or (b) a mechanism not yet examined in this investigation
(not ruled out, just not reached): a permission/allowlist gate at the actual
HTTP-invocation layer distinct from what the prompt describes, or something
specific to that turn's raw model response (`stream_turn-*.txt` in the same
prompt-log directory was located but not yet read).

## Non-goals

- Not claiming a confirmed root cause. This ticket documents a real symptom
  and a real resume-boundary correlation, plus three specific ruled-out
  explanations, honestly — not a fix.
- Not re-litigating whether CLI providers should honor
  `use_code_execution_mode: false` from the manifest — the code comments
  cited above are unambiguous that ignoring it for CLI providers is
  intentional (CLI providers structurally need the HTTP bridge), and nothing
  found here contradicts that design choice.

## Suggested next step (not started)

Read the raw streamed model turn for the incident (`stream_turn-*.txt` in
`logs/agent_prompts/b5e39872-4e4e-4645-8059-6d6e7a1231db/`) to see the
model's actual reasoning/response verbatim, and check whether it ever
attempted a `get_api_spec`/tool call that failed at the HTTP layer (as
opposed to declining to try at all) before concluding this is a model
self-assessment issue versus an actual invocation-layer failure.

## Verification

Investigation only — no code change in this pass. Every claim above is
backed by a direct log or source citation; no speculation presented as
fact.
