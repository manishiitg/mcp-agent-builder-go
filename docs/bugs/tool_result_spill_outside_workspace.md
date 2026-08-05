# Bug: An oversized tool result becomes a file the agent is ordered to read and forbidden to open

## Status: RCA only (2026-08-04). Root cause established and verified. **No fix chosen.**

This is a recurrence of ticket #5 in
[what_the_runtime_tells_an_agent_about_itself.md](what_the_runtime_tells_an_agent_about_itself.md),
which that document's own review marked *"incomplete"*. It is recorded separately
because the earlier write-up describes the instance that appeared in the logs
(`execute_shell_command`), and the mechanism is not specific to that tool — it
reappeared through `read_skill` with nothing changed.

## Symptom

One session, `schedule-manual--2c694ae1`, working in
`Workflow/HDFC-Personal-Accounts`. Seven artifacts, all downstream of one event:

```text
1  read_skill → "result (67,971 characters across 112 lines) exceeds maximum
                allowed tokens. Output has been saved to
                ~/.claude/projects/<slug>/<session>/tool-results/
                    mcp-api-bridge-read_skill-1785866860551.txt
                ... You MUST read the content from the file at ..."
2  agent reads that path → access denied: truncated tool-result spill,
                           outside every workspace root
3  find /Users -name ...  → access denied: absolute host path (/Users)
4  Downloads/pulse_backlog.json → No such file or directory
5  curl … > Downloads/…    → curl: (23) Failure writing output
6  Downloads/migrate3.py:41 → SyntaxError: unterminated string literal
7  query_workflow_db        → no such column: answer
```

Only #1 is a cause. Everything after it is one agent improvising after being
handed an instruction it cannot carry out.

## The chain

```
read_skill returns 67,971 chars
  └─ exceeds the CLI's per-tool-result token budget
     └─ CLI truncates, writes the full copy under ~/.claude/projects/…/tool-results/
        └─ CLI instructs: "You MUST read the content from the file at <abs host path>"
           └─ agent obeys → execute_shell_command on that path
              └─ folder guard denies: outside every workspace root
                 └─ no legal way to comply → agent guesses (3–7 above)
```

## Root cause, part one — the request that was too large was three documents

`read_skill` accepts a batch. The result was one JSON envelope holding three
files, confirmed by reading the spill directly:

| file | bytes |
|---|---|
| `references/review-improve-log.md` | 47,253 |
| `references/html-output.md` | 15,246 |
| `references/review-improve-log-skeleton.md` | 12,223 |
| | **74,722** → 67,971 chars encoded |

No single document crossed the line. The skill contract tells agents to *"batch
up to five related reads in one call"*, and the reference docs are large enough
that following that advice reaches the budget. Two rules, each locally
reasonable, that were never checked against each other:
`post-run-monitor.md` alone is 70,230 bytes.

## Root cause, part two — the cap exists, at one call site

`capShellResultForAgent` bounds a result at 48,000 characters. It is applied in
exactly one place:

```text
agent_go/pkg/workspace/tools.go:138   → execute_shell_command only
```

`read_skill` has no cap. Neither does any other tool. The comment in
`shell_output_cap.go:13` notes that mcpagent's 100KB `maxOutputBytes` "guards a
different executor the builder never calls", so there is no backstop underneath.

The number itself was right: **67,971 > 48,000.** The existing cap would have
caught this exact payload. It was attached to a tool rather than to the boundary
every tool result crosses.

## Root cause, part three — the enforcing limit is not ours and can move

The limit that actually rejected the payload lives in the Claude Code CLI, not in
this repo. Verified against the installed binary, `v2.1.221`:

```js
function U0o() {
  let e = env.MAX_MCP_OUTPUT_TOKENS;
  if (e !== undefined && e > 0) return e;                       // 1
  let r = gate("tengu_velvet_ibis")?.mcp_tool;
  if (typeof r === "number" && isFinite(r) && r > 0) return r;  // 2
  return Xz_;                                                   // 3
}
Xz_ = 25000
```

| claim | how it was checked |
|---|---|
| this repo does not emit the message | `grep "exceeds maximum allowed tokens"` across the tree: 0 hits |
| the CLI does | same string, 6 occurrences in the `claude` binary |
| the knob is `MAX_MCP_OUTPUT_TOKENS` | present in the CLI's env-var registry |
| the default is 25,000 tokens | `Xz_ = 25000`, returned as the fallback |
| nothing here sets it | no occurrence in any `.go`, `.sh`, `.json`, `.env` |

Two consequences:

- **Branch 2 is a remote gate.** The effective limit can change server-side with
  no CLI update and no change here. A constant in this repo tuned against it is
  pinned to a number that can move.
- **It is one CLI's limit.** `codex-cli`, `cursor-cli` and `pi-cli` are also
  supported providers, each with its own ceiling and its own spill behaviour.
  None of them were examined.

