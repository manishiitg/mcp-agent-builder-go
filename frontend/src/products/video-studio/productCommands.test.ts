import { describe, expect, it, vi } from 'vitest'

vi.mock('../../services/api', () => ({
  agentApi: {},
  getApiBaseUrl: () => 'http://localhost:8000',
  getAuthToken: () => null,
}))
import { parseProductCommands } from './videoStudioData'

describe('parseProductCommands', () => {
  it('drops a command with no prompt rather than offering an empty one', () => {
    // A command in the menu that submits nothing reads as the product being
    // broken; not offering it is the honest failure.
    const commands = parseProductCommands({
      commands: [
        { name: 'production', description: 'Start one', icon: 'clapperboard', prompt: 'Do the thing' },
        { name: 'broken', description: 'No prompt', icon: 'terminal' },
        { description: 'No name', prompt: 'orphan' },
      ],
    })

    expect(commands.map((c) => c.name)).toEqual(['production'])
    expect(commands[0]).toMatchObject({ icon: 'clapperboard', prompt: 'Do the thing' })
  })

  it('falls back to a generic icon rather than failing', () => {
    const [command] = parseProductCommands({
      commands: [{ name: 'x', description: 'd', prompt: 'p' }],
    })
    expect(command.icon).toBe('terminal')
  })

  it('returns nothing when the profile declares no commands', () => {
    expect(parseProductCommands({})).toEqual([])
  })
})
