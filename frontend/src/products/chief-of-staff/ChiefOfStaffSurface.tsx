import { useEffect, useState } from 'react'
import { Users } from 'lucide-react'
import ChatArea from '../../components/ChatArea'
import { FileContentViewer } from '../../components/FileContentViewer'
import { ProductSurfaceSwitcher } from '../../components/ProductSurfaceSwitcher'
import { ChiefOfStaffMark } from './ChiefOfStaffMark'
import { useAppStore } from '../../stores/useAppStore'
import { useChatStore, waitForChatStoreHydration } from '../../stores/useChatStore'
import { useModeStore } from '../../stores/useModeStore'
import { restoreSession } from '../../utils/sessionRestore'
import { CHIEF_OF_STAFF_PROFILE_ID, isInteractiveChiefOfStaffTab } from '../../utils/chiefOfStaff'

function ChiefOfStaffHeader() {
  return (
    <header className="flex h-[62px] shrink-0 items-center gap-4 border-b border-slate-200 bg-white px-4 dark:border-slate-800 dark:bg-slate-950">
      <ProductSurfaceSwitcher />
      <div className="hidden h-7 w-px bg-slate-200 sm:block dark:bg-slate-800" />
      <div className="hidden items-center gap-2 text-xs font-semibold text-slate-700 sm:flex dark:text-slate-300">
        <ChiefOfStaffMark className="h-5 w-5" />
        Chief of Staff
      </div>
    </header>
  )
}

function ChiefOfStaffWelcome() {
  return (
    <div className="flex h-full items-center justify-center px-6 py-10">
      <div className="max-w-md text-center">
        <span className="mx-auto grid h-14 w-14 place-items-center rounded-2xl bg-indigo-100 text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300">
          <Users className="h-6 w-6" />
        </span>
        <h2 className="mt-5 text-lg font-semibold text-slate-900 dark:text-slate-100">What do you need?</h2>
        <p className="mt-2 text-sm leading-6 text-slate-500 dark:text-slate-400">
          Ask about any workflow's status, delegate a task, or check what needs your attention. Chief of Staff has
          read-only visibility across every automation.
        </p>
      </div>
    </div>
  )
}

/**
 * Chief of Staff's product surface -- chat-first, ChatArea-direct like Video
 * Studio, not the ChatTabs/ModePresetBar chrome AgentWorks uses. There is no
 * project-list home screen here: unlike Video Studio, there's exactly one
 * singleton Chief-of-Staff conversation to open, not a list to choose from.
 * A real automations-oversight dashboard as a landing view in front of this
 * chat is planned as a separate follow-up, not part of this pass.
 */
export function ChiefOfStaffSurface() {
  const [tabId, setTabId] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    const prepare = async () => {
      useModeStore.getState().setModeCategory('multi-agent')
      useAppStore.getState().setAgentMode('multi-agent')
      await waitForChatStoreHydration()
      if (cancelled) return
      const chatStore = useChatStore.getState()

      let tab = Object.values(chatStore.chatTabs).find(isInteractiveChiefOfStaffTab)
      if (tab && tab.metadata?.agentProfileId !== CHIEF_OF_STAFF_PROFILE_ID) {
        // Adopt a legacy no-profile tab in place, without touching its
        // session/history -- lazy and idempotent. isChiefOfStaffTab already
        // treats both shapes as equivalent everywhere else, so a user who
        // never opens this surface again is unaffected either way.
        chatStore.setTabMetadata(tab.tabId, {
          agentProfileId: CHIEF_OF_STAFF_PROFILE_ID,
          agentProfileVersion: 1,
          agentProfileWorkspace: 'Chats',
          agentProfileProjectTitle: 'Chief of Staff',
        })
        tab = chatStore.getTab(tab.tabId)
      }
      if (!tab) {
        const createdTabId = await chatStore.createChatTab('Chief of Staff', {
          mode: 'multi-agent',
          agentProfileId: CHIEF_OF_STAFF_PROFILE_ID,
          agentProfileVersion: 1,
          agentProfileWorkspace: 'Chats',
          agentProfileProjectTitle: 'Chief of Staff',
        })
        tab = chatStore.getTab(createdTabId)
      }
      if (cancelled || !tab) return

      const restoredTabId = await restoreSession(tab.sessionId ?? tab.tabId, {
        title: 'Chief of Staff',
        source: 'chief-of-staff-open',
        skipConfigRestore: true,
      })
      if (cancelled) return
      chatStore.switchTab(restoredTabId)
      setTabId(restoredTabId)
    }
    void prepare()
    return () => { cancelled = true }
  }, [])

  return (
    <div className="flex h-screen min-h-0 flex-col overflow-hidden bg-slate-50 dark:bg-slate-950">
      <ChiefOfStaffHeader />
      <main className="min-h-0 flex-1 overflow-hidden bg-white dark:bg-slate-950">
        {tabId ? (
          <ChatArea
            tabId={tabId}
            onNewChat={() => {}}
            landingContent={<ChiefOfStaffWelcome />}
            fullTurnStreaming
            showConversationUsage
          />
        ) : (
          <div className="grid h-full place-items-center text-xs text-slate-400">Connecting…</div>
        )}
      </main>
      <FileContentViewer />
    </div>
  )
}
