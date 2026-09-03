// FamilyApi against the AgentWorks platform: the SparkQuill product profiles
// (internal/sparkquillproduct) reached through the agent-profile chat routes,
// with the session event stream mapped back into the preview and result
// shapes the UI already understands. Slice 2 covers conversations; the
// workspace, setup and connector methods land in the next slices and say so
// loudly until then.
import type { ApiEngine, Activity, VoiceStatus } from '../stores/types'
import type {
  FamilyApi, FastMode, FileContent, ModelInfo, PulseConfig, PulseConfigPatch, ScheduleEntry, SetupState,
  StoredConversation, TreeResponse, TurnMessage, TurnResult, TurnStreamEvent, UploadResult, WeekResponse,
  WhatsAppStatus, WhatsAppVoiceTranscription,
} from './familyApi'
import { type EventBatch, type PlatformEvent, TurnCollector, isMainEvent, payloadOf, readSSE } from './platform/events'

export const PARENT_PROFILE = 'sparkquill'
export const CHILD_PROFILE = 'sparkquill-child'
const FAMILY_ROOT = 'Chats/SparkQuill'
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

  async function token(): Promise<string> {
    return store.get() ?? login()
  }

  async function request<T>(method: string, path: string, body?: unknown, retry = true): Promise<T> {
    const res = await fetch(`${base}${path}`, {
      method,
      headers: { Authorization: `Bearer ${await token()}`, ...(body === undefined ? {} : { 'Content-Type': 'application/json' }) },
      body: body === undefined ? undefined : JSON.stringify(body),
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

  async function conversation(profile: string, key: string): Promise<Conversation> {
    const cacheKey = `${profile}/${key}`
    const cached = conversations.get(cacheKey)
    if (cached) return cached
    const resolved = await request<{ session_id: string }>('POST', `/api/agent-profiles/${profile}/conversation`, key ? { conversation_key: key } : {})
    const batch = await request<EventBatch>('GET', `/api/sessions/${encodeURIComponent(resolved.session_id)}/events?since=0&working_set=session`).catch(() => null)
    const cursor = batch && typeof batch.last_processed_index === 'number' && batch.last_processed_index >= 0 ? batch.last_processed_index : 0
    const conv = { sessionID: resolved.session_id, cursor }
    conversations.set(cacheKey, conv)
    return conv
  }

  /** Follows a session's stream from its cursor; the returned function stops it. */
  function follow(conv: Conversation, onBatch: (batch: EventBatch) => void, onEnd?: (err?: Error) => void): () => void {
    const controller = new AbortController()
    let stopped = false
    const run = async () => {
      let attempts = 0
      while (!stopped) {
        try {
          const auth = await token()
          await readSSE(`${base}/api/sessions/${encodeURIComponent(conv.sessionID)}/events/stream?working_set=session&since=${conv.cursor}`,
            { Authorization: `Bearer ${auth}` }, controller.signal, (batch, lastID) => {
              if (lastID >= 0) conv.cursor = lastID
              else if (typeof batch.last_processed_index === 'number' && batch.last_processed_index >= 0) conv.cursor = batch.last_processed_index
              onBatch(batch)
            })
          attempts = 0
        } catch (err) {
          if (stopped) break
          attempts += 1
          if (attempts > 5) { onEnd?.(err instanceof Error ? err : new Error(String(err))); return }
          await new Promise((r) => setTimeout(r, 1000 * attempts))
        }
      }
      onEnd?.()
    }
    void run()
    return () => { stopped = true; controller.abort() }
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
      stop = follow(conv, (batch) => {
        for (const e of batch.events ?? []) {
          collector.feed(e)
          arm()
          if (collector.done) { finish(); return }
        }
      }, (err) => { if (err) finish(err) })
      request<{ session_id?: string; status?: string; error?: string }>('POST', `/api/agent-profiles/${profile}/query`, key ? { message: text, conversation_key: key } : { message: text })
        .then((resp) => {
          if (resp.session_id && resp.session_id !== conv.sessionID) {
            // The server rebound the conversation; follow the session it chose.
            stop()
            conv.sessionID = resp.session_id
            conv.cursor = 0
            stop = follow(conv, (batch) => {
              for (const e of batch.events ?? []) {
                collector.feed(e)
                arm()
                if (collector.done) { finish(); return }
              }
            }, (err) => { if (err) finish(err) })
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
  async function history(profile: string, key: string): Promise<StoredConversation | null> {
    const conv = await conversation(profile, key)
    const batch = await request<EventBatch>('GET', `/api/sessions/${encodeURIComponent(conv.sessionID)}/events?since=0&working_set=session`)
    const messages: NonNullable<StoredConversation['messages']> = []
    for (const e of batch.events ?? []) {
      const type = e.type ?? e.data?.type
      const p = payloadOf(e)
      if (type === 'user_message' && typeof p.content === 'string') { messages.push({ role: 'user', text: p.content }); continue }
      if (type === 'product_interaction') {
        const payload = (p.payload ?? {}) as Record<string, unknown>
        if (p.kind === 'celebrate') messages.push({ role: 'tool', tool: 'celebrate', stars: Number(payload.stars ?? 1), reason: String(payload.reason ?? '') })
        if (p.kind === 'scene' && typeof payload.html === 'string') messages.push({ role: 'tool', tool: 'scene', html: payload.html })
        continue
      }
      if (type === 'unified_completion' && isMainEvent(e, conv.sessionID) && typeof p.final_result === 'string' && p.final_result.trim()) {
        messages.push({ role: 'assistant', text: p.final_result })
      }
    }
    return { messages }
  }

  // ---- files (slice 3 fills this in; readFile is needed for setup now) ----
  async function readFile(path: string): Promise<FileContent> {
    const rel = path.replace(/^\/+/, '')
    const full = rel.startsWith('_users/') || rel.startsWith(FAMILY_ROOT) ? rel : `${FAMILY_ROOT}/${rel}`
    const encoded = full.split('/').map(encodeURIComponent).join('/')
    const resp = await request<{ success?: boolean; data?: { content?: string; is_binary?: boolean; size?: number } }>('GET', `/api/wp/api/documents/${encoded}`)
    const data = resp?.data ?? {}
    return { path: rel, is_text: !data.is_binary, content: data.content ?? '', size: data.size }
  }

  async function setup(): Promise<SetupState> {
    const family = await readFile('family.json').catch(() => null)
    let state: { child?: { name?: string; grade?: string; board?: string } | null; parent_label?: string; pin_hash?: string } = {}
    try { state = family?.content ? JSON.parse(family.content) : {} } catch { state = {} }
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

  async function engines(): Promise<ApiEngine[]> {
    const profile = await request<{ runtime?: { provider_options?: { id: string; label: string; default?: boolean }[] } }>('GET', `/api/agent-profiles/${PARENT_PROFILE}`)
    return (profile.runtime?.provider_options ?? []).map((o) => ({ id: o.id, name: o.label, runtime_command: '', runtime_available: true, auth_configured: true, usable: true } as ApiEngine))
  }

  const api: FamilyApi = {
    baseUrl: base,

    setup,
    engines,
    validateEngine: async () => ({ valid: true, message: 'The platform manages the model.' }),
    selectEngine: async () => {},
    saveChild: notYet('saving the child profile from the setup screen'),
    setPin: notYet('setting the PIN'),
    verifyPin: notYet('verifying the PIN'),

    sendParentTurn: ({ messages, conversationId: _conversationId }, onEvent) => sendTurn(PARENT_PROFILE, '', lastUserText(messages), onEvent),
    steerParent: (_conversationId, message) => steer(PARENT_PROFILE, '', message),
    watchParent: (_conversationId, onEvent) => watch(PARENT_PROFILE, '', onEvent),
    loadParentConversation: () => history(PARENT_PROFILE, ''),

    childActivity: notYet('the current activity') as () => Promise<Activity | null>,
    handoff: notYet('the handoff'),
    sendChildTurn: ({ messages, conversationId }, onEvent) => sendTurn(CHILD_PROFILE, conversationKeyFor(CHILD_PROFILE, conversationId), lastUserText(messages), onEvent),
    steerChild: (conversationId, message) => steer(CHILD_PROFILE, conversationKeyFor(CHILD_PROFILE, conversationId), message),
    watchChild: (activityDir, onEvent) => watch(CHILD_PROFILE, conversationKeyFor(CHILD_PROFILE, activityDir), onEvent),
    loadChildConversation: (activityDir) => history(CHILD_PROFILE, conversationKeyFor(CHILD_PROFILE, activityDir)),

    tree: notYet('the workspace tree') as () => Promise<TreeResponse>,
    readFile,
    rawUrl: (path) => `${base}/api/wp/api/documents/${`${FAMILY_ROOT}/${path.replace(/^\/+/, '')}`.split('/').map(encodeURIComponent).join('/')}?download=true&token=${encodeURIComponent(store.get() ?? '')}`,
    assetUrl: (relPath) => `${base}/${relPath.replace(/^\/+/, '')}`,
    upload: notYet('uploads') as (file: File, scope: 'parent' | 'child') => Promise<UploadResult>,
    saveState: notYet('scene state'),
    loadState: notYet('scene state'),
    activities: notYet('the activity list') as () => Promise<Activity[]>,
    week: notYet('the week view') as (offset: number) => Promise<WeekResponse>,
    saveSchedule: notYet('saving the schedule') as (entries: ScheduleEntry[]) => Promise<void>,

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

/** Unused-import guard for types only referenced in the FamilyApi shape. */
export type { PlatformEvent }
