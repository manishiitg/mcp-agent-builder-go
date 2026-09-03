// Live check against a running AgentWorks server with SparkQuill enabled:
//   SPARKQUILL_PLATFORM_URL=http://127.0.0.1:18790 npx vitest run platformApi.live
// Skipped when the variable is unset. Runs a real parent turn and asserts
// the adapter produced what the UI needs from it.
import { describe, expect, it } from 'vitest'
import type { TurnStreamEvent } from './familyApi'
import { createPlatformApi } from './platformApi'

const url = process.env.SPARKQUILL_PLATFORM_URL

describe.skipIf(!url)('platformApi (live)', () => {
  it('runs a parent turn end to end', async () => {
    let saved: string | null = null
    const api = createPlatformApi({ baseUrl: url!, tokenStore: { get: () => saved, set: (t) => { saved = t } }, turnInactivityMs: 4 * 60 * 1000 })
    const setup = await api.setup()
    expect(['child', 'pin', 'done']).toContain(setup.next_step)
    const engines = await api.engines()
    expect(engines.map((e) => e.id)).toContain('claude-code')
    const before = await api.loadParentConversation()
    const events: TurnStreamEvent[] = []
    const result = await api.sendParentTurn(
      { messages: [{ role: 'user', text: 'Reply with exactly one short friendly line, then call suggest_actions with two buttons.' }], conversationId: 'main' },
      (e) => events.push(e),
    )
    expect(result.error).toBeUndefined()
    expect((result.reply ?? '').length).toBeGreaterThan(0)
    expect(result.suggestions?.length ?? 0).toBeGreaterThanOrEqual(1)
    expect(events.some((e) => e.type === 'tool_call' || e.type === 'delta')).toBe(true)
    const after = await api.loadParentConversation()
    expect((after?.messages?.length ?? 0)).toBeGreaterThan(before?.messages?.length ?? 0)
    const steer = await api.steerParent('main', 'anything')
    expect(steer.steered).toBe(false) // nothing is running now
    console.log('REPLY:', result.reply, '| suggestions:', JSON.stringify(result.suggestions), '| events:', events.length)
  }, 5 * 60 * 1000)
})
