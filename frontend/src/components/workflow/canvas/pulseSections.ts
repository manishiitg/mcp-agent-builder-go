export type PulseCommandDefinition = {
  id: string
  label: string
  description: string
}

export const PULSE_MODULE_COMMANDS: PulseCommandDefinition[] = [
  { id: 'bug_review', label: 'Bug review', description: 'Read-only reliability checks; Pulse Fixer applies safe fixes' },
  { id: 'artifact_review', label: 'Artifact review', description: 'Plan-change artifact drift, across all six stores' },
  { id: 'report_health', label: 'Report health', description: 'Dashboard/report accuracy' },
  { id: 'eval_health', label: 'Eval health', description: 'Rubric and eval wiring quality' },
  { id: 'stores_health', label: 'Stores health', description: 'Learnings, knowledge base, and database freshness/quality' },
  { id: 'llm_ops_review', label: 'Ops review', description: 'Cost, timing, tool/runtime reliability, model routing, setup, and plan-design hygiene' },
  { id: 'strategy_auditor', label: 'Strategy Auditor', description: 'Cross-run diagnosis of whether the plan can achieve the goal' },
  { id: 'goal_advisor', label: 'Goal Advisor', description: 'Strategic review when goal evidence is weak' },
]

export const PULSE_FIXED_COMMANDS: PulseCommandDefinition[] = [
  { id: 'dashboard', label: 'Dashboard + questions', description: 'Updates the Pulse narrative and records decisions that need your input' },
  { id: 'backup', label: 'Backup', description: 'Saves current workflow artifacts when changed' },
  { id: 'publish', label: 'Publish', description: 'Refreshes a verified public report when stale' },
  { id: 'notify', label: 'Notify', description: 'Sends the final run summary' },
]
