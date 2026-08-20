# Direct API Transport vs. Routing Through Pi/MCP

## Status: decided 2026-08-20 — direct API transport deprecated in the UI

Five `api_model` providers (openai, anthropic, vertex, bedrock, azure) are marked
`deprecated: true` in `agent_go/cmd/server/llm_provider_manifest.go` and hidden
from the provider catalog, the LLM config modal, and the workflow LLM picker.
Existing saved configurations keep working — this is a soft deprecation, not a
removal. `minimax` is unaffected: in this manifest it is `integration_kind:
"audio_provider"` (speech/music), not one of these five text-LLM providers.

This document exists so the next person does not have to re-derive the
reasoning. It was argued out in a live conversation on 2026-08-20, pressure-
tested round by round rather than decided up front, and each round is recorded
because the ones that turned out wrong are as informative as the one that held.

Not to be confused with `product_api_transport_for_coding_agents.md`, which is
about a different thing entirely — how a coding CLI reaches *product* HTTP APIs
(native_shell vs. bridge_shell). This document is about how *mcpagent itself*
reaches an *LLM provider*.

## The question

AgentWorks has always maintained two ways to reach a model:

1. **Direct API transport** — `multi-llm-provider-go`'s own adapters
   (openai/anthropic/vertex/bedrock/azure), a stateless HTTP call inside our own
   Go process.
2. **Coding CLI, over MCP** — pi, Claude Code, Codex CLI, Cursor CLI. We expose
   tools once over MCP; the CLI translates them into whatever its backend model
   needs.

Given that pi alone already supports every one of those same backend providers,
is there still a reason to maintain (1) separately, or should new work route
through (2) exclusively?

## The argument, round by round

Each round responded directly to the one before it. Recording the ones that
didn't survive is the point — a shorter list would just repeat the tempting
first answer.

### Round 1 — "Stateless is safe"

**Claim:** direct API transport has no subprocess, no session, nothing to
wedge or leak. Every reliability bug found earlier the same day (a zombie
process leak, a 45-minute internal wedge, a lost-final-event polling race) was
structural to the CLI/subprocess path.

**Where it broke:** pi's *structured* mode (`pi --print --mode json`) is
one-shot too — no session persists between calls, same as a direct API call.
The 45-minute wedge (PLAT-153) happened on exactly that mode. A native process
sample confirmed it directly: pi's Node event loop sitting completely idle, no
thread in any syscall, nothing pending. Statelessness didn't protect it.

### Round 2 — "Lightweight is safe"

**Claim:** pi is a light CLI with a plugin architecture — flexible, not heavy.

**Where it broke:** pi's own maintainers attribute their worst documented
freeze (5.5 hours, `earendil-works/pi#8004`) directly to a loaded extension
that never released the event loop: *"The subagent extension had no timeout
and resolved only on process close."* A plugin architecture means more
independent code that can each fail to clean up — more exposure to this
failure class, not less. "Lightweight" describes startup cost; it says nothing
about whether every loaded plugin correctly tears itself down.

### Round 3 — "We'd avoid per-provider tool-calling maintenance" — the round that actually held

**Claim:** maintaining tool-calling translation for 5+ direct providers is
real, compounding work — each provider has its own function-calling schema,
and `multi-llm-provider-go`'s adapters each do their own translation (verified
live: Vertex's own adapter logs *"Converting 2 tools to Gemini format,"
"Validating schema for function..."* — real per-provider code, not shared).

**Why it held:** MCP is the thing that actually solves this, and pi already
speaks it. We define a tool once; translating it into whatever the backend
model needs becomes pi's problem, not ours. This isn't hypothetical — the
API-provider P0 certification built earlier the same day already showed
measured gaps from trying to keep pace with 6 providers alone: **azure had
zero test files, minimax had no live end-to-end test.**

This is a scale/surface-area argument, not a "someone else tested it more"
argument — see the next round.

### Round 4 — "Community-tested beats internal" — too broad, doesn't survive as stated

**Claim:** code we write and only we exercise gets less real-world testing
than a widely-used open-source project's equivalent.

**Where it's too broad:** the specific failure class that matters most here —
the wedge — is pi's *own* community-tested code, and community testing hasn't
resolved it. `#8004` is an open report, not a merged fix. Several related pi
issues are auto-closed as new-contributor reports without action, not
genuinely resolved. Community testing volume reduces the odds of *shallow*
bugs; it did not catch or fix the *deep* one we actually hit.

