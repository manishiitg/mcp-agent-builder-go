import { agentApi } from '../../services/api'

export type WorkspacePresentation = {
  id: string
  kind: string
  schemaVersion: number
  sessionId?: string
  title: string
  payload: Record<string, unknown>
  resources: Array<Record<string, unknown>>
  actions: Array<Record<string, unknown>>
  status: string
  revision: number
  createdAt: string
  updatedAt: string
}

function parseJSONRecord(value: unknown): Record<string, unknown> | null {
  try {
    const parsed = JSON.parse(String(value || '{}')) as unknown
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed as Record<string, unknown> : null
  } catch {
    return null
  }
}

function parseJSONArray(value: unknown): Array<Record<string, unknown>> {
  try {
    const parsed = JSON.parse(String(value || '[]')) as unknown
    return Array.isArray(parsed)
      ? parsed.filter((item): item is Record<string, unknown> => !!item && typeof item === 'object' && !Array.isArray(item))
      : []
  } catch {
    return []
  }
}

export function parseWorkspacePresentations(rows: Record<string, unknown>[]): WorkspacePresentation[] {
  return rows.flatMap((row) => {
    const id = typeof row.id === 'string' ? row.id.trim() : ''
    const kind = typeof row.kind === 'string' ? row.kind.trim() : ''
    const title = typeof row.title === 'string' ? row.title.trim() : ''
    const status = typeof row.status === 'string' ? row.status.trim() : ''
    const payload = parseJSONRecord(row.payload_json)
    if (!id || !kind || !title || !status || !payload) return []
    return [{
      id,
      kind,
      schemaVersion: Number(row.schema_version) || 1,
      sessionId: typeof row.session_id === 'string' ? row.session_id : undefined,
      title,
      payload,
      resources: parseJSONArray(row.resources_json),
      actions: parseJSONArray(row.actions_json),
      status,
      revision: Number(row.revision) || 1,
      createdAt: typeof row.created_at === 'string' ? row.created_at : '',
      updatedAt: typeof row.updated_at === 'string' ? row.updated_at : '',
    }]
  })
}

export async function loadWorkspacePresentations(workspacePath: string, kinds?: string[]): Promise<WorkspacePresentation[]> {
  const clauses = ["status = 'ready'"]
  if (kinds?.length) {
    const safeKinds = kinds.filter((kind) => /^[a-z][a-z0-9._-]{0,79}$/i.test(kind))
    if (safeKinds.length > 0) clauses.push(`kind IN (${safeKinds.map((kind) => `'${kind}'`).join(',')})`)
  }
  try {
    const response = await agentApi.queryWorkflowDB(
      `${workspacePath}/db/db.sqlite`,
      `SELECT id, kind, schema_version, session_id, title, payload_json, resources_json, actions_json, status, revision, created_at, updated_at FROM ui_presentations WHERE ${clauses.join(' AND ')} ORDER BY updated_at DESC`,
    )
    return parseWorkspacePresentations(response.data?.rows || [])
  } catch {
    // A product workspace legitimately has no managed database until its profile
    // initializer runs. The presentation surface is empty, not broken.
    return []
  }
}
