import { getOwnedTerminalOwnerKeys, getTerminalOwnerPayload } from './eventOwnership'
import { isMainAgentTerminal } from './terminalIdentity'
import type { PollingEvent, TerminalSnapshot } from '../services/api-types'

// Pure selection/grouping logic for the terminal clean view.
//
// Deliberately free of React and component imports so it can be tested on its
// own: these two functions decide WHICH conversation a terminal shows and how
// tool runs collapse, and both fail silently if wrong (you get a plausible but
// incorrect transcript rather than an error).

// The transcript is meant to read as the conversation actually happened:
//
//   user → tools → assistant → pre-validation → auto-notification
//
// Cost, context usage, and provider status snapshots live in the terminal
// footer. They are not part of the conversation, so keep them out of Clear
// View. A long reviewer can emit dozens of status_line updates; rendering each
// one as a card pushes the actual finding off screen.
const NON_TRANSCRIPT_TYPES = new Set(['token_usage', 'status_line'])

// A full run is a CONTAINER, not an agent: it has no conversation of its own,
// only the steps beneath it. The backend already declares this
// (ExecutionKindFullRun) and keeps it out of the terminal rail for exactly that
// reason, but its lifecycle card still rendered at the top of the transcript --
// a "Full Run [Group / Iteration 0]" row restating the panel header above it
// before the first thing that actually happened.
const CONTAINER_EXECUTION_KINDS = new Set(['full_run'])
const TOOL_CALL_TYPES = new Set([
  'tool_call_start',
  'tool_call_end',
  'tool_call_error',
])

// Absorbed into an adjacent tool batch so they don't split one in half, but
// never worth a row of their own. delegation_* interleave between a
// tool_call_start/end pair in workflow and multi-agent runs.
//
// llm_generation_end is only a bridge when it is EMPTY. It is also the event
// that carries the assistant's final answer, and nothing else in the terminal
// transcript renders that text: EventDispatcher has no streaming_chunk case,
// so chunks draw nothing here. Treating it as an unconditional bridge meant a
// tool-heavy turn -- a CDP browser step ends on a tool call, then answers --
// swept the answer into the tool batch, where pairToolCalls emits only
// tool rows and drops it. The turn rendered with no response at all.
const BATCH_BRIDGE_TYPES = new Set(['delegation_start', 'delegation_end'])

// An llm_generation_end with text must never be absorbed; an empty one still
// behaves as a bridge so it doesn't split a tool batch in half.
function isEmptyGenerationEnd(event: PollingEvent): boolean {
  return textField(eventFields(event).content).trim().length === 0
}

// A lifecycle start is useful while an agent is running. Once the matching
// completion/failure arrives, rendering both is just duplicate information:
// the retained "Running" card makes a finished task look active next to its
// actual result. Keep the terminal transcript to one lifecycle card per agent.
const LIFECYCLE_EVENT_FAMILIES: Record<string, { start?: boolean; terminal?: boolean; family: string }> = {
  agent_start: { start: true, family: 'agent' },
  agent_end: { terminal: true, family: 'agent' },
  agent_error: { terminal: true, family: 'agent' },
  orchestrator_agent_start: { start: true, family: 'orchestrator-agent' },
  orchestrator_agent_end: { terminal: true, family: 'orchestrator-agent' },
  orchestrator_agent_error: { terminal: true, family: 'orchestrator-agent' },
  background_agent_started: { start: true, family: 'background-agent' },
  background_agent_completed: { terminal: true, family: 'background-agent' },
  background_agent_failed: { terminal: true, family: 'background-agent' },
  background_agent_terminated: { terminal: true, family: 'background-agent' },
}

function isFullRunContainerEvent(event: PollingEvent): boolean {
  const fields = eventFields(event)
  const kind = textField(event.execution_kind) || textField(fields.execution_kind)
  if (CONTAINER_EXECUTION_KINDS.has(kind)) return true

  // Compatibility for events recorded before completion events preserved the
  // creator-declared full_run kind. Their stable workflow-full-* execution id
  // still identifies the orchestration container unambiguously.
  const identity = textField(event.execution_id) ||
    textField(fields.execution_id) ||
    textField(fields.agent_id)
  return identity.startsWith('workflow-full-')
}

