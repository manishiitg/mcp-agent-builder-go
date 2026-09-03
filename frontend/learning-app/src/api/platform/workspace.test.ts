import { describe, expect, it } from 'vitest'
import { FamilyWorkspace, computeWeek, documentsURL, familyRelative, workspacePath } from './workspace'

function fakeRequester(files: Record<string, string>, listing: unknown[] = []) {
  const calls: { method: string; path: string; body?: unknown }[] = []
  const request = async <T,>(method: string, path: string, body?: unknown): Promise<T> => {
    calls.push({ method, path, body })
    if (method === 'GET' && path.startsWith('/api/wp/api/documents?')) return { success: true, data: listing } as T
    if (method === 'GET') {
      const rel = decodeURIComponent(path.replace('/api/wp/api/documents/', ''))
      if (!(rel in files)) throw new Error('not found')
      return { success: true, data: { filepath: rel, content: files[rel] } } as T
    }
    if (method === 'PUT') {
      const rel = decodeURIComponent(path.replace('/api/wp/api/documents/', ''))
      files[rel] = (body as { content: string }).content
      return { success: true } as T
    }
    if (method === 'POST' && path === '/api/wp/api/upload') return { filepath: 'Chats/SparkQuill/inbox/photo.jpg' } as T
    throw new Error(`unexpected ${method} ${path}`)
  }
  return { request, calls, files }
}

describe('paths', () => {
  it('maps family-relative paths onto the user workspace and back', () => {
    expect(workspacePath('reports/x.html')).toBe('Chats/SparkQuill/reports/x.html')
    expect(workspacePath('Chats/SparkQuill/a')).toBe('Chats/SparkQuill/a')
    expect(workspacePath('_users/u1/Chats/SparkQuill/a')).toBe('_users/u1/Chats/SparkQuill/a')
    expect(familyRelative('_users/u1/Chats/SparkQuill/activities/a')).toBe('activities/a')
    expect(familyRelative('Chats/SparkQuill')).toBe('')
    expect(documentsURL('a b/c.html', '/raw')).toBe('/api/wp/api/documents/Chats/SparkQuill/a%20b/c.html/raw')
  })
})

describe('FamilyWorkspace', () => {
  it('reads files, converts the tree and sums sizes', async () => {
    const listing = [
      { filepath: 'Chats/SparkQuill/reports', type: 'folder', children: [{ filepath: 'Chats/SparkQuill/reports/progress.html', type: 'file', size: 10 }] },
      { filepath: 'Chats/SparkQuill/family.json', type: 'file', size: 5 },
    ]
    const { request } = fakeRequester({ 'Chats/SparkQuill/reports/progress.html': '<h1>hi</h1>' }, listing)
    const ws = new FamilyWorkspace(request)
    const f = await ws.readFile('reports/progress.html')
    expect(f).toEqual({ path: 'reports/progress.html', is_text: true, content: '<h1>hi</h1>', size: undefined })
    const tree = await ws.tree()
    expect(tree).toEqual({ nodes: [
      { name: 'reports', path: 'reports', type: 'dir', children: [{ name: 'progress.html', path: 'reports/progress.html', type: 'file', size: 10 }] },
      { name: 'family.json', path: 'family.json', type: 'file', size: 5 },
    ], total_size: 15 })
  })

  it('lists activities from their manifests and follows the current-activity pointer', async () => {
    const files = {
      'Chats/SparkQuill/activities/2026-09-03-fractions/activity.json': JSON.stringify({ title: 'Fractions', subject: 'Math', items: ['quick-check.html'], goal: 'g', created_at: '2026-09-03T00:00:00Z' }),
      'Chats/SparkQuill/activities/2026-09-01-old/activity.json': JSON.stringify({ title: 'Old', created_at: '2026-09-01T00:00:00Z' }),
      'Chats/SparkQuill/current-activity.json': JSON.stringify({ dir: 'activities/2026-09-03-fractions' }),
    }
    const listing = [
      { filepath: 'Chats/SparkQuill/activities/2026-09-03-fractions', type: 'folder' },
      { filepath: 'Chats/SparkQuill/activities/2026-09-01-old', type: 'folder' },
      { filepath: 'Chats/SparkQuill/activities/stray.txt', type: 'file' },
    ]
    const ws = new FamilyWorkspace(fakeRequester(files, listing).request)
    const acts = await ws.activities()
    expect(acts.map((a) => a.dir)).toEqual(['activities/2026-09-03-fractions', 'activities/2026-09-01-old'])
    expect(acts[0].items).toEqual([{ path: 'activities/2026-09-03-fractions/quick-check.html', name: 'quick-check.html' }])
    const current = await ws.currentActivity()
    expect(current?.title).toBe('Fractions')
  })

  it('uploads into a folder and keeps scene state as JSON files', async () => {
    const fake = fakeRequester({})
    const ws = new FamilyWorkspace(fake.request)
    const res = await ws.upload(new File(['x'], 'photo.jpg'), 'inbox')
    expect(res).toEqual({ name: 'photo.jpg', path: 'inbox/photo.jpg' })
    const fd = fake.calls.find((c) => c.path === '/api/wp/api/upload')!.body as FormData
    expect(fd.get('folder_path')).toBe('Chats/SparkQuill/inbox')
    await ws.writeJSON(ws.stateFile('scene:room/1'), { key: 'k', data: { score: 3 } })
    expect(Object.keys(fake.files)).toContain('Chats/SparkQuill/state/scene_room_1.json')
    expect((await ws.readJSON<{ data: unknown }>('state/scene_room_1.json'))?.data).toEqual({ score: 3 })
  })

  it('saves the schedule into family.json and the memory mirror', async () => {
    const fake = fakeRequester({ 'Chats/SparkQuill/family.json': JSON.stringify({ child: { name: 'Maya' } }) })
    const ws = new FamilyWorkspace(fake.request)
    await ws.saveSchedule([{ day: 'Monday', start: '08:00', end: '14:00', label: 'School' }])
    expect(JSON.parse(fake.files['Chats/SparkQuill/family.json'])).toEqual({ child: { name: 'Maya' }, schedule: [{ day: 'Monday', start: '08:00', end: '14:00', label: 'School' }] })
    expect(JSON.parse(fake.files['Chats/SparkQuill/memory/child-schedule.json']).entries).toHaveLength(1)
  })
})

describe('computeWeek', () => {
  it('builds a Monday-based week with schedule, log and deadlines placed on their days', () => {
    const now = new Date(2026, 8, 3, 12) // Thursday 3 Sep 2026
    const week = computeWeek(0, now,
      [{ day: 'Monday', start: '08:00', end: '14:00', label: 'School' }],
      [{ date: '2026-09-02', activity_dir: 'activities/a', title: 'A' }],
      [{ title: 'Test', due_date: '2026-09-05', kind: 'test' }, { title: 'Later', due_date: '2026-10-01' }])
    expect(week.week_start).toBe('2026-08-31')
    expect(week.week_end).toBe('2026-09-06')
    expect(week.days[0].schedule).toHaveLength(1)
    expect(week.days[2].activities).toHaveLength(1)
    expect(week.days[5].deadlines?.[0].title).toBe('Test')
    expect(week.upcoming_deadlines).toHaveLength(1)
    expect(computeWeek(1, now, [], [], []).week_start).toBe('2026-09-07')
  })
})
