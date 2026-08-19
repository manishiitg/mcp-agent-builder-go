[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-152 — Pi CLI's interactive adapter emits native tool-call chunks; Claude Code's and Codex CLI's do not

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `filed` — confirmed from source across all three adapters; no code change in this ticket |
| Last synchronized | `2026-08-19` |

- **Priority:** P2 — not itself a user-visible bug (PLAT-149 already ships a
  provider-agnostic recovery path that does not depend on this), but it is
  the concrete evidence that mcpagent's coding-agent adapters are not
  standardized against a shared contract, which is the stated job of that
  layer.
- **Owner:** `multi-llm-provider-go/pkg/adapters/{claudecode,codexcli,picli}`
  interactive adapters.

## How it was found

While hunting PLAT-149's still-unlocated "mechanism B" — the source of
tool-call events carrying a provider's own native id (Claude's
`toolu_XXXXX`, Codex's `call_XXXX`) for interactive/tmux-transport sessions.
Ruling out every candidate in `claudecode_interactive_adapter.go` and
`codexcli_interactive_adapter.go` meant reading every adapter's `StreamChan`
send site directly, which surfaced this asymmetry by comparison rather than
by looking for it directly.

## What is proven

Direct grep for `StreamChunkTypeToolCallStart`/`StreamChunkTypeToolCallEnd`
construction, confirmed by reading the surrounding code at each hit:

- `pkg/adapters/picli/picli_interactive_adapter.go:1410` and `:1425` —
  Pi's interactive adapter constructs and sends both chunk types natively,
  as part of its own stream processing.
- `pkg/adapters/claudecode/claudecode_interactive_adapter.go` — zero
  references to either type anywhere in the file. The only chunk kinds sent
  directly on `streamChan <-` are `StreamChunkTypeTerminal` and
  `StreamChunkTypeStatusLine` (confirmed by grepping the raw send syntax, not
  just the type-constant name, since a send could in principle construct the
  value inline without naming the constant at the call site).
- `pkg/adapters/codexcli/codexcli_interactive_adapter.go` — same result: no
  literal construction of either chunk type anywhere in the file.

So of the three interactive (tmux) coding-agent adapters this platform runs,
exactly one reports its own tool calls through the adapter contract instead
of leaving them to be reconstructed after the fact by another layer.

## Why this matters here specifically

`mcpagent`'s stated job (per standing product direction) is to standardize
coding-agent behavior so the platform does not need per-provider fixes. This
adapter contract is the concrete place where that standardization is
currently missing: a consumer of the `StreamChan` cannot rely on tool-call
chunks existing for a session just because the provider is one of the
supported CLIs — it depends on which CLI. PLAT-149's recovery path was
deliberately built not to depend on this (matching by name/time against
`toolcalllog`, a provider-agnostic HTTP-layer signal instead), which is why
this asymmetry did not block that fix — but it means any *future* consumer
that expects adapter-level tool-call chunks uniformly will work correctly
against Pi and silently misbehave against Claude Code and Codex CLI, exactly
the kind of single-provider surprise the standardization goal exists to
prevent.

## What was not investigated here

- Whether Pi's mechanism is itself complete/correct (paired starts and ends,
  correct ids, no missed calls) — only that it exists, unlike the other two.
- Whether Claude Code's and Codex CLI's *end-of-turn transcript
  reconstruction* (`readClaudeTranscriptMessages` /
  `readCodexTranscriptMessages`, both unconditional, both already read in
  full while investigating PLAT-141/149) is meant to be the intentional
  substitute for live per-chunk reporting, or is simply what was built first
  and never reconciled with Pi's adapter. That design question — one shared
  contract both other adapters implement live, vs. Pi's live path being
  brought in line with the transcript-reconstruction approach instead — is
  the actual fix decision for whoever picks this up, not resolved here.

## Acceptance

- A decision is recorded on which shape is the platform's real adapter
  contract for tool-call reporting: live per-chunk (Pi's shape) or
  end-of-turn reconstruction (Claude/Codex's shape).
- Whichever is chosen, `claudecode_interactive_adapter.go`,
  `codexcli_interactive_adapter.go`, and `picli_interactive_adapter.go` all
  implement it the same way, so a consumer of any interactive adapter's
  `StreamChan` sees the same contract regardless of provider.
