// A single-page app keeps the JavaScript it loaded; a new build only reaches
// an open tab on reload. This watches the served index.html for a different
// script bundle and offers that reload, so a tab never silently runs a stale
// build after a deploy or a local rebuild.
import { useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'

const POLL_MS = 60_000

function loadedBundle(): string | null {
  const script = Array.from(document.scripts).find((s) => /\/assets\/index-[^/]+\.js$/.test(s.src))
  return script ? new URL(script.src).pathname : null
}

async function servedBundle(): Promise<string | null> {
  const res = await fetch(`${window.location.pathname}?build=${Date.now()}`, { cache: 'no-store' })
  if (!res.ok) return null
  const html = await res.text()
  const m = html.match(/src="([^"]*\/assets\/index-[^"/]+\.js)"/)
  return m ? new URL(m[1], window.location.origin).pathname : null
}

export function BuildUpdateNotice() {
  const [stale, setStale] = useState(false)
  useEffect(() => {
    const current = loadedBundle()
    if (!current) return
    let cancelled = false
    const check = () => {
      if (document.hidden) return
      servedBundle().then((served) => { if (!cancelled && served && served !== current) setStale(true) }).catch(() => {})
    }
    const timer = window.setInterval(check, POLL_MS)
    document.addEventListener('visibilitychange', check)
    window.addEventListener('focus', check)
    return () => { cancelled = true; window.clearInterval(timer); document.removeEventListener('visibilitychange', check); window.removeEventListener('focus', check) }
  }, [])
  if (!stale) return null
  return (
    <button type="button" className="fl-update-notice" onClick={() => window.location.reload()}>
      <RefreshCw size={14} /> SparkQuill has updated — tap to reload
    </button>
  )
}
