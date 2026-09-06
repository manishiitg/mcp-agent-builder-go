// The API the app uses: the AgentWorks platform, reached through the
// SparkQuill product profiles. The standalone family server is gone
// (docs/design/sparkquill_desktop_on_platform_plan.md, P2a); the desktop
// shell serves this app from the platform server's own origin, and a
// browser dev session points VITE_PLATFORM_API at one.
import { FAMILY_API } from '../apiBase'
import type { FamilyApi } from './familyApi'
import { createPlatformApi } from './platformApi'


export const platformBaseUrl = FAMILY_API

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

export const api: FamilyApi = createPlatformApi({ baseUrl: platformBaseUrl, tokenStore: platformTokenStore })
export type { FamilyApi } from './familyApi'
