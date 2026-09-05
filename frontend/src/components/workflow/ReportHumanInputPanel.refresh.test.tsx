// @vitest-environment happy-dom
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
import type { PollingEvent, ReportHumanInput } from '../../services/api-types'

vi.mock('../../services/api', () => ({ agentApi: { listReportHumanInputs: vi.fn() } }))
vi.mock('../../stores/useChatStore', () => ({ useChatStore: { getState: () => ({ addToast: vi.fn() }) } }))
vi.mock('./reportWidgets/tableHelpers', () => ({ useContainerSizeTier: () => [null, 'desktop'] }))
vi.mock('../../utils/reportHumanInputChat', () => ({
  delegateReportHumanInputActionToChat: vi.fn(), sendReportHumanInputQuestionToChat: vi.fn(),
}))
vi.mock('../ui/PlainMarkdown', () => ({ PlainMarkdown: ({ content }: { content: string }) => <span>{content}</span> }))

import { agentApi } from '../../services/api'
import { ReportHumanInputPanel } from './ReportHumanInputPanel'
import { WORKFLOW_LOG_REFRESH_EVENT } from './workflowEvents'
import { decisionMutationNeedsRefresh } from '../../utils/decisionRefresh'

describe('decision card refresh during chat', () => {
  it('moves a saved answer out of pending without a chat completion or manual refresh', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    const workspace = 'Workflow/example'
    const input = { id: 'decision-1', workspace_path: workspace, source: 'technical_review',
      status: 'pending', question: 'Approve measurement?', options: [{ id: 'approve', title: 'Approve' }],
      allow_free_text: false, created_at: '2026-09-05T00:00:00Z' } as ReportHumanInput
    vi.mocked(agentApi.listReportHumanInputs).mockResolvedValue({ success: true, inputs: [input] })
    const container = document.createElement('div')
    document.body.append(container)
    const root = createRoot(container)
    try {
      await act(async () => root.render(<ReportHumanInputPanel workspacePath={workspace} />))
      expect(container.textContent).toContain('Needs your decision')
      const answered = { ...input, status: 'answered' as const, selected_option_id: 'approve' }
      vi.mocked(agentApi.listReportHumanInputs).mockResolvedValue({ success: true, inputs: [answered] })
      const receipt = { id: 'tool-1', type: 'tool_call_end', data: { type: 'tool_call_end', data: {
        tool_name: 'answer_human_input_request', result: JSON.stringify({ status: 'answered', input: answered }),
      } } } as PollingEvent
      await act(async () => {
        if (decisionMutationNeedsRefresh(receipt, workspace)) window.dispatchEvent(new CustomEvent(WORKFLOW_LOG_REFRESH_EVENT))
      })
      expect(container.textContent).not.toContain('Needs your decision')
      expect(container.textContent).not.toContain('Save answer')
      expect(container.textContent).toContain('Approve measurement?')
    } finally {
      await act(async () => root.unmount())
      container.remove()
    }
  })
})
