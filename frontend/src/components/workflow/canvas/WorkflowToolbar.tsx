import React, { useEffect, useLayoutEffect, useRef, useMemo, useCallback, useState } from 'react'
import { useShallow } from 'zustand/react/shallow'
import {
  Cloud,
  Globe,
  LoaderCircle,
  Play,
  ShieldCheck,
  Activity,
  BellRing,
  CalendarClock,
  ClipboardCheck,
  RefreshCw,
  X,
} from 'lucide-react'
import ModalPortal from '../../ui/ModalPortal'
import { useWorkflowStore, type RunFolder } from '../../../stores/useWorkflowStore'
import { WORKSPACE_VIEWS, isInspectorView, type WorkspaceViewId } from '../workspaceViews'
import { useWorkflowManifestStore } from '../../../stores/useWorkflowManifestStore'
import { useChatStore } from '../../../stores/useChatStore'
import { useAuthStore } from '../../../stores/useAuthStore'
import type {
  PulseFinalCommandState,
  PulseModuleState,
  PulseRunMode,
  PulseReviewFocus,
  PulseShadowSignalObservation,
  ScheduledJob,
  VariablesManifest,
} from '../../../services/api-types'
import type { PlanningResponse } from '../../../utils/stepConfigMatching'
import type { WorkflowExecutionStatus } from '../hooks/useWorkflowExecution'
import type { ExecutionOptions } from '../../../services/api-types'
import { agentApi } from '../../../services/api'
import { schedulerApi } from '../../../api/scheduler'
import WorkflowBackupPopup from '../WorkflowBackupPopup'
import { getBackupDotClass } from '../backupStatus'
import WorkflowPublishPopup from '../WorkflowPublishPopup'
import { getPublishDotClass } from '../publishStatus'
import WorkflowNotificationPopup from '../WorkflowNotificationPopup'
import { PulseWorkspace } from '../PulseWorkspace'
import { getNotificationDotClass } from '../notificationStatus'
import { loadWorkflowNotificationInfo, type WorkflowNotificationState } from '../../../services/workflow-notifications'
import WorkflowAccessPopup from '../WorkflowAccessPopup'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '../../ui/tooltip'
import { WORKFLOW_SOUL_REFRESH_EVENT } from '../SoulViewer'
import { hasWorkflowWriteAccess, hasWorkflowOwnerAccess } from '../../../utils/workflowPermissions'
import {
  PULSE_FIXED_COMMANDS,
  PULSE_MODULE_COMMANDS,
} from './pulseSections'
import { sendWorkflowMessageToChat } from '../../../utils/reportHumanInputChat'

// Execution phase ID - special phase that should be displayed separately
const EXECUTION_PHASE_ID = 'execution'
const WORKFLOW_SCHEDULE_TOOLBAR_LIMIT = 10_000

// Product-tour / test hooks on specific toolbar buttons. Kept here rather
// than in the view registry because they describe this toolbar's buttons,
// not the views themselves.
const CAPABILITY_BUTTON_ATTRS: Partial<Record<WorkspaceViewId, { 'data-tour': string; 'data-testid': string }>> = {
  bots: { 'data-tour': 'bot-connector', 'data-testid': 'tour-bot-connector' },
}

type WorkflowScheduleStats = {
  total: number
  running: number
  enabled: number
  paused: number
  missed: number
  issues: number
}

const EMPTY_WORKFLOW_SCHEDULE_STATS: WorkflowScheduleStats = {
  total: 0,
  running: 0,
  enabled: 0,
  paused: 0,
  missed: 0,
  issues: 0,
}

function normalizeWorkspacePath(path?: string | null): string {
  return (path || '').replace(/\/+$/, '')
}

interface CompactToolbarMenuProps {
  label: string
  icon: React.ReactNode
  active?: boolean
  children: (close: () => void) => React.ReactNode
}

function CompactToolbarMenu({ label, icon, active = false, children }: CompactToolbarMenuProps) {
  const [open, setOpen] = useState(false)
  const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0, ready: false })
  const containerRef = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  const updateMenuPosition = useCallback(() => {
    const trigger = containerRef.current?.querySelector('button')
    const menu = menuRef.current
    if (!trigger || !menu) return
    const triggerRect = trigger.getBoundingClientRect()
    const menuWidth = menu.offsetWidth || 208
    const menuHeight = menu.offsetHeight || 0
    const gap = 8
    const viewportGap = 8
    const left = Math.max(
      viewportGap,
      Math.min(triggerRect.right - menuWidth, window.innerWidth - menuWidth - viewportGap),
    )
    const spaceBelow = window.innerHeight - triggerRect.bottom - gap - viewportGap
    const spaceAbove = triggerRect.top - gap - viewportGap
    const openAbove = menuHeight > spaceBelow && spaceAbove > spaceBelow
    const desiredTop = openAbove
      ? triggerRect.top - gap - menuHeight
      : triggerRect.bottom + gap
    const top = Math.max(viewportGap, Math.min(desiredTop, window.innerHeight - menuHeight - viewportGap))
    setMenuPosition({ top, left, ready: true })
  }, [])

  useLayoutEffect(() => {
    if (!open) return
    updateMenuPosition()
    window.addEventListener('resize', updateMenuPosition)
    window.addEventListener('scroll', updateMenuPosition, true)
    return () => {
      window.removeEventListener('resize', updateMenuPosition)
      window.removeEventListener('scroll', updateMenuPosition, true)
    }
  }, [open, updateMenuPosition])

  useEffect(() => {
    if (!open) return
    const closeOnOutsideClick = (event: MouseEvent) => {
      if (
        containerRef.current
        && !containerRef.current.contains(event.target as Node)
        && !menuRef.current?.contains(event.target as Node)
      ) {
        setOpen(false)
      }
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', closeOnOutsideClick)
    document.addEventListener('keydown', closeOnEscape)
    return () => {
      document.removeEventListener('mousedown', closeOnOutsideClick)
      document.removeEventListener('keydown', closeOnEscape)
    }
  }, [open])

  return (
    <div ref={containerRef} className="relative">
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            onClick={() => setOpen(current => !current)}
            className={`flex h-7 w-7 items-center justify-center rounded-md transition-colors ${open || active ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'}`}
            aria-label={label}
            aria-haspopup="menu"
            aria-expanded={open}
          >
            {icon}
          </button>
        </TooltipTrigger>
        <TooltipContent side="bottom"><p>{label}</p></TooltipContent>
      </Tooltip>
      {open && (
        <ModalPortal>
          <div
            ref={menuRef}
            role="menu"
            aria-label={label}
            className="fixed z-[10000] max-h-[calc(100vh-1rem)] min-w-52 overflow-y-auto rounded-lg border border-border bg-popover p-1.5 text-popover-foreground shadow-xl"
            style={{
              top: menuPosition.top,
              left: menuPosition.left,
              visibility: menuPosition.ready ? 'visible' : 'hidden',
            }}
          >
            {children(() => setOpen(false))}
          </div>
        </ModalPortal>
      )}
    </div>
  )
}

