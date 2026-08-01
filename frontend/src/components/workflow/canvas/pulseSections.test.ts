import { describe, expect, it } from 'vitest'
import { PULSE_FIXED_COMMANDS, PULSE_MODULE_COMMANDS } from './pulseSections'

describe('Pulse workspace registry', () => {
  it('exposes the complete database-native review module set once', () => {
    const moduleIds = PULSE_MODULE_COMMANDS.map(module => module.id)
    expect(moduleIds).toEqual([
      'bug_review',
      'artifact_review',
      'report_health',
      'eval_health',
      'stores_health',
      'llm_ops_review',
      'strategy_auditor',
      'goal_advisor',
    ])
    expect(new Set(moduleIds).size).toBe(moduleIds.length)
  })

  it('exposes the complete finalization command set once', () => {
    const commandIds = PULSE_FIXED_COMMANDS.map(command => command.id)
    expect(commandIds).toEqual(['dashboard', 'backup', 'publish', 'notify'])
    expect(new Set(commandIds).size).toBe(commandIds.length)
  })

  it('keeps Strategy Auditor before Goal Advisor escalation', () => {
    const moduleIds = PULSE_MODULE_COMMANDS.map(module => module.id)
    expect(moduleIds.indexOf('strategy_auditor')).toBeLessThan(moduleIds.indexOf('goal_advisor'))
  })
})
