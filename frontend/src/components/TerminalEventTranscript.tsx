import React, { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Virtuoso, type VirtuosoHandle } from 'react-virtuoso'
import { CheckCircle2, ChevronDown, ChevronRight, CircleDashed, Loader2, XCircle } from 'lucide-react'
import { EventDispatcher } from './events/EventDispatcher'
import { ConversationMarkdownRenderer } from './ui/MarkdownRenderer'
import {
  buildTranscriptItems,
  internalTranscriptMessageTitle,
  isExecutionPromptTranscriptMessage,
  isInternalTranscriptMessage,
  pairToolCalls,
  runActivity,
  shouldCollapseTranscriptUserMessage,
  type PairedToolCall,
  selectTerminalEvents,
  type TranscriptItem,
} from '../utils/terminalEventTranscript'
import { formatDurationCompact } from '../utils/duration'
import { formatToolCallArguments, formatToolCallResult } from '../utils/toolCallFormatting'
import type { PollingEvent, TerminalSnapshot } from '../services/api-types'
import { parseProductInteraction, type ProductInteraction } from '../../shared/session/interactions'

// Message text sizes multiply --chat-scale (default 1), so a product can offer
// a bigger reading size (SparkQuill's Child Mode "T" button sets it on the
// chat section) without the transcript knowing about the control.
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

function transcriptTimestamp(event: PollingEvent): string {
  const payload = transcriptEventPayload(event)
  const rawTimestamp = event.timestamp || (typeof payload.timestamp === 'string' ? payload.timestamp : '')
  return rawTimestamp && Number.isFinite(Date.parse(rawTimestamp))
    ? new Date(rawTimestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
    : ''
}

// The event the transcript renders as the agent's reply for a turn.
function isAgentResponseEvent(event: PollingEvent): boolean {
  return AGENT_RESPONSE_EVENT_TYPES.has(event.type || '') && Boolean(assistantResponseText(event))
}

const TranscriptEvent: React.FC<{
  event: PollingEvent
  onSendMessage?: (msg: string) => void
  compactUserBottom?: boolean
  /** Rendered inside a turn block that already draws the border and header. */
  inTurn?: boolean
  renderInteraction?: (interaction: ProductInteraction, event: PollingEvent) => React.ReactNode
  assistantLabel?: string
  assistantIcon?: React.ReactNode
  /** The turn's clock is shown elsewhere or not at all; draw no time on this row. */
  hideTimestamp?: boolean
}> = ({ event, onSendMessage, compactUserBottom = false, inTurn = false, renderInteraction, assistantLabel, assistantIcon, hideTimestamp = false }) => {
  if (event.type === 'product_interaction') {
    const interaction = parseProductInteraction(event)
    return interaction && renderInteraction ? <>{renderInteraction(interaction, event)}</> : null
  }
  const payload = transcriptEventPayload(event)
  const content = typeof payload.content === 'string' ? payload.content.trim() : ''
  const timestamp = hideTimestamp ? '' : transcriptTimestamp(event)

  const run = runActivity(event)
  if (run) return <RunActivityEvent {...run} timestamp={timestamp} />

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
    return <AssistantTranscriptMessage event={event} content={responseContent} timestamp={timestamp} framed={!inTurn} label={assistantLabel} icon={assistantIcon} />
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
        <div className="whitespace-pre-wrap break-words text-[length:calc(14px*var(--chat-scale,1))] leading-[calc(24px*var(--chat-scale,1))] text-foreground">{shown}</div>
        {timestamp && <div className="mt-1 text-[10px] tabular-nums text-muted-foreground">{timestamp}</div>}
      </div>
    )
  }

  return (
    <article className={`ml-auto mt-4 w-[min(92%,52rem)] rounded-lg border border-border bg-muted/30 px-4 py-3 text-left ${compactBottom ? 'mb-1' : 'mb-4'}`}>
      <div className="whitespace-pre-wrap break-words text-[length:calc(13px*var(--chat-scale,1))] leading-[calc(24px*var(--chat-scale,1))] text-foreground/90">{shown}</div>
      <div className="mt-2 flex items-center gap-3">
        <button
          type="button"
          onClick={() => setExpanded(value => !value)}
          className="text-[11px] font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          {expanded ? 'Show less' : 'Show full message'}
        </button>
        {timestamp && <span className="ml-auto text-[10px] tabular-nums text-muted-foreground">{timestamp}</span>}
      </div>
    </article>
  )
}

