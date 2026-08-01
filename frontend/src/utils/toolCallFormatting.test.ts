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

describe('bridge tool failures that exit 0', () => {
  // A tool that fails behind the HTTP bridge returns its error as ordinary
  // stdout: the curl succeeded, so exit_code is 0 and every exit-code check
  // reports success. One day of codex rollouts (2026-08-01) held 34 of these,
  // all rendered with a green check — 46 get_api_spec rejections, 14 failed
  // mark_pulse_module_result calls, 8 get_pulse_review_result misses.
  it('flags a failed tool call carried in stdout with exit_code 0', () => {
    const result = formatToolCallResult(JSON.stringify({
      stdout: 'ERROR: tool execution failed: layer=custom_tool_handler '
        + 'tool=get_pulse_review_result session=abc: sql: no rows in result set',
      stderr: '',
      exit_code: 0,
      execution_time_ms: 25,
    }))

    expect(result.isError).toBe(true)
  })

  it('flags the virtual-tool form too', () => {
    const result = formatToolCallResult(JSON.stringify({
      stdout: 'ERROR: tool execution failed: layer=virtual_tool_handler '
        + 'tool=get_api_spec session=xyz: server "custom" is not available.',
      exit_code: 0,
    }))

    expect(result.isError).toBe(true)
  })

  it('flags canceled and timed-out envelopes', () => {
    for (const kind of ['canceled', 'timed out']) {
      const result = formatToolCallResult(JSON.stringify({
        stdout: `ERROR: tool execution ${kind}: layer=custom_tool_handler tool=t session=s`,
        exit_code: 0,
      }))
      expect(result.isError, kind).toBe(true)
    }
  })

  // Matching keys off `layer=`, not the word "error", so tool output that
  // merely discusses errors stays clean. A findings table or a log query would
  // otherwise light up red on every row.
  it('does not flag output that merely mentions errors', () => {
    const result = formatToolCallResult(JSON.stringify({
      stdout: 'ERROR count: 3\nrecent errors: tool execution was reviewed for failure modes',
      exit_code: 0,
    }))

    expect(result.isError).toBe(false)
  })

  it('finds the failure nested inside an MCP envelope', () => {
    const result = formatToolCallResult(JSON.stringify({
      content: [{
        type: 'text',
        text: JSON.stringify({
          stdout: 'ERROR: tool execution failed: layer=custom_tool_handler '
            + 'tool=mark_pulse_module_result session=s: review_run_id must start with a UTC date-time',
          exit_code: 0,
        }),
      }],
    }))

    expect(result.isError).toBe(true)
  })
})
