// SparkQuill desktop shell.
//
// M1 of docs/design/sparkquill_desktop_on_platform_plan.md: this shell now
// spawns the same two platform binaries the AgentWorks desktop spawns
// (workspace-server + agent-server) instead of the standalone family-server
// binary, and serves the existing learning-app frontend as the agent
// server's static/ directory. The turn engine, tools, and workspace API are
// now 100% shared with AgentWorks — nothing SparkQuill-specific lives in Go
// beyond internal/sparkquillproduct/product.yaml.
//
// AGENT_PRODUCTS is deliberately left UNSET below. Setting it to "sparkquill"
// alone would make isSingleProductServerDeployment() true and 400 every
// Claude Code turn with no stored OAuth token (see A3 in the design doc) —
// the desktop case is supposed to fall back to the user's locally logged-in
// CLI, exactly like the AgentWorks desktop does.
//
// This is a sibling of desktop/ (AgentWorks), not a fork of it. P1 of the
// design doc extracts the shared parts of both into desktop/lib/ + per-product
// descriptors; until then this file duplicates the parts it needs, the same
// as it always has.

const { app, BrowserWindow, shell, dialog, nativeTheme, Menu, Tray, nativeImage } = require('electron')
const { spawn, spawnSync } = require('child_process')
const detect = require('detect-port')
const path = require('path')
const fs = require('fs')
const crypto = require('crypto')
const http = require('http')
const { checkForUpdates, startUpdateChecks, openReleaseNotes } = require('./updater')
const { buildAgentServerEnv } = require('./lib/agentEnv')

const HEALTH_TIMEOUT_MS = 90000
const HEALTH_POLL_MS = 500
const HEALTH_INITIAL_DELAY_MS = 1000
const LOG_MAX_BYTES = 25 * 1024 * 1024

// Own preferred ports, one digit off AgentWorks' (45678/45679) — collisions
// are soft either way (detect-port falls forward to the next free port,
// which just resets that origin's localStorage/JWT), but a distinct pair
// means the common case of running both apps at once needs no fallback at all.
const PREFERRED_AGENT_PORT = 45778
const PREFERRED_WORKSPACE_PORT = 45779

let agentProcess = null
let workspaceProcess = null
let mainWindow = null
let agentPort = PREFERRED_AGENT_PORT
let workspacePort = PREFERRED_WORKSPACE_PORT
const workspaceApiToken = crypto.randomBytes(32).toString('hex')
let tray = null
// Closing the window only HIDES it (see the 'close' handler in createWindow),
// so the servers keep running and the menu-bar icon stays. This flag is what
// distinguishes "the parent closed the window" from a real quit, which is the
// only time the window is allowed to actually close.
let isQuitting = false

// --- login-shell environment -------------------------------------------------
// A GUI-launched .app inherits a minimal PATH (/usr/bin:/bin:/usr/sbin:/sbin),
// which is fatal here: the agent server shells out to the family's chosen
// coding CLI (codex, claude, cursor-agent, pi), none of which live there.
// Import the real interactive-login environment once at startup. Markers
// delimit the env block so shell rc banners can't corrupt the parse.
function importLoginShellEnv() {
  if (process.platform === 'win32') return
  try {
    const shellPath = process.env.SHELL || '/bin/zsh'
    const res = spawnSync(shellPath, ['-ilc', 'printf __SQ_BEGIN__; /usr/bin/env -0; printf __SQ_END__'], {
      encoding: 'utf8',
      timeout: 10000,
    })
    const out = res.stdout || ''
    const begin = out.indexOf('__SQ_BEGIN__')
    const end = out.indexOf('__SQ_END__')
    if (begin === -1 || end === -1) return
    out.slice(begin + '__SQ_BEGIN__'.length, end)
      .split('\0')
      .forEach((pair) => {
        const eq = pair.indexOf('=')
        if (eq <= 0) return
        const key = pair.slice(0, eq)
        const val = pair.slice(eq + 1)
        // PATH always wins — that's the whole reason for doing this. Everything
        // else only fills gaps, so anything Electron set intentionally stands.
        if (key === 'PATH' || process.env[key] === undefined) process.env[key] = val
      })
  } catch { /* a missing login env is survivable; a crashed launch isn't */ }
}

