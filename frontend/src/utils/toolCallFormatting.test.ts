import { describe, expect, it } from 'vitest'
import { formatToolCallArguments, formatToolCallResult } from './toolCallFormatting'

describe('formatToolCallArguments', () => {
  it('pretty-prints JSON arguments', () => {
    expect(formatToolCallArguments('{"command":"echo ok","timeout":30}')).toEqual({
      format: 'json',
      text: '{\n  "command": "echo ok",\n  "timeout": 30\n}',
      isError: false,
    })
  })

  it('leaves non-JSON arguments unchanged', () => {
    expect(formatToolCallArguments('plain text')).toEqual({
      format: 'text',
      text: 'plain text',
      isError: false,
    })
  })
})

describe('formatToolCallResult', () => {
  it('unwraps an MCP text envelope and formats JSON stdout', () => {
    const shellResult = JSON.stringify({
      stdout: JSON.stringify({
        top_type: 'object',
        length: 2,
        keys: ['result', 'success'],
      }),
      stderr: '',
      exit_code: 0,
      execution_time_ms: 27,
    })
    const mcpEnvelope = JSON.stringify({
      content: [{ type: 'text', text: shellResult }],
      structured_content: null,
    })

    expect(formatToolCallResult(mcpEnvelope)).toEqual({
      format: 'shell',
      text: 'exit code 0 · 27ms\n\nstdout\n{\n  "top_type": "object",\n  "length": 2,\n  "keys": [\n    "result",\n    "success"\n  ]\n}',
      isError: false,
    })
  })

  it('keeps multiline shell stdout readable and omits empty stderr', () => {
    const result = JSON.stringify({
      content: [{
        type: 'text',
        text: JSON.stringify({
          stdout: 'post-run-monitor 880\nassumption-audit 92\nreview-improve-log 447\n',
          stderr: '',
          exit_code: 0,
          execution_time_ms: 72,
        }),
      }],
      structured_content: null,
    })

    expect(formatToolCallResult(result)).toEqual({
      format: 'shell',
      text: 'exit code 0 · 72ms\n\nstdout\npost-run-monitor 880\nassumption-audit 92\nreview-improve-log 447\n',
      isError: false,
    })
  })

  it('expands JSON strings nested inside ordinary JSON results', () => {
    const result = JSON.stringify({
      success: true,
      result: JSON.stringify({ reference: 'PATH DISCIPLINE' }),
    })

    expect(formatToolCallResult(result)).toEqual({
      format: 'json',
      text: '{\n  "success": true,\n  "result": {\n    "reference": "PATH DISCIPLINE"\n  }\n}',
      isError: false,
    })
  })

  it('leaves plain-text results unchanged', () => {
    expect(formatToolCallResult('done')).toEqual({
      format: 'text',
      text: 'done',
      isError: false,
    })
  })

  it('marks a non-zero nested shell exit as an error and preserves stderr', () => {
    const result = JSON.stringify({
      content: [{
        type: 'text',
        text: JSON.stringify({
          stdout: '',
          stderr: 'Error: in prepare, unable to open database file (14)\n',
          exit_code: 14,
          execution_time_ms: 35,
        }),
      }],
      structured_content: null,
    })

    expect(formatToolCallResult(result)).toEqual({
      format: 'shell',
      text: 'exit code 14 · 35ms\n\nstderr\nError: in prepare, unable to open database file (14)\n',
      isError: true,
    })
  })
})
