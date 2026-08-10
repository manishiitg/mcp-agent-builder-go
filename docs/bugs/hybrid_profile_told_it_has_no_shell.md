# Bug Report: A Hybrid Profile Is Told It Has No Shell

## Status: Defects 1–2 fixed ✅ · defect 3 dissolved by reverting `native_shell` (2026-08-10, uncommitted)

Defects 1 and 2 are fixed and verified live; both are correct independent of
transport and are kept. Defect 3 — Codex reporting no native shell — is no
longer reachable: `api_transport: native_shell` has been turned **off**, so
product APIs go back through the bridge shell, which every provider can call.
The investigation is preserved below because the finding generalises, and
because `native_shell` is still in the tree awaiting a future attempt. See
[`../design/product_api_transport_for_coding_agents.md`](../design/product_api_transport_for_coding_agents.md).

Video Studio is the first profile to combine `agent_tools: hybrid`,
`api_transport: native_shell`, and a `tool_policy` allow-list. Each defect below
existed before that combination and was invisible without it.

## Symptoms

Asked to list workspace secret names, Video Studio + Codex answers:

> I couldn't retrieve the inventory in this session because the workspace
> command transport is currently not registered for this session.

Claude Code, same profile, same question, succeeds. The asymmetry is the
symptom that made this look provider-specific. It is not: Claude Code hits the
first defect too and silently absorbs it.

## The shared shape

The README's shape, twice, at two different layers:

- **one fact, two sources** — what the session *may run* (the tool gate) versus
  what the agent is *told exists* (the bridge catalog)
- **one fact, two sources** — the product prompt says "use your own shell";
  a second prompt block says "your native tools are disabled"

Both halves were introduced by correct changes. Nothing checked they agreed.

---

## 1. The bridge catalog advertised a tool the gate had removed

`productToolGate` correctly filtered `execute_shell_command`
(`[PRODUCT_TOOL_GATE] filtered=49: … execute_shell_command …`). The bridge
advertised it anyway.

`BuildBridgeMCPConfig` (`mcpagent/agent/coding_agents_bridge.go:133`) resolves
each wanted tool, and on failure **synthesizes one**:

```go
def := a.lookupBridgeTool(want.name, want.toolType, logger)
if def == nil {
    def = defaultBridgeToolDef(want.name, want.toolType, logger)   // ← the leak
}
if def != nil {
    toolDefs = append(toolDefs, *def)
} else {
    logger.Warn("Bridge tool not found — skipping", …)             // ← unreachable for these 3
}
```

`lookupBridgeTool` already returned `nil` — the correct answer. The fallback was
inserted *between* the check and its handler, turning "don't advertise what
doesn't exist" into "advertise it anyway". The synthesized description is the
one the agent acted on:

```go
// mcpagent/agent/codeexec/shell.go:26
const ShellCommandDescription = "Execute a shell command and return stdout, stderr, and exit code. " +
    "Use this to run code, call HTTP endpoints with curl, or perform any shell operation."
```

Codex searched the catalog, found a tool whose description says *"use this to
call HTTP endpoints with curl"* — exactly its task — called it, and was refused
server-side:

```
error="custom tool execute_shell_command is not registered for session video-studio:project:269f4c15-…"
```

The gate guarded **execution**. Nothing guarded **advertisement**.

### Why it survived six weeks

Added 2026-06-25 in `f311d13` ("Add Pi CLI bridge support"), uncommented. Its
premise — *the core three are always available* — was true for every profile
that had ever existed, so the fallback and correct behavior were
indistinguishable. Video Studio is the first profile to remove one.

`docs/design/agent_tool_surface_single_source.md` states the invariant as rule 2,
**"Never filter the catalog … the agent then always knows what exists"**, and
guards only one direction. `catalog ⊂ registered` (hiding a real tool) was
prevented. `catalog ⊃ registered` (advertising a phantom) was not. The doc's own
Open Question 3 asks whether MCP bridge tools are "a fourth registration path
needing the same gate" — and would not have caught this, because
`defaultBridgeToolDef` is not a registration path at all. It registers nothing,
executes nothing, grants nothing. It only *describes*. Enumerating doors misses
a sign pointing at a door that isn't there.

