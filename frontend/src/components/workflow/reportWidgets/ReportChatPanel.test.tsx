// @vitest-environment happy-dom
import React, { act } from 'react'
import { createRoot } from 'react-dom/client'
import { describe, expect, it, vi } from 'vitest'
vi.mock('../../../utils/reportHumanInputChat', () => ({ sendWorkflowMessageToChat: vi.fn() }))
import { ReportChatPanel } from './ReportChatPanel'
import { ReportChatRequestController } from './reportChatRequest'

describe('report message host UI', () => {
  it('shows the message and chat choice, blocks synthetic submission, and cancels without dispatch', async () => {
    Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true })
    const host = document.createElement('div')
    document.body.append(host)
    const root = createRoot(host)
    const dispatch = vi.fn()
    const controller = new ReportChatRequestController('Workflow/websiteaeo', dispatch)
    try {
      await act(async () => root.render(<ReportChatPanel controller={controller} />))
      let result!: ReturnType<typeof controller.request>
      await act(async () => { result = controller.request('Apply approved finding AEO-42') })
      const dialog = document.querySelector('dialog')!
      expect(dialog.textContent).toContain('Workflow/websiteaeo')
      expect(dialog.querySelector('textarea')?.value).toBe('Apply approved finding AEO-42')
      expect(dialog.textContent).toContain('Start a new chat')
      expect(dialog.textContent).toContain('queues behind it')
      await act(async () => dialog.querySelector<HTMLButtonElement>('[type="submit"]')!.click())
      expect(dispatch).not.toHaveBeenCalled()
      await act(async () => dialog.querySelector<HTMLButtonElement>('[type="button"]')!.click())
      await expect(result).resolves.toEqual({ status: 'cancelled' })
      expect(document.querySelector('dialog')).toBeNull()
    } finally {
      await act(async () => root.unmount())
      host.remove()
    }
  })
})
