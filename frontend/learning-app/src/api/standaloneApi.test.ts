// Contract test: the standalone implementation must call exactly the
// endpoints the component used to call inline, with the same bodies, and
// must open the preview stream for the duration of a turn and close it after.
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { standaloneApi as api } from './standaloneApi'
import { FAMILY_API } from '../apiBase'

type Call = { url: string; init?: RequestInit }
const calls: Call[] = []
const sources: FakeEventSource[] = []

class FakeEventSource {
  url: string
  onmessage: ((ev: { data: string }) => void) | null = null
  onerror: (() => void) | null = null
  closed = false
  constructor(url: string) { this.url = url; sources.push(this) }
  close() { this.closed = true }
}

function jsonResponse(body: unknown, ok = true): Response {
  return { ok, status: ok ? 200 : 500, json: async () => body } as unknown as Response
}

beforeEach(() => {
  calls.length = 0
  sources.length = 0
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    calls.push({ url, init })
    return jsonResponse({ reply: 'hi', names: ['a'], data: { x: 1 }, content: '{"messages":[{"role":"user","text":"q"}]}', enabled: true })
  }))
})
afterEach(() => vi.unstubAllGlobals())

const body = (c: Call) => JSON.parse(String(c.init?.body))

describe('standaloneApi', () => {
  it('keeps the setup, state and settings endpoints byte for byte', async () => {
    await api.setup(); await api.engines(); await api.validateEngine('codex-cli'); await api.selectEngine('codex-cli')
    await api.saveChild({ name: 'Maya', grade: '6', board: 'CBSE' }); await api.setPin('1234'); await api.verifyPin('1234')
    await api.saveSchedule([{ day: 'Monday', start: '08:00', end: '14:00', label: 'School' }]); await api.week(1)
    await api.saveModel('m'); await api.saveFastMode({ enabled: true, child_enabled: false })
    await api.saveSecret('n', 'v'); await api.deleteSecret('n'); await api.savePulseConfig({ enabled: true }); await api.runPulse()
    await api.whatsappUnpair('919999'); await api.whatsappVoice(true); await api.handoff('activities/a', true)
    await api.steerParent('c1', 'msg'); await api.steerChild('activities/a', 'msg'); await api.saveState('k', { s: 1 }); await api.loadState('k')
    const seen = calls.map((c) => `${c.init?.method ?? 'GET'} ${c.url.replace(FAMILY_API, '')}`)
    expect(seen).toEqual([
      'GET /api/setup', 'GET /api/engines', 'POST /api/engines/validate', 'POST /api/engine/selection',
      'POST /api/child', 'POST /api/parent/pin', 'POST /api/parent/pin/verify',
      'POST /api/child-schedule', 'GET /api/week?offset=1',
      'POST /api/models', 'POST /api/fast-mode',
      'POST /api/secrets', 'DELETE /api/secrets', 'POST /api/pulse/config', 'POST /api/pulse/run',
      'POST /api/whatsapp/unpair', 'POST /api/whatsapp/voice', 'POST /api/parent/handoff',
      'POST /api/parent/steer', 'POST /api/child/steer', 'POST /api/workspace/state', 'GET /api/workspace/state?key=k',
    ])
    expect(body(calls[2])).toEqual({ provider: 'codex-cli', model_id: '' })
    expect(body(calls[4])).toEqual({ name: 'Maya', grade: '6', board: 'CBSE' })
    expect(body(calls[7])).toEqual({ entries: [{ day: 'Monday', start: '08:00', end: '14:00', label: 'School' }] })
    expect(body(calls[9])).toEqual({ model_id: 'm' })
    expect(body(calls[18])).toEqual({ conversation_id: 'c1', message: 'msg' })
    expect(body(calls[20])).toEqual({ key: 'k', data: { s: 1 } })
  })

  it('reads files and conversations through the workspace endpoints', async () => {
    await api.readFile('reports/progress.html')
    const conv = await api.loadParentConversation()
    await api.loadChildConversation('activities/a')
    expect(calls.map((c) => c.url.replace(FAMILY_API, ''))).toEqual([
      '/api/workspace/file?path=reports%2Fprogress.html',
      '/api/workspace/file?path=conversations%2Fparent.json',
      '/api/workspace/file?path=activities%2Fa%2Fconversation.json',
    ])
    expect(conv?.messages?.[0].text).toBe('q')
    expect(api.rawUrl('a b.png')).toBe(`${FAMILY_API}/api/workspace/raw?path=a%20b.png`)
    expect(api.rawUrl('x.pdf', { download: true })).toBe(`${FAMILY_API}/api/workspace/raw?path=x.pdf&download=1`)
    expect(api.rawUrl('x.html', { print: true })).toBe(`${FAMILY_API}/api/workspace/raw?path=x.html&print=1`)
    expect(api.assetUrl('lib/jsxgraph.css')).toBe(`${FAMILY_API}/lib/jsxgraph.css`)
    expect(api.whatsappPairImageUrl(3)).toBe(`${FAMILY_API}/api/whatsapp/pair?n=3`)
  })

  it('opens the preview stream for a turn, forwards events, and closes it after the reply', async () => {
    const events: unknown[] = []
    const pending = api.sendParentTurn({ messages: [{ role: 'user', text: 'hello' }], conversationId: 'c1', viewerPath: 'reports/progress.html' }, (e) => events.push(e))
    expect(sources).toHaveLength(1)
    expect(sources[0].url).toBe(`${FAMILY_API}/api/parent/status?conversation_id=c1`)
    sources[0].onmessage?.({ data: JSON.stringify({ type: 'delta', text: 'he' }) })
    sources[0].onmessage?.({ data: 'not json' })
    const result = await pending
    expect(result.reply).toBe('hi')
    expect(events).toEqual([{ type: 'delta', text: 'he' }])
    expect(sources[0].closed).toBe(true)
    const post = calls.find((c) => c.url.endsWith('/api/parent/message'))!
    expect(body(post)).toEqual({ messages: [{ role: 'user', text: 'hello' }], conversation_id: 'c1', viewer_path: 'reports/progress.html' })

    await api.sendChildTurn({ messages: [{ role: 'user', text: 'x' }], conversationId: 'activities/a' }, () => {})
    expect(sources[1].url).toBe(`${FAMILY_API}/api/child/status?conversation_id=activities%2Fa`)
    expect(sources[1].closed).toBe(true)
    expect(body(calls.find((c) => c.url.endsWith('/api/child/message'))!)).toEqual({ messages: [{ role: 'user', text: 'x' }], conversation_id: 'activities/a' })
  })

  it('keeps a watcher open until unsubscribed', () => {
    const stop = api.watchChild('activities/a', () => {})
    expect(sources[0].closed).toBe(false)
    stop()
    expect(sources[0].closed).toBe(true)
  })
})
