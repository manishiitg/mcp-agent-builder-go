# Bug Report: Workflow Agents Fail SQLite WAL Reads and Guess Database Schemas

## Status

Implemented locally through the production MCP-bridge boundary as of
2026-08-02. Focused Go tests and the bridge-to-workspace SQLite E2E pass. A full
LLM-driven Pulse pass and ordinary scheduled workflow run remain rollout
verification, not missing implementation.
Open-site audit completed 2026-08-01: two affected call sites, one of them
(`loopclosure`) outside the agent path and previously unlisted — see "Every
affected open site". The reproduction and the proposed `PRAGMA query_only`
containment have both been verified directly against SQLite.
The original diagnosis blamed the agent shell sandbox / folder guard. A direct
SQLite reproduction disproved that explanation. The first observed incident was
a standalone Pulse Fixer, but the defect applies to any workflow step or
background agent that opens a live WAL database with `-readonly`/`mode=ro` while
SQLite needs to create the shared-memory sidecar.

## Summary

Workflow agents can repeatedly spend tool calls recovering from two separate
database-inspection failures:

1. `sqlite3 -readonly <absolute DB path>` fails with SQLite error 14 even though
   the database exists and is healthy.
2. After retrying with `immutable=1`, the Fixer chains schema discovery and a
   prewritten data query in the same shell command. The query therefore guesses
   column names before the agent has seen the schema and fails again.

The concrete incident was not a read-only reviewer. It was:

```text
Pulse fixer: standalone pulse fixer 20260801T122006Z
```

Pulse Fixers are deliberately granted bounded mutation authority, including the
workflow `db/` folder and Pulse lifecycle writer tools. The initial open failure
is therefore not explained by the Fixer role being intentionally read-only.

The same SQLite condition can occur in ordinary workflow steps. It is not a
Pulse-only defect and should be repaired in the common workflow database access
contract rather than patched only in one Fixer prompt.

## Observed Reproduction

Workflow:

```text
Workflow/social-media
```

Database:

```text
<workspace-docs>/Workflow/social-media/db/db.sqlite
```

The Fixer first ran:

```bash
sqlite3 -readonly <workspace-docs>/Workflow/social-media/db/db.sqlite ".tables"
```

Observed:

```text
Error: unable to open database file
Error: in prepare, unable to open database file (14)
```

It then retried with:

```bash
sqlite3 -readonly 'file:<workspace-docs>/Workflow/social-media/db/db.sqlite?immutable=1' ".schema pulse_review_log"
```

That opened the database and printed the schema. However, the same shell command
had already appended a query using these guessed columns:

```sql
created_at,
markdown,
verifications_json
```

The real `pulse_review_log` columns include:

```text
recorded_at
artifact_markdown
```

There is no `verifications_json` column. Fix verification records live in the
separate `pulse_fix_verifications` table.

## Evidence That the Database Is Healthy

At investigation time:

- `db.sqlite` existed and was readable.
- `db.sqlite-wal` and `db.sqlite-shm` existed.
- `PRAGMA journal_mode` returned `wal` on a normal read-only connection.
- The WAL file was empty at the sampled moment.
- Outside the agent shell sandbox, `sqlite3 -readonly <same absolute path>`
  opened successfully.
- The database contained 107 schema objects and returned the expected
  `pulse_review_log` schema.

The database is indeed healthy. This report originally read the list above as
evidence of an execution-boundary interaction between WAL/SHM access, the folder
guard or host-path mount, and the shell tool. **That inference was wrong**, and
the list is in fact the clearest evidence for the real cause — see Root Cause 1.

Two of those bullets were taken while a writer held the database open, which had
materialized `-shm` and allowed the read-only open to succeed. The meaningful
variable between the failing and succeeding observations was **sidecar state**,
not where the shell was running.

## Root Causes

### 1. A WAL database cannot be opened read-only without an existing `-shm`

This is plain SQLite semantics, not a sandbox, folder-guard, or mount issue.

A WAL-mode database requires its `-shm` shared-memory index to be present before
any connection — including a reader — can use it. If `-shm` does not exist, the
opening connection must create it. `-readonly` forbids that, so the open fails
with `SQLITE_CANTOPEN` (14) and the misleading message `unable to open database
file`. `immutable=1` succeeds because it bypasses WAL and locking entirely.

