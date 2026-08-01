# mcpagent public API simplification

**Status:** In progress
**Date:** 2026-08-01
**Repositories:** `mcpagent`, `mcp-agent-builder-go`
**Related:** `docs/bugs/custom_tool_category_as_agent_addressing.md`

## Decision

An agent definition has only three meaningful parts:

1. **Instructions** — what the agent should do and the constraints it follows.
2. **Skills** — reusable domain or procedural knowledge available to it.
3. **Tools** — capabilities it may discover and execute, whether implemented
   directly or supplied by MCP servers.

These three inputs are fixed when the agent is created. They are not mutable
lifecycle state.

Model selection, provider transport, workspace paths, permissions, session
handles, events, logging, usage accounting, caches, retries, and routing are
runtime infrastructure. They may configure or operate an agent, but they are
not additional agent-content concepts and should not expand the main `Agent`
API.

## Why this refactor is needed

`mcpagent.Agent` currently exposes 70 methods. The large surface allows callers
to manage internal lifecycle order themselves:

- overwrite, append, clear, inspect, and rebuild prompt state;
- register a tool and then explicitly refresh a second registry;
- narrow tool access after prompt construction;
- address custom tools through implementation categories;
- manipulate provider, bridge, event, virtual-tool, and continuation internals;
- choose between several overlapping conversation and delivery APIs.

This is how state became duplicated across prompt text, tool maps, allow lists,
execution registries, schema caches, and session handles. A caller can invoke
individually valid methods in an invalid order.

The first containment pass reduced the surface from 84 to 70 methods and
removed the worst prompt and registry lifecycle combinations. It is not the
final design. In particular, methods such as `SetInstructions`,
`AddInstructions`, `ResetInstructions`, `SetToolAccess`, and
`RegisterCustomTool` still allow an already-created agent to change identity.

Production evidence makes the cost concrete:

- 46 failed `get_api_spec` calls were recorded in one day, dominated by callers
  guessing a category or server even though the tool name already resolved.
- One background agent received a 10,476-character prompt with no tool inventory
  while `query_workflow_db` was registered and executable.
- A materialized prompt previously grew to 14 times its intended size across
  turns because clearing one prompt list did not rebuild the copied prompt text.

## Core invariant

```text
agent_definition = instructions + skills + tools

effective_tools = agent_definition.tools intersect runtime_permissions
```

The definition is immutable. Runtime permissions may deny an operation, but
they must not rewrite the definition, mutate prompt text, or create another
tool-addressing scheme.

If instructions, skills, or the available tool set need to change, create a new
agent definition. Do not reset an existing agent.

## Target public model

```go
type AgentDefinition struct {
    Instructions string
    Skills       []Skill
    Tools        ToolSet
}

type ToolSet struct {
    Direct []ToolDefinition
    MCP    []MCPToolSource
}

type ToolDefinition struct {
    Name        string
    Description string
    InputSchema map[string]any
    Execute     ToolExecutor
    Timeout     time.Duration
}

type MCPToolSource struct {
    ServerID string
    Config   MCPServerConfig
}

type RuntimeConfig struct {
    Model         ModelConfig
    Transport     TransportConfig
    Workspace     WorkspaceAccess
    Observability ObservabilityConfig
}
```

Construction becomes explicit:

```go
agent, err := mcpagent.NewAgent(ctx, AgentDefinition{
    Instructions: instructions,
    Skills:       skills,
    Tools: ToolSet{
        Direct: directTools,
        MCP:    mcpServers,
    },
}, runtimeConfig)
```

MCP is a tool source, not a second kind of agent. MCP tools and direct tools
enter one name-keyed registry and become indistinguishable to the model after
registration.

## Categories are not part of the public contract

The current `RegisterCustomTool(..., category)` signature gives `category`
several unrelated jobs:

- configuration bundle;
- display grouping;
- filtering key;
- schema-discovery address;
- routing hint.

Only the first two are legitimate long-term uses.

Categories may remain internal metadata for configuration expansion or UI
grouping. They must not appear in:

- `AgentDefinition` tool addresses;
- `get_api_spec` calls;
- execution URLs chosen by the model;
- authorization decisions after construction;
- cache identity.

The public and model-facing address is the globally unique tool name. Registry
construction fails if two sources claim the same name.

## Target runtime API

The ordinary API should have approximately 8–12 methods, not 70:

