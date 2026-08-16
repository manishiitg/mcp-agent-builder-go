import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { ConversationThinkingEventDisplay } from './ConversationThinkingEvent'

describe('ConversationThinkingEventDisplay', () => {
  it('renders a readable structured thinking update', () => {
    const html = renderToStaticMarkup(
      <ConversationThinkingEventDisplay
        event={{ thinking: 'Checking the supplied video assets.', turn: 2 }}
      />,
    )

    expect(html).toContain('Thinking')
    expect(html).toContain('Checking the supplied video assets.')
    expect(html).toContain('Turn 2')
  })

  it('does not render an empty update', () => {
    expect(renderToStaticMarkup(<ConversationThinkingEventDisplay event={{ thinking: '  ' }} />)).toBe('')
  })
})