Reproduced 2026-08-01 on the exact database above:

```text
WAL db, no -shm present   →  sqlite3 -readonly    →  error 14
one read-write open       →  -wal and -shm created
same file, -readonly      →  107 schema objects   ✓
```

The database was then copied to a **fully writable scratch directory** — no
folder guard, no host-path mount, no MCP bridge — where `-readonly` failed
identically. The execution boundary is not involved at any point.

This is why the failure looked intermittent and environment-dependent. Whether
`-shm` is present depends on SQLite connection lifecycle, checkpoint/cleanup
behavior, persistence settings, and abnormal exits. A writer commonly creates
it, but it may remain afterward. The relevant condition is whether the sidecar
exists when the read-only open occurs—not whether the command happens to run in
an agent shell.

`immutable=1` remains the wrong general contract, and for a stronger reason than
originally stated. It does not merely risk staleness: on a live database with a
non-empty WAL it **silently ignores committed data held in the WAL**, returning a
confidently wrong answer rather than an error. That is a correctness hazard for a
Fixer deciding whether a fix landed.

### 1a. Every affected open site (audited 2026-08-01)

`mode=ro` fails exactly as `-readonly` does — neither can create `-shm`.
Confirmed by reproduction on a checkpointed WAL database with its sidecars
removed:

```text
mode=ro + query_only(true)   →  error 14 (unable to open database file)
mode=rw + PRAGMA query_only  →  row returned
```

| Site | Open | Status |
|---|---|---|
| `workspace/handlers/query.go` (`openQueryOnlyDB`, backs `POST /api/query`) | `mode=rw&_pragma=query_only(true)` | fixed; WAL regressions added |
| `agent_go/pkg/loopclosure/loopclosure.go` | `mode=rw&_pragma=query_only(true)` | fixed; no-sidecar observation test added |
| `agent_go/.../run_concerns.go:153` (`openRunConcernsDB`, all Pulse lifecycle IO) | plain `sql.Open("sqlite", dbPath)` | not affected |

The loopclosure site is easy to miss and fails worse than the handler. It reads
workflow state to observe loop closure, so a failed open surfaces as a
`PingContext` error inside an observability layer rather than as a visible tool
failure — the workflow keeps running and simply stops being watched. Its comment
states the reasoning that produces this bug in the first place:

```go
// Read-only: this layer observes, it never mutates workflow state.
```

That intent is correct. `mode=ro` is the wrong mechanism for expressing it,
because SQLite's read-only open is about *file access*, not about whether the
caller intends to write. `PRAGMA query_only` is the mechanism that means what
this comment means. Any future read-only open should be reviewed against this
distinction rather than against the author's intent.

### 2. Schema discovery and querying happen in one model action

The Fixer issued `.schema` and `SELECT` in one shell command. Although the schema
appears above the error in the tool result, the model had already written the
incorrect `SELECT` before receiving that result. It never had an opportunity to
adapt the query to the observed columns.

This pattern creates the appearance that the agent ignored the schema when it
actually never received a reasoning turn between discovery and use.

### 3. Pulse internals are exposed as an undocumented relational schema

The Fixer needs concepts such as reviews, findings, attempts, and verification,
but it is forced to infer their storage layout from table names. The relational
schema has evolved (`artifact_markdown`, separate verification rows), while the
agent naturally guesses generic names such as `markdown`, `created_at`, and
`verifications_json`.

Every stateless Fixer can therefore repeat the same discovery and guessing cost.

## Why WAL Should Remain Enabled

Removing WAL is not the fix. Workflow execution, the server/UI, Pulse, and
background reviewers may read or write the same workflow database concurrently.
WAL allows readers and a writer to make progress with substantially less
blocking than the rollback journal and preserves committed WAL records across a
crash. Reverting journal mode would trade this access bug for more lock
contention, longer commits, and poorer runtime behavior.

The defect is the way agents open a live WAL database, not WAL itself.

## Proposed Solution

### Immediate containment: stop using `-readonly` in write-authorized sessions

