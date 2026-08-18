[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-139 — a workflow step held its caller for 65 minutes after its real work finished; cause unknown after the first diagnosis was disproven

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `open` — cause **unknown**. The first diagnosis was investigated, acted on twice, and disproven; this ticket exists so the real cause is found from evidence rather than re-theorised |
| Last synchronized | `2026-08-18` |

- **Priority:** P1 — a genuinely successful run reports as stuck/never-completing
  and holds its caller indefinitely. Unattended paths (Pulse, schedules,
  workflow steps) have nobody watching to notice, and the run's own
  `run_metadata.json` can be left frozen at `"status": "running"`.
- **Owner:** unassigned — the layer is not yet identified. Candidate layers in
  "Where to look next" below.
- **Related:** [PLAT-116](plat-116.md) (same *shape* of symptom on the tmux
  path; its Pi structured section documents this incident's disproven first
  diagnosis in full)

## The incident

2026-08-18, workflow `ICICI-BANK-PARSING-v2`, group `manishiitg`, step
`statement-download`, execution `workflow-full-msy44vw303`.

- The step's **real work completed at 09:45** — statements downloaded,
  `logout_verified: true` recorded in its own output.
- The caller was then **held for ~65 minutes**, until the stack was stopped by
  hand. The scheduler sat polling
  `query_step(execution_id: workflow-full-msy44vw303)` for a completion that
  never arrived.
- Because the manual shutdown killed the workspace server before the
  orchestrator flushed terminal status, that write was swallowed and
  `run_metadata.json` is **still frozen at `"status": "running"`** — the run
  reads as live rather than failed. (That swallowed-terminal-write behaviour is
  its own defect, noted in PLAT-116 and not owned here.)

## What this is NOT — ruled out, with evidence

Recording these so the same ground is not re-tread. Each was believed and, in
two cases, shipped against.

### 1. NOT "pi never emits `agent_settled`"

The original diagnosis was that pi 0.84.2 stopped emitting `agent_settled`, so
the structured adapter's only teardown trigger was dead code. The supporting
measurement — *"0 occurrences of `agent_settled` against 57 `agent_end` across a
day of production logs"* — was a **logging artifact created by the measurement
itself**: the adapter *handled* `agent_settled` in a `case` arm that logs
nothing, while `agent_end` fell through to the `default:` arm whose only job is
`Debugf("pi: unhandled event type=%q")`. The grep compared a deliberately-silent
handled event against a deliberately-logged unhandled one.

Refuted by running the real CLI with a real key, a real stdio MCP server via
`.pi/mcp.json`, and the adapter's exact flags
(`--print --mode json --no-builtin-tools -e npm:pi-mcp-adapter --approve`):

```
   1 "type":"agent_end"
   1 "type":"agent_settled"     <-- emitted, and last
>>> pi EXITED on its own after ~5s      (no stale pi or MCP processes)
```

Re-verified twice more (2 consecutive runs, clean exit in 4s each, zero
leftover `pi` or MCP server processes).

`agent_settled` is also correct *by construction* in pi's source
(`dist/core/agent-session.js`): emitted from a `finally` block — unskippable on
success, error, or abort — exactly once per run, **after** the
`while (_handlePostAgentRun())` loop that drives multi-step tool use and
retries has drained. `agent_end` fires *inside* that loop, many times per run.

### 2. NOT fixable by treating `agent_end` as terminal

`multi-llm-provider-go@6609765` did this. It killed a healthy, still-working pi
process within hours — the 3s natural-exit grace expired mid-run:

```
11:56:27 [SHUTDOWN] natural-exit grace (3s) expired for pid 67763
11:56:27 [SHUTDOWN] SIGTERM attempt 1/3 to pgrp 67763
11:56:27 [SHUTDOWN] process 67763 exited after SIGTERM #1
```

surfacing as `pi run failed: exit status 143` (pi's print-mode SIGTERM handler
is `process.exit(143)`). Reverted in `@fd00585`.

### 3. NOT a child process inheriting and holding pi's stdout

A follow-up theory held that pi exits while a spawned child (the MCP bridge)
keeps stdout's write end open, so `<-scannerDone` never unblocks. A four-adapter
rework was built on this and **abandoned before commit**. The MCP SDK spawns
stdio servers with `stdio: ['pipe','pipe', stderr]` — the child gets its **own**
pipes and never inherits the parent's stdout. pi's own adapter additionally
passes `stderr: "ignore"` outside debug mode.

### 4. NOT a missing P0 certification

Also claimed and disproven. `RequiredP0CodingAgentCertificationIDs` early-returns
`nil` for `Transport != tmux`, which looked like "structured transport has zero
P0 coverage" — but **no provider is registered as `Transport: structured`**.
That field is the provider's *primary* transport; all four run tmux primary and
structured secondary, and structured behaviour is certified via capability flags
(`UsesPersistentSession`, `SupportsStructuredStreaming`). Verified by executing
the function:

```
provider=claude-code  transport=tmux  requiredP0=17  [structured_streaming structured_multi_turn]
provider=codex-cli    transport=tmux  requiredP0=17  [structured_streaming structured_multi_turn]
provider=cursor-cli   transport=tmux  requiredP0=16  [structured_streaming structured_multi_turn]
provider=pi-cli       transport=tmux  requiredP0=16  [structured_streaming structured_multi_turn]
```

Structured *completion* is covered specifically: each provider's
`CertStructuredMultiTurn` is a real, live-gated E2E (e.g.
`TestPiCLIStructuredTwoTurnResume`). A two-turn resume cannot pass unless turn
one completed and its process exited — so a genuine structured hang would fail
it. All four P0 enforcement tests pass.

## What is still true and unexplained

The incident itself is real: work finished at 09:45, the caller was held ~65
minutes, and the step never reported completion. None of the four explanations
above accounts for it.

## Where to look next

Untested hypotheses, roughly in order of fit:

1. **A tool call that never returned.** The strongest remaining candidate. pi
   emits no events between `tool_execution_start` and `tool_execution_end`, so a
   hung MCP tool (this step is browser/banking automation) looks identical to a
   silent-but-healthy long call. This workflow runs `TOOL_EXECUTION_TIMEOUT=90m`,
   so a hang inside that window is bounded only by the tool itself. Check whether
   the last event before the silence was a `tool_execution_start` with no
   matching `_end`.
2. **The layer above the adapter**, i.e. the same `for chunk := range textChan`
   bridge PLAT-116 traces for the tmux path — it is provider- and
   transport-agnostic and has no timeout of its own.
3. **The orchestrator/scheduler poll loop** — whether `query_step` can keep
   reporting in-progress for an execution whose underlying turn has already
   returned.

## Evidence needed (was not captured this time)

The 09:45 window is **no longer in `server_debug.log`** — it had rolled over by
the time the disproven diagnosis was unwound, which is part of why this stayed
unresolved. For the next occurrence, capture before restarting anything:

- the raw pi JSON event stream for the step (specifically: last event before the
  silence, and whether a `tool_execution_start` is unmatched);
- `ps` for the `pi` process **and** its children at the moment of the stall
  (note `pi` is a `#!/usr/bin/env node` script, so match on the full command
  line, not the process name);
- whether the platform's own completion logging
  (`[STREAMING_LIFECYCLE] StreamWithEvents completed`, `[COMPLETION] Updating
  session ... completed`) ever fires;
- goroutine dump of the server, to see which layer is actually parked.

## Acceptance

- The 65-minute stall is explained by evidence from a real occurrence, not by
  theory.
- Whatever layer is responsible either completes or fails within a bounded,
  logged time rather than holding its caller indefinitely.
- A run whose work genuinely finished never reports as permanently `running`.

## Process note

The first diagnosis was reached by reading logs and source, defended across
three rounds of further source-reading, and shipped twice — before anyone ran
the CLI. Two live runs (~2 minutes) refuted it. For this class of bug, **run the
real thing early**; absence of a log line is not absence of an event, especially
when the code under investigation is what decides whether that event is logged.
