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
- **Implementation (corrected 2026-08-04):** conversion changes only the tab's
  view-only/scheduled metadata and preserves its session ID. The ordinary query
  path already prefers direct delivery to the retained main tmux. If that pane
  is genuinely gone, the same session is relaunched through the persisted
  native resume handle and its replacement tmux is materialized in the UI.
- **Verification:** backend coverage proves a settled retained tmux accepts a
  follow-up and becomes running/live again, while a session without a live tmux
  falls through to the normal same-session resume path. Frontend regression
  tests cover scheduled and bot observations and assert that conversion keeps
  both `tabId` and `sessionId`; TypeScript compilation passes. A real converted
  schedule remains to be exercised after deployment.
- **Regression tests:**
  `TestTryDeliverQueryAsLiveInputReactivatesSettledRetainedTmux` plus
  `workflowChatTabConversion.test.ts` for scheduled and bot conversations.
- **Acceptance:** active, settled, and relaunched scheduled coding-agent chats
  keep the same logical session ID and conversation context. The first user
  message is delivered exactly once—to the existing tmux when live, otherwise
  to the resumed replacement—and remains visible in the same tab.
