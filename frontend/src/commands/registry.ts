import type { ModeCategory } from '../stores/useModeStore'
import type { CommandDefinition, WorkshopMode } from './types'
import { builtinCommands } from './builtin-commands'
import { CHIEF_OF_STAFF_PROFILE_ID } from '../utils/chiefOfStaff'

let userCommands: CommandDefinition[] = []
let productCommands: CommandDefinition[] = []

function matchesMode(cmd: CommandDefinition, mode?: ModeCategory, workshopMode?: WorkshopMode, agentProfileId?: string): boolean {
  if (cmd.hidden) return false
  // A command written for Chief of Staff (its own notification config,
  // create_workflow) assumes tools and concepts a product's agent does not
  // have. An agentProfileId set to a DIFFERENT product's id means we are
  // inside that product's tab, not Chief of Staff -- but agentProfileId can
  // now also be the explicit 'chief-of-staff' id itself (not just absent,
  // the legacy shape), which must still count as Chief of Staff, not "a
  // product." See isChiefOfStaffTab (utils/chiefOfStaff.ts) for the same
  // distinction against a full tab rather than a bare id.
  if (cmd.chiefOfStaffOnly && agentProfileId && agentProfileId !== CHIEF_OF_STAFF_PROFILE_ID) return false
  if (mode === undefined || mode === null) return true

  if (mode === 'workflow') {
    if (!(cmd.modes?.includes('workflow') ?? false)) return false
    // Filter by workshop mode if set
    if (workshopMode && cmd.requiredWorkshopMode) {
      const allowed = Array.isArray(cmd.requiredWorkshopMode)
        ? cmd.requiredWorkshopMode
        : [cmd.requiredWorkshopMode]
      return allowed.includes(workshopMode)
    }
    return true
  }

  if (mode === 'multi-agent') {
    return cmd.modes?.includes('multi-agent') ?? cmd.modes === undefined
  }

  return true
}

export function setUserCommands(cmds: CommandDefinition[]) {
  userCommands = cmds
}

// Registered when a product's profile loads. Cleared by passing an empty list
// so switching products cannot leave the previous product's commands in the
// menu, offering flows the current agent has no skills for.
export function setProductCommands(cmds: CommandDefinition[]) {
  productCommands = cmds
}

export function getCommands(mode?: ModeCategory, workshopMode?: WorkshopMode, agentProfileId?: string): CommandDefinition[] {
  return [...productCommands, ...builtinCommands, ...userCommands].filter(cmd => matchesMode(cmd, mode, workshopMode, agentProfileId))
}

export function findCommand(name: string, mode?: ModeCategory, agentProfileId?: string): CommandDefinition | undefined {
  return [...productCommands, ...builtinCommands, ...userCommands].find(cmd =>
    cmd.command === name && matchesMode(cmd, mode, undefined, agentProfileId)
  )
}

export function findCommandAnyMode(name: string): CommandDefinition | undefined {
  return productCommands.find(c => c.command === name)
    ?? builtinCommands.find(c => c.command === name)
    ?? userCommands.find(c => c.command === name)
}
