// The API the app uses. The backend is chosen once, at load: the standalone
// family server (default) or the AgentWorks platform, selected by the
// desktop shell (window.sparkquill.backend) or VITE_SPARKQUILL_BACKEND.
import { FAMILY_API } from '../apiBase'
import type { FamilyApi } from './familyApi'
import { createPlatformApi } from './platformApi'
import { standaloneApi } from './standaloneApi'

type Shell = { sparkquill?: { backend?(): string; apiBaseUrl(): string } }
const env = (import.meta as { env?: Record<string, string | undefined> }).env ?? {}
const backend =
  (typeof window !== 'undefined' ? (window as Shell).sparkquill?.backend?.() : undefined)
  ?? env.VITE_SPARKQUILL_BACKEND
  ?? 'standalone'

export const api: FamilyApi = backend === 'platform'
  ? createPlatformApi({ baseUrl: env.VITE_PLATFORM_API ?? FAMILY_API })
  : standaloneApi
export type { FamilyApi } from './familyApi'
