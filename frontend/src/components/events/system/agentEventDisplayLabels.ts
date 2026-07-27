export function completionTitle(agentName: string | undefined, isMessageSequenceItem: boolean, label: string): string {
  if (isMessageSequenceItem) return 'Completed'
  return `${label} completed: ${agentName || 'Agent'}`
}
