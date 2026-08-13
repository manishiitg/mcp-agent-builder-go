// In-app updates for SparkQuill.
//
// Deliberately thin. install-sparkquill.sh already does the whole install:
// it finds the latest sparkquill-v* release, downloads that dmg, quits the
// running copy, swaps the bundle, clears quarantine and relaunches. This file
// used to duplicate most of that (list releases, pick the asset, download it,
// keep a little download state machine, hand the dmg path back to the script)
// — two implementations of the same thing, and only one of them was the one
// actually exercised whenever anyone updated from a terminal.
//
// So this now does only the part the script genuinely cannot: work out whether
// a newer version even exists, and ask before restarting the app.
//
// One thing that does have to live here: SparkQuill releases are tagged
// `sparkquill-v*` and AgentWorks' are plain `v*`, in the SAME repository. So
// /releases/latest is wrong — it returns whichever app shipped most recently,
// almost always AgentWorks. The release list must be filtered by tag prefix.

const { app, dialog, shell, Notification } = require('electron')
const { spawn } = require('child_process')
const https = require('https')

const REPO = 'manishiitg/coding-agent-loop'
const TAG_PREFIX = 'sparkquill-v'
const INSTALL_SH = `https://raw.githubusercontent.com/${REPO}/main/install-sparkquill.sh`
// Re-check occasionally so a long-running install still notices a release
// without the parent having to think about it.
const CHECK_INTERVAL_MS = 6 * 60 * 60 * 1000

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

/** Newest published SparkQuill version ("0.2.5"), or null. */
async function latestVersion() {
  const releases = await getJson(`https://api.github.com/repos/${REPO}/releases?per_page=30`)
  if (!Array.isArray(releases)) return null
  // Newest-first from the API; take the first SparkQuill-tagged, non-draft one.
  const rel = releases.find((r) => !r.draft && typeof r.tag_name === 'string' && r.tag_name.startsWith(TAG_PREFIX))
  return rel ? rel.tag_name.slice(TAG_PREFIX.length) : null
}

// Runs the installer detached (nohup) so it outlives the app it is replacing —
// the script quits any running copy itself before swapping the bundle. No
// version is pinned: the script installs the latest, which is the version the
// parent was just told about a moment ago.
function runInstaller() {
  const inner = `curl -fsSL ${INSTALL_SH} | bash > /tmp/sparkquill-update.log 2>&1`
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
        title: 'Updating SparkQuill…',
        body: 'Downloading and installing. The app will reopen on its own.',
      }).show()
    }
  } catch { /* a missing notification must not block the install */ }
  setTimeout(() => app.quit(), 600)
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

  let version
  try {
    version = await latestVersion()
  } catch (err) {
    if (manual) dialog.showErrorBox('Could not check for updates', String(err?.message || err))
    return
  }

  const current = app.getVersion()
  if (!version || !isNewer(version, current)) {
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

  // The BACKGROUND check must never put a modal dialog on screen: it fires on
  // a timer, and the person in front of the app is often a child mid-question,
  // for whom a surprise dialog over her work is worse than not knowing about an
  // update. So it mentions it once, passively, and leaves it at that — the
  // parent updates from the SparkQuill menu when it suits them.
  if (!manual) {
    try {
      if (Notification.isSupported()) {
        new Notification({
          title: `SparkQuill ${version} is available`,
          body: 'Update from the SparkQuill menu → Check for Updates.',
        }).show()
      }
    } catch { /* nothing to do if notifications are unavailable */ }
    return
  }

  const choice = await dialog.showMessageBox({
    type: 'info',
    title: 'Update available',
    message: `SparkQuill ${version} is available. You have ${current}.`,
    detail:
      'It downloads and installs in one go, which takes a minute or so. The app closes ' +
      'and reopens by itself — anything in progress will be interrupted.',
    buttons: ['Update Now', 'Later'],
    defaultId: 0,
    cancelId: 1,
  })
  if (choice.response === 0) runInstaller()
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
