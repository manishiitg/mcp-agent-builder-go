export type PulseCommandDefinition = {
  id: string
  label: string
  description: string
}

export const PULSE_MODULE_COMMANDS: PulseCommandDefinition[] = [
  { id: 'technical_review', label: 'Technical review', description: 'One focused review of correctness, stores, runtime, orchestration, tools, model choice, cost, or execution efficiency' },
  { id: 'strategic_review', label: 'Strategic review', description: 'Audits hidden strategic mechanisms and conditionally explores materially different approaches' },
]

export const PULSE_FIXED_COMMANDS: PulseCommandDefinition[] = [
  { id: 'dashboard', label: 'Dashboard + questions', description: 'Updates the Pulse narrative and records decisions that need your input' },
  { id: 'backup', label: 'Backup', description: 'Saves current workflow artifacts when changed' },
  { id: 'publish', label: 'Publish', description: 'Refreshes a verified public report when stale' },
  { id: 'notify', label: 'Notify', description: 'Sends the final run summary' },
]