**The sharper distinction:** not "who tested it more" but "who can fix it when
it breaks." A bug in our own compaction/token-counting code (found the same
day, described below) was fixed and shipped the same session. A bug in pi is
on pi's release cycle — and the evidence is that cycle isn't fast for this
exact class; some of those issues are months old.

**Refined takeaway:** narrow, self-contained logic we can fully verify and fix
ourselves is still worth owning — the reason isn't lower bug odds, it's a
same-day fix cycle when a bug is found. Broad, cross-cutting translation work
across many providers is worth avoiding — not because our code is worse, but
because the surface is too large to keep pace with alone, which round 3
already measured directly (azure: 0 tests, minimax: no live E2E).

### A claim that didn't survive contact with the code — auto-compaction

**Claim raised:** direct API transport would additionally need us to build
context compaction/auto-summarization ourselves, which pi already has "out of
the box."

**Checked, not assumed — false as stated.** `mcpagent/agent/context_summarization.go`
(534 lines) already implements this, is already wired into `askWithHistory` —
the same function every provider goes through, CLI or direct API — and has no
provider- or transport-specific branch anywhere in the file. This was never a
cost unique to direct API transport; it lives at the orchestration layer,
above the provider distinction, for both paths equally.

**Two things worth recording precisely, not just "it's fine":**

- This exact code path is what produced two of the same day's other bugs — an
  unbounded tiktoken vocab download and a retry storm on a failed token count,
  both inside `CountTokensForModel`, which `shouldSummarizeOnTokenThreshold`
  depends on. Real, found, fixed the same day — supporting evidence for round
  4's refined takeaway (narrow owned code, fixed fast), not a reason to
  distrust the file.
- **Pi's own compaction is not obviously more mature.** A same-day search of
  `earendil-works/pi`'s issue tracker for compaction specifically turned up a
  real list of open, compaction-specific bugs: auto-compaction failing to
  trigger past 100% context (`#6879`, open), a context-budget calculation bug
  causing overflow-recovery retries to fail (`#8061`, open), and — notably — a
  compaction summary that can be **persisted truncated mid-word** when
  generation hits its token cap (`#7048`, open: `stopReason: 'length'` not
  checked). Several related issues are closed, but describe serious failures
  (a retried turn silently lost during compaction, compaction failing
  entirely and leaving a session stuck over the limit). Compaction is real and
  useful in pi; it is not a solved problem there either.

## Decision

Stop investing in direct API transport as a general-purpose path. Tool-heavy
and agentic work routes through pi and the other coding CLIs over MCP, where
the per-provider translation cost is already paid for. No further work toward
certifying openai/anthropic/vertex/bedrock/azure to parity.

**Shipped 2026-08-20:** the five providers are marked `deprecated: true` in the
provider manifest (`agent_go/cmd/server/llm_provider_manifest.go`,
`isDeprecatedLLMProvider` in `cmd/server/llm_config_handlers.go`) and fully
hidden — not shown with a deprecated marker, simply absent — from:

- the LLM configuration modal (`LLMConfigurationModal.tsx`, pre-existing
  `entry.deprecated` filter — previously dead code, since the backend function
  it reads always returned `false` before this change)
- the provider catalog (`LibraryTab.tsx`, via the new
  `nonDeprecatedProviders` helper in `utils/providerCatalogFilter.ts`)
- the workflow LLM picker (`useLLMStore.ts`'s `FRONTEND_DEPRECATED_PROVIDER_IDS`,
  the same mechanism already used for the prior `agy-cli` coding-agent
  deprecation)

Existing saved LLM configurations on these providers are **not** removed and
continue to function; this only affects new setup.

## Not decided by this

- **Deleting** the five adapters from `multi-llm-provider-go`. Hiding them from
  new setup is reversible in one line; removing the adapters is a separate,
  much less reversible call, not made here.
- **What currently depends on this path in production.** AgentWorks exposes a
  per-user "bring your own API key" option that may route through direct API
  transport today. Auditing actual usage — not just configuration — was out of
  scope for this pass.
- **Whether pi's own provider configuration covers the same bring-your-own-key
  need** users might have been relying on direct API transport for. Needs
  checking before any further step (like adapter removal) is taken.
- Whether a genuine latency- or cost-bound use case exists anywhere in this
  codebase that would justify keeping direct API transport for a narrow
  purpose. None had surfaced as of this decision.
