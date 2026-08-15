import { describe, expect, it } from 'vitest'
import type { DiscoveredWorkflow } from '../services/api-types'
import type { CustomPreset } from '../types/preset'
import { resolveWorkflowHistoryPath } from './workflowHistoryPath'

const workflow = (id: string, workspacePath: string): DiscoveredWorkflow => ({
  manifest: { id, label: id } as DiscoveredWorkflow['manifest'],
  workspace_path: workspacePath,
})

const preset = (id: string, workspacePath: string): CustomPreset => ({
  id,
  label: id,
  agentMode: 'workflow',
  createdAt: 0,
  selectedFolder: {
    filepath: workspacePath,
    content: '',
    last_modified: '',
    type: 'folder',
    children: [],
  },
})

describe('resolveWorkflowHistoryPath', () => {
  it('uses the selected manifest when the preset projection still points at the previous workflow', () => {
    expect(resolveWorkflowHistoryPath(
      'sales-outreach-id',
      [
        workflow('testing-id', 'Workflow/testing'),
        workflow('sales-outreach-id', 'Workflow/salesoutreach'),
      ],
      preset('testing-id', 'Workflow/testing'),
    )).toBe('Workflow/salesoutreach')
  })

  it('falls back to the preset path while manifests are still loading', () => {
    expect(resolveWorkflowHistoryPath(
      'sales-outreach-id',
      [],
      preset('sales-outreach-id', 'Workflow/salesoutreach'),
    )).toBe('Workflow/salesoutreach')
  })

  it('does not leak a previous workflow path after selection is cleared', () => {
    expect(resolveWorkflowHistoryPath(
      null,
      [workflow('sales-outreach-id', 'Workflow/salesoutreach')],
      preset('sales-outreach-id', 'Workflow/salesoutreach'),
    )).toBeNull()
  })
})
