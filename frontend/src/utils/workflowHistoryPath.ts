import type { DiscoveredWorkflow } from '../services/api-types'
import type { CustomPreset, PredefinedPreset } from '../types/preset'

type WorkflowPreset = CustomPreset | PredefinedPreset

/**
 * Resolve chat history from the currently selected workflow identity.
 *
 * During an in-app workflow switch the persisted preset projection can lag one
 * render behind activePresetId. The manifest registry is refreshed first and
 * is therefore authoritative; using the stale preset path briefly requests the
 * previous workflow's history and renders a false "No chats yet" state.
 */
export function resolveWorkflowHistoryPath(
  activePresetId: string | null,
  workflows: DiscoveredWorkflow[],
  activePreset: WorkflowPreset | null,
): string | null {
  if (!activePresetId) return null

  const manifestPath = workflows.find(
    workflow => workflow.manifest.id === activePresetId,
  )?.workspace_path

  return manifestPath || activePreset?.selectedFolder?.filepath || null
}
