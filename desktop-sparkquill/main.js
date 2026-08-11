// SparkQuill desktop shell.
//
// Deliberately small: it starts the family-server Go binary, waits for it to be
// healthy, and points a window at it. The entire UI is the same web app served
// by that server (FAMILY_WEB_DIR), so there is no second copy of the frontend
// here and nothing to keep in sync.
//
// This is a sibling of desktop/ (AgentWorks), not a fork of it — that app
// carries a tray, MCP config rewriting, an auth secret, and a docs-dir picker
// that SparkQuill has no use for. Shared mechanisms (login-shell env import,
// spawn + health-wait, quarantine-free install via install.sh) are reproduced
// here rather than abstracted, since two ~200-line files are easier to reason
// about than one parameterized 2000-line one.

const { app, BrowserWindow, shell, dialog, nativeTheme, Menu, Tray, nativeImage } = require('electron')
const { spawn, spawnSync } = require('child_process')
const detect = require('detect-port')
const path = require('path')
const fs = require('fs')
const http = require('http')
const { checkForUpdates, startUpdateChecks, openReleaseNotes } = require('./updater')

const PREFERRED_PORT = 8010
const HEALTH_TIMEOUT_MS = 90000
const HEALTH_POLL_MS = 500
const LOG_MAX_BYTES = 25 * 1024 * 1024

let serverProcess = null
let mainWindow = null
let serverPort = PREFERRED_PORT
let tray = null
// Closing the window only HIDES it (see the 'close' handler in createWindow),
// so the server keeps running and the menu-bar icon stays. This flag is what
// distinguishes "the parent closed the window" from a real quit, which is the
// only time the window is allowed to actually close.
let isQuitting = false

// --- login-shell environment -------------------------------------------------
// A GUI-launched .app inherits a minimal PATH (/usr/bin:/bin:/usr/sbin:/sbin),
// which is fatal here: family-server shells out to the family's chosen coding
// CLI (codex, claude, cursor-agent, pi) and to tools like `gws`, none of which
// live there. Import the real interactive-login environment once at startup.
// Markers delimit the env block so shell rc banners can't corrupt the parse.
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
// The server's stdout/stderr is the only diagnostic a user can send us, so it
// goes to a real file — capped, keeping the tail, since a long-running session
// with a chatty CLI can otherwise fill a disk.
function createLogWriter() {
  const dir = path.join(app.getPath('userData'), 'logs')
  fs.mkdirSync(dir, { recursive: true })
  const file = path.join(dir, 'family-server.log')
  return (chunk) => {
    try {
      fs.appendFileSync(file, chunk)
      if (fs.statSync(file).size > LOG_MAX_BYTES) {
        const keep = fs.readFileSync(file).slice(-Math.floor(LOG_MAX_BYTES * 0.75))
        fs.writeFileSync(file, keep)
      }
    } catch { /* logging must never take the app down */ }
  }
}

function resourcesDir() {
  return app.isPackaged ? process.resourcesPath : path.join(__dirname, 'resources')
}

