// The API the app uses. The backend is chosen once, at load: the standalone
// family server (default) or the AgentWorks platform, selected by the
// desktop shell (window.sparkquill.backend) or VITE_SPARKQUILL_BACKEND.
import { FAMILY_API } from '../apiBase'
import type { FamilyApi } from './familyApi'
import { createPlatformApi } from './platformApi'
import { standaloneApi } from './standaloneApi'

type Shell = { sparkquill?: { backend?(): string; apiBaseUrl(): string } }
const env = (import.meta as { env?: Record<string, string | undefined> }).env ?? {}
export const backend =
  (typeof window !== 'undefined' ? (window as Shell).sparkquill?.backend?.() : undefined)
  ?? env.VITE_SPARKQUILL_BACKEND
  ?? 'standalone'

export const platformBaseUrl = env.VITE_PLATFORM_API ?? FAMILY_API

// The shared service layer's base URL is set in platform/runtimeConfig.ts,
// which main.tsx imports before anything else; only the login token is
// mirrored here (into the key the shared services read).
const TOKEN_KEY = 'sparkquill.platform.token'
const platformTokenStore = {
  get: () => { try { return localStorage.getItem(TOKEN_KEY) } catch { return null } },
  set: (t: string | null) => {
    try {
      if (t) { localStorage.setItem(TOKEN_KEY, t); localStorage.setItem('auth_token', t) }
      else { localStorage.removeItem(TOKEN_KEY); localStorage.removeItem('auth_token') }
    } catch { /* storage unavailable */ }
  },
}

export const api: FamilyApi = backend === 'platform'
  ? createPlatformApi({ baseUrl: platformBaseUrl, tokenStore: platformTokenStore })
  : standaloneApi
export type { FamilyApi } from './familyApi'