// The turn's header line: who spoke, turn, duration, time. It sits at the top
// of the agent's block, which starts at the turn's first tool call when there
// is one, so tool work reads as part of the reply rather than a stray chip.
const AssistantTurnHeader: React.FC<{ event: PollingEvent; timestamp: string; label?: string; icon?: React.ReactNode }> = ({ event, timestamp, label = 'Agent', icon }) => {
  const fields = transcriptEventPayload(event)
  const duration = typeof fields.duration === 'number' && fields.duration > 0
    ? formatDurationCompact(fields.duration)
    : ''
  const turn = typeof fields.turn === 'number' ? fields.turn : undefined
  const metadata = [turn != null ? `Turn ${turn}` : '', duration, timestamp].filter(Boolean).join(' · ')
  return (
    <div data-testid="terminal-clear-assistant-header" className="mb-2 flex items-center gap-2 text-[10px] font-semibold uppercase tracking-[0.12em] text-muted-foreground">
      {icon && <span className="inline-flex h-4 w-4 shrink-0 items-center justify-center [&>img]:h-4 [&>img]:w-4 [&>svg]:h-4 [&>svg]:w-4" aria-hidden="true">{icon}</span>}
      <span>{label}</span>
      {metadata && <>
        <span className="h-1 w-1 rounded-full bg-muted-foreground/60" />
        <span className="normal-case font-medium tracking-normal text-muted-foreground">{metadata}</span>
      </>}
    </div>
  )
}

const AGENT_BLOCK_CLASS = 'pl-3 pr-1'

const IS_ELECTRON = typeof navigator !== 'undefined' && /Electron/i.test(navigator.userAgent)

// Pins a scroller to its end over a few frames: Virtuoso measures newly
// rendered items after paint and compensates scrollTop for the difference,
// so a single assignment can land short. Stops early once it is there.
function settleToEnd(scroller: HTMLElement, frames = 8): void {
  let left = frames
  const step = () => {
    scroller.scrollTop = scroller.scrollHeight
    left -= 1
    if (left > 0 && scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight > 1) {
      window.requestAnimationFrame(step)
    }
  }
  window.requestAnimationFrame(step)
}

// Where an item sits in its agent turn. A turn is everything between two user
// messages: tool batches, thoughts, replies, presentation and activity rows.
type TurnSlot = { agent: boolean; first: boolean; last: boolean; header?: PollingEvent; showTime: boolean }

// A time label only where time passed: the first message, and any message
// more than this long after the previous labelled one. Back-to-back turns
// carry no clock.
const TIME_LABEL_GAP_MS = 5 * 60 * 1000

function itemTime(item: TranscriptRenderItem | undefined): number {
  if (!item || item.kind === 'live') return NaN
  const event = item.kind === 'event' ? item.event : item.events[0]
  if (!event) return NaN
  const payload = transcriptEventPayload(event)
  return Date.parse(event.timestamp || (typeof payload.timestamp === 'string' ? payload.timestamp : ''))
}

function isUserItem(item: TranscriptRenderItem | undefined): boolean {
  return item?.kind === 'event' && item.event.type === 'user_message'
}

function buildTurnSlots(data: TranscriptRenderItem[]): TurnSlot[] {
  const slots: TurnSlot[] = []
  let inTurn = false
  let lastLabelled = NaN
  const decideTime = (item: TranscriptRenderItem): boolean => {
    const at = itemTime(item)
    if (!Number.isFinite(at)) return false
    if (Number.isFinite(lastLabelled) && at - lastLabelled < TIME_LABEL_GAP_MS) return false
    lastLabelled = at
    return true
  }
  data.forEach((item, index) => {
    if (item.kind === 'live' || isUserItem(item)) {
      inTurn = false
      slots.push({ agent: false, first: false, last: false, showTime: item.kind !== 'live' && decideTime(item) })
      return
    }
    const first = !inTurn
    inTurn = true
    const next = data[index + 1]
    slots.push({ agent: true, first, last: !next || next.kind === 'live' || isUserItem(next), showTime: first && decideTime(item) })
  })
  // The header carries the turn's reply metadata (turn, duration, time): the
  // turn's first reply, or its first event while there is no reply yet.
  slots.forEach((slot, index) => {
    if (!slot.first) return
    for (let j = index; j < data.length && slots[j]?.agent; j++) {
      const item = data[j]
      if (item.kind === 'event' && isAgentResponseEvent(item.event)) {
        slot.header = item.event
        return
      }
    }
    const item = data[index]
    slot.header = item.kind === 'event' ? item.event : item.kind === 'live' ? undefined : item.events[0]
  })
  return slots
}

