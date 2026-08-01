# Bug Report: Custom-Tool Categories Leak Into the Agent Contract as Addresses

## Status

Immediate addressing failure contained 2026-08-01 in
`mcpagent/agent/code_execution_tools.go`
(`get_api_spec` now resolves by tool name regardless of `server_name`), across
two rounds: the first pass missed the case where `server_name` names a valid
category that does not hold the requested tool. The long-term tool-name-only
design is endorsed below, but the migration is not complete. A later Pulse
Fixer run exposed a second surface of the same architectural problem: a tool can
be correctly registered and callable while still being absent from the prompt
the agent actually receives.

These failures were invisible before 2026-08-01 because a bridge tool error
arrives as stdout with `exit_code: 0`; the UI reported them as successes. See
"How often this actually fires".

A coverage audit on 2026-08-01 found the original addressing containment broad,
but the later prompt-delivery failure disproved the stronger claim that the
agent-facing tool contract was complete. Tool-name uniqueness is detected but
not yet enforced. See "Coverage audit" and "Why this now warrants a refactor".

Code referenced here lives in `manishiitg/mcpagent`, not this repository.

## How often this actually fires

Scanning one day of codex rollouts (2026-08-01) for the harness tool-failure
envelope, before any of these fixes were live:

```text
46  virtual_tool_handler   get_api_spec      ← 18 "server …", 12+ "not found in category …"
14  custom_tool_handler    mark_pulse_module_result
 8  custom_tool_handler    get_pulse_review_result
 6  custom_tool_handler    execute_shell_command
 4  custom_tool_handler    mark_human_input_consumed
```

`get_api_spec` is the single largest source of failed tool calls in the system,
and effectively all of it is this bug. None of it was visible: a tool that fails
behind the HTTP bridge returns its error as stdout with `exit_code: 0`, so the
UI rendered every one with a green check until
`frontend/src/utils/toolCallFormatting.ts` learned to recognise the envelope.
This is why the defect survived so long — it was expensive and silent.

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

### The second form: a valid category that does not hold the tool

The same agent, an hour later, having learned that `custom` was rejected, walked
the list it had been handed:

```json
get_api_spec({"server_name": "auto_improvement", "tool_name": "get_pulse_module_state"})
```

```text
ERROR: tool(s) [get_pulse_module_state] not found in category "auto_improvement".
Available tools in "auto_improvement": [get_reference_doc get_workflow_command_guidance]
```

Six consecutive failures — `get_pulse_module_state`,
`get_pulse_finding_backlog`, `get_pulse_review_result`, `report_human_inputs`,
`start_pulse_fix_attempt`, `mark_pulse_module_result` — each naming a tool that
exists, is permitted, and is uniquely identified by the name already supplied.

This form matters because it is what the error message *invites*. Told "custom is
not available, here are the categories", the only available strategy is to guess
among them, and each guess costs a call. Nothing in the response says which
category holds the tool the agent asked for, though the harness knows exactly.

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
`server_name`, generalising the existing tool-name fallback.

The first attempt at this fix was incomplete, and the second failure form is why.
It guarded the fallback with `!isCustomCategory`, which handles a `server_name`
that is not a category at all — but a *valid* category that simply lacks the tool
leaves `isCustomCategory` true, skipping the fallback entirely. The resolution is
therefore computed unconditionally:

```go
// Computed unconditionally, because server_name can be wrong in two ways and
// both must resolve. It can be a name that is not a category at all
// ("custom"), or a name that is a perfectly valid category which simply does
// not hold the requested tool.
```

Worth recording as its own lesson: the natural instinct is to treat "unknown
server" as the error case, because that is the message the operator sees. The
actual invariant is narrower — *whenever the requested tool names resolve, the
request is answerable* — and any fix expressed in terms of what `server_name`
looks like will keep missing cases.

Deliberately keyed on tool names, not on the literal `"custom"`:

