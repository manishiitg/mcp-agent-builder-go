// The platform server's base URL for the few modules that call it directly
// (the voice settings and dictation modules; the FamilyApi goes through
// api/index.ts). The page is served by that server and runtime-config.js,
// which the agent server generates, names it; the page's own origin is the
// fallback. Lives in its own module so extracted components can import it
// without pulling in the whole app file.
type RuntimeConfigWindow = Window & { __APP_RUNTIME_CONFIG__?: { apiBaseUrl?: unknown } }

function configuredApiBase(): string {
  if (typeof window === 'undefined') return ''
  const configured = (window as RuntimeConfigWindow).__APP_RUNTIME_CONFIG__?.apiBaseUrl
  return typeof configured === 'string' && configured.trim() ? configured.replace(/\/+$/, '') : window.location.origin
}

export const FAMILY_API = configuredApiBase()