function isContainerTranscriptNoise(event: PollingEvent): boolean {
  if (!isFullRunContainerEvent(event)) return false
  return LIFECYCLE_EVENT_FAMILIES[event.type || ''] !== undefined ||
    event.type === 'auto_notification_steered'
}

// Message-sequence items are announced twice: the generic background runner
// emits a blue/green lifecycle banner, then the delegated step emits the real
// task/result card. The generic rows add no content ("Running" / "completed")
// and remain visually loud after the useful card has appeared. Keep failures
// and cancellations visible, but remove the redundant happy-path wrapper.
const MESSAGE_SEQUENCE_WRAPPER_TYPES = new Set([
  'background_agent_started',
  'background_agent_completed',
])

function isMessageSequenceWrapperEvent(event: PollingEvent): boolean {
  if (!MESSAGE_SEQUENCE_WRAPPER_TYPES.has(event.type || '')) return false
  const fields = eventFields(event)
  const metadata = fields.metadata && typeof fields.metadata === 'object'
    ? fields.metadata as Record<string, unknown>
    : undefined
  const name = textField(fields.name).toLowerCase()
  const agentID = textField(fields.agent_id) || textField(metadata?.agent_id)
  return name.startsWith('message sequence item') ||
    agentID.toLowerCase().startsWith('msgseq-') ||
    metadata?.message_sequence_item === true ||
    metadata?.message_sequence_item === 'true'
}

function eventFields(event: PollingEvent): Record<string, unknown> {
  const outer = event.data
  if (!outer || typeof outer !== 'object') return {}
  const nested = (outer as { data?: unknown }).data
  return nested && typeof nested === 'object'
    ? nested as Record<string, unknown>
    : outer as Record<string, unknown>
}