- tool names are unique across custom tools, so it cannot answer for the wrong
  tool;
- `$MCP_CUSTOM` is category-free, so one request may legitimately span
  `workflow` and `human_tools` — splitting it would expose the internal grouping
  the prompt told the agent to ignore;
- an unknown tool name still errors, rather than being quietly absorbed.

Tests in `agent/get_api_spec_custom_server_test.go` cover the reported call, the
valid-but-wrong category, a cross-category request, and the unknown-tool guard.
Verified honest: each fix was reverted in turn and the matching test fails,
reproducing the original error text.

The change is only live where `mcpagent` is a local checkout. `agent_go/go.mod`
had no `replace` for it, so the first fix was never compiled into the server that
was failing; one was added alongside the existing `multi-llm-provider-go` entry,
and both should be dropped when the modules are tagged.

### Two gaps the long-term design surfaced, now closed in the patch

The design section below identified two defects in the containment itself. Both
were verified against the code and fixed rather than deferred.

**Authorization was not part of resolution.** `isToolAllowed` guards the tool
index (`code_execution_tools.go:459`) but never guarded `handleGetAPISpec`, which
reads `a.customTools` directly. Name-based resolution therefore widened an
existing hole: before, an agent had to name the right category to obtain a spec;
after, any known tool name resolved — including tools deliberately withheld from
that session. `SetToolAllowList` is precisely how a Pulse Fixer stage is bounded,
so this is not hypothetical.

Severity is disclosure, not escalation: the allow list also gates the
code-execution registry, so a leaked spec cannot be called. But the agent now
discovers a tool, attempts it, and fails — the exact wasted-call cost this report
exists to remove. `isToolAllowed` is now applied during resolution.

**Resolution was not atomic.** A request for four tools where three matched
returned a spec for three and silently dropped the fourth. The agent reads that
as success and discovers the omission only when it calls the missing tool. A
partly resolvable request now fails as one unit and names what could not be
resolved.

Both have regression tests, each verified by reverting the fix and observing the
matching failure.

## Coverage audit (2026-08-01)

Checked every place a tool is addressed, to establish whether the containment is
complete or whether other surfaces carry the same defect.

| Surface | Addressing | Status |
|---|---|---|
| `get_api_spec` | `server_name` + tool names | **was broken** — four defects, all fixed above |
| `get_prompt`, `get_resource` | `server` | not affected, and near-dead — see below |
| Execution via `$MCP_CUSTOM/{tool_name}` | tool name | correct already — category-free, and the contract discovery disagreed with |
| Tool index in the system prompt | grouped by category | not affected — display only, and it already applies `isToolAllowed` |
| `RegisterCustomTool` | tool name | **gap — see below** |

The audit's useful negative result: execution was never wrong. `$MCP_CUSTOM` has
always addressed tools by name alone. Discovery was the only surface that
demanded a second address, which is why the failure looked like a model error —
the same agent could *call* a tool it could not *look up*.

### How execution actually resolves a tool

Traced 2026-08-01, because it settles what an address needs to be:

```text
codex  →  curl $MCP_CUSTOM/{tool_name} -H "$MCP_AUTH"
       →  executor/handlers.go            (req.Tool, req.Args, req.SessionID)
       →  codeexec.CallCustomToolWithSession(ctx, sessionID, tool, args)
       →  agent/codeexec/registry.go:710
```

`session_id` travels on the request and a process-global registry resolves
everything from it, through two independent session-keyed maps:

**Authorization — `sessionToolAllowLists[sessionID]`, checked first.** A tool
outside the session's allow list is refused before resolution. This is the
enforcement half of `SetToolAllowList`, and it is why the spec-disclosure gap
fixed above was disclosure rather than escalation: a leaked spec still cannot be
called.

**Resolution — `sessionCustomTools[sessionID][toolName]`,** with a deliberate
three-way outcome:

