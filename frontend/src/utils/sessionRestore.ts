import { useChatStore } from '../stores/useChatStore'
import { useModeStore } from '../stores/useModeStore'
import { agentApi } from '../services/api'
import type { ChatHistoryConversation, ChatHistoryMessage, PollingEvent } from '../services/api-types'
import { truncateTabTitle } from './textUtils'
import axios from 'axios'

const TAG = '[SessionRestore]'

type RuntimeSessionState = {
  status: string
  hasRunningBackgroundAgents?: boolean
  isSyntheticTurn?: boolean
  canSteer?: boolean
}

function isForegroundStreaming(state: RuntimeSessionState): boolean {
  if (state.status !== 'running') return false
  // Background-only work should not lock the composer after restore.
  // Synthetic auto-notification turns are activity, but they should not queue user input.
  return !state.isSyntheticTurn && (!state.hasRunningBackgroundAgents || !!state.canSteer)
}

/**
 * Per-session async lock to prevent duplicate restores.
 * If restoreSession is called concurrently for the same session,
 * subsequent calls return the existing Promise.
 */
const restoreInProgress = new Map<string, Promise<string>>()

/**
 * Apply session status (completed/streaming/restored) to a tab.
 */
function applySessionStatus(tabId: string, state: RuntimeSessionState): void {
  const chatStore = useChatStore.getState()
  const isDone = state.status === 'completed' || state.status === 'stopped'
  const isError = state.status === 'error'
  chatStore.setTabCompleted(tabId, isDone)
  chatStore.setTabStreaming(tabId, isDone || isError ? false : isForegroundStreaming(state))
  chatStore.setTabHasRunningBgAgents(tabId, !!state.hasRunningBackgroundAgents)
  chatStore.setTabSyntheticTurn(tabId, !!state.isSyntheticTurn)
  chatStore.setTabCanSteer(tabId, !!state.canSteer)
  if (isDone || isError) {
    chatStore.setTabMetadata(tabId, { isRestored: true })
  }
}

/**
 * Unified session restoration function.
 * Handles all restore flows: auto-restore, page-refresh hydration, sidebar click, resume dialog.
 *
 * Returns the tabId for the restored session.
 */
export async function restoreSession(
  sessionId: string,
  options?: {
    title?: string
    source?: string
    skipConfigRestore?: boolean
    workspacePath?: string
  }
): Promise<string> {
  // Async lock: if already restoring this session, return the existing promise
  const existing = restoreInProgress.get(sessionId)
  if (existing) {
    console.log(`${TAG} Dedup hit for ${sessionId} (source=${options?.source}), returning existing promise`)
    return existing
  }

  const promise = doRestoreSession(sessionId, options)
  restoreInProgress.set(sessionId, promise)

  try {
    return await promise
  } finally {
    restoreInProgress.delete(sessionId)
  }
}