For a Pulse Fixer or workflow step that already has bounded write access to the
workflow `db/` directory, replace:

```bash
sqlite3 -readonly "$DB_PATH" 'SELECT ...'
```

with:

```bash
sqlite3 "$DB_PATH" 'PRAGMA query_only=ON; SELECT ...;'
```

The connection is opened read-write-capable so SQLite can create or update its
WAL/SHM sidecars, while `query_only` prevents ordinary mutations through that
connection. This is an interim compatibility measure, not the final security
boundary: an agent controlling arbitrary SQL could issue
`PRAGMA query_only=OFF`, so prompt text alone is not sufficient isolation.

Continue to require `$DB_PATH` supplied by the harness rather than copied or
reconstructed absolute paths. Never use `immutable=1` against a live workflow
database.

### Permanent common database interface

Implement the database capability once in the workflow-builder/backend and
expose it selectively to workflow steps and managed background agents. Do not
build separate copies for Pulse, reviewers, and workflow steps.

The existing `agent_configs.db_access` remains the trusted source of authority;
do not introduce a parallel model-controlled permission. Its current semantics
are:

```text
db_access="read"       → db is readable but absent from FolderGuard write paths
db_access="read-write" → db is in FolderGuard read and write paths
db_access absent/other  → read-write (backward-compatible default)
```

Evaluation steps are forced to effective read access unless their evaluation
contract explicitly permits a DB write. A message-sequence item may narrow the
step's effective write access for that turn but may never escalate beyond the
step-level `db_access`. The current configuration has no `none` DB mode; adding
one would be a separate least-privilege feature rather than part of this repair.

There should be at most two agent-facing tools:

#### `query_workflow_db`

This tool should:

- Resolve an existing database from the authorized workflow/session; it must
  not accept an arbitrary host database path from the model.
- Open the database in existing-file `mode=rw`, not `mode=rwc`, so SQLite can
  manage WAL/SHM but cannot silently create a database at a wrong path.
- Immediately apply `PRAGMA query_only=ON` and a bounded busy timeout.
- Enforce read-only behavior server-side with SQLite authorization and statement
  validation. Allow bounded `SELECT`, safe read-only `PRAGMA`, and `EXPLAIN`;
  reject writes, `ATTACH`, extension loading, unsafe PRAGMAs, and extra
  statements. `query_only` is defense in depth, not the only guard.
- Support schema discovery (`describe`/table metadata) so agents do not guess
  table or column names.
- Return structured columns and rows rather than shell-formatted text.
- Bound rows and response bytes. Save oversized complete output under the
  agent's working directory using the bridge large-output contract and return
  its path and original size.
- Surface SQLite error code, operation, resolved workflow-relative path, and
  relevant locking/WAL state when a query fails.

Although the connection is read-write-capable at the filesystem level, the tool
is logically and enforceably read-only. This distinction is necessary because a
strict filesystem/SQLite read-only open may be unable to materialize `-shm`.

This is not entirely new infrastructure. The repository already has a
`QueryWorkflowDB` client and workspace `POST /api/query` handler used by
foreach, pre-validation, and report/database UI reads. That handler currently
opens with `mode=ro`, so it can reproduce this bug. Repair and harden that common
path, then expose an authorized agent-facing wrapper. Do not merely change its
DSN to `mode=rw`: first add the server-side query authorization above, because
the endpoint currently accepts caller-supplied SQL.

#### `mutate_workflow_db`

This tool should be available only when the execution context has explicit
workflow database write or read-write authority. It should:

- Resolve the same authorized workflow database server-side.
- Open the existing database normally with WAL and a bounded busy timeout.
- Execute permitted `INSERT`, `UPDATE`, or `DELETE` operations in a transaction.
- Roll back the complete operation on any error.
- Return affected-row counts and structured, bounded result data.
- Reject arbitrary paths, `ATTACH`, extension loading, journal-mode changes,
  unsafe PRAGMAs, and schema mutations.
- Record enough execution metadata for a Fixer attempt or workflow run to audit
  what changed without logging sensitive row contents unnecessarily.

Where narrow typed lifecycle writers already exist, Pulse Fixer should continue
using those instead of general SQL. The mutation tool is the controlled escape
hatch for authorized workflow-owned data repairs, not a replacement for every
domain-specific API.

