// FamilyApi against the standalone family server (cmd/family-server): the
// endpoints the app has always called, moved out of the component verbatim.
import { FAMILY_API } from '../apiBase'
import type { ApiEngine, Activity, VoiceStatus } from '../stores/types'
import type {
  FamilyApi, FastMode, FileContent, ModelInfo, PulseConfig, PulseConfigPatch, SetupState,
  StoredConversation, TreeResponse, TurnResult, TurnStreamEvent, UploadResult,
  WhatsAppStatus, WhatsAppVoiceTranscription,
} from './familyApi'

const json = { 'Content-Type': 'application/json' }

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${FAMILY_API}${path}`)
  if (!res.ok) throw new Error(String(res.status))
  return res.json() as Promise<T>
}

async function sendJSON<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${FAMILY_API}${path}`, { method, headers: json, body: body === undefined ? undefined : JSON.stringify(body) })
  return res.json() as Promise<T>
}

/** Subscribes to the server's preview stream; the returned function closes it. */
function stream(scope: 'parent' | 'child', conversationId: string, onEvent: (e: TurnStreamEvent) => void): () => void {
  const source = new EventSource(`${FAMILY_API}/api/${scope}/status?conversation_id=${encodeURIComponent(conversationId)}`)
  source.onmessage = (ev) => {
    try { onEvent(JSON.parse(ev.data) as TurnStreamEvent) } catch { /* ignore malformed event */ }
  }
  source.onerror = () => source.close()
  return () => source.close()
}

async function runTurn(scope: 'parent' | 'child', body: Record<string, unknown>, conversationId: string, onEvent: (e: TurnStreamEvent) => void): Promise<TurnResult> {
  const stop = stream(scope, conversationId, onEvent)
  try {
    const res = await fetch(`${FAMILY_API}/api/${scope}/message`, { method: 'POST', headers: json, body: JSON.stringify(body) })
    return (await res.json()) as TurnResult
  } finally {
    stop()
  }
}

async function readConversation(path: string): Promise<StoredConversation | null> {
  const d = await getJSON<FileContent>(`/api/workspace/file?path=${encodeURIComponent(path)}`)
  if (!d?.content) return null
  return JSON.parse(d.content) as StoredConversation
}

// The standalone server has no product manifest, so its quick menus are fixed
// here (the platform serves the same lists from product.yaml `commands:`).
const STANDALONE_COMMANDS = {
  parent: [
  { label: "Create study material", message: "Create study material for my child \u2014 follow your create-study-material skill and make it a designed, static (view-only) HTML page." },
  { label: "Create a practice test", message: "Create a practice test for my child \u2014 follow your create-test skill: an interactive HTML page that records my child\u2019s typed answers, plus a separate answer key for me." },
  { label: "Update progress report", message: "Build an updated progress report \u2014 follow your create-progress-report skill, make it a designed HTML page, and give me a short coach-style read of the evidence here in chat too." },
  { label: "Back up workspace", message: "Back up my workspace now \u2014 follow your backup skill." },
],
  child: [
  { label: "Update answers for print", message: "Please update all my answered questions on this page so it's ready to print." },
  { label: "No more hints", message: "Don't give me any more hints \u2014 just tell me if I'm right or wrong." },
  { label: "Be stricter", message: "Grade my answers more strictly from now on \u2014 don't count a partial or close answer as correct, and tell me exactly what's wrong." },
  { label: "Harder question", message: "Can you give me a harder question?" },
  { label: "Easier question", message: "Can you give me an easier question?" },
  { label: "Give me a hint", message: "Can you give me a hint?" },
  { label: "Keep quizzing me, harder each time", message: "Keep asking me progressively harder questions on this until I fully understand it." },
  { label: "One section at a time", message: "Let's go one section at a time \u2014 keep quizzing me on this section until I really understand it before moving to the next." },
  { label: "Explain it a different way", message: "Can you explain this in a completely different way \u2014 not just the same explanation again?" },
  { label: "Give me a real example", message: "Can you give me a real-world example of this, not just the definition?" },
  { label: "Check my full working", message: "Please check my full working step by step, not just whether my final answer is right." },
],
}