### Fix

`CodingRuntimeConfig.BridgeToolAdmit func(name string) bool` (nil ⇒ admit all,
i.e. every existing caller is byte-identical), consulted in the **core loop
only**:

```go
for _, want := range bridgeTools {
    if want.toolType == "custom" && a.bridgeToolAdmit != nil && !a.bridgeToolAdmit(want.name) {
        continue
    }
    …
}
```

`agent_go` passes the gate's own predicate (`BridgeToolAdmit: config.AdmitTool`),
so one predicate now governs execution *and* advertisement.

The filter must **not** apply to virtual tools or `withAdditionalBridgeTools`
entries: neither passes through `RegisterCustomToolWithTimeout`, so `Admit()`
returns false for both, and a naive filter strips `get_api_spec` — the discovery
door — and skills. A test asserts they survive.

### Verified

```
Core bridge tool excluded by profile tool policy  tool=execute_shell_command
Core bridge tool excluded by profile tool policy  tool=diff_patch_workspace_file
Built bridge MCP config  tool_count=3
```

And from inside Codex's own sandbox:

```js
ALL_TOOLS.filter(x => x.name.includes("execute_shell_command"))  // → []
ALL_TOOLS.filter(x => /api_bridge/.test(x.name))
// → ["mcp__api_bridge__agent_browser","mcp__api_bridge__get_api_spec","mcp__api_bridge__read_skill"]
```

Claude Code was hitting this too — one guaranteed-to-fail call per product-API
task, recovered by falling back to `Bash`. It read as "the allowlist working".

---

## 2. A bridge-only prompt injected into a profile that keeps its native tools

`BuildCLIToolEnvironmentPrompt` (`virtual-tools/delegation_tools.go:759`) opens:

```
## CLI Tool Environment (CRITICAL — Read Carefully)

Your native tools (Bash, Read, Write, etc.) are **disabled**. All tool access
goes through the MCP api-bridge.

| mcp_api-bridge_execute_shell_command | Run any shell command (replaces Bash/Read/Write) |
```

True for `mcp_only`. **False for hybrid**, whose entire premise is that the CLI
keeps its own tools. Injected at `server.go:5258` on one condition:

```go
if common.IsCLIProvider(req.Provider) {
```

It never asked what profile it was in — although `resolvedProfile` is in scope
**118 lines above**, and is consulted at 5140 and 5169 in the same block. The
drift is not distance or missing data. It is an unasked question.

So the agent was told its shell is a tool the allow-list had removed, and
concluded — correctly, given what it was told — that *"the required shell
transport isn't available here."* mcpagent's own bridge-routing block is already
suppressed for hybrid (`server.go:4226`, `profileBridgeRoutingInstructions = &empty`).
This second, separate prompt source was missed.

### Fix

Guard the injection on `agent_tools: hybrid`, mirroring the existing suppression,
and log the skip. A test in `virtual-tools` asserts the prompt still *makes* the
bridge-only claims, so the coupling is visible from both sides: soften that text
and the guard may be unnecessary; strengthen it and the guard matters more.

### Verified applied

```
[CLI PROVIDER] Skipped bridge-only tool environment prompt for codex-cli:
profile "video-studio" keeps its native coding tools
```

**It did not fix the symptom.** See defect 3.

---

## 3. Unresolved: Codex behaves as if it has no shell, and the evidence contradicts itself

After both fixes, Codex still answers *"I don't have a native shell execution
tool in this session"* and emits **zero** tool calls when asked to run one.

Evidence that it **should** have a shell:

- Standalone `codex exec` (0.147.0), no MCP: runs `/bin/zsh -lc 'echo …'`, exit 0.
- Same, **with** an MCP server attached (same `mcpbridge` binary the app uses):
  still runs the shell. So MCP attachment is not the cause.
