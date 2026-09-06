import type { ChatHistorySession } from '../services/api-types'

// Resume a workflow conversation with the same prompt/tool policy that created
// it. Missing mode metadata is from legacy Builder chats, before Run chat
// existed, and therefore resolves to Workshop.
export function chatHistoryWorkshopMode(session: ChatHistorySession): 'workshop' | 'run' {
  const raw = (session.runtime?.workshop_mode || session.workshop_mode || '').trim().toLowerCase()
  return raw === 'run' || raw === 'ask' || raw === 'debugger' || raw === 'runner'
    ? 'run'
    : 'workshop'
}
