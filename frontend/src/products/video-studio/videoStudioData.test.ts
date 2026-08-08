import { describe, expect, it, vi } from 'vitest'

vi.mock('../../services/api', () => ({
  agentApi: {},
  getApiBaseUrl: () => 'http://localhost:8000',
  getAuthToken: () => null,
}))
import { parsePresentations, parseProjectManifest, parseWorkflowSteps, projectSlug, relativeTime } from './videoStudioData'

describe('Video Studio workspace data', () => {
  it('accepts only a complete Video Studio product manifest', () => {
    const project = parseProjectManifest(JSON.stringify({
      schema_version: 1,
      product: 'video-studio',
      id: 'project-1',
      title: 'Launch film',
      description: 'A concise product launch.',
      session_id: 'video-studio:project:project-1',
      created_at: '2026-08-07T09:00:00Z',
    }), 'Chats/Video Studio/projects/launch-film', '2026-08-07T10:00:00Z')

    expect(project).toMatchObject({
      id: 'project-1',
      title: 'Launch film',
      workspacePath: 'Chats/Video Studio/projects/launch-film',
      updatedAt: '2026-08-07T10:00:00Z',
      videos: 0,
    })
    expect(parseProjectManifest('{"product":"video-studio"}', 'Chats/bad')).toBeNull()
  })

  it('parses only ready video presentations with valid payloads', () => {
    const presentations = parsePresentations([
      { id: 'v1', kind: 'media.video', title: 'Final', status: 'ready', revision: 2, updated_at: '2026-08-07T10:00:00Z', payload_json: JSON.stringify({ path: 'outputs/final.mp4', qa_report_path: 'work/quality-report.json', verdict: 'pass' }) },
      { id: 'draft', kind: 'media.video', title: 'Draft', status: 'draft', payload_json: JSON.stringify({ path: 'outputs/draft.mp4' }) },
      { id: 'image', kind: 'media.image', title: 'Frame', status: 'ready', payload_json: JSON.stringify({ path: 'outputs/frame.png' }) },
    ])

    expect(presentations).toHaveLength(1)
    expect(presentations[0]).toMatchObject({ id: 'v1', path: 'outputs/final.mp4', verdict: 'pass', revision: 2, workspacePresentation: { kind: 'media.video' } })
  })

  it('parses the fixed workflow route and produces safe project slugs', () => {
    const steps = parseWorkflowSteps(JSON.stringify({ steps: [{
      id: 'route', type: 'routing', title: 'Route', routes: [
        { route_id: 'cinematic', route_name: 'Cinematic', next_step_id: 'story' },
      ],
    }, { id: 'story', type: 'regular', title: 'Story' }] }))

    expect(steps).toHaveLength(2)
    expect(steps[0].routes?.[0]).toEqual({ id: 'cinematic', title: 'Cinematic', nextStepId: 'story' })
    expect(projectSlug('  Café / Product Launch!  ')).toBe('cafe-product-launch')
    expect(relativeTime('2026-08-07T09:55:00Z', Date.parse('2026-08-07T10:00:00Z'))).toBe('5m ago')
  })
})