const AssistantTranscriptMessage: React.FC<{ event: PollingEvent; content: string; timestamp: string; label?: string; icon?: React.ReactNode; framed?: boolean }> = ({ event, content, timestamp, label = 'Agent', icon, framed = true }) => {
  return (
    <article data-testid="terminal-clear-assistant-message" className={framed ? `my-4 ${AGENT_BLOCK_CLASS}` : 'py-1'}>
      {framed && <AssistantTurnHeader event={event} timestamp={timestamp} label={label} icon={icon} />}
      <div className="[&_li]:!text-[length:calc(14px*var(--chat-scale,1))] [&_p]:!text-[length:calc(14px*var(--chat-scale,1))] [&_li]:!leading-[calc(24px*var(--chat-scale,1))] [&_p]:!leading-[calc(24px*var(--chat-scale,1))]">
        <ConversationMarkdownRenderer content={content} framed={false} maxHeight="none" />
      </div>
    </article>
  )
}

const InternalActivityEvent: React.FC<{ title: string; content: string; timestamp: string }> = ({ title, content, timestamp }) => {
  const [open, setOpen] = useState(false)
  return (
    <div data-testid="terminal-clear-system-activity" className="my-3 border-y border-border/60 py-2">
      <button
        type="button"
        onClick={() => setOpen(value => !value)}
        aria-expanded={open}
        className="flex w-full items-center gap-2 text-left text-[11px] text-muted-foreground transition-colors hover:text-foreground"
      >
        <CircleDashed className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">Automation update · {title}</span>
        {timestamp && <span className="ml-auto shrink-0 tabular-nums text-muted-foreground">{timestamp}</span>}
        {open ? <ChevronDown className="h-3.5 w-3.5 shrink-0" /> : <ChevronRight className="h-3.5 w-3.5 shrink-0" />}
      </button>
      {open && <pre className="mt-2 max-h-56 overflow-auto whitespace-pre-wrap break-words rounded-lg bg-muted/60 p-3 text-[11px] leading-5 text-muted-foreground">{content}</pre>}
    </div>
  )
}

const PresentationActivityEvent: React.FC<{ label: string; title: string; destination: string; detail: string; timestamp: string }> = ({ label, title, destination, detail, timestamp }) => (
  // Token-driven so it reads on light and dark products alike: the earlier
  // violet-on-violet palette hid the title on a light surface.
  <div data-testid="terminal-clear-presentation-activity" className="my-3 rounded-lg border border-border/60 bg-muted/40 px-3 py-2">
    <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
      <CircleDashed className="h-3.5 w-3.5 shrink-0 text-primary/70" />
      <span className="truncate"><span className="font-medium text-foreground/85">{label}</span> · <span className="text-foreground">{title}</span></span>
      <span className="hidden shrink-0 sm:inline">{detail} in {destination}</span>
      {timestamp && <span className="ml-auto shrink-0 tabular-nums">{timestamp}</span>}
    </div>
  </div>
)

