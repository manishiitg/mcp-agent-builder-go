// The parent chat in platform mode: AgentWorks' own ChatArea, exactly as
// Video Studio hosts it, so SSE, restore, streaming, submission and tool
// rendering are the same code in every product and a fix lands everywhere.
// SparkQuill adds only what is its own: the suggestion pills under a reply,
// and the product events (presentations, family/activity/pin updates) that
// the workspace panel reacts to.
import '../../../src/index.css'
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import ChatArea, { type ChatContentRendererProps } from '../../../src/components/ChatArea'
import { TerminalEventTranscript } from '../../../src/components/TerminalEventTranscript'
import ToastHost from '../../../src/components/ui/ToastHost'
import { agentApi } from '../../../src/services/api'
import { useChatStore, waitForChatStoreHydration } from '../../../src/stores/useChatStore'
import { useModeStore } from '../../../src/stores/useModeStore'
import { useAppStore } from '../../../src/stores/useAppStore'
import { hydrateTabEvents, restoreSession } from '../../../src/utils/sessionRestore'
import { setProductCommands } from '../../../src/commands/registry'
import { parsePresentationUpdatedEvent, type PollingEvent } from '../../../shared/session'
import type { QuickCommand } from '../stores/types'
import { api } from '../api'
import { toProductCommandDefinitions } from './productCommands'

export const PARENT_PROFILE_ID = 'sparkquill'
export const PARENT_PROFILE_VERSION = 1
export const FAMILY_WORKSPACE = 'Chats/SparkQuill'

export type ProductInteraction = { kind: string; payload: Record<string, unknown> }
export type ProductPresentation = { kind: string; payload: Record<string, unknown> }

type Props = {
  title: string
  childName: string
  theme: 'light' | 'dark'
  commands: QuickCommand[]
  landing: ReactNode
  onInteraction: (e: ProductInteraction) => void
  onPresentation: (p: ProductPresentation) => void
}

const queryClient = new QueryClient()
// The one tab this page opened for the parent conversation (module-level so a
// remount finds it).
let openedTab: Promise<{ tabId: string; sessionId: string }> | null = null

function payloadOf(e: PollingEvent): Record<string, unknown> {
  return ((e.data as { data?: Record<string, unknown> } | undefined)?.data ?? {}) as Record<string, unknown>
}

/** The suggestion pills of the latest reply: the last `suggestions` interaction after the last user message. */
function latestSuggestions(events: PollingEvent[]): { label: string; message: string }[] {
  let out: { label: string; message: string }[] = []
  for (const e of events) {
    const type = e.type ?? (e.data as { type?: string } | undefined)?.type
    if (type === 'user_message') { out = []; continue }
    if (type !== 'product_interaction') continue
    const p = payloadOf(e)
    if (p.kind !== 'suggestions') continue
    const actions = ((p.payload as { actions?: unknown[] } | undefined)?.actions ?? []) as { label?: unknown; message?: unknown }[]
    out = actions
      .map((a) => ({ label: String(a.label ?? '').trim(), message: String(a.message ?? '').trim() }))
      .filter((a) => a.label && a.message)
  }
  return out
}

function SparkQuillConversation({ events, isStreaming, isRestoring, streamingText, streamingStatus, hasOlder, loadingOlder, historyError, onLoadOlder, landingContent, onSubmitQuery }: ChatContentRendererProps) {
  const suggestions = isStreaming ? [] : latestSuggestions(events)
  if (!isRestoring && events.length === 0 && !streamingText) return <>{landingContent}</>
  return (
    <div className="fl-platform-transcript">
      <TerminalEventTranscript
        events={events}
        terminal={null}
        loading={isRestoring}
        error={historyError}
        streamingText={streamingText}
        streamingStatus={streamingStatus}
        hasOlder={hasOlder}
        loadingOlder={loadingOlder}
        onLoadOlder={onLoadOlder}
        onRetry={onLoadOlder}
        surfaceClassName="fl-platform-surface"
        autoScrollMode="reveal-first-response"
      />
      {isStreaming && !streamingText && (
        <div className="fl-thinking fl-platform-working"><img src="/sparkquill-loader.svg" alt="" width={30} height={30} /><span>{streamingStatus ? `Quill is: ${streamingStatus}…` : 'Working on it…'}</span></div>
      )}
      {suggestions.length > 0 && onSubmitQuery && (
        <div className="fl-suggestions" aria-label="Recommended next steps">
          {suggestions.map((s, i) => (
            <button key={i} type="button" className="fl-suggestion" onClick={() => onSubmitQuery(s.message)}>{s.label}</button>
          ))}
        </div>
      )}
    </div>
  )
}

