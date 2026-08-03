import { describe, expect, it } from 'vitest'
import type { PulseLoopClosureFinding } from '../../services/api-types'
import {
  pulseLoopClosureHeading,
  summarizePulseLoopSignals,
} from './pulseHeaderStatus'

function signal(kind: string): PulseLoopClosureFinding {
  return {
    kind,
    severity: 'high',
    subject: 'Subject',
    detail: 'Detail',
    evidence: 'Evidence',
    age_days: 1,
  }
}

describe('Pulse header ownership labels', () => {
  it('never treats an already answered decision as user work', () => {
    expect(summarizePulseLoopSignals([
      signal('answer_not_applied'),
      signal('decision_waiting_on_user'),
      signal('concern_keeps_recurring'),
      signal('future_signal'),
    ])).toEqual({
      needsUser: 1,
      awaitingPulse: 1,
      recurring: 1,
      other: 1,
    })
  })

  it('uses an ownership-specific heading for answered decisions', () => {
    expect(pulseLoopClosureHeading({ needsUser: 0, awaitingPulse: 1, recurring: 0, other: 0 }))
      .toBe('1 answered decision awaiting Pulse')
    expect(pulseLoopClosureHeading({ needsUser: 2, awaitingPulse: 0, recurring: 0, other: 0 }))
      .toBe('2 decisions need your answer')
  })
})