```text
session registry exists, tool present  →  execute
session registry exists, tool absent   →  hard error, no fallback
session registry absent                →  fall through to the global tool map
```

The middle case is the cross-workflow contamination guard: once a session
declares its tools, an unknown name fails rather than silently reaching another
workflow's tool.

**The key point for this report:** the execution registry is keyed
`sessionID → toolName → executor`. There is no category and no server name
anywhere in it. Execution has always addressed tools exactly the way the
long-term design proposes for discovery, which is why `$MCP_CUSTOM/{tool_name}`
works with no category segment and why the two halves of the contract disagreed.

### `get_prompt` and `get_resource`: keep them, stop asserting they exist

Both resolve `a.Clients` — real MCP servers — so neither carries the addressing
defect. They are also, in practice, never used:

```go
hasMCPServers = len(a.Clients) > 0        // false whenever ServerNames = [NO_SERVERS]
if hasPrompts   { register get_prompt }    // only if a server advertises prompts
if hasResources { register get_resource }  // only if a server advertises resources
```

Every Pulse stage, reviewer, fixer, and background agent runs with `NoServers`,
so neither tool is ever created for them. For agents that do get servers, the 16
configured here (Notion, the AWS suite, context7, gmail, google-docs/sheets,
Parallel Search) are tool-only; prompts and resources are the least-implemented
parts of MCP. No call to either appears in months of codex transcripts.

**Recommendation: do not remove them.** The conditional registration is correct
design — an absent capability costs nothing because the tool is never created,
and deleting a spec-compliant MCP feature would break the day someone connects a
server that does advertise prompts or resources. This is dead weight only in the
sense that a correctly-closed valve is dead weight.

What should change is the places that assert they exist **unconditionally**,
because those are live and wrong. See the next section.

### The wrong-tool recovery message names tools that may not exist

`agent/parallel_tool_execution.go:330` answers any call to an unknown tool with a
hardcoded list:

```text
❌ Tool '%s' is not available in this system.
🔧 Available tools include:
- get_prompt, get_resource (virtual tools)
- search_large_output (read/search/query operations for offloaded files)
- MCP server tools (check system prompt for full list)
```

None of it is derived from what the agent actually has. For every `NoServers`
agent — which is every Pulse agent — `get_prompt` and `get_resource` are not
registered, so the correction offers tools that will fail if taken up.

This is the third instance today of one pattern, and the pattern is the finding:

| Error | Recovery hint it gave | Why it was wrong |
|---|---|---|
| `get_api_spec` server rejected | listed the categories | the prompt had said categories are not addresses |
| SQLite error 14 | "unable to open database file" | the file was fine; `-shm` could not be created |
| unknown tool called | "available tools include get_prompt…" | not registered for this agent |

An agent that gets a bad error message does not stop — it acts on the hint. Each
of these turned one failed call into several. The recovery text is not
documentation; it is an instruction the model will follow, and it should be held
to the same standard of truth as the tool result itself.

**Fixed 2026-08-01.** `unknownToolFeedback` now builds the list from
`a.filteredTools` filtered through `isToolAllowed`, so the correction names what
this agent actually has and never offers a denied tool as the way out. Two
details worth keeping:

- An agent with nothing callable is told so explicitly and instructed not to
  retry, rather than being handed an empty list it would read as "try again".
- The list is bounded at 60 names with a count of the remainder; a full surface
  runs to hundreds, and an unreadable wall of text is its own bad hint.

Three regression tests cover unregistered virtual tools, allow-list respect, and
the tool-less agent.

This was the recovery path for *every* wrong tool call, not only these two, so it
is worth more than the specific bug that exposed it.

### The gap: tool-name uniqueness is not enforced

`agent.go:4033` registers with a plain map assignment:

```go
a.customTools[name] = CustomTool{...}   // silently overwrites
```

A second registration under the same name replaces the first, and the earlier
tool becomes unreachable with no error, no log, and no startup failure. Tool
names are therefore unique *by accident*, not by construction.

