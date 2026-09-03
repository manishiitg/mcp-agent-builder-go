// The platform's session event stream, read the way the AgentWorks frontend
// reads it (frontend/src/services/sse.ts, polling.go) but without React:
// a fetch-based SSE reader, the envelope shapes, main-agent filtering, and
// a pure collector that turns a stream of platform events into the
// preview events and the final TurnResult the SparkQuill UI expects.
import type { ToolCallRecord } from '../../stores/types'
import type { ToolEvent, TurnResult, TurnStreamEvent } from '../familyApi'

/** One event as the agent server delivers it (polling and SSE alike). */
export type PlatformEvent = {
  id?: string
  type?: string
  timestamp?: string
  session_id?: string
  execution_id?: string
  execution_kind?: string
  sequence?: number
  error?: string
  data?: {
    type?: string
    event_index?: number
    hierarchy_level?: number | string
    component?: string
    correlation_id?: string
    turn_id?: string
    session_id?: string
    data?: Record<string, unknown>
  }
}

/** The envelope both the polling route and each SSE `event` frame carry. */
export type EventBatch = {
  events?: PlatformEvent[]
  has_more?: boolean
  session_status?: string
  display_status?: string
  last_processed_index?: number
  can_steer?: boolean
  runtime_state?: { foreground_turn?: { busy?: boolean; can_steer?: boolean }; phase?: string }
}

export function payloadOf(e: PlatformEvent): Record<string, unknown> {
  return (e.data?.data ?? {}) as Record<string, unknown>
}

/** Mirrors isForegroundSessionEvent in the AgentWorks frontend. */
export function isMainEvent(e: PlatformEvent, sessionID: string): boolean {
  if (e.session_id && sessionID && e.session_id !== sessionID) return false
  const component = e.data?.component ?? ''
  const correlation = e.data?.correlation_id ?? ''
  if (/^(delegation|workshop)-/.test(component) || /^(delegation|workshop)-/.test(correlation)) return false
  if (e.execution_kind && e.execution_kind !== 'main_agent') return false
  if (e.execution_id && !e.execution_id.startsWith('main:')) return false
  return true
}

/** Platform tool names carry an MCP server prefix; the UI shows bare names. */
export function bareToolName(name: string): string {
  return name.replace(/^mcp__[^_]+(?:-[^_]+)*__/, '')
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
  private lastText = ''
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
      case 'presentation_updated':
        this.presentation(String(p.kind ?? ''), (p.payload ?? {}) as Record<string, unknown>)
        return
    }
    if (!isMainEvent(e, this.sessionID)) return
    switch (type) {
      case 'streaming_chunk': {
        const content = typeof p.content === 'string' ? p.content : ''
        if (content && p.is_delta !== false && !p.is_tool_call) this.onEvent({ type: 'delta', text: content })
        return
      }
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
        else this.reply = typeof p.final_result === 'string' ? p.final_result : this.lastText
        this.done = true
        return
      }
    }
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

/**
 * readSSE opens the session stream with fetch (so the token rides in a
 * header and the same code runs under node) and hands each `event` frame's
 * batch to onBatch. Resolves when the stream ends or the signal aborts.
 */
export async function readSSE(url: string, headers: Record<string, string>, signal: AbortSignal, onBatch: (batch: EventBatch, lastID: number) => void): Promise<void> {
  const res = await fetch(url, { headers: { ...headers, Accept: 'text/event-stream' }, signal })
  if (!res.ok || !res.body) throw new Error(`event stream HTTP ${res.status}`)
  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let eventName = ''
  let dataLines: string[] = []
  let lastID = -1
  const flush = () => {
    if (dataLines.length > 0 && (eventName === 'event' || eventName === '')) {
      try {
        onBatch(JSON.parse(dataLines.join('\n')) as EventBatch, lastID)
      } catch { /* malformed frame: skip */ }
    }
    eventName = ''
    dataLines = []
  }
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    let nl: number
    while ((nl = buffer.indexOf('\n')) >= 0) {
      const line = buffer.slice(0, nl).replace(/\r$/, '')
      buffer = buffer.slice(nl + 1)
      if (line === '') { flush(); continue }
      if (line.startsWith(':')) continue
      if (line.startsWith('id:')) { const n = Number(line.slice(3).trim()); if (!Number.isNaN(n)) lastID = n; continue }
      if (line.startsWith('event:')) { eventName = line.slice(6).trim(); continue }
      if (line.startsWith('data:')) { dataLines.push(line.slice(5).replace(/^ /, '')); continue }
    }
  }
  flush()
}
