export type PulseCommandDefinition = {
  id: string
  label: string
  description: string
}

export const PULSE_MODULE_COMMANDS: PulseCommandDefinition[] = [
  { id: 'workflow_review', label: 'Workflow review', description: 'One continuous review of correctness, artifacts, reports, stores, cost, and tool operations' },
  { id: 'strategy_auditor', label: 'Strategy Auditor', description: 'Finds missing or ineffective pieces inside the current strategy' },
  { id: 'goal_advisor', label: 'Goal Advisor', description: 'Proposes materially different approaches outside the current plan' },
]

export const PULSE_FIXED_COMMANDS: PulseCommandDefinition[] = [
  { id: 'dashboard', label: 'Dashboard + questions', description: 'Updates the Pulse narrative and records decisions that need your input' },
  { id: 'backup', label: 'Backup', description: 'Saves current workflow artifacts when changed' },
  { id: 'publish', label: 'Publish', description: 'Refreshes a verified public report when stale' },
  { id: 'notify', label: 'Notify', description: 'Sends the final run summary' },
]
