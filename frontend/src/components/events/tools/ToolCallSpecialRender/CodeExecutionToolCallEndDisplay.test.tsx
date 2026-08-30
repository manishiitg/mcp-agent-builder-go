import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'

vi.mock('../../../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}))
vi.mock('../../../ui/CsvRenderer', () => ({
  CsvRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}))
vi.mock('../../../ui/CircularProgress', () => ({
  CircularProgress: () => null,
}))
vi.mock('../../../ui/tooltip', () => ({
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))

import type { ToolCallEndEvent } from '../../../../generated/events'
import { CodeExecutionToolCallEndDisplay } from './CodeExecutionToolCallEndDisplay'

describe('CodeExecutionToolCallEndDisplay', () => {
  it('renders an object-valued MCP shell result without calling string methods on its content array', () => {
    const event: ToolCallEndEvent = {
      tool_name: 'execute_shell_command',
      result: {
        content: [{
          type: 'text',
          text: JSON.stringify({ stdout: 'ok\n', stderr: '', exit_code: 0 }),
        }],
        structured_content: null,
      } as unknown as string,
    }

    expect(() => renderToStaticMarkup(
      <CodeExecutionToolCallEndDisplay event={event} />,
    )).not.toThrow()
    expect(renderToStaticMarkup(
      <CodeExecutionToolCallEndDisplay event={event} />,
    )).toContain('Command Completed')
  })

  // PLAT-160. The backend's synthetic settle (the tool never reported its own
  // end, so the event store closed the chip itself) was rendered by this
  // component as an ordinary green "Command Completed" — the generic
  // ToolCallEndEvent.tsx already special-cases synthetic_settle, but this
  // shell-specific renderer never checked the flag at all.
  it('does not present a synthetic settle as an ordinary completion, even with a recovered result', () => {
    const event: ToolCallEndEvent = {
      tool_name: 'execute_shell_command',
      result: 'recovered stdout\n',
      turn: 0,
      duration: 5_000_000, // 5ms, a real recovered duration
      synthetic_settle: true,
    }

    const markup = renderToStaticMarkup(<CodeExecutionToolCallEndDisplay event={event} />)
    expect(markup).not.toContain('Command Completed')
    expect(markup).not.toContain('Turn: 0')
    expect(markup).toContain('recovered')
  })

  it('labels a synthetic settle with no recovered result as unreported, not as a measured duration', () => {
    const event: ToolCallEndEvent = {
      tool_name: 'execute_shell_command',
      result: '',
      duration: 45_000_000_000, // the open-to-settle wait, not a real duration
      synthetic_settle: true,
    }

    const markup = renderToStaticMarkup(<CodeExecutionToolCallEndDisplay event={event} />)
    expect(markup).not.toContain('Command Completed')
    expect(markup).toContain('not tool runtime')
  })
})
