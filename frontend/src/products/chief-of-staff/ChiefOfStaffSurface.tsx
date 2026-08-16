import { useEffect, useState } from 'react'
import { CalendarClock, KeyRound, ListChecks, Users } from 'lucide-react'
import ChatArea from '../../components/ChatArea'
import { FileContentViewer } from '../../components/FileContentViewer'
import { ProductSurfaceSwitcher } from '../../components/ProductSurfaceSwitcher'
import { ChiefTasksPanel } from '../../components/org/OrgHtmlPanels'
import MultiAgentSchedulesPopup from '../../components/scheduler/MultiAgentSchedulesPopup'
import SecretsManagerModal from '../../components/secrets/SecretsManagerModal'
import SecretSelectionDropdown from '../../components/secrets/SecretSelectionDropdown'
import { useAppStore } from '../../stores/useAppStore'
import { useChatStore, waitForChatStoreHydration } from '../../stores/useChatStore'
import { useModeStore } from '../../stores/useModeStore'
import { restoreSession } from '../../utils/sessionRestore'
import { CHIEF_OF_STAFF_PROFILE_ID, isInteractiveChiefOfStaffTab } from '../../utils/chiefOfStaff'
import { setProductCommands } from '../../commands/registry'
import { toChiefOfStaffCommandDefinitions } from './productCommands'
import { loadChiefOfStaffProfileData, type ChiefOfStaffUIPanels } from './chiefOfStaffData'

const EMPTY_UI_PANELS: ChiefOfStaffUIPanels = { secrets: false, schedules: false }
const EMPTY_SECRET_IDS: string[] = []

type ChiefOfStaffPanel = 'tasks' | 'schedules'

// Model selection deliberately has no header control here, unlike Video
// Studio's provider dropdown: Chief of Staff uses the same full published-LLM
// picker every multi-agent chat already has in ChatInput (see
// resolveAgentProfileForQuery's isGlobalScope branch in
// agent_profile_runtime.go for why the profile's runtime.provider/model_id
// is only a starting default here, not an authoritative pin).
function ChiefOfStaffHeader({ tabId, uiPanels }: { tabId: string | null; uiPanels: ChiefOfStaffUIPanels }) {
  const [showSecretsManager, setShowSecretsManager] = useState(false)
  const selectedSecrets = useChatStore((state) => tabId ? state.chatTabs[tabId]?.config.selectedSecrets ?? EMPTY_SECRET_IDS : EMPTY_SECRET_IDS)

  const updateSelectedSecrets = (next: string[]) => {
    if (tabId) useChatStore.getState().setTabConfig(tabId, { selectedSecrets: next })
  }

  return (
    <header className="flex h-[62px] shrink-0 items-center gap-4 border-b border-slate-200 bg-white px-4 dark:border-slate-800 dark:bg-slate-950">
      <ProductSurfaceSwitcher />
      <div className="ml-auto flex items-center gap-2">
        {uiPanels.secrets && tabId ? (
          <SecretSelectionDropdown
            selectedSecrets={selectedSecrets}
            onSecretToggle={(secretId) => updateSelectedSecrets(selectedSecrets.includes(secretId) ? selectedSecrets.filter((id) => id !== secretId) : [...selectedSecrets, secretId])}
            onSelectAll={updateSelectedSecrets}
            onClearAll={() => updateSelectedSecrets([])}
            placement="below"
            align="right"
          />
        ) : null}
        {uiPanels.secrets ? (
          <button
            type="button"
            onClick={() => setShowSecretsManager(true)}
            className="grid h-8 w-8 place-items-center rounded-lg border border-slate-200 bg-slate-50 text-slate-500 shadow-sm transition hover:border-indigo-300 hover:bg-indigo-50 hover:text-indigo-700 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:border-indigo-700 dark:hover:bg-indigo-950/40 dark:hover:text-indigo-300"
            aria-label="Manage secrets"
            title="Manage secrets"
          >
            <KeyRound className="h-3.5 w-3.5" />
          </button>
        ) : null}
      </div>
      {showSecretsManager ? <SecretsManagerModal onClose={() => setShowSecretsManager(false)} /> : null}
    </header>
  )
}

function ChiefOfStaffWelcome() {
  return (
    <div className="flex h-full items-center justify-center px-6 py-10">
      <div className="max-w-md text-center">
        <span className="mx-auto grid h-14 w-14 place-items-center rounded-2xl bg-indigo-100 text-indigo-700 dark:bg-indigo-950 dark:text-indigo-300">
          <Users className="h-6 w-6" />
        </span>
        <h2 className="mt-5 text-lg font-semibold text-slate-900 dark:text-slate-100">What do you need?</h2>
        <p className="mt-2 text-sm leading-6 text-slate-500 dark:text-slate-400">
          Ask about any workflow's status, delegate a task, or check what needs your attention. Chief of Staff has
          read-only visibility across every automation.
        </p>
      </div>
    </div>
  )
}