export default function PlatformChat({ title, childName, theme, commands, landing, onInteraction, onPresentation }: Props) {
  const [tabId, setTabId] = useState<string | null>(null)
  const [sessionId, setSessionId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  // The shared components style themselves by the html class, the way the
  // AgentWorks shell does; keep it in step with this app's theme.
  useEffect(() => {
    document.documentElement.classList.remove('light', 'dark')
    document.documentElement.classList.add(theme)
  }, [theme])

  useEffect(() => {
    setProductCommands(toProductCommandDefinitions(commands))
    return () => setProductCommands([])
  }, [commands])

  // Same sequence Video Studio runs to open its project conversation. It runs
  // once per page load: a remount (or React's dev double-invoke) reuses the
  // tab it already opened instead of re-hydrating it, which would throw away
  // the live turn in the store.
  useEffect(() => {
    let cancelled = false
    const prepare = async () => {
      if (openedTab) {
        const { tabId: existingTabId, sessionId: existingSessionId } = await openedTab
        if (cancelled) return
        useChatStore.getState().switchTab(existingTabId)
        setSessionId(existingSessionId)
        setTabId(existingTabId)
        return
      }
      openedTab = openTab()
      const opened = await openedTab
      if (cancelled) return
      setSessionId(opened.sessionId)
      setTabId(opened.tabId)
    }
    const openTab = async () => {
      // The shared service layer reads this app's login from local storage;
      // make sure it exists before the first request goes out.
      await api.ensureSession()
      useModeStore.getState().setModeCategory('multi-agent')
      useAppStore.getState().setAgentMode('multi-agent')
      await waitForChatStoreHydration()
      const chatStore = useChatStore.getState()
      const existing = Object.values(chatStore.chatTabs).find((tab) => tab.metadata?.agentProfileId === PARENT_PROFILE_ID)
      const conversation = await agentApi.resolveAgentProfileConversation(PARENT_PROFILE_ID, {}, existing?.sessionId ?? undefined)
      const createdTabId = await chatStore.createChatTab(title, {
        mode: 'multi-agent',
        agentProfileId: PARENT_PROFILE_ID,
        agentProfileVersion: PARENT_PROFILE_VERSION,
        agentProfileWorkspace: FAMILY_WORKSPACE,
        agentProfileChatContract: 'profile-v1',
        agentProfileConversationKey: conversation.conversation_key,
        agentProfileConversationId: conversation.conversation_id,
      }, conversation.session_id)
      if (!chatStore.getTab(createdTabId)) throw new Error('the conversation tab could not be created')
      const restoredTabId = await restoreSession(conversation.session_id, { title, source: 'sparkquill-open', skipConfigRestore: true, workspacePath: FAMILY_WORKSPACE })
      await hydrateTabEvents(conversation.session_id, { workspacePath: FAMILY_WORKSPACE, fallbackToChatHistory: true, preferChatHistory: true })
      chatStore.switchTab(restoredTabId)
      return { tabId: restoredTabId, sessionId: conversation.session_id }
    }
    void prepare().catch((err) => {
      openedTab = null
      if (!cancelled) setError(err instanceof Error ? err.message : 'Could not open the conversation.')
    })
    return () => { cancelled = true }
  }, [title])

  // Product events ride the same stream ChatArea opens; hand the new ones to
  // the workspace panel. Events already present when the tab is opened are
  // history, not something to act on again.
  const events = useChatStore((s) => (sessionId ? s.tabEvents[sessionId] : undefined))
  const seen = useRef<Set<string> | null>(null)
  useEffect(() => {
    if (!events) return
    if (!seen.current) {
      seen.current = new Set(events.map((e) => e.id))
      return
    }
    for (const e of events) {
      if (seen.current.has(e.id)) continue
      seen.current.add(e.id)
      const type = e.type ?? (e.data as { type?: string } | undefined)?.type
      if (type === 'product_interaction') {
        const p = payloadOf(e)
        onInteraction({ kind: String(p.kind ?? ''), payload: (p.payload ?? {}) as Record<string, unknown> })
      } else if (type === 'presentation_updated') {
        const parsed = parsePresentationUpdatedEvent(e)
        if (parsed) onPresentation({ kind: parsed.kind, payload: parsed.payload })
      }
    }
  }, [events, onInteraction, onPresentation])

  return (
    <QueryClientProvider client={queryClient}>
      <div className="fl-platform-chat">
        {error ? (
          <p className="fl-note">Couldn’t open the conversation: {error}</p>
        ) : tabId ? (
          <ChatArea
            tabId={tabId}
            onNewChat={() => {}}
            landingContent={landing}
            contentRenderer={SparkQuillConversation}
            inputVariant="product"
            fullTurnStreaming
            hideRuntimeStatus
            composerPlaceholder={`Ask anything about ${childName || 'your child'}’s learning…`}
          />
        ) : (
          <p className="fl-note">Connecting to Quill…</p>
        )}
        <ToastHost />
      </div>
    </QueryClientProvider>
  )
}