// --- logging -----------------------------------------------------------------
// Each server's stdout/stderr is the only diagnostic a user can send us, so it
// goes to a real file — capped, keeping the tail, since a long-running session
// with a chatty CLI can otherwise fill a disk.
function createLogWriter(name) {
  const dir = path.join(app.getPath('userData'), 'logs')
  fs.mkdirSync(dir, { recursive: true })
  const file = path.join(dir, `${name}.log`)
  return {
    file,
    write(chunk) {
      try {
        fs.appendFileSync(file, chunk)
        if (fs.statSync(file).size > LOG_MAX_BYTES) {
          const keep = fs.readFileSync(file).slice(-Math.floor(LOG_MAX_BYTES * 0.75))
          fs.writeFileSync(file, keep)
        }
      } catch { /* logging must never take the app down */ }
    },
  }
}

function resourcesDir() {
  return app.isPackaged ? process.resourcesPath : path.join(__dirname, 'resources')
}

// --- settings + auth secret ---------------------------------------------------
// The agent server fatals on start with an empty/default AUTH_SECRET
// (ValidateConfiguredAuthSecret, server.go:1524-1526) — the standalone
// family-server never needed one, so the shell has to mint and persist its
// own the first time it runs the platform binaries.
function settingsPath() {
  return path.join(app.getPath('userData'), 'config.json')
}

function loadSettings() {
  try {
    if (fs.existsSync(settingsPath())) return JSON.parse(fs.readFileSync(settingsPath(), 'utf8'))
  } catch (e) {
    console.error('[main] Failed to load settings:', e)
  }
  return {}
}

function saveSettings(settings) {
  fs.mkdirSync(path.dirname(settingsPath()), { recursive: true })
  fs.writeFileSync(settingsPath(), JSON.stringify(settings, null, 2))
}

function resolveAuthSecret() {
  const settings = loadSettings()
  if (settings.authSecret) return settings.authSecret
  const secret = crypto.randomBytes(32).toString('hex')
  saveSettings({ ...settings, authSecret: secret })
  return secret
}

function workspaceDocsDir() {
  return path.join(app.getPath('userData'), 'workspace-docs')
}

// --- servers -------------------------------------------------------------
function spawnWorkspace() {
  return new Promise((resolve, reject) => {
    const bin = path.join(resourcesDir(), 'workspace-server')
    if (!fs.existsSync(bin)) {
      return reject(new Error(`workspace-server not found at ${bin}.\n\nRun desktop-sparkquill/dev-setup.sh to build it.`))
    }
    const docsDir = workspaceDocsDir()
    const dataDir = path.join(app.getPath('userData'), 'data')
    fs.mkdirSync(dataDir, { recursive: true })
    const log = createLogWriter('workspace')

    const env = {
      ...process.env,
      DOCS_DIR: docsDir,
      DATA_DIR: dataDir,
      // workspace-server executes /api/execute shell commands. Mark it native
      // so the safe shell env preserves the imported login-shell PATH/HOME
      // instead of the Docker-style minimal PATH.
      NATIVE_WORKSPACE: 'true',
      WORKSPACE_API_TOKEN: workspaceApiToken,
    }

    detect(PREFERRED_WORKSPACE_PORT).then((port) => {
      const child = spawn(bin, ['server', '--port', String(port), '--docs-dir', docsDir], {
        cwd: resourcesDir(),
        env,
        stdio: ['ignore', 'pipe', 'pipe'],
      })
      workspaceProcess = child
      let portFound = false

      child.on('error', (err) => {
        log.write(`[workspace] spawn error: ${err}\n`)
        if (!portFound) reject(err)
      })
      child.on('exit', (code, signal) => {
        log.write(`\n=== workspace-server exited code=${code} signal=${signal} ===\n`)
        workspaceProcess = null
      })
      child.stdout.on('data', (d) => {
        const output = d.toString()
        log.write(output)
        if (!portFound) {
          const match = output.match(/DynamicPort: (\d+)/)
          if (match) {
            workspacePort = parseInt(match[1], 10)
            portFound = true
            resolve()
          }
        }
      })
      child.stderr.on('data', (d) => log.write(d.toString()))
    }).catch(reject)
  })
}

