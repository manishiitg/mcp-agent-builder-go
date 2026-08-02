# mcpagent public API simplification

**Status:** Immutable cutover and planned public-surface reduction complete
**Date:** 2026-08-01
**Last updated:** 2026-08-02
**Repositories:** `mcpagent`, `mcp-agent-builder-go`
**Related:** `docs/bugs/custom_tool_category_as_agent_addressing.md`

## Implementation status

Completed core boundary:

- `Agent` is opaque: it exposes exactly four methods and no exported fields.
- `Session` exposes exactly five methods; both lifecycle types use
  `Close() error`.
- Builder, workflow, chat, delegation, gRPC, Family Server, and test callers no
  longer read or write public `Agent` state.
- Construction-time runtime values use typed `RuntimeConfig`; request-specific
  streaming and tool policy use `Turn`.
- Read-only access uses definition, diagnostics, runtime-info, and opaque
  session-handle snapshots.
- `DefinitionAssembly`, `LegacyOptions`, all 57 exported `With*` functions, and
  the three legacy public constructors were removed from the supported API.
- Unsafe testing/mutation helpers, example-only conveniences, and dead recovery
  wrappers were removed. The package now has exactly 45 exported functions,
  down from 148 at the start of this pass.
- The unused example trees were deleted rather than allowing demonstrations to
  keep compatibility APIs public. Command tests call `Agent.Run` directly.
- The legacy Tool Search mode was removed completely. Provider-native Codex
  `tool_search` disabling remains a separate transport capability.
- Attached skills now have one transport-neutral access contract owned by
  `mcpagent`: attaching the first skill installs the reserved `read_skill` tool,
  exposes it through the coding-agent bridge, and keeps native CLI projection
  as an optional optimization rather than the only way to read a skill.
- Golden tests pin the exact 45-function inventory and the exact 14-function
  `Agent` facade, as well as the four/five method contracts, zero exported
  `Agent` fields, matching close contracts, and no exported `ForTesting` helper.

Verified on the current working tree (2026-08-02):

- `go test ./... -count=1` passes in `mcpagent`, including the exact API
  ratchets.
- `go test -race ./agent -count=1` and `go vet ./agent` pass in
  `mcpagent`.
- `go test ./... -count=1` passes in `mcp-agent-builder-go/agent_go`.
- That suite compiles and verifies the real `agent_go/cmd/family-server` target.
- `npm run build` passes in `frontend`, and the rebuilt static bundle contains
  no legacy structured-output event renderer.
- `npm test -- --run` passes all 54 frontend test files (335 tests).

### Surface audit (2026-08-02): exact contract after reduction

The final measured surface is:

```text
*Agent methods                                    4
Agent exported fields                             0
*Session methods                                  5
package functions                                45
  exported With* options                          0
  public constructors returning *Agent            1
  functions accepting or returning *Agent        14
```

The AST-based golden test pins the sorted names, not merely these counts. A
deleted export cannot be silently replaced by a different export, and moving a
method into a package function changes the reviewed 14-name facade list.

### Consumer boundary and Family Server

The supported local consumers are `mcpagent`'s gRPC/command packages and
`mcp-agent-builder-go/agent_go`, which uses the local module replacement. The
real Family Server is `agent_go/cmd/family-server`; it lives on
`mcp-agent-builder-go` main and is compiled by the full builder suite. Its small
refactor-branch delta uses the immutable session API.

The separately named `/Users/mipl/ai-work/mcpagent-family-server` folder is an
old worktree of the **same mcpagent module** on
`fix/stale-claude-native-resume`. It is not the Family Server application and is
not a downstream consumer to migrate. Its stale-resume commits should be merged
through normal branch integration if still wanted; its legacy public API does
not define the supported contract.

No third-party consumer was found in the local workspace. A breaking published
release still requires an explicit external-consumer policy.

### Completed reduction work

The five-step follow-up is complete on the supported code path:

