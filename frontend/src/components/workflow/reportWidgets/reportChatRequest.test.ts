import { describe, expect, it, vi } from 'vitest'
import { ReportChatRequestController } from './reportChatRequest'

const queued = { tabId: 'chat-one', reused: true, queuedBehindRunningTurn: true }

describe('report to chat requests', () => {
  it('requires host review and sends only the edited message to the host workspace', async () => {
    const dispatch = vi.fn().mockResolvedValue(queued)
    const controller = new ReportChatRequestController('Workflow/websiteaeo', dispatch)
    const result = controller.request('Apply finding AEO-42', { requestId: 'AEO-42:v1:apply' })
    expect(dispatch).not.toHaveBeenCalled()
    expect(controller.getSnapshot()?.message).toBe('Apply finding AEO-42')
    await controller.send('Apply only approved finding AEO-42 using remediation.', false)
    expect(dispatch).toHaveBeenCalledExactlyOnceWith({
      workspacePath: 'Workflow/websiteaeo',
      message: 'From the report for Workflow/websiteaeo:\n\nApply only approved finding AEO-42 using remediation.',
      newChat: false,
    })
    await expect(result).resolves.toEqual({ status: 'queued', ...queued })
    expect(controller.getSnapshot()).toBeNull()
  })

  it('honors the new-chat choice and deduplicates repeated requests and Send clicks', async () => {
    let finish!: (value: typeof queued) => void
    const dispatch = vi.fn().mockImplementation(() => new Promise(resolve => { finish = resolve }))
    const controller = new ReportChatRequestController('Workflow/one', dispatch)
    const result = controller.request('Apply item 42', { requestId: '42:v1' })
    expect(controller.request('Apply item 42', { requestId: '42:v1' })).toBe(result)
    const sending = controller.send('Apply item 42', true)
    await controller.send('Apply item 42', true)
    controller.cancel() // cannot label an in-flight send as cancelled
    expect(dispatch).toHaveBeenCalledTimes(1)
    expect(dispatch.mock.calls[0][0].newChat).toBe(true)
    finish(queued)
    await sending
    await expect(controller.request('Apply item 42', { requestId: '42:v1' })).resolves.toEqual(await result)
    expect(controller.getSnapshot()).toBeNull()
    await expect(controller.request('Apply something else', { requestId: '42:v1' })).rejects.toThrow('different message')
  })

  it('keeps failed dispatch retryable without another DB write or duplicate queued message', async () => {
    const dispatch = vi.fn().mockRejectedValueOnce(new Error('Automation unavailable')).mockResolvedValue(queued)
    const controller = new ReportChatRequestController('Workflow/one', dispatch)
    const result = controller.request('Apply item 42')
    await controller.send('Apply item 42', false)
    expect(controller.getSnapshot()).toMatchObject({ sending: false, error: 'Automation unavailable' })
    await controller.send('Apply item 42', false)
    await expect(result).resolves.toMatchObject({ status: 'queued' })
    expect(dispatch).toHaveBeenCalledTimes(2)
  })

  it('cancels unsent work when the view closes and rejects stale callbacks', async () => {
    const dispatch = vi.fn()
    const controller = new ReportChatRequestController('Workflow/one', dispatch)
    const result = controller.request('Apply item 42')
    await expect(controller.request('Apply item 99')).rejects.toThrow('Finish or cancel')
    controller.dispose()
    await expect(result).resolves.toEqual({ status: 'cancelled' })
    await expect(controller.request('Apply item 42')).rejects.toThrow('no longer open')
    await controller.send('Apply item 42', false)
    expect(dispatch).not.toHaveBeenCalled()
  })

  it('settles a successful enqueue even if navigation unmounts the view during sending', async () => {
    let finish!: (value: typeof queued) => void
    const controller = new ReportChatRequestController('Workflow/one', () => new Promise(resolve => { finish = resolve }))
    const result = controller.request('Apply item 42')
    const sending = controller.send('Apply item 42', false)
    controller.dispose()
    finish(queued)
    await sending
    await expect(result).resolves.toMatchObject({ status: 'queued' })
  })

  it('validates inputs before opening the host panel', async () => {
    const controller = new ReportChatRequestController('Workflow/one', vi.fn())
    for (const value of ['', '  ', 'x'.repeat(12001), 42, null]) {
      await expect(controller.request(value as string)).rejects.toThrow('message between')
    }
    await expect(controller.request('Apply', { requestId: '' })).rejects.toThrow('requestId')
    expect(controller.getSnapshot()).toBeNull()
  })

  it('reports an enqueue failure after navigation as an error, not a user cancellation', async () => {
    let fail!: (error: Error) => void
    const controller = new ReportChatRequestController('Workflow/one', () => new Promise((_resolve, reject) => { fail = reject }))
    const result = controller.request('Apply item 42')
    const rejected = expect(result).rejects.toThrow('Queue unavailable')
    const sending = controller.send('Apply item 42', false)
    controller.dispose()
    fail(new Error('Queue unavailable'))
    await sending
    await rejected
  })
})
