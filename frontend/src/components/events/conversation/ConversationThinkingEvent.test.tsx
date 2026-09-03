import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { ConversationThinkingEventDisplay } from './ConversationThinkingEvent'

describe('ConversationThinkingEventDisplay', () => {
  it('renders the thinking as plain text, not a labelled card', () => {
    const html = renderToStaticMarkup(
      <ConversationThinkingEventDisplay
        event={{ thinking: 'Checking the supplied video assets.', turn: 2 }}
      />,
    )

    expect(html).toContain('Checking the supplied video assets.')
    expect(html).not.toContain('Thinking')
    expect(html).not.toContain('Turn 2')
    expect(html).not.toContain('<button')
  })

  it('does not render an empty update', () => {
    expect(renderToStaticMarkup(<ConversationThinkingEventDisplay event={{ thinking: '  ' }} />)).toBe('')
  })
})
