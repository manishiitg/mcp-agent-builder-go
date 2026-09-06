// Live check against a running AgentWorks server with SparkQuill enabled:
//   SPARKQUILL_PLATFORM_URL=http://127.0.0.1:18790 npx vitest run platformApi.live
// Skipped when the variable is unset. Exercises the adapter's non-chat
// surface end to end (setup, engines, workspace through the proxy, state,
// activities, history). Turns run through the shared ChatArea and are
// covered by the desktop's live Electron pass, not here.
import { describe, expect, it } from 'vitest'
import { createPlatformApi } from './platformApi'

const url = (globalThis as { process?: { env?: Record<string, string | undefined> } }).process?.env?.SPARKQUILL_PLATFORM_URL

describe.skipIf(!url)('platformApi (live)', () => {
  it('serves setup, engines, the workspace and history', async () => {
    let saved: string | null = null
    const api = createPlatformApi({ baseUrl: url!, tokenStore: { get: () => saved, set: (t) => { saved = t } } })
    const setup = await api.setup()
    expect(['engine', 'child', 'pin', 'done']).toContain(setup.next_step)
    const engines = await api.engines()
    expect(engines.map((e) => e.id)).toContain('claude-code')

    // Workspace through the proxy: tree, reads, an upload, scene state, activities.
    const tree = await api.tree()
    const nodes = Array.isArray(tree) ? tree : (tree.nodes ?? [])
    expect(nodes.some((n) => n.path === 'family.json')).toBe(true)
    const family = await api.readFile('family.json')
    expect(family.is_text).toBe(true)
    const uploaded = await api.upload(new File(['hello from the live test'], 'live-test.txt', { type: 'text/plain' }), 'parent')
    expect(uploaded.error).toBeUndefined()
    expect(uploaded.path).toBe('inbox/live-test.txt')
    expect((await api.readFile('inbox/live-test.txt')).content).toBe('hello from the live test')
    await api.saveState('live:key', { score: 7 })
    expect(await api.loadState('live:key')).toEqual({ score: 7 })
    const activities = await api.activities()
    expect(Array.isArray(activities)).toBe(true)
    if (activities[0]) {
      const history = await api.loadChildConversation(activities[0].dir)
      expect(history === null || Array.isArray(history.messages)).toBe(true)
    }
    expect(api.rawUrl('inbox/live-test.txt')).toContain('/api/wp/api/documents/Chats/SparkQuill/inbox/live-test.txt/raw?token=')
    const raw = await fetch(api.rawUrl('inbox/live-test.txt'))
    expect(raw.status).toBe(200)
    expect(await raw.text()).toBe('hello from the live test')
  }, 2 * 60 * 1000)
})
