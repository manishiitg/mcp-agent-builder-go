// A read-only look at the isolated check-in conversation (product.yaml
// schedules[].isolated), opened by a known session id rather than resolved
// like the parent's own chat — that resolve endpoint refuses this
// conversation's key on purpose (a singleton profile only hands back "main"
// to a client), and this conversation isn't one a person turns anyway; the
// scheduler is its only writer. Uses AgentWorks' own restore/hydrate/render
// path (the same one PlatformChat uses for the parent's chat), read-only.
import { useEffect, useState } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import ChatArea from '../../../components/ChatArea'
import { useChatStore, waitForChatStoreHydration } from '../../../stores/useChatStore'
import { useModeStore } from '../../../stores/useModeStore'
import { useAppStore } from '../../../stores/useAppStore'
import { hydrateTabEvents, restoreSession } from '../../../utils/sessionRestore'
import { api } from '../api'
import { FAMILY_WORKSPACE, PARENT_PROFILE_ID, PARENT_PROFILE_VERSION, SparkQuillConversation, queryClient } from './PlatformChat'

export default function PulseHistoryViewer({ sessionId, onClose }: { sessionId: string; onClose: () => void }) {
  const [tabId, setTabId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    let openedTabId: string | null = null
    const open = async () => {
      await api.ensureSession()
      useModeStore.getState().setModeCategory('multi-agent')
      useAppStore.getState().setAgentMode('multi-agent')
      await waitForChatStoreHydration()
      const chatStore = useChatStore.getState()
      const createdTabId = await chatStore.createChatTab('Check-in history', {
        mode: 'multi-agent',
        agentProfileId: PARENT_PROFILE_ID,
        agentProfileVersion: PARENT_PROFILE_VERSION,
        agentProfileWorkspace: FAMILY_WORKSPACE,
        agentProfileChatContract: 'profile-v1',
      }, sessionId)
      if (!chatStore.getTab(createdTabId)) throw new Error('the check-in history tab could not be created')
      const restoredTabId = await restoreSession(sessionId, { title: 'Check-in history', source: 'sparkquill-pulse-history', skipConfigRestore: true, workspacePath: FAMILY_WORKSPACE })
      await hydrateTabEvents(sessionId, { workspacePath: FAMILY_WORKSPACE, fallbackToChatHistory: true, preferChatHistory: true })
      if (cancelled) { await chatStore.closeTab(restoredTabId, true, false); return }
      openedTabId = restoredTabId
      chatStore.switchTab(restoredTabId)
      setTabId(restoredTabId)
    }
    void open().catch((err) => {
      if (!cancelled) setError(err instanceof Error ? err.message : 'Could not open the check-in history.')
    })
    return () => {
      cancelled = true
      if (openedTabId) void useChatStore.getState().closeTab(openedTabId, true, false)
    }
  }, [sessionId])

  return (
    <div className="fl-pulse-history-overlay" role="dialog" aria-label="Check-in history">
      <div className="fl-pulse-history-panel">
        <div className="fl-pulse-history-head">
          <span>Check-in history</span>
          <button type="button" className="fl-pulse-popover-close" onClick={onClose} aria-label="Close">×</button>
        </div>
        <div className="fl-pulse-history-body">
          {error ? (
            <p className="fl-note">Couldn't open the check-in history: {error}</p>
          ) : tabId ? (
            <QueryClientProvider client={queryClient}>
              <ChatArea
                tabId={tabId}
                onNewChat={() => {}}
                contentRenderer={SparkQuillConversation}
                inputVariant="product"
                fullTurnStreaming
                hideRuntimeStatus
                hideInput
              />
            </QueryClientProvider>
          ) : (
            <p className="fl-note">Loading…</p>
          )}
        </div>
      </div>
    </div>
  )
}
