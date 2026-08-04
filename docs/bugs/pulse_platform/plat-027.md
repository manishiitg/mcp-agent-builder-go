[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-027 — a live asynchronous child disappears after its parent completes

| Coordination | Value |
|---|---|
| Assigned agent | `Codex` |
| Ticket state | `implemented` |
| Last synchronized | `2026-08-04` |

- **Priority:** P0
- **Owner:** terminal execution-tree projection and polling
- **Evidence:** in the Social Media run, the route/orchestrator turn reported
  completion after dispatching `execute-allocate`, while the asynchronous child
  remained live. The execution tree knew about the child, but the terminal rail
  showed no active agent, making a live workflow appear stopped.
- **Root cause:** the rail was built only from retained terminal snapshots. A
  child can register in the execution tree before its terminal/transcript
  snapshot exists, and polling could stop during the short idle gap between the
  parent finishing and the child registering.
- **Implementation:** `TerminalCenter` now polls the session execution tree
  while the surrounding session expects activity and projects every visible
  live child into the rail. A projected row is an ephemeral running placeholder
  identified by the real execution ID. When the terminal snapshot arrives it
  enriches/replaces that identity rather than creating a second tab. Session
  roots, main agents, synthetic turns, and completed historical children are
  not projected.
- **Primary files:** `frontend/src/utils/terminalExecutionProjection.ts`,
  `frontend/src/components/TerminalCenter.tsx`,
  `frontend/src/hooks/useSessionExecutionTree.ts`, and
  `frontend/src/services/api-types.ts`.
- **Verification:** unit tests cover (1) a running asynchronous child with no
  terminal yet, (2) reconciliation with a later real terminal without a
  duplicate, and (3) suppression of completed historical children. Frontend
  typecheck, lint, and the full test suite passed during implementation.
- **Acceptance:** when a parent turn ends after dispatch, its still-running
  child remains visible and marked running until it really settles, even before
  the detailed terminal is retained. A real Social Media producing run remains
  the runtime recheck.
