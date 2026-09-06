// Runs before anything else (main.tsx imports it first). The shared
// AgentWorks service layer reads its base URL from this runtime config when
// its modules are evaluated, so it has to exist before those modules load;
// set here, with no imports of its own, it always does.
//
// In the desktop app the page is served by the platform server itself, so
// the base is the page's own origin (the shell's preload confirms it);
// VITE_PLATFORM_API overrides it for a browser dev session against a
// separately running server.
type Shell = { sparkquill?: { apiBaseUrl(): string } }
const env = (import.meta as { env?: Record<string, string | undefined> }).env ?? {}
if (typeof window !== 'undefined') {
  const base = env.VITE_PLATFORM_API ?? (window as Shell).sparkquill?.apiBaseUrl?.() ?? window.location.origin
  const w = window as Window & { __APP_RUNTIME_CONFIG__?: Record<string, string> }
  w.__APP_RUNTIME_CONFIG__ = { ...(w.__APP_RUNTIME_CONFIG__ ?? {}), apiBaseUrl: base, workspaceApiBaseUrl: `${base}/api/wp` }
}
export {}
