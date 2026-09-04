import { describe, expect, it } from 'vitest'
import { intermediateUpdateFromTranscriptChunk } from './transcriptChunkUpdates'
import type { PollingEvent } from '../../shared/session/types'

function chunk(inner: Record<string, unknown>, id = 'sess_streaming_chunk_1'): PollingEvent {
  return {
    id,
    type: 'streaming_chunk',
    timestamp: '2026-09-04T10:43:09.518Z',
    session_id: 'sess',
    data: { type: 'streaming_chunk', data: { chunk_index: 2, ...inner } },
  } as unknown as PollingEvent
}

describe('intermediateUpdateFromTranscriptChunk', () => {
  it('turns a whole-message transcript chunk into an intermediate reply row', () => {
    const update = intermediateUpdateFromTranscriptChunk(chunk({
      content: 'I’ll separate original posts from replies and count both by day.',
      source: 'transcript',
      is_delta: false,
      is_tool_call: false,
    }))
    expect(update).not.toBeNull()
    expect(update!.type).toBe('llm_generation_end')
    expect(update!.id).toBe('sess_streaming_chunk_1-update')
    const inner = (update!.data as { data: Record<string, unknown> }).data
    expect(inner.content).toBe('I’ll separate original posts from replies and count both by day.')
    expect(inner.restored_intermediate_update).toBe(true)
  })

  it('ignores token deltas, terminal frames, tool markers and empty chunks', () => {
    expect(intermediateUpdateFromTranscriptChunk(chunk({ content: 'I’l', source: 'transcript', is_delta: true }))).toBeNull()
    expect(intermediateUpdateFromTranscriptChunk(chunk({ content: 'screen', source: 'terminal', is_delta: false }))).toBeNull()
    expect(intermediateUpdateFromTranscriptChunk(chunk({ content: 'screen', source: 'transcript', is_delta: false, metadata: { kind: 'terminal', replace: true } }))).toBeNull()
    expect(intermediateUpdateFromTranscriptChunk(chunk({ content: 'exec', source: 'transcript', is_delta: false, is_tool_call: true }))).toBeNull()
    expect(intermediateUpdateFromTranscriptChunk(chunk({ content: '   ', source: 'transcript', is_delta: false }))).toBeNull()
    expect(intermediateUpdateFromTranscriptChunk(chunk({ content: 'hi' }))).toBeNull()
  })
})
