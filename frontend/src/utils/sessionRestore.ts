import { useChatStore } from '../stores/useChatStore'
import { useModeStore } from '../stores/useModeStore'
import { agentApi } from '../services/api'
import type { ChatHistoryConversation, ChatHistoryMessage, PollingEvent } from '../services/api-types'
import { truncateTabTitle } from './textUtils'
import axios from 'axios'
import { isProviderTranscriptArtifact } from './restoredConversationFilter'

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
      // Every chat uses the same recovery contract: the workspace-backed
      // conversation is the authoritative durable transcript, while the
      // polling endpoint is only the volatile live tail. Do this for all
      // products (and while a turn is running) so a refresh cannot leave a
      // user-only event buffer in the UI. If a legacy session has no durable
      // transcript, retain the live events already loaded above.
      await tryHydrateTabEventsFromChatHistory(
        sessionId,
        options?.workspacePath || existingTab?.metadata?.agentProfileWorkspace,
      )
      if (runtime.last_processed_index !== undefined) {
        chatStore.setTabLastEventIndex(sessionId, runtime.last_processed_index)
      }
      if (runtime.has_more !== undefined) {
        chatStore.setTabHasMoreOlderEvents(sessionId, runtime.has_more)
      }
      console.log(`${TAG} [${src}] Refreshed runtime state for existing tab ${tabId}`)
    } else {
      const runtime = await hydrateTabEvents(sessionId, {
        workspacePath: options?.workspacePath || existingTab?.metadata?.agentProfileWorkspace,
        fallbackToChatHistory: true,
      })
      applySessionStatus(tabId, runtime)
      const eventCount = chatStore.getTabEvents(sessionId).length
      console.log(`${TAG} [${src}] Hydrated ${eventCount} events`)
    }
  } catch (err) {
    const workspacePath = options?.workspacePath || existingTab?.metadata?.agentProfileWorkspace
    // A bounded live cursor can be rejected after a server restart. The
    // durable transcript is independent of that volatile window and applies
    // equally to every product, so recover it before preserving an incomplete
    // local cache.
    const restored = await tryHydrateTabEventsFromChatHistory(sessionId, workspacePath)
    if (restored) {
      applySessionStatus(tabId, restored)
      console.log(`${TAG} [${src}] Recovered persisted transcript after runtime sync failure`)
      return tabId
    }
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

export function conversationToRestoredEvents(conversation: ChatHistoryConversation): PollingEvent[] {
  const sessionId = conversation.session_id
  const messages = conversation.conversation_history || []
  // Page identity is durable across "Load earlier" requests. Without this,
  // every page restarts at restored-…-0 and the event store/UI deduplicates
  // distinct older turns as if they were the newest page.
  const eventIndexBase = Math.max(0, conversation.history_pagination?.start_turn ?? 0) * 2
  const events: PollingEvent[] = [
    makeRestoredEvent(sessionId, 'conversation_resumed', {
      previous_event_count: messages.length,
      has_more_history: conversation.history_pagination?.has_more === true,
      restored_from: 'workspace_chat_history',
    }, eventIndexBase),
  ]

  // Scheduled read-only restore may request the parent's compact persisted UI
  // trace. It preserves the actual child-agent prompts, answers and paired
  // tools that the conversational summary intentionally omits. Do not add a
  // synthetic second copy of the user/final messages in that case.
  if (conversation.ui_events && conversation.ui_events.length > 0) {
    return [
      ...events,
      ...conversation.ui_events.map((event, index) => markPersistedRestoreTrace(event as PollingEvent, sessionId, eventIndexBase + index + 1)),
    ]
  }

  let turn = 0
  let currentQuestion = ''
  let pendingAssistant = ''
  const flushAssistant = () => {
    if (!pendingAssistant) return
    // Recreate both answer carriers emitted by a live turn. The developer
    // transcript reads llm_generation_end, while the product conversation
    // reads unified_completion. The transcript layer deduplicates matching
    // carriers when both are displayed together.
    events.push(makeRestoredEvent(sessionId, 'llm_generation_end', {
      status: 'completed',
      question: currentQuestion,
      content: pendingAssistant,
      result: pendingAssistant,
      turns: turn,
    }, eventIndexBase + events.length))
    events.push(makeRestoredEvent(sessionId, 'unified_completion', {
      status: 'completed',
      question: currentQuestion,
      final_result: pendingAssistant,
      result: pendingAssistant,
      turns: turn,
    }, eventIndexBase + events.length))
    pendingAssistant = ''
  }

  for (const message of messages) {
    const role = getMessageRole(message)
    if (role === 'system' || role === 'tool') continue

    const content = getMessageText(message)
    if (!content) continue

    if (role === 'human' || role === 'user') {
      flushAssistant()
      turn += 1
      currentQuestion = content
      events.push(makeRestoredEvent(sessionId, 'user_message', {
        content,
        role: 'user',
        turn,
      }, eventIndexBase + events.length))
    } else if (role === 'ai' || role === 'assistant') {
      if (isProviderTranscriptArtifact(content)) continue
      // Coding providers persist commentary and tool markers as separate AI
      // messages. The final ordinary AI message before the next user message
      // is the completed reply that belongs in the resumed chat.
      pendingAssistant = content
    }
  }
  flushAssistant()

  return events
}

function markPersistedRestoreTrace(event: PollingEvent, parentSessionId: string, eventIndex: number): PollingEvent {
  const outer = event.data && typeof event.data === 'object' ? event.data as Record<string, unknown> : {}
  const nested = outer.data && typeof outer.data === 'object' ? outer.data as Record<string, unknown> : {}
  const metadata = nested.metadata && typeof nested.metadata === 'object' ? nested.metadata as Record<string, unknown> : {}
  return {
    ...event,
    // Persisted UI events are recorded under the parent session even when the
    // nested event belongs to a background child. Preserve that ownership so
    // one restored schedule remains one tab/timeline.
    session_id: event.session_id || parentSessionId,
    event_index: typeof event.event_index === 'number' ? event.event_index : eventIndex,
    data: {
      ...outer,
      data: {
        ...nested,
        metadata: { ...metadata, restored_persisted_trace: true },
      },
    },
  } as PollingEvent
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

async function hydrateTabEventsFromChatHistory(sessionId: string, workspacePath?: string, includeUiEvents = false): Promise<RuntimeSessionState> {
  const chatStore = useChatStore.getState()
  // getChatHistoryResumeConversation (not the unbounded preview variant) keeps
  // this lightweight, and conversationToRestoredEvents does the real work of
  // projecting persisted turns into replayable events. Tool-call arguments
  // still need a pass of their own: provider stream events can persist with
  // empty tool_params.arguments even though the structured conversation_history
  // has them, so restoreToolArgumentsFromConversation patches those back in
  // regardless of which path built the underlying events.
  const conversation = includeUiEvents
    ? await agentApi.getChatHistoryResumeConversation(sessionId, workspacePath, 100, 0, true)
    : await agentApi.getChatHistoryResumeConversation(sessionId, workspacePath)
  const rawEvents = conversationToRestoredEvents(conversation)
  const events = restoreToolArgumentsFromConversation(rawEvents, conversation)

  chatStore.setTabEvents(sessionId, events)
  chatStore.setTabLastEventIndex(sessionId, events.length - 1)
  chatStore.setTabHasMoreOlderEvents(sessionId, conversation.history_pagination?.has_more ?? false)
  chatStore.setTabHistoryPagination(
    sessionId,
    conversation.history_pagination
      ? {
          hasMore: conversation.history_pagination.has_more,
          nextOffset: conversation.history_pagination.next_offset,
        }
      : null,
  )
  console.info(`${TAG} Hydrated persisted conversation`, {
    sessionId,
    eventCount: events.length,
    source: includeUiEvents ? 'conversation_history + persisted_ui_events' : 'conversation_history',
  })

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
  includeUiEvents = false,
): Promise<RuntimeSessionState | null> {
  try {
    return await hydrateTabEventsFromChatHistory(sessionId, workspacePath, includeUiEvents)
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
    // Kept for callers that explicitly request history. Durable history is
    // now the default for every session, so this no longer changes behavior.
    preferChatHistory?: boolean
    // Read-only schedule restore asks for its bounded persisted UI trace so
    // background child work remains inspectable after server restart.
    includeUiEvents?: boolean
  } = {},
): Promise<RuntimeSessionState> {
  const chatStore = useChatStore.getState()

  let response
  try {
    response = await agentApi.getRecentSessionEvents(sessionId)
  } catch (error) {
    if (isNotFoundError(error)) {
      console.log(`${TAG} Polling session ${sessionId} not found; restoring from workspace chat history`)
      const restored = await tryHydrateTabEventsFromChatHistory(sessionId, options.workspacePath, options.includeUiEvents)
      if (restored) return restored
    }
    throw error
  }

  // The event store is a short-lived transport cache and can contain only
  // prompts after a browser reload. Prefer the durable conversation for every
  // session, regardless of its owning product. Runtime status remains
  // authoritative so a currently running turn still renders as streaming.
  const restored = await tryHydrateTabEventsFromChatHistory(sessionId, options.workspacePath, options.includeUiEvents)
  if (restored) {
    return {
      status: response.session_status || restored.status,
      hasRunningBackgroundAgents: response.has_running_background_agents,
      isSyntheticTurn: response.is_synthetic_turn,
      canSteer: response.can_steer,
    }
  }

  if (response.events.length > 0) {
    chatStore.setTabEvents(sessionId, response.events)
    // This is a live event window, not a paged durable conversation. A cursor
    // left over from an earlier resume must not offer unrelated history here.
    chatStore.setTabHistoryPagination(sessionId, null)
    const lastIndex = response.last_processed_index ?? (response.events.length - 1)
    chatStore.setTabLastEventIndex(sessionId, lastIndex)
    if (response.has_more !== undefined) {
      chatStore.setTabHasMoreOlderEvents(sessionId, response.has_more)
    }
  } else if (options.fallbackToChatHistory) {
    // A restored terminal can recreate an in-memory session shell whose status
    // is "completed" but whose event buffer is empty. Status therefore cannot
    // tell us whether the durable transcript exists. The caller only enables
    // this fallback for an explicitly restored chat, so prefer its persisted
    // history whenever the volatile event buffer has no events.
    const restored = await tryHydrateTabEventsFromChatHistory(sessionId, options.workspacePath, options.includeUiEvents)
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
