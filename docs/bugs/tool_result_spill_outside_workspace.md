# Bug: An oversized tool result becomes a file the agent is ordered to read and forbidden to open

## Status: OPEN (as of 2026-08-05). Root cause established and verified. Every known route closed except one: `agent_browser`.

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

## 2026-08-05 — what changed, what is still open

### Closed

| route | how it was closed |
|---|---|
| `read_skill` batching | `maxReadSkillBatchSize` 5 -> 1 in mcpagent. A count cannot bound a payload; five small files are fine and two large ones are not. |
| a single oversized reference doc | `post-run-monitor.md` (25,769 tokens, the only doc that alone exceeded the cap) removed from the runtime bundle; the one-time migration contract split out of `review-improve-log.md`. No bundle doc now exceeds one result. |
| the shell result cap not actually capping | enforced on the SERIALIZED payload, HTML escaping dropped. See the section above. |

### Still open: `agent_browser`

Of the four workspace tools an agent can actually call, three are bounded and
one is not:

```text
execute_shell_command      capped (48,000 chars, enforced post-encode)
read_skill                 one file per call
diff_patch_workspace_file  small by nature
agent_browser              NOT capped   <- the remaining route
```

`NewBrowserExecutor` wires `agent_browser` straight to
`browserExecutor.HandleAgentBrowser` with no cap, and page content is exactly
the kind of thing that gets large. Capping it makes the deadlock unreachable
rather than merely unlikely.

### Correction to an earlier claim in this document's discussion

During this audit `read_workspace_file` was measured against real
`builder/improve.html` files (up to 2.3x the cap) and reported as a live route.
**It was not.** The server only ever registers the advanced executor set; the
basic file tools had not been agent-reachable since the basic/advanced split,
and `CreateWorkspaceToolExecutors()` was consumed by one dev harness. A stale
comment at `base_orchestrator_tools.go:44` claimed `"workspace_tools:*"`
resolved to it, which had not been true for as long.

The measurements were real; the reachability was not. The basic executors have
since been removed precisely because reading them in `pkg/workspace/tools.go`
leads a reasonable auditor — human or agent — to the wrong conclusion. Check
tool *registration*, not tool *definition*, before calling a route live.

## Why the CLI tells the agent to read a path it cannot read

Worth recording, because it looks like a CLI bug and is not.

Claude Code's spill message says *"Use offset and limit parameters to read
specific portions of the file"*. Those are the parameters of its built-in
`Read` tool. In an ordinary Claude Code session that advice is correct and
optimal: the file is local, `Read` opens any path, it paginates, and re-running
the command would be wasted work.

This platform disables that tool. `mcpagent/agent/prompt/builder.go:22` tells
the agent not to use provider-native filesystem tools, and
`mcpagent/agent/agent.go:1896` restricts it to declared tools. So the agent is
left holding three individually correct instructions:

| source | instruction |
|---|---|
| Claude Code CLI | read the spill file with `Read(offset, limit)` |
| mcpagent prompt | do not use `Read`; only declared MCP tools |
| folder guard | the declared shell tool may not touch that path |

Under this platform, re-running with less output is the ONLY recovery, and
reading the saved file is impossible — the exact inverse of what the vendor
message advises. `codingCLIToolResultSpillDenial` exists to override that
advice, but it can only fire after the agent has already tried and been denied.

The message ships inside the `claude` binary. It cannot be edited, and it will
reappear whenever a result goes over. The only controllable variable is whether
a result ever goes over — which is why prevention, not recovery, is the fix.

## Which component produced the spill — and a second mechanism found while checking

The claim "Claude Code wrote this file" was challenged and re-verified on
2026-08-05, because mcpagent turns out to have its own tool-output offloader.
Four independent discriminators, not just the string match:

| | mcpagent's offloader | the file actually observed |
|---|---|---|
| filename | `tool_20260805_150405_123456789_1_read_skill.txt` | `mcp-api-bridge-read_skill-1785866860551.txt` |
| timestamp | `20060102_150405` + nanoseconds + counter | `1785866860551` — millisecond epoch |
| prefix | `tool_` | `mcp-api-bridge-` (the MCP server name) |
| folder | `tool_output_folder/<session>/` | `~/.claude/projects/<slug>/<session>/tool-results/` |

Plus the message strings, searched across all three modules
(mcp-agent-builder-go, mcpagent, multi-llm-provider-go):

```text
"exceeds maximum allowed tokens"   0 hits in our repos, 6 in the claude binary
"Output has been saved to"         0 hits in our repos
"REQUIREMENTS FOR SUMMARIZATION"   0 hits in our repos
```