```go
type Agent interface {
    Start(ctx context.Context) (Session, error)
    Definition() AgentDefinitionView
    Close() error
}

type Session interface {
    Run(ctx context.Context, turn Turn) (Result, error)
    Send(ctx context.Context, input Input) (DeliveryResult, error)
    Snapshot() SessionHandle
    Events() <-chan Event
    Close() error
}

type Turn struct {
    Input      Input
    ToolPolicy ToolPolicy
}
```

`Run` starts or continues a model turn and applies that turn's runtime tool
policy. `Send` delivers steering or user input into an already-running turn; it
does not begin a second turn. The effective tool manifest is rendered at the
outbound request boundary from the immutable definition intersected with the
current `ToolPolicy`. It is never stored in the base instructions.

For simple one-turn callers, a convenience method may be provided:

```go
result, err := agent.Run(ctx, input)
```

The exact count is less important than the ownership boundary:

- `Agent` owns one immutable definition.
- `Session` owns conversation history, continuation, steering, and delivery.
- `Result` owns output, usage, and diagnostics.
- internal packages own bridge routing, virtual tools, caches, event emission,
  and provider transport.

## Current 70 methods and intended disposition

### Core execution

| Current method | Disposition |
|---|---|
| `Ask` | Replace with `Run` convenience method. |
| `AskWithHistory` | Move history ownership to `Session.Run`. |
| `Close` | Keep, returning an error consistently. |

### Instructions

| Current method | Disposition |
|---|---|
| `SetInstructions` | Remove; instructions are construction input. |
| `AddInstructions` | Remove; builder assembles final instructions before construction. |
| `ResetInstructions` | Remove; a phase change creates a new agent. |
| `Instructions` | Replace with read-only `Definition()` or diagnostic snapshot. |

### Skills

| Current method | Disposition |
|---|---|
| `AttachSkill` | Remove from runtime API; skills are construction input. |
| `AttachedSkills` | Expose only through read-only definition diagnostics if needed. |
| `DetachSkill` | Remove; create a new agent definition. |
| `ClearSkills` | Remove; create a new agent definition. |

### Tool definition, registration, and policy

| Current method | Disposition |
|---|---|
| `RegisterCustomTool` | Replace with `ToolDefinition` in `AgentDefinition.Tools.Direct`. |
| `RegisterCustomToolWithTimeout` | Fold timeout into `ToolDefinition`. |
| `ReplaceCustomToolExecutor` | Remove; registry is immutable after construction. |
| `GetCustomToolExecutor` | Internal registry operation. |
| `GetCustomTools` | Read-only definition diagnostics, not mutable map exposure. |
| `GetCustomToolsByCategory` | Remove; category is not runtime addressing. |
| `GetCustomToolCategories` | Move to configuration/UI metadata if still needed. |
| `SetToolAccess` | Replace mutable agent state with `Turn.ToolPolicy`; permissions remain runtime policy, not identity. |
| `SetToolArgTransformer` | Fold into the tool definition or internal adapter. |
| `GetToolOutputHandler` | Internal. |
| `SetToolOutputHandler` | Runtime configuration at construction. |
| `GetToolToServer` | Internal registry routing. |
| `GetSelectedTools` | Read-only definition diagnostics if needed. |

### Provider, MCP, workspace, and connection state

| Current method | Disposition |
|---|---|
| `GetProvider` | Read from definition/runtime diagnostic view. |
| `SetProvider` | Remove; provider is immutable runtime configuration. |
| `GetLLMModelConfig` | Read from runtime diagnostic view. |
| `GetPrompts` | Remove from `Agent`; MCP prompt resources are internal tool-source data. |
| `GetResources` | Remove from `Agent`; MCP resources are internal tool-source data. |
| `GetServerNames` | Read-only tool-source diagnostics if needed. |
| `GetConfiguredServerName` | Remove singular server abstraction; definition contains tool sources. |
| `GetMCPConfigJSON` | Internal bridge adapter operation. |
| `SetFolderGuardPaths` | Construction-time `WorkspaceAccess`. |
| `GetFolderGuardPaths` | Runtime diagnostic view only. |
| `GetContext` | Remove; callers pass contexts to operations. |
| `IsCancelled` | Remove; operation errors and context state are authoritative. |
| `CheckConnectionHealth` | Move to a diagnostics interface/service. |
| `GetConnectionStats` | Move to a diagnostics interface/service. |

### Events and usage