async function doRestoreSession(
  sessionId: string,
  options?: {
    title?: string
    source?: string
    skipConfigRestore?: boolean
    workspacePath?: string
  }
): Promise<string> {
  const src = options?.source || 'unknown'
  console.log(`${TAG} Start session=${sessionId} source=${src} title=${options?.title ?? '(none)'}`)
  const chatStore = useChatStore.getState()

  // Step 1: Check for existing tab with events already loaded
  const existingTabWithSession = Object.values(chatStore.chatTabs).find(tab => tab.sessionId === sessionId)
  const existingTab = existingTabWithSession
  const existingEventCount = existingTab ? chatStore.getTabEvents(sessionId).length : 0
  if (existingTab) {
    if (existingEventCount > 0) {
      console.log(`${TAG} [${src}] Tab ${existingTab.tabId} already has ${existingEventCount} events, refreshing runtime state`)
    } else {
      console.log(`${TAG} [${src}] Tab ${existingTab.tabId} exists but has 0 events, will hydrate`)
    }
  }

  // Chat sessions are in-memory on the backend now — there is no persisted
  // session metadata to fetch. Tab state (title, config) is the frontend's
  // responsibility; session status comes from the polling API.
  const tabMode = 'multi-agent' as const
  useModeStore.getState().setModeCategory('multi-agent')

  let tabId: string
  if (existingTab) {
    tabId = existingTab.tabId
    console.log(`${TAG} [${src}] Reusing existing tab ${tabId}`)
  } else {
    const title = truncateTabTitle(options?.title || 'Chat')
    tabId = await chatStore.createChatTab(
      title,
      { mode: tabMode, isRestored: false },
      sessionId,
    )
    console.log(`${TAG} [${src}] Created tab ${tabId} mode=${tabMode}`)
  }

  // Step 7: Sync runtime state / events
  try {
    if (existingEventCount > 0) {
      const currentLastIndex = chatStore.getTabLastEventIndex(sessionId)
      const runtime = await agentApi.getSessionEvents(sessionId, currentLastIndex)
      applySessionStatus(tabId, {
        status: runtime.session_status,
        hasRunningBackgroundAgents: runtime.has_running_background_agents,
        isSyntheticTurn: runtime.is_synthetic_turn,
        canSteer: runtime.can_steer,
      })
      if (runtime.events.length > 0) {
        chatStore.addTabEvents(sessionId, runtime.events)
      }
      if (
        options?.workspacePath &&
        !isForegroundStreaming({
          status: runtime.session_status,
          hasRunningBackgroundAgents: runtime.has_running_background_agents,
          isSyntheticTurn: runtime.is_synthetic_turn,
          canSteer: runtime.can_steer,
        })
      ) {
        // A completed coding-agent turn's live event window often contains an
        // empty provider tool-start. Replace it with the durable structured
        // transcript, which is enriched with bridge-authoritative arguments.
        // This also fixes partial browser caches after a backend restart.
        await hydrateTabEventsFromChatHistory(sessionId, options.workspacePath)
      }
      if (runtime.last_processed_index !== undefined) {
        chatStore.setTabLastEventIndex(sessionId, runtime.last_processed_index)
      }
      if (runtime.has_more !== undefined) {
        chatStore.setTabHasMoreOlderEvents(sessionId, runtime.has_more)
      }
      console.log(`${TAG} [${src}] Refreshed runtime state for existing tab ${tabId}`)
    } else {
      const runtime = await hydrateTabEvents(sessionId, {
        workspacePath: existingTab?.metadata?.agentProfileWorkspace,
        fallbackToChatHistory: true,
      })
      applySessionStatus(tabId, runtime)
      const eventCount = chatStore.getTabEvents(sessionId).length
      console.log(`${TAG} [${src}] Hydrated ${eventCount} events`)
    }
  } catch (err) {
    if (isNotFoundError(err) && existingEventCount > 0) {
      console.log(`${TAG} [${src}] Session ${sessionId} no longer in memory; keeping locally restored events`)
      applySessionStatus(tabId, {
        status: 'completed',
        hasRunningBackgroundAgents: false,
        isSyntheticTurn: false,
        canSteer: false,
      })
    } else {
      console.error(`${TAG} [${src}] Failed to sync runtime state for ${sessionId}:`, err)
    }
  }

  console.log(`${TAG} [${src}] Done session=${sessionId} tab=${tabId}`)
  return tabId
}

function isNotFoundError(error: unknown): boolean {
  return axios.isAxiosError(error) && error.response?.status === 404
}

function getMessageRole(message: ChatHistoryMessage): string {
  return String(message.Role || message.role || '').toLowerCase()
}

function getMessageText(message: ChatHistoryMessage): string {
  const parts = message.Parts || message.parts || []
  const texts = parts
    .map(part => {
      if (!part || typeof part !== 'object') return ''
      return part.Text || part.text || part.Content || part.content || ''
    })
    .filter(text => typeof text === 'string' && text.trim().length > 0)
  return texts.join('\n\n')
}

function makeRestoredEvent(
  sessionId: string,
  type: string,
  data: Record<string, unknown>,
  index: number,
): PollingEvent {
  const timestamp = typeof data.timestamp === 'string' ? data.timestamp : new Date().toISOString()
  return {
    id: `restored-${sessionId}-${index}-${type}`,
    type,
    timestamp,
    session_id: sessionId,
    event_index: index,
    data: {
      type,
      timestamp,
      session_id: sessionId,
      data: {
        timestamp,
        session_id: sessionId,
        ...data,
      },
    },
  } as PollingEvent
}

function conversationToRestoredEvents(conversation: ChatHistoryConversation): PollingEvent[] {
  const sessionId = conversation.session_id
  const messages = conversation.conversation_history || []
  const events: PollingEvent[] = [
    makeRestoredEvent(sessionId, 'conversation_resumed', {
      previous_event_count: messages.length,
      restored_from: 'workspace_chat_history',
    }, 0),
  ]

  let turn = 0
  let lastUserMessage = ''
  let lastAssistantMessage = ''

  for (const message of messages) {
    const role = getMessageRole(message)
    if (role === 'system' || role === 'tool') continue

    const content = getMessageText(message)
    if (!content) continue

    if (role === 'human' || role === 'user') {
      turn += 1
      lastUserMessage = content
      events.push(makeRestoredEvent(sessionId, 'user_message', {
        content,
        role: 'user',
        turn,
      }, events.length))
    } else if (role === 'ai' || role === 'assistant') {
      lastAssistantMessage = content
      events.push(makeRestoredEvent(sessionId, 'conversation_end', {
        status: 'completed',
        question: lastUserMessage,
        result: content,
        turns: turn,
      }, events.length))
    }
  }

  if (events.length === 1 && lastAssistantMessage) {
    events.push(makeRestoredEvent(sessionId, 'conversation_end', {
      status: 'completed',
      result: lastAssistantMessage,
      turns: turn,
    }, events.length))
  }

  return events
}

