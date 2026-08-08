import React from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it, vi } from 'vitest'

vi.mock('../../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}))
vi.mock('../../ui/CsvRenderer', () => ({
  CsvRenderer: ({ content }: { content: string }) => <div>{content}</div>,
}))
vi.mock('../../ui/CircularProgress', () => ({
  CircularProgress: () => null,
}))
vi.mock('../../ui/tooltip', () => ({
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}))
vi.mock('./ToolCallSpecialRender', () => ({
  WorkspaceToolCallEndDisplay: () => null,
  CodeExecutionToolCallEndDisplay: () => null,
}))
vi.mock('./ToolCallSpecialRender/ImageGenToolCallEndDisplay', () => ({
  ImageGenToolCallEndDisplay: () => null,
}))

import type { ToolCallEndEvent } from '../../../generated/events'
import { ToolCallEndEventDisplay } from './ToolCallEndEvent'

describe('ToolCallEndEventDisplay', () => {
  it('marks a bridge failure red on the generic custom-tool path', () => {
    const event: ToolCallEndEvent = {
      tool_name: 'record_pulse_result',
      result: JSON.stringify({
        stdout: 'ERROR: tool execution failed: layer=custom_tool_handler tool=record_pulse_result: invalid payload',
        stderr: '',
        exit_code: 0,
      }),
    }

    const markup = renderToStaticMarkup(<ToolCallEndEventDisplay event={event} />)
    expect(markup).toContain('Tool Call Failed')
    expect(markup).toContain('border-red-300')
  })
})
