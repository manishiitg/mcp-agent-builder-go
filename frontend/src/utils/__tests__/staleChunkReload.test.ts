import { afterAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearStaleChunkReloadFlag, isStaleChunkError, reloadOnceForStaleChunk } from '../staleChunkReload'

// Node test environment: stand in for the two browser globals the helper uses.
const reload = vi.fn()
const store = new Map<string, string>()
const fakeWindow = {
  location: { reload },
  sessionStorage: {
    getItem: (k: string) => store.get(k) ?? null,
    setItem: (k: string, v: string) => { store.set(k, v) },
    removeItem: (k: string) => { store.delete(k) },
  },
}
const hadWindow = 'window' in globalThis
;(globalThis as unknown as { window: unknown }).window = fakeWindow

describe('staleChunkReload', () => {
  beforeEach(() => {
    reload.mockReset()
    store.clear()
  })
  afterAll(() => {
    if (!hadWindow) delete (globalThis as unknown as { window?: unknown }).window
  })

  it('recognises the browser messages for a chunk from a previous build', () => {
    expect(isStaleChunkError(new Error('Failed to fetch dynamically imported module: https://x/assets/Panel-Dg4M5crE.js'))).toBe(true)
    expect(isStaleChunkError(new TypeError('Importing a module script failed.'))).toBe(true)
    expect(isStaleChunkError('error loading dynamically imported module')).toBe(true)
    expect(isStaleChunkError(new Error('Cannot read properties of undefined'))).toBe(false)
    expect(isStaleChunkError(null)).toBe(false)
  })

  it('reloads once per session and never loops on a genuinely broken build', () => {
    const error = new Error('Failed to fetch dynamically imported module: /assets/A-1.js')
    expect(reloadOnceForStaleChunk(error)).toBe(true)
    expect(reload).toHaveBeenCalledTimes(1)
    expect(reloadOnceForStaleChunk(error)).toBe(false)
    expect(reload).toHaveBeenCalledTimes(1)
    clearStaleChunkReloadFlag()
    expect(reloadOnceForStaleChunk(error)).toBe(true)
    expect(reload).toHaveBeenCalledTimes(2)
  })

  it('ignores unrelated errors', () => {
    expect(reloadOnceForStaleChunk(new Error('boom'))).toBe(false)
    expect(reload).not.toHaveBeenCalled()
  })
})
