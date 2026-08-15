import { describe, expect, it } from 'vitest'
import { shouldKeepWorkflowSessionSubscribed } from './workflowSessionSubscription'

describe('shouldKeepWorkflowSessionSubscribed', () => {
  it('keeps listening after the foreground turn settles while the backend session is active', () => {
    expect(shouldKeepWorkflowSessionSubscribed({
      isStreaming: false,
      hasRunningBackgroundAgents: false,
      isBackendActive: true,
    })).toBe(true)
  })

  it('keeps listening for foreground and background activity', () => {
    expect(shouldKeepWorkflowSessionSubscribed({
      isStreaming: true,
      hasRunningBackgroundAgents: false,
      isBackendActive: false,
    })).toBe(true)
    expect(shouldKeepWorkflowSessionSubscribed({
      isStreaming: false,
      hasRunningBackgroundAgents: true,
      isBackendActive: false,
    })).toBe(true)
  })

  it('allows a genuinely idle workflow session to disconnect', () => {
    expect(shouldKeepWorkflowSessionSubscribed({
      isStreaming: false,
      hasRunningBackgroundAgents: false,
      isBackendActive: false,
    })).toBe(false)
  })
})
