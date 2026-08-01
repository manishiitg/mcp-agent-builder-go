# Bug Report: Custom-Tool Categories Leak Into the Agent Contract as Addresses

## Status

Immediate failure fixed 2026-08-01 in `mcpagent/agent/code_execution_tools.go`
(`get_api_spec` now resolves by tool name regardless of `server_name`). The
design question behind it — whether `CustomTool.Category` should be part of the
agent-facing contract at all — is open.

Code referenced here lives in `manishiitg/mcpagent`, not this repository.

## The reported failure

A standalone Pulse Fixer, mid-run:

```json
get_api_spec({
  "server_name": "custom",
  "tool_name": ["start_pulse_fix_attempt", "mark_pulse_module_result",
                "get_pulse_module_state", "get_pulse_finding_backlog"]
})
```

```text
ERROR: server "custom" is not available. Available servers/categories:
[auto_improvement human_tools workflow workspace_advanced]
```

The agent was not confused. It used the name the harness taught it.
`guidance.go:460` states:

> Built-in/custom categories such as `human_tools`, `workflow`,
> `workspace_advanced`, `auto_improvement`, and `knowledgebase_tools` are **not
> MCP servers**; call them as `$MCP_CUSTOM/{tool_name}` **with no category
> segment**.

So the contract is: *ignore the categories, address these tools through
`$MCP_CUSTOM` by name*. The agent did exactly that, and `get_api_spec` — the
tool whose entire job is discovery — demanded the category it had just been told
was not an address.

Three things make this worse than an unhelpful error:

1. `custom` is not an invented name. `agent.go:4047` sets
   `a.toolToServer[name] = "custom"` for every built-in tool, and
   `code_execution_tools.go` treats it as a known value at lines 164, 198, 213,
   287, and 402.
2. The error's recovery hint **filters `custom` out** of the list it prints
   (`code_execution_tools.go:164`), so the name the agent was taught cannot
   appear even in the correction.
3. The categories it does print are the ones the prompt told the agent not to
   use as addresses.

## Root cause: one field doing two unrelated jobs

`CustomTool.Category` is a single string carrying two responsibilities:

**1. Authorization and filtering — legitimate.** Which groups of tools an agent
receives (`workflow`, `human_tools`, `workspace_advanced`, `auto_improvement`).
This is real policy, configured per workflow, enforced by `ToolFilter` via
`customToolCategories` / `systemCategories`. It should stay.

**2. Addressing — the defect.** The value an agent must pass as
`get_api_spec(server_name=...)`. This is where the failure lives, and it is
unnecessary: `customTools` is `map[string]CustomTool` **keyed by tool name**, so
tool names are unique by construction. The category adds no information the tool
name does not already carry.

An agent therefore has to know an internal grouping in order to discover a tool
it is then told to call without that grouping.

## This seam has been patched repeatedly

The strongest evidence that the concept is wrong is how much repair has
accumulated on it. A single category currently answers to at least four
spellings — `workspace`, `workspace_tools`, hyphen/underscore variants, and
`openapi.GetPackageName(category)` — and every lookup site has to try several:

```go
// code_execution_tools.go:76
isCustomCategory := a.toolFilter.IsCategoryDirectory(serverName) ||
    a.toolFilter.IsCategoryDirectory(serverName+"_tools")

// code_execution_tools.go:143
if ct.Category == serverName || openapi.GetPackageName(ct.Category) == serverName+"_tools" {
```

`IsCategoryDirectory` carries four `🔧 FIX` comments, one recording a production
symptom in the same family as today's:

> Previously, system categories like "workspace" were only in systemCategories
> map, but IsCategoryDirectory() only checked customToolCategories. This caused
> "workspace" to be incorrectly filtered as an MCP server, leading to "server
> workspace is filtered out" errors.

And `get_api_spec` **already had** a narrower version of today's fix:

```go
// If not a category, check if server_name is actually a custom tool name —
// resolve to its category. This lets the LLM call
// get_api_spec(server_name="agent_browser") instead of needing to know the
// category name "workspace_browser".
```

That is the same bug, found earlier, patched for the single case where the agent
passes a tool name as the server. `AddCustomCategory` ("safe to call after
`NewToolFilter`") is another repair, for categories registered after the filter
was built.

Four workarounds, one root cause: agents are expected to know a name that only
means something inside the process.

## The fix applied

`get_api_spec` now resolves from the **requested tools** rather than from
`server_name`, generalising the existing tool-name fallback:

```go
// Same resolution as above, widened from the server_name to the tools it asked
// for: when every requested tool is a known custom tool, their categories
// answer the request no matter what server_name was passed.
```

Deliberately keyed on tool names, not on the literal `"custom"`:

- tool names are unique across custom tools, so it cannot answer for the wrong
  tool;
- `$MCP_CUSTOM` is category-free, so one request may legitimately span
  `workflow` and `human_tools` — splitting it would expose the internal grouping
  the prompt told the agent to ignore;
- an unknown tool name still errors, rather than being quietly absorbed.

Tests in `agent/get_api_spec_custom_server_test.go` cover the reported call, a
cross-category request, and the unknown-tool guard. Verified honest: disabling
the resolution fails the first test.

## Do we still need the category?

**Yes for authorization. No for addressing.**

Keep `Category` as the filtering and grouping key — that is genuine policy, and
nothing else expresses "this agent may use workflow tools but not human tools".
It is also a reasonable way to organise the tool index in the system prompt,
where grouping aids reading.

Remove it from the agent-facing contract:

1. **Discovery should be name-based.** `get_api_spec` should never fail on
   `server_name` when the requested tool names resolve. (Done.)
2. **Stop advertising categories as `server_name` values.** The index can group
   by category for readability while the stated contract is "pass the tool names
   you want the spec for". Today the prompt does both, and they contradict.
3. **Include `custom` in the available-servers list**, or drop that list when
   tool names were supplied — printing the categories the agent was told not to
   use is an actively misleading hint.
4. **Longer term, separate the two meanings** so the authorization group and the
   display grouping cannot drift into an address. Every fix above is containment
   until this happens; the four accumulated workarounds are what containment
   looks like when the same seam keeps splitting.

## Acceptance

- `get_api_spec` succeeds for any combination of resolvable tool names,
  irrespective of `server_name` — including `custom`, a category, a tool name, or
  a stale value.
- An unresolvable tool name still errors, naming that tool.
- A real MCP server name continues to route to its server, unchanged.
- Prompt text and `get_api_spec` agree on what an address is. Today they do not,
  and that disagreement is the bug — not the model's inference from it.

## Related

`docs/bugs/pulse_fixer_sqlite_readonly_wal_and_schema_guessing.md` — same class:
an agent burning tool calls rediscovering a harness contract that was never
stated in terms it was given.
