import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Activity, ArrowUpRight, Bot, CalendarClock, CheckCircle2, ChevronDown, ChevronRight, CircleAlert, CircleDashed, Clock3, Code2, Loader2, MessageSquare, Paperclip, Play, RotateCcw, Trash2, XCircle, type LucideIcon } from 'lucide-react'
import { agentApi } from '../services/api'
import { schedulerApi } from '../api/scheduler'
import {
  type ChatHistoryConversation,
  type ChatHistoryMessage,
  type ChatHistoryPreviewMessage,
  type ChatHistorySession,
  type ScheduledJob,
  type ScheduledJobRun,
} from '../services/api-types'
import { useChatStore } from '../stores/useChatStore'
import { isScheduledChatHistorySession } from '../utils/chatHistoryOpenDisposition'
import { ConversationMarkdownRenderer } from './ui/MarkdownRenderer'
import {
  CHAT_HISTORY_CLEANUP_AGE_OPTIONS,
  type ChatHistoryCleanupAgeDays,
  CleanupOldChatsDropdown,
} from './CleanupOldChatsDropdown'

const PAGE_SIZE = 5
// Keep the first paint cheap. Workflow builder conversations can be large, and
// the backend has to parse each returned file to build previews. The panel only
// renders five rows at a time, so loading 100 upfront makes the spinner feel
// stuck on workflows with months of schedule/pulse history.
const FETCH_LIMIT = 25
// How many stored messages the first expand pulls. Every fetched message is
// rendered (assistant prose, tool calls, tool results), so this is the whole
// budget rather than a fetch-more-than-you-show buffer -- "Load more" walks
// further back through the conversation in increments.
const EXPANDED_FETCH_LIMIT = 16
const EXPANDED_FETCH_INCREMENT = 30
// Fallback rows built from the session-list metadata (session.preview_messages),
// shown before the details fetch lands or if it fails.
const PREVIEW_MESSAGE_LIMIT = 14

type PreviousChatKind = 'chat' | 'schedule' | 'bot'
type PreviousChatFilter = PreviousChatKind
type EmptyStateIcon = LucideIcon

type ScheduleActivityItem = {
  id: string
  job: ScheduledJob
  run?: ScheduledJobRun
  kind: 'run' | 'missed'
  occurredAt: string
}

const emptyStateContent: Record<PreviousChatFilter, {
  icon: EmptyStateIcon
  title: string
  body: string
}> = {
  chat: {
    icon: MessageSquare,
    title: 'No chats yet',
    body: 'Start a chat from the composer below. After the first saved turn, it will appear here so you can resume it later.',
  },
  schedule: {
    icon: CalendarClock,
    title: 'No scheduled chats yet',
    body: 'Use the Schedules control in the top bar to create a recurring task. After a run starts, the latest scheduled chat will appear here.',
  },
  bot: {
    icon: Bot,
    title: 'No bot chats yet',
    body: 'Use the Bot connector button in the top bar to connect and configure a bot. Sessions started or resumed from that bot will appear here.',
  },
}

const firstRunHints: Array<{
  icon: EmptyStateIcon
  label: string
  body: string
}> = [
  {
    icon: MessageSquare,
    label: 'Chat',
    body: 'Ask anything, then return here to resume the thread.',
  },
  {
    icon: CalendarClock,
    label: 'Schedules',
    body: 'Run recurring work on a schedule and review the latest run.',
  },
  {
    icon: Bot,
    label: 'Bots',
    body: 'Use the Bot connector button to configure the external bot.',
  },
]

export function chatHistorySessionTitle(session: ChatHistorySession, maxLength = 110): string {
  const query = session.query?.replace(/\s+/g, ' ').trim()
  if (query) return query.length > maxLength ? `${query.slice(0, maxLength)}...` : query
  return `${(session.agent_mode || 'chat').replace(/_/g, ' ')} ${session.session_id.slice(0, 8)}`
}

export function chatHistoryConversationPath(session: ChatHistorySession): string {
  if (session.conversation_path) return session.conversation_path
  const userId = session.user_id || 'default'
  return `_users/${userId}/chat_history/${session.session_id}/conversation.json`
}

export function chatHistoryRuntimeLabel(session: ChatHistorySession): string | undefined {
  const runtime = session.runtime
  const provider = runtime?.provider?.trim()
  if (!runtime || !provider) return undefined

  const model = runtime.model_id?.trim()
  if (model && model !== provider) return `${provider} · ${model}`
  return provider
}

function chatHistoryRuntimeTransport(session: ChatHistorySession): string {
  const runtime = session.runtime
  const transport = runtime?.transport?.trim().toLowerCase()
  if (transport) return transport
  return runtime?.agent_session_handle?.provider?.transport?.trim().toLowerCase() || ''
}

export function chatHistorySupportsNativeResume(session: ChatHistorySession): boolean {
  const runtime = session.runtime
  if (!runtime || runtime.kind !== 'coding_agent') return false
  if (runtime.resume_supported === false) return false
  const handle = runtime.agent_session_handle?.provider
  return Boolean(
    runtime.resume_supported ||
    runtime.external_session_id?.trim() ||
    runtime.project_dir_id?.trim() ||
    handle?.native_session_id?.trim() ||
    handle?.project_dir_id?.trim()
  )
}

export function chatHistoryUsesTerminalRestore(session: ChatHistorySession): boolean {
  const runtime = session.runtime
  if (!runtime || runtime.kind !== 'coding_agent') return false
  return chatHistoryRuntimeTransport(session) === 'tmux'
}

export function chatHistoryWorkshopModeLabel(session: ChatHistorySession): string | undefined {
  const raw = (session.runtime?.workshop_mode || session.workshop_mode || '').trim().toLowerCase()
  if (!raw) return undefined
  if (raw === 'optimizer') return 'Optimizer'
  if (raw === 'builder') return 'Builder'
  if (raw === 'run') return 'Run'
  if (raw === 'reporting') return 'Reporting'
  return raw.replace(/_/g, ' ')
}

