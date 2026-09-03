// Runs before anything else in platform mode (main.tsx imports it first).
// The shared AgentWorks service layer reads its base URL from this runtime
// config when its modules are evaluated, so it has to exist before those
// modules load; set here, with no imports of its own, it always does.
type Shell = { sparkquill?: { backend?(): string; apiBaseUrl(): string } }
const env = (import.meta as { env?: Record<string, string | undefined> }).env ?? {}
const backend = (typeof window !== 'undefined' ? (window as Shell).sparkquill?.backend?.() : undefined) ?? env.VITE_SPARKQUILL_BACKEND ?? 'standalone'
if (backend === 'platform' && typeof window !== 'undefined') {
  const base = env.VITE_PLATFORM_API ?? (window as Shell).sparkquill?.apiBaseUrl?.() ?? ''
  const w = window as Window & { __APP_RUNTIME_CONFIG__?: Record<string, string> }
  w.__APP_RUNTIME_CONFIG__ = { ...(w.__APP_RUNTIME_CONFIG__ ?? {}), apiBaseUrl: base, workspaceApiBaseUrl: `${base}/api/wp` }
}
export {}