Every `.claude/projects` reference in our code is a reader or a detector —
`strings.Contains(slashed, "/.claude/projects/")` in `execute_shell_command.go`
to improve the denial message, and the transcript registry in
multi-llm-provider-go reading `*.jsonl`. Nothing in our code writes there.

**Conclusion: Claude Code produced the file and the message.** Verifying tool
REGISTRATION and file PROVENANCE beats matching a symptom — the same lesson as
the `read_workspace_file` correction above.

### The second mechanism: `mcpagent/agent/tool_output_handler.go` — NOT traced

Found while checking the above, unexamined, and recorded so it is not
rediscovered as a surprise:

```go
DefaultLargeToolOutputThreshold = 10000    // offload above this
DefaultMaxToolOutputTokenLimit  = 100000   // absolute ceiling
OutputFolder                    = "tool_output_folder"   // RELATIVE path
```

It writes offloaded output with `os.WriteFile` under
`<OutputFolder>/<SessionID>/`. Two open questions:

1. **The budgets disagree by 4x.** mcpagent's ceiling is 100,000 tokens; the
   Claude Code limit that actually rejects a payload is 25,000. So mcpagent will
   pass through results the consumer then refuses — the same "one fact, two
   sources, nothing checking they agree" shape as the rest of this ticket.
2. **`tool_output_folder` is relative.** Where it resolves at runtime decides
   whether an agent can read back its own offloaded output, and whether the
   folder guard permits that path at all. If it lands outside every workspace
   root, this is a SECOND instance of the deadlock described here, reached
   through a different component.

Neither has been traced. Do that before assuming the spill path is the only one.

## Decision: do NOT add `~/.claude` to the folder guard

Considered and rejected on 2026-08-05.

`~/.claude` holds 1,185 project directories — full conversation transcripts for
every session on the machine, including unrelated work — plus `.mcp.json`
(commonly holding API keys), `settings.json`, `history.jsonl`, `file-history/`
and `backups/`. Granting read access there to reach one spill file the agent
produced itself trades a bounded annoyance for an unbounded confidentiality
problem, and re-opens the class of defect
[workspace_docs_path_inside_repo.md](workspace_docs_path_inside_repo.md)
deliberately closed.

If defence in depth is ever wanted, the only defensible scope is the LIVE
session's own directory:

```text
~/.claude/projects/<project-slug>/<session-id>/tool-results/
```

read-only, resolved from the running session id rather than a static path,
justified narrowly because that directory holds output this agent just produced.
Even then it is a hole in a guard whose value is having none — and if the caps
hold, nothing will ever read it.


## Directions considered

Recorded so the next reader does not have to re-derive them. Status as of
2026-08-05 is noted per item.

1. **Pin the limit — NOT DONE.** Set `MAX_MCP_OUTPUT_TOKENS` explicitly so
   branch 1 wins and the value stops depending on a remote gate or a vendor
   default. One line. Cheap, and makes any cap here meaningful. Does not by
   itself stop spills.
2. **Move the cap to the boundary — NOT DONE; `agent_browser` is why it still
   matters.** Enforce one budget where every tool result is serialised, instead
   of per tool. Closes the class rather than the instance. Open question:
   scripted steps parse tool output as schema-validated JSON and must keep every
   byte — the existing cap avoids this by living in the tool executor, and a
   boundary-level cap has to preserve that carve-out. A generic truncation also
   cannot simply cut the JSON: the result must stay parseable, so the safety net
   has to return a valid explanatory envelope rather than a truncated payload.
3. **Make the spill readable — REJECTED 2026-08-05.** See the `~/.claude`
   decision above. A whole-directory grant is out of the question; only a
   live-session-scoped `tool-results/` grant would be defensible, and if the
   caps hold nothing would ever read it.
4. **Make the results smaller — DONE for the skill path.** `read_skill` is one
   file per call, `post-run-monitor.md` is out of the bundle, and the one-time
   migration contract is split out of `review-improve-log.md`. No bundle doc now
   exceeds a single result. This does not help `agent_browser`, whose payload
   size is decided by the page, not by us.

Options 2 and 4 are not alternatives — 4 reduces how often 2 is needed. With 4
done for the skill path, 2 is what remains, and `agent_browser` is the concrete
reason to do it.

**Next action when this is picked up: cap `agent_browser`.** It is the last
known route, and capping one tool is a smaller change than the chokepoint
refactor while closing the same live gap.

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
