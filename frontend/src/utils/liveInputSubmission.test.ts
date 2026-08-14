import { describe, expect, it, vi } from 'vitest'

import { createLiveInputSubmissionCoordinator, shouldAppendOptimisticLiveInputMessage } from './liveInputSubmission'

describe('createLiveInputSubmissionCoordinator', () => {
  it('executes a rapid duplicate live message exactly once', async () => {
    const submitLiveInput = createLiveInputSubmissionCoordinator()
    let resolve!: (value: boolean) => void
    const submit = vi.fn(() => new Promise<boolean>(done => { resolve = done }))

    const first = submitLiveInput('session-a', 'continue with this', submit)
    const duplicate = submitLiveInput('session-a', 'continue with this', submit)

    expect(duplicate).toBe(first)
    expect(submit).toHaveBeenCalledTimes(1)
    resolve(true)
    await expect(Promise.all([first, duplicate])).resolves.toEqual([true, true])
  })

  it('does not suppress different messages or a later intentional repeat', async () => {
    const submitLiveInput = createLiveInputSubmissionCoordinator()
    const submit = vi.fn(async (value: string) => value)

    await Promise.all([
      submitLiveInput('session-a', 'first', () => submit('first')),
      submitLiveInput('session-a', 'second', () => submit('second')),
    ])
    await submitLiveInput('session-a', 'first', () => submit('first-again'))

    expect(submit).toHaveBeenCalledTimes(3)
  })
})

describe('shouldAppendOptimisticLiveInputMessage', () => {
  it('does not append a second copy when SSE delivered the acknowledged message first', () => {
    const events = [{
      type: 'user_message',
      data: {
        data: {
          content: 'how are you',
          metadata: { message_id: 'steer-123' },
        },
      },
    }]

    expect(shouldAppendOptimisticLiveInputMessage(events, 'steer-123')).toBe(false)
  })

  it('still appends for a different acknowledgement or before the backend echo arrives', () => {
    const events = [{
      type: 'user_message',
      data: { data: { content: 'same text', metadata: { message_id: 'older' } } },
    }]

    expect(shouldAppendOptimisticLiveInputMessage(events, 'newer')).toBe(true)
    expect(shouldAppendOptimisticLiveInputMessage([], 'newer')).toBe(true)
  })
})
