import type { ChatTab } from '../../stores/useChatStore'

// Convert an observed scheduled/bot conversation into an editable Builder tab
// without changing the logical conversation identity. The same session ID is
// what lets the backend steer a retained tmux or resume its native CLI handle.
export function convertObservedWorkflowTabToInteractive(tab: ChatTab): ChatTab {
  return {
    ...tab,
    name: 'Automation Builder',
    metadata: {
      ...tab.metadata,
      mode: 'workflow',
      phaseId: 'workflow-builder',
      phaseName: 'Automation Builder',
      presetQueryId: tab.metadata?.presetQueryId,
      isViewOnly: false,
      isScheduledRun: false,
      scheduledJobName: undefined,
      isBotRun: false,
      botPlatform: undefined,
      readOnlyRestoredAt: undefined,
      userInteractiveContinuation: true,
    },
  }
}