| Current method | Disposition |
|---|---|
| `AddEventListener` | Replace with construction-time observer or `Session.Events`. |
| `RemoveEventListener` | Eliminated by session/event-subscription lifetime. |
| `EmitTypedEvent` | Internal only. |
| `HandleEvent` | Internal adapter/interface only. |
| `HasStreamingCapability` | Runtime capability view, not a primary method. |
| `GetEventStream` | Merge into `Session.Events`. |
| `SubscribeToEvents` | Merge into `Session.Events` or a separate observer. |
| `GetTokenUsage` | Return structured usage in `Result`. |
| `GetTokenUsageWithPricing` | Return structured usage/cost in `Result`. |

### Conversation, continuation, and steering

| Current method | Disposition |
|---|---|
| `AddSteerMessage` | Internal queue operation behind `Session.Send`. |
| `DrainSteerMessages` | Internal only. |
| `ContinueConversation` | Merge into `Session.Run`/`Session.Send`. |
| `TurnInFlight` | Read-only session state if genuinely needed. |
| `SupportsSteering` | Session capability view. |
| `Deliver` | Merge into `Session.Send`. |
| `DeliverControlKey` | Optional specialized session-control API. |
| `DeliverUserMessage` | Merge into `Session.Send`. |
| `StartCodingAgentTransportSession` | Internal provider adapter. |
| `StartCodingAgentTmuxSession` | Remove legacy transport-specific entry point. |
| `CurrentAgentSessionHandle` | Replace with `Session.Snapshot`. |
| `ApplyAgentSessionHandle` | Handle is supplied to `ResumeSession` construction. |
| `ContinueAgentSession` | Merge into resumed `Session.Run`. |
| `ContinueAgentSessionWithHistory` | Merge into resumed `Session.Run`. |

### Bridge, virtual tools, and large-output internals

| Current method | Disposition |
|---|---|
| `BuildBridgeMCPConfig` | Internal transport adapter. |
| `CreateVirtualTools` | Internal registry construction. |
| `HandleVirtualTool` | Internal routing. |
| `CreateLargeOutputVirtualTools` | Internal registry construction. |
| `HandleLargeOutputVirtualTool` | Internal routing. |
| `BuildLargeOutputFilePath` | Internal storage service. |

### Tool-search diagnostics

| Current method | Disposition |
|---|---|
| `GetDiscoveredToolCount` | Diagnostics/result metadata. |
| `GetDeferredToolCount` | Diagnostics/result metadata. |

### Legacy prompt method

| Current method | Disposition |
|---|---|
| `RebuildSystemPromptWithFilteredServers` | Delete; contradicts immutable definitions and request-time rendering. |

## Required builder changes

`mcp-agent-builder-go` currently constructs an agent and then incrementally
changes it. The builder must instead complete definition assembly first.

### Before

```text
create agent
  -> register tools one at a time
  -> add instruction fragments
  -> clear/reset instructions for a phase
  -> attach skills
  -> apply tool allow list
  -> start or resume
```

### After

```text
resolve phase
  -> build final instructions
  -> resolve enabled skills
  -> expand configured tool bundles and MCP sources
  -> validate unique tool names
  -> create immutable agent
  -> start or resume session
  -> apply the current permissions to each turn
```

When the workflow phase changes identity inputs, the builder creates a new agent
with the new definition. A workshop-mode change that only changes authorization
uses a new per-turn `ToolPolicy`, not a new agent. Conversation continuity is
preserved through an opaque `SessionHandle`; it is not preserved by mutating the
old agent.

Dynamic values should be routed deliberately:

| Value | Destination |
|---|---|
| User request, run-specific evidence | Input message/context |
| Stable role and constraints | `Instructions` |
| Reusable procedural guidance | `Skills` |
| Executable capabilities | `Tools` |
| Secrets and environment values | Tool runtime environment, never prompt mutation |
| Folder permissions | `RuntimeConfig.Workspace` plus per-turn `ToolPolicy` |
| Continuation state | `SessionHandle` |
| Usage, cost, tool failures | `Result` and observability |

## Migration plan

Every stage is independently shippable. Each stage lands as its own commit in
both repositories, retains the previously working path until its replacement is
covered, and can be reverted without reverting later unrelated work. Production
diagnostics must identify whether the legacy or replacement path handled a run.

### Stage 1 — freeze the target API

