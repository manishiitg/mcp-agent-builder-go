import { describe, expect, it } from 'vitest'
import type { ChatTab } from '../stores/useChatStore'
import { CHIEF_OF_STAFF_PROFILE_ID, isChiefOfStaffTab } from './chiefOfStaff'

function tabWith(metadata: ChatTab['metadata']): ChatTab {
  return { name: 'Chat', metadata } as ChatTab
}

describe('isChiefOfStaffTab', () => {
  it('matches the legacy shape: multi-agent mode with no agentProfileId', () => {
    expect(isChiefOfStaffTab(tabWith({ mode: 'multi-agent' }))).toBe(true)
  })

  it('matches the new explicit shape: agentProfileId === chief-of-staff', () => {
    expect(isChiefOfStaffTab(tabWith({ mode: 'multi-agent', agentProfileId: CHIEF_OF_STAFF_PROFILE_ID }))).toBe(true)
  })

  it('rejects a different product profile', () => {
    expect(isChiefOfStaffTab(tabWith({ mode: 'multi-agent', agentProfileId: 'video-studio' }))).toBe(false)
  })

  it('rejects workflow mode even with no agentProfileId', () => {
    expect(isChiefOfStaffTab(tabWith({ mode: 'workflow' }))).toBe(false)
  })

  it('rejects a tab with no metadata at all', () => {
    expect(isChiefOfStaffTab(tabWith(undefined))).toBe(false)
  })

  it('rejects null and undefined tabs', () => {
    expect(isChiefOfStaffTab(null)).toBe(false)
    expect(isChiefOfStaffTab(undefined)).toBe(false)
  })
})
