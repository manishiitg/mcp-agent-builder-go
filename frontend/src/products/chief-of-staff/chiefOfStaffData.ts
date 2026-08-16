import { getApiBaseUrl, getAuthToken } from '../../services/api'

export const CHIEF_OF_STAFF_PROFILE_VERSION = 1

function asString(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

export type ChiefOfStaffCommand = {
  name: string
  description: string
  icon: string
  prompt: string
}

export type ChiefOfStaffUIPanels = {
  secrets: boolean
  schedules: boolean
}

type AgentProfileResponse = {
  commands?: Array<{
    name?: unknown
    description?: unknown
    icon?: unknown
    prompt?: unknown
  }>
  ui_panels?: {
    secrets?: unknown
    schedules?: unknown
  }
}

export type ChiefOfStaffProfileData = {
  commands: ChiefOfStaffCommand[]
  uiPanels: ChiefOfStaffUIPanels
}

// Slash commands and panel toggles both come from the same product.yaml-
// loaded profile response -- one fetch feeds Chief of Staff's command menu
// and which optional panels (Secrets, Schedules) it shows, so the frontend
// never hardcodes what this product offers. Model selection is deliberately
// NOT sourced here -- see ChiefOfStaffHeader's comment for why Chief of
// Staff uses ChatInput's own published-LLM picker instead of a declared
// provider_options list.
export async function loadChiefOfStaffProfileData(): Promise<ChiefOfStaffProfileData> {
  const token = getAuthToken()
  const response = await fetch(`${getApiBaseUrl()}/api/agent-profiles/chief-of-staff?version=${CHIEF_OF_STAFF_PROFILE_VERSION}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  })
  if (!response.ok) throw new Error(`Unable to load Chief of Staff agent profile (${response.status})`)
  const profile = await response.json() as AgentProfileResponse

  const commands = (profile.commands ?? []).flatMap((command) => {
    const name = asString(command.name)
    const prompt = asString(command.prompt)
    if (!name || !prompt) return []
    return [{
      name,
      description: asString(command.description),
      icon: asString(command.icon) || 'terminal',
      prompt,
    }]
  })

  const uiPanels: ChiefOfStaffUIPanels = {
    secrets: profile.ui_panels?.secrets === true,
    schedules: profile.ui_panels?.schedules === true,
  }

  return { commands, uiPanels }
}
