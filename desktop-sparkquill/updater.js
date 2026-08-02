// In-app updates for SparkQuill.
//
// Mirrors desktop/main.js (AgentWorks) in shape — check GitHub Releases,
// download the dmg, then hand off to the install script detached so it can
// replace the running app — with one difference that matters:
//
//   SparkQuill releases are tagged `sparkquill-v*`, AgentWorks' are plain `v*`,
//   and they share one repository. So /releases/latest is WRONG here: it
//   returns whichever app shipped most recently, which is almost always
//   AgentWorks (it releases far more often). The release list has to be
//   filtered by tag prefix. install-sparkquill.sh had exactly this bug and
//   carries the same fix.
//
// The installer contract is already in place: install-sparkquill.sh honours
// SPARKQUILL_VERSION and SPARKQUILL_DMG_PATH, so the post-quit gap is just
// mount, copy, relaunch — no second download.

const { app, dialog, shell, Notification } = require('electron')
const { spawn } = require('child_process')
const path = require('path')
const fs = require('fs')
const os = require('os')
const https = require('https')

const REPO = 'manishiitg/coding-agent-loop'
const TAG_PREFIX = 'sparkquill-v'
const INSTALL_SH = `https://raw.githubusercontent.com/${REPO}/main/install-sparkquill.sh`
// Re-check occasionally so a long-running install still notices a release
// without the parent having to think about it.
const CHECK_INTERVAL_MS = 6 * 60 * 60 * 1000

let state = { downloading: false, version: null, dmgPath: null }

function getJson(url, redirectsLeft = 5) {
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { 'User-Agent': 'SparkQuill', Accept: 'application/vnd.github+json' } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          res.resume()
          if (redirectsLeft <= 0) return reject(new Error('too many redirects'))
          return resolve(getJson(res.headers.location, redirectsLeft - 1))
        }
        if (res.statusCode !== 200) {
          res.resume()
          return reject(new Error(`HTTP ${res.statusCode}`))
        }
        let body = ''
        res.setEncoding('utf8')
        res.on('data', (c) => { body += c })
        res.on('end', () => {
          try { resolve(JSON.parse(body)) } catch (err) { reject(err) }
        })
      })
      .on('error', reject)
  })
}

function download(url, dest, redirectsLeft = 5) {
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { 'User-Agent': 'SparkQuill' } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          res.resume()
          if (redirectsLeft <= 0) return reject(new Error('too many redirects'))
          return resolve(download(res.headers.location, dest, redirectsLeft - 1))
        }
        if (res.statusCode !== 200) {
          res.resume()
          return reject(new Error(`HTTP ${res.statusCode}`))
        }
        const file = fs.createWriteStream(dest)
        res.pipe(file)
        file.on('finish', () => file.close(() => resolve(dest)))
        file.on('error', reject)
      })
      .on('error', reject)
  })
}

/** Numeric semver-ish compare; true when a is newer than b. */
function isNewer(a, b) {
  const pa = String(a).split('.').map((n) => parseInt(n, 10) || 0)
  const pb = String(b).split('.').map((n) => parseInt(n, 10) || 0)
  for (let i = 0; i < Math.max(pa.length, pb.length); i++) {
    const d = (pa[i] || 0) - (pb[i] || 0)
    if (d !== 0) return d > 0
  }
  return false
}

async function latestSparkQuillRelease() {
  const releases = await getJson(`https://api.github.com/repos/${REPO}/releases?per_page=30`)
  if (!Array.isArray(releases)) return null
  // Newest-first from the API; take the first SparkQuill-tagged, non-draft one.
  return releases.find((r) => !r.draft && typeof r.tag_name === 'string' && r.tag_name.startsWith(TAG_PREFIX)) || null
}