This matters more now than it did yesterday. Both the containment above and the
long-term design below resolve by name, so uniqueness has become load-bearing —
an invariant everything depends on and nothing checks.

Detection was added as a loud error-level log rather than a hard failure, and
deliberately so: whether duplicates exist today is not statically decidable,
because most registrations come from tool-definition lists rather than string
literals. Failing startup on an unprovable assumption would trade a silent bug
for an outage. Promote it to a rejected registration once the log has been quiet
in production — which is also the evidence the long-term design needs before it
can rely on name uniqueness.

The same weakness exists one layer down: the execution registry stores
`sessionCustomTools[sessionID]` as a name-keyed map, so a duplicate *within one
session* silently overwrites there too. Scope matters when reading the new log —
uniqueness is required per session, not process-wide, and different sessions
reusing a name is correct rather than a collision.

## Why this now warrants a refactor

The category failure was initially understandable as one bad argument in
`get_api_spec`. The later database-tool incident shows that the deeper problem
is duplicated, lifecycle-dependent tool state.

A standalone social-media Pulse Fixer received a 10,476-character custom system
prompt with no `<available_tools>` block, no `workflow_db`, and no
`query_workflow_db`. The tool was registered and callable — `get_api_spec`
eventually reported `[mutate_workflow_db query_workflow_db]` — but nothing in the
prompt named it. The agent therefore:

1. tried direct SQLite and was correctly denied by the database guard;
2. retried with `immutable=1`;
3. guessed six nonexistent tool names; and
4. found `query_workflow_db` only because an error finally listed the category's
   real tools.

This was reasonable recovery from an incomplete contract, not evidence that the
model could not follow instructions. The harness told it *how* to call a custom
tool but omitted *which* custom tools existed.

### The same fact is copied into too many mutable places

Whether a tool exists and is usable is currently represented across:

- `customTools`;
- `Tools` and `filteredTools`;
- `toolToServer`;
- custom/system category maps;
- the code-execution registry and its session registry;
- the session allow list;
- the OpenAPI-spec cache; and
- a serialized copy embedded in `systemPrompt`.

Those copies are updated by different operations. Registration rebuilds the
prompt; a later allow-list change can narrow the session; a custom
`SetSystemPrompt` can overwrite the rebuilt inventory; and a cached spec can
outlive the policy under which it was generated. Correctness therefore depends
on call order rather than on one invariant.

The observed background-agent order was:

```text
register tools
  -> rebuild a prompt containing the inventory
  -> apply the stage allow list
  -> SetSystemPrompt(background template)
  -> erase the inventory
  -> launch the agent
```

Adding another branch to `rebuildSystemPromptWithUpdatedToolStructure` does not
fix that order: the rebuild already happened before the custom prompt overwrite.
It also leaves every future template responsible for remembering a placeholder.
That is the same opt-in correctness failure as category addressing in a new
form.

### The invariant the refactor must establish

At the moment an LLM request is sent, derive one immutable view:

```text
effective_tools = canonical_registry intersect current_session_policy
```

Use that exact view for all five consumers:

1. the compact tool manifest appended to the outgoing prompt;
2. `get_api_spec` resolution;
3. execution authorization and routing;
4. unknown-tool recovery messages; and
5. observability/debug output.

The prompt must not be a mutable store of tool state. `SetSystemPrompt` should
own instructions only. Tool registration should not rebuild prompts, and
allow-list changes should not edit prompt text. The final transport layer should
render the current manifest immediately before sending the request, after all
registration and policy decisions are complete.

This is why the refactor is now justified. The current code does not merely have
several complicated branches; it maintains several independently stale copies
of the same authorization and discovery fact. Patching each lifecycle order is
more work, and less reliable, than removing the duplicated state.

### Review of the refactor case: endorsed, with four additions

