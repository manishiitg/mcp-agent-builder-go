// Which events belong to the session's own foreground agent, as opposed to
// a delegated sub-agent, a workshop step, or another execution. Lifted out
// of AgentWorks' ChatArea so every app applies the same rule.
import type { PollingEvent } from './types'

export function isForegroundSessionEvent(
  event: PollingEvent,
  component: unknown,
  correlationId: unknown,
): boolean {
  const componentText = typeof component === 'string' ? component : ''
  const correlationText = typeof correlationId === 'string' ? correlationId : ''
  if (
    componentText.startsWith('delegation-') ||
    componentText.startsWith('workshop-') ||
    correlationText.startsWith('delegation-') ||
    correlationText.startsWith('workshop-')
  ) {
    return false
  }

  const kind = (event.execution_kind || '').trim().toLowerCase()
  if (kind && kind !== 'main_agent') {
    return false
  }

  const executionId = (event.execution_id || '').trim().toLowerCase()
  if (!executionId || executionId.startsWith('main:')) {
    return true
  }

  return false
}
