# Bug: Per-chat tool registries are never retired

## Status

Open — documented 2026-08-03. No runtime fix has been applied.

This is a cross-repository lifecycle defect. The registry implementation lives
in `mcpagent`; the production chat/session lifecycle that should terminate it
lives mainly in `mcp-agent-builder-go`.

## Simple model: a per-chat phonebook

Think of the tool registry as a per-chat phonebook.

When a chat starts, the runtime writes down:

- which custom tools that chat may call;
- the exact executor function for each tool;
- agent-specific virtual tools such as discovery helpers;
- the latest virtual-tool scope for the chat; and
- the chat's tool allow-list.

That separation is necessary. Two workflows can expose a tool with the same
name but different folder guards, workspace paths, or permissions. Looking up a
tool in the wrong chat's phonebook could execute it with the wrong authority.

The defect is not that these phonebooks exist. The defect is that completed
chats never have their phonebooks thrown away.

## User-visible and operational impact

The immediate symptom is gradual process growth rather than one deterministic
request failure:

- session maps grow for the lifetime of the backend process;
- executor closures can retain agents, workflow paths, guards, and other state;
- old allow-lists and virtual scopes remain addressable;
- registry diagnostics and logs become increasingly noisy; and
- a long-running server carries stale authorization/execution state for chats
  that no longer exist.

Map lookup remains approximately constant time, so this finding alone does not
prove that every slow API request is caused by the registry. It is a confirmed
resource/lifecycle leak and a stale-state risk, not yet a complete explanation
for general backend latency.

## Evidence

A scan of `agent_go/logs` on 2026-08-03 found:

```text
4,047  "Session-scoped custom tools registered"
  278  "Session-scoped virtual tools registered"
    0  "Cleaned up session-scoped tools"

maximum logged total_sessions: 274
```

The 4,325 registration events do **not** mean 4,325 distinct chats. Dynamic
custom-tool registration republishes the session's cumulative tool map, so one
agent can produce many registration events while its tool count rises. The
`total_sessions=274` observation is the clearer retained-phonebook count.

The code confirms the missing production lifecycle:

- `mcpagent/agent/codeexec/registry.go` defines `CleanupSession(sessionID)` and
  removes custom tools, virtual scopes, latest-scope pointers, and allow-lists.
- Repository-wide search finds calls only in tests and one stress-test command.
  No production completion path calls it.
- `mcpagent/agent/turn_session.go:Session.Close` clears only in-memory turn
  history.
- `mcpagent/agent/agent.go:Agent.Close` deliberately preserves session-level
  resources because agents are rebuilt between turns.
- `mcpagent/agent/session.go:CloseSession` closes MCP connections and removes
  the isolated workspace, but does not clean the code-execution tool registry.
- `mcpagent/agent/session.go:CloseHTTPSession` delegates to the MCP connection
  registry. That closes the mapped MCP sessions, but likewise does not retire
  their tool-registry entries.
- `mcp-agent-builder-go` already calls `CloseHTTPSession` from session stop and
  cleanup paths, so the application believes it has ended the session while one
  of its resource registries survives.

## Why the obvious fix is unsafe

Do not simply call `CleanupSession` from every `Agent.Close` or after every
turn.

An `Agent` is often replaced between turns of the same live chat. Connections
and session-scoped tools are intentionally reusable across those replacements.
Background agents and virtual-tool scopes can also share the base chat session.
Cleaning the phonebook when one temporary agent finishes could remove tools
while another turn or child agent is still using them.

Cleanup belongs to the **true session owner**, after active work has been
canceled and joined—not to a short-lived agent instance.

## Recommended long-term fix

Create one idempotent session-termination operation in `mcpagent` that owns all
session resources. Conceptually:

```text
EndSession(sessionID)
  1. prevent new work/reconnection for the session
  2. wait for or cancel active tool calls
  3. close MCP connections
  4. remove custom-tool executors
  5. remove base and :vt: virtual-tool scopes
  6. remove latest-scope pointers and allow-lists
  7. remove the isolated session workspace
```

`CloseSession` should become this complete operation rather than a connection-
only operation. `CloseHTTPSession` must enumerate every MCP session associated
with the HTTP chat and invoke the same complete termination path for each one.
The implementation should not make `mcpclient` import `codeexec`; the higher
`mcpagent` layer should coordinate both registries.

`mcp-agent-builder-go` should then call this single operation at its existing
true terminal boundaries:

- explicit chat/session clear;
- explicit stop after active work is canceled;
- terminal workflow or scheduled-run cleanup when the session will not be
  reused; and
- server shutdown for every remaining session.

If stop-and-resume intentionally reuses the same session ID, the next turn must
rebuild its registry before tool execution. That behavior needs an explicit
test; retaining stale executors is not a valid resume mechanism.

A bounded idle-session sweeper can be added as crash/leak defense, but it should
not replace deterministic lifecycle cleanup.

## Diagnostics needed before implementation

Add a read-only registry snapshot with counts only:

- custom-tool session count and executor count;
- virtual-tool base/scope count and executor count;
- allow-list session count;
- oldest/last-touched session age; and
- cleanup count by reason (`completed`, `stopped`, `cleared`, `shutdown`,
  `expired`).

These metrics make the fix measurable without exposing tool arguments,
credentials, or executor details.

## Acceptance criteria

1. Creating and ending 100 unique chats returns every session-registry count to
   its original baseline.
2. Ending chat A does not change the available tools or allow-list of concurrent
   chat B.
3. Cleanup removes `sessionID`, all `sessionID:vt:*` scopes, the latest-scope
   pointer, and the allow-list.
4. Cleanup is idempotent and race-tested against an in-flight tool call.
5. A stopped then resumed chat rebuilds the correct guarded executors before its
   first tool call.
6. Normal agent replacement between turns does not trigger session cleanup.
7. `CloseHTTPSession` and server shutdown leave zero retained tool sessions.
8. Production logs or metrics show one terminal cleanup reason per ended
   session instead of zero cleanup activity.

## Non-goals

- Do not remove session scoping; it prevents cross-workflow contamination.
- Do not merge all tools into a global executor map.
- Do not couple cleanup to chat-history deletion—the persisted conversation and
  the live executable phonebook have different lifetimes.
- Do not redesign tool categories or discovery as part of this fix.