function spawnAgent() {
  return new Promise((resolve, reject) => {
    const bin = path.join(resourcesDir(), 'agent-server')
    if (!fs.existsSync(bin)) {
      return reject(new Error(`agent-server not found at ${bin}.\n\nRun desktop-sparkquill/dev-setup.sh to build it.`))
    }
    // Resources dir is what makes server.go's `./static/` mount resolve to
    // the frontend staged there by dev-setup.sh/CI (server.go:2533).
    const cwd = resourcesDir()
    const docsDir = workspaceDocsDir()
    const log = createLogWriter('agent')

    const configDir = path.join(app.getPath('userData'), 'configs')
    fs.mkdirSync(configDir, { recursive: true })
    const mcpConfigPath = path.join(configDir, 'mcp_servers.json')
    if (!fs.existsSync(mcpConfigPath)) {
      const defaultConfigPath = path.join(cwd, 'configs', 'mcp_servers_clean.json')
      try {
        if (fs.existsSync(defaultConfigPath)) fs.copyFileSync(defaultConfigPath, mcpConfigPath)
      } catch (err) {
        log.write(`[agent] failed to copy default mcp config: ${err}\n`)
      }
    }
    try {
      fs.mkdirSync(path.join(docsDir, 'Downloads'), { recursive: true })
    } catch { /* non-fatal */ }

    const authSecret = resolveAuthSecret()
    const args = ['server', '--port', '0', '--log-file', log.file, '--log-level', 'debug', '--mcp-config', mcpConfigPath]

    const env = buildAgentServerEnv(process.env, {
      authSecret,
      workspacePort,
      docsDir,
      logFile: log.file,
      workspaceApiToken,
    })
    // AGENT_PRODUCTS and MULTI_USER_MODE are deliberately absent from env
    // above — see the file header and lib/agentEnv.test.js. Do not add either
    // without re-reading A3 in the design doc.

    detect(PREFERRED_AGENT_PORT).then((port) => {
      args[2] = String(port)
      const child = spawn(bin, args, { cwd, env, stdio: ['ignore', 'pipe', 'pipe'] })
      agentProcess = child
      let portFound = false

      child.on('error', (err) => {
        log.write(`[agent] spawn error: ${err}\n`)
        if (!portFound) reject(err)
      })
      child.on('exit', (code, signal) => {
        log.write(`\n=== agent-server exited code=${code} signal=${signal} ===\n`)
        agentProcess = null
      })
      child.stdout.on('data', (d) => {
        const output = d.toString()
        log.write(output)
        if (!portFound) {
          const match = output.match(/DynamicPort: (\d+)/)
          if (match) {
            agentPort = parseInt(match[1], 10)
            portFound = true
            resolve()
          }
        }
      })
      child.stderr.on('data', (d) => log.write(d.toString()))
    }).catch(reject)
  })
}

function stopServers() {
  if (workspaceProcess) { try { workspaceProcess.kill('SIGTERM') } catch { /* already gone */ } workspaceProcess = null }
  if (agentProcess) { try { agentProcess.kill('SIGTERM') } catch { /* already gone */ } agentProcess = null }
}

function fetchHealth(url) {
  return new Promise((resolve) => {
    const req = http.get(url, (res) => { res.resume(); resolve(res.statusCode === 200) })
    req.on('error', () => resolve(false))
    req.setTimeout(5000, () => { req.destroy(); resolve(false) })
  })
}

async function waitForHealth() {
  const agentUrl = `http://127.0.0.1:${agentPort}/api/health`
  const workspaceUrl = `http://127.0.0.1:${workspacePort}/health`
  await new Promise((r) => setTimeout(r, HEALTH_INITIAL_DELAY_MS))
  const deadline = Date.now() + HEALTH_TIMEOUT_MS
  for (;;) {
    if (agentProcess === null || workspaceProcess === null) {
      throw new Error('a server exited before it became ready — see Help → Open Logs.')
    }
    const [agentOk, workspaceOk] = await Promise.all([fetchHealth(agentUrl), fetchHealth(workspaceUrl)])
    if (agentOk && workspaceOk) return
    if (Date.now() > deadline) {
      const parts = []
      if (!agentOk) parts.push(`agent (port ${agentPort})`)
      if (!workspaceOk) parts.push(`workspace (port ${workspacePort})`)
      throw new Error(`Servers did not become ready in time. Not ready: ${parts.join(' and ')}.`)
    }
    await new Promise((r) => setTimeout(r, HEALTH_POLL_MS))
  }
}

