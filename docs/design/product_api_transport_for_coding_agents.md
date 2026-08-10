# Product API Transport for Coding Agents

## Status: implemented, deliberately NOT enabled (2026-08-10)

`api_transport.mode: native_shell` works end to end and is left **off**. Video
Studio reaches product HTTP APIs through `execute_shell_command` on the MCP
bridge, as it always has.

This document exists so the next person does not rediscover it. Everything below
was measured against **codex-cli 0.147.0**, **cursor-cli**, and **claude-code**
in a live Video Studio session, not reasoned from the code.

---

## The question

A product tool like `list_secrets` is an HTTP endpoint, not a callable tool. The
agent discovers it with `get_api_spec` and then has to *issue a request*. Which
shell issues it?

- **`bridge_shell`** (current) — `execute_shell_command`, an MCP bridge tool.
  Runs server-side in the executor process.
- **`native_shell`** (built, off) — the coding CLI's **own** shell, with
  `$MCP_CUSTOM` / `$MCP_AUTH` / `$MCP_API_URL` injected into its environment.

`native_shell` is attractive because a hybrid profile already gives the CLI its
native tools, so the bridge shell looks redundant. **It is not redundant.**

## Why it is off

**The design assumes every agent can write and run `curl`. That is false.**

| Provider | Has a shell? | Can call product APIs from it? |
|---|---|---|
| claude-code | yes (`Bash`) | ✅ yes — the only one that worked |
| codex-cli | **effectively no** | ❌ no path at all |
| cursor-cli | yes | ❌ blocked by unanswerable approval prompts |

### codex-cli: the JS sandbox is sealed

Codex operates through `functions.exec`, a JavaScript sandbox. Measured inside
it:

```js
typeof fetch     // "undefined"
typeof process   // "undefined"   → no env, so no $MCP_AUTH / $MCP_CUSTOM
```

Its only reach is `tools.*` — the MCP tools. So it cannot issue an HTTP request
at all, and cannot read the variables `native_shell` injects. With
`execute_shell_command` removed from the allow-list, Codex had **zero** paths to
any product API. It said so, accurately:

> I couldn't retrieve the inventory in this session because the workspace
> command transport is currently not registered for this session.

Codex diagnosed the right fix itself, unprompted:

```js
await tools.mcp__api_bridge__list_secrets({})
// → TypeError: tools.mcp__api_bridge__list_secrets is not a function
```

### cursor-cli: approvals cannot be answered headlessly

Cursor named the transport correctly — *"get_api_spec for list_secrets, then my
built-in shell to POST `$MCP_CUSTOM/list_secrets` with `$MCP_AUTH`"* — tried it,
and was stopped:

> Approval didn't go through for the secrets API. … the approval prompts for
> those secret-discovery calls were declined.

`approvals: provider_auto` means each native command needs approval. In a
non-interactive session there is nobody to grant it. Every CLI test that
succeeded here used `--ask-for-approval never`; the product uses `untrusted`.

This is very likely the same reason Codex reports having no shell: a tool that
always requires unattainable approval is indistinguishable from a tool that does
not exist.

### The pattern

`bridge_shell` works everywhere because it is an **MCP tool call** — the one
mechanism all three providers demonstrably have. `native_shell` depends on
provider-specific capabilities (a real shell, an approval posture, env
injection, sandbox network policy) that vary per CLI and per version.

---

## What `native_shell` costs, in machinery

All of this exists solely so a model can hand-write `curl`:

- env injection of `MCP_API_URL` / `MCP_API_TOKEN` / `MCP_SESSION_ID`, plus
  `PopulateMCPBridgeShortEnv` deriving `$MCP_CUSTOM` / `$MCP_AUTH`
- the scoped-key policy (`llmtypes.IsScopedCodingAgentEnvironmentKey`) admitting
  `SECRET_*`, `VAR_*` and a closed MCP route set
- `CodexNetworkAccess` — Codex's `workspace-write` sandbox blocks network unless
  asked. macOS does not enforce this (verified: `curl` succeeds either way on
  0.147.0); **Linux does**, which is where it would silently break
- fail-closed handling when the bridge env is incomplete
- the session baked into the base URL (`/s/<sessionID>`) so hand-written curls
  are session-scoped by construction

Against a tool call, where the model emits a name and arguments and the runtime
builds the request: none of it is needed.

## Security note

