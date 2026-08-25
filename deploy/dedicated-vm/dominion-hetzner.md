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

The public site must show only Dominion. It must not expose AgentWorks, Video
Studio, Finance, workflow execution, or unrelated server data.

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
  `/srv/dominion/data/docs/Workflow/tectonicusadaytrading/`
- Caddy hostname block added for `trader.tectonicmarkets.com` (root step),
  cert obtained via `tls-alpn-01`, public HTTPS verified

Known follow-up, not yet done:

- Cloudflare SSL/TLS mode is not yet switched to Full (strict) — the DNS
  record is currently unproxied (DNS-only), which is why certificate
  issuance worked without any Cloudflare-side change. Flipping to proxied +
  Full (strict) is optional, not blocking, and is the account owner's call
  since it changes what protects this hostname.
