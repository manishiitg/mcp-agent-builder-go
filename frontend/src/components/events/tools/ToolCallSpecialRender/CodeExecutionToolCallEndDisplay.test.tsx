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
})