The diagnosis above is correct and is the right altitude. In particular *"adding
another branch to `rebuildSystemPromptWithUpdatedToolStructure` does not fix that
order"* is exactly right — that branch was written, its tests passed, and they
passed because they set the prompt first and rebuilt second, which is the reverse
of production. It was reverted. A fix whose test does not reproduce the live
lifecycle order proves nothing here.

**1. The strongest evidence predates today and is missing above.**
`ClearAppendedSystemPrompts` documents a named production bug from this same root
cause:

> deliberately does NOT rewrite `a.systemPrompt` — that is left to a following
> `SetSystemPrompt` re-base. If a caller clears here and then re-appends WITHOUT
> a re-base, the materialized `a.systemPrompt` keeps its old appended blocks and
> grows turn over turn (the CLAUDE.md 14x-bloat bug).

A prompt that grew 14× across turns, from a materialised copy drifting out of
sync with its inputs. The current defence is a *convention* — callers must
re-base in the right order — which is the same class of guarantee that failed
here. This is three symptoms of one missing invariant, not one bug: the 14×
bloat, the erased tool inventory, and the stale allow list.

**2. The spec-cache point is right, and understates itself.** The cache key is
`serverName + ":" + sortedToolNames` with no policy component, and the only
invalidation is `delete(a.openAPISpecCache, toolCategory)` on registration —
keyed by *category* while entries are keyed by `server:tools`, so it cannot match
a real key. `SetToolAllowList` does not touch the cache at all. The cache is
therefore effectively never invalidated by policy change, not merely at risk of
outliving it.

**3. The migration is smaller than "refactor" suggests, but the proposed
one-function leverage point is not real.** Measured across all three
repositories:

```text
mcpagent/agent/agent.go          125 refs (17 writes)
mcpagent/agent/conversation.go    20
mcpagent/agent/llm_generation.go   7
mcpagent/agent/prompt/*            6      (the renderer)
mcpagent/mcpcache/*               15      NOT a risk — its SystemPrompt field is a
                                          cache-result field, explicitly left empty
                                          ("agent will get proper system prompt
                                          from agent creation")
mcp-agent-builder-go               5 callers, all via GetSystemPrompt()
multi-llm-provider-go              0
```

`GetSystemPrompt()` (`agent.go:4635`) is the only exported read path, but it is
**not** the model send path. `conversation.go` constructs the system message from
`a.systemPrompt` directly, and coding-agent launch and continuation paths in
`llm_generation.go` also project `a.systemPrompt` directly. Composing only in
`GetSystemPrompt()` would make builder logs and exported callers look fixed while
the model could still receive the stale prompt — a worse observability split
than the original bug.

The useful leverage point is therefore one shared `effectiveSystemPrompt`
composer called by every outbound boundary and by `GetSystemPrompt()`: normal
conversation assembly, coding-agent launch, coding-agent continuation, prompt
events/logging, and exported reads. The raw stored instruction text may remain
internal during migration, but no send or verification path may read it
directly.

Concrete deletions the refactor should produce, as its acceptance criteria:
`rebuildSystemPromptWithUpdatedToolStructure` gone, the second tool-index
renderer gone (two exist today, at `agent.go:3814` untagged and `:4721` tagged,
which is why extracting by marker produces unbalanced `<available_tools>` tags),
and `{{TOOL_STRUCTURE}}` reduced to a compatibility no-op.

**4. Where the surprise will be.** `ClearAppendedSystemPrompts`' documented
ordering convention must be *unwound*, not preserved — under derivation its
warning becomes obsolete, but any caller relying on the current materialised
behaviour needs checking. That, and the internal reads in `agent.go` that assume
"this is the exact string last set", are the parts to review closely. The
composition itself is the easy half.

## Long-term design: the tool name is the only address

