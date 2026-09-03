// SparkQuill's view of the platform's session events: the shared session
// client (frontend/shared/session) does the transport, the foreground
// filter and the presentation parsing; what lives here is the product's
// own mapping from platform events to the preview events and TurnResult
// the SparkQuill UI expects.
import type { ToolCallRecord } from '../../stores/types'
import type { ToolEvent, TurnResult, TurnStreamEvent } from '../familyApi'
import type { StoredMsg } from '../../stores/types'
import { appendStreamingText, eventBelongsToSession, isForegroundSessionEvent, looksLikeTerminalScreenText, mcpToolDisplayName, parsePresentationUpdatedEvent, splitStreamingStatusAndText } from '../../../../shared/session'
import type { PollingEvent, SSEEventMessage } from '../../../../shared/session'

/** The shapes the UI code below reads; aliases of the shared contract. */
export type PlatformEvent = PollingEvent
export type EventBatch = SSEEventMessage

/**
 * A coding-agent turn's whole assistant text (narration before tool calls
 * included), recorded by the provider in the generation metadata. The
 * conversation shows this; `final_result` stays the last message only, the
 * way workflows consume it.
 */
export function assistantTurnText(p: Record<string, unknown>): string {
  const meta = p.metadata as Record<string, unknown> | undefined
  const turn = meta?.assistant_turn_text
  return typeof turn === 'string' ? turn.trim() : ''
}

/**
 * The stored conversation a restored event stream describes: user turns, one
 * assistant message per turn (the recorded turn text, else every restored
 * assistant update joined, else final_result), and the product cards that
 * survive a reload. Used for history after a reload or a server restart, so
 * it must agree with what the live turn showed.
 */
export function messagesFromEvents(events: PlatformEvent[], sessionID: string): StoredMsg[] {
  const messages: StoredMsg[] = []
  let turnText = ''
  let pieces: string[] = []
  for (const e of events) {
    const type = e.type ?? e.data?.type
    const p = payloadOf(e)
    if (type === 'user_message' && typeof p.content === 'string') {
      messages.push({ role: 'user', text: p.content })
      turnText = ''; pieces = []
      continue
    }
    if (type === 'product_interaction') {
      const payload = (p.payload ?? {}) as Record<string, unknown>
      if (p.kind === 'celebrate') messages.push({ role: 'tool', tool: 'celebrate', stars: Number(payload.stars ?? 1), reason: String(payload.reason ?? '') })
      if (p.kind === 'scene' && typeof payload.html === 'string') messages.push({ role: 'tool', tool: 'scene', html: payload.html })
      continue
    }
    if (!isMainEvent(e, sessionID)) continue
    if (type === 'llm_generation_end') {
      turnText = assistantTurnText(p) || turnText
      if (typeof p.content === 'string' && p.content.trim()) pieces.push(p.content.trim())
      continue
    }
    if (type === 'unified_completion' && typeof p.final_result === 'string' && p.final_result.trim()) {
      messages.push({ role: 'assistant', text: turnText || pieces.join('\n\n') || p.final_result })
      turnText = ''; pieces = []
    }
  }
  return messages
}

export function payloadOf(e: PlatformEvent): Record<string, unknown> {
  return ((e.data as { data?: Record<string, unknown> } | undefined)?.data ?? {}) as Record<string, unknown>
}

/** Foreground-agent events of this session only, the way AgentWorks decides it. */
export function isMainEvent(e: PlatformEvent, sessionID: string): boolean {
  if (sessionID && !eventBelongsToSession(sessionID, e)) return false
  const data = e.data as { component?: unknown; correlation_id?: unknown } | undefined
  return isForegroundSessionEvent(e, data?.component, data?.correlation_id)
}

/** Platform tool names carry an MCP server prefix; the UI shows bare names. */
export function bareToolName(name: string): string {
  return mcpToolDisplayName(name).name
}
const statusLabels: Record<string, string> = {
  execute_shell_command: 'Working in the workspace',
  diff_patch_workspace_file: 'Editing a page',
  read_image: 'Reading the image',
  web_search: 'Looking up best practices',
  agent_browser: 'Checking the browser',
  find_image: 'Finding a picture',
  create_learning_activity: 'Putting the activity together',
}