1. The exact exported inventory and Agent-facade inventory are golden-tested.
2. The builder constructs `AgentDefinition` directly; `DefinitionAssembly` and
   all duplicate `AddDefinition*` wrappers are gone.
3. `RuntimeConfig` is grouped into generation, tools, context, coding, MCP,
   workspace, and observability values. `LegacyOptions`, exported `With*`
   options, and public legacy constructors are gone.
4. Package-only ask, retry, summarization-threshold, recovery, and definition
   lookup helpers are private. Dead constructors and one-shot structured/test
   conveniences were deleted. Unused example trees were deleted with user
   approval; command tests use `Agent.Run` and diagnostics.
5. The actual Family Server target is part of the active builder suite and
   passes. The similarly named old mcpagent worktree was correctly excluded as
   a consumer.

The legacy structured-output subsystem went further than originally planned.
The structured ask/conversion helpers and dynamically forced completion tool
were removed outright, together with their private implementation, bespoke
events, tracing spans, UI renderers, and obsolete command tests. A grep across
both supported consumers found no runtime caller. Builder-owned JSON results
continue to use explicit prompt contracts plus prevalidation, or ordinary
declared tools. Coding-agent `transport=structured` remains intact because it
is the CLI transport protocol, not this deleted response-conversion feature.

What remains public is context compaction/summarization, resume, steering,
diagnostics, and retirement, because active production code calls them. They
form the reviewed 14-name facade ratchet. Moving some of them into narrower
runtime services may be a future cleanup, but is not a compatibility shim or
blocker for this refactor.

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

Before this cutover, `mcpagent.Agent` exposed 70 methods and 64 exported mutable
fields. That large surface allowed callers to manage internal lifecycle order
themselves:

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
removed the worst prompt and registry lifecycle combinations. The completed
core cutover reduced the concrete `Agent` surface again—from 70 methods and 64
exported fields to four methods and zero exported fields. The old mutation
operations are no longer methods on a live agent; the remaining compatibility
work is confined to constructors and package-level bridge/runtime helpers.

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

### Removed legacy Tool Search diagnostics

| Current method | Disposition |
|---|---|
| `GetDiscoveredToolCount` | Removed with the legacy Tool Search mode. |
| `GetDeferredToolCount` | Removed with the legacy Tool Search mode. |

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

## Cutover progress (updated 2026-08-02)

The branch-level cutover now has a working end-to-end spine:

- the concrete `Agent` now exposes exactly four methods and zero exported
  fields; `Session` exposes exactly five methods, and both `Close` methods
  return `error`;
- package-level exported functions fell from 148 to 45. AST golden tests pin
  every exported name plus the reviewed 14-function Agent facade and reject
  `ForTesting` backdoors;
- the builder no longer writes runtime state into public `Agent` fields. It
  supplies provider keys, prompt labels, workspace paths, session handles, and
  per-turn streaming callbacks through typed construction/turn inputs;
- read-only state needed by callers is exposed through immutable definition,
  diagnostics, runtime-info, and opaque session-handle snapshots rather than
  mutable fields;
- the full Go suites pass in both repositories, including
  `agent_go/cmd/family-server`;
- direct and MCP tools enter one canonical name-keyed registry;
- request-time manifests and `get_api_spec` resolve from that registry;
- conflicting implementation owners fail before replacing registry state;
- `AgentDefinition` is validated and deeply cloned before runtime creation;
- the reusable `agentsession` path and the main orchestrator assemble direct
  tools and MCP sources before constructing the agent;
- attached skills are now readable through the intrinsic, reserved
  `read_skill` tool on API and coding-agent transports; CLI filesystem
  projection remains an optimization, so background/stage-agent isolation no
  longer changes the skill-reading contract;
- `BaseAgent` executes through `Run(Turn)` rather than choosing among ask and
  continuation methods itself;
