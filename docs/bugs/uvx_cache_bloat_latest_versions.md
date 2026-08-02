# Bug: uvx/npx Cache Bloat from @latest MCP Server Packages

## Status: Open — reconfirmed 2026-08-02, still growing

Re-verified rather than assumed. Nothing in the proposed fix was built:

```text
~/.cache/uv                                        15 GB   (was "10GB+ observed")
uvx servers still pinned to @latest                7 of 15 in mcp_servers_clean_user.json
mcpagent/mcpclient/version_pin_cache.go            never created
VersionPin / pinnedVersion / UV_CACHE guards       no occurrences in either repo
```

A note for anyone re-reading this: `version_pin_cache.go` appears below under
"Proposed Fix" as a **new** file. It was a proposal, not a description of code
that later regressed — searching for it and finding nothing is the expected
result, not evidence the fix was reverted.

The cache has grown ~50% since the original report and the configuration that
causes it is unchanged.

Two things were wrong with this report as originally written, both corrected
below: it proposed no cleanup at all, so implementing its fix verbatim would
still leave the cache growing; and it assumed the framework could manage
`~/.cache/uv`, which is user-global and shared with any other `uvx` on the
machine. See "The primary fix: scope the cache before managing it".

## Problem

The `~/.cache/uv/` directory grows unbounded (15GB observed 2026-08-02) over time due to repeated `uvx` ephemeral venv creation for MCP servers configured with `@latest`.

## Root Cause

### 1. `@latest` prevents venv reuse

7 uvx servers and 1 npx server in `mcp_servers_clean_user.json` use `@latest`:
- `awslabs.aws-api-mcp-server@latest`
- `awslabs.aws-pricing-mcp-server@latest`
- `awslabs.cost-explorer-mcp-server@latest`
- `awslabs.cloudwatch-mcp-server@latest`
- `awslabs.billing-cost-management-mcp-server@latest`
- `awslabs.aws-documentation-mcp-server@latest`
- `awslabs.eks-mcp-server@latest`
- `kubernetes-mcp-server@latest` (npx)

`uvx package@latest` creates a new ephemeral virtual environment each time it runs because it cannot guarantee the cached venv has the latest version. Pinned versions (`uvx package==0.4.2`) allow uvx to reuse cached venvs.

### 2. Multiple spawn points create throwaway processes

Processes are spawned at:
- **Server startup**: Background discovery for uncached servers (`tools.go:runBackgroundDiscovery`)
- **Every 24h**: Periodic refresh (`tools.go:startPeriodicRefresh`) re-discovers all servers
- **Per query** (session registry path): Creates long-lived connections, but first spawn per server still adds to cache

Each throwaway discovery spawn creates a new venv entry in `~/.cache/uv/` that is never cleaned up.

### 3. No uvx cache cleanup

`uv` does not automatically prune old environments. They accumulate indefinitely.

## Impact

- Disk usage grows ~1.4GB per discovery cycle (7 servers x ~200MB venv with AWS SDK deps)
- Over weeks of server restarts + 24h refreshes: 10GB+
- Performance degradation from disk pressure

## Affected Files

- `agent_go/configs/mcp_servers_clean_user.json` — Server configs with `@latest`
- `mcpagent/mcpclient/client.go:connectOnce()` — Process spawn point (line ~192)
- `mcpagent/mcpclient/stdio_manager.go` — StdioManager creates subprocess
- `mcpagent/mcpcache/integration.go` — Background discovery creates throwaway connections
- `agent_go/cmd/server/tools.go` — Periodic refresh and background discovery triggers

## Where the space actually goes (measured 2026-08-02)

```text
~/.cache/uv/archive-v0/     15 GB    528 entries      ← all of it
~/.cache/uv/simple-v15/    118 MB
~/.cache/uv/environments-v2/  2 entries
```

`environments-v2/` holding two entries is misleading and cost this
investigation a wrong turn: the ephemeral tool environments are real, they are
just stored content-addressed under `archive-v0/`. Each is a complete virtualenv
(`bin/`, `lib/*/site-packages/`, `_virtualenv.pth`) at roughly 274–284 MB for
the AWS servers, and several are near-identical siblings — the churn this report
describes.

## The primary fix: scope the cache before managing it

**`~/.cache/uv` is user-global.** `uvx` is a general-purpose tool; anything else
on the machine may have warmed that cache and may depend on it. The framework
must not prune it — doing so is an unbounded side effect outside our boundary,
and it would silently evict another tool's work.

The fix is to stop writing there at all:

1. **Set `UV_CACHE_DIR` on MCP server spawns** to a framework-owned path. `uv`
   supports it directly (`--cache-dir <CACHE_DIR> [env: UV_CACHE_DIR=]`), and the
   spawn environment is already ours — `NewStdioManager(command, args, env,
   workingDir, logger)` passes `env` straight to `cmd.Env`.
