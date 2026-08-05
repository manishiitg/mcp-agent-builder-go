export type PulseCommandDefinition = {
  id: string
  label: string
  description: string
}

export const PULSE_MODULE_COMMANDS: PulseCommandDefinition[] = [
  { id: 'workflow_review', label: 'Engineering review', description: 'Implementation correctness across execution, reports/evals, plan changes, artifacts, and stores' },
  { id: 'llm_ops_review', label: 'LLM & operations', description: 'Cost, time, model selection, tools, and runtime reliability' },
  { id: 'strategy_auditor', label: 'Strategy Auditor', description: 'Product/business review of whether the current strategy and measurements can achieve the goal' },
  { id: 'goal_advisor', label: 'Goal Advisor', description: 'User-facing proposals for materially different approaches outside the current plan' },
]

export const PULSE_FIXED_COMMANDS: PulseCommandDefinition[] = [
  { id: 'dashboard', label: 'Dashboard + questions', description: 'Updates the Pulse narrative and records decisions that need your input' },
  { id: 'backup', label: 'Backup', description: 'Saves current workflow artifacts when changed' },
  { id: 'publish', label: 'Publish', description: 'Refreshes a verified public report when stale' },
  { id: 'notify', label: 'Notify', description: 'Sends the final run summary' },
]
