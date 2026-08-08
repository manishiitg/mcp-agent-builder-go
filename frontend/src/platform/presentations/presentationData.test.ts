import { describe, expect, it, vi } from 'vitest'

vi.mock('../../services/api', () => ({ agentApi: {} }))

import { parseWorkspacePresentations } from './presentationData'

describe('presentation integration layer', () => {
  it('normalizes generic tool-owned presentation records', () => {
    const result = parseWorkspacePresentations([{ id: 'video-1', kind: 'media.video', schema_version: 1, session_id: 'session-1', title: 'Final', payload_json: '{"path":"outputs/final.mp4"}', resources_json: '[{"kind":"workspace.file"}]', actions_json: '[]', status: 'ready', revision: 2, created_at: 'a', updated_at: 'b' }])
    expect(result).toEqual([expect.objectContaining({ id: 'video-1', kind: 'media.video', revision: 2, payload: { path: 'outputs/final.mp4' }, resources: [{ kind: 'workspace.file' }] })])
  })

  it('drops malformed records instead of handing them to a renderer', () => {
    expect(parseWorkspacePresentations([{ id: 'bad', kind: 'media.video', title: 'Bad', status: 'ready', payload_json: '{' }])).toEqual([])
  })
})
