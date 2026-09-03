import { describe, expect, it } from 'vitest'
import { quickCommandsFromProfile } from './commands'

describe('quickCommandsFromProfile', () => {
  it('maps product commands to menu entries and drops empty ones', () => {
    expect(quickCommandsFromProfile({ commands: [
      { name: 'create-test', description: 'Create a practice test', icon: 'x', prompt: 'Create a practice test for my child.' },
      { name: 'backup', prompt: 'Back up my workspace now.' },
      { name: 'broken', description: 'No prompt' },
    ] })).toEqual([
      { label: 'Create a practice test', message: 'Create a practice test for my child.' },
      { label: 'backup', message: 'Back up my workspace now.' },
    ])
    expect(quickCommandsFromProfile(null)).toEqual([])
  })
})