### Schema migrations are internal, not a third agent tool

`CREATE`, `ALTER`, and `DROP` should not be exposed as routine agent mutations.
Workflow schema upgrades belong to the builder/harness version-upgrade path as
versioned, idempotent migrations. Consequently, this design adds two
agent-facing tools—not three tools to every agent.

### Capability and exposure matrix

The server must decide tool availability from trusted execution configuration;
the model must not self-declare access.

| Execution role | Query | Mutate | Direct `db.sqlite*` shell access |
|---|---:|---:|---|
| Reviewer | yes | no | remove after migration |
| Strategy/ops background agent | yes | no | remove after migration |
| Pulse Fixer | yes | yes, bounded to delegated workflow/run | remove after migration |
| Read-only workflow step | yes | no | remove for agentic steps after migration |
| Read-write agentic workflow step | yes | yes | remove after migration |
| Explicitly authorized application/script step | as needed | as needed | retain temporarily for compatibility |

For agent-facing tool registration, map the existing setting directly:

```text
db_access="read"       → expose query_workflow_db
db_access="read-write" → expose query_workflow_db + mutate_workflow_db
db_access absent        → expose both (current backward-compatible default)
```

The server reads this capability from trusted execution configuration; the
model cannot request or widen it in a tool argument.

### Folder-guard policy and compatibility rollout

The target is not to block the entire `db/` tree. It contains three distinct
surfaces with different access needs:

```text
db/README.md      schema/ownership contract; keep readable
db/assets/        durable workflow files; retain read/write per step authority
db/db.sqlite*     database plus WAL/SHM sidecars; block from managed-agent shell
```

For managed reviewers, Pulse agents, and agentic workflow steps, FolderGuard
should therefore exclude `db/db.sqlite`, `db/db.sqlite-wal`, and
`db/db.sqlite-shm` from direct shell/file access after the tools are proven. It
should still expose `db/README.md` for schema context and `db/assets/` according
to the step's access. The query/mutation backend resolves the database from the
authorized workflow/session and operates outside the step's filesystem sandbox.

Once raw SQLite access is blocked for an agentic step, do not inject or advertise
`$DB_PATH` to that agent; doing so invites a command that FolderGuard must deny.
The tools must not require a path argument from the model.

Do not immediately apply that restriction to every workflow step. Existing
Python, Go, shell, and application steps may legitimately open SQLite from saved
code, and an abrupt file-level guard change would break them. Preserve an
explicit, trusted compatibility capability for those steps until they are
migrated; do not infer direct access merely because a model asks for it.

Roll out in phases:

1. Add the common query and mutation tools with authorization and WAL tests.
2. Update all generated guidance, slash-command prompts, Pulse prompts, and
   workflow-step instructions to prefer the tools.
3. Add telemetry for remaining direct SQLite shell access and verify existing
   workflows through real bridge/harness runs.
4. Replace the current directory-wide DB grant with precise access: retain
   `db/README.md` and authorized `db/assets/`, while blocking `db.sqlite*` for
   managed reviewers, strategy/ops agents, and Pulse Fixer once their tool path
   is proven.
5. Apply the same database-file block to agentic workflow steps. Map their
   existing `db_access` to query/mutation tools instead of raw file access.
6. Preserve and audit a trusted direct-DB compatibility path for explicitly
   authorized application/script steps until their code is migrated.

Folder guard remains the outer filesystem boundary. The database tools provide
the finer read-versus-write boundary. Merely changing prompts while leaving raw
database access universally available is guidance, not enforcement.

**What "proven" must mean in phases 4–6.** Removing raw `db.sqlite*` access is
the highest-risk rollout step here: if the tools have a gap, every agent that relied
on the shell loses its fallback at once, and the symptom will be agents blocked
mid-run rather than a failing test. Proven therefore means the tool path has
carried real work through the production MCP bridge — at minimum one full Pulse
pass and one ordinary workflow run per affected role — not a green unit suite.
The P0 tests below are necessary and not sufficient for that gate.

### Correct the generated workflow-step message

