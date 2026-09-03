// FamilyApi against the AgentWorks platform: the SparkQuill product profiles
// (internal/sparkquillproduct) reached through the agent-profile chat routes,
// with the session event stream mapped back into the preview and result
// shapes the UI already understands. Slice 2 covers conversations; the
// workspace, setup and connector methods land in the next slices and say so
// loudly until then.
import type { ApiEngine, QuickCommand, VoiceStatus } from '../stores/types'
import type {
  FamilyApi, FastMode, ModelInfo, PulseConfig, PulseConfigPatch, SetupState,
  StoredConversation, TurnMessage, TurnResult, TurnStreamEvent,
  WhatsAppStatus, WhatsAppVoiceTranscription,
} from './familyApi'
import { TurnCollector, type EventBatch, messagesFromEvents, type PlatformEvent } from './platform/events'
import { quickCommandsFromProfile } from './platform/commands'
import { fetchSessionEvents, followSession, conversationToRestoredEvents, type RestorableConversation } from '../../../shared/session'
import { FamilyWorkspace, documentsURL } from './platform/workspace'

export const PARENT_PROFILE = 'sparkquill'
export const CHILD_PROFILE = 'sparkquill-child'
const TOKEN_KEY = 'sparkquill.platform.token'

export type PlatformApiOptions = {
  baseUrl: string
  /** Credentials for multi-user servers; a single-user server needs none. */
  login?: { username: string; password: string }
  /** Overrides the token store (node tests). */
  tokenStore?: { get(): string | null; set(token: string | null): void }
  /** Time without any event before a turn is given up on. */
  turnInactivityMs?: number
}

type Conversation = { sessionID: string; cursor: number }

const notYet = (what: string) => () => Promise.reject(new Error(`${what} is not available on the platform yet`))