With a shell, `tool_policy` is advisory — the agent can `curl` anything. During
testing Codex escaped the policy entirely via an unrelated MCP server:

```
tools.mcp__node_repl__js → /bin/zsh -lc 'curl … "$MCP_CUSTOM/list_secrets" …'
```

`node_repl` came from the developer's personal `~/.codex/config.toml`. **Product
sessions inherit personal MCP servers**, and one of them can execute arbitrary
code. Tracked in
[`../bugs/hybrid_profile_told_it_has_no_shell.md`](../bugs/hybrid_profile_told_it_has_no_shell.md).
Worth fixing regardless of transport (`--ignore-user-config`, or an isolated
`CODEX_HOME`).

Also observed there: `$MCP_CUSTOM` was **empty** in that shell (`curl: (3) URL
rejected: No host part in the URL`). Not conclusive — a different process that
would not inherit Codex's env — but it is the only direct observation of those
variables in a shell, and it was empty. **Verify env injection actually lands
before trusting `native_shell`.**

---

## If we revisit this

**Preferred direction: don't.** Register the product's allow-listed tools as
real MCP bridge tools instead, and drop the shell from the API path entirely.

`withAdditionalBridgeTools` already exists for this and states the rationale —
*"native calling is more reliable than asking the model to discover-then-curl
each tool"* — and `internal/agentsession/agentsession.go:338` already uses it.
Set `runtime.Tools.AdditionalBridge` from the profile's allow-list.

Why this now fits, when it didn't before: `get_api_spec` + curl exists to keep
the launch catalog small (~63 platform tools of schemas, cached once at launch).
But `tool_policy` already narrows this product to **14 declared** tools.
**The allow-list solved the problem schema-on-demand was invented for.**

Then:

- Codex calls `tools.mcp__api_bridge__list_secrets({})` — the call it already writes
- Cursor and Claude Code call tools directly; no approval prompts, no curl
- Tokens stop appearing in argv (`Authorization: Bearer $MCP_API_TOKEN` is
  currently written into a command line)
- `tool_policy` becomes a real boundary: 14 means 14

Costs: the launch catalog grows 3 → ~16, and Claude Code's working path changes
from curl to direct calls — probably an improvement, but a behavior change to a
provider that works today, so it needs its own live retest.

**Keep `agent_tools: hybrid` either way.** Native tools for workspace work
(rendering, files, QA) and the transport for product APIs are independent
decisions that `native_shell` conflated.

## Before enabling `native_shell` again, verify

1. Every target provider can actually run a shell command in a **headless**
   session under the profile's `approvals` mode — not just at the CLI with
   `--ask-for-approval never`.
2. `$MCP_CUSTOM` / `$MCP_AUTH` are non-empty **inside that provider's own
   shell** (a set/empty probe is enough; do not ask an agent to echo the value —
   it will refuse under the secret-handling rules, correctly).
3. Network egress works under the provider's sandbox on **Linux**, not just
   macOS.
4. Trust behavior, never self-report. Codex claimed it had no `exec_command` and
   then ran `/bin/zsh` in the same session.

## What is still in the tree

`native_shell` remains implemented and tested — only the profile setting is off:

- `agent_go/cmd/server/server.go` — the `native_shell` branch, fail-closed, and
  `CodexNetworkAccess` wiring
- `agent_go/pkg/agentprofiles/validate.go` — `native_shell` requires
  `agent_tools: hybrid`
- `agent_go/cmd/server/coding_agent_environment_contract_test.go` — the scoped
  child-process environment contract
- `multi-llm-provider-go/llmtypes/options.go` —
  `IsScopedCodingAgentEnvironmentKey`, the single owner of the key policy

Two fixes made while investigating are **kept**, because they are correct
independent of transport:

- the coding-agent bridge no longer advertises a tool the profile's gate removed
  (`BridgeToolAdmit`)
- the bridge-only "your native tools are disabled" prompt is no longer injected
  into a hybrid profile

Both are described in
[`../bugs/hybrid_profile_told_it_has_no_shell.md`](../bugs/hybrid_profile_told_it_has_no_shell.md).

## Related

- [`agent_tool_surface_single_source.md`](agent_tool_surface_single_source.md) —
  the gate that decides a profile's tool surface
- [`native_coding_agent_environment_policy.md`](native_coding_agent_environment_policy.md) —
  the scoped environment `native_shell` depends on