// --- server ------------------------------------------------------------------
async function startServer() {
  const bin = path.join(resourcesDir(), 'family-server')
  if (!fs.existsSync(bin)) {
    throw new Error(`family-server not found at ${bin}.\n\nRun desktop-sparkquill/dev-setup.sh to build it.`)
  }
  // Prefer 8010 so the origin stays stable across restarts — the web app keeps
  // per-origin state in localStorage (which side of the handoff you were on,
  // theme), and a shifting port would silently reset it every launch.
  serverPort = await detect(PREFERRED_PORT)

  const webDir = path.join(resourcesDir(), 'web')
  const log = createLogWriter()
  log(`\n=== SparkQuill ${app.getVersion()} starting on :${serverPort} ===\n`)

  serverProcess = spawn(bin, ['--port', String(serverPort)], {
    cwd: resourcesDir(),
    env: { ...process.env, FAMILY_PORT: String(serverPort), FAMILY_WEB_DIR: webDir },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  serverProcess.stdout.on('data', log)
  serverProcess.stderr.on('data', log)
  serverProcess.on('exit', (code, signal) => {
    log(`\n=== family-server exited code=${code} signal=${signal} ===\n`)
    serverProcess = null
  })
}

// --- voice model lifecycle ---------------------------------------------------
// The speech model costs 15-20s to load and ~600MB resident. Tie it to whether
// the app is actually on screen: a parent who closes the window to the menu bar
// is done for now, and one who reopens it is about to use it. A pure idle timer
// did worse at both ends — it held the memory while the app sat in the
// background, and still made the mic slow after any gap longer than the timeout.
function voiceLifecycle(action) {
  const req = http.request(
    { host: '127.0.0.1', port: serverPort, path: `/api/voice/native/${action}`, method: 'POST' },
    (res) => res.resume(),
  )
  // Best-effort: a window event must never surface an error, and the server
  // may simply not be up yet.
  req.on('error', () => {})
  req.end()
}

function health(url) {
  return new Promise((resolve) => {
    const req = http.get(url, (res) => {
      res.resume()
      resolve(res.statusCode === 200)
    })
    req.on('error', () => resolve(false))
    req.setTimeout(5000, () => { req.destroy(); resolve(false) })
  })
}

async function waitForServer() {
  const url = `http://127.0.0.1:${serverPort}/api/health`
  const deadline = Date.now() + HEALTH_TIMEOUT_MS
  while (Date.now() < deadline) {
    if (serverProcess === null) throw new Error('family-server stopped before it became ready — see the log in Help → Open Logs.')
    if (await health(url)) return
    await new Promise((r) => setTimeout(r, HEALTH_POLL_MS))
  }
  throw new Error(`family-server did not become ready within ${HEALTH_TIMEOUT_MS / 1000}s.`)
}

function stopServer() {
  if (!serverProcess) return
  try { serverProcess.kill('SIGTERM') } catch { /* already gone */ }
  serverProcess = null
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
  mainWindow.loadURL(`http://127.0.0.1:${serverPort}/?v=${app.getVersion()}`)

  // Anything that isn't the local app opens in the real browser rather than
  // replacing the app window.
  const isLocal = (url) => url.startsWith(`http://127.0.0.1:${serverPort}`) || url.startsWith(`http://localhost:${serverPort}`)
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
  // Closing hides rather than quits: the family-server keeps running, so
  // Pulse check-ins and WhatsApp stay live, and reopening from the menu bar
  // is instant (no server start, no health wait). Only a real quit — the tray
  // item, Cmd-Q, or a system shutdown — is allowed through.
  mainWindow.on('close', (e) => {
    if (isQuitting) return
    e.preventDefault()
    mainWindow?.hide()
  })
  mainWindow.on('closed', () => { mainWindow = null })
  // 'hide' fires for close-to-menu-bar (see the close handler above) and for
  // Cmd-H; 'show' for the tray item, the Dock, and activate.
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
  if (serverProcess || process.env.DEV_URL) createWindow()
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
          click: () => shell.openPath(path.join(app.getPath('home'), '.sunlit-learning')),
        },
      ],
    },
  ]
  Menu.setApplicationMenu(Menu.buildFromTemplate(template))
}

function failFast(err) {
  dialog.showErrorBox('SparkQuill could not start', String(err?.message || err))
  stopServer()
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
      serverPort = Number(new URL(process.env.DEV_URL).port) || PREFERRED_PORT
      createWindow()
      mainWindow.loadURL(process.env.DEV_URL)
      return
    }
    await startServer()
    await waitForServer()
    createWindow()
    startUpdateChecks()
  } catch (err) {
    failFast(err)
  }
})

app.on('activate', showWindow)
// Deliberately NOT quitting when the last window goes: closing only hides the
// window (see createWindow), and the running family-server is what drives
// Pulse check-ins and holds the WhatsApp connection — so it stays up, reachable
// from the menu bar, until the parent actually quits.
app.on('window-all-closed', () => {})
app.on('before-quit', () => { isQuitting = true; stopServer() })
app.on('will-quit', stopServer)