/** Strips the per-user product root so paths match what the UI already uses. */
export function familyRelativePath(path: string): string {
  const clean = String(path ?? '').replace(/^\/+/, '')
  const marker = 'Chats/SparkQuill/'
  const i = clean.indexOf(marker)
  return i >= 0 ? clean.slice(i + marker.length) : clean
}

/**
 * TurnCollector consumes the events of one turn. Feed it every event; it
 * forwards the preview (delta / status / tool_call), gathers the side-effect
 * signals, and reports done once the main agent's completion arrives.
 */
export class TurnCollector {
  private readonly toolCalls = new Map<string, ToolCallRecord>()
  private readonly toolEvents: ToolEvent[] = []
  private suggestions: TurnResult['suggestions']
  private scene: string | undefined
  private reply = ''
  private preview = ''
  private lastChunkIndex = -1
  private lastText = ''
  private turnText = ''
  private error: string | undefined
  done = false

  constructor(private readonly sessionID: string, private readonly onEvent: (e: TurnStreamEvent) => void) {}

  feed(e: PlatformEvent): void {
    if (this.done) return
    const type = e.type ?? e.data?.type
    const p = payloadOf(e)
    switch (type) {
      case 'product_interaction':
        this.interaction(String(p.kind ?? ''), (p.payload ?? {}) as Record<string, unknown>)
        return
      case 'presentation_updated': {
        const parsed = parsePresentationUpdatedEvent(e)
        if (parsed) this.presentation(parsed.kind, parsed.payload)
        return
      }
    }
    if (!isMainEvent(e, this.sessionID)) return
    switch (type) {
      case 'streaming_chunk':
        this.chunk(p)
        return
      case 'tool_call_start': {
        const name = bareToolName(String(p.tool_name ?? ''))
        const id = String(p.tool_call_id ?? `${name}-${this.toolCalls.size}`)
        const record: ToolCallRecord = { tool_call_id: id, tool_name: name, server_name: 'direct_execution', arguments: argumentsOf(p), status: 'running' } as ToolCallRecord
        this.toolCalls.set(id, record)
        this.onEvent({ type: 'tool_call', tool_call: record })
        const label = statusLabels[name]
        if (label) this.onEvent({ type: 'status', text: label })
        return
      }
      case 'tool_call_end':
      case 'tool_call_error': {
        const name = bareToolName(String(p.tool_name ?? ''))
        const id = String(p.tool_call_id ?? '')
        const prev = this.toolCalls.get(id) ?? ({ tool_call_id: id, tool_name: name, server_name: 'direct_execution', arguments: '' } as ToolCallRecord)
        const record: ToolCallRecord = {
          ...prev,
          result: typeof p.result === 'string' ? p.result : prev.result,
          error: type === 'tool_call_error' ? String(p.error ?? 'failed') : undefined,
          duration: typeof p.duration === 'number' ? p.duration : prev.duration,
          status: type === 'tool_call_error' ? 'failed' : 'completed',
        } as ToolCallRecord
        this.toolCalls.set(id || record.tool_call_id || name, record)
        this.onEvent({ type: 'tool_call', tool_call: record })
        this.onEvent({ type: 'status', text: '' })
        return
      }
      case 'llm_generation_end': {
        if (typeof p.content === 'string' && p.content.trim()) this.lastText = p.content
        const turn = assistantTurnText(p)
        if (turn) this.turnText = turn
        return
      }
      case 'agent_error':
      case 'conversation_error':
      case 'llm_generation_error': {
        this.error = String(p.error ?? 'something went wrong')
        this.done = true
        return
      }
      case 'unified_completion': {
        const status = String(p.status ?? '')
        if (status === 'error') this.error = String(p.error ?? 'the turn failed')
        else this.reply = this.turnText || (typeof p.final_result === 'string' ? p.final_result : this.lastText)
        this.done = true
        return
      }
    }
  }

