import { describe, expect, it } from 'vitest'
import type { PollingEvent } from '../services/api-types'
import { buildCleanConversationItems, buildProductionActivityItems, cleanConversationActivity } from '../utils/cleanConversation'

function event(id: string, type: string, data: Record<string, unknown>, extra: Partial<PollingEvent> = {}): PollingEvent {
  return {
    id,
    type,
    timestamp: '2026-08-07T12:00:00.000Z',
    data: { type, data } as PollingEvent['data'],
    ...extra,
  }
}

describe('buildCleanConversationItems', () => {
  it('keeps the human conversation, surfaces auto-notifications, and removes other internal runtime messages', () => {
    const items = buildCleanConversationItems([
      event('user-1', 'user_message', {
        content: 'Make a launch video\n\nPrevious workflow-builder conversation file: Chats/internal.md',
      }),
      event('notice', 'user_message', { content: '[AUTO-NOTIFICATION] internal runtime update' }),
      event('child', 'unified_completion', { final_result: 'Worker internals' }, { execution_kind: 'background_agent' }),
      event('done', 'unified_completion', { final_result: 'Your launch video is ready.' }, { execution_kind: 'main_agent' }),
    ])

    expect(items).toEqual([
      expect.objectContaining({ role: 'user', content: 'Make a launch video' }),
      expect.objectContaining({ role: 'notification', content: 'internal runtime update' }),
      expect.objectContaining({ role: 'assistant', content: 'Your launch video is ready.' }),
    ])
  })

  it('deduplicates echoed user messages and repeated final answers', () => {
    const items = buildCleanConversationItems([
      event('user-local', 'user_message', { content: 'Create it' }),
      event('user-echo', 'user_message', { content: 'Create it' }),
      event('done-1', 'conversation_end', { result: 'Finished.' }),
      event('done-2', 'unified_completion', { final_result: 'Finished.' }),
    ])

    expect(items.map((item) => `${item.role}:${item.content}`)).toEqual([
      'user:Create it',
      'assistant:Finished.',
    ])
  })

  it('keeps structured reasoning separate from the final answer', () => {
    const items = buildCleanConversationItems([
      event('user', 'user_message', { content: 'Create the teaser' }),
      event('thinking', 'conversation_thinking', { thinking: 'I am checking the supplied assets before choosing the production path.' }),
      event('done', 'conversation_end', { result: 'The teaser plan is ready.' }),
    ])

    expect(items.map((item) => `${item.role}:${item.content}`)).toEqual([
      'user:Create the teaser',
      'reasoning:I am checking the supplied assets before choosing the production path.',
      'assistant:The teaser plan is ready.',
    ])
  })

  it('attaches native structured token usage to the completed assistant answer', () => {
    const items = buildCleanConversationItems([
      event('user', 'user_message', { content: 'Create the teaser' }),
      event('done', 'unified_completion', { final_result: 'The teaser is ready.' }),
      event('usage', 'token_usage', {
        context: 'conversation_total',
        prompt_tokens: 1234,
        completion_tokens: 56,
        generation_info: { cumulative_cache_tokens: 789 },
      }),
    ])

    expect(items.at(-1)).toEqual(expect.objectContaining({
      role: 'assistant',
      usage: { inputTokens: 1234, outputTokens: 56, cacheTokens: 789, isEstimated: false },
    }))
  })

  it('removes delayed backend echoes without hiding an intentional repeated prompt', () => {
    const items = buildCleanConversationItems([
      event('user-message-local-1', 'user_message', { content: 'Check the report' }),
      event('user-message-local-steer', 'user_message', { content: 'Include the dimensions' }),
      event('backend-echo-1', 'user_message', { content: 'Check the report' }),
      event('backend-echo-steer', 'user_message', { content: 'Include the dimensions' }),
      event('user-message-local-2', 'user_message', { content: 'Check the report' }),
      event('backend-echo-2', 'user_message', { content: 'Check the report' }),
    ])

    expect(items.map((item) => item.content)).toEqual([
      'Check the report',
      'Include the dimensions',
      'Check the report',
    ])
  })

  it('turns cancellation into a simple product-facing message', () => {
    const items = buildCleanConversationItems([
      event('user', 'user_message', { content: 'Create it' }),
      event('cancel', 'context_cancelled', {}),
      event('cancel-echo', 'context_cancelled', {}),
    ])

    expect(items).toHaveLength(2)
    expect(items.at(-1)).toEqual(expect.objectContaining({
      role: 'error',
      content: 'The current response was cancelled.',
    }))
  })
})

