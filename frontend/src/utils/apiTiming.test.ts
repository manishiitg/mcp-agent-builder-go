import { afterEach, describe, expect, it } from 'vitest'
import { apiTimingPathFor, clearApiTimings, getApiTimings, recordApiTiming, summarizeApiTimings } from './apiTiming'

afterEach(() => {
  clearApiTimings()
})

describe('apiTimingPathFor', () => {
  it('strips origin and query string, keeping only the pathname', () => {
    expect(apiTimingPathFor('/api/workflows/manifests?x=1', 'https://trader.tectonicmarkets.com')).toBe(
      '/api/workflows/manifests'
    )
    expect(apiTimingPathFor('https://example.com/api/health', undefined)).toBe('/api/health')
  })

  it('falls back gracefully for an unparseable url', () => {
    expect(apiTimingPathFor(undefined, undefined)).toBe('(unknown)')
  })
})

describe('recordApiTiming / getApiTimings', () => {
  it('records entries and caps the ring buffer at 500', () => {
    for (let i = 0; i < 510; i++) {
      recordApiTiming({ method: 'GET', path: '/api/health', status: 200, durationMs: 10, timestamp: i })
    }
    expect(getApiTimings().length).toBe(500)
    // Oldest entries are dropped first.
    expect(getApiTimings()[0].timestamp).toBe(10)
  })
})

describe('summarizeApiTimings', () => {
  it('aggregates per endpoint and sorts by total time descending', () => {
    recordApiTiming({ method: 'GET', path: '/api/health', status: 200, durationMs: 50, timestamp: 1 })
    recordApiTiming({ method: 'GET', path: '/api/health', status: 200, durationMs: 150, timestamp: 2 })
    recordApiTiming({ method: 'GET', path: '/api/workflows/manifests', status: 200, durationMs: 5000, timestamp: 3 })
    recordApiTiming({ method: 'POST', path: '/api/query', status: 'error', durationMs: 300, timestamp: 4 })

    const { aggregates, recent } = summarizeApiTimings()

    expect(aggregates[0].endpoint).toBe('GET /api/workflows/manifests')
    expect(aggregates[0].calls).toBe(1)
    expect(aggregates[0].avgMs).toBe(5000)

    const health = aggregates.find(a => a.endpoint === 'GET /api/health')
    expect(health?.calls).toBe(2)
    expect(health?.avgMs).toBe(100)
    expect(health?.maxMs).toBe(150)

    const errored = aggregates.find(a => a.endpoint === 'POST /api/query')
    expect(errored?.errors).toBe(1)

    // Recent entries are sorted slowest-first.
    expect(recent[0].durationMs).toBe(5000)
  })
})