  // Mirrors AgentWorks' appendStreamingChunk (stores/useChatStore.ts): the
  // backend's `source` is authoritative, so a raw tmux pane frame (source
  // "terminal", the "⏺ … ✻ Churned for 3s" capture) is never shown as prose;
  // heartbeat/tool markers become a status line; chunk 0/1 starts a new
  // generation; is_delta picks verbatim vs block joining.
  private chunk(p: Record<string, unknown>) {
    const content = typeof p.content === 'string' ? p.content : ''
    if (!content || p.is_tool_call) return
    const chunkIndex = typeof p.chunk_index === 'number' ? p.chunk_index : -1
    if (chunkIndex === 0 || chunkIndex === 1) {
      this.lastChunkIndex = -1
      this.preview = ''
    }
    if (chunkIndex >= 0 && chunkIndex <= this.lastChunkIndex) return
    this.lastChunkIndex = chunkIndex
    const source = typeof p.source === 'string' ? p.source.trim().toLowerCase() : ''
    const { statusText, text } = splitStreamingStatusAndText(content)
    const terminal = source === 'terminal' || looksLikeTerminalScreenText(text || content)
    const safeText = terminal ? '' : text
    const status = statusText || (terminal ? 'Working' : null)
    if (status) this.chunkStatus(status)
    if (!safeText) return
    if (!status) this.chunkStatus('')
    const next = appendStreamingText(this.preview, safeText, p.is_delta === true ? true : p.is_delta === false ? false : undefined)
    if (next === this.preview) return
    this.preview = next
    this.onEvent({ type: 'replace', text: next })
  }

  private lastChunkStatus = ''
  private chunkStatus(text: string) {
    if (text === this.lastChunkStatus) return
    this.lastChunkStatus = text
    this.onEvent({ type: 'status', text })
  }

  private interaction(kind: string, payload: Record<string, unknown>) {
    switch (kind) {
      case 'family_updated': {
        const child = payload.child as { name?: string; grade?: string; board?: string } | undefined
        if (child) this.toolEvents.push({ tool: 'set_child_profile', name: child.name, grade: child.grade, board: child.board })
        if (typeof payload.parent_label === 'string') this.toolEvents.push({ tool: 'set_parent_label', parent_label: payload.parent_label })
        if (payload.schedule) this.toolEvents.push({ tool: 'set_child_schedule' })
        return
      }
      case 'pins_updated':
        this.toolEvents.push({ tool: 'pins_updated' })
        return
      case 'activity_created':
        this.toolEvents.push({ tool: 'create_learning_activity', path: familyRelativePath(String(payload.dir ?? '')), package: String(payload.title ?? '') })
        return
      case 'suggestions': {
        const raw = Array.isArray(payload.actions) ? (payload.actions as { label?: string; message?: string }[]) : []
        this.suggestions = raw.filter((a) => a.label && a.message).map((a) => ({ label: String(a.label), message: String(a.message) }))
        return
      }
      case 'celebrate':
        this.toolEvents.push({ tool: 'celebrate', stars: Number(payload.stars ?? 1), reason: String(payload.reason ?? '') })
        return
      case 'scene':
        if (typeof payload.html === 'string') this.scene = payload.html
        return
    }
  }

  private presentation(kind: string, payload: Record<string, unknown>) {
    if (kind === 'document.file' && typeof payload.path === 'string') {
      this.toolEvents.push({ tool: 'open_file', path: familyRelativePath(payload.path), focus: typeof payload.focus === 'string' ? payload.focus : undefined })
    } else if (kind === 'sparkquill.activity' && typeof payload.dir === 'string') {
      this.toolEvents.push({ tool: 'open_activity', path: familyRelativePath(payload.dir) })
    }
  }

  result(): TurnResult {
    return {
      reply: this.reply || undefined,
      error: this.error,
      suggestions: this.suggestions,
      tool_events: this.toolEvents,
      tool_calls: [...this.toolCalls.values()],
      scene: this.scene,
    }
  }
}

function argumentsOf(p: Record<string, unknown>): string {
  const params = p.tool_params as { arguments?: unknown } | undefined
  const args = params?.arguments ?? p.arguments
  if (typeof args === 'string') return args
  if (args === undefined) return ''
  try { return JSON.stringify(args) } catch { return '' }
}
