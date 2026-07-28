import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { UserMessageEventDisplay } from './UserMessageEvent'

describe('UserMessageEventDisplay', () => {
  it('renders generated structured input as a compact task instead of a human message', () => {
    const html = renderToStaticMarkup(
      <UserMessageEventDisplay
        event={{
          content: '## Orchestrator Instructions\nInternal generated prompt',
          role: 'user',
          turn: 0,
          metadata: {
            source: 'execution_prompt',
            step_name: 'Nested Word Task',
          },
        }}
      />,
    )

    expect(html).toContain('data-testid="terminal-execution-prompt"')
    expect(html).toContain('Nested Word Task')
    expect(html).toContain('Instructions')
    expect(html).not.toContain('No message content')
    // The instructions ARE the task: rendering the card with them collapsed
    // left a header with nothing under it, so the details element must open by
    // default and the prompt text must be present without interaction.
    expect(html).toContain('<details open')
    expect(html).toContain('Internal generated prompt')
  })

  it('keeps live user input visually distinct from execution prompts', () => {
    const html = renderToStaticMarkup(
      <UserMessageEventDisplay
        event={{
          content: 'Please check this result',
          role: 'user',
          metadata: { source: 'coding_agent_live_input' },
        }}
      />,
    )

    expect(html).toContain('Please check this result')
    expect(html).not.toContain('terminal-execution-prompt')
  })

  it('recognizes persisted step-scoped prompts created before source metadata existed', () => {
    const html = renderToStaticMarkup(
      <UserMessageEventDisplay
        event={{
          content: 'Legacy generated executor prompt',
          role: 'user',
          turn: 0,
          metadata: {
            current_step_id: 'nested-word-task',
            step_name: 'Nested Word Task',
          },
        }}
      />,
    )

    expect(html).toContain('terminal-execution-prompt')
    expect(html).toContain('Nested Word Task')
  })
})
