// Lightweight, in-memory API response-time tracking -- no backend, no
// third-party analytics, nothing leaves the browser. Mirrors the existing
// window.perfDiag() console-diagnostic pattern (see App.tsx) rather than
// adding a dependency: this is for a developer/operator to run in DevTools
// when a deployment "feels slow", not for aggregate product analytics.

export interface ApiTimingEntry {
  method: string
  path: string
  status: number | 'error'
  durationMs: number
  timestamp: number
}

const MAX_ENTRIES = 500
const entries: ApiTimingEntry[] = []

// Strips the origin and query string so entries group by endpoint shape
// (e.g. "/api/workflows/manifests"), not by every distinct query/host.
export function apiTimingPathFor(url: string | undefined, baseURL: string | undefined): string {
  if (!url) return '(unknown)'
  try {
    // The dummy fallback base only matters for a relative url with no
    // baseURL; new URL() ignores it entirely when url is already absolute.
    return new URL(url, baseURL || 'http://localhost').pathname
  } catch {
    return url.split('?')[0]
  }
}

export function recordApiTiming(entry: ApiTimingEntry) {
  entries.push(entry)
  if (entries.length > MAX_ENTRIES) entries.shift()
}

export function getApiTimings(): ApiTimingEntry[] {
  return entries.slice()
}

export function clearApiTimings() {
  entries.length = 0
}

interface ApiTimingAggregate {
  endpoint: string
  calls: number
  errors: number
  avgMs: number
  p95Ms: number
  maxMs: number
  totalMs: number
}

function percentile(sorted: number[], p: number): number {
  if (sorted.length === 0) return 0
  const index = Math.min(sorted.length - 1, Math.ceil((p / 100) * sorted.length) - 1)
  return sorted[Math.max(0, index)]
}

export function summarizeApiTimings(): { aggregates: ApiTimingAggregate[]; recent: ApiTimingEntry[] } {
  const byEndpoint = new Map<string, ApiTimingEntry[]>()
  for (const entry of entries) {
    const key = `${entry.method} ${entry.path}`
    const bucket = byEndpoint.get(key)
    if (bucket) bucket.push(entry)
    else byEndpoint.set(key, [entry])
  }

  const aggregates: ApiTimingAggregate[] = []
  for (const [endpoint, calls] of byEndpoint.entries()) {
    const durations = calls.map((c) => c.durationMs).sort((a, b) => a - b)
    const totalMs = durations.reduce((sum, d) => sum + d, 0)
    aggregates.push({
      endpoint,
      calls: calls.length,
      errors: calls.filter((c) => c.status === 'error' || (typeof c.status === 'number' && c.status >= 400)).length,
      avgMs: Math.round(totalMs / durations.length),
      p95Ms: Math.round(percentile(durations, 95)),
      maxMs: Math.round(durations[durations.length - 1] ?? 0),
      totalMs: Math.round(totalMs),
    })
  }
  aggregates.sort((a, b) => b.totalMs - a.totalMs)

  const recent = entries.slice(-30).slice().sort((a, b) => b.durationMs - a.durationMs)

  return { aggregates, recent }
}
