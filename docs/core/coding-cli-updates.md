# Coding CLI updates

The backend checks installed Codex, Claude Code, Cursor Agent, and Pi CLIs on
startup when due, then on an hourly background timer. A successful check is due
again 24 hours later. Each failed CLI is retried after one hour, on the next
timer tick, without re-running updates for the successful providers. This works
on a continuously running server and does not depend on a restart or a workflow
schedule. It makes no model calls and consumes no LLM tokens.

## State and configuration

The durable, atomically written `state.json` lives in:

- `$AGENTWORKS_STATE_ROOT/cli-updates/state.json` when an absolute instance state
  root is supplied.
- Otherwise, Go's user config directory under `agentworks/cli-updates/state.json`
  (normally `~/Library/Application Support/agentworks/cli-updates/state.json` on
  macOS, and `${XDG_CONFIG_HOME:-~/.config}/agentworks/cli-updates/state.json` on Linux).

The JSON contains `schema_version`, `last_attempt`, `last_success`, `next_check_at`
and a `clis` object keyed by executable name. Each CLI records its status,
verified version, selected absolute executable, last attempt, last success,
next check, and any failure. `checking` is saved before starting an update, so
an interrupted attempt is visible after a crash. Statuses are `checking`,
`updated`, `current`, `failed`, `not_installed`, and `operator_pinned`.

The state file is mode 0600 and private directories are 0700. An OS file lock
serializes backend processes sharing the same root and is automatically released
on process exit. Invalid/unknown JSON is preserved and logged as an error;
the updater does not silently reset its history.

`CLI_UPDATE_ENABLED=false` disables the manager and managed launch routing.
`CLI_UPDATE_DRY_RUN=true` also disables automatic installs, preserving the old
deployment's dry-run opt-out. An explicit `PI_BIN` remains an operator pin and
is not updated. Only already installed providers are managed; a newly installed
CLI is discovered at a later check. Native Windows is not supported by this
implementation; the backend's CLI runtime targets macOS and Linux/WSL.

## Installation and chat behavior

The familiar manual commands are `codex update`, `claude update`, `agent update`
(or `cursor-agent update`), and `pi update --self`. The installed Pi command is
`pi`, not `pi-dev`. These commands are useful for personal installations, but
can update shared directories according to the installation method.

For automatic updates, AgentWorks instead installs npm-distributed Codex, Claude,
and Pi into a fresh private prefix, using the official npm registry's `latest`
version. Cursor's official installer runs with a fresh private HOME. Candidate
installers receive no provider credentials, workflow secrets, or user npmrc.
Network proxy and CA configuration is preserved. Each attempt has a 15-minute
deadline and cancellation stops its subprocess group.

Each candidate must pass `--version` (30-second limit); npm packages must report
the requested version. Cursor is also checked with `--disable-auto-update`, the
flag used at launch. A failed candidate is discarded, leaving the selected
executable unchanged. This verifies installation and launchability, **not**
authentication, model access, or compatibility of every provider protocol.
Those still require the isolated workflow acceptance tests.

Backend-local shims select the managed releases. Terminal launches resolve these
before entering the login shell, so shell PATH resets or an old tmux environment
cannot bypass selection. Structured transports find them through the backend
PATH. Explicit executable overrides remain explicit overrides.

On first launch, each application chat atomically pins the selected executable
for that provider in `sessions/<hashed-chat-id>/<provider>`. Follow-up turns and
restored chats use that pin even after an update. New chats use the newly selected
release. Native background updates are disabled for managed Claude and Cursor
launches; Codex's adapter already disables startup update checks. The updater
never changes the user's global CLI files or shell PATH. Until the first managed
install succeeds, the discovered existing executable is the fallback.

Releases and session pins are retained. Do not delete an old release while any
chat may resume it; automatic garbage collection is intentionally not included.
External tools can still modify the original global fallback installation.

## Deployment and isolated verification

New backend builds enable the manager by default. No currently running process
acquires this behavior until it is rebuilt/restarted. The rootless EC2 deploy
disables the former `video-studio-cli-update.timer`; its compatibility script is
now a no-op. This removes the old updater that modified global installations in
place. Deploy the sibling `mcpagent` and `multi-llm-provider-go` changes together
with the backend; the workspace Go replacements exercise them locally.

The standard isolated instance launcher disables CLI updates by default. Opt in
with `CLI_UPDATE_ENABLED=true scripts/run-local-instance.sh ...`; its absolute
state root keeps installations and JSON separate from the regular app. The
updater does not solve an unauthenticated CLI's login requirement.

Offline regression checks (from the corresponding repository/module):

```sh
# mcp-agent-builder-go/agent_go
go test -race ./internal/cliupdate

# multi-llm-provider-go
go test ./llmtypes ./internal/shelllaunch

# mcp-agent-builder-go
bash scripts/test-local-instance.sh
```

These tests use fake CLIs/installers and real subprocesses to exercise timing,
restart persistence, failed updates, retries, lock contention, atomic selection,
concurrent chat pinning, login-shell PATH resets, isolated installer environments,
and failed-candidate cleanup. They do not update any actual installed CLI.

Real download/install acceptance is also available as an explicit opt-in, from
`agent_go`:

```sh
AGENTWORKS_TEST_CLI_INSTALL=all go test ./internal/cliupdate -run '^TestLivePrivateReleaseInstall$' -v -count=1 -timeout 20m
```

It installs into disposable directories without activating the resulting CLIs.
On 2026-09-06 this passed for Codex `0.153.4`, Claude `2.1.263`, Pi `0.85.1`, and
Cursor `2026.09.02-c22c1a3`. Both existing backend processes remained running.