The prompt tells a non-evaluation execution step that it may write and upsert
`db/db.sqlite` even when effective `db_access="read"`. FolderGuard still denies
the write, but the message contradicts the enforced permission and causes
avoidable failed tool calls.

Precisely: the conditional exists but keys off the wrong value. At
`execution_only_agent.go:77` the branch is

```text
{{if eq .IsEvaluationMode "true"}}   → READ-ONLY workflow evidence, $DB_PATH for SELECT only
{{else}}                             → workflow state and results, INSERT ... ON CONFLICT upsert
```

so evaluation steps are already handled. What is missing is a branch on
`db_access` itself, which is why a *non-evaluation* step with `db_access="read"`
still receives the write-and-upsert paragraph. This is a wrong discriminator, not
an absent one — the fix is to derive the section from effective access (with
evaluation mode as one input), not to introduce conditionality that is not there.

Generate the DB section from effective access instead:

- `read`: describe the DB as read-only evidence and instruct the agent to use
  `query_workflow_db`; do not show write examples or `$DB_PATH`.
- `read-write`: describe query and mutation tools and their transaction/upsert
  rules; do not show raw SQLite examples to managed agentic steps.
- explicit application/script compatibility: inject `$DB_PATH` and document
  direct SQLite behavior because the saved program, not the model, owns that
  compatibility path.

The printed Allowed READ/WRITE lists remain useful diagnostics, but they should
agree with the dedicated DB instructions rather than serving as the only signal
that a generated write example is forbidden.

### Implemented shape (2026-08-01)

- Added shared `query_workflow_db` and `mutate_workflow_db` virtual tools.
- Hardened `/api/query` with existing-file `mode=rw`, `query_only`, statement
  validation, safe schema PRAGMAs, row bounds, and stacked-statement rejection.
- Added token-protected `/api/mutate` with INSERT/UPDATE/DELETE-only validation,
  transactions, rollback, and affected-row receipts.
- Mapped effective `db_access` to tool exposure, including evaluation downgrade
  and message-sequence write narrowing.
- Updated regular/message-sequence/todo-task prompts to use the managed tools;
  saved scripted steps retain `$DB_PATH` compatibility.
- Hard-blocked the three concrete paths `db/db.sqlite`, `db/db.sqlite-wal`, and
  `db/db.sqlite-shm` for managed agentic/background sessions while leaving
  `db/README.md` and `db/assets/` under their normal folder policy.
- Changed loop closure to the same WAL-capable query-only connection contract.
- Added checkpointed-no-sidecar, active-WAL visibility, unsafe SQL, transaction
  rollback, missing-path, read-only capability, and raw-file-block regression tests.

### Remaining gaps closed (2026-08-02)

The follow-up audit found five concrete gaps in the first implementation. They
are now closed in this order:

1. **Read-only scripted snapshot.** Saved scripts and their repair shells no
   longer receive the live WAL database when effective `db_access="read"`.
   The trusted workspace service uses SQLite `VACUUM INTO` to create a
   transactionally consistent, standalone per-process snapshot under the
   step's authorized output folder, replaces `DB_PATH` for that process, and
   removes the snapshot afterward. The regression keeps a non-empty WAL open
   and proves the snapshot sees its committed row without needing snapshot
   WAL/SHM sidecars. Read-write scripts continue to receive the live DB.
2. **Exact sidecar blocking.** Managed sessions explicitly block
   `db.sqlite`, `db.sqlite-wal`, and `db.sqlite-shm`; the policy no longer relies
   on a comment or prefix interpretation to cover the sidecars.
3. **All workflow background-agent query surfaces.** A shared read-only
   background-reviewer bundle now gives `query_workflow_db` (and never
   `mutate_workflow_db`) to plan, timing, cost/ops, and saved-code reviewers.
   Goal Advisor/Pulse stages already share an allow-list whose invariant test
   requires the query tool. Generic background tasks and background todo
   agents already use the normal capability-derived workflow tool bundle.
   KB consolidate/reorganize agents now also receive query-only DB access and
   a read-only managed DB session. This removes the one-reviewer-at-a-time
   patch pattern: every workflow-scoped background agent can inspect structured
   state, while only explicitly authorized writers can mutate it.