export function createPlatformApi(options: PlatformApiOptions): FamilyApi {
  const base = options.baseUrl.replace(/\/+$/, '')
  const store = options.tokenStore ?? browserTokenStore()
  const inactivity = options.turnInactivityMs ?? 20 * 60 * 1000
  const conversations = new Map<string, Conversation>()

  // ---- auth ------------------------------------------------------------
  async function login(): Promise<string> {
    const res = await fetch(`${base}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: options.login ? JSON.stringify({ ...options.login, provider: 'local' }) : undefined,
    })
    if (!res.ok) throw new Error(`login failed: HTTP ${res.status}`)
    const data = (await res.json()) as { token?: string }
    if (!data.token) throw new Error('login returned no token')
    store.set(data.token)
    return data.token
  }

  let loginInFlight: Promise<string> | null = null
  async function token(): Promise<string> {
    const saved = store.get()
    if (saved) return saved
    // Several requests start together on boot; log in once, not once each.
    if (!loginInFlight) loginInFlight = login().finally(() => { loginInFlight = null })
    return loginInFlight
  }

  async function request<T>(method: string, path: string, body?: unknown, retry = true): Promise<T> {
    const form = typeof FormData !== 'undefined' && body instanceof FormData
    const res = await fetch(`${base}${path}`, {
      method,
      headers: { Authorization: `Bearer ${await token()}`, ...(body === undefined || form ? {} : { 'Content-Type': 'application/json' }) },
      body: body === undefined ? undefined : form ? (body as FormData) : JSON.stringify(body),
    })
    if (res.status === 401 && retry) {
      store.set(null)
      return request<T>(method, path, body, false)
    }
    const text = await res.text()
    let parsed: unknown = null
    try { parsed = text ? JSON.parse(text) : null } catch { parsed = null }
    if (!res.ok) {
      const message = (parsed as { error?: string } | null)?.error ?? text ?? `HTTP ${res.status}`
      throw new Error(message)
    }
    return parsed as T
  }

  // ---- conversations -----------------------------------------------------
  function conversationKeyFor(profile: string, activityDir?: string): string {
    if (profile === PARENT_PROFILE) return ''
    const slug = String(activityDir ?? '').replace(/\/+$/, '').split('/').pop() ?? ''
    if (!slug) throw new Error('the child needs an activity to talk in')
    return slug
  }

  const sessionClient = { baseUrl: base, token }

  async function conversation(profile: string, key: string): Promise<Conversation> {
    const cacheKey = `${profile}/${key}`
    const cached = conversations.get(cacheKey)
    if (cached) return cached
    const resolved = await request<{ session_id: string }>('POST', `/api/agent-profiles/${profile}/conversation`, key ? { conversation_key: key } : {})
    const batch = await fetchSessionEvents(sessionClient, resolved.session_id, 0).catch(() => null)
    const cursor = batch && typeof batch.last_processed_index === 'number' && batch.last_processed_index >= 0 ? batch.last_processed_index : 0
    const conv = { sessionID: resolved.session_id, cursor }
    conversations.set(cacheKey, conv)
    return conv
  }

  /** Follows a session's stream from its cursor; the returned function stops it. */
  function follow(conv: Conversation, onBatch: (batch: EventBatch, frameID: number) => void, onEnd?: (err?: Error) => void): () => void {
    return followSession(sessionClient, conv.sessionID, conv.cursor, {
      onBatch: (batch, frameIndex) => {
        if (frameIndex >= 0) conv.cursor = Math.max(conv.cursor, frameIndex)
        onBatch(batch, frameIndex)
      },
      onEnd,
    })
  }

  async function sendTurn(profile: string, key: string, text: string, onEvent: (e: TurnStreamEvent) => void): Promise<TurnResult> {
    const conv = await conversation(profile, key)
    const collector = new TurnCollector(conv.sessionID, onEvent)
    return new Promise<TurnResult>((resolve, reject) => {
      let finished = false
      let timer: ReturnType<typeof setTimeout> | undefined
      let stop: () => void = () => {}
      const finish = (err?: Error) => {
        if (finished) return
        finished = true
        if (timer) clearTimeout(timer)
        stop()
        if (err) reject(err)
        else resolve(collector.result())
      }
      const arm = () => {
        if (timer) clearTimeout(timer)
        timer = setTimeout(() => finish(new Error('the turn went quiet for too long')), inactivity)
      }
      arm()

      // The stream replays history when the server's store is cold, and a
      // replayed completion must never pass for this turn's reply. So: open
      // the stream first, let its opening batch settle to learn where the
      // conversation currently ends, and only then send the query; events
      // at or before that mark belong to earlier turns.
      // Frames carry the store's index as their SSE id (the per-event
      // event_index field is not that index); the opening frame is the
      // backfill and its id is where history ends.
      let baseline = -1
      let anchored = false
      let resolveAnchor: () => void = () => {}
      const anchor = new Promise<void>((r) => { resolveAnchor = r })
      const onBatch = (batch: EventBatch, frameID: number) => {
        if (!anchored) {
          baseline = Math.max(frameID, typeof batch.last_processed_index === 'number' ? batch.last_processed_index : -1)
          anchored = true
          resolveAnchor()
          return
        }
        if (frameID >= 0 && frameID <= baseline) return
        for (const e of batch.events ?? []) {
          collector.feed(e)
          arm()
          if (collector.done) { finish(); return }
        }
      }
      stop = follow(conv, onBatch, (err) => { if (err) finish(err) })
      Promise.race([anchor, new Promise<void>((r) => setTimeout(r, 3000))])
        .then(() => {
          if (!anchored) { anchored = true; baseline = conv.cursor }
          return request<{ session_id?: string; status?: string; error?: string }>('POST', `/api/agent-profiles/${profile}/query`, key ? { message: text, conversation_key: key } : { message: text })
        })
        .then((resp) => {
          if (resp.session_id && resp.session_id !== conv.sessionID) {
            // The server rebound the conversation; follow the session it chose.
            stop()
            conv.sessionID = resp.session_id
            conv.cursor = 0
            baseline = -1
            stop = follow(conv, onBatch, (err) => { if (err) finish(err) })
          }
        })
        .catch((err: Error) => finish(err))
    })
  }

  async function steer(profile: string, key: string, message: string): Promise<{ steered?: boolean }> {
    const conv = await conversation(profile, key)
    const status = await request<{ can_steer?: boolean }>('GET', `/api/sessions/${encodeURIComponent(conv.sessionID)}/status`).catch(() => null)
    if (!status?.can_steer) return { steered: false }
    const res = await request<{ delivery_status?: string }>('POST', `/api/sessions/${encodeURIComponent(conv.sessionID)}/live-input`, { message })
    return { steered: res.delivery_status === 'sent_to_cli' || res.delivery_status === 'queued_for_injection' }
  }

  function watch(profile: string, key: string, onEvent: (e: TurnStreamEvent) => void): () => void {
    let stop: () => void = () => {}
    let cancelled = false
    conversation(profile, key).then((conv) => {
      if (cancelled) return
      const collector = new TurnCollector(conv.sessionID, onEvent)
      stop = follow(conv, (batch) => {
        for (const e of batch.events ?? []) {
          collector.feed(e)
          if (collector.done) { /* a turn ended elsewhere; keep watching */ }
        }
      })
    }).catch(() => {})
    return () => { cancelled = true; stop() }
  }

  /** Rebuilds a transcript from the session's event history. */
  // The persisted chat history is the durable record (the live event store is
  // in memory and empty after a server restart); it is rebuilt into events by
  // the platform's own restore converter and then read like a live turn.
  async function history(profile: string, key: string): Promise<StoredConversation | null> {
    const conv = await conversation(profile, key)
    let stored: RestorableConversation | null = null
    try {
      stored = await request<RestorableConversation>('GET', `/api/chat-history/sessions/${encodeURIComponent(conv.sessionID)}`)
    } catch (err) {
      if (!/HTTP 404/.test(String(err))) throw err
    }
    const events: PlatformEvent[] = stored?.conversation_history?.length
      ? (conversationToRestoredEvents({ ...stored, session_id: conv.sessionID }) as PlatformEvent[])
      : (await fetchSessionEvents(sessionClient, conv.sessionID, 0)).events ?? []
    return { messages: messagesFromEvents(events, conv.sessionID) }
  }

  // ---- workspace -----------------------------------------------------------
  const ws = new FamilyWorkspace(request)
  const readFile = (path: string) => ws.readFile(path)

  async function setup(): Promise<SetupState> {
    const state = (await ws.readJSON<{ child?: { name?: string; grade?: string; board?: string } | null; parent_label?: string; pin_hash?: string }>('family.json')) ?? {}
    const childDone = !!(state.child && state.child.name)
    const pinSet = !!state.pin_hash
    return {
      engine: 'platform',
      child: state.child ?? null,
      parent_label: state.parent_label,
      pin_set: pinSet,
      setup_complete: childDone && pinSet,
      next_step: !childDone ? 'child' : !pinSet ? 'pin' : 'done',
    }
  }

  async function commands(): Promise<{ parent: QuickCommand[]; child: QuickCommand[] }> {
    const [parent, child] = await Promise.all([
      request<{ commands?: Array<Record<string, unknown>> }>('GET', `/api/agent-profiles/${PARENT_PROFILE}`),
      request<{ commands?: Array<Record<string, unknown>> }>('GET', `/api/agent-profiles/${CHILD_PROFILE}`),
    ])
    return { parent: quickCommandsFromProfile(parent), child: quickCommandsFromProfile(child) }
  }

  async function engines(): Promise<ApiEngine[]> {
    const profile = await request<{ runtime?: { provider_options?: { id: string; label: string; default?: boolean }[] } }>('GET', `/api/agent-profiles/${PARENT_PROFILE}`)
    return (profile.runtime?.provider_options ?? []).map((o) => ({ id: o.id, name: o.label, runtime_command: '', runtime_available: true, auth_configured: true, usable: true } as ApiEngine))
  }

  const api: FamilyApi = {
    baseUrl: base,

    setup,
    engines,
    commands,
    validateEngine: async () => ({ valid: true, message: 'The platform manages the model.' }),
    selectEngine: async () => {},
    saveChild: notYet('saving the child profile from the setup screen'),
    setPin: notYet('setting the PIN'),
    verifyPin: notYet('verifying the PIN'),

    sendParentTurn: ({ messages, conversationId: _conversationId }, onEvent) => sendTurn(PARENT_PROFILE, '', lastUserText(messages), onEvent),
    steerParent: (_conversationId, message) => steer(PARENT_PROFILE, '', message),
    watchParent: (_conversationId, onEvent) => watch(PARENT_PROFILE, '', onEvent),
    loadParentConversation: () => history(PARENT_PROFILE, ''),

    childActivity: () => ws.currentActivity(),
    handoff: notYet('the handoff'),
    sendChildTurn: ({ messages, conversationId }, onEvent) => sendTurn(CHILD_PROFILE, conversationKeyFor(CHILD_PROFILE, conversationId), lastUserText(messages), onEvent),
    steerChild: (conversationId, message) => steer(CHILD_PROFILE, conversationKeyFor(CHILD_PROFILE, conversationId), message),
    watchChild: (activityDir, onEvent) => watch(CHILD_PROFILE, conversationKeyFor(CHILD_PROFILE, activityDir), onEvent),
    loadChildConversation: (activityDir) => history(CHILD_PROFILE, conversationKeyFor(CHILD_PROFILE, activityDir)),

    tree: () => ws.tree(),
    readFile,
    // Browser elements cannot send headers, so the token rides in the URL
    // (the server accepts ?token= everywhere). /raw serves inline bytes with
    // range support, which is what <img>, <video> and the PDF frame need.
    rawUrl: (path, opts) => `${base}${documentsURL(path, '/raw')}?${opts?.download ? 'download=true&' : ''}token=${encodeURIComponent(store.get() ?? '')}`,
    // The app ships its own static assets (public/lib); they live on the app's origin.
    assetUrl: (relPath) => (typeof window !== 'undefined' ? `${window.location.origin}/${relPath.replace(/^\/+/, '')}` : `/${relPath.replace(/^\/+/, '')}`),
    upload: async (file, scope) => {
      let folder = 'inbox'
      if (scope === 'child') {
        const current = await ws.currentActivity()
        if (!current) return { name: file.name, error: 'no activity is currently active' }
        folder = current.dir
      }
      return ws.upload(file, folder)
    },
    saveState: (key, data) => ws.writeJSON(ws.stateFile(key), { key, data }),
    loadState: async (key) => (await ws.readJSON<{ data?: unknown }>(ws.stateFile(key)))?.data ?? null,
    activities: () => ws.activities(),
    week: (offset) => ws.week(offset),
    saveSchedule: (entries) => ws.saveSchedule(entries),

    models: async () => null as ModelInfo | null,
    saveModel: async () => {},
    fastMode: async () => ({ enabled: false, child_enabled: false }) as FastMode,
    saveFastMode: async () => {},
    secrets: notYet('secrets'),
    saveSecret: notYet('secrets'),
    deleteSecret: notYet('secrets'),
    voiceStatus: () => request<VoiceStatus>('GET', '/api/voice/status'),
    browserStatus: async () => ({ cli_installed: false }),

    whatsappStatus: notYet('WhatsApp status') as () => Promise<WhatsAppStatus>,
    whatsappPairImageUrl: () => '',
    whatsappUnpair: notYet('WhatsApp'),
    whatsappVoice: notYet('WhatsApp') as (enabled: boolean) => Promise<WhatsAppVoiceTranscription>,
    pulseConfig: notYet('the check-in settings') as () => Promise<PulseConfig>,
    savePulseConfig: notYet('the check-in settings') as (patch: PulseConfigPatch) => Promise<PulseConfig>,
    runPulse: notYet('running the check-in'),
  }
  return api
}

function lastUserText(messages: TurnMessage[]): string {
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].role === 'user') return messages[i].text
  }
  return messages[messages.length - 1]?.text ?? ''
}

function browserTokenStore() {
  return {
    get: () => { try { return localStorage.getItem(TOKEN_KEY) } catch { return null } },
    set: (t: string | null) => { try { if (t) localStorage.setItem(TOKEN_KEY, t); else localStorage.removeItem(TOKEN_KEY) } catch { /* ignore */ } },
  }
}