// Running a step, the workflow, or an evaluation is the headline action of a
// workflow turn, so it gets the same compact activity row a presentation gets
// rather than disappearing into a collapsed "N tool calls" chip.
const RunActivityEvent: React.FC<{ label: string; target: string; state: 'started' | 'finished' | 'failed'; timestamp: string }> = ({ label, target, state, timestamp }) => (
  <div data-testid="terminal-clear-run-activity" className="my-3 rounded-lg border border-border/60 bg-muted/40 px-3 py-2">
    <div className="flex items-center gap-2 text-[11px] text-muted-foreground">
      {state === 'started'
        ? <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin text-primary/70" aria-hidden="true" />
        : state === 'failed'
          ? <CircleDashed className="h-3.5 w-3.5 shrink-0 text-red-500" aria-hidden="true" />
          : <CircleDashed className="h-3.5 w-3.5 shrink-0 text-emerald-500" aria-hidden="true" />}
      <span className="truncate">
        <span className="font-medium text-foreground/85">{label}</span>
        {target && <> · <span className="text-foreground">{target}</span></>}
      </span>
      <span className="hidden shrink-0 sm:inline">
        {state === 'started' ? 'in the workflow' : state === 'failed' ? 'failed' : 'finished'}
      </span>
      {timestamp && <span className="ml-auto shrink-0 tabular-nums">{timestamp}</span>}
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
    displayStatus === 'error' ? 'text-red-400' : displayStatus === 'ok' ? 'text-emerald-400' : 'text-muted-foreground'
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
          ? 'border-red-300/70 bg-red-50/70 dark:border-red-900/80 dark:bg-red-950/20'
          : 'border-border/70 bg-card'
      }`}
    >
      <button
        type="button"
        onClick={() => setOpen(prev => !prev)}
        aria-expanded={open}
        className="flex w-full items-center gap-2 px-2 py-1.5 text-left text-xs hover:bg-muted/60"
      >
        <span className={`shrink-0 font-mono ${markClass}`}>{mark}</span>
        <span className="truncate font-medium text-foreground">{pair.name}</span>
        {pair.server && (
          <span className="shrink-0 rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
            {pair.server}
          </span>
        )}
        {duration && <span className="shrink-0 tabular-nums text-[10px] text-muted-foreground">{duration}</span>}
        {displayStatus === 'error' && (
          <span className="shrink-0 rounded bg-red-100/80 px-1.5 py-0.5 text-[10px] text-red-700 dark:bg-red-950 dark:text-red-300">
            failed
          </span>
        )}
        {open
          ? <ChevronDown className="ml-auto h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-label="Hide tool details" />
          : <ChevronRight className="ml-auto h-3.5 w-3.5 shrink-0 text-muted-foreground" aria-label="Show tool details" />}
      </button>

      {open && (
        <div className="space-y-2 border-t border-border/70 px-2 py-2">
          {pair.args && <ToolCallField label="Arguments" value={pair.args} />}
          {pair.result && <ToolCallField label="Output" value={pair.result} />}
          {!hasDetail && (
            <p className="text-[11px] leading-5 text-muted-foreground">
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
      <div className="mb-0.5 flex items-center gap-1.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
        <span>{label}</span>
        {formatted.format !== 'text' && (
          <span className="rounded bg-muted px-1 py-0.5 text-[9px] tracking-normal text-muted-foreground">
            {formatted.format}
          </span>
        )}
        {formatted.isError && (
          <span className="rounded bg-red-100 px-1 py-0.5 text-[9px] tracking-normal text-red-700 dark:bg-red-950 dark:text-red-300">
            error
          </span>
        )}
      </div>
      <pre className={`max-h-64 overflow-auto whitespace-pre-wrap break-words rounded border p-2 text-[11px] leading-5 ${
        formatted.isError
          ? 'border-red-200 bg-red-50 text-red-800 dark:border-red-900/70 dark:bg-red-950/30 dark:text-red-200'
          : 'border-border/60 bg-muted/40 text-foreground/90'
      }`}>
        {shown}
      </pre>
      {isLong && (
        <button
          type="button"
          onClick={() => setFull(prev => !prev)}
          className="mt-1 text-[10px] text-muted-foreground hover:text-foreground"
        >
          {full ? 'Show less' : `Show all (${formatted.text.length.toLocaleString()} chars)`}
        </button>
      )}
    </div>
  )
}

// Thinking is a collapsible block like a tool batch, open by default (user
// decision 2026-09-03: it used to minimise once the answer streamed, which
// hid commentary people were still reading). `live` only drives the pulse dot
// while the agent is still reasoning; the toggle is the user's alone.
const ThinkingBatch: React.FC<{ item: Extract<TranscriptItem, { kind: 'thinking' }>; live: boolean }> = ({ item, live }) => {
  const [expanded, setExpanded] = useState(true)
  const toggle = useCallback(() => setExpanded(prev => !prev), [])

  return (
    <div data-testid="terminal-clear-thinking-batch" className="my-1">
      <button
        type="button"
        onClick={toggle}
        aria-expanded={expanded}
        data-testid="terminal-clear-thinking-batch-toggle"
        className="flex items-center gap-1 py-1 text-left text-[11px] text-muted-foreground transition-colors hover:text-foreground"
      >
        <span>Thinking</span>
        {live && <span aria-hidden="true" className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-muted-foreground/70" />}
        {expanded
          ? <ChevronDown className="h-3 w-3 shrink-0" />
          : <ChevronRight className="h-3 w-3 shrink-0" />}
      </button>
      {expanded && (
        <p
          data-testid="terminal-clear-thinking-batch-content"
          className="mt-1 whitespace-pre-wrap break-words border-l border-border pl-3 text-xs leading-5 text-muted-foreground"
        >
          {item.text}
        </p>
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
        className="flex items-center gap-1 py-1 text-left text-[11px] text-muted-foreground transition-colors hover:text-foreground"
      >
        <span>{item.toolCount} tool {item.toolCount === 1 ? 'call' : 'calls'}</span>
        {expanded
          ? <ChevronDown className="h-3 w-3 shrink-0" />
          : <ChevronRight className="h-3 w-3 shrink-0" />}
      </button>
      {expanded && (
        <div data-testid="terminal-clear-tool-batch-content" className="mt-1 space-y-0.5 border-l border-border pl-3">
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
  /** Whether a product should follow an entire turn, or reveal meaningful
   * turn boundaries (send, tool, first stream, final answer) only. */
  autoScrollMode?: 'follow-turn' | 'reveal-first-response'
  /** Product interactions to show in place, inside the agent's turn, with the
   * product's own rendering (a celebration, an inline scene). Other
   * interaction kinds stay on the side channel. */
  productRows?: { kinds: string[]; render: (interaction: ProductInteraction, event: PollingEvent) => React.ReactNode }
  /** What the agent is called in turn headers ("Agent" by default; a product passes its own name, e.g. "Quill"). */
  assistantLabel?: string
  /** A small mark drawn before the label in turn headers (a product's logo); none by default. */
  assistantIcon?: React.ReactNode
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
  // Follow the whole turn by default (AgentWorks, SparkQuill, the terminal
  // center). Only a surface that says so gets reveal-first-response; the
  // default had drifted to that on 2026-09-02 and AgentWorks lost its
  // follow behaviour without any call site changing.
  autoScrollMode = 'follow-turn',
  productRows,
  assistantLabel = 'Agent',
  assistantIcon,
}) => {
  const scrollerRef = useRef<HTMLElement | Window | null>(null)
  const virtuosoRef = useRef<VirtuosoHandle | null>(null)
  const keptKinds = productRows?.kinds.join('\u0000') ?? ''
  const keepInteractionKinds = useMemo(() => new Set(keptKinds ? keptKinds.split('\u0000') : []), [keptKinds])
  const renderInteraction = productRows?.render
  const scoped = useMemo(
    () => selectTerminalEvents(events, terminal, siblingTerminals, keepInteractionKinds),
    [events, terminal, siblingTerminals, keepInteractionKinds],
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
  // Keep this unset until the first effect runs. Initialising it to the latest
  // user row made an in-progress turn restored after refresh look historical:
  // the first streaming response then never armed Video Studio's one-time
  // reveal and arrived below the composer.
  const firstResponseUserMessageKeyRef = useRef<string | null>(null)
  const initializedFirstResponseRevealRef = useRef(false)
  const revealFirstResponseRef = useRef(false)
  // Video Studio intentionally does not follow every streamed token. It does
  // need to reveal meaningful state changes in a turn, though: tool activity,
  // the first live response, and the durable final answer.
  const initializedActivityRevealRef = useRef(false)
  const previousLiveStreamRef = useRef(false)
  const lastRevealedToolKeyRef = useRef('')
  const lastRevealedAssistantKeyRef = useRef('')
  // Virtuoso is not always ready in the same commit that adds an optimistic
  // user row. Keep a short, keyed retry sequence alive across the status/text
  // re-renders that immediately follow submission.
  const userRevealGenerationRef = useRef(0)
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

  const listData = useMemo<TranscriptRenderItem[]>(
    () => (streamingText || streamingStatus
      ? [...items, { kind: 'live' as const, key: '__live-stream__', text: streamingText, status: streamingStatus }]
      : items),
    [items, streamingStatus, streamingText],
  )
  const turnSlots = useMemo(() => buildTurnSlots(listData), [listData])

  // initialTopMostItemIndex is a mount-time prop: Virtuoso keeps the list
  // invisible until it has scrolled there. Keep it fixed for the life of the
  // list and settle a late first fill (history hydrating after an empty
  // mount) with an explicit scroll to the end instead.
  const [initialTopMostItemIndex] = useState(() => Math.max(0, items.length - 1))
  // false even when items exist at mount: Virtuoso's initial scroll uses
  // size estimates, so the first fill always needs the settle below.
  const settledFirstFillRef = useRef(false)
  useEffect(() => {
    if (settledFirstFillRef.current || items.length === 0) return
    settledFirstFillRef.current = true
    const frame = window.requestAnimationFrame(() => {
      virtuosoRef.current?.scrollToIndex({ index: items.length - 1, align: 'end', behavior: 'auto' })
      const scroller = scrollerRef.current
      if (scroller instanceof HTMLElement) settleToEnd(scroller, 12)
    })
    return () => window.cancelAnimationFrame(frame)
  }, [items.length])

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
      // scrollToIndex positions by Virtuoso's size estimate; the real end is
      // the scroller's own height once the item is measured.
      const scroller = scrollerRef.current
      if (scroller instanceof HTMLElement) settleToEnd(scroller)
    }
    const frame = window.requestAnimationFrame(scrollToLatest)
    const settledLayoutTimer = window.setTimeout(scrollToLatest, 180)
    return () => {
      window.cancelAnimationFrame(frame)
      window.clearTimeout(settledLayoutTimer)
    }
  }, [autoScrollMode, items.length, latestUserMessageKey, transcriptTailRevision])

  // Stick to the end while a turn is being followed. Two things move the end
  // without a new item: chrome outside the list (a working indicator, pills,
  // delivery status) shrinks the viewport, and items get their real height
  // only after Virtuoso's first estimate (a scroll issued on send landed
  // short by that difference, and nothing corrected it until the next
  // event). Observing both the viewport and the list content covers both.
  //
  // Two guards keep this from fighting the reader: it only acts when the
  // reader was already at the end before the change (a list that grows while
  // they are scrolled up must not yank them down, which showed as a flicker
  // on every scroll), and it scrolls on the next frame, after Virtuoso's own
  // scroll compensation for re-measured items, then checks its work for a
  // few frames because measurements settle in more than one.
  const nearEndRef = useRef(true)
  useEffect(() => {
    if (autoScrollMode !== 'follow-turn') return
    const scroller = scrollerRef.current
    if (!(scroller instanceof HTMLElement) || typeof ResizeObserver === 'undefined') return
    const onScroll = () => {
      nearEndRef.current = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 48
      // Scrolling back to the end resumes following the turn.
      if (nearEndRef.current) followCurrentTurnRef.current = true
    }
    scroller.addEventListener('scroll', onScroll, { passive: true })
    const stick = () => {
      if (!followCurrentTurnRef.current || !nearEndRef.current) return
      settleToEnd(scroller)
    }
    const observer = new ResizeObserver(stick)
    observer.observe(scroller)
    const list = scroller.querySelector('[data-testid="virtuoso-item-list"]')
    if (list) observer.observe(list)
    return () => { observer.disconnect(); scroller.removeEventListener('scroll', onScroll) }
  }, [autoScrollMode])

  // Video Studio presents long-form creative work where a reader often starts
  // examining the first lines while the agent is still writing. Reveal that
  // first assistant text, then stop: continuously following every streamed
  // chunk steals the reader's scroll position and makes the response hard to
  // inspect. AgentWorks keeps the existing full-turn follow behaviour above.
  useEffect(() => {
    if (autoScrollMode !== 'reveal-first-response') return
    let userMessageFrame: number | undefined
    let userMessageSettledLayoutTimer: number | undefined
    const isInitialTranscript = !initializedFirstResponseRevealRef.current
    let shouldRevealUserMessage = false
    const isNewUserMessage = Boolean(
      !isInitialTranscript &&
      latestUserMessageKey &&
      firstResponseUserMessageKeyRef.current !== latestUserMessageKey,
    )
    if (isInitialTranscript) {
      initializedFirstResponseRevealRef.current = true
      firstResponseUserMessageKeyRef.current = latestUserMessageKey
      // A page reload can reconnect while the agent is already thinking. Arm
      // that live turn without treating a completed, restored conversation as
      // a new response that should steal the reader's position.
      // A brand-new conversation mounts only after its optimistic first user
      // row exists. That row is not history: it needs the same one-time
      // reveal as any later sent message once the response begins.
      const isOnlyPendingInitialUserMessage = Boolean(
        latestUserMessageKey && items.length === 1 && !assistantResponseAfterLatestUser,
      )
      revealFirstResponseRef.current = Boolean(
        latestUserMessageKey && !assistantResponseAfterLatestUser && (
          streamingText.trim() || streamingStatus.trim() || isOnlyPendingInitialUserMessage
        ),
      )
      shouldRevealUserMessage = isOnlyPendingInitialUserMessage
    } else if (isNewUserMessage) {
      firstResponseUserMessageKeyRef.current = latestUserMessageKey
      revealFirstResponseRef.current = true
      shouldRevealUserMessage = true
    }
    if (shouldRevealUserMessage) {
      // Video Studio deliberately does not follow every streamed token, but a
      // sent message must still move into view. ChatArea's legacy scroller is
      // outside this Virtuoso instance, so it cannot do this for product
      // transcripts. Reveal the new user row once; the existing branch below
      // will reveal the first assistant text once it arrives.
      const revealGeneration = ++userRevealGenerationRef.current
      const revealUserMessage = () => {
        if (userRevealGenerationRef.current !== revealGeneration) return
        virtuosoRef.current?.scrollToIndex({
          index: Math.max(0, items.length - 1),
          align: 'end',
          behavior: 'auto',
        })
      }
      // This must happen synchronously in the committed render. Streaming
      // status can update before the next animation frame; if scrolling lives
      // only in rAF, effect cleanup cancels it and the newly sent message stays
      // hidden above the fixed composer.
      revealUserMessage()
      userMessageFrame = window.requestAnimationFrame(revealUserMessage)
      userMessageSettledLayoutTimer = window.setTimeout(revealUserMessage, 160)
      // Do not cancel this final retry on the first status/text update. That
      // update is exactly what previously cancelled the only scheduled scroll
      // before Virtuoso had measured the new live row.
      window.setTimeout(revealUserMessage, 420)
    }

    const assistantHasBegun = Boolean(streamingText.trim()) || assistantResponseAfterLatestUser
    if (!revealFirstResponseRef.current || !assistantHasBegun) {
      return () => {
        if (userMessageFrame !== undefined) window.cancelAnimationFrame(userMessageFrame)
        if (userMessageSettledLayoutTimer !== undefined) window.clearTimeout(userMessageSettledLayoutTimer)
      }
    }

    revealFirstResponseRef.current = false
    const targetIndex = Math.max(0, (streamingText || streamingStatus) ? items.length : items.length - 1)
    // Reveal the opening of the assistant response once, keeping it just above
    // the composer. `align: start` pulled the whole transcript upward as soon
    // as the first streamed text arrived, which looked like a small jump right
    // after sending. At this point the live row is still short; end alignment
    // keeps both the sent message and the first reply in a stable position.
    const reveal = () => {
      virtuosoRef.current?.scrollToIndex({ index: targetIndex, align: 'end', behavior: 'auto' })
    }
    // Reveal now as well as after layout settles. The first SSE status/text
    // update can otherwise clean up the scheduled rAF before it has a chance
    // to run, which is exactly why the first response appeared below the
    // composer after sending a message.
    reveal()
    const frame = window.requestAnimationFrame(reveal)
    const settledLayoutTimer = window.setTimeout(reveal, 160)
    return () => {
      if (userMessageFrame !== undefined) window.cancelAnimationFrame(userMessageFrame)
      if (userMessageSettledLayoutTimer !== undefined) window.clearTimeout(userMessageSettledLayoutTimer)
      window.cancelAnimationFrame(frame)
      window.clearTimeout(settledLayoutTimer)
    }
  }, [assistantResponseAfterLatestUser, autoScrollMode, items.length, latestUserMessageKey, streamingStatus, streamingText])

  // Keep the active work visible at the important boundaries without stealing
  // the reader's position for every streaming update. This is separate from
  // the first-response reveal above because tool cards and durable completion
  // rows are normal transcript items, not live streaming text.
  useEffect(() => {
    if (autoScrollMode !== 'reveal-first-response') return

    const liveStreamActive = Boolean(streamingText.trim() || streamingStatus.trim())
    const tail = items[items.length - 1]
    const toolKey = tail?.kind === 'tools' ? tail.key : ''
    const assistantKey = tail?.kind === 'event' && assistantResponseText(tail.event)
      ? tail.key
      : ''

    if (!initializedActivityRevealRef.current) {
      initializedActivityRevealRef.current = true
      previousLiveStreamRef.current = liveStreamActive
      lastRevealedToolKeyRef.current = toolKey
      lastRevealedAssistantKeyRef.current = assistantKey
      return
    }

    const firstLiveChunk = liveStreamActive && !previousLiveStreamRef.current
    const newToolActivity = Boolean(toolKey && toolKey !== lastRevealedToolKeyRef.current)
    const newAssistantReply = Boolean(assistantKey && assistantKey !== lastRevealedAssistantKeyRef.current)
    previousLiveStreamRef.current = liveStreamActive
    if (toolKey) lastRevealedToolKeyRef.current = toolKey
    if (assistantKey) lastRevealedAssistantKeyRef.current = assistantKey

    if (!firstLiveChunk && !newToolActivity && !newAssistantReply) return

    const targetIndex = liveStreamActive
      ? items.length
      : Math.max(0, items.length - 1)
    const revealBoundary = () => {
      virtuosoRef.current?.scrollToIndex({ index: targetIndex, align: 'end', behavior: 'auto' })
    }
    revealBoundary()
    const frame = window.requestAnimationFrame(revealBoundary)
    const settledLayoutTimer = window.setTimeout(revealBoundary, 160)
    return () => {
      window.cancelAnimationFrame(frame)
      window.clearTimeout(settledLayoutTimer)
    }
  }, [autoScrollMode, items, streamingStatus, streamingText])

  // Electron occasionally fails to route a physical wheel/trackpad gesture to
  // Virtuoso's internal scroller even though accessibility scroll actions work.
  // Forward the gesture explicitly. Nested scroll regions (expanded tool output)
  // keep first refusal while they can still move in the requested direction.
  const handleWheelCapture = useCallback((event: React.WheelEvent<HTMLDivElement>) => {
    const scroller = scrollerRef.current
    if (!(scroller instanceof HTMLElement) || event.deltaY === 0) return
    if (event.deltaY < 0) {
      followCurrentTurnRef.current = false
    }
    // A browser scrolls natively, with the trackpad's own inertia and
    // smoothing. Applying the wheel delta by hand there replaced that with
    // one hard step per event, which read as jitter. Only Electron needs the
    // manual forwarding below.
    if (!IS_ELECTRON) return

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
            <div className="text-sm font-medium text-foreground">{title}</div>
            <div className="mt-1 text-xs leading-5 text-muted-foreground">{detail}</div>
            {error && onRetry && (
              <button
                type="button"
                onClick={onRetry}
                className="mt-3 rounded border border-border px-2.5 py-1 text-xs text-foreground/90 hover:bg-muted"
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
            : 'border-border bg-muted/40 text-muted-foreground'
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
              className="mx-auto rounded px-2 py-0.5 text-muted-foreground hover:bg-muted hover:text-foreground disabled:cursor-wait disabled:opacity-60"
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
        data={listData}
        className="custom-scrollbar min-h-0 flex-1"
        scrollerRef={ref => { scrollerRef.current = ref }}
        rangeChanged={({ startIndex }) => {
          setIsAtTranscriptStart(startIndex === 0)
        }}
        followOutput={autoScrollMode === 'follow-turn' ? 'auto' : false}
        // Render well beyond the viewport so scrolling reveals rows that are
        // already there instead of rows popping in as they mount.
        increaseViewportBy={{ top: 1200, bottom: 600 }}
        initialTopMostItemIndex={initialTopMostItemIndex}
        computeItemKey={(_, item) => item.key}
        itemContent={(index, item) => {
          if (item.kind === 'live') return <LiveAssistantTranscript text={item.text} status={item.status} />
          const slot = turnSlots[index]
          const testId = item.kind === 'event' ? `terminal-clear-event-${item.event.id || item.key}` : undefined
          const body = item.kind === 'tools'
            ? <ToolBatch item={item} />
            : item.kind === 'thinking'
              ? <ThinkingBatch item={item} live={index === items.length - 1 && !streamingText.trim()} />
              : (
                <TranscriptEvent
                  event={item.event}
                  onSendMessage={onSendMessage}
                  compactUserBottom={listData[index + 1]?.kind === 'tools'}
                  inTurn={Boolean(slot?.agent)}
                  renderInteraction={renderInteraction}
                  assistantLabel={assistantLabel}
                  assistantIcon={assistantIcon}
                  hideTimestamp={!(slot?.showTime ?? true)}
                />
              )
          if (!slot?.agent) {
            return <div data-testid={testId} className="px-3 py-0.5">{body}</div>
          }
          // One block per agent turn: the header once at the top, then every
          // tool batch, thought and reply of that turn on the same rail.
          return (
            <div data-testid={testId} className="px-3">
              <div className={`${AGENT_BLOCK_CLASS} ${slot.first ? 'mt-4' : ''} ${slot.last ? 'mb-2' : ''}`}>
                {slot.first && slot.header && <AssistantTurnHeader event={slot.header} timestamp={slot.showTime ? transcriptTimestamp(slot.header) : ''} label={assistantLabel} icon={assistantIcon} />}
                {body}
              </div>
            </div>
          )
        }}
      />
    </div>
  )
}

const LiveAssistantTranscript: React.FC<{ text: string; status: string }> = ({ text, status }) => (
  text ? (
    <article data-testid="terminal-clear-live-assistant-message" className="mx-3 mt-4 mb-2 pl-3 pr-1">
      <div className="[&_li]:!text-[length:calc(14px*var(--chat-scale,1))] [&_p]:!text-[length:calc(14px*var(--chat-scale,1))] [&_li]:!leading-[calc(24px*var(--chat-scale,1))] [&_p]:!leading-[calc(24px*var(--chat-scale,1))]">
        <ConversationMarkdownRenderer content={text} framed={false} maxHeight="none" />
      </div>
      <span aria-label="Writing" className="mt-1 inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-cyan-400" />
    </article>
  ) : status ? (
    // Virtuoso receives this row whenever the backend has tool/status progress
    // but no assistant prose yet. Returning null made it a zero-height item,
    // which breaks its measurements and leaves new user messages off-screen.
    // Keep it visually quiet while giving the virtual list a real anchor for
    // the one-time tool/status scroll.
    <div aria-hidden="true" className="h-px" data-testid="terminal-live-status-anchor" />
  ) : null
)

// Memoized: the parent re-renders on every terminal poll, and re-rendering the
// whole transcript each time would defeat EventDispatcher's own memoization.
export const TerminalEventTranscript = memo(TerminalEventTranscriptInner)