- the reusable `agentsession` adapter and the legacy `agentwrapper` execution
  paths now also run through `Session.Run`/`Agent.Run`, with continuation,
  updated history, usage, and cancellation results returned together;
- `Turn.ToolPolicy` is request-scoped and controls the rendered manifest,
  schema discovery, and session HTTP execution guard without changing identity;
- `Session` is pinned to exactly five methods: `Run`, `Send`, `Snapshot`,
  `Events`, and `Close`; and
- structured `Result.Usage` now includes token counts, cost components, and
  context utilization, removing the wrapper's need to query pricing state; and
- provider-native continuation is now supplied through `RuntimeConfig`, and
  workflow recovery/identity changes rebuild an agent around the opaque handle
  rather than mutating the live instance;
- workflow supplementary skills and prompt sections are applied by creating a
  replacement immutable definition before the turn, while preserving the MCP
  session and provider continuation handle; and
- the main chat and delegation wrappers now finalize their incrementally
  assembled legacy drafts into one immutable definition before the first turn;
  the freeze includes static prompt supplements, cloned skill definitions,
  direct tool executors/schemas, and observers, while replacement retirement
  preserves shared tracers and MCP/provider state;
- chat and delegation prompt/skill/tool assembly now builds a private
  `AgentDefinition` draft and constructs one immutable Agent at finalization;
  runtime `Agent` access is retained only for diagnostics, steering,
  continuation, and explicit replacement retirement;
- the gRPC adapter now owns an explicit `Session` and returns response, history,
  usage, and costs from `Result` instead of calling legacy `Ask*`, token getters,
  or raw tool maps; and
- the concrete `Agent` surface is now the final four methods: `Start`, `Run`,
  `Definition`, and `Close`; the golden test pins that exact list;
- workflow, chat, delegation, and gRPC tool factories assemble direct
  `AgentDefinition` values before the first-turn boundary and reject later
  identity changes;
- workflow and chat permissions are passed as `Turn.ToolPolicy`, while folder
  guards live in `RuntimeConfig` rather than mutable agent identity;
- workflow steering now goes through the active `Session`, whose delivery path
  remains available while a serialized `Run` is in flight; and
- gRPC custom tools are constructed into the immutable definition. Their stable
  executors proxy to the currently bound stream callback, so a new conversation
  changes runtime routing without replacing tool schemas or mutating the Agent;
- cleanup ticker shutdown now detaches lifecycle state under a mutex and uses
  goroutine-local channels, removing the construction/close race exposed by the
  new definition tests.

The duplicate construction paths are gone: no `LegacyOptions`, exported
`With*`, `DefinitionAssembly`, public legacy constructor, or legacy Tool Search
mode remains. The remaining 14 Agent-related package functions are exact-listed
because active production code uses them for context maintenance, diagnostics,
resume, steering, transport startup, replacement, and the one manual
virtual-tool test boundary. They are no longer an unbounded compatibility
surface.

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

## Canonical registry follow-up: complete

**Status: implemented and verified (2026-08-02).** The audit found that the
observation was real architectural debt, even though no production divergence
had yet been captured. `Agent.customTools` contained no information absent from
`registeredTool`: definition, executor, display group, and timeout all existed
in both records.

The two stores could silently disagree because registration wrote them in
separate operations. The timeout path was especially weak: it updated
`customTools` first and discarded any error while updating `toolRegistry`.
Meanwhile discovery, bridge schema lookup, serial execution, parallel execution,
skill-name collision checks, and timeout selection mostly read the projection.
The structure therefore made the object documented as canonical the minority
source in practice.

The follow-up removed `CustomTool` and `Agent.customTools` completely. The
canonical registry is now the sole Agent-side record for direct-tool identity,
schema, executor, display metadata, and timeout. All of these consumers read it:

- prompt and OpenAPI discovery;
- bridge MCP schema construction;
- serial and parallel direct-tool execution;
- per-tool timeout selection;
- direct-tool category/display-group enumeration;
- attached-skill reserved-name checks; and
- construction of the code-execution executor projection.

