const STORAGE_KEY = 'video-studio-panel-width'

export const PANEL_WIDTH_MIN = 340
export const PANEL_WIDTH_MAX = 900
export const PANEL_WIDTH_DEFAULT = 460 // the previous fixed width, so existing users see no change until they resize

export function clampPanelWidth(width: number, min = PANEL_WIDTH_MIN, max = PANEL_WIDTH_MAX): number {
  if (!Number.isFinite(width)) return PANEL_WIDTH_DEFAULT
  return Math.min(max, Math.max(min, Math.round(width)))
}

/** Reads the user's remembered panel width. Falls back to the default on a
 * fresh browser, a corrupted value, or when storage is unavailable (private
 * browsing, disabled cookies, no window -- this codebase's tests run under
 * Node, not jsdom) -- a missing preference should degrade to the previous
 * fixed-width behavior, not throw. */
export function loadStoredPanelWidth(): number {
  if (typeof window === 'undefined') return PANEL_WIDTH_DEFAULT
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return PANEL_WIDTH_DEFAULT
    return clampPanelWidth(Number.parseInt(raw, 10))
  } catch {
    return PANEL_WIDTH_DEFAULT
  }
}

export function saveStoredPanelWidth(width: number): void {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEY, String(clampPanelWidth(width)))
  } catch {
    // Storage unavailable -- the width still applies for this session via
    // React state, it just won't be remembered next time.
  }
}
