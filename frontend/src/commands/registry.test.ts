import { describe, expect, it, vi } from 'vitest'
import { findCommand, getCommands } from './registry'
import type { CommandContext } from './types'

const { runPulseMock } = vi.hoisted(() => ({ runPulseMock: vi.fn() }))
vi.mock('../api/scheduler', () => ({ schedulerApi: { runPulse: runPulseMock } }))

describe('Pulse slash commands', () => {
  it('runs the complete manual Pulse lifecycle through the scheduler backend', async () => {
    runPulseMock.mockResolvedValueOnce({ run_id: 'manual-pulse-1' })
    const addToast = vi.fn()
    const command = findCommand('pulse', 'workflow')

    await command?.execute({
      beforeSlash: '',
      onSubmit: vi.fn(),
      workshopMode: 'workshop',
      getWorkspaceStore: () => ({ activeFolder: 'Workflow/social-media' }),
      addToast,
    } as unknown as CommandContext)

    expect(runPulseMock).toHaveBeenCalledWith('Workflow/social-media')
    expect(addToast).toHaveBeenCalledWith('Pulse started', 'success')
  })

  it('has no slash command for recurring Pulse setup — it is a toolbar/popup toggle', () => {
    const workflowCommand = findCommand('pulse-setup', 'workflow')
    const orgCommand = findCommand('pulse-setup', 'multi-agent')

    expect(workflowCommand).toBeUndefined()
    expect(orgCommand).toBeUndefined()
  })

  it('exposes manual Pulse modules in workflow workshop mode', () => {
    const workflowCommands = getCommands('workflow', 'workshop').map(command => command.command)
    const orgCommands = getCommands('multi-agent').map(command => command.command)

    for (const command of [
      'pulse', 'pulse-backlog', 'pulse-review', 'pulse-fixer', 'goal-advisor',
      'pulse-review-knowledge', 'pulse-review-learnings', 'pulse-review-database',
      'pulse-review-execution-health', 'plan-prompt-bloat', 'pulse-review-validation-contract', 'pulse-review-report-quality', 'pulse-review-evaluation-quality', 'pulse-review-model-cost',
    ]) {
      expect(workflowCommands).toContain(command)
      expect(orgCommands).not.toContain(command)
    }
    for (const retiredCommand of ['bug-review', 'review-speed', 'review-cost', 'llm-ops-review', 'ops-review', 'engineering-review', 'specialize-advisors', 'pulse-setup', 'improve-knowledge', 'improve-learnings', 'improve-database', 'improve-report', 'improve-evaluation', 'pulse-review-stores', 'pulse-review-report', 'pulse-review-evaluation']) {
      expect(workflowCommands).not.toContain(retiredCommand)
    }
  })

  it('keeps focused Pulse reviews discoverable from the execution-log run view', () => {
    const runCommands = getCommands('workflow', 'run').map(command => command.command)

    for (const command of ['pulse-review-execution-health', 'plan-prompt-bloat', 'pulse-review-validation-contract']) {
      expect(runCommands).toContain(command)
    }
  })

  it('makes prompt-bloat review discoverable when searching Pulse commands', () => {
    const promptBloat = findCommand('plan-prompt-bloat', 'workflow')

    expect(promptBloat?.description.toLowerCase()).toContain('pulse review')
  })

  it('runs backlog consolidation through typed Pulse lifecycle tools only', () => {
    const command = findCommand('pulse-backlog', 'workflow')
    let submitted = ''
    command?.execute({
      beforeSlash: 'focus on repeated database tool symptoms',
      onSubmit: (message: string) => { submitted = message },
      workshopMode: 'workshop',
    } as CommandContext)

    expect(submitted).toContain('get_pulse_state(view="backlog", detail="compact")')
    expect(submitted).toContain('detail="full" only for the bounded issue_ids')
    expect(submitted).toContain('merge_pulse_issues')
    expect(submitted).toContain('do not edit workflow artifacts')
  })

  it('routes Pulse Review through one retained review and fix task', () => {
    const command = findCommand('pulse-review', 'workflow')
    let submitted = ''

    command?.execute({
      beforeSlash: 'prioritize failed evaluation writes',
      onSubmit: (message: string) => { submitted = message },
      workshopMode: 'workshop',
      getWorkflowStore: () => ({ selectedRunFolder: 'iteration-9/default' }),
    } as CommandContext)

    expect(submitted).toContain('kind=\\"engineering-review\\"')
    expect(submitted).toContain('Run /pulse-review as a BACKGROUND task')
    expect(submitted).toContain('BACKGROUND task')
    expect(submitted).toContain('completion_mode="present_result"')
		expect(submitted).not.toContain('required_pulse_review_modules')
		expect(submitted).toContain('Do not call tools, reload state, or independently revalidate')
    expect(submitted).toContain('iteration-9/default')
    expect(submitted).toContain('prioritize failed evaluation writes')
  })

  it('routes Pulse Fixer to a separate background agent after review', () => {
    const command = findCommand('pulse-fixer', 'workflow')
    let submitted = ''

    command?.execute({
      beforeSlash: 'repair the highest-impact canonical issue',
      onSubmit: (message: string) => { submitted = message },
      workshopMode: 'workshop',
      getWorkflowStore: () => ({ selectedRunFolder: 'iteration-9/default' }),
    } as CommandContext)

    expect(submitted).toContain('kind=\\"pulse-fixer\\"')
    expect(submitted).toContain('BACKGROUND task')
		expect(submitted).toContain('completion_mode="present_result"')
    expect(submitted).toContain('iteration-9/default')
  })

  it('routes a manual technical focus through retained Technical Review and Fix', () => {
    const technical = findCommand('pulse-review-execution-health', 'workflow')
    let submitted = ''

    technical?.execute({
      beforeSlash: 'check the newest retry spike',
      onSubmit: (message: string) => { submitted = message },
      workshopMode: 'workshop',
      getWorkflowStore: () => ({ selectedRunFolder: 'iteration-9/default' }),
    } as CommandContext)
    expect(submitted).toContain('kind=\\"engineering-review\\"')
    expect(submitted).toContain('Manual Pulse review focus: execution_health')
    expect(submitted).not.toContain('required_pulse_review_modules')
		expect(submitted).toContain('bounded Review+Fix')
  })

  it('routes store review aliases through store_integrity review and bounded repair', () => {
    for (const commandName of ['pulse-review-knowledge', 'pulse-review-learnings', 'pulse-review-database']) {
      const command = findCommand(commandName, 'workflow')
      let submitted = ''
      command?.execute({
        beforeSlash: 'repair confirmed ownership drift',
        onSubmit: (message: string) => { submitted = message },
        workshopMode: 'workshop',
        getWorkflowStore: () => ({ selectedRunFolder: 'iteration-4/default' }),
      } as CommandContext)

      expect(submitted).toContain('kind=\\"engineering-review\\"')
      expect(submitted).toContain('Manual Pulse review focus: store_integrity')
		expect(submitted).toContain('bounded Review+Fix')
      expect(submitted).toContain('iteration-4/default')
    }
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
    expect(submitted).not.toContain('required_pulse_review_modules')
    expect(submitted).toContain('iteration-7/group-a')
    expect(submitted).toContain('focus on repeated targets')
  })

  it('routes Goal Advisor through the normal guided background review path', () => {
    const command = findCommand('goal-advisor', 'workflow')
    let submitted = ''

    command?.execute({
      beforeSlash: 'challenge feed concentration',
      onSubmit: (message: string) => { submitted = message },
      workshopMode: 'workshop',
    } as CommandContext)

    expect(submitted).toContain('get_workflow_command_guidance')
    expect(submitted).toContain('challenge feed concentration')
    expect(submitted).toContain('BACKGROUND task')
    expect(submitted).toContain('run_in_background')
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

  it('keeps workflow configuration actions out of the generic chat slash menu', () => {
    const chatCommands = getCommands('multi-agent').map(command => command.command)

    for (const command of ['build-skill', 'add-skill', 'mcp', 'mcp-add', 'models']) {
      expect(chatCommands).not.toContain(command)
    }
  })
})

describe('Product commands are scoped to their own surface', () => {
  it('shows a product\'s own commands once registered, and clears them when unregistered', async () => {
    // Product commands live in product.yaml and are delivered by the owning
    // product surface, then cleared on unmount. A profile-less chat does
    // not inherit those commands.
    const { setProductCommands } = await import('./registry')
    setProductCommands([{
      command: 'production', description: 'Start a video production', icon: null,
      modes: ['multi-agent'], source: 'product', execute: () => {},
    } as unknown as import('./types').CommandDefinition])
    try {
      const commands = getCommands('multi-agent').map(command => command.command)
      expect(commands).toContain('production')
    } finally {
      setProductCommands([])
    }
    expect(getCommands('multi-agent').map(command => command.command)).not.toContain('production')
  })
})