**Reviewed and endorsed 2026-08-01.** This is the right target, and it is
stronger than the containment above in two ways worth calling out, because both
found real defects that the patch had missed and has now fixed: it treats
authorization as part of resolution, and it requires resolution to be atomic.
The one caution is sequencing — steps 1–2 of the migration are safe now, while
step 5 (removing `server_name`) should wait for the deprecation metric in step 2
to actually read zero, not merely for the other steps to be done.

The current patch is containment. The simpler permanent contract is:

```json
get_api_spec({
  "tool_names": ["get_pulse_module_state", "mark_pulse_module_result"]
})
```

Remove `server_name` from the agent-facing API. The agent already supplies a
globally unique tool name; requiring it to also supply the implementation's MCP
server or custom-tool category introduces a second, fallible address for the
same thing.

Maintain one canonical tool registry internally:

```text
tool name
  -> implementation kind: custom | MCP
  -> internal server and optional category
  -> input schema
  -> authorization requirements
  -> execution handler
```

The registry, not the model, decides whether a call goes to a custom handler or
a real MCP server. Tool registration must fail at startup if two implementations
claim the same name. If globally unique names ever become impossible, introduce
an opaque canonical tool ID; do not make a display category the disambiguator.

The registry is the durable source of tool metadata; the effective session view
is derived data. Do not persist that derived view independently in `Tools`,
`filteredTools`, a prompt fragment, and an authorization-sensitive cache. Legacy
fields may remain during migration, but they must be projections of the
registry, not peer sources of truth.

Prompt assembly should therefore be:

```text
base/custom instructions
  + bridge usage instructions
  + render(effective_tools for this request)
```

Only compact names and optional display groups belong in the prompt. Full
schemas remain lazy through `get_api_spec`, so making discovery truthful does
not require placing hundreds of tool definitions in context.

### Categories become configuration bundles, not runtime policy keys

Categories remain useful for configuration and display grouping. For example,
an agent may be configured with `allow_categories: ["workflow"]`, and the tool
index may visually group workflow tools. But at session creation the harness
must compile all category rules, explicit allow/deny rules, and capabilities
into one concrete `allowed_tools` set.

Discovery and execution then authorize against that same set. `Category` does
not participate in runtime resolution and is never presented as an address.
This avoids the current split where the tool index respects `isToolAllowed` but
`get_api_spec` can inspect `a.customTools` directly.

### Resolution is atomic

Before returning any specification, resolve and authorize every requested name:

1. Every name must exist in the canonical registry.
2. Every name must be in the session's compiled `allowed_tools` set.
3. If any name fails either check, reject the whole request and report exact
   `unknown` and `not_allowed` lists.
4. Only after validation succeeds should the harness generate all requested
   specs and route each tool through its registered implementation.

Do not return a partial spec. A valid-category request containing one known and
one unknown tool can currently return only the known tool, silently dropping the
unknown name; all-or-nothing validation closes that gap.

Example structured failure:

```json
{
  "error": "tools_unavailable",
  "unknown": ["no_such_tool"],
  "not_allowed": ["mark_human_input_consumed"]
}
```

### Compatibility migration

1. Add name-only resolution behind the existing `get_api_spec` tool.
2. Temporarily accept `server_name`, ignore it for custom-tool resolution, and
   record a deprecation metric. It must never narrow or broaden authorization.
3. Update the tool index, bridge prompt, examples, and error recovery guidance
   to request specs by tool name only. Categories may remain visual labels, with
   an explicit statement that they are not addresses.
4. Keep existing execution URLs during migration; changing the agent contract
   does not require changing every bridge endpoint at once.
5. Remove `server_name` after deployed clients no longer send it.

Until that migration is complete, resolving known custom tools regardless of a
stale or wrong `server_name` is the correct compatibility behavior, but it
should not become the permanent API.

### Step 1 done: `server_name` is now optional