- Add `AgentDefinition`, `ToolSet`, `ToolDefinition`, and `RuntimeConfig`.
- Add a public-API golden test that enumerates exported `Agent` and `Session`
  methods.
- Freeze the legacy `*Agent` baseline at exactly 70 exported methods and only
  lower that committed number during migration.
- Set the final target at exactly four `Agent` methods (`Start`, `Run`,
  `Definition`, `Close`) and five `Session` methods (`Run`, `Send`, `Snapshot`,
  `Events`, `Close`).
- Do not add compatibility aliases without an explicit migration deadline.

### Stage 2 — structured tool registry

- Replace positional `RegisterCustomTool` calls with `ToolDefinition` values.
- Reject duplicate names before mutating any registry, identifying both owners.
- Expand category-based configuration bundles before `NewAgent`.
- Load MCP tools into the same name-keyed registry.
- Fail construction on duplicate names.
- Remove category from schema discovery, execution authorization, and cache keys.

### Stage 3 — immutable construction in the builder

- Introduce one builder function that returns `AgentDefinition` and
  `RuntimeConfig`.
- Move all current `AddInstructions`, `AttachSkill`, and tool registration
  identity assembly before construction.
- Replace phase mutation with new-agent construction.
- Replace per-turn `SetToolAccess` mutation with `Turn.ToolPolicy`.
- Delete `SetInstructions`, `AddInstructions`, `ResetInstructions`, and skill
  mutation.

### Stage 4 — one session abstraction

- Introduce `Session` as the owner of history, streaming, continuation,
  steering, and delivery.
- Replace overlapping `Ask*`, `Continue*`, `Deliver*`, and transport-specific
  methods.
- Persist and restore only opaque `SessionHandle` values.

### Stage 5 — move infrastructure off Agent

- Move bridge configuration and virtual-tool handlers into internal packages.
- Move event emission into an observer/transport layer and migrate all 11
  downstream listener-registration call sites.
- Return usage and diagnostics in `Result`.
- Move connection health into a diagnostics service.
- Remove raw mutable map getters.

### Stage 6 — delete legacy surface

- Remove old methods rather than retaining permanent deprecated wrappers.
- Remove old lifecycle documentation and tests.
- Update examples and both repositories in the same change.
- Run the public-API golden test to prevent regrowth.

## Acceptance criteria

- `AgentDefinition` exposes only instructions, skills, and tools as agent
  identity.
- Instructions, skills, tools, model, provider, and workspace policy cannot be
  mutated after construction.
- A phase change that alters identity creates a new definition; a workshop-mode
  authorization change supplies a new per-turn policy.
- Direct and MCP tools share one unique name-keyed registry.
- Category is absent from the public tool registration and discovery APIs.
- Duplicate tool names fail construction with both sources identified.
- The builder performs no prompt rebuilding, registry refreshing, allow-list
  mutation, or manual tool-manifest assembly.
- The final `Agent` surface has exactly four methods and `Session` exactly five.
- Events emitted for a run contain the same instructions and effective tool
  view actually sent to the model.
- Usage, cost, tool failures, and diagnostics are returned structurally rather
  than queried through unrelated `Agent` getters.
- Existing API-provider and coding-agent continuation behavior remains covered
  by end-to-end tests.
- `go test ./...` passes in `mcpagent` and `go test ./agent_go/...` passes in
  `mcp-agent-builder-go`.

## Review (2026-08-01)

Endorsed in direction. The core diagnosis — *"a caller can invoke individually
valid methods in an invalid order"* — is exactly what produced every failure in
the related bug report, and `agent_definition = instructions + skills + tools`,
immutable, is the right generalisation. The 70-method count is accurate
(verified: 46 in `agent.go`, 24 across nine other files). The public-API golden
test in Stage 1 is the single most valuable item here, because it is the only
part that prevents regrowth.

Four things to resolve before this is actionable.

### 1. The invariant and the method disposition contradict each other

The invariant permits runtime permissions:

```text
effective_tools = agent_definition.tools intersect runtime_permissions
```

But the table disposes of `SetToolAccess` as *"Remove from normal runtime API;
construct the agent with its effective tool set."* Those cannot both hold. The
second says permissions are construction-time; the first says they are not.

This is not hypothetical. `cmd/server/workflow_phase_tools.go` applies the allow
list **per turn, deliberately**:

> The chat-history auto-restore path … passes `applyAllowList=false` so the
> restored CLI sees the superset; `/api/query` later narrows it via
> `SetToolAllowList` **when the first user turn arrives**.