export const standaloneApi: FamilyApi = {
  baseUrl: FAMILY_API,

  setup: () => getJSON<SetupState>('/api/setup'),
  engines: () => getJSON<ApiEngine[]>('/api/engines'),
  commands: async () => STANDALONE_COMMANDS,
  validateEngine: (provider) => sendJSON('POST', '/api/engines/validate', { provider, model_id: '' }),
  selectEngine: async (engine) => { await fetch(`${FAMILY_API}/api/engine/selection`, { method: 'POST', headers: json, body: JSON.stringify({ engine }) }) },
  saveChild: async (child) => { await fetch(`${FAMILY_API}/api/child`, { method: 'POST', headers: json, body: JSON.stringify(child) }) },
  setPin: (pin) => sendJSON('POST', '/api/parent/pin', { pin }),
  verifyPin: (pin) => sendJSON('POST', '/api/parent/pin/verify', { pin }),

  sendParentTurn: ({ messages, conversationId, viewerPath }, onEvent) =>
    runTurn('parent', { messages, conversation_id: conversationId, viewer_path: viewerPath || undefined }, conversationId, onEvent),
  steerParent: (conversationId, message) => sendJSON('POST', '/api/parent/steer', { conversation_id: conversationId, message }),
  watchParent: (conversationId, onEvent) => stream('parent', conversationId, onEvent),
  loadParentConversation: () => readConversation('conversations/parent.json'),

  childActivity: () => getJSON<Activity | null>('/api/child/activity'),
  handoff: (dir, resume) => sendJSON('POST', '/api/parent/handoff', { dir, resume }),
  sendChildTurn: ({ messages, conversationId }, onEvent) =>
    runTurn('child', { messages, conversation_id: conversationId }, conversationId, onEvent),
  steerChild: (conversationId, message) => sendJSON('POST', '/api/child/steer', { conversation_id: conversationId, message }),
  watchChild: (activityDir, onEvent) => stream('child', activityDir, onEvent),
  loadChildConversation: (activityDir) => readConversation(`${activityDir}/conversation.json`),

  tree: () => getJSON<TreeResponse>('/api/workspace/tree'),
  readFile: (path) => getJSON<FileContent>(`/api/workspace/file?path=${encodeURIComponent(path)}`),
  rawUrl: (path, opts) => {
    let url = `${FAMILY_API}/api/workspace/raw?path=${encodeURIComponent(path)}`
    if (opts?.download) url += '&download=1'
    if (opts?.print) url += '&print=1'
    return url
  },
  assetUrl: (relPath) => `${FAMILY_API}/${relPath.replace(/^\/+/, '')}`,
  upload: async (file, scope) => {
    const fd = new FormData()
    fd.append('file', file)
    fd.append('scope', scope)
    const res = await fetch(`${FAMILY_API}/api/upload`, { method: 'POST', body: fd })
    return (await res.json()) as UploadResult
  },
  saveState: async (key, data) => { await fetch(`${FAMILY_API}/api/workspace/state`, { method: 'POST', headers: json, body: JSON.stringify({ key, data }) }) },
  loadState: async (key) => {
    const d = await getJSON<{ data?: unknown }>(`/api/workspace/state?key=${encodeURIComponent(key)}`)
    return d?.data ?? null
  },
  activities: () => getJSON<Activity[]>('/api/activities'),

  models: async () => {
    const d = await getJSON<ModelInfo>('/api/models')
    return d?.models?.length ? d : null
  },
  saveModel: async (modelId) => { await fetch(`${FAMILY_API}/api/models`, { method: 'POST', headers: json, body: JSON.stringify({ model_id: modelId }) }) },
  fastMode: () => getJSON<FastMode>('/api/fast-mode'),
  saveFastMode: async (patch) => { await fetch(`${FAMILY_API}/api/fast-mode`, { method: 'POST', headers: json, body: JSON.stringify(patch) }) },
  secrets: async () => (await getJSON<{ names?: string[] }>('/api/secrets')).names ?? [],
  saveSecret: async (name, value) => (await sendJSON<{ names?: string[] }>('POST', '/api/secrets', { name, value })).names ?? [],
  deleteSecret: async (name) => (await sendJSON<{ names?: string[] }>('DELETE', '/api/secrets', { name })).names ?? [],
  voiceStatus: () => getJSON<VoiceStatus>('/api/voice/status'),
  browserStatus: () => getJSON<{ cli_installed: boolean }>('/api/browser/status'),

  whatsappStatus: () => getJSON<WhatsAppStatus>('/api/whatsapp/status'),
  whatsappPairImageUrl: (nonce) => `${FAMILY_API}/api/whatsapp/pair?n=${nonce}`,
  whatsappUnpair: async (jid) => { await sendJSON('POST', '/api/whatsapp/unpair', { jid }) },
  whatsappVoice: (enabled) => sendJSON<WhatsAppVoiceTranscription>('POST', '/api/whatsapp/voice', { enabled }),
  pulseConfig: () => getJSON<PulseConfig>('/api/pulse/config'),
  savePulseConfig: (patch: PulseConfigPatch) => sendJSON<PulseConfig>('POST', '/api/pulse/config', patch),
  runPulse: async () => {
    const res = await fetch(`${FAMILY_API}/api/pulse/run`, { method: 'POST' })
    const d = (await res.json().catch(() => ({}))) as { error?: string }
    return { ok: res.ok, error: d.error }
  },
}