The schema declared `"required": ["server_name", "tool_name"]`, which is the
actual cause of the guessing. Every fix above makes a *wrong* value resolve;
requiring the field is what made the model produce one. Built-in tools are
addressed as `$MCP_CUSTOM/{tool_name}` with no category segment, so an agent
asked for a "server" had nothing correct to say and had to guess — 46 failed
calls in a day, walking the category list one rejection at a time.

`server_name` is now optional and documented as disambiguation for real MCP
servers only, with `tool_name` alone sufficient. Existing callers are unaffected;
the field is still accepted. `tool_name` remains required, so the call cannot be
made with no address at all.

This is migration step 1, and it is the piece that stops new occurrences rather
than tolerating them. Everything after it is cleanup of a contract that no longer
forces the mistake.

### Recommended sequencing: refactor in stages, not as a big bang

The database-tool incident invalidated the earlier conclusion that containment
covered every observed failure. The refactor should now be the next focused
`mcpagent` architecture change, but it should be staged so routing,
authorization, and prompt changes are independently testable.

1. **Establish a truthful request-time manifest.** Derive allowed tools after
   registration and allow-list application, and append one balanced,
   non-duplicated `<available_tools>` block at outbound request assembly. Add a
   real background-agent regression test matching the production lifecycle
   order. Do not treat another registration-time prompt rebuild as completion.
2. **Introduce the canonical registry behind existing behavior.** Route the
   current index, `get_api_spec`, execution registry, and error recovery through
   the same lookup without changing public URLs.
3. **Make session policy a compiled tool-name set.** Discovery and execution
   consume the same immutable effective view. Schema caches remain
   authorization-independent and are consulted only after per-request policy
   validation.
4. **Complete name-only discovery compatibility.** Keep accepting
   `server_name`, but make it incapable of adding, removing, authorizing, or
   changing the cache identity of a requested tool. Record its use.
5. **Remove legacy state and `server_name` only after evidence.** Delete prompt
   rebuild branches and peer tool lists once all consumers use the registry;
   remove the compatibility field only after its metric reads zero.

Before the staged refactor lands, the small containment fixes and production
restart remain worthwhile. They reduce current failures and provide a stable
baseline. They are not a reason to postpone the refactor indefinitely, and a
passing unit test that calls rebuild *after* setting a custom prompt is not proof
for a production path that performs those operations in the opposite order.

### Implementation progress (2026-08-01)

Stage 1 and the agent-facing part of stage 4 are now implemented in `mcpagent`:

- one request-time composer renders the current allowed tool manifest;
- normal conversation sends, coding-agent launch/continuation, exported prompt
  reads, prompt events, and token/debug accounting use that same outbound text;
- `SetSystemPrompt` stores instructions only and tool registration no longer
  rebuilds prompt text;
- supplementary prompts are stored separately and composed at send time, so
  `ClearAppendedSystemPrompts` takes effect without a later re-base and cannot
  reproduce the 14x materialisation-growth bug;
- the duplicate tool-index renderers and
  `rebuildSystemPromptWithUpdatedToolStructure` are removed;
- manifests and pre-discovered schemas both apply the current tool allow list;
- `get_api_spec` resolves both custom and real MCP tools by tool name, treats
  `server_name` as compatibility-only input, validates every requested name
  atomically, and authorizes before consulting its schema cache; and
- registration clears the schema cache when a definition changes. Policy
  changes do not need to invalidate it because every cache hit is now behind a
  fresh authorization check.

Regression coverage follows the production lifecycle order: set the prompt,
register tools, change the allow list, then assemble the outbound request. It
also covers balanced/idempotent manifest rendering, pre-discovered-spec policy,
cache-before-authorization bypass, name-only MCP/custom lookup, wrong legacy
`server_name`, and clearing supplements without re-basing.

This does **not** claim stages 2–3 are complete. `customTools`, `toolToServer`,
`Tools`, and the code-execution registries still exist as peer internal stores;
duplicate names are logged but do not yet fail registry construction. Moving
those consumers behind one canonical registry remains the next architecture
stage and should be done separately from this verified outbound-contract change.