// --- voice model lifecycle ---------------------------------------------------
// The speech model costs 15-20s to load and ~1 GB resident. Tie it to whether
// the app is actually on screen: a parent who closes the window to the menu
// bar is done for now, and one who reopens it is about to use it.
// /api/voice/* sits behind the platform's auth, and the main process holds no
// login token — so the shell only tells the renderer that visibility changed
// (see preload.js), and the app's platform/voiceLifecycle.ts, which does hold
// the token, calls unload/warm.
function voiceLifecycle(action) {
  mainWindow?.webContents.send('window-visibility', { visible: action === 'warm' })
}

// --- window ------------------------------------------------------------------
function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1280,
    height: 860,
    minWidth: 900,
    minHeight: 600,
    title: 'SparkQuill',
    icon: path.join(resourcesDir(), 'icons', 'icon.png'),
    backgroundColor: '#fbf7ef', // the app's own cream — avoids a white flash on load
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      nodeIntegration: false,
      contextIsolation: true,
    },
  })

  // Cache-bust per version: the origin never changes (same fixed port), so an
  // upgraded app would otherwise keep serving the previous build's JS.
  mainWindow.loadURL(`http://127.0.0.1:${agentPort}/?v=${app.getVersion()}`)

  const isLocal = (url) => url.startsWith(`http://127.0.0.1:${agentPort}`) || url.startsWith(`http://localhost:${agentPort}`)
  mainWindow.webContents.setWindowOpenHandler(({ url }) => {
    if (isLocal(url)) return { action: 'allow' }
    shell.openExternal(url)
    return { action: 'deny' }
  })
  mainWindow.webContents.on('will-navigate', (e, url) => {
    if (!isLocal(url)) { e.preventDefault(); shell.openExternal(url) }
  })
  // A renderer crash should recover, not strand the user on a blank window.
  mainWindow.webContents.on('render-process-gone', () => mainWindow?.webContents.reload())
  // Closing hides rather than quits: the servers keep running, so Pulse
  // check-ins stay live, and reopening from the menu bar is instant (no
  // server start, no health wait). Only a real quit — the tray item, Cmd-Q,
  // or a system shutdown — is allowed through.
  mainWindow.on('close', (e) => {
    if (isQuitting) return
    e.preventDefault()
    mainWindow?.hide()
  })
  mainWindow.on('closed', () => { mainWindow = null })
  mainWindow.on('hide', () => voiceLifecycle('unload'))
  mainWindow.on('show', () => voiceLifecycle('warm'))
}

// Reopen from the menu bar / Dock. The window is usually still alive and just
// hidden, so this is normally a show(); it only rebuilds when the window was
// genuinely destroyed.
function showWindow() {
  if (mainWindow) {
    mainWindow.show()
    mainWindow.focus()
    return
  }
  if (agentProcess || process.env.DEV_URL) createWindow()
}

function createTray() {
  if (tray) return
  const iconPath = path.join(resourcesDir(), 'icons', 'icon.png')
  if (!fs.existsSync(iconPath)) return
  const icon = nativeImage.createFromPath(iconPath)
  if (icon.isEmpty()) return
  // 18px is the usable height of the macOS menu bar — the source icon is
  // 1024px square, and handing Tray the full-size image gets it scaled badly.
  tray = new Tray(icon.resize({ width: 18, height: 18 }))
  tray.setToolTip('SparkQuill')
  tray.setContextMenu(Menu.buildFromTemplate([
    { label: 'Open SparkQuill', click: showWindow },
    { type: 'separator' },
    { label: 'Quit SparkQuill', click: () => { isQuitting = true; app.quit() } },
  ]))
  tray.on('click', showWindow)
}

