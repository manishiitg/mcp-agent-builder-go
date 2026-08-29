[← Pulse platform index](../pulse_platform_issue_register.md)

# PLAT-234 — `search_web_llm` with a CLI-backed provider needs a multi-minute caller budget, not ~180s

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `guidance fixed; confirmed no platform timeout exists to fix` |
| Last synchronized | `2026-08-29` |

- **Priority:** harness_issue (guidance gap), severity medium across findings.
- **Findings:** Twitter/social-media `PUL-743BAC2B`, `PUL-CA44C7EC`,
  `PUL-0B1A2E4B` — three independent runs, all `search_web_llm` with
  `provider=codex-cli, model_id=gpt-5.5`, all citing the identical
  `deadline_seconds=180`, all recovering via the sanctioned native
  `WebSearch` fallback after the timeout.

## Investigation: confirmed there is no platform-side 180s timeout to fix

Checked every layer between the tool call and the actual CLI process,
starting from the same "is this even fixable in-repo" question asked for
PLAT-224/PLAT-232:

- `search_web_llm`'s own tool schema (`agent_go/pkg/workspace/advanced_tools.go`)
  has no `timeout`/`deadline` parameter at all.
- The live server's own `TOOL_EXECUTION_TIMEOUT` is `90m`, not 180s
  (confirmed against the actual running process's environment, not
  documentation).
- The codex-cli adapter's own interactive-turn timeout
  (`multi-llm-provider-go/pkg/adapters/codexcli/codexcli_interactive_adapter.go`)
  defaults to `0` (unbounded) by explicit design — the code comment reads
  "Workflow/background callers own their execution deadline; the adapter
  should not cancel a still-running tmux coding agent before the outer
  workflow timeout." The same file's prompt-wait ceiling
  (`defaultCodexPromptMaxWait`) is `90 * time.Minute`, confirming cold CLI
  startup is expected to legitimately take minutes, not seconds.
- No "180" timeout constant exists anywhere in this repo or in the sibling
  `mcpagent`/`multi-llm-provider-go` repos tied to search or shell execution.

`search_web_llm(codex-cli)` calls `GenerateContent` with no interactive
session ID, so every call gets a fresh bounded tmux session
(`codex-bounded-<ts>-<rand>`) — real CLI process cold-start, every time,
with no reuse across calls. That cold-start cost is the platform's own
explicit, undocumented characteristic of this provider, and nothing in this
repo enforces or even exposes a caller-facing timeout around it. The
180-second deadline in all three findings' `strategy_today.json`/
`proposed_primitives` records is the *workflow's own* self-chosen research
budget, not anything this platform imposes or could shorten/lengthen on the
workflow's behalf.

This is the same boundary shape as PLAT-224/PLAT-232, but the missing piece
here is different: not a third-party binary passthrough, but an undocumented
latency characteristic of a platform-provided tool that no caller had any
way to budget for correctly.

## Fix

Added two notes to `workspace-media-tools.md` (the `builder-reference`
skill's full tool reference, loaded by every session holding
`search_web_llm`): one inline on the `search_web_llm` bullet naming the
cold-start cost and the CLI providers it applies to, and one under "Common
mistakes" naming the exact failure shape (~180s deadline killing a
slow-but-working CLI provider) and the corrective action (widen the budget,
don't conclude the provider is broken from one timeout).

## Verification

`go build ./...` and `go test ./cmd/server/guidance/...` pass (template
renders clean). No Go code changed — like PLAT-223/224/232/233, this is a
guidance-only fix because there is no platform-owned timeout to change.

## Reverify

No live step has loaded this corrected guidance through the deployed server
yet. Reverify by observing whether a future codex-cli `search_web_llm` call
in a Track-A-style research step is given a multi-minute budget instead of
~180s, and whether that reduces reliance on the WebSearch fallback for this
provider.
