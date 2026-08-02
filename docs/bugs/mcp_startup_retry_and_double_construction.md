# Bug: MCP Startup Retries and Double Agent Construction Delay Every Message

## Status: Open — investigated 2026-08-02

Cross-repo: the nested retry defect is in `mcpagent`; the duplicate agent
construction is in `mcp-agent-builder-go`.

## Summary

A selected MCP server that cannot start delays the agent before the LLM receives
the user's message. The delay is much larger than one failed startup because the
same server is retried by two nested policies, then the complete MCP-backed agent
is constructed a second time during definition finalization.

For one broken server and the default settings, one message can start the same
server process up to 24 times:

```text
3 attempts inside Client.Connect
× 4 attempts inside Client.ConnectWithRetry
× 2 calls to NewAgentFromDefinition
= 24 process launches per server per message
```

Before MCP alias deduplication was added, selecting both `google_sheets` and
`google-sheets` doubled the process count again to 48. Alias deduplication is now
fixed; the 24-launch amplification described here remains open.

## User-visible symptom

- A scheduled Pulse turn appeared to take roughly a minute to send its next
  message.
- The LLM was not thinking during most of that time. Agent construction was
  repeatedly starting a broken Google Sheets MCP process.
- The same startup delay repeated on the next sequential Pulse message even
  though the preceding turn had already established that the server could not
  start.

The Google Sheets process had a separate upstream dependency failure
(`mcp-google-sheets` resolved MCP SDK 2 while importing the MCP 1 FastMCP path).
That failure exposed this amplification bug but is not its root cause. Any
deterministically broken stdio, SSE, or HTTP MCP server can trigger it.

## Evidence

From `agent_go/logs/server_debug.log` on 2026-08-02:

```text
18:45:47  first NewAgentConnectionWithSession starts
18:46:14  first construction finishes    duration=27.0325s
18:46:16  second construction starts
18:46:43  second construction finishes   duration=26.9409s
```

The stream did not begin until both construction passes had finished: about 55
seconds after initialization began. The next Pulse turn at 18:47:09 repeated the
same pair of approximately 26-second startup phases.

The server log also shows the nested counter pattern: the inner connection log
reaches attempt 3, then the outer policy reports attempt 1 and begins another
sequence.

## Root cause 1: two retry owners

`mcpagent/mcpclient/client.go` contains two public connection paths that both own
retry behavior:

- `Client.Connect()` performs three attempts itself.
- `Client.ConnectWithRetry()` performs `MaxRetries + 1` attempts; the default
  `MaxRetries` is 3, so it invokes `Connect()` four times.

`ConnectWithRetry()` therefore does not perform four process attempts. It
performs four groups of three attempts, for a maximum of 12 process launches.
The two layers also have independent backoff delays, making latency and logs hard
to interpret.

## Root cause 2: the wrapper creates the underlying agent twice

`agent_go/pkg/agentwrapper/llm_agent.go` constructs an MCP agent in two places:

1. `NewLLMAgentWrapperWithTrace()` calls `mcpagent.NewAgentFromDefinition()`
   while the definition is still being assembled.
2. `LLMAgentWrapper.FinalizeDefinition()` calls
   `mcpagent.NewAgentFromDefinition()` again, replaces the first agent, and
   retires it.

Creating an agent performs MCP configuration loading, connection/cache lookup,
and tool discovery. Therefore finalization repeats the entire failed startup
sequence even though no user turn has run on the first agent.

## Root cause 3: failed startups are not remembered

The session connection registry stores successful connections and can reuse
them, but a failed startup leaves no short-lived negative entry. The next
message creates a new wrapper and immediately repeats every attempt against the
same unchanged server configuration.

This is particularly harmful for deterministic errors such as a missing binary,
invalid package dependency, malformed command, or authentication configuration
that cannot change during the current turn.

## Proposed fix

### 1. Make one function own retries

- Change `Client.Connect()` into one connection attempt (or make it private as
  `connectOnce`, using the existing method of that name directly).
- Keep retry/backoff policy only in `ConnectWithRetry()`.
- Define clearly whether `MaxRetries` means retries after the first attempt or
  total attempts; expose that value consistently in logs.
- Use context-aware waits only. Do not use an uncancellable `time.Sleep` in the
  connection path.

Recommended default for conversational startup: one initial attempt plus at
most two retries. Slow-server support should come from the per-attempt timeout,
not multiplicative retry layers.

### 2. Construct the underlying agent once

- `NewLLMAgentWrapperWithTrace()` should initialize the model, runtime, and
  mutable `AgentDefinition`, but should not create the underlying MCP agent.
- `FinalizeDefinition()` should freeze the assembled definition and call
  `NewAgentFromDefinition()` exactly once.
- Audit `GetUnderlyingAgent`, `Close`, observer registration, and diagnostics so
  they handle the pre-finalization state explicitly rather than requiring a
  throwaway agent.

### 3. Add a bounded startup-failure cache

- Key failures by the canonical server name plus a fingerprint of its effective
  configuration (protocol, command/URL, args, relevant environment names, and
  runtime overrides).
- Cache deterministic startup failures for a short TTL, for example 30–60
  seconds.
- A repeated turn during the TTL should fail fast with the original error and
  the remaining retry time.
- A config change must produce a different key and retry immediately.
- Never cache context cancellation or caller deadline expiration as a server
  failure.

### 4. Do not block unrelated conversational turns on optional MCP startup

When cached tool metadata is available, register the schema and defer the live
connection until the tool is actually called. A broken optional MCP should be
reported when selected/used, but it should not prevent Pulse or the workflow
builder from answering an unrelated message.

This is an optimization after the two correctness fixes above; it must not hide
missing required tools.

## Acceptance criteria

1. A wrapper turn calls `NewAgentFromDefinition()` exactly once.
2. With `MaxRetries = 2`, a permanently failing server process is launched
   exactly three times total, not nine times.
3. `google_sheets,google-sheets` resolves to one canonical server before the
   connection path (covered separately by alias regression tests).
4. A second message within the failure-cache TTL does not launch the unchanged
   broken server again.
5. Editing the server configuration bypasses the stale failure immediately.
6. Cancellation during backoff ends the attempt promptly.
7. A scheduled Pulse E2E containing one broken optional MCP still delivers its
   next LLM message within a bounded startup budget.

## Required regression tests

- `mcpclient`: count `connectOnce` invocations under a forced permanent failure
  and assert the configured total-attempt contract.
- `mcpclient`: cancel during retry backoff and assert prompt return.
- `mcpclient/session_registry`: assert failure-cache hit, TTL expiry, config-key
  invalidation, and exclusion of cancellation errors.
- `agentwrapper`: inject/count agent construction and assert exactly one build
  after `FinalizeDefinition`, including idempotent repeated finalization.
- Builder/Pulse E2E: select a deliberately broken optional stdio MCP, send two
  sequential messages, and assert both reach the LLM without repeated startup
  storms.

## Related work

- `docs/bugs/uvx_cache_bloat_latest_versions.md`: repeated subprocess creation
  also amplifies `uvx` cache and disk churn.
- MCP alias canonicalization and deduplication were implemented during this
  investigation; they reduce duplicate servers but do not remove nested retries
  or double construction.
- The local Google Sheets configuration now pins `mcp>=1.8,<2`, which removes
  the immediate upstream crash on this machine but does not fix this platform
  behavior.