// Coding-provider stream events can reach persistence with an empty
// tool_params.arguments, while the same conversation's structured tool-call
// message has the complete arguments. Retain the raw event's timing/result and
// hydrate only that missing input by the stable tool call id.
function restoreToolArgumentsFromConversation(
  events: PollingEvent[],
  conversation: ChatHistoryConversation,
): PollingEvent[] {
  const argumentsByCallID = new Map<string, string>()
  for (const message of conversation.conversation_history || []) {
    for (const part of message.Parts || message.parts || []) {
      if (!part || typeof part !== 'object') continue
      const record = part as Record<string, unknown>
      const callID = typeof record.ID === 'string' ? record.ID : ''
      const call = record.FunctionCall
      const args = call && typeof call === 'object' && typeof (call as Record<string, unknown>).Arguments === 'string'
        ? (call as Record<string, unknown>).Arguments as string
        : ''
      if (callID && args) argumentsByCallID.set(callID, args)
    }
  }
  if (argumentsByCallID.size === 0) return events

  return events.map((event) => {
    if (event.type !== 'tool_call_start') return event
    const envelope = event.data
    if (!envelope || typeof envelope !== 'object') return event
    const outer = envelope as Record<string, unknown>
    const nested = outer.data
    if (!nested || typeof nested !== 'object') return event
    const fields = nested as Record<string, unknown>
    const callID = typeof fields.tool_call_id === 'string' ? fields.tool_call_id : ''
    const args = argumentsByCallID.get(callID)
    if (!args) return event
    const existingParams = fields.tool_params && typeof fields.tool_params === 'object'
      ? fields.tool_params as Record<string, unknown>
      : {}
    return {
      ...event,
      data: {
        ...outer,
        data: { ...fields, tool_params: { ...existingParams, arguments: args } },
      },
    } as PollingEvent
  })
}

async function hydrateTabEventsFromChatHistory(sessionId: string, workspacePath?: string): Promise<RuntimeSessionState> {
  const chatStore = useChatStore.getState()
  const conversation = await agentApi.getChatHistoryConversation(sessionId, workspacePath)
  const rawEvents = (conversation.ui_events && conversation.ui_events.length > 0)
    ? (conversation.ui_events as PollingEvent[])
    : conversationToRestoredEvents(conversation)
  const events = restoreToolArgumentsFromConversation(rawEvents, conversation)

  chatStore.setTabEvents(sessionId, events)
  chatStore.setTabLastEventIndex(sessionId, events.length - 1)
  chatStore.setTabHasMoreOlderEvents(sessionId, false)

  return {
    status: 'completed',
    hasRunningBackgroundAgents: false,
    isSyntheticTurn: false,
    canSteer: false,
  }
}

async function tryHydrateTabEventsFromChatHistory(
  sessionId: string,
  workspacePath?: string,
): Promise<RuntimeSessionState | null> {
  try {
    return await hydrateTabEventsFromChatHistory(sessionId, workspacePath)
  } catch (error) {
    if (isNotFoundError(error)) {
      return null
    }
    throw error
  }
}

/**
 * Load events from the in-memory polling API and hydrate a tab's event state.
 * If the server restarted and no longer has the session in memory, restore
 * displayable conversation history from the workspace-backed chat history file.
 */
export async function hydrateTabEvents(
  sessionId: string,
  options: {
    workspacePath?: string
    fallbackToChatHistory?: boolean
  } = {},
): Promise<RuntimeSessionState> {
  const chatStore = useChatStore.getState()
  let response
  try {
    response = await agentApi.getRecentSessionEvents(sessionId)
  } catch (error) {
    if (isNotFoundError(error)) {
      console.log(`${TAG} Polling session ${sessionId} not found; restoring from workspace chat history`)
      const restored = await tryHydrateTabEventsFromChatHistory(sessionId, options.workspacePath)
      if (restored) return restored
    }
    throw error
  }

  if (response.events.length > 0) {
    chatStore.setTabEvents(sessionId, response.events)
    const lastIndex = response.last_processed_index ?? (response.events.length - 1)
    chatStore.setTabLastEventIndex(sessionId, lastIndex)
    if (response.has_more !== undefined) {
      chatStore.setTabHasMoreOlderEvents(sessionId, response.has_more)
    }
  } else if (options.fallbackToChatHistory) {
    const restored = await tryHydrateTabEventsFromChatHistory(sessionId, options.workspacePath)
    if (restored) {
      return {
        status: response.session_status || restored.status,
        hasRunningBackgroundAgents: response.has_running_background_agents,
        isSyntheticTurn: response.is_synthetic_turn,
        canSteer: response.can_steer,
      }
    }
  }
  return {
    status: response.session_status,
    hasRunningBackgroundAgents: response.has_running_background_agents,
    isSyntheticTurn: response.is_synthetic_turn,
    canSteer: response.can_steer,
  }
}