`shell_output_cap.go:21` reasons about "a 25k-token result cap". That inference
was correct, and it was reached by reading logs rather than by reading the value.

## Root cause, part four — two systems, contradictory orders, no owner

The CLI's recovery instruction points into its own project directory. The folder
guard categorically refuses paths outside the workspace roots. Neither is wrong.
Nothing reconciles them, so the agent is handed a mandatory instruction and a
categorical prohibition covering the same path.

`codingCLIToolResultSpillDenial` already recognises this case and replaces the
useless advice ("use workspace-relative paths") with a usable recovery. That
improves the *message*; the contradiction is unchanged, so every future spill
reproduces the deadlock.

This is the archive's recurring shape: **one fact, two sources, and nothing
checking they agree.**

## Blast radius

The failure is not confined to wasted calls. Verified at `scheduler.go:3213`, a
workflow upgrade preflight that does not stamp its target version aborts the run:

```text
ERROR: workflow upgrade preflight upgrade-1.0.18 did not stamp required
version "1.0.18" (found "1.0.17"); normal schedule message was not started
```

The preflight is agent-work: read the migration instructions, edit
`workflow.json`, stamp the version. An agent trapped in the spill loop cannot
finish it, the version is never stamped, and **the scheduled run never starts.**
A scheduled workflow can therefore be held indefinitely by a truncated tool
result, retrying and failing the same way each time.

## What is NOT established

- **Which workflow reported `1.0.17`.** Every workflow on disk is now at 1.0.11–1.0.19
  and `HDFC-Personal-Accounts` reads 1.0.19, so the preflight either later
  succeeded or the message came from another workflow. Not traced.
- **Frequency.** The "137 markers in 5h37m" figure quoted in discussion comes
  from the earlier document, not from a fresh log scan. `server_debug.log` was
  not read for this write-up.
- **The other CLIs.** Only Claude Code was inspected.
- **Whether #7 (`no such column: answer`) belongs here.** It is a schema guess
  and matches
  [pulse_fixer_sqlite_readonly_wal_and_schema_guessing.md](pulse_fixer_sqlite_readonly_wal_and_schema_guessing.md);
  it may be independent of this chain rather than downstream of it.

## Directions considered, none chosen

Recorded so the next reader does not have to re-derive them. **No decision.**

1. **Pin the limit.** Set `MAX_MCP_OUTPUT_TOKENS` explicitly so branch 1 wins and
   the value stops depending on a remote gate or a vendor default. Cheap, and
   makes any cap here meaningful. Does not by itself stop spills.
2. **Move the cap to the boundary.** Enforce one budget where every tool result
   is serialised, instead of per tool. Closes the class rather than the instance.
   Open question: scripted steps parse tool output as schema-validated JSON and
   must keep every byte — the existing cap avoids this by living in the tool
   executor, and a boundary-level cap has to preserve that carve-out.
3. **Make the spill readable.** A narrow read-only exception for the session's own
   `tool-results/` directory. Resolves the contradiction directly, at the cost of
   a hole in a guard whose value is that it has none.
4. **Make the results smaller.** Split the reference docs, return sections rather
   than whole files, and reconcile the "batch up to five" advice with the budget.
   Addresses the cause rather than the symptom; largest scope.

Options 2 and 4 are not alternatives — 4 reduces how often 2 is needed.

## Related, and now fixed: the cap did not bound the delivered payload

The 48,000 budget was applied to `stdout + stderr` *before* `marshalResult`
JSON-encoded them, so escaping expanded the payload after the check. Raised by
the independent review on 2026-08-04. Measured, on 48,000 capped characters:

| content | delivered | over budget |
|---|---|---|
| plain prose | 48,063 | 1.0x |
| nested JSON | 61,677 | 1.3x |
| HTML report output | 90,949 | 1.9x |
| quotes / backslashes | 95,717 | 2.0x |
| all `<`, or control characters | 286,333 | **6.0x** |

`encoding/json` HTML-escapes `<`, `>` and `&` to six bytes each by default —
protection for JSON embedded in a page, which this payload never is, on exactly
the content these workflows produce most. Ordinary HTML output already shipped at
1.9x, so the cap was not holding in the common case, not merely a pathological
one.

Fixed in `shell_output_cap.go`: encoding drops HTML escaping, and
`marshalCappedShellResultForAgent` encodes, measures the real payload, and
re-caps against a proportionally smaller budget until it fits. The marker
fallback for a budget smaller than the marker itself now also fits. Regression
tests cover each content class above and assert the encoded payload stays within
budget and still parses.

This narrows the window but does **not** close this ticket: it bounds
`execute_shell_command` only. `read_skill` — the tool that produced the spill
described here — remains uncapped.