2. **Prune that directory, not the user's.** Once scoped, nothing outside the
   framework references it, so bounding it is a local decision with no external
   blast radius.

Both changes belong at the same interception point this report already
identifies for version pinning — `client.go:connectOnce()`, where all four spawn
paths converge.

**The cost, stated plainly:** we lose cache sharing with the user's own `uvx`
usage, so the first spawn of each server version re-downloads its tree — a real
one-time hit at ~284 MB for the AWS servers. The trade is a bounded cache we may
manage against a warm one we may not.

### Pruning the existing 15 GB is an operator action

```bash
uv cache prune      # removes unreachable objects; keeps anything still referenced
```

Document it; do not automate it. It operates on the user's cache, which is
exactly the boundary above.

Run once on the reporting machine, 2026-08-02:

```text
before   15 GB    528 archive entries
after   1.9 GB    388 archive entries
removed 610,558 files (11.6 GiB)
```

**77% of the cache was unreachable.** Only 140 entries were removed, but they
carried 11.6 GB — the ~280 MB AWS virtualenvs. The 388 that remain total 1.7 GB,
so the survivors are small and genuinely referenced.

This is the size of the problem and also the shape of it: the waste is a small
number of very large, entirely dead environments. It will return at the same
rate — roughly one 280 MB environment per AWS server release — until the
`UV_CACHE_DIR` scoping above exists and the framework prunes its own directory.
Pruning by hand is a reset, not a fix.

### Why pinning alone is not sufficient

The version-pin design below is sound and worth building, but it is an
optimisation rather than the fix. Pinning reduces the *rate* of new environments
— from "one per differing resolution" to "one per actual release" — and old
environments still accumulate without bound. With seven AWS servers releasing
independently, that is slower growth, not bounded growth.

Implementing only the section below would leave this bug open at a lower slope.

## Optimisation: Version Pin Resolution

Resolve `@latest` to the actual version via PyPI/npm registry API, cache it for 24h, and use pinned versions for spawning.

### Flow
```
First run / every 24h:
  Resolve: GET https://pypi.org/pypi/awslabs.aws-api-mcp-server/json → version "0.4.2"
  Cache:   {"awslabs.aws-api-mcp-server": {"version": "0.4.2", "resolved_at": "..."}}
  Spawn:   uvx awslabs.aws-api-mcp-server==0.4.2  (uvx reuses cached venv)

Next 24h:
  Cache hit → uvx awslabs.aws-api-mcp-server==0.4.2  (no download, no new venv)

After 24h:
  Re-resolve @latest → maybe "0.4.3" → cache new version
```

### Implementation

1. **New: `mcpagent/mcpclient/version_pin.go`** — Core logic: detect `@latest` in args, resolve via HTTP to PyPI (`https://pypi.org/pypi/<pkg>/json`) or npm (`https://registry.npmjs.org/<pkg>/latest`), return pinned args
2. **New: `mcpagent/mcpclient/version_pin_cache.go`** — JSON file cache (`cache/version_pins.json`): singleton, 24h TTL, thread-safe, persists to disk
3. **Modify: `mcpagent/mcpclient/client.go`** — 2-line change in `connectOnce()` before `NewStdioManager`:
   ```go
   resolvedArgs := ResolveVersionPins(ctx, c.config.Command, c.config.Args, GetVersionPinCache(c.logger), c.logger)
   stdioManager := NewStdioManager(c.config.Command, resolvedArgs, env, c.logger)
   ```

### Why this interception point

All spawn paths converge at `client.go:connectOnce()` → `NewStdioManager(command, args, env)`:
- Session registry connections (main query path via `connection_session.go`)
- Background discovery (`integration.go:performOriginalConnectionLogic`)
- Periodic refresh (`tools.go:startPeriodicRefresh`)
- Fresh connection fallback (`integration.go:GetFreshConnection`)

One change covers all paths. Original config args are never mutated (copy is made).

### Pinned arg format
- uvx: `package==0.4.2` (PyPI convention)
- npx: `package@0.4.2` (npm convention)
- Scoped npm: `@scope/package@1.2.3`

### Graceful degradation
- If PyPI/npm is unreachable: 10s timeout, warning logged, `@latest` kept
- If package not found: warning logged, `@latest` kept
- If cache file corrupt: fresh empty cache created
- SSE/HTTP protocol servers: unaffected (resolution only runs for stdio)

### Configuration
| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `MCP_VERSION_PIN_TTL_HOURS` | `24` | How long to cache resolved versions |
| `MCP_CACHE_DIR` | `./cache` | Shared with existing tool cache |

## Workaround (Manual)

Run periodically to clean old uvx cache:
```bash
uv cache prune
```

Or pin versions manually in `mcp_servers_clean_user.json`:
```json
"args": ["awslabs.aws-api-mcp-server==0.4.2"]
```
