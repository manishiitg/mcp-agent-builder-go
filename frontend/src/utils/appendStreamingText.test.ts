import { describe, it, expect } from 'vitest'
import { appendStreamingText } from './streamingStatus'

/**
 * Fixtures are the REAL recorded clean streams from the live provider e2e
 * (mcpagent agent/testdata/agent-reviews/TestRealBridgeStreaming_*.json), so
 * these pin the two shapes the product actually receives rather than invented
 * ones.
 */

// claude-code: delta_content_count = 0 — every chunk is a COMPLETE message.
const CLAUDE_BLOCK_CHUNKS = [
  "I'll read the build ID and create the report table.",
  "Now I'll write the report with the build ID.",
  'Now I\'ll verify the report was created correctly.',
  '| Field | Value |\n|-------|-------|\n| build_id | BUILD_ID_07c40d80a7fc |\n| status | ok |',
]

// pi: all 17 chunks marked IsDelta — fragments of ONE message, split mid-word.
const PI_DELTA_CHUNKS = [
  'I will list the tools available on the `workspace_',
  'advanced` server.',
]

const reduce = (chunks: string[], isDelta: boolean | undefined) =>
  chunks.reduce((acc, c) => appendStreamingText(acc, c, isDelta), '')

describe('appendStreamingText', () => {
  describe('block providers (claude-code, codex, cursor)', () => {
    it('separates complete messages so they do not run together', () => {
      const got = reduce(CLAUDE_BLOCK_CHUNKS, false)

      // The exact user-visible defect: sentences glued to each other.
      expect(got).not.toContain('report table.Now')
      expect(got).toContain('report table.\n\nNow')
    })

    it('puts a BLANK line before a markdown table so GFM parses it', () => {
      const got = reduce(CLAUDE_BLOCK_CHUNKS, false)

      // A single newline is not enough: CommonMark/GFM treats the table as lazy
      // continuation of the preceding paragraph and renders literal pipes.
      expect(got).not.toMatch(/[^\n]\n\| Field \| Value \|/)
      expect(got).toContain('\n\n| Field | Value |')
    })

    it('separates complete messages into their own paragraphs', () => {
      // Each block chunk is a whole assistant message. Joined with a single
      // "\n" they soft-wrap into one run-on paragraph on render, losing the
      // segmentation the provider actually sent.
      const got = reduce(['First sentence.', 'Second sentence.'], false)
      expect(got).toBe('First sentence.\n\nSecond sentence.')
    })

    it('puts a markdown table on its own line so it can render as a table', () => {
      const got = reduce(CLAUDE_BLOCK_CHUNKS, false)

      // Previously the header was appended to the end of a sentence
      // ("...correctly.| Field | Value |"), which markdown cannot parse as a table.
      expect(got).not.toMatch(/[^\n]\| Field \| Value \|/)
      expect(got).toContain('\n| Field | Value |')
      // Every table row survives intact on its own line.
      for (const line of got.split('\n').filter(l => l.trimStart().startsWith('|'))) {
        expect(line.trimEnd().endsWith('|')).toBe(true)
      }
    })

    it('does not duplicate a repeated trailing chunk', () => {
      const once = appendStreamingText('alpha', 'beta', false)
      expect(appendStreamingText(once, 'beta', false)).toBe(once)
    })

    it('does not stack extra newlines when a side already has one', () => {
      expect(appendStreamingText('alpha\n', 'beta', false)).toBe('alpha\n\nbeta')
      expect(appendStreamingText('alpha\n\n', 'beta', false)).toBe('alpha\n\nbeta')
      expect(appendStreamingText('alpha', '\n\nbeta', false)).toBe('alpha\n\nbeta')
    })
  })

  describe('delta providers (pi)', () => {
    it('concatenates fragments verbatim, never splitting a word', () => {
      const got = reduce(PI_DELTA_CHUNKS, true)

      expect(got).toBe('I will list the tools available on the `workspace_advanced` server.')
      // The regression this guards: a newline injected mid-word.
      expect(got).not.toContain('workspace_\nadvanced')
    })
  })

  describe('chunks with no marker', () => {
    it('keeps the previous verbatim behaviour rather than guessing', () => {
      expect(appendStreamingText('alpha', 'beta', undefined)).toBe('alphabeta')
    })
  })

  it('handles empty inputs without inventing separators', () => {
    expect(appendStreamingText('', 'alpha', false)).toBe('alpha')
    expect(appendStreamingText('alpha', '', false)).toBe('alpha')
  })
})
