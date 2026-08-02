# Lazy Per-Terminal Event Loading

**Status:** Implemented and verified 2026-08-02
**Repositories:** `mcp-agent-builder-go` (`frontend` + `agent_go`)

## Outcome

The frontend no longer keeps every child agent's tool arguments, tool results,
LLM payloads, and terminal-screen snapshots in the session-wide Electron
working set.

For an owned child terminal:

1. The rail and runtime status still come from the lightweight session channel.
2. Opening the terminal requests its latest 300 transcript events.
3. A live selected terminal refreshes with an exclusive `after_sequence` cursor.
4. **Load earlier events** requests the preceding page with
   `before_sequence`.
5. Switching terminals releases the previous detailed page instead of caching
   every opened child indefinitely.

The main-agent transcript remains in the eager session working set in this
rollout. It is the foreground chat already being displayed, and retaining it
avoids changing the main conversation/streaming contract while removing the
large multiplicative cost: unopened child agents.

## Why this makes the frontend faster and lighter

The previous DOM was already virtualized by Virtuoso. The expensive part was
before rendering:

- session polling and SSE serialized child tool/LLM payloads;
- JSON parsing allocated them in the renderer process;
- Zustand retained them in `tabEvents`;
- every terminal selection filtered the full session array;
- an unused `ownedStreamingTerminalText` path retained a full screen snapshot
  for every child owner.

The implemented path changes those costs:

- `working_set=session` makes session polling/SSE omit child tool and LLM
  transcript detail while retaining main-agent detail and lifecycle/control
  events;
- suppressed SSE payloads advance a tiny cursor event, so reconnect does not
  replay them;
- live streaming events still cross SSE ephemerally to preserve structured CLI
  progress behavior;
- the frontend working-set filter is a compatibility safety net if an older
  server sends detailed child events;
- child terminal screen snapshots are no longer copied into unused Zustand
  maps;
- only the selected child page is held by `TerminalCenter` local state.

This is a runtime memory, parsing, and event-network improvement. It is not a
JavaScript bundle-size refactor. The production build currently passes the
1,010 kB hard JS gzip budget but warns at 1,000.03 kB against the 950 kB warning
threshold; code splitting is a separate follow-up.

## Backend contract

### One write-time owner

`EventStore.AddEvent` now materializes these fields once:

```text
terminal_owner_id   singular canonical transcript owner
terminal_id         exact terminal-store/API identity
sequence            monotonic per-session cursor
```

`ResolveTerminalOwnerID` is the canonical backend resolver used by both the
event store and terminal store. Main is a positive owner
(`main:<session_id>`), not a frontend complement. Workflow internal turns fold
into their owning workflow-step terminal before the event is indexed.

The old TypeScript owner-key derivation remains only as a fallback for events
created before these fields existed. Modern events take `terminal_id`
authoritatively.

### Bounded terminal index

The event store maintains a per-session/per-terminal index over the existing
bounded session working set. It is rebuilt whenever global retention prunes the
session, so the index cannot keep evicted payloads alive as a second unbounded
history store.

### Cursor API

```http
GET /api/terminals/{terminal_id}/events
    ?limit=300
    &before_sequence=<exclusive cursor>
    &after_sequence=<exclusive cursor>
```

Response:

```json
{
  "terminal_id": "...",
  "events": [],
  "has_older": false,
  "has_newer": false,
  "oldest_sequence": 0,
  "latest_sequence": 0
}
```

The route resolves the terminal snapshot first and applies the same session
authorization as other terminal routes. Cursor values are positive, exclusive,
mutually exclusive, and the limit is bounded to 1–1000.

## Frontend behavior

`TerminalCenter` owns one `SelectedTerminalEventPage`:

- initial selection: latest 300;
- selected live child: one-second incremental cursor refresh;
- terminal snapshot update/settle: final incremental refresh;
- older history: explicit user action;
- newer `after_sequence` refreshes preserve the client's older-history state
  instead of treating the incremental page's `has_older` flag as unseen history;
- request generation guards: stale responses from a previously selected
  terminal cannot replace the current transcript;
- overlapping pages merge by event ID/sequence and sort chronologically;
- fetch/refresh errors are visible and retryable.

`TerminalEventTranscript` remains virtualized. It now exposes loading, retry,
refresh-error, and load-earlier states rather than making a session-wide
hydration request when a terminal is opened.

## Compatibility and limits

- Events without `terminal_id` fail open in the session working set and use the
  legacy ownership resolver, so older in-memory shapes are not silently hidden.