The workshop mode is not known at construction, and a restored coding-agent CLI
must be created with the superset before it can be narrowed.

**Correction to an earlier draft of this review:** it claimed recreating the
agent per phase was impractical because it would re-run MCP setup and invalidate
the provider session. Both reasons are wrong. MCP connections are lazy and
session-keyed (`connection_session.go:404` — *"prefers the session registry
(lazy-connect, reuses existing connections)"*), so a new `Agent` on the same
`SessionID` reuses them. Continuation already survives recreation:
`AgentSessionHandle` carries `SessionID` plus `CodingProviderSessionHandle`, and
`ApplyAgentSessionHandle` restores it (`session_handle.go:63`, `:86`) — which is
the mechanism this document already names. Recreation on a **phase change** is
therefore viable, and the "new agent per phase" position stands.

What remains is narrower: the allow list is applied *per turn*, not per phase, so
"construct with the effective tool set" would mean a new agent every turn — a
heavier claim than the plan makes, and one the plan should either commit to
explicitly or drop.

The invariant is the part that is right. Keep `effective_tools` derived at
request time from the definition and the *current* permissions, and let a
permission-setting call remain in the runtime API — it changes policy, not
identity, which the invariant already allows. Then correct the `SetToolAccess`
row, and state explicitly that the tool manifest sent to the model is rendered
from `effective_tools` at request time rather than stored. Without that sentence,
a reader implements immutability by materialising the manifest at construction,
which is precisely the bug this refactor exists to remove.

### 2. There is no incremental verification or rollback story

Six stages across two repositories, ending with *"update examples and both
repositories in the same change"* and *"remove old methods rather than retaining
permanent deprecated wrappers"*. That is a big-bang cutover for the layer every
agent in the system runs on.

The evidence for caution is in this repository's own history: on 2026-08-01 two
separate, carefully-reasoned fixes to this exact area were wrong — one placed in
`SetSystemPrompt`, one in `rebuildSystemPromptWithUpdatedToolStructure` — and
both had passing tests that encoded the author's assumed lifecycle rather than
the production one. A rewrite of the whole surface by the same means needs an
answer to: what runs in production during Stages 2–5, and what is the revert if
Stage 4 is wrong?

Suggest making each stage independently shippable and observable, with Stage 2
(structured registry, duplicate-name failure, category out of discovery and cache
keys) first — it is the smallest, it is already half-done, and it carries most of
the correctness benefit.

### 3. Cite the production evidence

The "why" here is argued structurally. The concrete evidence is stronger and
lives in the sibling document: 46 failed `get_api_spec` calls in one day; a
background agent shipped a 10,476-character prompt with no tool inventory while
`query_workflow_db` sat registered; and the pre-existing **CLAUDE.md 14× bloat
bug**, recorded in `ClearAppendedSystemPrompts`, where a materialised prompt grew
14× across turns because its copy drifted from its inputs. That last one predates
all of this and shows the missing invariant has already been paid for once, and
was patched with a caller-ordering convention rather than a fix.

Anyone deciding whether to fund this work should see those three numbers.

### 4. Smaller notes

- `Session.Events() <-chan Event` replaces listener registration.
  `mcp-agent-builder-go` has 11 `AddEventListener`/`RemoveEventListener` call
  sites, so this is a bounded but real downstream migration and deserves naming
  in Stage 5 rather than the one-line "move event emission into an observer
  layer".
- `Ask` → `Run` and `Session.Run`/`Session.Send` overlap enough to be confused;
  worth one sentence on which is a turn and which is a delivery into an in-flight
  turn, since `Deliver`/`DeliverUserMessage`/`AddSteerMessage` all fold into
  `Send` and their current semantics differ.
- Acceptance criteria should include the golden test *count* as a committed
  number, not just "no more than 12". A count that can be edited is not a ratchet.

## Non-goals

- Replacing MCP itself.
- Putting full tool schemas into the system prompt.
- Removing runtime permission enforcement.
- Forcing API providers and coding-agent CLIs to use identical internal
  transports.
- Persisting an agent object. Persist only definitions and opaque session
  handles.

## Final target

The builder should be able to explain agent creation in one sentence:

> Resolve the instructions, skills, and tools; create the agent; run a session.

If builder code must refresh a registry, reset instructions, discover a tool
category, or manually synchronize prompt text with authorization state, the
ownership boundary is still wrong.
