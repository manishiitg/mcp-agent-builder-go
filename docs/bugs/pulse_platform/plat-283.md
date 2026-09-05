[← Pulse platform issue index](../pulse_platform_issue_register.md)

# PLAT-283 — A sandboxed step could never install a Python/Node package, on any Linux deployment

| Coordination | Value |
|---|---|
| Assigned agent | Claude Sonnet 5 |
| Ticket state | `fixed`, live-verified on the Dominion deployment |
| Last synchronized | `2026-09-05` |

- **Priority:** P1 — a hard capability wall, not a bug in one workflow: any
  `execute_shell_command` step on any Linux deployment that needed a package
  not already in the run image had no way to get it, full stop.
- **Owner:** `workspace/security/environment.go` (`buildNativeEnvironment`),
  `workspace/security/isolator_linux.go` (`landlockCommand`,
  `packageManagerCacheEnv`, `writableCacheBase`).
- **Related:** PLAT-281 (the same shape of bug — a real capability gap that
  looked like a fundamental sandbox limitation but was actually one missing
  grant — for `exec`, not filesystem cache writes) and PLAT-118 (Landlock
  fail-closed design generally).

## What was found

The Dominion trading workflow's agent reported, and had "tested and
documented" back on 2026-08-31, that installing a Python package inside its
sandboxed shell was not viable at all: `pip3 install yfinance` was refused
because Python is externally-managed (PEP 668); forcing it past that with
`--break-system-packages` then hit a raw permission error; and even
`python3 -m venv` failed the same way. The conclusion on file was that only
what ships in the run image (standard library + a few extras) is usable —
a permanent architectural limit.

That conclusion was wrong, and provably so: Landlock as configured in this
codebase (`landlock_runner_linux.go`) restricts filesystem access only —
there is no `LANDLOCK_ACCESS_NET_*` rule anywhere, and Dominion (like every
other Linux deployment here) has no egress firewall either
(`deploy/dedicated-vm/dominion-hetzner.md` confirms no firewall rules are
part of the deploy). PyPI/npm are reachable. The actual failure was two
separate, both-fixable gaps stacked on top of each other:

1. Native-mode shells (`NATIVE_WORKSPACE=true`, the standard mode per this
   session's STT-engine and Landlock work) never set
   `PIP_BREAK_SYSTEM_PACKAGES=1` — only the Docker environment builder did.
   That's exactly the "externally-managed-environment" refusal.
2. pip, npm, and `venv` all default to caching under `$HOME`
   (`~/.cache/pip`, `~/.local`, a `$TMPDIR`-based build scratch directory).
   A step's Landlock write grant is scoped to its own workspace folder, not
   the real `$HOME` — on Dominion, `HOME=/srv/dominion/home`, entirely
   outside every step's grant. `--break-system-packages` genuinely bypasses
   the PEP 668 check and then hits Landlock's real, correct denial the
   first time pip (or venv) tries to write its cache. That denial looked
   identical to "installs don't work here" from the agent's vantage point,
   but it's a routing gap, not a wall.

## What shipped

- `buildNativeEnvironment` (`environment.go`) now also sets
  `PIP_BREAK_SYSTEM_PACKAGES=1`, matching what `buildDockerEnvironment`
  already did — closing gap 1 for every native-mode deployment, not just
  Dominion.
- `landlockCommand` (`isolator_linux.go`) now appends
  `PIP_CACHE_DIR`, `XDG_CACHE_HOME`, `PYTHONUSERBASE`, `npm_config_cache`,
  and `TMPDIR`, all redirected into a `.cache`/`.local`/`.tmp` subtree of a
  directory the step can actually write to — closing gap 2. The base
  directory is chosen defensively: the step's own `WorkDir` when it falls
  inside a granted write path (the normal case), otherwise the first
  granted write path, otherwise no redirection at all (so a step with no
  write grant fails exactly as it did before, rather than pointing pip at
  somewhere Landlock will also deny). This applies to every Linux
  deployment using this Landlock launcher, not a Dominion-specific
  env-var carve-out — the fix the user explicitly asked to have "generic
  for linux" rather than a one-off patch.

## Verification

- 5 new tests: `TestNativeEnvironmentAllowsPipInstallOnExternallyManagedPython`
  (`environment_test.go`); `TestPackageManagerCacheEnvPointsIntoTheGrantedWritePath`,
  `TestPackageManagerCacheEnvFallsBackWhenWorkDirIsNotWritable`,
  `TestPackageManagerCacheEnvNoOpsWithoutAnyWritePath`, and
  `TestLandlockCommandCanInstallIntoItsOwnCache` (`isolator_linux_test.go`).
- Cross-compiled for Linux and run live on the Dominion box against the
  real, deployed `video-studio-landlock-runner` — all 5 new tests pass, and
  `TestLandlockCommandCanInstallIntoItsOwnCache` reproduces the exact
  incident shape (write `$PIP_CACHE_DIR/probe.whl` inside a Landlock
  sandbox with only the step's own workspace folder granted) and confirms
  the write now lands inside the granted path.
- Full `workspace/security` suite run live on Dominion, both before and
  after this change: one pre-existing failure
  (`TestLandlockEnforcesExternalFolderAccess`) reproduces identically
  against a clean pre-change binary built from the same `origin/main` tip,
  confirming it predates this work and is not a regression from it.
  Everything else passes unchanged.
- Not yet done: live reverify with an actual `pip3 install <package>` (not
  just a synthetic cache-write probe) from a real trading-workflow step,
  the next time one needs a package outside the run image.
