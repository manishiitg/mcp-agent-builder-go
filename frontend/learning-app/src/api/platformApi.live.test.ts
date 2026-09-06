// Live check against a running AgentWorks server with SparkQuill enabled:
//   SPARKQUILL_PLATFORM_URL=http://127.0.0.1:18790 npx vitest run platformApi.live
// Skipped when the variable is unset. Runs a real parent turn and asserts
// the adapter produced what the UI needs from it.
import { describe, expect, it } from 'vitest'
import type { TurnStreamEvent } from './familyApi'

const url = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env?.SPARKQUILL_PLATFORM_URL

describe.skipIf(!url)('platformApi (live)', () => {
  it('runs a parent turn end to end', async () => {
    const { createPlatformApi } = await import('./platformApi')
    let saved: string | null = null
    const api = createPlatformApi({ baseUrl: url!, tokenStore: { get: () => saved, set: (t) => { saved = t } }, turnInactivityMs: 4 * 60 * 1000 })
    const setup = await api.setup()
    expect(['child', 'pin', 'done']).toContain(setup.next_step)
    const engines = await api.engines()
    expect(engines.map((e) => e.id)).toContain('claude-code')
    const before = await api.loadParentConversation()
    const events: TurnStreamEvent[] = []
    // A nonce in the reply proves this is THIS turn's completion, not a
    // replayed one from the session's history.
    const nonce = `PING-${Math.random().toString(36).slice(2, 8)}`
    const result = await api.sendParentTurn(
      { messages: [{ role: 'user', text: `Reply with exactly one short friendly line that includes the word ${nonce}, then call suggest_actions with two buttons.` }], conversationId: 'main' },
      (e) => events.push(e),
    )
    expect(result.error).toBeUndefined()
    expect((result.reply ?? '').length).toBeGreaterThan(0)
    expect(result.suggestions?.length ?? 0).toBeGreaterThanOrEqual(1)
    expect(events.some((e) => e.type === 'tool_call' || e.type === 'delta')).toBe(true)
    const after = await api.loadParentConversation()
    expect((after?.messages?.length ?? 0)).toBeGreaterThan(before?.messages?.length ?? 0)
    // Replay guard: the reply must be the LAST assistant message in the
    // rebuilt history, right after our own message -- an older completion
    // taken by mistake would sit earlier.
    const msgs = after?.messages ?? []
    const lastUser = [...msgs].reverse().find((m) => m.role === 'user')
    const lastAssistant = [...msgs].reverse().find((m) => m.role === 'assistant')
    expect(lastUser?.text).toContain(nonce)
    expect(lastAssistant?.text).toBe(result.reply)
    const steer = await api.steerParent('main', 'anything')
    expect(steer.steered).toBe(false) // nothing is running now

    // Workspace through the proxy: tree, reads, an upload, scene state, activities, the week.
    const tree = await api.tree()
    const nodes = Array.isArray(tree) ? tree : (tree.nodes ?? [])
    expect(nodes.some((n) => n.path === 'family.json')).toBe(true)
    const family = await api.readFile('family.json')
    expect(family.is_text).toBe(true)
    expect(JSON.parse(family.content ?? '{}').child?.name).toBeTruthy()
    const uploaded = await api.upload(new File(['hello from the live test'], 'live-test.txt', { type: 'text/plain' }), 'parent')
    expect(uploaded.error).toBeUndefined()
    expect(uploaded.path).toBe('inbox/live-test.txt')
    expect((await api.readFile('inbox/live-test.txt')).content).toBe('hello from the live test')
    await api.saveState('live:key', { score: 7 })
    expect(await api.loadState('live:key')).toEqual({ score: 7 })
    const activities = await api.activities()
    expect(Array.isArray(activities)).toBe(true)
    const week = await api.week(0)
    expect(week.days).toHaveLength(7)
    expect(api.rawUrl('inbox/live-test.txt')).toContain('/api/wp/api/documents/Chats/SparkQuill/inbox/live-test.txt/raw?token=')
    const raw = await fetch(api.rawUrl('inbox/live-test.txt'))
    expect(raw.status).toBe(200)
    expect(await raw.text()).toBe('hello from the live test')
    console.log('REPLY:', result.reply, '| suggestions:', JSON.stringify(result.suggestions), '| events:', events.length)
  }, 5 * 60 * 1000)
})
