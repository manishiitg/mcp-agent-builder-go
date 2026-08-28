# Dominion on Hetzner

This is the deployment contract for the isolated Dominion product at
`trader.tectonicmarkets.com`. It deliberately does **not** reuse the legacy
shared-host deployment described in `README.md`.

## Deployment target

| Item | Value |
|---|---|
| Public hostname | `trader.tectonicmarkets.com` |
| Server | Hetzner host `116.202.210.102` |
| SSH port | `2299` |
| Runtime account | `dominion` |
| Application root | `/srv/dominion` |
| Product allowlist | `AGENT_PRODUCTS=dominion` |

The host already runs unrelated applications. Dominion must never change their
containers, bound ports, systemd units, Caddy sites, or firewall rules.

## Account and filesystem isolation

Root is only used for one-time machine administration: creating the
`dominion` account, enabling linger, installing the Caddy site, and validating
the SSH configuration. All application processes run as `dominion`:

```text
/srv/dominion/
├── current -> releases/<release-id>
├── releases/                 immutable deployed releases
├── data/docs/                Dominion workspace and trading data
├── home/                     HOME for Claude Code and tmux
├── logs/                     application logs
├── state/                    durable runtime state
├── tools/                    user-owned Node/Claude Code installation
└── .env                      mode 0600; never commit or print
```

The deployment key is authorized for the `dominion` account. Future release
uploads use `ssh -p 2299 dominion@116.202.210.102`, not root.

## Network layout

All application listeners remain loopback-only. The existing Caddy service is
the sole public listener and receives one additional hostname block.

```text
Internet
  │ HTTPS :443
  ▼
Caddy (existing shared service)
  │ reverse_proxy 127.0.0.1:21080
  ▼
Dominion gateway       127.0.0.1:21080
  ├── Agent API         127.0.0.1:21000
  └── Workspace API     127.0.0.1:21001
```

The non-default ports are intentional: this host already has services using
the usual AgentWorks ports (`8000`, `8080`, and `8090`). The gateway supports
`AGENT_API_URL`, `WORKSPACE_API_URL`, and `GATEWAY_ADDR` so the product can
coexist without port conflicts. `GATEWAY_USER_ID=dominion` and
`GATEWAY_USERNAME=dominion` keep its authenticated data separate from Video
Studio.

## DNS and TLS

Cloudflare manages `tectonicmarkets.com`. The required record is:

| Type | Name | Content |
|---|---|---|
| A | `trader` | `116.202.210.102` |

The record may be Cloudflare-proxied. Caddy should serve
`trader.tectonicmarkets.com` directly and obtain an origin certificate. In
Cloudflare, use **SSL/TLS: Full (strict)** after the Caddy certificate is
healthy. If certificate issuance is blocked, temporarily set the record to
DNS-only while validating Caddy rather than weakening the application to HTTP.

The cookie-auth gateway deliberately marks its session cookie `Secure`; do not
publish this product on a bare HTTP IP address.

## Secrets

Never put secret values in this repository, systemd unit files, shell history,
or deployment logs. The runtime `.env` is owned by `dominion` and has mode
`0600`.

The recommended secret source is AWS Secrets Manager,
`dominion/global-secrets`, containing only the keys this product needs:

```text
AUTH_SECRET
ACCESS_PASSWORD
CLAUDE_CODE_OAUTH_TOKEN
```

`CLAUDE_CODE_OAUTH_TOKEN` can be copied from the existing RTS shared setup
secret during a release without displaying it. `AUTH_SECRET` and
`ACCESS_PASSWORD` are unique to this deployment. AWS is a secret source only;
Dominion runs entirely on Hetzner and has no AWS runtime dependency.

## Product data

The Dominion UI reads its fixed workflow data from:

```text
Workflow/tectonicusadaytrading/
```

Copy that directory from the approved local workspace source into
`/srv/dominion/data/docs/Workflow/tectonicusadaytrading/`, preserving its
SQLite database and variables manifest. Do not deploy an empty workspace: the
dashboard and its restricted chat tools rely on that database.

## Release requirements

