import React, { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Virtuoso, type VirtuosoHandle } from 'react-virtuoso'
import { CheckCircle2, ChevronDown, ChevronRight, CircleDashed, XCircle } from 'lucide-react'
import { EventDispatcher } from './events/EventDispatcher'
import { ConversationMarkdownRenderer } from './ui/MarkdownRenderer'
import {
  buildTranscriptItems,
  internalTranscriptMessageTitle,
  isExecutionPromptTranscriptMessage,
  isInternalTranscriptMessage,
  pairToolCalls,
  shouldCollapseTranscriptUserMessage,
  type PairedToolCall,
  selectTerminalEvents,
  type TranscriptItem,
} from '../utils/terminalEventTranscript'
import { formatDurationCompact } from '../utils/duration'
import { formatToolCallArguments, formatToolCallResult } from '../utils/toolCallFormatting'
import type { PollingEvent, TerminalSnapshot } from '../services/api-types'

type TranscriptRenderItem = TranscriptItem | {
  kind: 'live'
  key: string
  text: string
  status: string
}

// Clean view = the SAME rich event components the tree used, laid out as one
// flat chronological conversation for a single terminal.
//
// The rail is the hierarchy now: every agent and sub-agent owns its own
// terminal entry, so parent/child nesting inside a transcript is redundant.
// What lived in EventHierarchy (tree layout, parent resolution, owned log
// panels) is deliberately NOT reproduced — only its two load-bearing
// behaviours are: virtualization, and collapsing consecutive tool calls.
//
// This replaces a renderer that parsed terminal TEXT into synthesized rows.
// That approach could not reuse the event components, so user messages
// rendered raw and every tool call degraded to an anonymous line.
//
// Selection/grouping logic lives in utils/terminalEventTranscript.ts so it can
// be unit-tested without pulling React in.

// ONE card per tool call — not one per event.
//
// A single call arrives as two events and the transcript used to draw a card
// for each. That was worse than verbose, it was misleading: the start event
// never carries arguments, so its "Arguments: (no arguments)" section was
// permanently empty, while the end event held both the arguments and the
// result behind a disclosure. The reader saw two boxes, the useful one closed.
//
// This renders the pair as one thing — identity from the start, args + result
// from the end — and deliberately does NOT nest the old per-event cards, which
// is what produced triplicated server names, boxes inside boxes, and a scroll
// container fighting itself.
const PREVIEW_LIMIT = 600
const AGENT_RESPONSE_EVENT_TYPES = new Set([
  'agent_end',
  'background_agent_completed',
  'llm_generation_end',
  'orchestrator_agent_end',
  'unified_completion',
])

function transcriptEventPayload(event: PollingEvent): Record<string, unknown> {
  const outer = event.data
  if (!outer || typeof outer !== 'object') return {}
  const nested = (outer as { data?: unknown }).data
  return nested && typeof nested === 'object'
    ? nested as Record<string, unknown>
    : outer as Record<string, unknown>
}

function assistantResponseText(event: PollingEvent): string {
  if (!AGENT_RESPONSE_EVENT_TYPES.has(event.type || '')) return ''
  const payload = transcriptEventPayload(event)
  const content = typeof payload.content === 'string' ? payload.content.trim() : ''
  const finalResult = typeof payload.final_result === 'string' ? payload.final_result.trim() : ''
  const result = typeof payload.result === 'string' ? payload.result.trim() : ''
  return content || finalResult || result
}

function presentationActivity(event: PollingEvent): { label: string; title: string; destination: string; detail: string } | null {
	if (event.type !== 'presentation_updated') return null
	const payload = transcriptEventPayload(event)
	const title = typeof payload.title === 'string' && payload.title.trim()
		? payload.title.trim()
		: 'Production item'
	const activity = payload.activity && typeof payload.activity === 'object'
		? payload.activity as Record<string, unknown>
		: {}
	// New events always provide these values from product.yaml. The neutral
	// fallback keeps old persisted events readable without recreating a
	// kind-to-panel map in the frontend.
	const label = typeof activity.label === 'string' && activity.label.trim() ? activity.label.trim() : 'Production update'
	const destination = typeof activity.destination === 'string' && activity.destination.trim() ? activity.destination.trim() : 'Production panel'
	const detail = typeof activity.detail === 'string' && activity.detail.trim() ? activity.detail.trim() : 'Updated'
	return { label, title, destination, detail }
}

