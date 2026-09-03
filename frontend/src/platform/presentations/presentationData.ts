import { agentApi } from '../../services/api'
import { parseWorkspacePresentations, type WorkspacePresentation } from '../../../shared/session/presentations'
export { parseWorkspacePresentations } from '../../../shared/session/presentations'
export type { WorkspacePresentation } from '../../../shared/session/presentations'

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
  } catch (cause) {
    // A product workspace legitimately has no managed database until its
    // profile initializer runs. That one case is genuinely empty, not broken.
    if (isUninitializedPresentationStore(cause)) return []
    // Everything else -- expired auth, network failure, a malformed response,
    // a schema change -- used to land here too and return [], which renders a
    // broken product as an empty one: no video, no error, nothing to retry.
    // Surface it so the caller can show a real state.
    throw cause
  }
}

/** Recognizes only "the managed database/table does not exist yet", which is the
 * expected state before a product's profile initializer has run. Anything else
 * is a real failure and must not be mistaken for an empty workspace. */
function isUninitializedPresentationStore(cause: unknown): boolean {
  const message = (cause instanceof Error ? cause.message : String(cause ?? '')).toLowerCase()
  if (!message) return false
  return (
    message.includes('no such table') ||
    message.includes('unable to open database') ||
    message.includes('does not exist') ||
    message.includes('no such file')
  )
}
