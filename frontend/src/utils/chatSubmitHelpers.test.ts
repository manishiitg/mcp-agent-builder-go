import { describe, expect, it } from 'vitest'
import type { AgentQueryRequest } from '../services/api-types'
import type { ChatTab } from '../stores/useChatStore'
import { applyAgentProfileBinding } from './chatSubmitHelpers'

describe('agent profile query binding', () => {
  it('pins a product query to its profile version and workspace', () => {
    const payload: AgentQueryRequest = { query: 'Make a launch film', agent_mode: 'multi-agent' }
    const tab = {
      name: 'Launch film',
      metadata: {
        mode: 'multi-agent',
        agentProfileId: 'video-studio',
        agentProfileVersion: 1,
        agentProfileWorkspace: 'Chats/Video Studio/projects/launch-film',
        agentProfileProjectTitle: 'Launch film',
        agentProfileWorkspaceDescription: 'A 30 second product launch film.',
      },
    } as ChatTab

    expect(applyAgentProfileBinding(payload, tab)).toEqual({
      ...payload,
      agent_profile_id: 'video-studio',
      agent_profile_version: 1,
      selected_folder: 'Chats/Video Studio/projects/launch-film',
      agent_profile_context: {
        project_title: 'Launch film',
        workspace_description: 'A 30 second product launch film.',
      },
    })
    expect(payload.agent_profile_id).toBeUndefined()
  })
})