Build Linux artifacts locally; do not use a Docker build on the deployment
host. A release contains:

- `dominion-agent` (AgentWorks, built for Linux amd64)
- `dominion-workspace` (workspace API, built for Linux amd64)
- `mcpbridge`
- `dominion-gateway`
- `video-studio-landlock-runner` (yes, that literal filename regardless of
  product — see below), built from `workspace/cmd/landlock-runner`
- the built frontend with a runtime configuration exposing only `dominion`
- `mcp_servers_dominion.json` with an empty `mcpServers` object

### The Landlock launcher is required, not optional (PLAT-118)

`dominion-workspace` resolves its shell sandbox launcher by a hardcoded name,
`video-studio-landlock-runner` (`workspace/security/landlock_policy.go`),
first next to its own binary (`filepath.Dir(executable)`), then on `PATH`
(`exec.LookPath`) — checked regardless of which product is actually running.
Without it, PLAT-118's fail-closed design means `execute_shell_command`
refuses every call with `SANDBOX_UNAVAILABLE` rather than running
unsandboxed — confirmed live: a fresh Dominion deploy's `/health` reported
`shell_sandbox.available=false`, and the agent's real tool call (Dominion's
only path to its own custom tools is `execute_shell_command` + curl, per
this profile's own product.yaml) failed with "Landlock launcher not found."

Build it alongside the other four binaries and place it in the same `bin/`
directory as `dominion-workspace`:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build \
  -o releases/<release-id>/bin/video-studio-landlock-runner \
  ./workspace/cmd/landlock-runner
```

Restart `dominion-workspace` after adding it, then confirm via
`curl http://127.0.0.1:21001/health` that `shell_sandbox.available=true` and
`shell_sandbox.backend=landlock` before considering a release complete.

The `dominion` account owns Node, Claude Code, and its tool cache under
`/srv/dominion/tools` and `/srv/dominion/home`. No application process runs as
root. Node/npm are already present system-wide on this host; Claude Code CLI
itself is not, and `dominion-agent` shells out to a bare `claude` on `PATH`
(`exec.LookPath("claude")` — no configurable override), so a release without
this step deploys a working agent that fails on its first real turn with
`claude cli not found in PATH`. Install it into the dominion-owned tool
prefix, as `dominion`, idempotently on every release:

```bash
npm install -g --prefix /srv/dominion/tools @anthropic-ai/claude-code
```

Then ensure `PATH` includes `/srv/dominion/tools/bin` ahead of the system
default in `/srv/dominion/.env` (`EnvironmentFile` for every service unit),
and restart `dominion-agent` after installing or upgrading it. Verify with
`/srv/dominion/tools/bin/claude --version`, and confirm it actually
authenticates with the deployed `CLAUDE_CODE_OAUTH_TOKEN` — a present-but-
unauthenticated CLI fails differently, not with the PATH error above:

```bash
CLAUDE_CODE_OAUTH_TOKEN=... HOME=/srv/dominion/home PATH=/srv/dominion/tools/bin:$PATH \
  claude -p --output-format json <<< "say OK"
# expect: {"is_error":false, ..., "result":"OK", ...}
```

## Verification

Before changing Caddy, validate the three local services as `dominion`:

```bash
curl -fsS http://127.0.0.1:21000/api/health
curl -fsS http://127.0.0.1:21001/health
curl -fsSI http://127.0.0.1:21080/login
```

After adding the Caddy hostname and reloading only after `caddy validate`,
verify:

```bash
curl -fsSI https://trader.tectonicmarkets.com/login
```