async function checkForUpdates(manual = false) {
  if (!app.isPackaged) {
    if (manual) {
      await dialog.showMessageBox({
        type: 'info',
        title: 'Updates',
        message: 'Update checks are disabled in development mode.',
        buttons: ['OK'],
      })
    }
    return
  }

  let release
  try {
    release = await latestSparkQuillRelease()
  } catch (err) {
    if (manual) dialog.showErrorBox('Could not check for updates', String(err?.message || err))
    return
  }
  if (!release) {
    if (manual) {
      await dialog.showMessageBox({ type: 'info', title: 'Updates', message: 'No SparkQuill releases found.', buttons: ['OK'] })
    }
    return
  }

  const version = release.tag_name.slice(TAG_PREFIX.length)
  const current = app.getVersion()
  if (!isNewer(version, current)) {
    if (manual) {
      await dialog.showMessageBox({
        type: 'info',
        title: "You're up to date",
        message: `SparkQuill ${current} is the latest version.`,
        buttons: ['OK'],
      })
    }
    return
  }

  // Already working on this exact version — surface the ready prompt rather
  // than starting a second download.
  if (state.version === version && (state.downloading || state.dmgPath)) {
    if (state.dmgPath) return promptInstall(version)
    if (manual) {
      await dialog.showMessageBox({ type: 'info', title: 'Downloading update', message: `SparkQuill ${version} is downloading…`, buttons: ['OK'] })
    }
    return
  }

  const name = `SparkQuill-${version}-arm64.dmg`
  const asset = (release.assets || []).find((a) => a.name === name)
  if (!asset) {
    if (manual) dialog.showErrorBox('Update unavailable', `${name} is not attached to the ${release.tag_name} release.`)
    return
  }

  if (manual) {
    const choice = await dialog.showMessageBox({
      type: 'info',
      title: 'Update available',
      message: `SparkQuill ${version} is available. You have ${current}.`,
      detail: 'It will download in the background. You will be asked before anything is installed.',
      buttons: ['Download', 'Later'],
      defaultId: 0,
      cancelId: 1,
    })
    if (choice.response !== 0) return
  }

  state = { downloading: true, version, dmgPath: null }
  const dest = path.join(os.tmpdir(), name)
  try {
    await download(asset.browser_download_url, dest)
  } catch (err) {
    state = { downloading: false, version: null, dmgPath: null }
    if (manual) dialog.showErrorBox('Download failed', String(err?.message || err))
    return
  }
  state = { downloading: false, version, dmgPath: dest }
  await promptInstall(version)
}

async function promptInstall(version) {
  if (!state.dmgPath || !fs.existsSync(state.dmgPath)) return
  const choice = await dialog.showMessageBox({
    type: 'info',
    title: 'Update ready',
    message: `SparkQuill ${version} is downloaded and ready to install.`,
    detail: 'Installing takes a few seconds and reopens the app. Any conversation in progress will be interrupted.',
    buttons: ['Restart & Install', 'Later'],
    defaultId: 0,
    cancelId: 1,
  })
  if (choice.response === 0) installDownloaded()
}

function installDownloaded() {
  if (!state.dmgPath || !fs.existsSync(state.dmgPath)) {
    dialog.showErrorBox('Update error', 'The downloaded update was not found. Please check for updates again.')
    return
  }
  // Detached + nohup so the installer outlives the app it is replacing; the
  // script quits any running copy itself before swapping the bundle.
  const inner =
    `export SPARKQUILL_VERSION='${TAG_PREFIX}${state.version}'; ` +
    `export SPARKQUILL_DMG_PATH='${state.dmgPath}'; ` +
    `curl -fsSL ${INSTALL_SH} | bash > /tmp/sparkquill-update.log 2>&1`
  const wrapped = `nohup bash -c ${JSON.stringify(inner)} >/dev/null 2>&1 &`
  try {
    const child = spawn('/bin/bash', ['-lc', wrapped], { detached: true, stdio: 'ignore' })
    child.unref()
  } catch (err) {
    dialog.showErrorBox('Update failed to start', String(err?.message || err))
    return
  }
  try {
    if (Notification.isSupported()) {
      new Notification({
        title: 'Installing SparkQuill…',
        body: `Installing ${state.version}. The app will reopen in a few seconds.`,
      }).show()
    }
  } catch { /* a missing notification must not block the install */ }
  setTimeout(() => app.quit(), 600)
}

function openReleaseNotes() {
  shell.openExternal(`https://github.com/${REPO}/releases`)
}

function startUpdateChecks() {
  if (!app.isPackaged) return
  // Not at the instant of launch: the first seconds are already busy starting
  // the server and waiting for health.
  setTimeout(() => checkForUpdates(false), 30_000)
  setInterval(() => checkForUpdates(false), CHECK_INTERVAL_MS)
}

module.exports = { checkForUpdates, startUpdateChecks, openReleaseNotes }
