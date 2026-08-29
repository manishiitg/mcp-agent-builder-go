[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-221 — workflows have a managed, auditable route to change their own SQLite schema

| Coordination | Value |
|---|---|
| Assigned agent | Claude Code |
| Ticket state | `implemented; runtime reverify` |
| Last synchronized | `2026-08-29` |

- **Priority:** P0 — LinkedIn `PUL-B995BF46` / `PUL-3BD9F422` (per the
  [2026-08-29 triage audit](../../audits/pulse-platform-triage-2026-08-29.md)).
- **Owner:** workflow-owned SQLite schema evolution.
- **Related:** [`pulse_fixer_sqlite_readonly_wal_and_schema_guessing.md`](../pulse_fixer_sqlite_readonly_wal_and_schema_guessing.md),
  which shipped `query_workflow_db`/`mutate_workflow_db` (2026-08-01/02) and
  explicitly deferred schema changes: *"Schema migrations are internal, not a
  third agent tool... belong to the builder/harness version-upgrade path"* —
  a path that was never actually built.

## Problem

Neither `query_workflow_db` (read-only) nor `mutate_workflow_db`
(INSERT/UPDATE/DELETE only, fail-closed on DDL) can create or change a
workflow's own tables, and raw `sqlite3` shell access to `db.sqlite`/`-wal`/
`-shm` is hard-blocked for every managed agentic session. An agent's only
sanctioned action when a step needs new schema is to author a migration
`.sql` file under `db/migrations/` (ordinary file access — not blocked) and
then stop, because nothing is authorized to execute it. This is exactly what
happened for LinkedIn: `db/migrations/2026-08-06-action-outcome-measurement.sql`
was written correctly on 2026-08-06 and had no way to run.

A backend primitive for exactly this already existed
(`POST /api/db/initialize` → `workspace/handlers/query.go:InitializeWorkflowDB`,
`agent_go/pkg/workspace/initialize_workflow_db.go`), but its own doc comment
said *"normal agents do not receive this capability as a tool"* — it was used
by exactly one caller, Video Studio's product runtime, with a fixed,
compiled-in migration list. It was never wired to anything a workflow's own
Fixer or Builder session could reach.

## Fix

**New tool: `apply_workflow_db_migration(migration_file)`**
(`agent_go/cmd/server/virtual-tools/workflow_db_tools.go`)

- Takes a bare filename only (`safeMigrationFileName`, no path separators
  representable at all); resolves it server-side to that session's own
  `Workflow/<name>/db/migrations/<file>`, the same way `query_workflow_db`/
  `mutate_workflow_db` resolve `db/db.sqlite` — the model never supplies a
  path. Never accepts inline SQL.
- Gated by the same fail-closed trust boundary as `mutate_workflow_db`:
  requires the session's `WORKFLOW_DB_ACCESS=read-write` exactly. Schema
  migration authority is not a separate role — Pulse Fixer and the main
  Builder session already share this session/trust boundary in the current
  architecture, and neither has any legitimate reason to need it beyond
  read-write.
- Reads the file through the existing FolderGuard-checked
  `ReadWorkspaceFile` client call, splits it into individual statements
  (quote/comment-aware, so a `;` inside a string literal doesn't create a
  spurious split), drops the file's own `BEGIN`/`COMMIT` transaction
  envelope (the backend already wraps every migration in one transaction),
  and validates each remaining statement against the same allow-listed
  shapes the backend enforces — failing closed with a clear per-statement
  error before the HTTP round trip if something doesn't match.
- Calls `client.InitializeWorkflowDB`, which does the actual work.

**Expanded `InitializeWorkflowDB` allow-list**
(`workspace/handlers/query.go`)

Widened from CREATE-only to the full schema-evolution set a workflow
actually needs, while keeping the two genuinely dangerous statement kinds
out entirely:

- `CREATE TABLE/INDEX IF NOT EXISTS` — idempotent, unchanged from before.
- `DROP TABLE/INDEX IF EXISTS` — idempotent (a bare `DROP TABLE` without
  `IF EXISTS` is rejected, so retrying a migration is always still safe).
- `ALTER TABLE ... RENAME TO / RENAME COLUMN ... TO ... / ADD COLUMN /
  DROP COLUMN` — SQLite has no idempotent form for `ALTER`, so a repeated
  `ALTER` fails loudly on retry instead of silently no-op'ing; that failure
  is safe, just not automatically idempotent.
- **`PRAGMA` and `ATTACH` remain permanently out of scope**, in any file,
  even after this change. `PRAGMA` can change database-wide behavior other
  concurrent readers/writers depend on (journal mode, foreign keys); `ATTACH`
  opens an arbitrary filesystem path entirely outside FolderGuard's
  authorization. Neither has a legitimate migration use case that isn't
  better served by one of the allow-listed shapes above.

**Automatic pre-migration backup for destructive statements.** There is no
human-approval gate on this route. Any statement that can remove an existing
table, column, or the name a caller resolves it by (`DROP`, `RENAME`,
`DROP COLUMN` — never `ADD COLUMN`, which can only add data) triggers a
`VACUUM INTO` snapshot of the live database *before* the migration runs,
written under `db/migrations/.backups/<nanosecond-timestamp>-pre-migration.sqlite`.
The response's `backup_path` is the recovery point if a migration turns out
wrong. Purely additive/idempotent-create migrations never pay this cost.

## Verification

```text
go test ./handlers/...                                    (workspace module)
go test ./cmd/server/... ./cmd/server/virtual-tools/...    (agent_go module)
```

New coverage: idempotent `DROP TABLE IF EXISTS` (including safe re-apply),
a destructive `ALTER TABLE DROP COLUMN` proving the pre-migration backup is
written and the dropped data is recoverable from it, `ADD COLUMN` proving no
backup is taken for the purely-additive case, `RENAME TO`/`RENAME COLUMN`,
`PRAGMA`/`ATTACH` staying rejected, a fresh-database destructive migration
needing no backup, and — through the production stdio MCP bridge, no
executor called directly —
`TestApplyWorkflowDBMigrationThroughMCPBridge`: an agent-authored migration
file is applied, the created table is visible to a fresh direct connection
to the live database (not just the connection the handler used), re-applying
is a safe no-op, and the migrated columns are visible through
`query_workflow_db describe`.

A real bug surfaced by the tests themselves during development: the first
backup-path implementation used second-resolution timestamps, so two
destructive migrations applied within the same second collided on the
backup filename and `VACUUM INTO` refused to overwrite it. Fixed with a
nanosecond-resolution name, matching the same collision-avoidance pattern
already used elsewhere in this codebase for exactly this reason
(`agent_browser_snapshot_<nanos>.txt`).

## Reverify

No live agent turn has called this tool yet through the deployed server —
the running dev server (`+dirty`, pre-existing before this session) was not
restarted as part of this work, since restarting a server actively executing
scheduled Upwork/LinkedIn/Sales Outreach runs is outside this ticket's scope
and needs its own explicit go-ahead. Deploy, then run one Pulse Fixer turn
that authors and applies a real migration end to end.

## Notes on the two originating findings

- **LinkedIn `PUL-B995BF46`** (the ready `2026-08-06-action-outcome-measurement.sql`
  migration) turned out to be **already resolved independently of this
  ticket** — `action_outcome_bindings` and `matched_action_outcome_comparisons`
  already exist in the live `Workflow/linkedin/db/db.sqlite` with real
  producer data dated as early as 2026-08-21, matching the migration file's
  exact schema. The `pulse_finding_details` row was never updated past its
  original `2026-08-11` filing, so the finding record itself is stale — the
  same "real when filed, already fixed, never re-verified" pattern as
  PLAT-191/195/201/202/210/212/213. How the tables were actually created is
  not established (not through this new tool, and not through any ledger
  this ticket adds); reclassify/close the workflow finding on this evidence.
- **LinkedIn `PUL-3BD9F422`** (immutable image-batch identity) is confirmed
  still genuinely open: `post_approval` still has only a single `image_path`
  column and `image_assets` still has no ordered batch/draft-binding
  identity. This is the concrete beneficiary of PLAT-221 going forward — its
  migration SQL has not been designed yet and is deliberately left as a
  separate follow-up, not bundled into this ticket.