- The cursor API pages the server's bounded in-memory event history. It does not
  turn `ui_events` into a durable transcript archive and cannot recover events
  already removed by server retention or a process restart.
- Initial pages are capped at 300 and the total indexed history remains bounded
  by the existing per-session EventStore retention. This change removes the
  frontend's all-child working set; it does not claim infinite history.
- Main-agent detail remains eager. Making main lazy is possible now that it has
  a positive owner, but should be a separate change because main streaming and
  foreground chat semantics have more consumers than child transcripts.

## Cross-language retention contract: complete

**Status: implemented and verified 2026-08-02.** The audit confirmed that the
two lists are correct as written. They represent different policies, so making
them identical would be a defect. The open risk was silent drift.

Event *ownership* is now resolved once, in Go, by `ResolveTerminalOwnerID`, with
the TypeScript derivation demoted to a legacy fallback. That removed the
duplication this design note originally warned about. But a second fact is now
stated twice, in two languages:

```text
agent_go/internal/events/terminal_ownership.go
  childTranscriptDetailEventTypes            10 types

frontend/src/utils/sessionEventWorkingSet.ts
  CHILD_TRANSCRIPT_DETAIL_EVENT_TYPES        13 types

delta (TypeScript only): streaming_start, streaming_chunk, streaming_end
```

The delta is deliberate and correct. Go must keep streaming events on the wire so
live terminal and CLI progress rendering still works; the frontend must not
retain them in Zustand, because per-child screen snapshots are precisely the
memory this change removes. Server-side wire policy and client-side retention
policy are genuinely different questions that happen to share a vocabulary.

The hazard was that the invariant binding them was held by two comments and
nothing else:

- a new high-volume child event type added to the **Go** list only — the frontend
  keeps retaining it, silently reintroducing the leak this work removed;
- added to the **TypeScript** list only — it is dropped from the store but still
  crosses the wire on every poll and SSE frame.

Neither raises an error. Both look like working software.

The trap for whoever notices the mismatch first: making the two lists identical
is the obvious "fix" and is wrong in both directions. Removing the streaming
triple from TypeScript restores the per-child snapshot retention; adding it to Go
stops live streaming from reaching the terminal at all.

The implemented guard asserts the relationship rather than incorrectly merging
the policies. `session_working_set_contract_test.go` reads the actual TypeScript
declaration and compares it with the actual Go map:

```text
GO_LIST is a subset of TS_LIST
TS_LIST - GO_LIST == {streaming_start, streaming_chunk, streaming_end}
```

This fails both drift directions with a reason specific to the consequence:
missing shared entries warn that session-wide frontend retention is being
restored; unexpected frontend-only entries require deciding whether backend wire
suppression should include them. The test resolves the frontend file from its
own source location, so it does not depend on the test process working directory.

Generation was deliberately not added. It would introduce another build
artifact and generation step for ten stable names while catching no additional
drift. If this vocabulary grows materially, a generated shared base may become
worthwhile; today the direct relationship test is smaller and clearer.

## Verification

Added coverage proves:

- canonical main and child ownership precedence;
- terminal identity consistency;
- child detail vs lifecycle session-working-set filtering;
- latest, before, and after cursor semantics;
- no cross-terminal leakage;
- terminal indexes follow global retention bounds;
- the HTTP response contains canonical owner, terminal, and sequence fields;
- frontend compatibility filtering;
- overlapping page merge/deduplication and cursor bounds;
- reaching the beginning keeps **Load earlier events** hidden across subsequent
  live `after_sequence` refreshes;
- existing transcript ownership/grouping behavior.
- the frontend child-retention set is exactly the backend wire-suppression set
  plus the three intentionally ephemeral streaming events.

Commands run successfully on 2026-08-02:

```text
go test ./...                                      PASS
npm test                                           56 files, 349 tests PASS
npm run build                                      PASS
npm run types:generate                             PASS
```

## Manual test

1. Restart the Go server so new events receive terminal ownership and sequence.
2. Run a workflow with several tool-heavy child agents.
3. Leave some child terminals unopened and inspect Electron memory: their tool
   arguments/results should not accumulate in `tabEvents`.
4. Open a completed child terminal: its transcript should load on demand.
5. Open a live child terminal: new tool calls should appear incrementally.
6. For a child with more than 300 retained events, click **Load earlier events**
   and confirm the preceding page appears without duplicates.
7. Switch rapidly between two terminals and confirm a slower response from the
   first never appears under the second terminal's header.
