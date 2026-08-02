import React, { memo, useCallback, useMemo, useState } from 'react'
import { Virtuoso } from 'react-virtuoso'
import { CheckCircle2, CircleDashed, XCircle } from 'lucide-react'
import { EventDispatcher } from './events/EventDispatcher'
import {
  buildTranscriptItems,
  pairToolCalls,
  type PairedToolCall,
  selectTerminalEvents,
  toolBatchLabel,
  type TranscriptItem,
} from '../utils/terminalEventTranscript'
import { formatDurationCompact } from '../utils/duration'
import { formatToolCallArguments, formatToolCallResult } from '../utils/toolCallFormatting'
import type { PollingEvent, TerminalSnapshot } from '../services/api-types'

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
        onClick={() => hasDetail && setOpen(prev => !prev)}
        aria-expanded={hasDetail ? open : undefined}
        disabled={!hasDetail}
        className={`flex w-full items-center gap-2 px-2 py-1.5 text-left text-xs ${
          hasDetail ? 'hover:bg-neutral-800/60' : 'cursor-default'
        }`}
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
        {hasDetail && <span className="ml-auto shrink-0 font-mono text-neutral-600">{open ? '▾' : '▸'}</span>}
      </button>

      {open && hasDetail && (
        <div className="space-y-2 border-t border-neutral-800 px-2 py-2">
          {pair.args && <ToolCallField label="Arguments" value={pair.args} />}
          {pair.result && <ToolCallField label="Result" value={pair.result} />}
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
    () => label === 'Result' ? formatToolCallResult(value) : formatToolCallArguments(value),
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
  // Open by default. This view exists to answer "what did this agent actually
  // do", and tool calls are the answer — collapsing them hides the substance
  // behind a count and makes every batch a click. It also hid failures: a batch
  // reading "6 tool calls" looks identical whether they succeeded or not, which
  // is how bridge errors stayed invisible even after the result formatter
  // learned to mark them.
  const [expanded, setExpanded] = useState(true)
  const label = useMemo(() => toolBatchLabel(item.events), [item.events])
  const pairs = useMemo(() => pairToolCalls(item.events), [item.events])
  const toggle = useCallback(() => setExpanded(prev => !prev), [])

  return (
    <div data-testid="terminal-clear-tool-batch" className="my-1">
      <button
        type="button"
        onClick={toggle}
        aria-expanded={expanded}
        data-testid="terminal-clear-tool-batch-toggle"
        className="flex w-full items-center gap-2 rounded px-2 py-1 text-left text-xs text-neutral-400 hover:bg-neutral-800/60 hover:text-neutral-200"
      >
        <span className="font-mono text-neutral-500">{expanded ? '▾' : '▸'}</span>
        <span>
          {item.toolCount} tool {item.toolCount === 1 ? 'call' : 'calls'}
        </span>
        {label && <span className="truncate text-neutral-500">· {label}</span>}
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
}) => {
  const scoped = useMemo(
    () => selectTerminalEvents(events, terminal, siblingTerminals),
    [events, terminal, siblingTerminals],
  )
  const items = useMemo(() => buildTranscriptItems(scoped), [scoped])

  if (items.length === 0) {
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
        className="flex min-w-0 flex-1 items-center justify-center overflow-y-auto bg-[#0b0d0c] px-5 py-8"
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
    <div data-testid="terminal-clear-view" className="flex min-w-0 flex-1 flex-col overflow-hidden bg-[#0b0d0c]">
      {(hasOlder || loadingOlder || error) && (
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
              onClick={onLoadOlder}
              disabled={!hasOlder || loadingOlder}
              className="mx-auto rounded px-2 py-0.5 text-neutral-400 hover:bg-neutral-800 hover:text-neutral-200 disabled:cursor-wait disabled:opacity-60"
            >
              {loadingOlder ? 'Loading earlier events…' : 'Load earlier events'}
            </button>
          )}
        </div>
      )}
      {/* Virtualized: the tree inherited this from EventHierarchy. A flat list
          that rendered every event would regress long sessions badly. */}
      <Virtuoso
        data={items}
        className="min-h-0 flex-1"
        followOutput="smooth"
        initialTopMostItemIndex={Math.max(0, items.length - 1)}
        computeItemKey={(_, item) => item.key}
        itemContent={(_, item) =>
          item.kind === 'tools' ? (
            <div className="px-3">
              <ToolBatch item={item} />
            </div>
          ) : (
            <div data-testid={`terminal-clear-event-${item.event.id || item.key}`} className="px-3 py-0.5">
              <EventDispatcher event={item.event} onSendMessage={onSendMessage} hideOrchestratorContext />
            </div>
          )
        }
      />
    </div>
  )
}

// Memoized: the parent re-renders on every terminal poll, and re-rendering the
// whole transcript each time would defeat EventDispatcher's own memoization.
export const TerminalEventTranscript = memo(TerminalEventTranscriptInner)
