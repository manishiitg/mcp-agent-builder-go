[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-178 — resume falls back to a stale conversation snapshot and silently drops everything the user did live after it, even though the full transcript still exists on disk

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — root cause fully traced, not implemented |
| Last synchronized | `2026-08-22` |

- **Priority:** P1 — real conversation data loss, user-visible and
  confusing (the agent appears to "forget" recent work and asks the user to
  re-explain things it already did), and the recovery source needed to fix
  it is already sitting on disk unused.
- **Owner:** conversation persistence (`cmd/server/chat_history_persistence.go`,
  `cmd/server/server.go`'s full-turn save block), terminal/session restore
  (`cmd/server/chat_history_routes.go`), Claude Code transcript reader
  (`multi-llm-provider-go`'s `claudecode_transcript_messages.go`).
- **Related:** [PLAT-177](plat-177.md) (same session, same resume boundary,
  different symptom — tool-access confusion vs. this ticket's conversation
  data loss; investigated together, filed separately since the root causes
  are unrelated).

## Symptom

On workflow "substack" (session `b5e39872-4e4e-4645-8059-6d6e7a1231db`), the
user resumed a chat and the agent presented an old assistant message ("Yes —
both bugs you reported are fixed... I also added a new 'Follow Health'
panel... still want to check the `step-connect-creators` wording?") as if it
were the current end of the conversation. It was not — the user had
continued the conversation well past that point. The user: "this is not the
final assistant response.. this is like the 2nd msg... what don't we resume
properly."

## Root cause, confirmed by direct evidence

**1. `conversation_history` is a one-shot snapshot, written only when a full
turn completes — not continuously as a live tmux CLI session runs.**

The save happens once, at the end of the streamed `/api/query` handler,
after a turn completes:
- `[CONVERSATION DEBUG] Native continuation merge: ...` — `cmd/server/server.go:6233`
- `[CONVERSATION DEBUG] Final save: ...` — `server.go:6236-6238`
- `[BUILDER LOG] Saved conversation log (752 messages) to
  Workflow/substack/builder/conversation/2026-08-22/session-b5e39872-...-conversation.json`
  — `server.go:6358`, inside the `isWorkflowPhase` block (`server.go:6326-6363`).

Live: this fired at `13:51:26` on 2026-08-22, saving exactly 752 messages
ending with the "Follow Health panel" reply. The file's mtime on disk is
`13:51:26` and it was never written again.

A structurally identical one-shot writer exists for background "synthetic"
turns (`cmd/server/background_agents.go:2394-2435`). Neither path is
periodic; the only ticker in this area
(`conversation_turn_lifecycle.go:317`, 250ms) is a scheduled-message
completion poller, unrelated to persistence.

**2. Once a session is "warm" (a live tmux pane already exists — exactly the
state right after any resume), further messages go through `/api/live-input`
instead of a full turn, and that path only ever persists the user's own
outgoing message, never the assistant's reply.**

`/api/live-input` (`server.go:8342-8477`) delivers directly into the
retained tmux session (`retainedSession.Send`,
`deliverRetainedMainTerminalInput`, `mcpagent.DeliverAgentInput`) whenever
`sessionHasLiveMainCodingTmux` is true (`server.go:8390`) — bypassing the
full-turn handler entirely, so it never reaches the save block in finding
#1. Its only persistence hook is
`appendLiveInputToPersistedChatHistory` (called at `server.go:8353,8370`,
defined `chat_history_persistence.go:2934-3009`), which reads the existing
JSON, and appends **only**:

```go
history = append(history, llmtypes.MessageContent{
    Role:  llmtypes.ChatMessageTypeHuman,
    Parts: []llmtypes.ContentPart{llmtypes.TextContent{Text: message}},
})
```

(`chat_history_persistence.go:2976-2979`) — hardcoded
`ChatMessageTypeHuman`. The CLI's assistant replies to these live-input
turns are never written back. Live: the user's session went straight into
this warm state after the 13:51:26 save (confirmed by
`sessionHasLiveMainCodingTmux`-gated CDP browser activity at `13:55:04` and
raw terminal polling through at least `14:31`), so nothing after 13:51:26
ever reached `conversation_history` — even though real, substantial
conversation happened.

**3. When the tmux pane later dies and the session needs to restore, the
fallback tier trusts that same stale snapshot with no attempt to recover
anything richer.**

`chat_history_routes.go:155-176` restore logic: tier `attach_existing`
(`chat_history_routes.go:250-271`) tries to reattach to the live tmux pane
via `captureTerminalPane`; live, this failed at `16:18:20` with
`reason=tmux_session_not_running` (the pane was gone by then). It falls
through to tier `persisted_snapshot`
(`chat_history_routes.go:273-301`/`selectPersistedTerminalSnapshot` at
`chat_history_routes.go:303-314`), which reads only the `terminal_snapshots`
already embedded in that same stale conversation JSON — i.e. the
13:51:26 state. No tier consults tmux scrollback, a separate transcript
log, or the CLI's own on-disk session file; the comment at
`chat_history_routes.go:141-153` explains the two launch-based recovery
tiers are deliberately skipped here (a real, separate constraint — avoiding
a tool-registration race by deferring launch to the next `/api/query`) —
that constraint is legitimate but unrelated to *this* gap, and no other
tier was added to compensate for it.

**4. The full, missing conversation genuinely exists on disk — the restore
path just never reads it.**

Claude Code CLI writes its own transcript independent of agent_go's JSON,
at `~/.claude/projects/<encoded-cwd>/<session-id>.jsonl`
(`multi-llm-provider-go/claudecode_transcript_path.go:10-20`). Confirmed
live at
`~/.claude/projects/-Users-mipl-ai-work-mcp-agent-builder-go-workspace-docs-Workflow-substack/71ed3dcb-fe3c-412d-94cb-e62226fc4648.jsonl`
(`external_session_id` from the session's own `runtime.agent_session_handle`)
— **155 entries timestamped after 13:51:26**, including 36 real user
messages, 63 real assistant messages, 37 attachments, and 3 system entries,
running until a final `<local-command-stdout>Bye!</local-command-stdout>`
entry at `15:19:59` local time. None of this reached
`conversation_history`; all of it is still sitting in this file, unread by
the restore path.

`readClaudeTranscriptMessages`
(`multi-llm-provider-go/claudecode_transcript_messages.go:43-134`) already
knows how to reconstruct full `[]llmtypes.MessageContent` (text, tool_use,
tool_result) from exactly this file format, but it is only ever called from
`claudecode_interactive_adapter.go:664`, scoped to a single turn's
`turnStart..now` window — never from the resume/restore path.

## Why this matters beyond the one incident

Any session that (a) resumes into a warm/retained tmux state — which is the
normal case, not an edge case, since a resumed session almost always still
has its tmux pane alive initially — and (b) exchanges any further messages
purely through `/api/live-input` before its tmux pane eventually dies for
any reason (intentional `/bye`, crash, terminal reap, host restart) will
have that entire stretch of conversation permanently invisible to
`conversation_history`, `costs`, and every UI/API surface that reads it —
even though Claude Code's own transcript still has it. This is not specific
to the substack workflow or to this one incident.

## Suggested fix direction (not implemented)

Two independent gaps, either one closing most of the practical impact:

1. **Close the live-input assistant-reply gap** (finding #2): after a
   successful `/api/live-input` delivery, also capture and append the
   CLI's resulting assistant reply — not just the outgoing human message —
   to `conversation_history`, the same way a full turn would. This keeps the
   snapshot current continuously instead of only at full-turn boundaries.
2. **Make the `persisted_snapshot` restore tier read the native transcript**
   (finding #3/#4): for `provider=claude-code` sessions with a resolvable
   `ExternalSessionID` and workspace path, read
   `~/.claude/projects/<encoded-cwd>/<session-id>.jsonl` (a workingDir-scoped
   variant of `readClaudeTranscriptMessages`, currently turn-scoped only)
   and merge anything newer than the last saved `conversation_history`
   timestamp before falling back to the stale JSON. This is strictly
   additive recovery and doesn't touch the deliberate tool-registration-race
   avoidance already documented at `chat_history_routes.go:141-153`.

(1) prevents the gap from opening; (2) recovers it after the fact even if it
does. Doing both is the most robust fix; (2) alone is lower-risk to ship
first since it only reads an existing file and only activates on the
already-broken fallback path.

## Non-goals

- Not implementing either fix direction in this pass — filed for design +
  implementation as a follow-up, consistent with how PLAT-171 was filed.
- Not investigating why the tmux pane died in this specific incident — the
  transcript's own last entry (`<local-command-stdout>Bye!</local-command-stdout>`)
  suggests an intentional `/bye`/exit rather than a crash, but the fix
  needed here is the same regardless of why the pane ends.

## Acceptance tests (once a fix is designed)

1. A session that resumes into a warm tmux state, exchanges several more
   messages purely through `/api/live-input`, then has its tmux pane
   deliberately ended — resuming again shows the true latest assistant
   message, not the last full-turn snapshot.
2. `conversation_history`'s message count and content, read right after a
   sequence of live-input exchanges, matches the CLI's own native transcript
   for the same time window (not just the outgoing human messages).
3. The existing tool-registration-race avoidance
   (`chat_history_routes.go:141-153`) is unaffected — restore still defers
   CLI relaunch to the next `/api/query`, only the conversation content
   itself becomes current.

## Verification

Root cause traced and independently re-verified by direct evidence, not
implemented:
- `conversation_history` JSON's mtime (`13:51:26`) confirmed frozen while
  the live session continued (CDP browser call at `13:55:04`, terminal
  polling through `14:31`).
- `appendLiveInputToPersistedChatHistory`
  (`chat_history_persistence.go:2934-3009`) read directly, confirmed it only
  appends the human message.
- The native Claude Code transcript file was located on disk and read
  directly: 1774 total lines, 155 entries timestamped after `13:51:26`
  local (36 user, 63 assistant, 37 attachment, 16 queue-operation, 3
  system), ending `15:19:59` local — proving the "lost" conversation is
  fully intact and just unread by the current restore path.
