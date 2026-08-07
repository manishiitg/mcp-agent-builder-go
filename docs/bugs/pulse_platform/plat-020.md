[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-020 — converted scheduled chat must retain its session and tmux

| Coordination | Value |
|---|---|
| Assigned agent | `Codex` |
| Ticket state | `runtime_reverify` |
| Last synchronized | `2026-08-04` |

> Claim this ticket in this file before implementation. During active work,
> update this fragment rather than the shared index; synchronize the index
> once at handoff, review, or completion.


- **Priority:** P0
- **Owner:** workflow chat/session handoff
- **Source run:** RTS Latency
  `schedule-cron--42eca39a_1785810615371091000`
- **Problem:** the earlier proposed repair rotated the tab to a new UUID. That
  was the wrong abstraction: **Make interactive** means continue this exact
  conversation, not fork it. A new session bypasses the retained tmux and loses
  the provider-native conversation identity that already carries the context.
- **Implementation (corrected 2026-08-06):** conversion preserves the session
  ID and records an explicit durable `user_interactive_continuation` marker on
  the tab. Every later query carries that marker. The backend lets this explicit
  user promotion outrank the historical `schedule-*` ID, so a replacement CLI
  resumes the same native conversation and receives normal interactive
  retention. The ordinary query path still prefers direct delivery when the
  original tmux is live. No fork or new conversation is created.
- **Verification:** backend coverage proves a settled retained tmux accepts a
  follow-up and becomes running/live again, while a session without a live tmux
  falls through to the normal same-session resume path. Frontend regression
  tests cover scheduled and bot observations and assert that conversion keeps
  both `tabId` and `sessionId`; TypeScript compilation passes. A real converted
  schedule remains to be exercised after deployment.
- **Completion follow-up:** PLAT-035 records the separate defect where a
  successfully delivered retained turn stayed `foreground_turn.busy=true`
  after its tmux had returned to an idle prompt. PLAT-020 owns conversation
  continuity; PLAT-035 owns the retained turn's stream-driven end boundary.
- **Regression tests:**
  `TestTryDeliverQueryAsLiveInputReactivatesSettledRetainedTmux` plus
  `TestCodingAgentRequestAllowsPersistentInteractive` and
  `workflowChatTabConversion.test.ts` for scheduled and bot conversations.
- **Acceptance:** active, settled, and relaunched scheduled coding-agent chats
  keep the same logical session ID and conversation context. The first user
  message is delivered exactly once—to the existing tmux when live, otherwise
  to the resumed replacement—and remains visible in the same tab.
