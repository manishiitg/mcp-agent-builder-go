// The platform server's base URL, shared by every module that talks to it
// directly (the voice settings and dictation modules; the FamilyApi goes
// through api/index.ts).
//
// In the desktop app the page is served by the platform server itself, so
// this is the page's own origin (the Electron preload exposes it, and the
// window's origin is the same value). VITE_PLATFORM_API points a browser dev
// session at a separately running server.
//
// Lives in its own module (rather than inside LearningApp.tsx) so extracted
// components can import it without pulling in the whole app file.
export const FAMILY_API =
  (import.meta as { env?: { VITE_PLATFORM_API?: string } }).env?.VITE_PLATFORM_API
  ?? (typeof window !== 'undefined' ? (window as { sparkquill?: { apiBaseUrl(): string } }).sparkquill?.apiBaseUrl() : undefined)
  ?? (typeof window !== 'undefined' ? window.location.origin : '')