const formatChatTime = (value?: string): string => {
  if (!value) return 'Unknown time'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'Unknown time'
  return date.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

const formatMessageCount = (count?: number): string | undefined => {
  if (typeof count !== 'number') return undefined
  const formatted = new Intl.NumberFormat().format(count)
  return `${formatted} ${count === 1 ? 'message' : 'messages'}`
}

const formatDuration = (durationMs?: number): string | undefined => {
  if (typeof durationMs !== 'number' || durationMs < 0) return undefined
  if (durationMs < 60_000) return `${Math.round(durationMs / 1000)}s`
  if (durationMs < 3_600_000) return `${Math.round(durationMs / 60_000)}m`
  const hours = Math.floor(durationMs / 3_600_000)
  const minutes = Math.round((durationMs % 3_600_000) / 60_000)
  return minutes ? `${hours}h ${minutes}m` : `${hours}h`
}

const sameWorkspace = (left?: string, right?: string): boolean => {
  const normalize = (value?: string) => (value || '').trim().replace(/\\/g, '/').replace(/^\/+|\/+$/g, '')
  return Boolean(normalize(left) && normalize(left) === normalize(right))
}

const scheduleStatusPresentation = (item: ScheduleActivityItem): {
  label: string
  className: string
  Icon: LucideIcon
  detail: string
} => {
  if (item.kind === 'missed') {
    return {
      label: 'Missed slot',
      className: 'border-amber-500/35 bg-amber-500/10 text-amber-700 dark:text-amber-300',
      Icon: CircleAlert,
      detail: item.job.missed_run_reason || 'No execution was created for this scheduled time.',
    }
  }

  const run = item.run!
  switch (run.status) {
    case 'success':
      return { label: 'Completed', className: 'border-emerald-500/35 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300', Icon: CheckCircle2, detail: 'Finished successfully.' }
    case 'running':
      return { label: 'Running', className: 'border-sky-500/35 bg-sky-500/10 text-sky-700 dark:text-sky-300', Icon: CircleDashed, detail: 'This occurrence is still running.' }
    case 'waiting_for_capacity':
    case 'waiting_for_workflow':
      return { label: 'Waiting', className: 'border-sky-500/35 bg-sky-500/10 text-sky-700 dark:text-sky-300', Icon: Clock3, detail: run.error || 'Waiting for its schedule policy to permit execution.' }
    case 'partial':
      return { label: 'Partial', className: 'border-amber-500/35 bg-amber-500/10 text-amber-700 dark:text-amber-300', Icon: CircleAlert, detail: run.error || 'The occurrence completed only partially.' }
    case 'interrupted':
    case 'stopped':
      return { label: 'Interrupted', className: 'border-amber-500/35 bg-amber-500/10 text-amber-700 dark:text-amber-300', Icon: CircleAlert, detail: run.error || 'The occurrence stopped before completion.' }
    default:
      return { label: 'Failed run', className: 'border-destructive/45 bg-destructive/10 text-destructive', Icon: XCircle, detail: run.error || 'This recorded execution failed.' }
  }
}

// Schedule history is a record of a conversation, not just a server job. Keep
// the compact card useful without dumping the scheduler's raw error into the
// primary line: show the instruction that started the run and its latest human
// readable agent update. The complete transcript remains available through
// Open, while the raw failure stays in a small disclosure for diagnosis.
const scheduleRunStartMessage = (job: ScheduledJob, session?: ChatHistorySession): string => {
  const configuredMessage = (job.messages || []).find(message => message.trim()) || job.query || ''
  return (session?.query || configuredMessage || 'This scheduled run started without a saved instruction.').trim()
}

const scheduleRunLatestAgentMessage = (session?: ChatHistorySession): string | undefined => {
  return [...(session?.preview_messages || [])]
    .reverse()
    .find(message => ['assistant', 'ai'].includes(message.role.trim().toLowerCase()) && message.text.trim())
    ?.text
    .trim()
}

const scheduleRunExcerpt = (text: string, maxLength = 240): string => {
  const normalized = text.replace(/\s+/g, ' ').trim()
  return normalized.length > maxLength ? `${normalized.slice(0, maxLength).trimEnd()}…` : normalized
}

const sessionHasMessages = (session: ChatHistorySession): boolean => {
  return (session.message_count ?? 0) > 0 || (session.preview_messages?.length ?? 0) > 0 || !!session.query?.trim()
}

const isSessionOlderThanDays = (session: ChatHistorySession, days: number): boolean => {
  const timestamp = Date.parse(session.updated_at || session.created_at || '')
  if (Number.isNaN(timestamp)) return false
  return timestamp < Date.now() - days * 24 * 60 * 60 * 1000
}

const getChatKind = (session: ChatHistorySession): PreviousChatKind => {
  if (isScheduledChatHistorySession(session)) return 'schedule'
  if (session.session_id.startsWith('bot-')) return 'bot'
  return 'chat'
}

const previewMessages = (session: ChatHistorySession): ChatHistoryPreviewMessage[] => {
  return (session.preview_messages || [])
    .filter(message => message.text?.trim())
    .slice(-PREVIEW_MESSAGE_LIMIT)
}

const messageRole = (message: ChatHistoryMessage): string => {
  return String(message.Role || message.role || '').toLowerCase().trim()
}

const cleanMessageText = (text: string): string => {
  const markers = [
    '\n\nPrevious workflow-builder conversation file:',
    '\n\nPrevious builder chat file available:',
    '\n\nPrevious conversation file:',
  ]
  for (const marker of markers) {
    const markerIndex = text.indexOf(marker)
    if (markerIndex >= 0) return text.slice(0, markerIndex).trim()
  }
  return text.trim()
}

const messageText = (message: ChatHistoryMessage): string => {
  const parts = message.Parts || message.parts || []
  const text = parts
    .map(part => part.Text || part.text || part.Content || part.content || '')
    .filter(Boolean)
    .join('\n')
    .trim()
  return cleanMessageText(text)
}

const functionCallOf = (part: unknown): Record<string, unknown> | null => {
  const call = (part as Record<string, unknown>)?.FunctionCall ?? (part as Record<string, unknown>)?.functionCall
  return call && typeof call === 'object' ? call as Record<string, unknown> : null
}

const messageToolName = (message: ChatHistoryMessage): string => {
  const parts = message.Parts || message.parts || []
  for (const part of parts) {
    const call = functionCallOf(part)
    const name = call?.Name ?? call?.name
      ?? (part as Record<string, unknown>).Name ?? (part as Record<string, unknown>).name
    if (typeof name === 'string' && name.trim()) return name.trim()
  }
  return ''
}

// The tool CALL lives as a FunctionCall part on an assistant message and holds
// no text, so messageText() came back empty and the call was filtered out
// entirely -- only its result survived. Surface the arguments so a reader can
// see what was actually run, not just what came back.
const messageToolArgs = (message: ChatHistoryMessage): string => {
  const parts = message.Parts || message.parts || []
  for (const part of parts) {
    const call = functionCallOf(part)
    const args = call?.Arguments ?? call?.arguments
    if (typeof args === 'string' && args.trim()) {
      try {
        return JSON.stringify(JSON.parse(args), null, 2)
      } catch {
        return args.trim()
      }
    }
  }
  return ''
}

const messageIsError = (message: ChatHistoryMessage): boolean => {
  const parts = message.Parts || message.parts || []
  return parts.some(part => Boolean((part as Record<string, unknown>).IsError ?? (part as Record<string, unknown>).is_error))
}

const shouldSkipMessageText = (text: string): boolean => {
  return text.startsWith('[AUTO-NOTIFICATION]') ||
    text.startsWith('[Previous tool call') ||
    text.startsWith('[Previous tool result')
}

const conversationMessages = (conversation: ChatHistoryConversation): ChatHistoryPreviewMessage[] => {
  return (conversation.conversation_history || [])
    .flatMap((message): ChatHistoryPreviewMessage[] => {
      const role = messageRole(message)
      const text = messageText(message)
      const toolName = messageToolName(message)
      const rows: ChatHistoryPreviewMessage[] = []
      if (text) rows.push({ role, text, toolName, isError: messageIsError(message) })
      // An assistant turn often has prose AND a tool call; emit the call as its
      // own row so neither hides the other.
      if (role === 'ai' || role === 'assistant') {
        const args = messageToolArgs(message)
        if (args) rows.push({ role: 'tool_call', text: args, toolName })
      }
      return rows
    })
    .filter(message => {
      if (!message.text) return false
      // Prose heuristics (auto-notification banners, replayed tool summaries)
      // must not be applied to raw tool arguments/results.
      if (message.role !== 'tool' && message.role !== 'tool_call' && shouldSkipMessageText(message.text)) return false
      return message.role === 'human' ||
        message.role === 'user' ||
        message.role === 'ai' ||
        message.role === 'assistant' ||
        // Tool calls were filtered out entirely, so a chat that spent most of
        // its turns in tools -- and any tool that failed -- read as if nothing
        // happened between one message and the next.
        message.role === 'tool' ||
        message.role === 'tool_call'
    })
}

// No local slice: the server already returns exactly the requested window, and
// trimming again here would make "Load more" fetch messages it then discarded.
const recentConversationMessages = (messages: ChatHistoryPreviewMessage[]): ChatHistoryPreviewMessage[] => {
  return messages.filter(message => message.text?.trim())
}

const mergeSessions = (current: ChatHistorySession[], next: ChatHistorySession[]): ChatHistorySession[] => {
  const byId = new Map<string, ChatHistorySession>()
  for (const session of [...current, ...next]) {
    byId.set(session.session_id, session)
  }
  return Array.from(byId.values()).sort((a, b) =>
    Date.parse(b.updated_at || b.created_at || '') - Date.parse(a.updated_at || a.created_at || '')
  )
}

const PreviousChatEmptyState: React.FC<{
  filter: PreviousChatFilter
  hasAnySessions: boolean
  fallbackText: string
}> = ({ filter, hasAnySessions, fallbackText }) => {
  const content = hasAnySessions
    ? emptyStateContent[filter]
    : { ...emptyStateContent[filter], body: emptyStateContent[filter].body || fallbackText }
  const Icon = content.icon

  return (
    <div className="px-3 py-4">
      <div className="rounded-md border border-dashed border-border bg-muted/15 px-3 py-4">
        <div className="flex items-start gap-3">
          <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md border border-border bg-background text-muted-foreground">
            <Icon className="h-4 w-4" />
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium text-foreground">{content.title}</div>
            <p className="mt-1 max-w-xl text-xs leading-5 text-muted-foreground">{content.body || fallbackText}</p>

            {!hasAnySessions && (
              <div className="mt-3 grid gap-x-4 gap-y-2 border-t border-border/70 pt-3 sm:grid-cols-3">
                {firstRunHints.map(({ icon: HintIcon, label, body }) => (
                  <div key={label} className="min-w-0">
                    <div className="flex items-center gap-1.5 text-xs font-medium text-foreground">
                      <HintIcon className="h-3.5 w-3.5 text-muted-foreground" />
                      <span>{label}</span>
                    </div>
                    <p className="mt-1 text-[11px] leading-4 text-muted-foreground">{body}</p>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

interface PreviousChatHistoryPanelProps {
  workspacePath?: string
  activeSessionId?: string
  title?: string
  emptyText?: string
  actionLabel?: string
  onHasChatsChange?: (hasChats: boolean, isLoaded?: boolean) => void
  onSelectSession: (session: ChatHistorySession) => void | Promise<void>
  /** Dense layout for the narrow ~360px chat rail: icon-only filters + actions,
   *  single tight meta line, no runtime/workshop chips or message preview. */
  compact?: boolean
  /** Fill the available chat surface for landing dashboards. */
  fill?: boolean
}

export const PreviousChatHistoryPanel: React.FC<PreviousChatHistoryPanelProps> = ({
  workspacePath,
  activeSessionId,
  title = 'Previous chats',
  emptyText = 'No previous chats yet.',
  actionLabel = 'Open',
  onHasChatsChange,
  onSelectSession,
  compact = false,
  fill = false,
}) => {
  const [sessions, setSessions] = useState<ChatHistorySession[]>([])
  const [isLoading, setIsLoading] = useState(true)
  const [isCleanupLoading, setIsCleanupLoading] = useState(false)
  const [deletingSessionIds, setDeletingSessionIds] = useState<Set<string>>(() => new Set())
  const [activeFilter, setActiveFilter] = useState<PreviousChatFilter>('chat')
  const [visibleCount, setVisibleCount] = useState(PAGE_SIZE)
  const [expandedSessionIds, setExpandedSessionIds] = useState<Set<string>>(() => new Set())
  const [expandedMessagesBySession, setExpandedMessagesBySession] = useState<Record<string, ChatHistoryPreviewMessage[]>>({})
  const [loadingExpandedSessionIds, setLoadingExpandedSessionIds] = useState<Set<string>>(() => new Set())
  // How many stored messages each expanded session has pulled so far, and
  // whether the server still had older ones to give.
  const [expandedWindowBySession, setExpandedWindowBySession] = useState<Record<string, number>>({})
  const [hasMoreBySession, setHasMoreBySession] = useState<Record<string, boolean>>({})
  const [scheduleJobs, setScheduleJobs] = useState<ScheduledJob[]>([])
  const [scheduleRunsByJob, setScheduleRunsByJob] = useState<Record<string, ScheduledJobRun[]>>({})
  const [scheduleRunTotalsByJob, setScheduleRunTotalsByJob] = useState<Record<string, number>>({})
  const [isLoadingScheduleActivity, setIsLoadingScheduleActivity] = useState(false)
  const [scheduleJobsWorkspacePath, setScheduleJobsWorkspacePath] = useState('')
  const [expandedScheduleIDs, setExpandedScheduleIDs] = useState<Set<string>>(() => new Set())
  const [expandedScheduleMessageIDs, setExpandedScheduleMessageIDs] = useState<Set<string>>(() => new Set())
  const [loadingScheduleHistoryIDs, setLoadingScheduleHistoryIDs] = useState<Set<string>>(() => new Set())
  const expandedMessagesRef = useRef(expandedMessagesBySession)
  const loadingExpandedSessionIdsRef = useRef(loadingExpandedSessionIds)
  const addToast = useChatStore(state => state.addToast)

  useEffect(() => {
    expandedMessagesRef.current = expandedMessagesBySession
  }, [expandedMessagesBySession])

  useEffect(() => {
    loadingExpandedSessionIdsRef.current = loadingExpandedSessionIds
  }, [loadingExpandedSessionIds])

  useEffect(() => {
    let cancelled = false
    setSessions([])
    setActiveFilter('chat')
    setVisibleCount(PAGE_SIZE)
    setExpandedSessionIds(new Set())
    setExpandedMessagesBySession({})
    setLoadingExpandedSessionIds(new Set())
    setIsLoading(true)

    agentApi.listChatHistorySessions(FETCH_LIMIT, 0, workspacePath)
      .then(response => {
        if (cancelled) return
        setSessions(mergeSessions([], response.sessions || []))
      })
      .catch(() => {
        if (cancelled) return
        setSessions([])
        addToast('Failed to load previous chats', 'error')
      })
      .finally(() => {
        if (!cancelled) setIsLoading(false)
      })

    return () => { cancelled = true }
  }, [addToast, workspacePath])

  // Load the parent schedules as soon as the workflow changes. The Schedule
  // badge must never claim zero just because its tab has not been opened yet.
  // Individual run histories remain lazy and are loaded only when expanded.
  useEffect(() => {
    if (!workspacePath) {
      setScheduleJobs([])
      setScheduleJobsWorkspacePath('')
      return
    }
    let cancelled = false
    setIsLoadingScheduleActivity(true)

    void schedulerApi.listJobs({ entity_type: 'workflow', limit: 100 })
      .then(async response => {
        const jobs = (response.jobs || []).filter(job => sameWorkspace(job.workspace_path, workspacePath))
        if (cancelled) return
        setScheduleJobs(jobs)
        setScheduleRunsByJob({})
        setScheduleRunTotalsByJob({})
        setExpandedScheduleIDs(new Set())
        setExpandedScheduleMessageIDs(new Set())
        setLoadingScheduleHistoryIDs(new Set())
        setScheduleJobsWorkspacePath(workspacePath)
      })
      .catch(() => {
        if (cancelled) return
        setScheduleJobs([])
        setScheduleRunsByJob({})
        setScheduleRunTotalsByJob({})
        setScheduleJobsWorkspacePath(workspacePath)
        addToast('Failed to load schedule activity', 'error')
      })
      .finally(() => {
        if (!cancelled) setIsLoadingScheduleActivity(false)
      })

    return () => { cancelled = true }
  }, [addToast, workspacePath])

  const visibleSessions = useMemo(
    () => sessions.filter(session => session.session_id !== activeSessionId),
    [activeSessionId, sessions]
  )

  const filterCounts = useMemo(() => {
    const counts: Record<PreviousChatFilter, number> = {
      chat: 0,
      schedule: 0,
      bot: 0,
    }
    for (const session of visibleSessions) {
      counts[getChatKind(session)] += 1
    }
    return counts
  }, [visibleSessions])

  const filteredSessions = useMemo(
    () => visibleSessions.filter(session => getChatKind(session) === activeFilter),
    [activeFilter, visibleSessions]
  )

  const oldVisibleSessionCounts = useMemo(
    () => CHAT_HISTORY_CLEANUP_AGE_OPTIONS.reduce((counts, days) => {
      counts[days] = visibleSessions.filter(session => sessionHasMessages(session) && isSessionOlderThanDays(session, days)).length
      return counts
    }, {} as Record<ChatHistoryCleanupAgeDays, number>),
    [visibleSessions]
  )
  const hasOldVisibleSessions = useMemo(
    () => CHAT_HISTORY_CLEANUP_AGE_OPTIONS.some(days => oldVisibleSessionCounts[days] > 0),
    [oldVisibleSessionCounts]
  )

  const displayedSessions = useMemo(
    () => filteredSessions.slice(0, visibleCount),
    [filteredSessions, visibleCount]
  )

  const scheduleDataMatchesWorkspace = sameWorkspace(scheduleJobsWorkspacePath, workspacePath)

  const displayFilterCounts = useMemo(() => ({
    ...filterCounts,
    // The Schedule tab is organized by schedule, not by its child runs.
    // Before its durable schedule list arrives, show a loading mark rather
    // than the unrelated legacy chat-session count (often a false zero).
    schedule: scheduleDataMatchesWorkspace ? scheduleJobs.length : '…',
  }), [filterCounts, scheduleDataMatchesWorkspace, scheduleJobs.length])

  const sessionsByID = useMemo(
    () => new Map(visibleSessions.map(session => [session.session_id, session])),
    [visibleSessions]
  )

  useEffect(() => {
    setVisibleCount(PAGE_SIZE)
  }, [activeFilter])

  useEffect(() => {
    onHasChatsChange?.(!isLoading && visibleSessions.length > 0, !isLoading)
  }, [isLoading, onHasChatsChange, visibleSessions.length])

  const loadExpandedMessages = useCallback(async (session: ChatHistorySession, requestedWindow?: number) => {
    const sessionId = session.session_id
    const window = requestedWindow ?? EXPANDED_FETCH_LIMIT
    const isLoadMore = requestedWindow !== undefined
    // A plain expand is a no-op once loaded; "Load more" deliberately re-fetches
    // the same session with a larger window.
    if (!isLoadMore && expandedMessagesRef.current[sessionId]) return
    if (loadingExpandedSessionIdsRef.current.has(sessionId)) return

    const nextLoading = new Set(loadingExpandedSessionIdsRef.current)
    nextLoading.add(sessionId)
    loadingExpandedSessionIdsRef.current = nextLoading
    setLoadingExpandedSessionIds(nextLoading)
    try {
      const conversation = await agentApi.getChatHistoryConversation(sessionId, workspacePath, window)
      const messages = conversationMessages(conversation)
      const recentMessages = recentConversationMessages(messages)
      // The server returns the tail of the conversation capped at `window`.
      // Getting back fewer than asked for means we reached the beginning.
      const returned = conversation.conversation_history?.length ?? 0
      setExpandedWindowBySession(current => ({ ...current, [sessionId]: window }))
      setHasMoreBySession(current => ({ ...current, [sessionId]: returned >= window }))
      setExpandedMessagesBySession(current => {
        const next = {
          ...current,
          [sessionId]: recentMessages.length > 0 ? recentMessages : previewMessages(session),
        }
        expandedMessagesRef.current = next
        return next
      })
    } catch {
      setExpandedMessagesBySession(current => {
        const next = {
          ...current,
          [sessionId]: previewMessages(session),
        }
        expandedMessagesRef.current = next
        return next
      })
      addToast('Failed to load full chat details', 'error')
    } finally {
      setLoadingExpandedSessionIds(current => {
        const next = new Set(current)
        next.delete(sessionId)
        loadingExpandedSessionIdsRef.current = next
        return next
      })
    }
  }, [addToast, workspacePath])

  const toggleExpanded = useCallback((session: ChatHistorySession) => {
    const sessionId = session.session_id
    const wasExpanded = expandedSessionIds.has(sessionId)
    setExpandedSessionIds(current => {
      const next = new Set(current)
      if (next.has(sessionId)) {
        next.delete(sessionId)
      } else {
        next.add(sessionId)
      }
      return next
    })
    if (!wasExpanded) {
      void loadExpandedMessages(session)
    }
  }, [expandedSessionIds, loadExpandedMessages])

  const handleSelect = useCallback((session: ChatHistorySession) => {
    void onSelectSession(session)
  }, [onSelectSession])

  const handleDeleteSession = useCallback(async (session: ChatHistorySession) => {
    const title = chatHistorySessionTitle(session, 80)
    if (!window.confirm(`Delete this chat?\n\n${title}`)) return

    const sessionId = session.session_id
    setDeletingSessionIds(current => {
      const next = new Set(current)
      next.add(sessionId)
      return next
    })
    try {
      await agentApi.deleteChatHistorySession(sessionId, workspacePath)
      setSessions(current => current.filter(item => item.session_id !== sessionId))
      setExpandedSessionIds(current => {
        const next = new Set(current)
        next.delete(sessionId)
        return next
      })
      setExpandedMessagesBySession(current => {
        const next = { ...current }
        delete next[sessionId]
        expandedMessagesRef.current = next
        return next
      })
      addToast('Deleted previous chat', 'success')
    } catch {
      addToast('Failed to delete previous chat', 'error')
    } finally {
      setDeletingSessionIds(current => {
        const next = new Set(current)
        next.delete(sessionId)
        return next
      })
    }
  }, [addToast, workspacePath])

  const openScheduleActivity = useCallback((item: ScheduleActivityItem) => {
    const sessionID = item.run?.session_id
    const session = sessionID ? sessionsByID.get(sessionID) : undefined
    if (!session) {
      addToast('This run has no restorable conversation record', 'info')
      return
    }
    void onSelectSession(session)
  }, [addToast, onSelectSession, sessionsByID])

  const runScheduleNow = useCallback(async (job: ScheduledJob) => {
    if (!window.confirm(`Run “${job.name}” now? This creates a new scheduled occurrence and follows its normal side-effect and collision rules.`)) return
    try {
      await schedulerApi.triggerJob(job.id)
      addToast(`Started ${job.name}`, 'success')
      const response = await schedulerApi.getJobRuns(job.id, 200)
      setScheduleRunsByJob(current => ({ ...current, [job.id]: response.runs || [] }))
      setScheduleRunTotalsByJob(current => ({ ...current, [job.id]: response.total || 0 }))
    } catch {
      addToast(`Could not start ${job.name}`, 'error')
    }
  }, [addToast])

  const toggleScheduleHistory = useCallback(async (job: ScheduledJob) => {
    const alreadyOpen = expandedScheduleIDs.has(job.id)
    if (alreadyOpen) {
      setExpandedScheduleIDs(current => {
        const next = new Set(current)
        next.delete(job.id)
        return next
      })
      return
    }

    setExpandedScheduleIDs(current => new Set(current).add(job.id))
    if (scheduleRunsByJob[job.id]) return
    setLoadingScheduleHistoryIDs(current => new Set(current).add(job.id))
    try {
      // schedule-runs.json retains at most 200 entries. Fetching that one
      // schedule's complete retained history lets the user inspect every
      // execution without flattening every schedule into one noisy feed.
      const response = await schedulerApi.getJobRuns(job.id, 200)
      setScheduleRunsByJob(current => ({ ...current, [job.id]: response.runs || [] }))
      setScheduleRunTotalsByJob(current => ({ ...current, [job.id]: response.total || 0 }))
    } catch {
      setExpandedScheduleIDs(current => {
        const next = new Set(current)
        next.delete(job.id)
        return next
      })
      addToast(`Could not load execution history for ${job.name}`, 'error')
    } finally {
      setLoadingScheduleHistoryIDs(current => {
        const next = new Set(current)
        next.delete(job.id)
        return next
      })
    }
  }, [addToast, expandedScheduleIDs, scheduleRunsByJob])

  const handleCleanupOldChats = useCallback(async (olderThanDays: ChatHistoryCleanupAgeDays) => {
    const scopeLabel = workspacePath || 'all chats'
    const oldSessionCount = oldVisibleSessionCounts[olderThanDays] || 0
    if (oldSessionCount === 0) {
      addToast(`No chats older than ${olderThanDays} days`, 'info')
      return
    }
    if (!window.confirm(`Delete ${oldSessionCount} chat${oldSessionCount === 1 ? '' : 's'} older than ${olderThanDays} days from ${scopeLabel}? This cannot be undone.`)) return

    setIsCleanupLoading(true)
    try {
      const response = await agentApi.cleanupChatHistorySessions(olderThanDays, workspacePath)
      const deletedCount = response.result?.deleted_count ?? 0
      const refreshed = await agentApi.listChatHistorySessions(FETCH_LIMIT, 0, workspacePath)
      setSessions(mergeSessions([], refreshed.sessions || []))
      setExpandedSessionIds(new Set())
      setExpandedMessagesBySession({})
      expandedMessagesRef.current = {}
      addToast(
        deletedCount === 0
          ? `No chats older than ${olderThanDays} days`
          : `Deleted ${deletedCount} chat${deletedCount === 1 ? '' : 's'} older than ${olderThanDays} days`,
        'success'
      )
    } catch {
      addToast('Failed to delete old chats', 'error')
    } finally {
      setIsCleanupLoading(false)
    }
  }, [addToast, oldVisibleSessionCounts, workspacePath])

  const ActionIcon = actionLabel.toLowerCase() === 'attach' ? Paperclip : ArrowUpRight
  // This row is a content filter inside the conversation hub, not another
  // set of top-level chat tabs. "Recent" makes that distinction explicit:
  // the workspace tab owns the active/new conversation, while this list is
  // where someone returns to a saved one.
  const filterItems = [
    { filter: 'chat' as const, label: 'Recent', icon: MessageSquare },
    { filter: 'schedule' as const, label: 'Schedules', icon: CalendarClock },
    { filter: 'bot' as const, label: 'Bots', icon: Bot },
  ]

  return (
    <div className={`${fill ? 'flex min-h-0 flex-1 flex-col overflow-hidden' : 'shrink-0'} border-b border-border bg-background`}>
      <div className={`${fill ? 'flex min-h-0 flex-1 flex-col' : ''} w-full`}>
        <div className={`flex flex-wrap items-center gap-2 border-b border-border px-3 py-2 ${compact ? 'justify-end' : title ? 'justify-between' : 'justify-start'}`}>
          {/* The "Previous … chats" heading is redundant in the compact rail —
              the filter pills + list make the purpose obvious — so hide it there. */}
          {!compact && title && (
            <div className="flex min-w-0 items-center gap-2 text-sm">
              <MessageSquare className="h-4 w-4 shrink-0 text-muted-foreground/80" />
              <span className="truncate font-medium text-foreground">{activeFilter === 'schedule' ? 'Schedule activity' : title}</span>
            </div>
          )}

          {!isLoading && (
            <div className="flex max-w-full flex-wrap items-center justify-end gap-2">
              <div className="flex max-w-full items-center gap-0.5 overflow-x-auto rounded-md border border-border bg-muted/30 p-0.5">
                {filterItems.map(({ filter, label, icon: Icon }) => {
                const isActive = activeFilter === filter
                return (
                  <button
                    key={filter}
                    type="button"
                    onClick={() => setActiveFilter(filter)}
                    className={`inline-flex shrink-0 items-center gap-1.5 rounded px-2 py-1 text-xs font-medium transition-colors ${
                      isActive
                        ? 'bg-background text-foreground shadow-sm ring-1 ring-border/40'
                        : 'text-muted-foreground hover:bg-background/60 hover:text-foreground'
                    }`}
                  >
                    <Icon className="h-3.5 w-3.5" />
                    {!compact && <span>{label}</span>}
                    <span className={`min-w-4 rounded-full px-1 py-0.5 text-center text-[10px] leading-none ${
                      isActive
                        ? 'bg-muted text-foreground'
                        : 'bg-background/60 text-muted-foreground'
                    }`}>
                      {displayFilterCounts[filter]}
                    </span>
                  </button>
                )
                })}
              </div>
              {activeFilter !== 'schedule' && hasOldVisibleSessions && (
                <CleanupOldChatsDropdown
                  counts={oldVisibleSessionCounts}
                  isLoading={isCleanupLoading || isLoading}
                  onSelect={handleCleanupOldChats}
                />
              )}
            </div>
          )}
        </div>

        {isLoading ? (
          <div className="px-3 py-3 text-xs text-muted-foreground">Loading previous chats...</div>
        ) : activeFilter === 'schedule' ? (
          <div className={`${fill ? 'min-h-0 flex-1 overflow-y-auto' : ''}`}>
            {isLoadingScheduleActivity ? (
              <div className="flex items-center gap-2 px-3 py-4 text-xs text-muted-foreground">
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                <span>Loading schedule activity...</span>
              </div>
            ) : scheduleJobs.length === 0 ? (
              <div className="px-3 py-4 text-sm text-muted-foreground">No schedules are configured for this workflow yet.</div>
            ) : (
              <div className="divide-y divide-border">
                {scheduleJobs.map(job => {
                  const historyOpen = expandedScheduleIDs.has(job.id)
                  const historyLoading = loadingScheduleHistoryIDs.has(job.id)
                  const messagesOpen = expandedScheduleMessageIDs.has(job.id)
                  // `pulse_review_only` is the canonical API flag. Match the
                  // established schedule name too so a frontend hot reload can
                  // render existing Pulse schedules before the backend restart
                  // that begins returning that new field.
                  const isPulseSchedule = job.pulse_review_only || /^periodic\s+pulse\s+review$/i.test(job.name.trim())
                  const configuredMessages = (job.messages || []).map(message => message.trim()).filter(Boolean)
                  // Pulse-only schedules deliberately persist no generic
                  // messages: the scheduler composes a run-specific context
                  // from current evidence at trigger time. Surface that real
                  // contract rather than incorrectly showing an empty state.
                  const scheduleMessages = configuredMessages.length > 0
                    ? configuredMessages
                    : isPulseSchedule
                      ? ['**Generated Pulse review instructions**\n\nAt the scheduled time, Pulse creates a run-specific review context from the workflow\'s retained evidence, then continues the same conversation through **Gate → Review & Fix → Finalize**. The exact prompt includes the current Pulse run ID, available evidence, and due review modules, so it is not stored as a static schedule message.']
                      : []
                  const runs = scheduleRunsByJob[job.id] || []
                  const recordedRunCount = scheduleRunTotalsByJob[job.id] ?? job.run_count ?? 0
                  const missedCount = job.missed_run_count || 0

                  return (
                    <div key={job.id} className="group px-3 py-3 transition-colors hover:bg-muted/20">
                      <div className="flex items-start gap-2">
                        <div className="min-w-0 flex-1">
                          <div className="flex min-w-0 items-center gap-1.5 text-sm font-medium text-foreground">
                            {isPulseSchedule && <Activity className="h-4 w-4 shrink-0 text-cyan-500" aria-label="Pulse review" />}
                            <span className="truncate">{job.name}</span>
                          </div>
                          <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted-foreground">
                            <span>{job.enabled ? 'Enabled' : 'Disabled'}</span>
                            <span>{recordedRunCount} recorded execution{recordedRunCount === 1 ? '' : 's'}</span>
                            {job.next_run_at && <span>next {formatChatTime(job.next_run_at)}</span>}
                          </div>
                          {missedCount > 0 && (
                            <div className="mt-2 flex items-start gap-1.5 text-xs text-amber-700 dark:text-amber-300">
                              <CircleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                              <span>
                                {missedCount} scheduled slot{missedCount === 1 ? '' : 's'} missed{job.latest_missed_run_at ? `; latest due ${formatChatTime(job.latest_missed_run_at)}` : ''}. No execution record was created.
                              </span>
                            </div>
                          )}
                        </div>
                        <div className="flex shrink-0 items-center gap-1">
                          <button
                            type="button"
                            onClick={() => void runScheduleNow(job)}
                            className="inline-flex items-center gap-1 rounded border border-border bg-background px-2 py-1 text-xs font-medium text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground"
                          >
                            <Play className="h-3.5 w-3.5" />
                            {!compact && <span>Run now</span>}
                          </button>
                        </div>
                      </div>
                      {scheduleMessages.length > 0 && (
                        <div className="mt-2">
                          <button
                            type="button"
                            onClick={() => setExpandedScheduleMessageIDs(current => {
                              const next = new Set(current)
                              if (next.has(job.id)) next.delete(job.id)
                              else next.add(job.id)
                              return next
                            })}
                            className="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
                          >
                            {messagesOpen ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                            <span>{messagesOpen ? 'Hide scheduled instructions' : isPulseSchedule ? 'Show generated Pulse instructions' : `Show scheduled instruction${scheduleMessages.length === 1 ? '' : 's'}`}</span>
                          </button>
                          {messagesOpen && (
                            <div className="mt-2 space-y-2 rounded border border-border bg-muted/10 px-3 py-2.5">
                              {scheduleMessages.map((message, index) => (
                                <div key={`${job.id}-message-${index}`} className="text-xs leading-5 text-muted-foreground">
                                  {scheduleMessages.length > 1 && <div className="mb-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">Message {index + 1}</div>}
                                  <ConversationMarkdownRenderer content={message} maxHeight="none" framed={false} />
                                </div>
                              ))}
                            </div>
                          )}
                        </div>
                      )}
                      <div className="mt-2">
                        <button
                          type="button"
                          onClick={() => void toggleScheduleHistory(job)}
                          className="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
                        >
                          {historyLoading ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : historyOpen ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
                          <span>{historyOpen ? 'Hide execution history' : `Show all ${recordedRunCount} recorded execution${recordedRunCount === 1 ? '' : 's'}`}</span>
                        </button>
                      </div>
                      {historyOpen && !historyLoading && (
                        <div className="mt-2 divide-y divide-border/70 rounded border border-border bg-muted/10">
                          {runs.length === 0 ? (
                            <div className="px-3 py-2 text-xs text-muted-foreground">No execution record exists for this schedule yet.</div>
                          ) : runs.map(run => {
                            const item: ScheduleActivityItem = { id: run.id, job, run, kind: 'run', occurredAt: run.started_at }
                            const presentation = scheduleStatusPresentation(item)
                            const Icon = presentation.Icon
                            const session = run.session_id ? sessionsByID.get(run.session_id) : undefined
                            const canResume = run.status === 'interrupted' && !!session
                            const duration = formatDuration(run.duration_ms)
                            const isDeleting = !!session && deletingSessionIds.has(session.session_id)
                            const startedWith = scheduleRunStartMessage(job, session)
                            const latestAgentUpdate = scheduleRunLatestAgentMessage(session)
                            const outcome = latestAgentUpdate || presentation.detail
                            return (
                              <div key={run.id} className="space-y-2.5 px-3 py-3">
                                <div className="flex items-start gap-2">
                                  <div className={`mt-0.5 inline-flex shrink-0 items-center gap-1 rounded border px-1.5 py-0.5 text-[10px] font-semibold ${presentation.className}`}>
                                    <Icon className={`h-3 w-3 ${run.status === 'running' ? 'animate-spin' : ''}`} />
                                    <span>{presentation.label}</span>
                                  </div>
                                  <div className="min-w-0 flex-1">
                                    <div className="text-sm font-medium text-foreground">
                                      {run.status === 'success'
                                        ? duration ? `Completed in ${duration}` : 'Completed'
                                        : run.status === 'running'
                                          ? 'Run in progress'
                                          : duration ? `Stopped after ${duration}` : 'Run stopped'}
                                    </div>
                                    <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-muted-foreground">
                                      <span>Started {formatChatTime(run.started_at)}</span>
                                      {run.completed_at && <span>ended {formatChatTime(run.completed_at)}</span>}
                                      {run.group_names?.length ? <span>{run.group_names.join(', ')}</span> : null}
                                    </div>
                                  </div>
                                  <div className="flex shrink-0 items-center gap-1">
                                    {session && (
                                      <button
                                        type="button"
                                        onClick={() => openScheduleActivity(item)}
                                        className="inline-flex items-center gap-1 rounded border border-border bg-background px-2 py-1 text-xs font-medium text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground"
                                      >
                                        {canResume ? <RotateCcw className="h-3.5 w-3.5" /> : <ArrowUpRight className="h-3.5 w-3.5" />}
                                        {!compact && <span>{canResume ? 'Resume' : 'Open'}</span>}
                                      </button>
                                    )}
                                    {session && (
                                      <button
                                        type="button"
                                        onClick={() => { void handleDeleteSession(session) }}
                                        disabled={isDeleting}
                                        className="inline-flex items-center rounded border border-border bg-background p-1 text-destructive/75 transition-colors hover:bg-destructive/10 hover:text-destructive disabled:opacity-50"
                                        aria-label="Delete conversation record"
                                        title="Delete conversation record; the schedule execution remains"
                                      >
                                        {isDeleting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Trash2 className="h-3.5 w-3.5" />}
                                      </button>
                                    )}
                                  </div>
                                </div>

                                <div className="space-y-2 border-l-2 border-border/80 pl-3">
                                  <div>
                                    <div className="mb-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">Started with</div>
                                    <p className="line-clamp-2 text-xs leading-5 text-muted-foreground">{scheduleRunExcerpt(startedWith)}</p>
                                  </div>
                                  <div>
                                    <div className="mb-0.5 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">{latestAgentUpdate ? 'Latest agent update' : 'Outcome'}</div>
                                    <p className="line-clamp-3 text-xs leading-5 text-foreground/90">{scheduleRunExcerpt(outcome)}</p>
                                  </div>
                                </div>

                                {run.error && (
                                  <details className="text-[11px] text-muted-foreground">
                                    <summary className="cursor-pointer select-none font-medium hover:text-foreground">Technical details</summary>
                                    <pre className="mt-1 max-h-32 overflow-auto whitespace-pre-wrap break-words rounded border border-destructive/20 bg-destructive/5 px-2 py-1.5 font-mono text-[10px] leading-4 text-muted-foreground">{run.error}</pre>
                                  </details>
                                )}
                              </div>
                            )
                          })}
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        ) : visibleSessions.length === 0 ? (
          <PreviousChatEmptyState
            filter={activeFilter}
            hasAnySessions={false}
            fallbackText={emptyText}
          />
        ) : filteredSessions.length === 0 ? (
          <PreviousChatEmptyState
            filter={activeFilter}
            hasAnySessions
            fallbackText={`No previous ${activeFilter} chats yet.`}
          />
        ) : (
          <div className={`${fill ? 'min-h-0 flex-1 overflow-y-auto' : ''} divide-y divide-border`}>
            {displayedSessions.map(session => {
              const messages = expandedMessagesBySession[session.session_id] || previewMessages(session)
              const isExpanded = expandedSessionIds.has(session.session_id)
              const isLoadingDetails = loadingExpandedSessionIds.has(session.session_id)
              // Distinguishes a first expand (blank, show a spinner) from a
              // "Load more" (keep the list on screen while the next page loads).
              const hasLoadedMessages = Boolean(expandedMessagesBySession[session.session_id]?.length)
              const runtimeLabel = chatHistoryRuntimeLabel(session)
              const isDeleting = deletingSessionIds.has(session.session_id)
              const timeLabel = formatChatTime(session.updated_at || session.created_at)
              const messageCountLabel = formatMessageCount(session.message_count)

              return (
                <div key={session.session_id} className="group bg-background transition-colors hover:bg-muted/20">
                  <div className={`flex items-start gap-2 ${compact ? 'px-2.5 py-2' : 'px-3 py-2.5'}`}>
                    <button
                      type="button"
                      onClick={() => toggleExpanded(session)}
                      disabled={messages.length === 0 && !session.message_count}
                      className="mt-0.5 rounded p-1 text-muted-foreground transition-colors hover:bg-background hover:text-foreground disabled:cursor-default disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-muted-foreground"
                      aria-label={isExpanded ? 'Hide chat details' : 'Show chat details'}
                    >
                      {isExpanded ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
                    </button>

                    <button
                      type="button"
                      onClick={() => handleSelect(session)}
                      className="min-w-0 flex-1 text-left"
                    >
                      <div className="line-clamp-1 text-sm font-medium text-foreground">{chatHistorySessionTitle(session)}</div>
                      <div className={`mt-1 flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground ${compact ? 'text-[10px]' : 'text-[11px]'}`}>
                        <span className="inline-flex min-w-0 items-center gap-1">
                          <CalendarClock className="h-3 w-3 shrink-0" />
                          <span className="truncate">{timeLabel}</span>
                        </span>
                        {messageCountLabel && (
                          <span className="inline-flex items-center gap-1">
                            <MessageSquare className="h-3 w-3 shrink-0" />
                            <span>{messageCountLabel}</span>
                          </span>
                        )}
                        {runtimeLabel && (
                          <span className="inline-flex min-w-0 max-w-full items-center gap-1 rounded border border-border/70 bg-muted/30 px-1.5 py-0.5">
                            <Code2 className="h-3 w-3 shrink-0" />
                            <span className="truncate">{runtimeLabel}</span>
                          </span>
                        )}
                      </div>
                    </button>

                    <div className="flex shrink-0 items-center gap-1">
                      <button
                        type="button"
                        onClick={() => { void handleDeleteSession(session) }}
                        disabled={isDeleting}
                        className="inline-flex items-center gap-1 rounded border border-border bg-background px-2 py-1 text-xs font-medium text-destructive opacity-70 transition-colors hover:bg-destructive/10 group-hover:opacity-100 disabled:cursor-not-allowed disabled:opacity-50"
                        title="Delete this chat"
                        aria-label="Delete this chat"
                      >
                        {isDeleting ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Trash2 className="h-3.5 w-3.5" />}
                      </button>
                      <button
                        type="button"
                        onClick={() => handleSelect(session)}
                        className="inline-flex items-center gap-1 rounded border border-border bg-background px-2 py-1 text-xs font-medium text-muted-foreground opacity-80 transition-colors hover:border-primary/40 hover:text-foreground group-hover:opacity-100"
                      >
                        <ActionIcon className="h-3.5 w-3.5" />
                        {!compact && <span>{actionLabel}</span>}
                      </button>
                    </div>
                  </div>

                  {isExpanded && (
                    <div className="px-10 pb-3">
                      <div className="max-h-80 space-y-2 overflow-y-auto rounded-md border border-border bg-muted/20 p-2 text-xs text-foreground">
                        {isLoadingDetails && !hasLoadedMessages && (
                          <div className="flex items-center gap-2 text-muted-foreground">
                            <Loader2 className="h-3.5 w-3.5 animate-spin" />
                            <span>Loading recent messages...</span>
                          </div>
                        )}
                        {!isLoadingDetails && messages.length === 0 && (
                          <div className="text-muted-foreground">No displayable messages found.</div>
                        )}
                        {isLoadingDetails && hasLoadedMessages && (
                          <div className="flex items-center justify-center gap-2 py-1 text-[11px] text-muted-foreground">
                            <Loader2 className="h-3 w-3 animate-spin" />
                            <span>Loading older messages...</span>
                          </div>
                        )}
                        {!isLoadingDetails && hasMoreBySession[session.session_id] && (
                          <button
                            type="button"
                            onClick={() => void loadExpandedMessages(
                              session,
                              (expandedWindowBySession[session.session_id] ?? EXPANDED_FETCH_LIMIT) + EXPANDED_FETCH_INCREMENT,
                            )}
                            className="w-full rounded border border-border/60 px-2 py-1 text-[11px] text-muted-foreground transition-colors hover:bg-muted/40 hover:text-foreground"
                          >
                            Load older messages
                          </button>
                        )}
                        {(!isLoadingDetails || hasLoadedMessages) && messages.map((message, index) => {
                          const isToolCall = message.role === 'tool_call'
                          const isTool = message.role === 'tool' || isToolCall
                          const normalizedRole = isTool
                            ? `${isToolCall ? 'Tool call' : 'Tool result'}${message.toolName ? ` · ${message.toolName}` : ''}`
                            : message.role === 'ai' || message.role === 'assistant' ? 'Assistant' : 'User'
                          const roleClass = message.isError
                            ? 'text-red-600 dark:text-red-400'
                            : isTool
                              ? 'text-amber-600 dark:text-amber-400'
                              : normalizedRole === 'Assistant'
                                ? 'text-emerald-600 dark:text-emerald-400'
                                : 'text-sky-600 dark:text-sky-400'

                          return (
                            <div key={`${session.session_id}-previous-preview-${index}`} className="space-y-1 rounded bg-background/70 px-2 py-1.5">
                              <div className={`text-[10px] font-semibold uppercase leading-none ${roleClass}`}>
                                {normalizedRole}
                              </div>
                              {/* Rendered as markdown, not raw text: the stored
                                  answer is markdown (bold, headings, tables) and
                                  whitespace-pre-wrap showed it literally, e.g.
                                  "**Toptal**" with the asterisks visible. */}
                              {isTool ? (
                                <pre className={`max-h-40 overflow-auto whitespace-pre-wrap break-words rounded px-1 py-0.5 font-mono text-[11px] leading-4 ${
                                  message.isError
                                    ? 'bg-red-950/25 text-red-200'
                                    : 'bg-muted/40 text-muted-foreground'
                                }`}>
                                  {message.text}
                                </pre>
                              ) : (
                                <div className="break-words leading-relaxed text-muted-foreground">
                                  <ConversationMarkdownRenderer content={message.text} maxHeight="none" framed={false} />
                                </div>
                              )}
                            </div>
                          )
                        })}
                      </div>
                    </div>
                  )}
                </div>
              )
            })}
          </div>
        )}

        {!isLoading && (activeFilter === 'chat' || activeFilter === 'bot') && filteredSessions.length > displayedSessions.length && (
          <div className="border-t border-border px-3 py-2">
            <button
              type="button"
              onClick={() => setVisibleCount(count => count + PAGE_SIZE)}
              className="inline-flex items-center gap-1 rounded border border-border bg-background px-2 py-1 text-xs font-medium text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground"
            >
              <span>Load {PAGE_SIZE} more</span>
            </button>
          </div>
        )}
      </div>
    </div>
  )
}
