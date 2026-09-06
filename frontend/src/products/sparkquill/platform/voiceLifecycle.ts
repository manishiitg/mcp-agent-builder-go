// Frees the ~1 GB speech model when the desktop window is hidden and warms it
// again on show. The Electron shell only signals visibility (preload.js's
// onWindowVisibility); this side holds the login token that the platform's
// /api/voice/* routes require, so it makes the calls.
//
// Warm is deliberately conditional on the model already being installed: the
// platform's /api/voice/warm starts a first-time download when it is not,
// and reopening a window must never trigger 690 MB of traffic by itself.
// The composer's own mic button remains the explicit "set up voice" step.
import { FAMILY_API } from '../apiBase'

// The parent profile id (internal/sparkquillproduct/product.yaml).
const PROFILE_ID = 'sparkquill'

type Shell = { sparkquill?: { onWindowVisibility?(callback: (visible: boolean) => void): void } }
type VoiceStatus = { installed?: boolean; ready?: boolean; loading?: boolean; downloading?: boolean }

function token(): string {
  try { return localStorage.getItem('auth_token') ?? '' } catch { return '' }
}

async function post(path: string): Promise<void> {
  const t = token()
  if (!t) return
  await fetch(`${FAMILY_API}${path}?profile_id=${PROFILE_ID}`, { method: 'POST', headers: { Authorization: `Bearer ${t}` } }).catch(() => undefined)
}

async function warmIfInstalled(): Promise<void> {
  const t = token()
  if (!t) return
  try {
    const res = await fetch(`${FAMILY_API}/api/voice/status`, { headers: { Authorization: `Bearer ${t}` } })
    if (!res.ok) return
    const status = (await res.json()) as VoiceStatus
    if (status.installed && !status.ready && !status.loading && !status.downloading) await post('/api/voice/warm')
  } catch { /* best effort: a hidden→shown transition must never surface an error */ }
}

let installed = false

/**
 * Subscribes to the shell's window-visibility signal once per page. The
 * preload's listener cannot be removed, so this is idempotent rather than
 * returning a cleanup; the surface calls it on mount.
 */
export function installVoiceLifecycle(): void {
  if (installed || typeof window === 'undefined') return
  const shell = (window as Shell).sparkquill
  if (!shell?.onWindowVisibility) return
  installed = true
  shell.onWindowVisibility((visible) => {
    if (visible) void warmIfInstalled()
    else void post('/api/voice/unload')
  })
}
