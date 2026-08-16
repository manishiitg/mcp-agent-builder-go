import { describe, expect, it } from 'vitest'
import type { ReportHumanInput } from '../services/api-types'
import type { ChatTab } from '../stores/useChatStore'
import {
  buildReportHumanInputChatMessage,
  buildReportHumanInputDelegatedActionMessage,
  selectReportDiscussionTab,
} from './reportHumanInputChat'

function tab(tabId: string, overrides: Partial<ChatTab> = {}): ChatTab {
  return {
    tabId,
    name: tabId,
    sessionId: `${tabId}-session`,
    isStreaming: false,
    isCompleted: false,
    hasRunningBgAgents: false,
    isSyntheticTurn: false,
    canSteer: false,
    hideToolCalls: true,
    viewMode: 'terminal',
    config: {
      inputText: '',
      useCodeExecutionMode: false,
      selectedServers: [],
      selectedSkills: [],
      selectedSecrets: [],
      llmConfig: { provider: 'codex-cli', model_id: 'gpt-5.6-sol', fallback_models: [] },
      fileContext: [],
      workflowContext: [],
      queuedMessages: [],
    },
    createdAt: 1,
    lastViewedEventCount: 0,
    lastViewedEventCounts: { micro: 0 },
    metadata: { mode: 'workflow', presetQueryId: 'workflow-one' },
    ...overrides,
  }
}

describe('Pulse decision chat routing', () => {
  it('keeps a running schedule separate and chooses the interactive workflow chat', () => {
    const schedule = tab('schedule', {
      isStreaming: true,
      metadata: {
        mode: 'workflow',
        presetQueryId: 'workflow-one',
        isViewOnly: true,
        isScheduledRun: true,
      },
    })
    const chat = tab('chat', {
      metadata: { mode: 'workflow', presetQueryId: 'workflow-one', phaseId: 'workflow-builder' },
    })

    expect(selectReportDiscussionTab(
      { schedule, chat },
      { mode: 'workflow', presetId: 'workflow-one' },
    )?.tabId).toBe('chat')
  })

  it('never routes a Chief-of-Staff Pulse question to a different product\'s tab', () => {
    // Regression test: isInteractiveChiefOfStaffTab used to check only
    // mode/isOrganizationAssistant/isViewOnly/isScheduledRun/isBotRun, never
    // agentProfileId -- so a normal, fully interactive Video Studio project
    // chat (also mode: 'multi-agent') could have been selected as the target
    // for a Chief-of-Staff Pulse question. Fixed by consolidating onto the
    // shared isInteractiveChiefOfStaffTab (utils/chiefOfStaff.ts), which does
    // check profile identity.
    const videoStudioTab = tab('video-studio', {
      metadata: { mode: 'multi-agent', agentProfileId: 'video-studio', agentProfileWorkspace: 'Chats/Video Studio/projects/launch' },
    })
    const chiefOfStaffTab = tab('chief-of-staff', { metadata: { mode: 'multi-agent' } })

    expect(selectReportDiscussionTab(
      { videoStudioTab, chiefOfStaffTab },
      { mode: 'multi-agent' },
    )?.tabId).toBe('chief-of-staff')
  })

  it('matches the new explicit chief-of-staff profile id the same as the legacy no-profile shape', () => {
    const explicit = tab('explicit', {
      metadata: { mode: 'multi-agent', agentProfileId: 'chief-of-staff' },
    })

    expect(selectReportDiscussionTab(
      { explicit },
      { mode: 'multi-agent' },
    )?.tabId).toBe('explicit')
  })

  it('prefers a running interactive chat when more than one retained tab exists', () => {
    const idle = tab('idle', { createdAt: 20 })
    const running = tab('running', { isStreaming: true, createdAt: 10 })

    expect(selectReportDiscussionTab(
      { idle, running },
      { mode: 'workflow', presetId: 'workflow-one' },
      'idle',
    )?.tabId).toBe('running')
  })

  it('includes decision context while making clear that chat must not answer it implicitly', () => {
    const input = {
      id: 'decision-one',
      workspace_path: 'Workflow/example',
      source: 'pulse',
      status: 'pending',
      priority: 'medium',
      question: 'How should I finish the job?',
      context: 'The previous cleanup did not reach the limit.',
      options: [{ id: 'archive', title: 'Archive older entries', description: 'Nothing is lost.' }],
      allow_free_text: true,
      evidence: 'builder/improve.html',
      created_at: '2026-08-04T08:22:00Z',
      updated_at: '2026-08-04T08:22:00Z',
    } as ReportHumanInput

    const message = buildReportHumanInputChatMessage(input, 'Workflow/example', 'Why is archiving safer?')

    expect(message).toContain('Do not submit, dismiss, or mark the decision handled yet')
    expect(message).toContain('call answer_human_input_request')
    expect(message).toContain('Decision ID: decision-one')
    expect(message).toContain('How should I finish the job?')
    expect(message).toContain('Archive older entries [option_id=archive] — Nothing is lost.')
    expect(message).toContain('My question:\nWhy is archiving safer?')
  })

  it('makes an explicit delegated action authorize a choice without allowing an invented unsafe action', () => {
    const input = {
      id: 'decision-one',
      workspace_path: 'Workflow/example',
      source: 'pulse',
      status: 'pending',
      priority: 'medium',
      question: 'How should I finish the job?',
      context: 'The previous cleanup did not reach the limit.',
      options: [{ id: 'archive', title: 'Archive older entries', description: 'Nothing is lost.' }],
      allow_free_text: true,
      evidence: 'builder/improve.html',
      created_at: '2026-08-04T08:22:00Z',
      updated_at: '2026-08-04T08:22:00Z',
    } as ReportHumanInput

    const message = buildReportHumanInputDelegatedActionMessage(input, 'Workflow/example')

    expect(message).toContain('I delegate this pending Pulse decision to you.')
    expect(message).toContain('choose the best supported option')
    expect(message).toContain('do not invent one or take an unsafe action')
    expect(message).toContain('call answer_human_input_request')
    expect(message).toContain('Do not mark the decision consumed yourself.')
    expect(message).toContain('Archive older entries [option_id=archive] — Nothing is lost.')
  })
})