interface CompactToolbarMenuItemProps {
  icon: React.ReactNode
  label: string
  detail?: string
  active?: boolean
  trailingAction?: {
    label: string
    icon: React.ReactNode
    onClick: () => void
    active?: boolean
  }
  'data-testid'?: string
  onClick: () => void
}

function CompactToolbarMenuItem({ icon, label, detail, active = false, trailingAction, onClick, 'data-testid': dataTestId }: CompactToolbarMenuItemProps) {
  return (
    <div className={`flex items-center rounded-md ${active ? 'bg-accent text-accent-foreground' : 'text-foreground hover:bg-accent hover:text-accent-foreground'}`}>
      <button
        type="button"
        data-testid={dataTestId}
        role="menuitem"
        onClick={onClick}
        className="flex min-w-0 flex-1 items-center gap-2.5 px-2.5 py-2 text-left text-xs"
      >
        <span className="flex h-5 w-5 shrink-0 items-center justify-center text-muted-foreground">{icon}</span>
        <span className="min-w-0 flex-1">
          <span className="block font-medium">{label}</span>
          {detail && <span className="mt-0.5 block truncate text-[10px] text-muted-foreground">{detail}</span>}
        </span>
      </button>
      {trailingAction && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              onClick={trailingAction.onClick}
              aria-label={trailingAction.label}
              aria-pressed={trailingAction.active}
              className={`mr-1.5 flex h-6 w-6 shrink-0 items-center justify-center rounded transition-colors ${trailingAction.active ? 'text-primary hover:bg-primary/10' : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'}`}
            >
              {trailingAction.icon}
            </button>
          </TooltipTrigger>
          <TooltipContent side="left"><p>{trailingAction.label}</p></TooltipContent>
        </Tooltip>
      )}
    </div>
  )
}