- Only one code path disables it — `coding_agent_integrations.go:152`, inside
  `!nativeCodingToolsEnabled()`.
- That branch was **not** taken: the session's `turn_context` records
  `approvals_reviewer: auto_review`, set only under
  `nativeCodingToolsEnabled() && !approveAllCodingTools()`.
- The captured session TOML disables nothing relevant (only `remote_plugin`,
  `skill_mcp_dependency_install`).

Evidence that it **doesn't**:

- In-app it names its own tools as `functions.exec`, `functions.wait`,
  `functions.request_user_input`, `collaboration.*` — no `exec_command`,
  no `apply_patch`, no `tools.*` natives.
- Asked plainly to run `echo`, it declines without attempting.

Flags say enabled. Behavior says absent. **Not reconciled.**

### Do not repeat these

- **`functions.exec` ≠ `exec_command`.** Two Codex tools, confusingly similar
  names. `exec_command` is the real shell and *is* removable by flag.
  `functions.exec` is the JS sandbox and survives
  `--disable code_mode_host/code_mode/unified_exec/shell_tool/tool_search`
  (verified on 0.147.0). Two comments in this codebase contradict each other
  because each describes a different one:
  `real_bridge_streaming_e2e_test.go:231` ("cannot be removed by any flag") and
  `coding_agent_integrations.go:166` ("this CORRECTS a long-standing belief").
  Both are right about their own tool. Worth renaming in both comments.
- **Disabling code mode makes it worse, not better.** Those flags remove
  `exec_command` (the working path) and leave `functions.exec` (the sealed one).
- **The JS sandbox is sealed.** Measured: `typeof fetch → undefined`,
  `typeof process → undefined`. No network, no env, no shell. Its only reach is
  `tools.*`. A prompt-only fix is therefore impossible.
- **Codex's self-report is unreliable.** In the control it claimed to have no
  `exec_command`, then immediately ran `/bin/zsh`. Do not accept "I don't have
  X" as evidence — this is the same "agent self-reports are symptoms, not
  evidence" trap recorded elsewhere in this archive.

### Very likely cause, found via Cursor

Cursor — same profile, same question — named the transport correctly and then
was stopped by something the other two never surfaced:

> Approval didn't go through for the secrets API. … the approval prompts for
> those secret-discovery calls were **declined**.

`approvals: provider_auto` requires approval for each native command, and a
headless session has nobody to grant it. **Every CLI test that succeeded used
`--ask-for-approval never`; the product uses `untrusted`.** A tool that always
needs unattainable approval is, to the model, a tool it does not have — which is
exactly how Codex phrased it.

This reconciles the contradiction: the flags do enable the shell, and the
approval posture makes it unusable.

Not fully confirmed for Codex — an `approve_all` experiment was set up but the
test message never sent before the transport was reverted. If it is ever worth
confirming, the decisive artifact is a shim `codex` binary early on the server's
PATH that logs argv and execs the real one. `ps` polling misses the short-lived
`codex exec`; `~/.codex/logs_2.sqlite` is the ChatGPT **desktop app's** log, not
the CLI's.

### Resolution taken

`native_shell` is off; product APIs use the bridge shell again, and
`execute_shell_command` is back in the allow-list. Codex reaches it as an MCP
tool from inside its sandbox, which is the one thing it can always do.

The longer-term direction — making the product's allow-listed tools callable
directly, since `tools.*` is all Codex can reach — `withAdditionalBridgeTools`
already exists for exactly this and documents the rationale — *"native calling is
more reliable than asking the model to discover-then-curl each tool"* — and
`agentsession.go:338` already uses it. The profile's allow-list is the
"small, app-specific, known-in-advance tool set" it was built for.

Codex reached this conclusion unprompted, writing the call itself:

```js
await tools.mcp__api_bridge__list_secrets({})
// → TypeError: tools.mcp__api_bridge__list_secrets is not a function
```