function buildMenu() {
  const isMac = process.platform === 'darwin'
  const template = [
    // Spelled out rather than `{ role: 'appMenu' }` for one reason: macOS
    // convention puts "Check for Updates…" in the app menu directly under
    // About (Safari, Chrome, Slack all do this), and that is the first place
    // anyone looks for it — but the appMenu role is opaque, so an item cannot
    // be inserted into it. Everything else here is the standard role set, in
    // the standard order, so this behaves exactly like the built-in otherwise.
    ...(isMac
      ? [{
          label: app.name,
          submenu: [
            { role: 'about' },
            { type: 'separator' },
            { label: 'Check for Updates…', click: () => checkForUpdates(true) },
            { type: 'separator' },
            { role: 'services' },
            { type: 'separator' },
            { role: 'hide' },
            { role: 'hideOthers' },
            { role: 'unhide' },
            { type: 'separator' },
            { role: 'quit' },
          ],
        }]
      : []),
    { role: 'editMenu' },
    {
      label: 'View',
      submenu: [
        { role: 'reload' },
        { role: 'forceReload' },
        { role: 'toggleDevTools' },
        { type: 'separator' },
        { role: 'resetZoom' }, { role: 'zoomIn' }, { role: 'zoomOut' },
        { type: 'separator' },
        { role: 'togglefullscreen' },
      ],
    },
    { role: 'windowMenu' },
    {
      role: 'help',
      submenu: [
        // On macOS this lives in the app menu above, where it belongs; keep it
        // here only on platforms that have no app menu to put it in.
        ...(isMac ? [] : [{ label: 'Check for Updates…', click: () => checkForUpdates(true) }]),
        { label: 'Release Notes', click: openReleaseNotes },
        { type: 'separator' },
        {
          label: 'Open Logs',
          click: () => shell.openPath(path.join(app.getPath('userData'), 'logs')),
        },
        {
          label: 'Open SparkQuill Folder',
          click: () => shell.openPath(workspaceDocsDir()),
        },
      ],
    },
  ]
  Menu.setApplicationMenu(Menu.buildFromTemplate(template))
}

function failFast(err) {
  dialog.showErrorBox('SparkQuill could not start', String(err?.message || err))
  stopServers()
  app.exit(1)
}

importLoginShellEnv()

app.whenReady().then(async () => {
  nativeTheme.themeSource = 'light' // the app is designed light-first
  // The window's own `icon` option (see createWindow) doesn't affect the
  // macOS Dock icon while running unpackaged (`npm start` / `electron .`) —
  // that's a separate API, and without this the Dock just shows the generic
  // Electron logo instead of SparkQuill's own icon. Packaged builds don't
  // need this (electron-builder bakes the icon into the .app bundle itself).
  if (!app.isPackaged) app.dock?.setIcon(path.join(resourcesDir(), 'icons', 'icon.png'))
  buildMenu()
  createTray()
  try {
    // DEV_URL points at the Vite dev server and skips spawning entirely, so
    // desktop chrome can be worked on against a hot-reloading frontend.
    if (process.env.DEV_URL) {
      agentPort = Number(new URL(process.env.DEV_URL).port) || PREFERRED_AGENT_PORT
      createWindow()
      mainWindow.loadURL(process.env.DEV_URL)
      return
    }
    await spawnWorkspace()
    await spawnAgent()
    await waitForHealth()
    createWindow()
    startUpdateChecks()
  } catch (err) {
    failFast(err)
  }
})

app.on('activate', showWindow)
// Deliberately NOT quitting when the last window goes: closing only hides the
// window (see createWindow), and the running servers are what drive Pulse
// check-ins — so they stay up, reachable from the menu bar, until the parent
// actually quits.
app.on('window-all-closed', () => {})
app.on('before-quit', () => { isQuitting = true; stopServers() })
app.on('will-quit', stopServers)
// A SIGTERM/SIGINT to the Electron main process (a `kill`, a supervisor, a
// dev script) does not run the quit handlers above, and would otherwise
// orphan both servers — still bound to their ports, still writing into the
// workspace the next launch expects to own.
for (const signal of ['SIGTERM', 'SIGINT']) {
  process.on(signal, () => { isQuitting = true; stopServers(); app.exit(0) })
}
