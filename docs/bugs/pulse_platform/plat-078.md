[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-078 — spilled bridge tool output (large agent_browser snapshots) landed outside every granted read path

| Coordination | Value |
|---|---|
| Assigned agent | unassigned |
| Ticket state | `implemented` — folder-guard fix shipped; the second contributing mechanism (no output-size cap on `agent_browser` snapshot) remains unfixed |
| Last synchronized | `2026-08-10` |

- **Priority:** P2 — a step gets an unrecoverable, unactionable failure when
  a browser snapshot is large enough to overflow the coding CLI's own
  MCP result cap
- **Owner:** step execution folder guard (`setupExecutionFolderGuard`,
  `controller_agent_factory.go`)
- **Found on:** PLAT-073 cluster F, `dd9ede3c` (upwork): *"agent_browser
  snapshot results continue to overflow the tool-result size limit
  post-v1.0.21-purification (toptal-scan-jobs 68,223 chars,
  search-find-and-shortlist 130,705 chars on 2026-08-07), and the
  spilled-result fallback read also fails as outside every workspace root."*

## Root cause

Two independent, contributing mechanisms — only the first is fixed here:

1. **`agent_browser`'s `snapshot` command has no output-size cap.** Claude
   Code's own MCP tool-result cap (independent of, and smaller than,
   mcpagent's own 128 KiB bridge-result limit) truncates a large snapshot and
   spills the untruncated copy to a location under its own CLI project
   directory — the exact same failure shape already named and fixed for
   `read_skill` (`mcpagent/agent/skill.go:21-33`, `maxReadSkillBatchSize=1`,
   *"the coding CLI truncated it against its 25,000-token result cap, wrote
   the full copy under its own project directory, and told the agent to read
   that path — which the workspace folder guard forbids. The agent had no
   legal way to comply."*). `agent_browser` has no equivalent bound.
2. **`MCP_TOOL_OUTPUT_DIR` (mcpagent's own 128 KiB inline-truncation spill
   target) resolves outside the folder guard's grants.** It's set to
   `<CodingAgentWorkingDir>/tool_output_folder`
   (`mcpagent/agent/coding_agents_bridge.go:198-200`), and
   `CodingAgentWorkingDir` resolves to the bare workflow root
   (`resolveCodingAgentWorkingDir`, `pkg/orchestrator/base_orchestrator_agent_factory.go:174-183`,
   fed `bo.GetWorkspacePath()`) — a sibling of `runs/` that
   `setupExecutionFolderGuard` never granted a normal execution step read
   access to. Confirmed against the live filesystem:
   `workspace-docs/Workflow/<name>/tool_output_folder/` exists as a sibling
   of `runs/`, `db/`, `execution/`, holding spilled files up to 16 MB that a
   normal step could not read back.

## Fixed (2026-08-10)

`setupExecutionFolderGuard` (`controller_agent_factory.go:427`) now adds
`<workspace-root>/tool_output_folder` to `readPaths` for every step. This
closes mechanism 2 directly — a step that hits mcpagent's own 128 KiB spill
can now read the result back — without touching
`resolveCodingAgentWorkingDir`, which is shared by every coding-agent step
and would have been a far riskier, broader-blast-radius change to make just
for this one symptom.

Test: `TestExecutionFolderGuardGrantsToolOutputFolderRead`
(`controller_agent_factory_test.go`) — asserts the read grant exists and,
mirroring the sibling run-folder-widening test, that granting it does not
widen access to the bare workflow root.

## Not fixed here — mechanism 1 remains open

`agent_browser`'s `snapshot` command still has no output-size bound. The
128 KiB mcpagent-level spill (now readable, per the fix above) is a strictly
larger boundary than Claude Code's own ~25K-token MCP result cap, so a
snapshot that overflows the CLI's cap but stays under mcpagent's 128 KiB can
still spill to a location this fix does not cover (the CLI's own project
directory, not `MCP_TOOL_OUTPUT_DIR`). The `dd9ede3c` finding's own numbers
(68,223 and 130,705 chars) are consistent with either boundary depending on
exact byte/token conversion, so this ticket does not claim the finding is
fully resolved — only that the read-path half of it is.

Suggested fix location: `pkg/browser/executor.go` / `pkg/browser/tools.go`
(agent_go) — add a size cap or pagination mode to `snapshot`, mirroring
`read_skill`'s "bound the input so the output can never cross the CLI's cap"
strategy, rather than trying to make an arbitrarily large single-call result
readable after the fact.

## Verification

- `go build ./...` clean.
- New test passes; full baseline
  (`go test ./cmd/server/... ./pkg/orchestrator/agents/workflow/step_based_workflow/...`)
  still shows exactly 22 pre-existing failures — no new failures.
- **Not yet reverified live** — requires a restart and an upwork run that
  produces a large `agent_browser` snapshot before `dd9ede3c` can be closed,
  and even then only the read-path half of the finding should be considered
  proven; the snapshot-size-cap half needs separate, dedicated work.

## Acceptance

- A step that triggers mcpagent's 128 KiB bridge-result spill can read the
  spilled file back from `tool_output_folder/` without a folder-guard denial.
- `agent_browser` snapshot pagination/size-capping remains open — do not
  close `dd9ede3c` on this fix alone without confirming which boundary the
  original overflow actually hit.