// A final response can be represented by two different protocol events during
// a restore (for example `llm_generation_end` plus `unified_completion`). The
// event selector normally removes that pair, but a mixed live/history tail can
// still carry both. The conversation should never make the reader see the
// identical reply twice, so retain just the first adjacent response card.
function removeAdjacentDuplicateAssistantResponses(items: TranscriptItem[]): TranscriptItem[] {
  let previousAnswer = ''
  return items.filter((item) => {
    if (item.kind !== 'event') return true
    if (item.event.type === 'user_message') {
      previousAnswer = ''
      return true
    }
    const answer = assistantResponseText(item.event)
    if (!answer) return true
    const comparable = answer.replace(/\s+/g, ' ').trim().toLowerCase()
    if (comparable && comparable === previousAnswer) return false
    previousAnswer = comparable
    return true
  })
}

const TranscriptEvent: React.FC<{
  event: PollingEvent
  onSendMessage?: (msg: string) => void
  compactUserBottom?: boolean
}> = ({ event, onSendMessage, compactUserBottom = false }) => {
  const payload = transcriptEventPayload(event)
  const content = typeof payload.content === 'string' ? payload.content.trim() : ''
  const rawTimestamp = event.timestamp || (typeof payload.timestamp === 'string' ? payload.timestamp : '')
  const timestamp = rawTimestamp && Number.isFinite(Date.parse(rawTimestamp))
    ? new Date(rawTimestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : ''

  const presentation = presentationActivity(event)
  if (presentation) {
    return <PresentationActivityEvent {...presentation} timestamp={timestamp} />
  }

  if (isInternalTranscriptMessage(event)) {
    return <InternalActivityEvent title={internalTranscriptMessageTitle(event)} content={content} timestamp={timestamp} />
  }

  // Different runtime transports carry a completed agent answer in different
  // fields.  They are all agent responses, so render them with one component
  // and one type scale instead of falling through to several event cards.
  const finalResult = typeof payload.final_result === 'string' ? payload.final_result.trim() : ''
  const result = typeof payload.result === 'string' ? payload.result.trim() : ''
  const responseContent = content || finalResult || result
  if (AGENT_RESPONSE_EVENT_TYPES.has(event.type || '') && responseContent) {
    return <AssistantTranscriptMessage event={event} content={responseContent} timestamp={timestamp} />
  }

  if (isExecutionPromptTranscriptMessage(event)) {
    return <EventDispatcher event={event} onSendMessage={onSendMessage} compact hideOrchestratorContext />
  }

  if (event.type !== 'user_message') {
    return <EventDispatcher event={event} onSendMessage={onSendMessage} compact hideOrchestratorContext />
  }

  return <UserTranscriptMessage content={content || 'Message sent'} timestamp={timestamp} compactBottom={compactUserBottom} />
}

const USER_MESSAGE_PREVIEW_LIMIT = 480

const UserTranscriptMessage: React.FC<{ content: string; timestamp: string; compactBottom?: boolean }> = ({ content, timestamp, compactBottom = false }) => {
  const collapsible = shouldCollapseTranscriptUserMessage(content)
  const [expanded, setExpanded] = useState(false)
  const shown = collapsible && !expanded
    ? `${content.slice(0, USER_MESSAGE_PREVIEW_LIMIT).trimEnd()}…`
    : content

  if (!collapsible) {
    return (
      <div className={`ml-auto mt-4 max-w-[84%] text-right ${compactBottom ? 'mb-1' : 'mb-4'}`}>
        <div className="whitespace-pre-wrap break-words text-[14px] leading-6 text-neutral-200">{shown}</div>
        {timestamp && <div className="mt-1 text-[10px] tabular-nums text-neutral-600">{timestamp}</div>}
      </div>
    )
  }

  return (
    <article className={`ml-auto mt-4 w-[min(92%,52rem)] rounded-lg border border-neutral-800 bg-neutral-900/45 px-4 py-3 text-left ${compactBottom ? 'mb-1' : 'mb-4'}`}>
      <div className="whitespace-pre-wrap break-words text-[13px] leading-6 text-neutral-300">{shown}</div>
      <div className="mt-2 flex items-center gap-3">
        <button
          type="button"
          onClick={() => setExpanded(value => !value)}
          className="text-[11px] font-medium text-neutral-500 transition-colors hover:text-neutral-300"
        >
          {expanded ? 'Show less' : 'Show full message'}
        </button>
        {timestamp && <span className="ml-auto text-[10px] tabular-nums text-neutral-600">{timestamp}</span>}
      </div>
    </article>
  )
}

const AssistantTranscriptMessage: React.FC<{ event: PollingEvent; content: string; timestamp: string; label?: string }> = ({ event, content, timestamp, label = 'Agent' }) => {
  const fields = transcriptEventPayload(event)
  const duration = typeof fields.duration === 'number' && fields.duration > 0
    ? formatDurationCompact(fields.duration)
    : ''
  const turn = typeof fields.turn === 'number' ? fields.turn : undefined

  return (
    <article data-testid="terminal-clear-assistant-message" className="my-4 border-l border-emerald-400/55 pl-4 pr-2">
      <div className="mb-2 flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.12em] text-emerald-300/75">
        <span>{label}</span>
        <span className="h-1 w-1 rounded-full bg-neutral-600" />
        <span className="normal-case font-medium tracking-normal text-neutral-500">
          {turn != null ? `Turn ${turn}` : 'Response'}{duration ? ` · ${duration}` : ''}{timestamp ? ` · ${timestamp}` : ''}
        </span>
      </div>
      <div className="[&_li]:!text-[14px] [&_p]:!text-[14px]">
        <ConversationMarkdownRenderer content={content} framed={false} maxHeight="none" />
      </div>
    </article>
  )
}

const InternalActivityEvent: React.FC<{ title: string; content: string; timestamp: string }> = ({ title, content, timestamp }) => {
  const [open, setOpen] = useState(false)
  return (
    <div data-testid="terminal-clear-system-activity" className="my-3 border-y border-neutral-800/60 py-2">
      <button
        type="button"
        onClick={() => setOpen(value => !value)}
        aria-expanded={open}
        className="flex w-full items-center gap-2 text-left text-[11px] text-neutral-500 transition-colors hover:text-neutral-300"
      >
        <CircleDashed className="h-3.5 w-3.5 shrink-0 text-neutral-500" />
        <span className="truncate">Automation update · {title}</span>
        {timestamp && <span className="ml-auto shrink-0 tabular-nums text-neutral-600">{timestamp}</span>}
        {open ? <ChevronDown className="h-3.5 w-3.5 shrink-0" /> : <ChevronRight className="h-3.5 w-3.5 shrink-0" />}
      </button>
      {open && <pre className="mt-2 max-h-56 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-black/20 p-3 text-[11px] leading-5 text-neutral-400">{content}</pre>}
    </div>
  )
}

const PresentationActivityEvent: React.FC<{ label: string; title: string; destination: string; detail: string; timestamp: string }> = ({ label, title, destination, detail, timestamp }) => (
  <div data-testid="terminal-clear-presentation-activity" className="my-3 border-y border-violet-900/35 bg-violet-950/15 py-2">
    <div className="flex items-center gap-2 px-1 text-[11px] text-violet-200/80">
      <CircleDashed className="h-3.5 w-3.5 shrink-0 text-violet-300/75" />
      <span className="truncate"><span className="font-medium text-violet-100">{label}</span> · {title}</span>
      <span className="hidden shrink-0 text-violet-300/55 sm:inline">{detail} in {destination}</span>
      {timestamp && <span className="ml-auto shrink-0 tabular-nums text-violet-300/50">{timestamp}</span>}
    </div>
  </div>
)

function wheelDeltaPixels(deltaY: number, deltaMode: number, pageHeight: number): number {
  if (deltaMode === 1) return deltaY * 16
  if (deltaMode === 2) return deltaY * Math.max(1, pageHeight)
  return deltaY
}

function elementCanConsumeVerticalWheel(element: HTMLElement, deltaY: number): boolean {
  if (element.scrollHeight <= element.clientHeight + 1) return false
  if (deltaY < 0) return element.scrollTop > 0
  if (deltaY > 0) return element.scrollTop + element.clientHeight < element.scrollHeight - 1
  return false
}

const ToolCallCard: React.FC<{ pair: PairedToolCall }> = ({ pair }) => {
  const [open, setOpen] = useState(false)
  const hasDetail = Boolean(pair.args || pair.result)
  const resultFormatting = useMemo(
    () => pair.result ? formatToolCallResult(pair.result) : null,
    [pair.result],
  )
  const displayStatus = resultFormatting?.isError ? 'error' : pair.status

  const mark = displayStatus === 'error' ? '✗' : displayStatus === 'ok' ? '✓' : '⋯'
  const markClass =
    displayStatus === 'error' ? 'text-red-400' : displayStatus === 'ok' ? 'text-emerald-400' : 'text-neutral-500'
  // Shared formatter rather than a local one: the local copy assumed milliseconds
  // while the wire value is nanoseconds, and it had no minutes branch, so long
  // calls printed absurd second counts.
  const duration =
    pair.durationNs != null && pair.durationNs > 0 ? formatDurationCompact(pair.durationNs) : null

  return (
    <div
      data-testid="terminal-clear-tool-call"
      className={`rounded border ${
        displayStatus === 'error'
          ? 'border-red-900/80 bg-red-950/20'
          : 'border-neutral-800 bg-neutral-900/40'
      }`}
    >
      <button
        type="button"
        onClick={() => setOpen(prev => !prev)}
        aria-expanded={open}
        className="flex w-full items-center gap-2 px-2 py-1.5 text-left text-xs hover:bg-neutral-800/60"
      >
        <span className={`shrink-0 font-mono ${markClass}`}>{mark}</span>
        <span className="truncate font-medium text-neutral-200">{pair.name}</span>
        {pair.server && (
          <span className="shrink-0 rounded bg-neutral-800 px-1.5 py-0.5 text-[10px] text-neutral-400">
            {pair.server}
          </span>
        )}
        {duration && <span className="shrink-0 tabular-nums text-[10px] text-neutral-500">{duration}</span>}
        {displayStatus === 'error' && (
          <span className="shrink-0 rounded bg-red-950 px-1.5 py-0.5 text-[10px] text-red-300">
            failed
          </span>
        )}
        {open
          ? <ChevronDown className="ml-auto h-3.5 w-3.5 shrink-0 text-neutral-500" aria-label="Hide tool details" />
          : <ChevronRight className="ml-auto h-3.5 w-3.5 shrink-0 text-neutral-500" aria-label="Show tool details" />}
      </button>

      {open && (
        <div className="space-y-2 border-t border-neutral-800 px-2 py-2">
          {pair.args && <ToolCallField label="Arguments" value={pair.args} />}
          {pair.result && <ToolCallField label="Output" value={pair.result} />}
          {!hasDetail && (
            <p className="text-[11px] leading-5 text-neutral-500">
              This coding-CLI call did not retain its arguments or output. New calls will include them once the bridge trace update is running.
            </p>
          )}
        </div>
      )}
    </div>
  )
}

// Long args/results must scroll INSIDE their own box. Letting them size the
// card is what broke scrolling once a tool was opened: a multi-KB result grew
// the row past the viewport and took the transcript's scroll with it.
const ToolCallField: React.FC<{ label: string; value: string }> = ({ label, value }) => {
  const [full, setFull] = useState(false)
  const formatted = useMemo(
    () => label === 'Arguments' ? formatToolCallArguments(value) : formatToolCallResult(value),
    [label, value],
  )
  const isLong = formatted.text.length > PREVIEW_LIMIT
  const shown = full || !isLong ? formatted.text : `${formatted.text.slice(0, PREVIEW_LIMIT)}…`
  return (
    <div className="min-w-0">
      <div className="mb-0.5 flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wide text-neutral-500">
        <span>{label}</span>
        {formatted.format !== 'text' && (
          <span className="rounded bg-neutral-800 px-1 py-0.5 text-[9px] tracking-normal text-neutral-400">
            {formatted.format}
          </span>
        )}
        {formatted.isError && (
          <span className="rounded bg-red-950 px-1 py-0.5 text-[9px] tracking-normal text-red-300">
            error
          </span>
        )}
      </div>
      <pre className={`max-h-64 overflow-auto whitespace-pre-wrap break-words rounded border p-2 text-[11px] leading-5 ${
        formatted.isError
          ? 'border-red-900/70 bg-red-950/30 text-red-200'
          : 'border-transparent bg-black/30 text-neutral-300'
      }`}>
        {shown}
      </pre>
      {isLong && (
        <button
          type="button"
          onClick={() => setFull(prev => !prev)}
          className="mt-1 text-[10px] text-neutral-500 hover:text-neutral-300"
        >
          {full ? 'Show less' : `Show all (${formatted.text.length.toLocaleString()} chars)`}
        </button>
      )}
    </div>
  )
}

const ToolBatch: React.FC<{ item: Extract<TranscriptItem, { kind: 'tools' }> }> = ({ item }) => {
  const pairs = useMemo(() => pairToolCalls(item.events), [item.events])
  // A conversation should lead with what the agent said, not implementation
  // detail. Even failures stay closed initially: the visible failed count is
  // the signal, and the user chooses when to inspect arguments/results.
  const [expanded, setExpanded] = useState(false)
  const toggle = useCallback(() => setExpanded(prev => !prev), [])

  return (
    <div data-testid="terminal-clear-tool-batch" className="my-1">
      <button
        type="button"
        onClick={toggle}
        aria-expanded={expanded}
        data-testid="terminal-clear-tool-batch-toggle"
        className="flex items-center gap-1 py-1 text-left text-[11px] text-neutral-600 transition-colors hover:text-neutral-400"
      >
        <span>{item.toolCount} tool {item.toolCount === 1 ? 'call' : 'calls'}</span>
        {expanded
          ? <ChevronDown className="h-3 w-3 shrink-0" />
          : <ChevronRight className="h-3 w-3 shrink-0" />}
      </button>
      {expanded && (
        <div data-testid="terminal-clear-tool-batch-content" className="mt-1 space-y-0.5 border-l border-neutral-700/60 pl-3">
          {pairs.map(pair => (
            <ToolCallCard key={pair.key} pair={pair} />
          ))}
        </div>
      )}
    </div>
  )
}

interface TerminalEventTranscriptProps {
  events: PollingEvent[] | undefined
  terminal: TerminalSnapshot | null | undefined
  // Full terminal list for the session. Required for a correct main-agent
  // transcript (it needs to know which events sibling owned terminals already
  // claim) — see selectTerminalEvents. Optional here only because an owned
  // terminal's own scoping does not need it.
  siblingTerminals?: TerminalSnapshot[]
  onSendMessage?: (msg: string) => void
  loading?: boolean
  loadingOlder?: boolean
  hasOlder?: boolean
  error?: string
  onLoadOlder?: () => void
  onRetry?: () => void
  /** Transient response text from SSE. It is intentionally not persisted as
   * individual protocol events, but belongs in the readable live transcript. */
  streamingText?: string
  streamingStatus?: string
  /** Optional product skin for the transcript backdrop. AgentWorks keeps its
   * existing default; product surfaces can use their own visual identity. */
  surfaceClassName?: string
  /** Whether a product should follow an entire turn, or reveal only the first
   * assistant text and then leave the reader in control of the scroll position. */
  autoScrollMode?: 'follow-turn' | 'reveal-first-response'
}

const TerminalEventTranscriptInner: React.FC<TerminalEventTranscriptProps> = ({
  events,
  terminal,
  siblingTerminals,
  onSendMessage,
  loading = false,
  loadingOlder = false,
  hasOlder = false,
  error,
  onLoadOlder,
  onRetry,
  streamingText = '',
  streamingStatus = '',
  surfaceClassName,
  autoScrollMode = 'follow-turn',
}) => {
  const scrollerRef = useRef<HTMLElement | Window | null>(null)
  const virtuosoRef = useRef<VirtuosoHandle | null>(null)
  const scoped = useMemo(
    () => selectTerminalEvents(events, terminal, siblingTerminals),
    [events, terminal, siblingTerminals],
  )
  const items = useMemo<TranscriptRenderItem[]>(
    () => removeAdjacentDuplicateAssistantResponses(buildTranscriptItems(scoped)),
    [scoped],
  )
  const latestUserMessageKey = useMemo(() => {
    for (let index = items.length - 1; index >= 0; index -= 1) {
      const item = items[index]
      if (item.kind === 'event' && item.event.type === 'user_message') return item.key
    }
    return ''
  }, [items])
  const transcriptTailRevision = useMemo(() => {
    const tail = items[items.length - 1]
    if (!tail) return `empty:${streamingStatus}:${streamingText.length}`
    if (tail.kind === 'event') {
      const payload = transcriptEventPayload(tail.event)
      const body = typeof payload.content === 'string'
        ? payload.content
        : typeof payload.result === 'string'
          ? payload.result
          : ''
      return `${tail.key}:${body.length}:${streamingStatus}:${streamingText.length}`
    }
    return `${tail.key}:${streamingStatus}:${streamingText.length}`
  }, [items, streamingStatus, streamingText.length])
  const followedUserMessageKeyRef = useRef(latestUserMessageKey)
  const followCurrentTurnRef = useRef(true)
  const firstResponseUserMessageKeyRef = useRef(latestUserMessageKey)
  const revealFirstResponseRef = useRef(false)
  const suppressFirstResponseRevealRef = useRef(false)
  const assistantResponseAfterLatestUser = useMemo(() => {
    let latestUserIndex = -1
    for (let index = items.length - 1; index >= 0; index -= 1) {
      const item = items[index]
      if (item.kind === 'event' && item.event.type === 'user_message') {
        latestUserIndex = index
        break
      }
    }
    if (latestUserIndex < 0) return false
    return items.slice(latestUserIndex + 1).some(item =>
      item.kind === 'event' && Boolean(assistantResponseText(item.event)),
    )
  }, [items])
  // Do not reserve a permanent header for history. The user reaches this
  // control at the oldest currently-loaded item; it only exists when another
  // page can actually be fetched from the backend. A short restored transcript
  // can fit entirely in the viewport, which means it starts at item zero and
  // has no physical scroll gesture to make. Treat that as "at the top" too —
  // otherwise a reader can see "Previous conversation" but has no way to load
  // the older durable page.
  const [isAtTranscriptStart, setIsAtTranscriptStart] = useState(false)
  const showEarlierMessagesControl = Boolean(
    error || (isAtTranscriptStart && (hasOlder || loadingOlder) && onLoadOlder),
  )

  const handleEarlierMessages = useCallback(() => {
    if (!hasOlder) return
    onLoadOlder?.()
  }, [hasOlder, onLoadOlder])

  // Sending a message changes more than the transcript: the optimistic user
  // row appears immediately, then delivery/status chrome can reduce the
  // transcript viewport a frame later. Virtuoso's normal followOutput handles
  // the first change but not a same-length live-to-final replacement. Follow
  // the whole current turn through its final answer, and stop only when the
  // reader deliberately scrolls upward.
  useEffect(() => {
    if (autoScrollMode !== 'follow-turn') return
    const isNewUserMessage = Boolean(
      latestUserMessageKey && followedUserMessageKeyRef.current !== latestUserMessageKey,
    )
    if (isNewUserMessage) {
      followedUserMessageKeyRef.current = latestUserMessageKey
      followCurrentTurnRef.current = true
    }
    if (!followCurrentTurnRef.current) return

    const scrollToLatest = () => {
      virtuosoRef.current?.scrollToIndex({
        index: Math.max(0, items.length - 1),
        align: 'end',
        behavior: 'auto',
      })
    }
    const frame = window.requestAnimationFrame(scrollToLatest)
    const settledLayoutTimer = window.setTimeout(scrollToLatest, 180)
    return () => {
      window.cancelAnimationFrame(frame)
      window.clearTimeout(settledLayoutTimer)
    }
  }, [autoScrollMode, items.length, latestUserMessageKey, transcriptTailRevision])

  // Video Studio presents long-form creative work where a reader often starts
  // examining the first lines while the agent is still writing. Reveal that
  // first assistant text, then stop: continuously following every streamed
  // chunk steals the reader's scroll position and makes the response hard to
  // inspect. AgentWorks keeps the existing full-turn follow behaviour above.
  useEffect(() => {
    if (autoScrollMode !== 'reveal-first-response') return
    const isNewUserMessage = Boolean(
      latestUserMessageKey && firstResponseUserMessageKeyRef.current !== latestUserMessageKey,
    )
    if (isNewUserMessage) {
      firstResponseUserMessageKeyRef.current = latestUserMessageKey
      revealFirstResponseRef.current = true
      suppressFirstResponseRevealRef.current = false
    }

    const assistantHasBegun = Boolean(streamingText.trim()) || assistantResponseAfterLatestUser
    if (!revealFirstResponseRef.current || suppressFirstResponseRevealRef.current || !assistantHasBegun) return

    revealFirstResponseRef.current = false
    const targetIndex = Math.max(0, (streamingText || streamingStatus) ? items.length : items.length - 1)
    const frame = window.requestAnimationFrame(() => {
      virtuosoRef.current?.scrollToIndex({ index: targetIndex, align: 'end', behavior: 'smooth' })
    })
    return () => window.cancelAnimationFrame(frame)
  }, [assistantResponseAfterLatestUser, autoScrollMode, items.length, latestUserMessageKey, streamingStatus, streamingText])

  // Electron occasionally fails to route a physical wheel/trackpad gesture to
  // Virtuoso's internal scroller even though accessibility scroll actions work.
  // Forward the gesture explicitly. Nested scroll regions (expanded tool output)
  // keep first refusal while they can still move in the requested direction.
  const handleWheelCapture = useCallback((event: React.WheelEvent<HTMLDivElement>) => {
    const scroller = scrollerRef.current
    if (!(scroller instanceof HTMLElement) || event.deltaY === 0) return
    if (event.deltaY < 0) {
      followCurrentTurnRef.current = false
      suppressFirstResponseRevealRef.current = true
    }

    let target = event.target instanceof HTMLElement ? event.target : null
    while (target && target !== event.currentTarget) {
      if (target !== scroller && elementCanConsumeVerticalWheel(target, event.deltaY)) return
      target = target.parentElement
    }

    if (!elementCanConsumeVerticalWheel(scroller, event.deltaY)) return
    event.preventDefault()
    event.stopPropagation()
    scroller.scrollTop += wheelDeltaPixels(event.deltaY, event.deltaMode, scroller.clientHeight)
    if (event.deltaY > 0 && scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 32) {
      followCurrentTurnRef.current = true
    }
  }, [])

  if (items.length === 0 && !streamingText && !streamingStatus) {
    const state = (terminal?.state || '').trim().toLowerCase()
    const failed = state === 'failed' || state === 'error' || state === 'stale'
    const completed = state === 'completed' || state === 'closing'
    const Icon = error ? XCircle : failed ? XCircle : completed ? CheckCircle2 : CircleDashed
    const title = loading
      ? 'Loading conversation…'
      : error
        ? 'Conversation could not be loaded.'
        : failed
      ? 'This agent did not finish.'
      : completed
        ? 'This agent completed.'
        : 'Waiting for this agent to begin.'
    const detail = error || (completed || failed
      ? 'Conversation details are not available for this retained run.'
      : 'Its conversation will appear here when the first event arrives.')
    return (
      <div
        data-testid="terminal-clear-view-empty"
        className={`flex min-w-0 flex-1 items-center justify-center overflow-y-auto px-5 py-8 ${surfaceClassName ?? 'bg-[#0b0d0c]'}`}
      >
        <div className="flex max-w-md items-start gap-3 text-left">
          <Icon
            className={`mt-0.5 h-5 w-5 shrink-0 ${
              error || failed
                ? 'text-red-400'
                : completed
                  ? 'text-emerald-400'
                  : loading
                    ? 'animate-spin text-cyan-400'
                    : 'text-cyan-400'
            }`}
          />
          <div>
            <div className="text-sm font-medium text-neutral-200">{title}</div>
            <div className="mt-1 text-xs leading-5 text-neutral-500">{detail}</div>
            {error && onRetry && (
              <button
                type="button"
                onClick={onRetry}
                className="mt-3 rounded border border-neutral-700 px-2.5 py-1 text-xs text-neutral-300 hover:bg-neutral-800"
              >
                Retry
              </button>
            )}
          </div>
        </div>
      </div>
    )
  }

  return (
    <div
      data-testid="terminal-clear-view"
      className={`flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden ${surfaceClassName ?? 'bg-[#0d100f]'}`}
      onWheelCapture={handleWheelCapture}
    >
      {showEarlierMessagesControl && (
        <div className={`flex shrink-0 items-center border-b px-3 py-1.5 text-[11px] ${
          error
            ? 'border-red-900/60 bg-red-950/25 text-red-300'
            : 'border-neutral-800 bg-neutral-950/80 text-neutral-500'
        }`}>
          {error ? (
            <>
              <span className="truncate">Refresh failed: {error}</span>
              {onRetry && (
                <button type="button" onClick={onRetry} className="ml-auto shrink-0 text-red-200 hover:text-white">
                  Retry
                </button>
              )}
            </>
          ) : (
            <button
              type="button"
              onClick={handleEarlierMessages}
              disabled={loadingOlder}
              className="mx-auto rounded px-2 py-0.5 text-neutral-400 hover:bg-neutral-800 hover:text-neutral-200 disabled:cursor-wait disabled:opacity-60"
            >
              {loadingOlder ? 'Loading earlier messages…' : 'Load earlier messages'}
            </button>
          )}
        </div>
      )}
      {/* Virtualized: the tree inherited this from EventHierarchy. A flat list
          that rendered every event would regress long sessions badly. */}
      <Virtuoso
        ref={virtuosoRef}
        data={streamingText || streamingStatus
          ? [...items, { kind: 'live' as const, key: '__live-stream__', text: streamingText, status: streamingStatus }]
          : items}
        className="custom-scrollbar min-h-0 flex-1"
        scrollerRef={ref => { scrollerRef.current = ref }}
        rangeChanged={({ startIndex }) => {
          setIsAtTranscriptStart(startIndex === 0)
        }}
        followOutput={autoScrollMode === 'follow-turn' ? 'smooth' : false}
        initialTopMostItemIndex={Math.max(0, items.length - 1)}
        computeItemKey={(_, item) => item.key}
        itemContent={(index, item) =>
          item.kind === 'live' ? (
            <LiveAssistantTranscript text={item.text} status={item.status} />
          ) : item.kind === 'tools' ? (
            <div className="px-5">
              <ToolBatch item={item} />
            </div>
          ) : (
            <div data-testid={`terminal-clear-event-${item.event.id || item.key}`} className="px-5 py-0.5">
              <TranscriptEvent
                event={item.event}
                onSendMessage={onSendMessage}
                compactUserBottom={items[index + 1]?.kind === 'tools'}
              />
            </div>
          )
        }
      />
    </div>
  )
}

const LiveAssistantTranscript: React.FC<{ text: string; status: string }> = ({ text }) => (
  text ? (
    <article data-testid="terminal-clear-live-assistant-message" className="mx-5 my-4 border-l-2 border-cyan-400/55 pl-4 pr-2">
      <div className="[&_li]:!text-[14px] [&_p]:!text-[14px]">
        <ConversationMarkdownRenderer content={text} framed={false} maxHeight="none" />
      </div>
      <span aria-label="Writing" className="mt-1 inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-cyan-400" />
    </article>
  ) : null
)

// Memoized: the parent re-renders on every terminal poll, and re-rendering the
// whole transcript each time would defeat EventDispatcher's own memoization.
export const TerminalEventTranscript = memo(TerminalEventTranscriptInner)
