# Terminal lifecycle

Runloop tracks a coding-agent terminal as two separate resources:

1. **Process lease**: ownership of the live tmux session.
2. **Terminal snapshot**: the read-only capture rendered in the UI.

The process lease is authoritative for cleanup. Hiding or expiring a snapshot
does not release a live process. Bounded workflow-step processes receive a
short shutdown grace after completion, while their snapshots can remain visible
for the longer provider display-retention period. Main-agent sessions are
persistent and use the idle backstop instead of the bounded deadline.

Every observed coding-agent tmux session is tagged with:

- `@runloop_instance_id`
- `@runloop_owner_pid`
- `@runloop_owner_started_at`
- `@runloop_heartbeat`

Startup recovery only removes tagged sessions whose owner PID/start-time pair
is no longer live. It preserves untagged legacy sessions and sessions owned by
another running backend or test process.

Terminal API state exposes:

- `process_state`: `live`, `closing`, or `closed`
- `snapshot_kind`: `live` or `archived`
- `close_reason`: the lifecycle reconciliation reason, when available

Operator actions follow these semantics:

- **Dismiss** hides only the snapshot.
- **Complete** asks the provider to finish and moves the lease toward closing.
- **Fail** closes the process and records a failed archived capture.
- **Kill** force-closes the process and records a failed archived capture.

SSE event buffers and session identity are retained for 24 hours after a session
reaches a terminal state so reconnect/resume keeps its event cursor and chat
context consistent. The event store's per-session count limit bounds memory;
terminal process cleanup and terminal snapshot retention remain independent of
this session-resume window.

## Frontend authority rules

The terminal UI combines data from endpoints with different responsibilities.
Keep these rules intact when changing polling, caching, or live attach:

1. **The terminal-list poll owns lifecycle state.** It decides `state`,
   `active`, `process_state`, `snapshot_kind`, and the tmux identity. A selected
   terminal detail/history response may contribute only its body
   (`content`, `content_source`, and `rows`). Detail responses can be older than
   the latest list poll; allowing them to overwrite lifecycle fields makes a
   live pane briefly appear completed and repeatedly reconnect.
2. **A completed turn is not necessarily a closed CLI.** Interactive main-agent
   CLIs remain available for follow-up messages after the current turn reaches
   `completed`. Continue live streaming while `process_state=live`; stop only
   when the process closes or the snapshot is archived.
3. **A live connection has one fixed terminal grid.** Do not resize xterm while
   bytes from the old grid can still arrive. Geometry changes must suspend old
   output, close the socket, fit the new grid, then reconnect for a fresh seed.
   Ignore duplicate `ResizeObserver` notifications when the containing box did
   not actually change size.
4. **Only one browser window owns live geometry.** A second viewer supersedes
   the first and the backend closes the first WebSocket with code `4001`. The
   superseded viewer must keep its already-rendered local frame. It must not
   import a snapshot captured at the new owner's grid because reflowing that
   frame at a different width corrupts wrapping. The user can explicitly
   **Take over**, which fits and reconnects using the local grid.

The focused regression coverage is in:

- `frontend/src/utils/terminalSnapshotIdentity.test.ts`
- `frontend/src/utils/terminalReconnect.test.ts`
- `agent_go/cmd/server/terminal_live_attach_test.go`

## Logical execution identity

The rail represents one logical execution once, even when that execution emits
events through several wrappers. A background task can have lifecycle events
keyed by `background_agent_id` while delegated content is keyed by a separate
`correlation_id`. `delegation_start` is the canonical link between those IDs;
the terminal store records it and resolves subsequent content to the lifecycle
owner.

Do not solve duplicate entries by hiding cards in React. Producers must emit a
declared `execution_kind` and stable parent/owner metadata, and the terminal
store must normalize those identities before snapshots reach the frontend.
Internal message-sequence items, scripted turns, and routers fold into their
workflow-step transcript. Independently selectable background agents remain
separate terminals.

The backend regression coverage is in
`agent_go/internal/terminals/store_test.go`, especially
`TestStoreUnifiesBackgroundDelegationContentWithItsLifecycleTerminal`.
