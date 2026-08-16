import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { PANEL_WIDTH_DEFAULT, PANEL_WIDTH_MAX, PANEL_WIDTH_MIN, clampPanelWidth, loadStoredPanelWidth, saveStoredPanelWidth } from './panelWidth'

describe('clampPanelWidth', () => {
  it('keeps a value already inside the range unchanged', () => {
    expect(clampPanelWidth(600)).toBe(600)
  })

  it('clamps below the minimum', () => {
    expect(clampPanelWidth(10)).toBe(PANEL_WIDTH_MIN)
  })

  it('clamps above the maximum', () => {
    expect(clampPanelWidth(PANEL_WIDTH_MAX + 500)).toBe(PANEL_WIDTH_MAX)
  })

  it('falls back to the default for a non-finite value', () => {
    expect(clampPanelWidth(NaN)).toBe(PANEL_WIDTH_DEFAULT)
  })

  it('rounds a fractional pointer position to a whole pixel', () => {
    expect(clampPanelWidth(500.6)).toBe(501)
  })
})

describe('stored panel width', () => {
  let values: Map<string, string>

  beforeEach(() => {
    values = new Map()
    vi.stubGlobal('window', {
      localStorage: {
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => values.set(key, value),
      },
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('returns the default when nothing has been saved yet', () => {
    // A user who never resizes the panel should see the same width the
    // fixed-width layout always used, not an arbitrary new default.
    expect(loadStoredPanelWidth()).toBe(PANEL_WIDTH_DEFAULT)
  })

  it('remembers a width across the equivalent of a page reload', () => {
    saveStoredPanelWidth(650)
    expect(loadStoredPanelWidth()).toBe(650)
  })

  it('clamps a corrupted stored value rather than applying it as-is', () => {
    values.set('video-studio-panel-width', 'not-a-number')
    expect(loadStoredPanelWidth()).toBe(PANEL_WIDTH_DEFAULT)
  })

  it('clamps an out-of-range stored value on load', () => {
    values.set('video-studio-panel-width', '5000')
    expect(loadStoredPanelWidth()).toBe(PANEL_WIDTH_MAX)
  })

  it('does nothing when there is no window (e.g. server-side)', () => {
    vi.unstubAllGlobals()
    expect(() => saveStoredPanelWidth(600)).not.toThrow()
    expect(loadStoredPanelWidth()).toBe(PANEL_WIDTH_DEFAULT)
  })
})