The site block must also include `encode zstd gzip`. Caddy does not compress
responses by default -- a block with no `encode` directive silently ships
the full, uncompressed frontend bundle (multiple MB) on every page load.
This was missed on the original 2026-08-24 deploy and confirmed live
2026-08-28 as the root cause of a real "the server is slow" report: the main
JS bundle was transferring at its full ~3.7 MB instead of the ~1 MB gzip
would produce. Fixed by adding the directive to only this site's block in
the shared host's `/etc/caddy/Caddyfile` (root-only file; back it up before
editing, `caddy validate` before `systemctl reload caddy`, and never touch
the other unrelated sites' blocks in the same file). Verify against the real
bundle, not just the small `/login` page (which compresses trivially either
way and can look fine even when the real bundle isn't compressed):

```bash
curl -sS -H "Accept-Encoding: gzip" -D - -o /dev/null \
  https://trader.tectonicmarkets.com/assets/index-<hash>.js
# expect: content-encoding: gzip
```

By default the public site must show only Dominion and must not expose Video
Studio, Finance, or unrelated server data. `AGENT_PRODUCTS=dominion` alone
already guarantees this: it's process-wide and never registers Video
Studio/Finance profiles on this host at all, regardless of frontend config.

AgentWorks (the generic, profile-less workflow builder) is the one
deliberate exception: `frontend/current/runtime-config.js`'s
`enabledProductSurfaces` lists `"agentworks"` alongside `"dominion"` so it
can render for a user who is explicitly granted it, but every logged-in user
who is NOT granted it (the default, e.g. `john`) never sees or can reach it
— see `config/user-product-access.json` below. Confirm this per-user
boundary after any change here, not just that the page loads.

## Current bootstrap state

Live on the target host, first deployed 2026-08-24:

- `dominion` system account, private directory tree, deployment-key SSH
  access over port `2299`, systemd user lingering enabled
- `.env` written with `AUTH_SECRET`, `ACCESS_PASSWORD`, `CLAUDE_CODE_OAUTH_TOKEN`,
  and the gateway/agent product env vars from this doc
- Claude Code CLI installed under `/srv/dominion/tools`, authenticated via
  `CLAUDE_CODE_OAUTH_TOKEN`
- `dominion-agent`, `dominion-workspace`, `dominion-gateway` installed as
  `systemd --user` units (`~/.config/systemd/user/`, not `/etc/systemd/system/`
  — no root needed for the app services themselves), enabled and running
- `Workflow/tectonicusadaytrading/` transferred in full to
  `/srv/dominion/data/docs/Workflow/tectonicusadaytrading/` (its
  `workflow.json` `id` is `wf_36021393` — the folder name and the manifest
  ID are different strings; always confirm the real ID via
  `GET /api/workflows/manifests` rather than assuming they match)
- Caddy hostname block added for `trader.tectonicmarkets.com` (root step),
  cert obtained via `tls-alpn-01`, public HTTPS verified
- Per-user product/workflow access control added 2026-08-25:
  `frontend/current/runtime-config.js`'s `enabledProductSurfaces` widened to
  `["dominion", "agentworks"]`, and `config/user-product-access.json`
  written granting `manish` `products: ["dominion", "agentworks"]` scoped to
  `workflow_ids: ["wf_36021393"]`, and restricting `john` to
  `products: ["dominion"]`. A user with no entry in that file is
  unrestricted by design (see `agent_go/cmd/server/user_product_access.go`)
  — every user this deployment creates going forward needs an explicit
  entry to stay scoped the way this doc describes.
- `/srv/dominion/home/Downloads` created 2026-08-25 — its absence broke the
  shell sandbox's Folder Guard policy setup for every session
  (`SANDBOX_UNAVAILABLE: ... stat /srv/dominion/home/Downloads: no such file
  or directory`), silently blocking `execute_shell_command` (Dominion's only
  path to its own custom tools) since the original 2026-08-24 deploy. Any
  future dedicated-VM product needs this directory created alongside
  `HOME=/srv/dominion/home` itself, not as an afterthought.
- Scheduling ownership moved from local dev to this server, 2026-08-28: the
  local instance's copy of `tectonicusadaytrading`'s 3 schedules were
  disabled (`enabled: false` in its `workflow.json`) before a one-time
  `rsync` of the workflow's working files (excluding `.git`) from local into
  `/srv/dominion/data/docs/Workflow/tectonicusadaytrading/`, overwriting the
  stale 2026-08-24 copy. The 3 schedules were then re-enabled on the
  server's copy only. This server is now the sole source of scheduled runs
  for this workflow — re-enabling them locally would cause duplicate paper
  trades and duplicate market-data API usage against the same workflow.
- `encode zstd gzip` added to this site's Caddy block, 2026-08-28 — see the
  Verification section above. Missing since the original deploy; the main
  JS bundle now transfers at ~1 MB instead of ~3.7 MB.
- Gateway shared-password layer disabled, 2026-08-28: `GATEWAY_DISABLE_PASSWORD_GATE=true`
  added to `/srv/dominion/.env`, with `deploy/aws-ec2/server/auth-gateway.go`
  changed to make that an explicit per-deployment opt-out (default
  unchanged everywhere else, including Video Studio). Decision: with real
  per-user login (`manish`/`john`) already gating the app, the extra shared
  password was pure friction, not additional real protection for this
  deployment. Accepted trade-off: a handful of routes the agent API
  intentionally leaves public for pre-login bootstrap (`/api/health`,
  `/api/capabilities`, `/api/shared/*`, `/api/auth/*`) are now reachable
  from the open internet without any password — they were already reachable
  to anyone who cleared the shared password before, so this is a narrowing
  of what the shared password protected, not new exposure of anything the
  inner app didn't already intend to expose at that boundary. The gateway
  still runs and still routes/serves the frontend/proxies `/api`, `/api/wp`,
  `/ws` — only the password-session check is skipped.
- Stale local absolute paths in synced workflow state, found and bulk-fixed
  2026-08-28: the original local→server sync of `Workflow/tectonicusadaytrading/`
  (see the scheduling-ownership entry above) carried over 221 `builder/`
  conversation/session files whose `runtime.agent_session_handle.provider.working_dir`
  still pointed at the local dev machine's absolute path
  (`/Users/mipl/ai-work/mcp-agent-builder-go/workspace-docs/...`). Resuming
  any of those native Claude Code sessions failed with `SANDBOX_UNAVAILABLE:
  ... mkdir /Users: permission denied` (the launcher trying to `mkdir -p` a
  path that only exists on the original dev machine). Bulk-corrected with a
  literal string replace across `builder/` to the real server path
  (`/srv/dominion/data/docs/Workflow/tectonicusadaytrading`). **Any future
  full workflow-directory sync from a local/dev machine to a server must
  repeat this check** — grep the synced tree for the source machine's own
  absolute path before considering the sync complete; this class of bug is
  silent until a native coding-agent session tries to resume.
- `kernel.apparmor_restrict_unprivileged_userns` disabled host-wide,
  2026-08-28 (`/etc/sysctl.d/60-dominion-userns.conf`, applied at
  `sysctl -w` time and persisted across reboots). Ubuntu 24.04's default of
  `1` blocks unprivileged mount-namespace creation for any process without
  an explicit AppArmor profile, even though the underlying
  `kernel.unprivileged_userns_clone` sysctl is enabled. This host's Landlock
  fallback path (`workspace/security/isolator_linux.go`,
  `executeIsolatedMountNamespace`) depends on mount namespaces being
  available for any Folder Guard policy Landlock's purely-additive rule
  model can't express on its own — concretely, "write-access to a workflow
  folder except its `planning/` subfolder" (a real, intentional policy the
  generic AgentWorks path requests, which Dominion's own narrower profile
  never happened to trigger). Without the fallback, that combination hard-
  failed with `SANDBOX_UNAVAILABLE: Landlock cannot represent this Folder
  Guard policy and mount namespaces are unavailable: blocked-write path
  overlaps writable path`. **Trade-off, decided explicitly rather than
  defaulted into**: this is a host-wide toggle affecting every process on
  this shared VM (not just Dominion's), chosen over a narrower
  Dominion-only AppArmor profile grant for speed. Revisit if this host ever
  needs the stricter Ubuntu 24.04 posture back for an unrelated tenant.

Known follow-up, not yet done:

- Cloudflare SSL/TLS mode is not yet switched to Full (strict) — the DNS
  record is currently unproxied (DNS-only), which is why certificate
  issuance worked without any Cloudflare-side change. Flipping to proxied +
  Full (strict) is optional, not blocking, and is the account owner's call
  since it changes what protects this hostname.
