import { describe, expect, it } from 'vitest'
import { findCommand, getCommands } from './registry'
import type { CommandContext } from './types'

describe('Pulse slash commands', () => {
  it('keeps workflow and Chief of Staff pulse setup commands mode-scoped', () => {
    const workflowCommand = findCommand('pulse-setup', 'workflow')
    const orgCommand = findCommand('pulse-setup', 'multi-agent')

    expect(workflowCommand?.description).toContain('recurring workflow run')
    expect(orgCommand?.description).toContain('Daily Org Pulse')
  })

  it('exposes manual Pulse modules only in workflow workshop mode', () => {
    const workflowCommands = getCommands('workflow', 'workshop').map(command => command.command)
    const orgCommands = getCommands('multi-agent').map(command => command.command)

    for (const command of ['pulse', 'bug-review', 'ops-review', 'strategy-auditor', 'pulse-fixer', 'goal-advisor']) {
      expect(workflowCommands).toContain(command)
      expect(orgCommands).not.toContain(command)
    }
    for (const retiredCommand of ['review-speed', 'review-cost', 'llm-ops-review']) {
      expect(workflowCommands).not.toContain(retiredCommand)
    }
  })

  it('routes the unified Ops Review command to the canonical backend guidance', () => {
    const command = findCommand('ops-review', 'workflow')
    let submitted = ''

    command?.execute({
      beforeSlash: 'check failed tool calls',
      onSubmit: (message: string) => { submitted = message },
      workshopMode: 'workshop',
      getWorkflowStore: () => ({ selectedRunFolder: 'iteration-8/default' }),
    } as CommandContext)

    expect(submitted).toContain('Run the /ops-review review as a BACKGROUND task')
    expect(submitted).toContain('kind=\\"ops-review\\"')
    expect(submitted).toContain('iteration-8/default')
    expect(submitted).toContain('check failed tool calls')
  })

  it('runs Strategy Auditor as a background guided review anchored to the selected run', () => {
    const command = findCommand('strategy-auditor', 'workflow')
    let submitted = ''

    command?.execute({
      beforeSlash: 'focus on repeated targets',
      onSubmit: (message: string) => { submitted = message },
      workshopMode: 'workshop',
      getWorkflowStore: () => ({ selectedRunFolder: 'iteration-7/group-a' }),
    } as CommandContext)

    expect(submitted).toContain('Run the /strategy-auditor review as a BACKGROUND task')
    expect(submitted).toContain('kind=\\"strategy-auditor\\"')
    expect(submitted).toContain('iteration-7/group-a')
    expect(submitted).toContain('focus on repeated targets')
  })

  it('routes Goal Advisor directly to its persisted native background pipeline', () => {
    const command = findCommand('goal-advisor', 'workflow')
    let submitted = ''

    command?.execute({
      beforeSlash: 'challenge feed concentration',
      onSubmit: (message: string) => { submitted = message },
      workshopMode: 'workshop',
    } as CommandContext)

    expect(submitted).toContain('run_goal_advisor_review')
    expect(submitted).toContain('challenge feed concentration')
    expect(submitted).toContain('do not wrap it in run_in_background')
    expect(submitted).toContain('persists its complete result and open CONCERNS in SQLite')
  })

  it('uses design-plan as the single comprehensive plan review command', () => {
    const workshopCommands = getCommands('workflow', 'workshop').map(command => command.command)
    const runCommands = getCommands('workflow', 'run').map(command => command.command)

    expect(workshopCommands).toContain('design-plan')
    expect(runCommands).toContain('design-plan')
    expect(workshopCommands).not.toContain('review-plan')
    expect(runCommands).not.toContain('review-plan')
  })

  it('keeps design-plan coordination in the main conversation', () => {
    const command = findCommand('design-plan', 'workflow')
    let submitted = ''

    command?.execute({
      beforeSlash: '',
      onSubmit: (message: string) => { submitted = message },
    } as CommandContext)

    expect(submitted).toContain('get_workflow_command_guidance')
    expect(submitted).not.toContain('Run the /design-plan review as a BACKGROUND task')
  })

  it('does not truncate background review results to a top three', () => {
    const command = findCommand('review-artifact-drift', 'workflow')
    let submitted = ''

    command?.execute({
      beforeSlash: '',
      onSubmit: (message: string) => { submitted = message },
      workshopMode: 'workshop',
    } as CommandContext)

    expect(submitted).toContain('every finding and recommendation in severity order')
    expect(submitted).toContain('Do not truncate the result to a Top 3')
  })

  it('keeps workspace configuration actions out of the Chief of Staff slash menu', () => {
    const orgCommands = getCommands('multi-agent').map(command => command.command)

    for (const command of ['build-skill', 'add-skill', 'mcp', 'mcp-add', 'models']) {
      expect(orgCommands).not.toContain(command)
    }
  })
})
