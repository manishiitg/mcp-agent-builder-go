[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-003 — granted DB access does not produce a reachable DB tool

| Coordination | Value |
|---|---|
| Assigned agent | `Claude Code` |
| Ticket state | `runtime_reverify` |
| Last synchronized | `2026-08-04` |

> Claim this ticket in this file before implementation. During active work,
> update this fragment rather than the shared index; synchronize the index
> once at handoff, review, or completion.


- **Priority:** P0
- **Owner:** workflow-step capability materialization/API bridge
- **Source finding:** `HARNESS-DBTOOL-NOT-EXPOSED-EXEC-2026-08-03`
- **Primary source database:** `Workflow/build-in-public/db/db.sqlite`
- **Related evidence:** Instagram `route-design-plan`; RTS collectors denied
  direct SQLite and unable to discover the sanctioned DB path; Social Media
  standalone Pulse Fixer `manual-fixer--20260803T164745Z-1785775665916620000`
  spent 27 minutes, then its approved one-row normalization was denied with
  effective `db_access=""`.
- **Problem:** steps with `db_access=read-write` cannot resolve
  `query_workflow_db`/`mutate_workflow_db` as callable tools. The same tools may
  exist behind an undocumented raw `$MCP_CUSTOM` curl route.
- **Impact:** the permission contract and actual capability disagree. Agents
  burn failed calls, abandon persistence, or publish a false claim that the DB
  capability does not exist.
- **Current implementation:** managed DB tools are already capability-derived
  from trusted `db_access`, even when a step has a narrower explicit custom-tool
  list. `read` materializes query only; `read-write` materializes query and
  mutation. Direct SQLite/WAL/SHM paths remain blocked for managed agentic
  sessions, while the mutation executor independently fails closed without an
  explicit read-write grant.
- **Verification (2026-08-03):** the real stdio MCP bridge → custom executor →
  workspace HTTP API → WAL-mode SQLite E2E passes for query and mutation.
  Focused capability tests pass for read-only/read-write exposure and for
  read-only/no-grant mutation denial. The three source findings should be moved
  to platform reverify rather than prompting another workflow-level repair.
- **Fixer follow-up (2026-08-03):** isolated workshop stage tools already used
  the child MCP session directly, but `execute_shell_command` inherited the
  parent workshop's MCP URL. A Fixer calling the managed DB tool through the
  shell bridge was therefore authorized as the wrong session. Workshop stage
  sessions now override the bridge URL/session in their trusted shell env. A
  Pulse Fixer also receives an explicit read-write DB capability and must pass
  a pre-provider capability check covering DB write scope, logical grant,
  child-session bridge routing, and both query/mutation tool executors. Missing
  capability fails before the expensive LLM run; mutation remains fail-closed.
- **Current workaround:** raw `$MCP_CUSTOM/query_workflow_db` and
  `$MCP_CUSTOM/mutate_workflow_db` calls when their exact API is known.
- **Acceptance:** real workflow-step bridge E2E tests for read-only and
  read-write grants prove the matching tools are discoverable and callable;
  no-grant and read-only mutation attempts fail closed.
