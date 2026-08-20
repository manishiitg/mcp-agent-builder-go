import { describe, expect, it } from 'vitest'
import type { ChatHistorySession } from '../services/api-types'
import { chatHistoryOpenDisposition } from '../utils/chatHistoryOpenDisposition'
import { isProviderTranscriptArtifact } from '../utils/restoredConversationFilter'

function session(overrides: Partial<ChatHistorySession>): ChatHistorySession {
  return {
    session_id: 'chat-1',
    created_at: '2026-08-20T00:00:00Z',
    updated_at: '2026-08-20T00:00:00Z',
    ...overrides,
  }
}

describe('chatHistoryOpenDisposition', () => {
  it('keeps a scheduled coding-agent tmux session read-only', () => {
    expect(chatHistoryOpenDisposition(session({
      session_id: 'schedule-manual--37c45de4_1787214437488081000',
      runtime: {
        kind: 'coding_agent',
        transport: 'tmux',
        resume_supported: true,
      },
    }))).toBe('read-only-schedule')
  })

  it('resumes an ordinary retained coding-agent chat through its transport', () => {
    expect(chatHistoryOpenDisposition(session({
      session_id: 'chat-retained-1',
      runtime: {
        kind: 'coding_agent',
        transport: 'tmux',
        resume_supported: true,
      },
    }))).toBe('interactive-transport')
  })
})

describe('restored schedule transcript', () => {
  it('does not render a canceled provider tool dump as assistant prose', () => {
    expect(isProviderTranscriptArtifact(
      '[Canceled run context — tools executed before cancellation:\n- execute_shell_command({"command":"git status"}) -> huge output]'
    )).toBe(true)
  })
})
