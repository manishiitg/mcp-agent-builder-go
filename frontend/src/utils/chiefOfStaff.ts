import type { ChatTab } from '../stores/useChatStore'

export const CHIEF_OF_STAFF_PROFILE_ID = 'chief-of-staff'

/**
 * True for both the legacy no-profile shape (mode: 'multi-agent', no
 * agentProfileId -- how every Chief of Staff tab looked before it had a real
 * product.yaml profile) and the new explicit shape
 * (agentProfileId === 'chief-of-staff'). Mirrors the same duality the
 * backend's widened isChiefOfStaffChat check already implements
 * (agent_go/cmd/server/server.go): resolvedProfile == nil ||
 * isGlobalScopedProfile(resolvedProfile). Neither shape is forcibly migrated
 * to the other -- a legacy tab keeps working exactly as it always has.
 *
 * `mode: 'multi-agent'` itself is unchanged and unrelated to this -- it's
 * the shared execution substrate every product (including Video Studio)
 * already runs on, not something Chief of Staff owns or redefines.
 */
export function isChiefOfStaffTab(tab: ChatTab | null | undefined): boolean {
  if (!tab || tab.metadata?.mode !== 'multi-agent') return false
  const profileId = tab.metadata?.agentProfileId
  return !profileId || profileId === CHIEF_OF_STAFF_PROFILE_ID
}
