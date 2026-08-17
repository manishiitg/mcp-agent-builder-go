import type { ChatTab } from '../stores/useChatStore'
import { isScheduledSession } from './workflowSessionKinds'

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

/**
 * A Chief-of-Staff tab that's a read-only schedule observer, not the live
 * interactive chat -- Chief of Staff has two independent lanes (see
 * useChatStore's createChatTab dedup comment), and a schedule tab must never
 * be treated as "the" chat to switch to, adopt, or reuse.
 */
export function isChiefOfStaffScheduleTab(tab: ChatTab | null | undefined): boolean {
  if (!tab) return false
  return isChiefOfStaffTab(tab) && (
    tab.metadata?.isScheduledRun === true ||
    isScheduledSession({ sessionId: tab.sessionId })
  )
}

/**
 * The live interactive Chief-of-Staff chat specifically -- excludes every
 * read-only observer variant (the schedule lane above, an
 * Organization-assistant tab, a view-only restore, a bot-triggered run).
 * This is what a surface should use to find/adopt/switch to "the" chat, not
 * bare isChiefOfStaffTab, which also matches all of those.
 */
export function isInteractiveChiefOfStaffTab(tab: ChatTab | null | undefined): boolean {
  if (!tab) return false
  return isChiefOfStaffTab(tab) &&
    tab.metadata?.isOrganizationAssistant !== true &&
    tab.metadata?.isViewOnly !== true &&
    tab.metadata?.isBotRun !== true &&
    !isChiefOfStaffScheduleTab(tab)
}