/**
 * Chief of Staff's product surface -- chat-first, ChatArea-direct like Video
 * Studio, not the ChatTabs/ModePresetBar chrome AgentWorks uses. There is no
 * project-list home screen here: unlike Video Studio, there's exactly one
 * singleton Chief-of-Staff conversation to open, not a list to choose from.
 * A real automations-oversight dashboard as a landing view in front of this
 * chat is planned as a separate follow-up, not part of this pass.
 *
 * Chat sits left, an aside on the right holds Tasks (always on) and
 * Schedules (shown when the profile's ui_panels.schedules is set) --
 * mirroring Video Studio's own chat-left/panel-right layout.
 */
export function ChiefOfStaffSurface() {
  const [tabId, setTabId] = useState<string | null>(null)
  const [panel, setPanel] = useState<ChiefOfStaffPanel>('tasks')
  const [uiPanels, setUiPanels] = useState<ChiefOfStaffUIPanels>(EMPTY_UI_PANELS)

  useEffect(() => {
    let cancelled = false
    void loadChiefOfStaffProfileData().then(({ commands, uiPanels: panels }) => {
      if (cancelled) return
      setProductCommands(toChiefOfStaffCommandDefinitions(commands))
      setUiPanels(panels)
    }).catch(() => {})
    return () => { cancelled = true; setProductCommands([]) }
  }, [])

  useEffect(() => {
    let cancelled = false
    const prepare = async () => {
      useModeStore.getState().setModeCategory('multi-agent')
      useAppStore.getState().setAgentMode('multi-agent')
      await waitForChatStoreHydration()
      if (cancelled) return
      const chatStore = useChatStore.getState()

      let tab = Object.values(chatStore.chatTabs).find(isInteractiveChiefOfStaffTab)
      if (tab && tab.metadata?.agentProfileId !== CHIEF_OF_STAFF_PROFILE_ID) {
        // Adopt a legacy no-profile tab in place, without touching its
        // session/history -- lazy and idempotent. isChiefOfStaffTab already
        // treats both shapes as equivalent everywhere else, so a user who
        // never opens this surface again is unaffected either way.
        chatStore.setTabMetadata(tab.tabId, {
          agentProfileId: CHIEF_OF_STAFF_PROFILE_ID,
          agentProfileVersion: 1,
          agentProfileWorkspace: 'Chats',
          agentProfileProjectTitle: 'Chief of Staff',
        })
        tab = chatStore.getTab(tab.tabId)
      }
      if (!tab) {
        const createdTabId = await chatStore.createChatTab('Chief of Staff', {
          mode: 'multi-agent',
          agentProfileId: CHIEF_OF_STAFF_PROFILE_ID,
          agentProfileVersion: 1,
          agentProfileWorkspace: 'Chats',
          agentProfileProjectTitle: 'Chief of Staff',
        })
        tab = chatStore.getTab(createdTabId)
      }
      if (cancelled || !tab) return

      const restoredTabId = await restoreSession(tab.sessionId ?? tab.tabId, {
        title: 'Chief of Staff',
        source: 'chief-of-staff-open',
        skipConfigRestore: true,
      })
      if (cancelled) return
      chatStore.switchTab(restoredTabId)
      setTabId(restoredTabId)
    }
    void prepare()
    return () => { cancelled = true }
  }, [])

  const panelTabs: Array<{ key: ChiefOfStaffPanel; label: string; icon: typeof ListChecks }> = [
    { key: 'tasks', label: 'Tasks', icon: ListChecks },
    ...(uiPanels.schedules ? [{ key: 'schedules' as const, label: 'Schedules', icon: CalendarClock }] : []),
  ]

  return (
    <div className="flex h-screen min-h-0 flex-col overflow-hidden bg-slate-50 dark:bg-slate-950">
      <ChiefOfStaffHeader tabId={tabId} uiPanels={uiPanels} />
      <div className="grid min-h-0 flex-1 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <main className="flex min-h-0 min-w-0 flex-col overflow-hidden bg-white dark:bg-slate-950">
          {tabId ? (
            <ChatArea
              tabId={tabId}
              onNewChat={() => {}}
              landingContent={<ChiefOfStaffWelcome />}
              fullTurnStreaming
              showConversationUsage
            />
          ) : (
            <div className="grid h-full place-items-center text-xs text-slate-400">Connecting…</div>
          )}
        </main>
        <aside className="hidden min-h-0 min-w-0 border-l border-slate-200 bg-slate-50 lg:flex lg:flex-col dark:border-slate-800 dark:bg-slate-900/40">
          <div className="flex h-14 shrink-0 items-center border-b border-slate-200 px-3 dark:border-slate-800">
            {panelTabs.map(({ key, label, icon: Icon }) => (
              <button
                key={key}
                type="button"
                onClick={() => setPanel(key)}
                className={`flex h-full items-center gap-1.5 border-b-2 px-3 text-xs font-semibold ${
                  panel === key
                    ? 'border-indigo-600 text-indigo-700 dark:text-indigo-300'
                    : 'border-transparent text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'
                }`}
              >
                <Icon className="h-3.5 w-3.5" />
                {label}
              </button>
            ))}
          </div>
          <div className="min-h-0 flex-1 overflow-hidden">
            {panel === 'schedules' && uiPanels.schedules ? (
              <MultiAgentSchedulesPopup embedded />
            ) : (
              <ChiefTasksPanel hideHeader />
            )}
          </div>
        </aside>
      </div>
      <FileContentViewer />
    </div>
  )
}