function formatPulseTimestamp(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleString(undefined, {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
}

interface WorkflowToolbarProps {
  status: WorkflowExecutionStatus
  hasPlan: boolean
  plan?: PlanningResponse | null  // Plan data used by toolbar actions
  currentPhase?: string
  workspacePath?: string | null
  presetQueryId?: string | null  // Used to persist settings per workflow
  // API data passed as props (avoids store subscription issues)
  runFolders: RunFolder[]
  variablesManifest: VariablesManifest | null
  isLoadingWorkspaceState?: boolean  // Whether workspace state (iterations, manifest) is loading
  onStartPhase: (phaseId: string, executionOptions?: ExecutionOptions) => void
  onCreatePlan: () => void
  showChatArea?: boolean
  onToggleChatArea?: () => void
  onExport?: () => void
  // Chat tab strip (WorkflowChatTabs) rendered inline on the left of this bar so the
  // workflow chat tabs + new-chat share one row with the status/tools instead of
  // sitting in a separate bar below.
  chatTabsSlot?: React.ReactNode
  // Used by cross-workflow decision links. This is intentionally one-shot:
  // opening a decision must surface Pulse, but normal re-renders must not keep
  // reopening a modal the user deliberately closed.
  openPulseOnMount?: boolean
  className?: string
}

export const WorkflowToolbar: React.FC<WorkflowToolbarProps> = ({
  status,
  hasPlan,
  workspacePath,
  presetQueryId,
  variablesManifest,
  isLoadingWorkspaceState = false,
  chatTabsSlot,
  openPulseOnMount = false,
  className = ''
}) => {
  const canWriteWorkflow = useAuthStore(state => hasWorkflowWriteAccess(state.user, state.isMultiUserMode))
  const canManageAccess = useAuthStore(state => state.isMultiUserMode && hasWorkflowOwnerAccess(state.user, state.isMultiUserMode))

  // Workspace store for opening folders

  // Workflow store - use useShallow to prevent unnecessary re-renders
  // Note: runFolders, variablesManifest come from props (passed from WorkflowCanvas)
  const {
    selectedRunFolder,
    selectedGroupIds,
    currentRunningGroupId,
    loadSavedSettings,
    setSelectedGroupIds,
    restoreSelectionFromLocalStorage,
  } = useWorkflowStore(useShallow(state => ({
    selectedRunFolder: state.selectedRunFolder,
    selectedGroupIds: state.selectedGroupIds,
    currentRunningGroupId: state.currentRunningGroupId,
    loadSavedSettings: state.loadSavedSettings,
    setSelectedGroupIds: state.setSelectedGroupIds,
    restoreSelectionFromLocalStorage: state.restoreSelectionFromLocalStorage,
  })))

  const workflowWorkspaceView = useWorkflowStore(state => state.workflowWorkspaceView)
  const canvasViewMode = useWorkflowStore(state => state.canvasViewMode)
  const showWorkspacePane = useWorkflowStore(state => state.showWorkspacePane)
  const openWorkspaceView = useWorkflowStore(state => state.openWorkspaceView)

  const activeWorkspaceView: WorkspaceViewId = workflowWorkspaceView === 'files' || isInspectorView(workflowWorkspaceView)
    ? workflowWorkspaceView
    : canvasViewMode === 'flow' ? 'flow' : 'report'

  // Button clusters come from the view registry, in registry order. The
  // Plan button is the one view that hides itself until a plan exists.
  const workspaceViewDefinitions = useMemo(
    () => WORKSPACE_VIEWS.filter(view => view.toolbarGroup === 'views' && (view.id !== 'flow' || hasPlan)),
    [hasPlan],
  )
  const capabilityViewDefinitions = useMemo(
    () => WORKSPACE_VIEWS.filter(view => view.toolbarGroup === 'capabilities'),
    [],
  )

  const pulseConfig = useWorkflowManifestStore(useShallow((s) => {
    const wf = s.workflows.find((w) => w.workspace_path === workspacePath)
    return {
      enabled: wf?.manifest.pulse?.enabled,
      legacyEnabled: wf?.manifest.schedules?.some((schedule) => schedule.pulse_review_only && schedule.enabled),
    }
  }))
  const monitorOn = !!(pulseConfig.enabled || pulseConfig.legacyEnabled)
  const updateWorkflowManifest = useWorkflowManifestStore((s) => s.updateWorkflow)
  const [monitorSaving, setMonitorSaving] = useState(false)
  const toggleMonitor = useCallback(async () => {
    if (!workspacePath || monitorSaving) return
    setMonitorSaving(true)
    try {
      await updateWorkflowManifest(workspacePath, { pulse_enabled: !monitorOn })
    } catch (err) {
      console.error('[WorkflowToolbar] Failed to toggle Pulse review schedule:', err)
    } finally {
      setMonitorSaving(false)
    }
  }, [workspacePath, monitorOn, monitorSaving, updateWorkflowManifest])
  const [showMonitorHelp, setShowMonitorHelp] = useState(false)
  const [pulseModuleStates, setPulseModuleStates] = useState<PulseModuleState[]>([])
  const [pulseFinalCommandStates, setPulseFinalCommandStates] = useState<PulseFinalCommandState[]>([])
  const [pulseGateMode, setPulseGateMode] = useState<PulseRunMode | null>(null)
  const [pulseReviewFocuses, setPulseReviewFocuses] = useState<PulseReviewFocus[]>([])
  const [pulseReviewFocusSelections, setPulseReviewFocusSelections] = useState<PulseReviewFocus[]>([])
  const [pulseLoopClosureObservation, setPulseLoopClosureObservation] = useState<PulseShadowSignalObservation | null>(null)
  const [pulseStatusLoading, setPulseStatusLoading] = useState(false)
  const [pulseStatusError, setPulseStatusError] = useState<string | null>(null)
  // Backup popup state
  const [showBackupPopup, setShowBackupPopup] = useState(false)
  const [backupState, setBackupState] = useState<string>('loading')
  const [showPublishPopup, setShowPublishPopup] = useState(false)
  const [publishState, setPublishState] = useState<string>('not_configured')
  const [showNotifications, setShowNotifications] = useState(false)
  const [notificationState, setNotificationState] = useState<WorkflowNotificationState | 'loading'>('loading')
  const [showAccessPopup, setShowAccessPopup] = useState(false)
  const [workflowScheduleStats, setWorkflowScheduleStats] = useState<WorkflowScheduleStats>(EMPTY_WORKFLOW_SCHEDULE_STATS)
  const [manualPulseStarting, setManualPulseStarting] = useState(false)

  const runPulseNow = useCallback(async () => {
    if (!workspacePath || manualPulseStarting) return
    const confirmed = window.confirm(
      'Run Pulse now? This performs the workflow version preflight, reviews the latest retained run, applies eligible fixes, and runs configured backup, publish, and notification actions. It does not execute the workflow.'
    )
    if (!confirmed) return

    setManualPulseStarting(true)
    try {
      await schedulerApi.runPulse(workspacePath)
      useChatStore.getState().addToast('Pulse started', 'success')
    } catch (error) {
      const responseData = (error as { response?: { data?: unknown } })?.response?.data
      const detail = typeof responseData === 'string'
        ? responseData
        : error instanceof Error
          ? error.message
          : 'Unable to start Pulse'
      useChatStore.getState().addToast(detail.trim() || 'Unable to start Pulse', 'error')
    } finally {
      setManualPulseStarting(false)
    }
  }, [manualPulseStarting, workspacePath])

  const refreshPulseModuleStates = useCallback(async (showLoading = true) => {
    if (!workspacePath) {
      setPulseModuleStates([])
      setPulseFinalCommandStates([])
      setPulseGateMode(null)
      setPulseReviewFocuses([])
      setPulseReviewFocusSelections([])
      setPulseLoopClosureObservation(null)
      setPulseStatusError(null)
      return
    }
    if (showLoading) setPulseStatusLoading(true)
    setPulseStatusError(null)
    try {
      const resp = await agentApi.getPulseModuleState(workspacePath)
      if (!resp.success) {
        throw new Error(resp.error || 'Failed to load Pulse status')
      }
      setPulseModuleStates(resp.modules || [])
      setPulseFinalCommandStates(resp.commands || [])
      setPulseGateMode(resp.gate_mode || null)
      setPulseReviewFocuses(resp.review_focus_history || [])
      setPulseReviewFocusSelections(resp.review_focus_selections || [])
      setPulseLoopClosureObservation(
        (resp.shadow_signal_observations || []).find(observation => observation.detector === 'loop_closure') || null
      )
    } catch (err) {
      const message = err instanceof Error ? err.message : 'Failed to load Pulse status'
      setPulseStatusError(message)
    } finally {
      if (showLoading) setPulseStatusLoading(false)
    }
  }, [workspacePath])

  const openedInitialPulseRef = useRef(false)
  useEffect(() => {
    if (!openPulseOnMount || openedInitialPulseRef.current) return
    openedInitialPulseRef.current = true
    setShowMonitorHelp(true)
    void refreshPulseModuleStates()
  }, [openPulseOnMount, refreshPulseModuleStates])

  useEffect(() => {
    if (!showMonitorHelp) return
    void refreshPulseModuleStates()
    const timer = window.setInterval(() => { void refreshPulseModuleStates(false) }, 5_000)
    return () => window.clearInterval(timer)
  }, [showMonitorHelp, refreshPulseModuleStates])

  const pulseModuleStateByModule = useMemo(() => {
    return new Map(pulseModuleStates.map(state => [state.module, state]))
  }, [pulseModuleStates])

  const pulseFinalCommandStateByCommand = useMemo(() => {
    return new Map(pulseFinalCommandStates.map(state => [state.command, state]))
  }, [pulseFinalCommandStates])

  const pulseOverview = useMemo(() => {
    const timestamps = [
      ...pulseModuleStates.map(state => state.updated_at || state.last_ran_at || state.last_checked_at),
      ...pulseFinalCommandStates.map(state => state.updated_at || state.finished_at || state.started_at),
      pulseLoopClosureObservation?.observed_at,
    ].filter((value): value is string => !!value)
    const latestTimestamp = timestamps.reduce((latest, value) => {
      const time = new Date(value).getTime()
      return Number.isNaN(time) || time <= latest ? latest : time
    }, 0)
    const recordedModuleStates = PULSE_MODULE_COMMANDS
      .map(command => pulseModuleStateByModule.get(command.id))
      .filter((state): state is PulseModuleState => !!state)
    const recordedFinalStates = PULSE_FIXED_COMMANDS
      .map(command => pulseFinalCommandStateByCommand.get(command.id))
      .filter((state): state is PulseFinalCommandState => !!state)
    return {
      recorded: recordedModuleStates.length + recordedFinalStates.length,
      total: PULSE_MODULE_COMMANDS.length + PULSE_FIXED_COMMANDS.length,
      latest: latestTimestamp > 0 ? formatPulseTimestamp(new Date(latestTimestamp).toISOString()) : '',
    }
  }, [pulseFinalCommandStateByCommand, pulseFinalCommandStates, pulseLoopClosureObservation, pulseModuleStateByModule, pulseModuleStates])

  const updateWorkflowScheduleStats = useCallback((jobs: ScheduledJob[]) => {
    const normalizedWorkspacePath = normalizeWorkspacePath(workspacePath)
    const matchingJobs = jobs.filter((job) => {
      if (presetQueryId && job.preset_query_id === presetQueryId) return true
      if (!normalizedWorkspacePath) return false
      return normalizeWorkspacePath(job.workspace_path) === normalizedWorkspacePath
    })
    setWorkflowScheduleStats({
      total: matchingJobs.length,
      running: matchingJobs.filter(job => job.last_status === 'running').length,
      enabled: matchingJobs.filter(job => job.enabled).length,
      paused: matchingJobs.filter(job => !job.enabled).length,
      missed: matchingJobs.filter(job => job.enabled && (job.missed_run_count ?? 0) > 0).length,
      issues: matchingJobs.filter(job => job.last_status === 'error').length,
    })
  }, [workspacePath, presetQueryId])

  const refreshWorkflowScheduleStats = useCallback(async () => {
    if (!workspacePath && !presetQueryId) {
      setWorkflowScheduleStats(EMPTY_WORKFLOW_SCHEDULE_STATS)
      return
    }

    try {
      const resp = await schedulerApi.listJobs({
        entity_type: 'workflow',
        limit: WORKFLOW_SCHEDULE_TOOLBAR_LIMIT,
      })
      updateWorkflowScheduleStats(resp.jobs || [])
    } catch {
      setWorkflowScheduleStats(EMPTY_WORKFLOW_SCHEDULE_STATS)
    }
  }, [workspacePath, presetQueryId, updateWorkflowScheduleStats])

  useEffect(() => {
    void refreshWorkflowScheduleStats()
  }, [refreshWorkflowScheduleStats])

  // Lightweight backup-status poll so the toolbar dot reflects health at a glance.
  const refreshBackupState = useCallback(async () => {
    if (!workspacePath) {
      setBackupState('not_configured')
      return
    }
    try {
      const resp = await agentApi.getWorkflowBackup(workspacePath)
      setBackupState(resp.effective_state || 'not_configured')
    } catch {
      // Leave the last known state; a transient fetch failure shouldn't flip the dot.
    }
  }, [workspacePath])

  useEffect(() => {
    setBackupState('loading')
    void refreshBackupState()
  }, [refreshBackupState])

  const refreshPublishState = useCallback(async () => {
    if (!workspacePath) {
      setPublishState('not_configured')
      return
    }
    try {
      const resp = await agentApi.getWorkflowPublish(workspacePath)
      setPublishState(resp.effective_state || 'not_configured')
    } catch {
      // Leave the last known state.
    }
  }, [workspacePath])

  useEffect(() => {
    refreshPublishState()
  }, [refreshPublishState])

  const refreshNotificationState = useCallback(async () => {
    if (!workspacePath) {
      setNotificationState('not_configured')
      return
    }
    try {
      const info = await loadWorkflowNotificationInfo(workspacePath)
      setNotificationState(info.effectiveState)
    } catch {
      setNotificationState('not_configured')
    }
  }, [workspacePath])

  useEffect(() => {
    void refreshNotificationState()
  }, [refreshNotificationState])

  const closeAllPopups = useCallback(() => {
    setShowBackupPopup(false)
    setShowPublishPopup(false)
    setShowNotifications(false)
    setShowMonitorHelp(false)
  }, [])
  
  // Close popups only when switching between two concrete workflows.
  // Preset refreshes can briefly unset workspacePath; treating that as a switch
  // closes every toolbar popup even though the user is still on the same workflow.
  const prevWorkspacePathRef = useRef<string | null>(workspacePath ?? null)
  useEffect(() => {
    if (!workspacePath) {
      return
    }
    if (prevWorkspacePathRef.current && prevWorkspacePathRef.current !== workspacePath) {
      closeAllPopups()
    }
    prevWorkspacePathRef.current = workspacePath
  }, [workspacePath, closeAllPopups])
  
  // Main workflow execution phase for the canvas toolbar
  const targetExecutionPhaseId = EXECUTION_PHASE_ID
  
  // Check if execution phase specifically is running (not just any phase)
  // Use a selector that only recalculates when chatTabs, pollingInterval, or sseConnections change
  const isExecutionRunning = useChatStore(state => {
    const chatTabs = state.chatTabs
    const pollingInterval = state.pollingInterval
    const sseConnections = state.sseConnections
    const allTabs = Object.values(chatTabs)

    try {
      // Filter for execution phase tabs belonging to the current preset
      const executionTabs = allTabs.filter(tab =>
        tab.metadata?.mode === 'workflow' &&
        tab.metadata?.phaseId === targetExecutionPhaseId &&
        tab.metadata?.presetQueryId === presetQueryId
      )

      // Check if any execution tab is streaming
      return executionTabs.some(tab => {
        // If tab is completed, it's not streaming
        if (tab.isCompleted) return false

        // Tab is streaming if there's an active connection (SSE or polling) and tab is not manually paused
        const hasActiveConnection = pollingInterval !== null
          || (tab.sessionId != null && sseConnections[tab.sessionId] != null)
        if (hasActiveConnection) {
          return tab.isStreaming !== false // Respect manual pause
        }

        // Also show Stop if tab.isStreaming is explicitly true (set immediately on query submit,
        // before SSE/polling connects)
        return tab.isStreaming === true
      })
    } catch (error) {
      console.error('[WorkflowToolbar] Error checking execution phase status:', error)
      return false
    }
  }) // Zustand will handle memoization - only re-render if result changes

  // Per-tab live status (busy/idle/stopped) + Stop now live inside each chat tab
  // pill (see WorkflowChatTabs), so the toolbar no longer renders a status badge.

  // Load saved settings when preset changes
  useEffect(() => {
    if (presetQueryId) {
      loadSavedSettings(presetQueryId)
    }
  }, [presetQueryId, loadSavedSettings])

  // Restore selection from localStorage after workspace state finishes loading
  // This ensures localStorage values are restored AFTER all API data is loaded
  const hasRestoredRef = useRef(false)
  useEffect(() => {
    // Only restore once when workspace loading completes and manifest is available
    if (!isLoadingWorkspaceState && variablesManifest && !hasRestoredRef.current) {
      restoreSelectionFromLocalStorage()
      hasRestoredRef.current = true
    }
    // Reset the flag when workspace starts loading (preset change)
    if (isLoadingWorkspaceState) {
      hasRestoredRef.current = false
    }
  }, [isLoadingWorkspaceState, variablesManifest, restoreSelectionFromLocalStorage])

  // Restore selectedGroupIds from execution state when page refreshes during execution
  // This handles the case where execution is running but selectedGroupIds was lost on page refresh
  useEffect(() => {
    if (isExecutionRunning && selectedGroupIds.length === 0 && currentRunningGroupId) {
      // If execution is running but no groups are selected, restore from currentRunningGroupId
      console.log('[WorkflowToolbar] Restoring selectedGroupIds from currentRunningGroupId:', currentRunningGroupId)
      setSelectedGroupIds([currentRunningGroupId])
    } else if (isExecutionRunning && selectedGroupIds.length === 0 && variablesManifest?.groups) {
      // If we have groups in manifest but none selected, try to infer from selectedRunFolder
      // Extract group ID from selectedRunFolder if it's a group path
      if (selectedRunFolder && selectedRunFolder.includes('/')) {
        const parts = selectedRunFolder.split('/')
        if (parts.length === 2) {
          const groupFolderName = parts[1]
          // Try to find matching group in manifest
          const matchingGroup = variablesManifest.groups.find(g => {
            if (g.name === groupFolderName) return true
            const sanitized = groupFolderName.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/-+/g, '-').trim()
            const groupSanitized = g.name.toLowerCase().replace(/[^a-z0-9-]/g, '-').replace(/-+/g, '-').trim()
            return sanitized === groupSanitized
          })
          if (matchingGroup) {
            console.log('[WorkflowToolbar] Restoring selectedGroupIds from selectedRunFolder:', matchingGroup.name)
            setSelectedGroupIds([matchingGroup.name])
          }
        }
      }
    }
  }, [isExecutionRunning, selectedGroupIds.length, currentRunningGroupId, variablesManifest, selectedRunFolder, setSelectedGroupIds])

  // selectedGroupIds is already included in the batched selector above
  
  // Settings are no longer persisted to localStorage - removed save logic

  // NOTE: loadRunFolders is NOT called here anymore.
  // useWorkspaceState in WorkflowCanvas handles initial load of:
  // - run_folders (via setRunFolders)
  // - variables_manifest (via setVariablesManifest)
  // This eliminates duplicate API calls on initial page load.

  const scheduleTooltip = useMemo(() => {
    if (workflowScheduleStats.total === 0) return 'Schedules · None configured'
    if (workflowScheduleStats.issues > 0 || workflowScheduleStats.missed > 0) {
      const parts: string[] = []
      if (workflowScheduleStats.issues > 0) {
        parts.push(`${workflowScheduleStats.issues} failed`)
      }
      if (workflowScheduleStats.missed > 0) {
        parts.push(`${workflowScheduleStats.missed} missed`)
      }
      if (workflowScheduleStats.running > 0) {
        parts.push(`${workflowScheduleStats.running} running`)
      }
      return `Schedules · ${parts.join(' · ')}`
    }
    if (workflowScheduleStats.running > 0) {
      return `Schedules · ${workflowScheduleStats.running} running`
    }
    if (workflowScheduleStats.enabled > 0) {
      return `Schedules · ${workflowScheduleStats.enabled} enabled`
    }
    return `Schedules · ${workflowScheduleStats.total === 1 ? 'Paused' : `All ${workflowScheduleStats.total} paused`}`
  }, [workflowScheduleStats])
  const scheduleStatusDotClass = workflowScheduleStats.issues > 0
    ? 'bg-red-500'
    : workflowScheduleStats.missed > 0
        ? 'bg-amber-500'
        : workflowScheduleStats.running > 0
          ? 'bg-sky-500 animate-pulse'
          : workflowScheduleStats.enabled > 0
            ? 'bg-emerald-500'
            : 'bg-muted-foreground/40'

  return (
    <>
    <div className={`
      flex min-h-10 min-w-0 flex-nowrap items-center gap-3 overflow-visible px-3 py-1.5
      bg-background border-b border-border
      relative z-10
      ${className}
    `}>
      {/* Left side - chat tab strip (grows). Per-tab status dot + Stop live
          inside each tab pill (WorkflowChatTabs), not as a separate badge here. */}
      <div className="flex min-w-0 flex-1 items-center gap-3 overflow-hidden">
        {chatTabsSlot}
      </div>

      {/* Center - Status indicator */}
      <div className="flex shrink-0 items-center gap-1.5">
        {status === 'waiting_feedback' && (
          <div className="flex items-center gap-1.5 px-2 py-1 bg-amber-100 dark:bg-amber-900/30 text-amber-700 dark:text-amber-300 rounded-md text-xs">
            <div className="w-1.5 h-1.5 bg-amber-500 rounded-full animate-pulse" />
            Waiting for feedback
          </div>
        )}
        {status === 'failed' && (
          <div className="flex items-center gap-1.5 px-2 py-1 bg-red-100 dark:bg-red-900/30 text-red-700 dark:text-red-300 rounded-md text-xs">
            <div className="w-1.5 h-1.5 bg-red-500 rounded-full" />
            Failed
          </div>
        )}
      </div>

      {/* Right side - View controls */}
      <div data-tour="workflow-tools" data-testid="tour-workflow-tools" className="ml-auto flex shrink-0 items-center gap-1">
        <TooltipProvider delayDuration={150}>
          {workspacePath && (
            <>
              <div className="inline-flex h-8 items-center gap-0.5 rounded-lg border border-border bg-muted/60 p-0.5 shadow-sm">
                {workspaceViewDefinitions.map(({ id: view, icon: Icon, label }) => {
                  const active = view === activeWorkspaceView
                  const viewButton = (
                    <button
                      type="button"
                      onClick={() => openWorkspaceView(view)}
                      className={`flex h-6 w-7 items-center justify-center rounded transition-colors ${active ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'}`}
                      aria-label={label}
                      aria-pressed={active}
                    >
                      <Icon className="h-3.5 w-3.5" />
                    </button>
                  )

                  // A clicked view button retains focus. A tooltip on that selected
                  // button can remain over the report/plan header, where it is both
                  // redundant and, in Electron, sometimes renders as an empty panel.
                  // Keep discovery labels for unselected icons only.
                  if (active) {
                    return <React.Fragment key={view}>{viewButton}</React.Fragment>
                  }

                  return (
                  <Tooltip key={view}>
                    <TooltipTrigger asChild>
                      {viewButton}
                    </TooltipTrigger>
                    <TooltipContent side="bottom"><p>{label}</p></TooltipContent>
                  </Tooltip>
                  )
                })}
              </div>

            </>
          )}

          {/* Pulse is the operational hub for monitoring, schedules, backup,
              publishing, and notifications. */}
          {workspacePath && (
            <div className="inline-flex h-8 items-center rounded-lg border border-border bg-background/90 shadow-sm backdrop-blur-sm">
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    onClick={() => setShowMonitorHelp(true)}
                    className="inline-flex h-full items-center gap-1.5 px-2 text-[11px] font-medium text-muted-foreground transition-colors hover:bg-muted"
                  >
                    <Activity className={`h-3.5 w-3.5 ${monitorOn ? 'text-primary' : ''}`} />
                    <span className={monitorOn ? 'text-foreground' : ''}>Pulse</span>
                    <span className={`text-[10px] font-semibold tracking-wide ${monitorOn ? 'text-primary' : 'text-muted-foreground/60'}`}>{monitorOn ? 'ON' : 'OFF'}</span>
                  </button>
                </TooltipTrigger>
                <TooltipContent side="bottom"><p>Pulse status and module cadence</p></TooltipContent>
              </Tooltip>
              <span className="h-4 w-px bg-border" aria-hidden="true" />
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    onClick={() => openWorkspaceView('evaluation')}
                    className="flex h-full w-8 items-center justify-center text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                    aria-label="Evaluation"
                  >
                    <ClipboardCheck className="h-3.5 w-3.5" />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="bottom"><p>Evaluation results</p></TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    onClick={() => openWorkspaceView('schedules')}
                    className="relative flex h-full w-8 items-center justify-center text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                    aria-label="Schedules"
                  >
                    <CalendarClock className="h-3.5 w-3.5" />
                    <span className={`absolute right-1.5 top-1.5 h-1.5 w-1.5 rounded-full border border-background ${scheduleStatusDotClass}`} />
                  </button>
                </TooltipTrigger>
                <TooltipContent side="bottom"><p>{scheduleTooltip}</p></TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <button
                    type="button"
                    onClick={runPulseNow}
                    disabled={!canWriteWorkflow || manualPulseStarting}
                    className="flex h-full w-8 items-center justify-center text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40"
                    aria-label="Run Pulse now"
                  >
                    {manualPulseStarting
                      ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
                      : <Play className="h-3.5 w-3.5" />}
                  </button>
                </TooltipTrigger>
                <TooltipContent side="bottom"><p>Run Pulse on the latest retained run</p></TooltipContent>
              </Tooltip>
            </div>
          )}

        {/* Workflow Access (multi-user mode only, owners only) */}
        {canManageAccess && (
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                onClick={() => setShowAccessPopup(true)}
                className="p-1.5 rounded-md bg-muted text-muted-foreground hover:bg-accent hover:text-accent-foreground transition-colors"
              >
                <ShieldCheck className="w-3.5 h-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="bottom"><p>Automation Access</p></TooltipContent>
          </Tooltip>
        )}

        {/* Workflow capabilities — write-only (read users don't see this) */}
        {canWriteWorkflow && (
          <div className="inline-flex h-8 items-center gap-0.5 rounded-lg border border-border bg-muted/60 p-0.5 shadow-sm">
            {capabilityViewDefinitions.map(({ id, icon: Icon, label }) => {
              const active = workflowWorkspaceView === id
              return (
                <Tooltip key={id}>
                  <TooltipTrigger asChild>
                    <button onClick={() => openWorkspaceView(id)} {...CAPABILITY_BUTTON_ATTRS[id]} className={`flex h-6 w-7 items-center justify-center rounded transition-colors ${active ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:bg-background/70 hover:text-foreground'}`} aria-label={label} aria-pressed={active}>
                      <Icon className="w-3.5 h-3.5" />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent side="bottom"><p>{label}</p></TooltipContent>
                </Tooltip>
              )
            })}
          </div>
        )}

        </TooltipProvider>
      </div>
    </div>
    {/* Database-native Pulse workspace */}
    {showMonitorHelp && (
      <ModalPortal>
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={() => { setShowMonitorHelp(false) }}>
          <div className="flex h-[calc(100vh-1rem)] w-[calc(100vw-1rem)] max-w-7xl flex-col overflow-hidden rounded-lg border bg-background shadow-xl sm:h-[calc(100vh-2rem)] sm:w-[calc(100vw-2rem)]" onClick={(e) => e.stopPropagation()}>
            <div className="flex shrink-0 items-center justify-between gap-3 border-b px-4 py-3.5 sm:px-5">
              <div className="flex min-w-0 items-center gap-3">
                <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-primary/25 bg-primary/10 text-primary">
                  <Activity className="h-4 w-4" />
                </div>
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <h2 className="text-sm font-semibold text-foreground">Pulse</h2>
                    <span className={`rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide ${monitorOn ? 'border-primary/25 bg-primary/10 text-primary' : 'border-border bg-muted text-muted-foreground'}`}>
                      {monitorOn ? 'On' : 'Off'}
                    </span>
                  </div>
                  <div className="mt-0.5 flex flex-wrap items-center gap-x-2 text-[11px] text-muted-foreground">
                    <span>{pulseOverview.recorded}/{pulseOverview.total} statuses recorded</span>
                    {pulseOverview.latest && <span>Updated {pulseOverview.latest}</span>}
                  </div>
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-1">
                {monitorOn && (
                  <button
                    type="button"
                    onClick={() => {
                      window.dispatchEvent(new CustomEvent(WORKFLOW_SOUL_REFRESH_EVENT))
                      void refreshPulseModuleStates()
                    }}
                    disabled={pulseStatusLoading}
                    className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground disabled:opacity-60"
                    aria-label="Refresh Pulse status"
                    title="Refresh Pulse status"
                  >
                    <RefreshCw className={`h-3.5 w-3.5 ${pulseStatusLoading ? 'animate-spin' : ''}`} />
                  </button>
                )}
                <button onClick={() => { setShowMonitorHelp(false) }} className="rounded-md p-1.5 text-muted-foreground hover:bg-accent hover:text-foreground" aria-label="Close">
                  <X className="h-4 w-4" />
                </button>
              </div>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto">
              <div className="p-3 sm:p-4">
                {workspacePath && (
                  <PulseWorkspace
                    workspacePath={workspacePath}
                    moduleStates={pulseModuleStates}
                    finalCommandStates={pulseFinalCommandStates}
                    reviewFocuses={pulseReviewFocuses}
                    reviewFocusSelections={pulseReviewFocusSelections}
                    statusError={pulseStatusError}
                  />
                )}
              </div>
            </div>

            {/* Pulse control and related workflow operations */}
            <div className="flex shrink-0 flex-col gap-3 border-t bg-background px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5">
              <div className="flex min-w-0 items-center gap-3">
                <button
                  type="button"
                  role="switch"
                  aria-checked={monitorOn}
                  onClick={() => { void toggleMonitor() }}
                  disabled={monitorSaving}
                  className={`relative inline-flex h-5 w-9 flex-none items-center rounded-full p-0 transition-colors disabled:opacity-50 ${monitorOn ? 'bg-primary' : 'bg-muted-foreground/30'}`}
                  aria-label="Toggle Pulse"
                >
                  <span className={`inline-block h-4 w-4 rounded-full bg-white shadow-sm transition-transform ${monitorOn ? 'translate-x-[18px]' : 'translate-x-[2px]'}`} />
                </button>
                <div className="min-w-0">
                  <div className="text-xs font-medium text-foreground">{monitorOn ? 'Reviews scheduled runs' : 'Pulse is off'}</div>
                  <div className="truncate text-[11px] text-muted-foreground">{monitorOn ? 'Pulse Gate runs after each normal scheduled run.' : 'Turn on to review completed scheduled runs.'}</div>
                </div>
              </div>
              <div className="inline-flex h-8 items-center overflow-hidden rounded-lg border border-border bg-muted/30">
                <button
                  type="button"
                  onClick={() => { setShowMonitorHelp(false); setShowBackupPopup(true) }}
                  className="relative inline-flex h-full items-center gap-1.5 px-3 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <Cloud className="h-3.5 w-3.5" />
                  Backup
                  <span className={`h-1.5 w-1.5 rounded-full ${getBackupDotClass(backupState)}`} />
                </button>
                <span className="h-4 w-px bg-border" aria-hidden="true" />
                <button
                  type="button"
                  onClick={() => { setShowMonitorHelp(false); setShowPublishPopup(true) }}
                  className="relative inline-flex h-full items-center gap-1.5 px-3 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <Globe className="h-3.5 w-3.5" />
                  Publish
                  <span className={`h-1.5 w-1.5 rounded-full ${getPublishDotClass(publishState)}`} />
                </button>
                <span className="h-4 w-px bg-border" aria-hidden="true" />
                <button
                  type="button"
                  data-testid="workflow-notification-settings-button"
                  onClick={() => { setShowMonitorHelp(false); setShowNotifications(true) }}
                  className="relative inline-flex h-full items-center gap-1.5 px-3 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  <BellRing className="h-3.5 w-3.5" />
                  Notify
                  <span className={`h-1.5 w-1.5 rounded-full ${getNotificationDotClass(notificationState)}`} />
                </button>
              </div>
            </div>
          </div>
        </div>
      </ModalPortal>
    )}

    {/* Backup Popup (dedicated remote backup status + strategy) */}
    <WorkflowBackupPopup
      isOpen={showBackupPopup}
      onClose={() => { setShowBackupPopup(false); refreshBackupState() }}
      workspacePath={workspacePath || null}
      onStateLoaded={setBackupState}
    />

    {/* Publish Popup (share HTML to a public URL) */}
    <WorkflowPublishPopup
      isOpen={showPublishPopup}
      onClose={() => { setShowPublishPopup(false); refreshPublishState() }}
      workspacePath={workspacePath || null}
      onStateLoaded={setPublishState}
    />

    {/* Agentic notification status + builder-driven setup */}
    <WorkflowNotificationPopup
      isOpen={showNotifications}
      onClose={() => { setShowNotifications(false); void refreshNotificationState() }}
      workspacePath={workspacePath || null}
      onStateLoaded={setNotificationState}
    />

    {/* Workflow Access Popup (multi-user owners only) */}
    <WorkflowAccessPopup
      isOpen={showAccessPopup}
      onClose={() => setShowAccessPopup(false)}
    />
    </>
  )
}

WorkflowToolbar.whyDidYouRender = true

export default WorkflowToolbar