describe('cleanConversationActivity', () => {
  it('uses friendly workflow and presentation labels instead of tool names', () => {
    expect(cleanConversationActivity([
      event('workflow', 'tool_call_start', { tool_name: 'run_full_workflow' }),
    ], 'Creating')).toBe('Running the production workflow')

    expect(cleanConversationActivity([
      event('video', 'tool_call_start', { tool_name: 'show_video' }),
    ], 'Creating')).toBe('Preparing the finished video')
  })

  it('turns low-level QA tools into product progress', () => {
    expect(cleanConversationActivity([
      event('skill', 'tool_call_start', { tool_name: 'read_skill' }),
    ], 'Creating')).toBe('Preparing the production checklist')

    expect(cleanConversationActivity([
      event('image', 'tool_call_start', { tool_name: 'read_image' }),
    ], 'Creating')).toBe('Reviewing the video frames')

    expect(cleanConversationActivity([
      event('shell', 'tool_call_start', { tool_name: 'execute_shell_command' }),
    ], 'Creating')).toBe('Checking the video output')
  })
})

describe('buildProductionActivityItems', () => {
  it('pairs live tool calls and exposes product-safe status details', () => {
    const items = buildProductionActivityItems([
      event('user', 'user_message', { content: 'Make the video' }),
      event('tool-start', 'tool_call_start', { tool_call_id: 'call-1', tool_name: 'execute_shell_command' }),
      event('tool-end', 'tool_call_end', { tool_call_id: 'call-1', tool_name: 'execute_shell_command' }),
    ])

    expect(items).toEqual([
      expect.objectContaining({ title: 'execute_shell_command', detail: 'Tool call', status: 'complete', kind: 'tool' }),
    ])
  })

  it('collapses wrapper and provider lifecycle events into one finished activity', () => {
    const items = buildProductionActivityItems([
      event('user', 'user_message', { content: 'Check the video' }),
      event('wrapper-start', 'tool_call_start', { tool_call_id: 'wrapper', tool_name: 'exec' }),
      event('wrapper-error', 'tool_call_error', { tool_call_id: 'wrapper-error', tool_name: 'exec' }),
      event('provider-error', 'tool_call_error', { tool_call_id: 'provider-error', tool_name: 'execute_shell_command' }),
      event('provider-end', 'tool_call_end', { tool_call_id: 'provider', tool_name: 'execute_shell_command' }),
    ])

    expect(items).toEqual([
      expect.objectContaining({ title: 'execute_shell_command', detail: 'Tool call', status: 'complete', kind: 'tool' }),
    ])
  })

  it('keeps workflow tools literal but gives them workflow context', () => {
    const items = buildProductionActivityItems([
      event('user', 'user_message', { content: 'Run the full workflow' }),
      event('workflow', 'tool_call_start', { tool_call_id: 'workflow-1', tool_name: 'run_full_workflow' }),
    ])

    expect(items).toEqual([
      expect.objectContaining({ title: 'run_full_workflow', detail: 'Workflow: running the full production workflow', status: 'running' }),
    ])
  })

  it('keeps raw tool details for developer inspection while redacting credentials', () => {
    const items = buildProductionActivityItems([
      event('user', 'user_message', { content: 'Inspect the tool' }),
      event('tool-start', 'tool_call_start', {
        tool_call_id: 'tool-1', tool_name: 'get_api_spec',
        tool_params: { arguments: '{"tool_name":"run_full_workflow","token":"should-not-show"}' },
      }),
      event('tool-end', 'tool_call_end', {
        tool_call_id: 'tool-1', tool_name: 'get_api_spec', result: 'Bearer super-secret-token',
      }),
    ])

    expect(items.at(-1)).toEqual(expect.objectContaining({
      title: 'get_api_spec',
      arguments: '{"tool_name":"run_full_workflow","token":"[redacted]"}',
      result: 'Bearer [redacted]',
    }))
  })

  it('shows the underlying Cursor MCP tool instead of its CallMcpTool wrapper', () => {
    const items = buildProductionActivityItems([
      event('user', 'user_message', { content: 'Inspect the project' }),
      event('cursor-tool', 'tool_call_start', {
        tool_call_id: 'cursor-1',
        tool_name: 'CallMcpTool',
        tool_params: { arguments: {
          serverIdentifier: 'api-bridge',
          toolName: 'get_api_spec',
          args: { tool_name: 'show_video' },
        } },
      }),
    ])

    expect(items).toEqual([
      expect.objectContaining({
        title: 'get_api_spec',
        arguments: '{"tool_name":"show_video"}',
      }),
    ])
  })

  it('shows the selected workflow route without exposing routing internals', () => {
    const items = buildProductionActivityItems([
      event('route', 'routing_evaluated', {
        routing_response: { selected_route_id: 'infographic' },
        routes: [{ route_id: 'infographic', route_name: 'Product infographic' }],
      }),
    ])

    expect(items.at(-1)).toEqual(expect.objectContaining({
      title: 'Choose production path',
      detail: 'Product infographic',
      status: 'complete',
    }))
  })
})
