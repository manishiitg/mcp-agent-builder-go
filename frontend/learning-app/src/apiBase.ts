// The family-server base URL, shared by every module that talks to it.
//
// In the packaged desktop app the Electron preload exposes the real port the
// server actually bound to (it may differ from the dev default if 8010 was
// taken — see desktop-sparkquill/main.js). The bridge only exists inside
// Electron; in a browser this falls through to the dev default unchanged.
//
// Lives in its own module (rather than inside LearningApp.tsx) so extracted
// components can import it without pulling in the whole app file.
export const FAMILY_API =
  (typeof window !== 'undefined' ? (window as { sparkquill?: { apiBaseUrl(): string } }).sparkquill?.apiBaseUrl() : undefined)
  ?? (import.meta as { env?: { VITE_FAMILY_API?: string } }).env?.VITE_FAMILY_API
  ?? 'http://127.0.0.1:8010'
