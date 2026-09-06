# SparkQuill desktop

The macOS app wrapper around SparkQuill. It starts the two platform servers
(`workspace-server` and `agent-server`, the same binaries the AgentWorks desktop
ships), waits for them to be healthy, and opens a window onto the learning app
that the agent server serves as its static site. SparkQuill is a product on
the platform (`agent_go/internal/sparkquillproduct/product.yaml`); there is no
separate server for it any more.

Sibling of `../desktop/` (AgentWorks), not a fork. The shell mechanics the two
apps share (login-shell environment import, bounded log writer, server spawn
with dynamic ports, health polling, external-link handling, signal shutdown)
live in `../desktop/lib` as the `agentworks-desktop-lib` package, linked here
as a `file:` dependency. What stays here is SparkQuill's own: the window, the
tray and menu, the persisted auth secret, the voice lifecycle IPC and the
updater. These two apps ship independently, on their own tags.

## Run it locally

```sh
./dev-setup.sh     # builds the frontend + both servers into resources/, installs deps
npm start
```

`dev-setup.sh` stages the learning app into `resources/static`, the default
MCP config into `resources/configs`, builds `agent-server` with the speech
engine (cgo, see `scripts/build-darwin-voice-binary.sh`) and `workspace-server`
without it.

To work on the Electron chrome against a hot-reloading frontend, skip the
bundled site and point at Vite:

```sh
cd ../frontend/learning-app && npm run dev     # in one terminal (:5174)
DEV_URL=http://127.0.0.1:5174 npm start         # here (still spawns the servers)
```

## Build a .dmg

```sh
./dev-setup.sh
npm run build      # unpacked, into dist/ — fastest way to check it launches
npm run dist       # real .dmg + .zip (needs GH_TOKEN to publish)
```

CI does the same steps — see `.github/workflows/sparkquill-desktop.yml`.

## Release

Push a `sparkquill-v*` tag (namespaced so it can't collide with AgentWorks'
plain `v*` tags in this same repo):

```sh
git tag sparkquill-v0.1.0 && git push origin sparkquill-v0.1.0
```

The workflow builds the arm64 dmg, syncs `package.json`'s version to the tag,
and publishes it to that release. Users then install with:

```sh
curl -fsSL https://raw.githubusercontent.com/manishiitg/coding-agent-loop/main/install-sparkquill.sh | bash
```

## Things worth knowing

- **Not notarized.** `mac.identity` is `null` (ad-hoc signing), which is why
  `install-sparkquill.sh` has to strip the quarantine flag. Shipping outside a
  trusted circle means adding a Developer ID and a notarize step.
- **The dmg filename is load-bearing.** `productName` drives it
  (`SparkQuill-<version>-arm64.dmg`) and the installer hardcodes that shape —
  changing one means changing both.
- **Ports.** Prefers 45778 (agent) and 45779 (workspace) and falls forward if
  either is taken, so it can run beside the AgentWorks desktop. The frontend
  reads its API base from `window.sparkquill.apiBaseUrl()` (see `preload.js`),
  so a shifted port still works.
- **Data lives in `userData/workspace-docs`** (`~/Library/Application
  Support/sparkquill-desktop/`), the family's conversations under
  `_users/<user>/Chats/SparkQuill/`. A pre-platform install's
  `~/.sunlit-learning` is copied in on first launch (never moved; see
  `agent_go/internal/sparkquillproduct/migrate.go`). Logs go to
  `userData/logs/` (Help → Open Logs).
- **Auth secret.** The agent server refuses to start without one, so the shell
  generates it once and keeps it in `userData/config.json`; provider keys are
  encrypted with it.
- **PATH.** A GUI-launched app gets a minimal PATH, but the agent server shells
  out to the family's coding CLI (codex/claude/cursor/pi) and tools like `gws`.
  The shared lib imports the real login-shell environment at startup to fix that.
