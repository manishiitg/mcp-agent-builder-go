import type { ReportHumanInput } from '../services/api-types'
import type { ChatTab } from '../stores/useChatStore'
import { useChatStore } from '../stores/useChatStore'
import { useGlobalPresetStore } from '../stores/useGlobalPresetStore'
import { useWorkflowStore } from '../stores/useWorkflowStore'
import { activateTab } from './activateTab'

function normalizeWorkspacePath(value?: string | null): string {
  return (value || '').trim().replace(/^\/+|\/+$/g, '').toLowerCase()
}

function tabRecency(tab: ChatTab): number {
  return tab.lastAccessedAt ?? tab.createdAt ?? 0
}

function isInteractiveChiefOfStaffTab(tab: ChatTab): boolean {
  return tab.metadata?.mode === 'multi-agent' &&
    tab.metadata?.isOrganizationAssistant !== true &&
    tab.metadata?.isViewOnly !== true &&
    tab.metadata?.isScheduledRun !== true &&
    tab.metadata?.isBotRun !== true
}

function isInteractiveWorkflowTab(tab: ChatTab, presetId: string): boolean {
  return tab.metadata?.mode === 'workflow' &&
    tab.metadata?.presetQueryId === presetId &&
    tab.metadata?.isViewOnly !== true &&
    tab.metadata?.isScheduledRun !== true &&
    tab.metadata?.isBotRun !== true
}

/**
 * Pick the conversational lane for a Pulse question. Read-only schedule/bot
 * tabs are deliberately excluded so asking a question can never take over the
 * schedule observer session.
 */
export function selectReportDiscussionTab(
  tabs: Record<string, ChatTab>,
  target: { mode: 'workflow'; presetId: string } | { mode: 'multi-agent' },
  activeTabId?: string | null,
): ChatTab | undefined {
  const candidates = Object.values(tabs).filter(tab => target.mode === 'workflow'
    ? isInteractiveWorkflowTab(tab, target.presetId)
    : isInteractiveChiefOfStaffTab(tab))

  return candidates.sort((left, right) => {
    const leftRunning = left.isStreaming || left.hasRunningBgAgents
    const rightRunning = right.isStreaming || right.hasRunningBgAgents
    if (leftRunning !== rightRunning) return leftRunning ? -1 : 1
    if ((left.tabId === activeTabId) !== (right.tabId === activeTabId)) {
      return left.tabId === activeTabId ? -1 : 1
    }
    const leftBuilder = left.metadata?.phaseId === 'workflow-builder'
    const rightBuilder = right.metadata?.phaseId === 'workflow-builder'
    if (leftBuilder !== rightBuilder) return leftBuilder ? -1 : 1
    return tabRecency(right) - tabRecency(left)
  })[0]
}

function sourceName(source: string): string {
  if (source === 'chief_of_staff') return 'Chief of Staff'
  if (source === 'goal_advisor') return 'Goal Advisor'
  return 'Pulse'
}

export function buildReportHumanInputChatMessage(
  input: ReportHumanInput,
  workspacePath: string,
  userQuestion: string,
): string {
  const lines = [
    `I want to discuss a pending ${sourceName(input.source)} decision. Do not submit, dismiss, or mark the decision handled yet; answer my question first.`,
    '',
    `Automation: ${workspacePath}`,
    '',
    'Decision:',
    input.question.trim(),
  ]

  if (input.context?.trim()) {
    lines.push('', 'Context:', input.context.trim())
  }
  if (input.options.length > 0) {
    lines.push('', 'Available options:')
    input.options.forEach((option, index) => {
      lines.push(`${index + 1}. ${option.title}${option.description ? ` — ${option.description}` : ''}`)
    })
  }
  if (input.evidence?.trim()) {
    lines.push('', `Evidence: ${input.evidence.trim()}`)
  }

  lines.push('', 'My question:', userQuestion.trim())
  return lines.join('\n')
}