4. **Fail-closed mutation authorization.** `mutate_workflow_db` now requires
   the trusted session value to equal `read-write` exactly. Missing/empty
   authority is denied. The long-lived main Builder session is explicitly
   configured as read-write, as are the already-bounded workflow/Pulse writer
   sessions; no legitimate writer depends on the previous missing-value
   fallback.
5. **Production bridge E2E.** `TestWorkflowStepDatabaseToolsThroughMCPBridge`
   runs the production stdio MCP bridge, production workflow DB executors,
   workspace `/api/query` and `/api/mutate` handlers, and real WAL-mode SQLite.
   It proves a workflow-step session reads a committed WAL row and writes an
   authorized `WITH ... INSERT` mutation back to the live database without
   calling an executor directly from the test.

### Follow-up: CTE-prefixed mutations rejected (fixed 2026-08-02)

The first mutation validator authorized SQL from only its first keyword. That
correctly accepted direct `INSERT`, `UPDATE`, and `DELETE`, but rejected valid
SQLite mutations such as `WITH incoming(...) AS (...) INSERT ...`. This was a
harness defect, not an agent SQL mistake: SQLite accepted the statement, but the
workspace authorization layer returned HTTP 400 before execution.

The validator now parses the narrow SQLite `WITH` envelope, skipping quoted
values, comments, and balanced CTE bodies, and authorizes only when the actual
top-level statement after all CTE definitions is `INSERT`, `UPDATE`, or
`DELETE`. It remains fail-closed for `WITH ... SELECT`, DDL, PRAGMA, malformed
CTEs, and stacked statements. The public tool description now explicitly tells
agents that CTE-prefixed mutations are supported.

Verification covers:

- a focused regression that failed with the original HTTP 400;
- accepted single, multiple, recursive, and materialization-hinted CTEs;
- rejected read-only, schema-changing, malformed, and stacked CTE forms;
- the production stdio MCP-bridge E2E using `WITH ... INSERT`;
- a deployed workspace-server smoke test on a disposable database, including a
  forced second-statement failure proving the first CTE mutation rolled back.

### Final workflow-agent coverage

| Agent/runtime surface | Query | Mutate | Raw live SQLite |
|---|---:|---:|---:|
| Plan, timing/ops, cost, saved-code reviewers | yes | no | blocked |
| Goal Advisor/Pulse read-only stages | yes | no | blocked |
| Pulse Fixer stage | yes | yes, explicit delegated session | blocked |
| Generic `run_in_background` Builder child | yes | yes when its trusted broad Builder write session grants it | blocked |
| Background todo agent | yes | follows effective step `db_access` | only scripted compatibility |
| KB consolidate/reorganize | yes | no | blocked |
| Agentic workflow step | yes | only with effective `db_access=read-write` | blocked |
| Scripted workflow step, effective read | via consistent `DB_PATH` snapshot | no live mutation | snapshot only |
| Scripted workflow step, effective read-write | available as configured | yes | live compatibility path |

Chief of Staff is intentionally not listed as a workflow DB agent. It is
org-scoped and has no single current workflow from which `query_workflow_db`
could safely resolve a database. Cross-workflow Chief of Staff inspection needs
a separate typed, workflow-addressed read API or delegation into a specific
workflow session; silently choosing one workflow would violate the pathless
authorization design.

### Verification completed on 2026-08-02

The following suites pass after the final background-agent expansion:

```text
workspace: go test ./handlers ./models
agent_go:  go test ./pkg/workspace ./cmd/server/virtual-tools ./pkg/orchestrator/agents/workflow/step_based_workflow
agent_go:  go test ./cmd/server -run '^TestWorkflowStepDatabaseToolsThroughMCPBridge$' -count=1
```

The tests cover committed active-WAL visibility in the scripted snapshot,
explicit main/WAL/SHM blocking, query-only background surfaces, denial when DB
write authority is absent, transactional mutation receipts, and the production
stdio MCP bridge path. A full LLM-driven Pulse pass and a scheduled ordinary
workflow run remain rollout smoke tests rather than unit/transport gaps.

### Prefer typed Pulse lifecycle tools where available