### Public API consolidation (2026-08-01)

The request-time design is now reflected in the API used by
`mcp-agent-builder-go`. Ten independently callable prompt lifecycle methods were
replaced by four operations with explicit semantics:

```text
SetInstructions(base)                 replace base; preserve supplements
AddInstructions(supplements...)       add idempotent supplements
ResetInstructions(base, extras...)    atomically replace the whole instruction set
Instructions()                        return the exact rendered outbound text
```

The builder no longer reads the supplement list, manually concatenates prompt
fragments, calls a separate placeholder resolver, or performs clear-then-set
sequences. Event/debug output reads `Instructions()`, so inspection and sending
share the same renderer. Phase changes use `ResetInstructions` as one operation.

Tool policy is similarly one operation:

```text
SetToolAccess(names)   non-empty = exact allow set; nil/empty = all registered tools
```

`SetToolAllowList`, `ClearToolAllowList`, and the public
`UpdateCodeExecutionRegistry` lifecycle hook are gone. Registration already
updates execution routing, while `SetToolAccess` updates the session execution
guard. The builder's five explicit registry-refresh calls and their ordering
comments were removed. Folder guards remain live agent state and never required
a registry rebuild.

This pass reduced exported `Agent` methods from 84 to 70, including making six
event/rendering helpers package-private. It intentionally did not add deprecated
aliases: keeping both APIs would preserve the invalid lifecycle combinations the
change is meant to remove. The next reduction should consolidate the current
positional `RegisterCustomTool` family into one structured registration API when
the canonical registry stage lands.

## Acceptance

- `get_api_spec` accepts tool names without requiring an agent-visible server or
  category address.
- Every requested name is validated before any spec is returned. Mixed
  known/unknown and allowed/disallowed requests fail atomically and name every
  unavailable tool.
- Discovery and execution use the same compiled session `allowed_tools` set; a
  tool hidden from the index cannot be recovered by guessing its name.
- Requests containing custom tools from multiple display categories succeed.
- During compatibility, resolvable custom tool names succeed with `custom`, the
  right category, a *wrong but valid* category, a tool name, or a stale
  `server_name`. The wrong-but-valid case must have its own regression test.
- Real MCP tools continue to resolve and execute through their registered MCP
  servers without requiring the model to know that routing detail.
- Duplicate tool names fail during registry construction rather than becoming
  ambiguous at runtime. Until then, a duplicate registration is logged at error
  level — and that log reading zero in production is the precondition for
  depending on name uniqueness at all.
- Prompt text, the tool index, `get_api_spec`, authorization, and execution all
  agree that the tool name is the address.
- The tool manifest is derived at request time from the current registry and
  session policy; overwriting a custom system prompt cannot erase it.
- Applying or clearing an allow list changes the next request's manifest without
  registration-time prompt mutation, stale entries, or duplicate tool sections.

## Adjacent observation, not part of this bug

Session isolation in the execution registry fails open. When a session has no
entry in `sessionCustomTools`, the lookup falls through to the un-scoped global
tool map rather than denying the call:

```text
session registry absent  →  fall through to the global tool map
```

The intent is documented and reasonable — non-workflow callers never register
session-scoped tools and would otherwise break. But it means the isolation
guarantee holds only for sessions that registered something, and an *absent*
registry is read as "not a workflow" rather than "deny". A session that failed to
register, registered late, or lost its entry gets the global surface instead of
an error.

Nothing observed depends on this, and it is unrelated to addressing. It is
recorded here only because it was found while tracing that path, and because it
is the kind of default that reads as safe without being safe. Worth a deliberate
decision if session scoping is ever treated as a security boundary rather than a
contamination guard.

## Related

`docs/bugs/pulse_fixer_sqlite_readonly_wal_and_schema_guessing.md` — same class:
an agent burning tool calls rediscovering a harness contract that was never
stated in terms it was given.
