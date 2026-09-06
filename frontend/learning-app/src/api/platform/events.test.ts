// The mapper is fed events shaped exactly like the agent server's polling
// and SSE frames (captured from a real SparkQuill turn), and must rebuild
// the stored transcript the activity history reads.
import { describe, expect, it } from 'vitest'
import { messagesFromEvents, type PlatformEvent } from './events'

const SID = 'product-379ea1f1'
const main = (type: string, data: Record<string, unknown>, extra: Partial<PlatformEvent> = {}): PlatformEvent => ({
  id: `${SID}_${type}_${Math.random()}`, type, session_id: SID, execution_kind: 'main_agent', execution_id: `main:${SID}`,
  data: { type, hierarchy_level: 3, component: 'agent', data }, ...extra,
})

describe('messagesFromEvents', () => {
  // Shaped like /api/chat-history/sessions/{id} for a claude-code turn: the
  // narration, the tool call and the closing line are separate "ai" messages.
  it('rebuilds a restored claude-code conversation with the whole turn text', async () => {
    const { conversationToRestoredEvents } = await import('../../../../shared/session')
    const conversation = {
      session_id: SID,
      conversation_history: [
        { Role: 'system', Parts: [{ Type: 'text', Text: 'You are Quill.' }] },
        { Role: 'human', Parts: [{ Type: 'text', Text: 'hi, what can you help me with today?' }] },
        { Role: 'ai', Parts: [{ Type: 'text', Text: "Hi! I'm here to help you support Maya's grade 6 CBSE learning." }, { Type: 'function' }] },
        { Role: 'tool', Parts: [{ Type: 'text', Text: '{"ok":true}' }] },
        { Role: 'ai', Parts: [{ Type: 'text', Text: 'Three options are ready below to get started! 🌟' }] },
        { Role: 'human', Parts: [{ Type: 'text', Text: 'thanks' }] },
        { Role: 'ai', Parts: [{ Type: 'text', Text: 'Anytime!' }] },
      ],
    }
    const msgs = messagesFromEvents(conversationToRestoredEvents(conversation) as PlatformEvent[], SID)
    expect(msgs).toEqual([
      { role: 'user', text: 'hi, what can you help me with today?' },
      { role: 'assistant', text: "Hi! I'm here to help you support Maya's grade 6 CBSE learning.\n\nThree options are ready below to get started! 🌟" },
      { role: 'user', text: 'thanks' },
      { role: 'assistant', text: 'Anytime!' },
    ])
  })

  it('prefers the recorded turn text on live events and keeps product cards', () => {
    const events = [
      main('user_message', { content: 'celebrate her' }),
      main('llm_generation_end', { content: 'Done!', metadata: { assistant_turn_text: 'Cheering now.\n\nDone!' } }),
      main('product_interaction', { product: 'sparkquill', kind: 'celebrate', payload: { stars: 3, reason: 'fractions' } }),
      main('unified_completion', { final_result: 'Done!', status: 'completed' }),
    ]
    expect(messagesFromEvents(events, SID)).toEqual([
      { role: 'user', text: 'celebrate her' },
      { role: 'tool', tool: 'celebrate', stars: 3, reason: 'fractions' },
      { role: 'assistant', text: 'Cheering now.\n\nDone!' },
    ])
  })
})
