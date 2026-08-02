import { describe, expect, it } from 'vitest'
import type { PollingEvent } from '../services/api-types'
import { mergeNewerTerminalEventPage, mergeTerminalEventPages, terminalEventSequenceBounds } from './terminalEventPage'

function event(id: string, sequence: number): PollingEvent {
  return {
    id,
    type: 'tool_call_end',
    timestamp: `2026-08-02T00:00:0${sequence}Z`,
    data: {},
    sequence,
  } as PollingEvent
}

describe('terminal event cursor pages', () => {
  it('merges overlapping pages without duplicate events', () => {
    const merged = mergeTerminalEventPages(
      [event('three', 3), event('four', 4)],
      [event('one', 1), event('two', 2), event('three', 3)],
    )

    expect(merged.map(item => item.id)).toEqual(['one', 'two', 'three', 'four'])
  })

  it('computes cursors from the merged page', () => {
    expect(terminalEventSequenceBounds([event('four', 4), event('nine', 9)])).toEqual({
      oldestSequence: 4,
      latestSequence: 9,
    })
  })

  it('does not resurrect Load earlier after the client reached the beginning', () => {
    const merged = mergeNewerTerminalEventPage(
      {
        events: [event('one', 1), event('two', 2)],
        hasOlder: false,
        oldestSequence: 1,
        latestSequence: 2,
      },
      {
        events: [event('three', 3)],
        // This is true from the server's after_sequence page perspective:
        // events 1 and 2 precede that incremental response. The client already
        // has them, so it must not show Load earlier again.
        has_older: true,
      },
    )

    expect(merged.events.map(item => item.id)).toEqual(['one', 'two', 'three'])
    expect(merged.hasOlder).toBe(false)
    expect(merged.oldestSequence).toBe(1)
    expect(merged.latestSequence).toBe(3)
  })

  it('keeps Load earlier available across newer refreshes until older pages are loaded', () => {
    const merged = mergeNewerTerminalEventPage(
      {
        events: [event('four', 4), event('five', 5)],
        hasOlder: true,
        oldestSequence: 4,
        latestSequence: 5,
      },
      { events: [], has_older: false },
    )

    expect(merged.hasOlder).toBe(true)
  })
})