function textField(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

function normalizedLifecycleName(fields: Record<string, unknown>): string {
  return (textField(fields.agent_name) || textField(fields.name))
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

function lifecycleFamily(event: PollingEvent): string {
  return LIFECYCLE_EVENT_FAMILIES[event.type || '']?.family || ''
}

function lifecycleExecutionID(event: PollingEvent): string {
  const fields = eventFields(event)
  const metadata = fields.metadata && typeof fields.metadata === 'object'
    ? fields.metadata as Record<string, unknown>
    : undefined

  // Lifecycle events for one message-sequence item do not always agree on
  // the outer execution_id. The generic background event carries the item's
  // agent_id directly, while the richer orchestrator event can retain the
  // parent workflow-step execution_id and put that same agent_id in metadata.
  // The agent id is the identity of the lifecycle being announced; the outer
  // execution id is only its container and remains the legacy fallback.
  return textField(fields.agent_id) ||
    textField(metadata?.agent_id) ||
    textField(event.execution_id) ||
    textField(fields.execution_id)
}

// The server and the delegated agent both announce the SAME execution, and they
// decorate the name differently at each end -- "Pulse reviewer: pulse 2026 07 27
// bug review" against "Background: Pulse reviewer - pulse-2026-07-27-bug-review".
// Including the name in the dedupe key therefore made one reviewer render as two
// agents. Normalising those strings is a losing game (they differ by word order,
// separators and prefix), so identity comes from the execution id instead.
//
// The name still has a job: sibling agents can legitimately share one execution
// id -- an eval step emits "...route-val-2" and "...route-val-3" under the same
// id -- and those must stay apart. The distinguishing signal is whether a single
// family emits more than one start for that execution: aliases arrive one per
// family, real siblings repeat within a family. Measured across 1101 executions
// in stored history, exactly 2 fall in the latter group.
function aliasCollapsibleExecutions(events: PollingEvent[]): Set<string> {
  const perExecutionFamilyCounts = new Map<string, Map<string, number>>()
  for (const event of events) {
    const descriptor = LIFECYCLE_EVENT_FAMILIES[event.type || '']
    if (!descriptor?.start) continue
    const executionID = lifecycleExecutionID(event)
    if (!executionID) continue
    const families = perExecutionFamilyCounts.get(executionID) || new Map<string, number>()
    const family = lifecycleFamily(event)
    families.set(family, (families.get(family) || 0) + 1)
    perExecutionFamilyCounts.set(executionID, families)
  }

  const collapsible = new Set<string>()
  for (const [executionID, families] of perExecutionFamilyCounts) {
    let repeats = false
    for (const count of families.values()) {
      if (count > 1) repeats = true
    }
    if (!repeats) collapsible.add(executionID)
  }
  return collapsible
}

function lifecycleKey(event: PollingEvent, aliasExecutions?: Set<string>): string | null {
  const descriptor = LIFECYCLE_EVENT_FAMILIES[event.type || '']
  if (!descriptor) return null
  const fields = eventFields(event)
  const name = normalizedLifecycleName(fields)

  // The server emits a generic background lifecycle event and the delegated
  // LLM emits an orchestrator lifecycle event for the same execution. Treat
  // those as aliases, not two agents. The execution id is shared across both
  // event families; the name protects sequence items that share a parent
  // execution from collapsing into one another.
  const executionID = lifecycleExecutionID(event)
  if (executionID) {
    // Alias case: one start per family for this execution, so the differing
    // names are two labels for the same agent.
    if (aliasExecutions?.has(executionID)) return `execution:${executionID}`
    return `execution:${executionID}:${name}`
  }

  // Correlation is shared by a start/end pair. Include the agent identity so
  // sibling agents that happen to share the same workflow correlation cannot
  // hide one another's live card.
  const correlation = textField(event.correlation_id) || textField(fields.correlation_id)
  if (correlation && name) return `correlation:${correlation}:${name}`
  return null
}

function lifecycleStartRichness(event: PollingEvent): number {
  const fields = eventFields(event)
  let score = event.type === 'orchestrator_agent_start' ? 10 : 0
  if (textField(fields.provider)) score += 4
  if (textField(fields.model_id)) score += 4
  if (textField(fields.agent_type)) score += 2
  if (fields.input_data && typeof fields.input_data === 'object') score += 1
  if (textField(fields.objective) || textField(fields.instruction)) score += 1
  return score
}

export function collapseCompletedLifecycleStarts(events: PollingEvent[]): PollingEvent[] {
  const openStarts = new Map<string, PollingEvent>()
  const hiddenStarts = new Set<PollingEvent>()
  const aliasExecutions = aliasCollapsibleExecutions(events)

  for (const event of events) {
    const descriptor = LIFECYCLE_EVENT_FAMILIES[event.type || '']
    const key = lifecycleKey(event, aliasExecutions)
    if (!descriptor || !key) continue
    if (descriptor.start) {
      const current = openStarts.get(key)
      if (!current) {
        openStarts.set(key, event)
        continue
      }

      // Prefer the delegated agent's enriched start payload over the generic
      // background notification. On equal detail, the latest event wins.
      if (lifecycleStartRichness(event) >= lifecycleStartRichness(current)) {
        hiddenStarts.add(current)
        openStarts.set(key, event)
      } else {
        hiddenStarts.add(event)
      }
      continue
    }
    if (descriptor.terminal) {
      const start = openStarts.get(key)
      if (start) {
        hiddenStarts.add(start)
        openStarts.delete(key)
      }
    }
  }

  // Only ONE completion card per execution. Starts were already collapsed
  // above, but terminal events never were, and they arrive in bulk: the server
  // reports the background execution finishing, the delegated agent reports its
  // own agent_end, and a retried step can emit several orchestrator_agent_end
  // events for the same work. Measured over stored history, 585 of 1562
  // executions emit more than one -- which is why a finished reviewer showed
  // "Agent Completed" and "Pulse Reviewer: ... completed (2m46s)" stacked.
  //
  // The last one wins rather than the first: a failure reported after an end
  // is the outcome that actually held, and the newest card carries the final
  // duration.
  const lastTerminalByKey = new Map<string, PollingEvent>()
  for (const event of events) {
    const descriptor = LIFECYCLE_EVENT_FAMILIES[event.type || '']
    if (!descriptor?.terminal) continue
    const key = lifecycleKey(event, aliasExecutions)
    if (!key) continue
    lastTerminalByKey.set(key, event)
  }
  const supersededTerminals = new Set<PollingEvent>()
  for (const event of events) {
    const descriptor = LIFECYCLE_EVENT_FAMILIES[event.type || '']
    if (!descriptor?.terminal) continue
    const key = lifecycleKey(event, aliasExecutions)
    if (!key) continue
    if (lastTerminalByKey.get(key) !== event) supersededTerminals.add(event)
  }

  return events.filter(event => !hiddenStarts.has(event) && !supersededTerminals.has(event))
}

// The lifecycle-start event type that can carry the SAME content a following
// "execution_prompt" user_message duplicates, keyed by which field holds it —
// orchestrator_agent_start uses user_message (base_orchestrator_agent.go),
// background_agent_started uses instruction (BackgroundAgentStartedEvent).
const KICKOFF_CONTENT_FIELD: Record<string, string> = {
  orchestrator_agent_start: 'user_message',
  background_agent_started: 'instruction',
}

function metadataField(fields: Record<string, unknown>, key: string): string {
  const metadata = fields.metadata
  if (!metadata || typeof metadata !== 'object') return ''
  return textField((metadata as Record<string, unknown>)[key])
}

/**
 * A step-scoped, turn-zero user_message tagged "execution_prompt" by the
 * backend's context-aware bridge (context_aware_bridge.go) is the prompt that
 * kicked an execution off. OrchestratorAgentStartEventDisplay now renders
 * that same content inline, always visible, as its Task section — so once a
 * lifecycle-start event with non-empty content has been seen in THIS
 * terminal's own scope, a following execution_prompt message is provably a
 * duplicate and is dropped.
 *
 * Deliberately conservative: the execution_prompt tag is applied broadly to
 * any step-scoped turn-zero message (context_aware_bridge.go), including
 * execution kinds whose own start event does not carry the content — for
 * those, no preceding start event will have non-empty content, so this never
 * drops anything. Never guess from the tag alone.
 */
function dropDuplicateExecutionPromptMessages(events: PollingEvent[]): PollingEvent[] {
  let precedingStartHasContent = false
  return events.filter(event => {
    const contentField = KICKOFF_CONTENT_FIELD[event.type || '']
    if (contentField) {
      precedingStartHasContent = Boolean(textField(eventFields(event)[contentField]))
      return true
    }
    if (event.type === 'user_message' && precedingStartHasContent) {
      const fields = eventFields(event)
      if (metadataField(fields, 'source') === 'execution_prompt') return false
    }
    return true
  })
}

function isTranscriptEvent(event: PollingEvent): boolean {
  if (NON_TRANSCRIPT_TYPES.has(event.type || '')) return false
  if (isContainerTranscriptNoise(event)) return false
  if (isMessageSequenceWrapperEvent(event)) return false
  return true
}

function isToolBatchEvent(event: PollingEvent): boolean {
  const type = event.type || ''
  if (type === 'llm_generation_end') return isEmptyGenerationEnd(event)
  return TOOL_CALL_TYPES.has(type) || BATCH_BRIDGE_TYPES.has(type)
}

// A batch is only worth collapsing if it contains real tool calls — a stray
// token_usage between two messages should stay inline, not become a control
// that hides nothing.
function countRealToolCalls(events: PollingEvent[]): number {
  return events.reduce((n, event) => (event.type === 'tool_call_start' ? n + 1 : n), 0)
}

export function toolBatchLabel(events: PollingEvent[]): string {
  const names = new Set<string>()
  for (const event of events) {
    if (event.type !== 'tool_call_start') continue
    const data = event.data as Record<string, unknown> | undefined
    const payload = (data?.data as Record<string, unknown>) || data
    const name = payload?.tool_name ?? payload?.name
    if (typeof name === 'string' && name.trim()) names.add(name.trim())
  }
  const list = Array.from(names)
  if (list.length === 0) return ''
  if (list.length <= 2) return list.join(', ')
  return `${list.slice(0, 2).join(', ')} +${list.length - 2} more`
}

export type TranscriptItem =
  | { kind: 'event'; key: string; event: PollingEvent }
  | { kind: 'tools'; key: string; events: PollingEvent[]; toolCount: number }

/**
 * selectTerminalEvents scopes the session's event stream to ONE terminal.
 *
 * The rail is the hierarchy now: every agent and sub-agent owns its own
 * terminal entry, so this is the ONLY place that decides which conversation a
 * terminal shows. Deliberately reuses getOwnedTerminalOwnerKeys /
 * getTerminalOwnerPayload — the SAME functions the (now-removed) tree used to
 * decide which owned terminal's internal log panel an event streamed into —
 * rather than re-deriving a narrower subset of that matching. An event's
 * owner id can live in any of ~20 different fields depending on event type
 * (execution_id, delegation_id, background_agent_id, agent_id, several
 * metadata variants, workflow-step composite ids, ...); that enumeration
 * already exists once and callers must not re-narrow it.
 *
 * Two cases, because "main" is not an owned id (getOwnedTerminalOwnerKeys
 * deliberately excludes anything starting with "main:"):
 *
 *   - Owned terminal (a workflow step, message-sequence item, delegation,
 *     background agent, ...): an event belongs to it iff its owner keys
 *     include this terminal's terminal_id (== `${session_id}:${owner_id}`,
 *     confirmed against the backend's own construction of that id).
 *   - Main-agent terminal: an event belongs to it iff it is in the same
 *     session and NOT claimed by any sibling owned terminal. `siblingTerminals`
 *     should be the full terminal list for the session — omit it only when
 *     that list is genuinely unavailable; the fallback below is deliberately
 *     permissive (shows too much rather than silently hiding a live turn).
 */
// A terminal's own opening lifecycle card duplicates its panel header: the
// header already names this agent (title, "Sub-agent", state), so a
// background_agent_started/orchestrator_agent_start card whose execution_id
// IS this terminal's own execution_id adds nothing above what the reader
// already knows just from having opened this terminal -- it is the same
// bug as the earlier "Full Run" container row, just for a plain sub-agent
// instead of a full_run: "Review artifact drift review" (header) followed
// immediately by "Review Artifact Drift Review" (its own start card).
//
// Only the START half is dropped. The matching completion card is kept: it
// carries genuinely new information the header does not show (status,
// duration), where the start card only repeats the name.
//
// A CHILD's start card is never touched -- a child has its own distinct
// execution_id, so this only ever matches the terminal's own root event.
function isOwnTerminalLifecycleStart(event: PollingEvent, terminal: TerminalSnapshot): boolean {
  const descriptor = LIFECYCLE_EVENT_FAMILIES[event.type || '']
  if (!descriptor?.start) return false
  const terminalExecutionId = (terminal.execution_id || '').trim()
  return Boolean(terminalExecutionId) && lifecycleExecutionID(event) === terminalExecutionId
}

export function selectTerminalEvents(
  events: PollingEvent[] | undefined,
  terminal: TerminalSnapshot | null | undefined,
  siblingTerminals?: TerminalSnapshot[],
): PollingEvent[] {
  if (!events || events.length === 0 || !terminal) return []
  const sessionId = terminal.session_id?.trim()
  if (!sessionId) return []

  const eventOwnerKeys = (event: PollingEvent): string[] =>
    getOwnedTerminalOwnerKeys(event, getTerminalOwnerPayload(event))

  let matched: PollingEvent[]

  if (isMainAgentTerminal(terminal)) {
    const ownedTerminalIds = new Set(
      (siblingTerminals ?? [])
        .filter(t => t.session_id === sessionId && t.terminal_id !== terminal.terminal_id && !isMainAgentTerminal(t))
        .map(t => t.terminal_id),
    )
    matched = events.filter(event => {
      if (event.session_id?.trim() !== sessionId) return false
      if (ownedTerminalIds.size === 0) return true // no sibling list: don't hide anything
      return !eventOwnerKeys(event).some(key => ownedTerminalIds.has(key))
    })
  } else {
    const terminalKey = terminal.terminal_id?.trim()
    if (!terminalKey) return []
    // A retained workflow execution root and its concrete evaluation/step
    // transcripts share an execution_id. The root is an orchestrator summary,
    // not a second copy of every child conversation. Exclude events claimed by
    // those concrete child terminals so each rail entry owns one transcript.
    const childTerminalIds = new Set(
      (siblingTerminals ?? [])
        .filter(candidate => (
          candidate.session_id === sessionId &&
          candidate.terminal_id !== terminalKey &&
          Boolean(terminal.execution_id) &&
          candidate.execution_id === terminal.execution_id &&
          Boolean(candidate.step_id) &&
          candidate.terminal_id.includes(':workflow-step:')
        ))
        .map(candidate => candidate.terminal_id),
    )
    matched = events.filter(event => {
      const ownerKeys = eventOwnerKeys(event)
      return ownerKeys.includes(terminalKey) &&
        !ownerKeys.some(key => childTerminalIds.has(key))
    })
  }

  // Stable chronological order. Out-of-order arrivals (retries, batched
  // flushes) would otherwise render in arrival order and read as scrambled.
  return matched
    .filter(isTranscriptEvent)
    .filter(event => !isOwnTerminalLifecycleStart(event, terminal))
    .map((event, index) => ({ event, index }))
    .sort((a, b) => {
      const at = Date.parse(a.event.timestamp || '')
      const bt = Date.parse(b.event.timestamp || '')
      if (Number.isFinite(at) && Number.isFinite(bt) && at !== bt) return at - bt
      return a.index - b.index // stable for equal/unparseable timestamps
    })
    .map(entry => entry.event)
}

/**
 * buildTranscriptItems flattens events into render items, folding consecutive
 * tool-call runs into one collapsible group.
 *
 * Collapsing is not cosmetic: the tree relied on auto-collapse to keep rendered
 * node counts down, and a flat list has no equivalent. This replaces it.
 */
// Events that render a completion card carrying the turn's answer.
const COMPLETION_ANSWER_TYPES = new Set([
  'unified_completion',
  'agent_end',
  'orchestrator_agent_end',
  'background_agent_completed',
])

function answerText(event: PollingEvent): string {
  const fields = eventFields(event)
  return textField(fields.content) || textField(fields.final_result) || textField(fields.result)
}

function comparableAnswer(text: string): string {
  return text.replace(/\s+/g, ' ').trim().toLowerCase()
}

// llm_generation_end has to render its answer when nothing else will -- that is
// what stopped tool-heavy turns (a CDP browser step) from showing no response
// at all. But a completion card usually repeats the very same text, and then
// the reader sees the answer twice in a row. Keep the completion card, which
// also carries the outcome, and drop the generation-end copy.
//
// Compared on normalised text with containment rather than equality: the two
// carriers legitimately differ at the edges (a completion card may prepend a
// status line, or hold prose from before a trailing tool call), and the point
// is only to detect that the reader is being shown the same answer twice.
function dropAnswersRepeatedByCompletionCard(events: PollingEvent[]): PollingEvent[] {
  const completionAnswers: string[] = []
  for (const event of events) {
    if (!COMPLETION_ANSWER_TYPES.has(event.type || '')) continue
    const text = comparableAnswer(answerText(event))
    if (text) completionAnswers.push(text)
  }
  if (completionAnswers.length === 0) return events

  return events.filter(event => {
    if ((event.type || '') !== 'llm_generation_end') return true
    const text = comparableAnswer(answerText(event))
    // Too short to judge: a handful of words can coincide without being the
    // same answer, so keep it rather than risk hiding a real reply.
    if (text.length < 24) return true
    return !completionAnswers.some(done => done.includes(text) || text.includes(done))
  })
}

export function buildTranscriptItems(events: PollingEvent[]): TranscriptItem[] {
  // Filter wrapper noise before lifecycle deduplication. Otherwise a generic
  // completion can supersede the richer delegated completion and then be
  // removed itself, accidentally hiding both records.
  const transcriptEvents = events.filter(isTranscriptEvent)
  const visibleEvents = dropAnswersRepeatedByCompletionCard(
    dropDuplicateExecutionPromptMessages(collapseCompletedLifecycleStarts(transcriptEvents)),
  )
  const items: TranscriptItem[] = []
  let cursor = 0

  while (cursor < visibleEvents.length) {
    const event = visibleEvents[cursor]
    if (!isTranscriptEvent(event)) {
      cursor += 1
      continue
    }
    if (!isToolBatchEvent(event)) {
      items.push({ kind: 'event', key: event.id || `evt-${cursor}`, event })
      cursor += 1
      continue
    }

    const batch: PollingEvent[] = []
    while (cursor < visibleEvents.length && isToolBatchEvent(visibleEvents[cursor])) {
      batch.push(visibleEvents[cursor])
      cursor += 1
    }

    const toolCount = countRealToolCalls(batch)
    if (toolCount === 0) {
      for (const item of batch) {
        items.push({ kind: 'event', key: item.id || `evt-${items.length}`, event: item })
      }
      continue
    }

    items.push({
      kind: 'tools',
      key: batch[0].id || `tools-${items.length}`,
      events: batch,
      toolCount,
    })
  }

  return items
}

export interface PairedToolCall {
  key: string
  /** Friendly tool name — MCP's `mcp__<server>__<tool>` reduced to `<tool>`. */
  name: string
  /** MCP server that owns the tool, when the name encoded one. */
  server?: string
  /** start/end/error events for this call, in arrival order, for detail rendering. */
  events: PollingEvent[]
  status: 'running' | 'ok' | 'error'
  /**
   * Tool-call duration in NANOSECONDS, as Go's time.Duration serializes it.
   * Named for its real unit: this was previously `durationMs`, so the card divided
   * nanoseconds by 1000 and printed the microseconds with an `s` suffix — a 120ms
   * query rendered as "120193.5s".
   */
  durationNs?: number
  /** Arguments the model passed. Carried on the START event (tool_params). */
  args?: string
  /** What the tool returned. Carried on the END event. */
  result?: string
}

/**
 * mcpToolDisplayName splits MCP's wire name into its parts.
 *
 * A tool arrives as `mcp__api-bridge__agent_browser`. Rendered raw it is the
 * least readable thing on screen, and the double underscores read as damage.
 * The server is worth keeping — just not welded into the name.
 */
export function mcpToolDisplayName(raw: string): { name: string; server?: string } {
  const match = /^mcp__(.+?)__(.+)$/.exec(raw.trim())
  if (!match) return { name: raw.trim() }
  return { name: match[2], server: match[1] }
}

/** Arguments live under tool_params.arguments on the start event. */
function toolCallArgs(event: PollingEvent): string {
  const params = toolCallField(event, 'tool_params')
  if (params && typeof params === 'object') {
    return textField((params as Record<string, unknown>).arguments)
  }
  return ''
}

function toolCallField(event: PollingEvent, key: string): unknown {
  const fields = eventFields(event)
  return fields[key]
}

/**
 * pairToolCalls collapses the start/end (or start/error) events of one tool
 * call into a single item.
 *
 * The transcript previously rendered a "Tool Call Start" row and a "Tool Call
 * End" row for every call — two near-identical rows, neither showing what the
 * tool did, and the pair split apart whenever calls interleaved. A reader wants
 * one line per call: what ran, whether it worked, how long it took.
 *
 * Pairs on tool_call_id. Events without one (or an end with no matching start)
 * still get their own item, so nothing is ever dropped.
 */
export function pairToolCalls(events: PollingEvent[]): PairedToolCall[] {
  const out: PairedToolCall[] = []
  const byCallID = new Map<string, PairedToolCall>()

  for (const event of events) {
    const type = event.type || ''
    if (!TOOL_CALL_TYPES.has(type)) continue

    const callID = textField(toolCallField(event, 'tool_call_id'))
    const rawName = textField(toolCallField(event, 'tool_name'))
    const existing = callID ? byCallID.get(callID) : undefined

    if (!existing) {
      const { name, server } = mcpToolDisplayName(rawName)
      const item: PairedToolCall = {
        key: event.id || `${type}-${out.length}`,
        name: name || 'tool',
        server: server || textField(toolCallField(event, 'server_name')) || undefined,
        events: [event],
        status: type === 'tool_call_error' ? 'error' : type === 'tool_call_end' ? 'ok' : 'running',
      }
      const duration = toolCallField(event, 'duration')
      if (typeof duration === 'number') item.durationNs = duration
      const args = toolCallArgs(event)
      if (args) item.args = args
      const result = textField(toolCallField(event, 'result'))
      if (result) item.result = result
      out.push(item)
      if (callID) byCallID.set(callID, item)
      continue
    }

    existing.events.push(event)
    if (type === 'tool_call_error') existing.status = 'error'
    else if (type === 'tool_call_end' && existing.status !== 'error') existing.status = 'ok'
    const duration = toolCallField(event, 'duration')
    if (typeof duration === 'number') existing.durationNs = duration
    const args = toolCallArgs(event)
    if (args) existing.args = args
    const result = textField(toolCallField(event, 'result'))
    if (result) existing.result = result
    // A start may lack the name that the end carries (and vice versa).
    if (existing.name === 'tool' && rawName) {
      const { name, server } = mcpToolDisplayName(rawName)
      existing.name = name
      existing.server = existing.server || server
    }
  }

  return out
}