Cost: the launch catalog grows 3 → ~16, cutting against schema-on-demand, and it
changes Claude Code's path from curl to direct tool calls — probably an
improvement, but a behavior change to a working provider.

---

## 4. Also found: personal MCP servers leak into product sessions

Codex routed around the product's tool policy entirely by using
`tools.mcp__node_repl__js` — an MCP server from the developer's personal
`~/.codex/config.toml` — to spawn a shell:

```
/bin/zsh -lc 'curl -fsS -X POST "$MCP_CUSTOM/list_secrets" -H "Authorization: Bearer $MCP_AUTH"'
→ curl: (3) URL rejected: No host part in the URL
```

Two things here:

1. **Containment gap.** A product session inherits whatever MCP servers the
   developer has configured (node_repl, figma, linear, github, supabase …). One
   of them can execute arbitrary code, which defeats `tool_policy` as a boundary.
   The generated session TOML sets `[apps._default] enabled = false` but does not
   use `--ignore-user-config` or an isolated `CODEX_HOME`.
2. `$MCP_CUSTOM` / `$MCP_AUTH` were **empty** there. Not conclusive about our
   injection — that shell was spawned by the node_repl server, a different
   process that would not inherit Codex's environment — but it is the only
   direct observation of those variables in a shell, and it was empty.

## Files changed (uncommitted)

| File | Change |
|---|---|
| `mcpagent/agent/agent.go` | `bridgeToolAdmit` field + `withBridgeToolAdmit` |
| `mcpagent/agent/definition.go` | `CodingRuntimeConfig.BridgeToolAdmit`, wired when non-nil |
| `mcpagent/agent/coding_agents_bridge.go` | filter in the core loop only |
| `mcpagent/agent/coding_agents_bridge_test.go` | nil predicate unchanged; excluded core tools dropped; `get_api_spec` + additional tools survive |
| `agent_go/pkg/agentwrapper/llm_agent.go` | `BridgeToolAdmit: config.AdmitTool`; `CodexNetworkAccess` |
| `agent_go/cmd/server/server.go` | hybrid guard on `BuildCLIToolEnvironmentPrompt`; `CodexNetworkAccess` wiring |
| `agent_go/cmd/server/product_tool_gate_test.go` | real Video Studio manifest excludes shell/diff, keeps `agent_browser` |
| `agent_go/cmd/server/virtual-tools/delegation_tools_test.go` | pins the bridge-only claims the guard depends on |
| `agent_go/internal/videoproduct/prompts/system-prompt.md` | provider-neutral shell wording (did not fix the symptom on its own) |

`CodexNetworkAccess` is included because a `native_shell` profile's APIs are
HTTP: Codex's `workspace-write` sandbox blocks network unless asked. macOS does
not enforce it (verified: curl succeeds either way on 0.147.0), so it changes
nothing locally — Linux does, which is where it would otherwise break silently.

## Related

- [`../design/agent_tool_surface_single_source.md`](../design/agent_tool_surface_single_source.md) —
  rule 2 is the invariant defect 1 violates, in the unguarded direction.
- [`what_the_runtime_tells_an_agent_about_itself.md`](what_the_runtime_tells_an_agent_about_itself.md) —
  same family: the runtime describing itself inaccurately to the agent.
- [`custom_tool_category_as_agent_addressing.md`](custom_tool_category_as_agent_addressing.md) —
  the prior case of an agent-visible description disagreeing with what the
  runtime permits.

## Follow-up worth its own work

`AddInstructions` has **20 call sites**, each with an ad-hoc condition, each
discarding its error with `_ =`. Defect 2 is what that produces. ~14 of them are
one contiguous run (`server.go:5127–5264`) — already a de-facto assembly point,
written as inline `if`s. A named registry (`Name` / `Applies(profile) bool` /
`Build`) with an `[PROMPT_SECTIONS] included=… skipped=…` log would make prompt
composition inspectable, and make contradictions like defect 2 testable rather
than discoverable only by reading a coding agent's session rollout — which is how
this one was found.
