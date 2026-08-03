export interface WorkflowSessionIdentity {
  sessionId?: string | null
  triggeredBy?: string | null
  botPlatform?: string | null
  parentSessionId?: string | null
  sessionKind?: string | null
}

/** Internal child sessions remain observable under their owning execution tree,
 * but must never restore or compete as independent top-level chats. */
export function isInternalChildSession(identity: WorkflowSessionIdentity): boolean {
  return Boolean((identity.parentSessionId || '').trim()) ||
    (identity.sessionKind || '').toLowerCase().trim() === 'pulse_reviewer'
}

/**
 * Scheduled sessions are an independent read-only lane. This applies to both
 * workflow schedules and Chief of Staff schedules.
 */
export function isScheduledSession(identity: WorkflowSessionIdentity): boolean {
  const trigger = (identity.triggeredBy || '').toLowerCase()
  const sessionId = (identity.sessionId || '').toLowerCase()

  return trigger.includes('schedule') ||
    trigger === 'cron' ||
    sessionId.startsWith('schedule-') ||
    sessionId.includes('-schedule-')
}

/**
 * Schedule and bot runs are independent, read-only workflow lanes. They may run
 * alongside the workflow's single interactive builder chat.
 */
export function isExternalReadOnlyWorkflowSession(identity: WorkflowSessionIdentity): boolean {
  const trigger = (identity.triggeredBy || '').toLowerCase()
  const sessionId = (identity.sessionId || '').toLowerCase()
  const botPlatform = (identity.botPlatform || '').toLowerCase()

  return isScheduledSession(identity) ||
    trigger.includes('bot') ||
    trigger.includes('whatsapp') ||
    trigger.includes('slack') ||
    botPlatform !== '' ||
    sessionId.startsWith('bot-')
}