export type ReportHumanInputChatResult = {
  tabId: string
  reused: boolean
  queuedBehindRunningTurn: boolean
}

async function findWorkflowPreset(workspacePath: string) {
  const find = () => {
    const normalizedTarget = normalizeWorkspacePath(workspacePath)
    return useGlobalPresetStore.getState().workflowPresets.find(preset =>
      normalizeWorkspacePath(preset.selectedFolder?.filepath) === normalizedTarget)
  }

  let preset = find()
  if (!preset) {
    await useGlobalPresetStore.getState().refreshPresets()
    preset = find()
  }
  return preset
}

/**
 * Open the appropriate interactive chat and enqueue a contextual question.
 * Enqueueing uses the same durable per-tab lane as ChatInput: idle/new chats
 * send immediately when ChatArea mounts, while a running chat keeps the message
 * queued for its next turn. Scheduled runs remain independent and untouched.
 */
export async function sendReportHumanInputQuestionToChat({
  input,
  workspacePath,
  userQuestion,
}: {
  input: ReportHumanInput
  workspacePath: string
  userQuestion: string
}): Promise<ReportHumanInputChatResult> {
  const question = userQuestion.trim()
  if (!question) throw new Error('Write a question before opening chat.')

  const chatStore = useChatStore.getState()
  let targetTab: ChatTab | undefined
  let tabId: string
  let reused = false

  if (input.source === 'chief_of_staff') {
    targetTab = selectReportDiscussionTab(chatStore.chatTabs, { mode: 'multi-agent' }, chatStore.activeTabId)
    if (targetTab) {
      tabId = targetTab.tabId
      reused = true
    } else {
      tabId = await chatStore.createChatTab('Chief of Staff', { mode: 'multi-agent' })
      targetTab = useChatStore.getState().getTab(tabId)
    }
  } else {
    const preset = await findWorkflowPreset(workspacePath)
    if (!preset) throw new Error(`Could not find the automation for ${workspacePath}.`)

    const applied = useGlobalPresetStore.getState().applyPreset(preset, 'workflow')
    if (!applied.success) throw new Error(applied.error || 'Failed to open the automation.')

    const latestChatStore = useChatStore.getState()
    targetTab = selectReportDiscussionTab(
      latestChatStore.chatTabs,
      { mode: 'workflow', presetId: preset.id },
      latestChatStore.activeTabId,
    )
    if (targetTab) {
      tabId = targetTab.tabId
      reused = true
    } else {
      tabId = await latestChatStore.createChatTab('Automation Builder', {
        mode: 'workflow',
        phaseId: 'workflow-builder',
        phaseName: 'Automation Builder',
        presetQueryId: preset.id,
      })
      targetTab = useChatStore.getState().getTab(tabId)
    }
  }

  if (!targetTab) throw new Error('Failed to open a chat for this question.')

  // Background agents do not block a new foreground turn; ChatArea only holds
  // the queue while the foreground tab itself is streaming.
  const queuedBehindRunningTurn = targetTab.isStreaming
  const message = buildReportHumanInputChatMessage(input, workspacePath, question)
  const latestChatStore = useChatStore.getState()
  const existingQueue = latestChatStore.getTabConfig(tabId)?.queuedMessages || []
  latestChatStore.setTabConfig(tabId, {
    inputText: '',
    queuedMessages: [...existingQueue, message],
  })
  latestChatStore.setTabViewMode(tabId, 'terminal')
  latestChatStore.setAutoScroll(true)
  activateTab(tabId)

  if (targetTab.metadata?.mode === 'workflow') {
    const workflowStore = useWorkflowStore.getState()
    workflowStore.setShowChatArea(true)
    workflowStore.setShowWorkspacePane(true)
    workflowStore.setFocusedPane('chat')
  }

  window.setTimeout(() => {
    window.dispatchEvent(new CustomEvent('chat-scroll-to-bottom'))
  }, 50)

  return { tabId, reused, queuedBehindRunningTurn }
}