For Pulse-owned concepts, prefer even narrower typed tools over general SQL.
Half of these already exist; the rest are proposals, verified 2026-08-01:

| Tool | Status |
|---|---|
| `get_pulse_review_result` | exists — `interactive_workshop_manager.go` |
| `get_pulse_module_state` | exists — `pulse_worklist.go` |
| `get_pulse_finding_backlog` | exists — `pulse_worklist.go` |
| `list_pulse_reviews` | does not exist |
| `get_pulse_fix_attempt` | does not exist |
| `get_pulse_verifications` | does not exist |

`get_pulse_verifications` is the highest-value gap: verification records are
exactly what the Fixer was hand-querying when it guessed `verifications_json`,
and they live in `pulse_fix_verifications` (columns `attempt_id`, `fingerprint`,
`check_text`, `verdict`, `expected`, `observed`, `evidence_json`, `verified_at`).

The existing typed Pulse tools should be used wherever they already cover the
Fixer's question. General SQL should remain an escape hatch for workflow-owned
data, not the primary Pulse lifecycle interface.

### Keep schema observation separate from dependent queries

The typed query tool now exists, but any agent querying an unfamiliar table
must still inspect it first and wait for that result before constructing a
dependent query. Raw-SQL compatibility guidance should state:

> When a table schema is not already known, inspect it in one tool call. Wait for
> that result, then construct the data query in a later tool call. Never chain
> `.schema`/`PRAGMA table_info` and a query against guessed columns in one shell
> command.

The guidance should also require `$DB_PATH` rather than a copied or reconstructed
absolute path. The harness, not the model, owns path resolution.

### Do not standardize `immutable=1` for live Pulse reads

If a consistent read snapshot is required, create it through a trusted backend
using SQLite's backup facilities (`VACUUM INTO` or the backup API) and query
that snapshot. Do not ask agents to copy the database, copy only `db.sqlite`
without WAL state, or guess URI modes from shell.

## Minimum Safe Interim Fix

Three interim rules. No backend work is required to stop the reported failure.

1. In a session already authorized to write `db/`, do not pass `-readonly` when
   inspecting the live workflow database. Open normally and immediately enable
   `PRAGMA query_only=ON` for the inspection connection. This permits WAL/SHM
   maintenance while preventing accidental data writes.
2. Never chain schema discovery and a dependent query in one shell command.
   Inspect the schema in one tool call, wait for the result, then write the query
   against the columns actually returned.
3. For genuinely read-only roles, do not grant raw shell access as a workaround.
   Route reads through the trusted backend query tool once available.

Then, as ordinary follow-up work rather than emergency fixes:

4. Route Pulse lifecycle reads through the existing typed Pulse tools.
5. Remove shell examples that encourage raw `sqlite3` inspection for Pulse
   internal tables.

The earlier version of this list opened with two new backend tools. That was
scoped against the sandbox hypothesis and is no longer the minimum.

## Acceptance Tests

### P0: checkpointed WAL database with no sidecars

The decisive condition is **not** the folder guard — it is whether `-shm` exists.
The original version of this test specified "a live connection" and "exercise the
production folder guard", which would have passed or failed on writer timing and
confirmed the wrong cause.

Create a workflow database in WAL mode, commit and checkpoint a known row, close
every writer, and remove any empty `-wal`/`-shm` sidecars. Then the production
query tool must:

- succeed without `immutable=1`;
- observe the latest checkpointed row from the main database;
- return within a bounded timeout.

This test proves that the tool can materialize the WAL shared-memory state when
no sidecars exist. It intentionally does not claim that data remains in a WAL
file that the fixture has deleted.

### P0: active non-empty WAL visibility

Hold a writer connection open with WAL auto-checkpointing disabled, checkpoint a
first row, then commit a second row that remains only in the non-empty WAL. The
production query tool must return both rows. As a negative control, show that an
`immutable=1` connection returns stale state or otherwise cannot satisfy the
assertion. This proves the solution reads committed WAL data rather than merely
making the open error disappear.

Run both P0 WAL cases through the production access implementation with and
without an independently attached writer where applicable.

### P0: write authorization and transaction behavior

Verify that:

- an authorized write/read-write step and Pulse Fixer can perform a permitted
  mutation and read it back;
- a reviewer and read-only step cannot call the mutation path;
- attempted `ATTACH`, DDL, unsafe PRAGMA, extension loading, or arbitrary-path
  access is rejected;
- a multi-operation failure rolls back the entire transaction;
- a wrong or missing database path produces a clear error and never creates a
  new empty database.

### P0: every read-only open site, not just the agent path

Run the checkpointed-WAL fixture (no sidecars) against **each** site in the
affected-sites table, not only the agent-facing tool. Loop closure must observe
the workflow and `POST /api/query` must return rows under the same condition
that currently produces error 14.

Loop closure needs an explicit assertion that it *observed*, not merely that it
did not error: its failure mode is a silent stop, so a test asserting "no panic"
would pass against the broken build.

### P0: standalone Pulse Fixer inspection

Run a standalone `/pulse-fixer` through the production MCP bridge. It must load
reviews, findings, attempts, and verifications without calling raw `sqlite3` for
Pulse lifecycle state and without any failed tool call. It must also complete an
authorized data repair through a typed lifecycle writer or
`mutate_workflow_db`, followed by a successful read-back.

### P0: ordinary workflow-step coverage

Run equivalent read and write cases through actual workflow steps rather than
testing only Pulse. A read-only step must query successfully; a write-enabled
step must commit a transaction; and both must observe active WAL data correctly.
Assert that tool exposure follows effective `db_access`, including the current
absent-field read-write default and message-sequence narrowing. The generated
read-only prompt must not recommend `$DB_PATH`, raw `sqlite3`, or a write/upsert.

### P1: schema-before-query behavior

Give the agent an unfamiliar table whose columns intentionally differ from
generic guesses. Verify that schema discovery completes in one tool call and the
subsequent query uses only returned column names.

### P1: bounded large results

Return more rows than the inline byte limit. Verify that:

- the MCP response is bounded;
- the complete result is saved under the agent working directory;
- the returned result includes the full output path and original size;
- a subsequent query on the same bridge remains usable.

### P1: authorization

Verify that a reviewer can perform read operations but not lifecycle writes, and
that a Pulse Fixer can use its delegated lifecycle writers only for its trusted
`pulse_run_id`. The database read tool must not broaden either role's mutation
authority.

### P1: compatibility and folder-guard migration

Verify that an explicitly authorized legacy application/script step can still
open its database during the compatibility period, while managed background
agents whose direct access has been retired cannot bypass the database tools via
shell. Verify independently that those managed agents can still read
`db/README.md` and can read/write `db/assets/` only as authorized. Record
remaining direct SQLite use so removal is evidence-driven.

## Non-Solutions

- Retrying arbitrary combinations of `-readonly`, `mode=ro`, and `immutable=1`.
  `mode=ro` fails for the same reason as `-readonly`: neither can create `-shm`.
- Attributing the failure to the sandbox, folder guard, or host-path mount. It
  reproduces in a fully writable directory with no guard involved.
- Copying `db.sqlite` alone to `/tmp` while ignoring WAL state.
- Granting unrestricted raw database writes to avoid a read-open failure.
- Removing WAL and accepting rollback-journal contention.
- Exposing schema migration as a general agent tool.
- Blocking the entire `db/` directory and thereby breaking schema-document and
  durable-asset access when only `db.sqlite*` needs to be hidden.
- Removing all direct workflow-step database access before existing application
  and script steps have a compatible migration path.
- Increasing model instructions while leaving the SQLite execution contract
  undefined.
- Assuming the agent can use schema output produced earlier in the same shell
  command to rewrite a query that was already sent.

## Expected Outcome

Workflow steps, Pulse Fixers, and reviewers should spend their reasoning budget
on workflow work, not repeatedly debugging SQLite access or rediscovering
internal schemas. Reads and writes should use one common, deterministic,
bounded, authorization-aware implementation across providers. WAL remains
enabled; managed agents receive only the query/mutation capabilities their role
requires; `db.sqlite*` is no longer reachable through their shell while
`db/README.md` and authorized `db/assets/` remain available; and direct SQLite
access is retired gradually without breaking explicitly authorized application
code.
