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

  const mark = pair.status === 'error' ? '✗' : pair.status === 'ok' ? '✓' : '⋯'
  const markClass =
    pair.status === 'error' ? 'text-red-400' : pair.status === 'ok' ? 'text-emerald-400' : 'text-neutral-500'
  const duration =
    pair.durationMs != null && pair.durationMs > 0
      ? pair.durationMs >= 1000
        ? `${(pair.durationMs / 1000).toFixed(1)}s`
        : `${pair.durationMs}ms`
      : null

  return (
    <div data-testid="terminal-clear-tool-call" className="rounded border border-neutral-800 bg-neutral-900/40">
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
  const isLong = value.length > PREVIEW_LIMIT
  const shown = full || !isLong ? value : `${value.slice(0, PREVIEW_LIMIT)}…`
  return (
    <div className="min-w-0">
      <div className="mb-0.5 text-[10px] font-medium uppercase tracking-wide text-neutral-500">{label}</div>
      <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded bg-black/30 p-2 text-[11px] leading-5 text-neutral-300">
        {shown}
      </pre>
      {isLong && (
        <button
          type="button"
          onClick={() => setFull(prev => !prev)}
          className="mt-1 text-[10px] text-neutral-500 hover:text-neutral-300"
        >
          {full ? 'Show less' : `Show all (${value.length.toLocaleString()} chars)`}
        </button>
      )}
    </div>
  )
}

const ToolBatch: React.FC<{ item: Extract<TranscriptItem, { kind: 'tools' }> }> = ({ item }) => {
  const [expanded, setExpanded] = useState(false)
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
}

const TerminalEventTranscriptInner: React.FC<TerminalEventTranscriptProps> = ({
  events,
  terminal,
  siblingTerminals,
  onSendMessage,
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
    const Icon = failed ? XCircle : completed ? CheckCircle2 : CircleDashed
    const title = failed
      ? 'This agent did not finish.'
      : completed
        ? 'This agent completed.'
        : 'Waiting for this agent to begin.'
    const detail = completed || failed
      ? 'Conversation details are not available for this retained run.'
      : 'Its conversation will appear here when the first event arrives.'
    return (
      <div
        data-testid="terminal-clear-view-empty"
        className="flex min-w-0 flex-1 items-center justify-center overflow-y-auto bg-[#0b0d0c] px-5 py-8"
      >
        <div className="flex max-w-md items-start gap-3 text-left">
          <Icon
            className={`mt-0.5 h-5 w-5 shrink-0 ${
              failed ? 'text-red-400' : completed ? 'text-emerald-400' : 'animate-spin text-cyan-400'
            }`}
          />
          <div>
            <div className="text-sm font-medium text-neutral-200">{title}</div>
            <div className="mt-1 text-xs leading-5 text-neutral-500">{detail}</div>
          </div>
        </div>
      </div>
    )
  }

  return (
    <div data-testid="terminal-clear-view" className="min-w-0 flex-1 overflow-hidden bg-[#0b0d0c]">
      {/* Virtualized: the tree inherited this from EventHierarchy. A flat list
          that rendered every event would regress long sessions badly. */}
      <Virtuoso
        data={items}
        className="h-full"
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