Registration now writes one complete canonical record, including timeout, in one
operation. Re-registering the same direct tool in a different display group is
rejected inside the locked registry as well as at the caller boundary, so two
concurrent registrations cannot bypass the invariant.

`agent/codeexec.ToolRegistry` still contains executor maps. Those are deliberate
session-scoped runtime projections used by the HTTP bridge: they contain only
callable functions, are rebuilt from the canonical snapshot, and cannot answer
identity, schema, category, or timeout questions. They are not a second
Agent-side tool registry.

Regression coverage now asserts that a timeout-bearing direct tool produces one
complete canonical record and that the executor projection is derived from that
record. `go test ./...`, `go test -race ./agent -count=1`, and `go vet ./agent`
pass in `mcpagent`; `go test ./agent_go/... -count=1` passes in the builder. The
exported API ratchets are unchanged: four `Agent` methods, zero exported `Agent`
fields, and 45 package functions.

Two related comments remain intentionally out of scope because their state is
still live: the legacy per-provider session ID fields alongside
`codingProviderSessionHandle`, and supported same-display-group re-registration
used to refresh session-aware executors.

### Session-scoped permissions after the projection change: verified

Removing `Agent.customTools` changed where the code-execution executor map comes
from, so the session permission model was re-traced end to end rather than
assumed. It is intact. There are two enforcement surfaces and both receive the
same policy from one place:

```text
Turn.ToolPolicy
  → normalizeToolPolicy
  → ctx turnPolicyContextKey  → isToolAllowedForContext   (in-process calls)
  → codeexec.SetSessionToolAllowList(sessionID, allowed)  (HTTP bridge calls)
```

`Session.Run` (`turn_session.go:189`) writes both on every turn, so a
code-executing agent cannot escape a per-turn policy by reaching a tool over the
HTTP bridge instead of calling it directly.

Three properties were confirmed in code, not inferred:

1. **Executor refresh still works.** `canonicalToolRegistry.register` overwrites
   an existing record when kind, source, and display group match. The builder
   paths that re-register a tool to swap in a session-aware executor therefore
   still take effect; the projection is rebuilt from the updated record.
2. **Session registration happens on both paths** — at construction
   (`agent.go:1866`) and on re-registration (`agent.go:3391`), each guarded by a
   non-empty `sessionID`.
3. **A session registry does not fall through to global.** Once
   `sessionCustomTools[sessionID]` exists, a missing tool is an error rather than
   a global lookup, so one workflow cannot borrow another's executor. Only a
   session with no registry at all uses the legacy global path.

**One real gap was found and closed.** The HTTP-bridge allow-list gate in
`CallCustomToolWithSession` — the half of the model that enforces `ToolPolicy`
over the bridge — had no test anywhere in either repository. The in-process half
was covered (`agent/session_policy_test.go`, `skill_reader_test.go`) and
cross-session executor isolation was covered
(`TestCallCustomToolWithSessionDoesNotBorrowGlobalExecutor`), but nothing
exercised the allow list itself. It could have stopped enforcing while every
existing test stayed green.

Four tests were added to `agent/codeexec/registry_test.go`:

- a tool outside the allow list is rejected and its executor never runs;
- a tool inside the allow list executes;
- a nil allow list means unrestricted, not blocked — `Session.Run` passes nil
  whenever `ToolPolicy.AllowedTools` is empty, so inverting this would break
  every unrestricted turn rather than fail closed on one;
- one session's allow list does not gate another's, since concurrent workflows
  share one process-wide registry.

They were mutation-checked: disabling the gate in `CallCustomToolWithSession`
fails exactly the two enforcement tests and leaves the two permissive ones
passing, which is the expected signature. `go test ./...`,
`go test -race ./agent ./agent/codeexec`, and `go vet` pass in `mcpagent`, and
`go test ./... -count=1` passes in `agent_go`.

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
