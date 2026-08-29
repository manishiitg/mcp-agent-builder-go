[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-246 — Workflow Builder lacks actionable scripted-use guidance for the active text and web-search tools

| Coordination | Value |
|---|---|
| Assigned agent | Codex |
| Ticket state | `filed` |
| Last synchronized | `2026-08-29` |

- **Priority:** harness issue, severity medium.
- **Findings:** No production workflow failure is linked yet. The gap was found
  while refactoring the shared provider-backed agent surface to retain only
  `generate_text_llm` and `search_web_llm`.
- **Related:** [PLAT-244](plat-244.md) (narrowed tool surface and restored
  reference pointer); [PLAT-234](plat-234.md) (search timeout guidance).

## Problem

The Builder/Workshop prompt correctly exposes the two active tools and points
agents at `builder-reference/references/workspace-media-tools.md`. That
reference documents the signatures and valid enum values, but it does not give
a workflow author or scripted step enough decision guidance to use the tools
well:

- when fresh, external evidence warrants `search_web_llm` instead of ordinary
  reasoning or an already-available source;
- when a bounded second model call warrants `generate_text_llm`, and how to
  choose `low`, `medium`, or `high` based on task difficulty and cost;
- that a script must invoke these tools through the authorized MCP bridge,
  following the `mcp-bridge` reference, rather than constructing raw requests
  or embedding credentials; and
- minimal success, timeout, provider-failure, and evidence-recording patterns.

Without this, agents know the API names but must infer operational policy. That
invites avoidable direct-answer behavior, unprincipled tier selection, and
unsafe shell/credential workarounds.

## Proposed fix

1. Expand `workspace-media-tools.md` into an active-tools workflow guide with
   concise decision rules and one safe example for each tool.
2. Add a scripted-agent section that explicitly routes calls through the MCP
   bridge, points to `mcp-bridge.md`, and forbids raw-key/raw-request patterns.
3. Keep provider/media retirement language out of the operational path so the
   two active tools are prominent and unambiguous.
4. Add prompt/materialization contract coverage: a Workshop agent with either
   tool must receive the guide; a scripted agent must be pointed to both this
   guide and the MCP bridge contract. Add an agentic P0 only if static contract
   coverage cannot prove the actual prompt/tool behavior.

## Acceptance criteria

- Builder, Workshop, and run-mode agents that are granted either active tool
  can discover an exact, current use pattern through `builder-reference`.
- The guide supplies tier-selection guidance for `generate_text_llm` and
  provider/freshness/failure guidance for `search_web_llm`.
- Scripted usage is bridge-only and never requires a raw credential or an
  invented endpoint.
- Regression tests prove the capability-derived guidance is present only when
  the relevant tools are available, avoiding the broad-reference regression
  previously addressed by PLAT-125.

## Reverify

Start a fresh Workshop session with each tool separately and a scripted step
with both. Confirm the agent reads the attached guidance, uses the MCP bridge
for a real call, records/cites returned search evidence where relevant, and
chooses a text tier consistent with the documented task class.
