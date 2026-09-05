import { readFileSync } from 'node:fs'
import { describe, expect, it } from 'vitest'

describe('UI control production route', () => {
  it('uses the /api namespace, not the SPA fallback', () => {
    // Check the real API wrapper, not a mocked client with its own URL.
    // Production baseURL is the origin; /sessions returns index.html (200).
    const source = readFileSync(new URL('../../services/api.ts', import.meta.url), 'utf8')
    const wrapper = source.slice(source.indexOf('export async function workflowUIControl'), source.indexOf('const DEDUPED_GET_REUSE_MS'))
    expect(wrapper).toContain('api.post(`/api/sessions/${encodeURIComponent(session)}/ui-control`, body)')
    expect(wrapper).not.toContain('api.post(`/sessions/')
  })
})
