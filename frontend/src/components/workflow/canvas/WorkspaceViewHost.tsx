import React, {
  Suspense,
  forwardRef,
  lazy,
  useCallback,
  useEffect,
  useImperativeHandle,
  useMemo,
  useRef,
  useState,
} from 'react'
import { ReactFlowProvider } from '@xyflow/react'
import Workspace from '../../Workspace'
import { FileContentViewerBody } from '../../FileContentViewer'
import { useWorkspaceStore } from '../../../stores/useWorkspaceStore'
import { WorkflowToolbar } from './WorkflowToolbar'
import { ReportView } from '../ReportViewer'
import { usePlanData } from '../hooks/usePlanData'
import { useEvaluationPlanData } from '../hooks/useEvaluationPlanData'
import { useWorkflowExecution } from '../hooks/useWorkflowExecution'
import { useWorkspaceState } from '../hooks/useWorkspaceState'
import { useWorkflowStore } from '../../../stores/useWorkflowStore'
import { useAppStore } from '../../../stores/useAppStore'
import type { ExecutionOptions, VariablesManifest } from '../../../services/api-types'
import { assertNeverView, isInspectorView, type InspectorViewId } from '../workspaceViews'
import {
  WorkspaceViewDataContext,
  useWorkspaceViewData,
  type FlowShell,
  type WorkflowImageExportFormat,
  type WorkspaceViewData,
} from './workspaceViewData'
import {
  WORKFLOW_REPORT_EXPORT_EVENT,
  WorkflowCanvasInner,
  usePreviewDevice,
  type WorkflowCanvasProps,
  type WorkflowCanvasRef,
} from './WorkflowCanvas'

// Every inspector view is lazy: only the one the user opens is fetched, so the
// eager bundle carries the flow canvas and nothing else from this list. The
// single <Suspense> around the inspector slot below shows the same muted
// "Loading…" line the views use for their own data loading.
const CostsPopup = lazy(() => import('../CostsPopup'))
const ExecutionLogsPopup = lazy(() => import('../ExecutionLogsPopup'))
const LearningsView = lazy(() => import('../LearningsView'))
const KnowledgebaseView = lazy(() => import('../KnowledgebaseView'))
const DatabaseView = lazy(() => import('../DatabaseView'))
const PulseEvalSummary = lazy(() => import('../PulseEvalSummary').then(module => ({ default: module.PulseEvalSummary })))
const WorkflowScheduleRunsPanel = lazy(() => import('../../scheduler/WorkflowScheduleRunsPanel'))
const WorkflowCapabilitiesPanel = lazy(() => import('../WorkflowCapabilitiesPanel'))
const WorkflowFolderAccessView = lazy(() => import('../WorkflowFolderAccessView'))

type ViewKind = 'flow' | 'report' | 'files' | 'inspector'

const noop = () => {}
const dispatchReportExport = () => window.dispatchEvent(new CustomEvent(WORKFLOW_REPORT_EXPORT_EVENT))

// ---------------------------------------------------------------------------
// View bodies: what goes under the toolbar for each kind. Each renders only
// the pane content; the host owns the root, the toolbar, and the pane wrapper.
// ---------------------------------------------------------------------------

function ReportBody({ workspacePath }: { workspacePath: string | null }) {
  // In Mobile the report sits in a narrow 480px column; when chat is focused we
  // keep the report mobile-framed so it fits. Laptop keeps its desktop report
  // beside a separate compact chat column, so it needs no report-width cap.
  const reportPreviewDevice = usePreviewDevice(workspacePath)
  const reportFocusTier: 'mobile' | undefined =
    useWorkflowStore(state => state.focusedPane === 'chat' && reportPreviewDevice === 'mobile')
      ? 'mobile'
      : undefined
  return (
    <div className="h-full min-h-0 relative">
      {workspacePath && <ReportView workspacePath={workspacePath} focusTier={reportFocusTier} />}
    </div>
  )
}

function FilesBody() {
  const canvasViewMode = useWorkflowStore(state => state.canvasViewMode)
  // While a file is open the pane shows the viewer instead of the tree. The
  // tree stays mounted (hidden) so its scroll position and any in-progress
  // search survive a round trip into a file and back.
  const showFileContent = useWorkspaceStore(state => state.showFileContent)
  const handleCloseFiles = useCallback(() => {
    useAppStore.getState().setWorkspaceMinimized(true)
    useWorkflowStore.getState().setWorkflowWorkspaceView(canvasViewMode)
  }, [canvasViewMode])
  return (
    <div className="relative flex h-full min-h-0 flex-col bg-background">
      <div className="min-h-0 flex-1" hidden={showFileContent}>
        <Workspace
          minimized={false}
          onToggleMinimize={handleCloseFiles}
          hideMinimizeControl
        />
      </div>
      {showFileContent && (
        <div className="min-h-0 flex-1">
          <FileContentViewerBody variant="pane" />
        </div>
      )}
    </div>
  )
}

function InspectorBody({ workspacePath, presetQueryId }: { workspacePath: string | null; presetQueryId: string | null }) {
  const workflowWorkspaceView = useWorkflowStore(state => state.workflowWorkspaceView)
  const { planData, selectedRunFolder, runFolderNames, workspace } = useWorkspaceViewData()
  const plan = planData.plan
  const refreshWorkspaceState = workspace.refresh

  const closeInspector = useCallback(() => {
    useWorkflowStore.getState().setShowWorkspacePane(false)
  }, [])

  // One explicit branch per inspector view. The `default` is a compile-time
  // exhaustiveness check: a view added to the registry without a branch here
  // is a type error, not a silent fallthrough into some other view.
  const renderInspector = (view: InspectorViewId) => {
    switch (view) {
      case 'costs':
        return (
          <CostsPopup
            isOpen
            embedded
            onClose={closeInspector}
            workspacePath={workspacePath}
            runFolders={runFolderNames}
            selectedRunFolder={selectedRunFolder}
          />
        )
      case 'execution-logs':
        return (
          <ExecutionLogsPopup
            isOpen
            embedded
            onClose={closeInspector}
            workspacePath={workspacePath}
            runFolder={selectedRunFolder}
            runFolders={runFolderNames}
            onRefreshRunFolders={refreshWorkspaceState}
          />
        )
      case 'learnings':
        return <LearningsView workspacePath={workspacePath} plan={plan} />
      case 'knowledgebase':
        return <KnowledgebaseView workspacePath={workspacePath} />
      case 'database':
        return <DatabaseView workspacePath={workspacePath} />
      case 'evaluation':
        return (
          <div className="h-full overflow-y-auto">
            <PulseEvalSummary workspacePath={workspacePath || ''} className="min-h-full rounded-none border-0" />
          </div>
        )
      case 'schedules':
        return (
          <WorkflowScheduleRunsPanel
            embedded
            workflowScope={{ presetQueryId: presetQueryId || undefined, workspacePath: workspacePath || undefined }}
            onClose={closeInspector}
          />
        )
      case 'folders':
        return <WorkflowFolderAccessView workspacePath={workspacePath} />
      case 'skills':
      case 'mcp':
      case 'secrets':
      case 'browser':
      case 'llm':
      case 'bots':
        return <WorkflowCapabilitiesPanel section={view} workspacePath={workspacePath} />
      default:
        return assertNeverView(view)
    }
  }

  if (!isInspectorView(workflowWorkspaceView)) return null
  return (
    <Suspense
      fallback={
        <div className="flex items-center justify-center gap-2 py-12 text-sm text-muted-foreground">
          Loading…
        </div>
      }
    >
      {renderInspector(workflowWorkspaceView)}
    </Suspense>
  )
}

// ---------------------------------------------------------------------------
// Host
// ---------------------------------------------------------------------------

/**
 * The one component that owns the right-side workspace pane: it loads the
 * toolbar data once, renders the toolbar once, and mounts the selected view
 * below it. Its root element and the toolbar keep their identity across view
 * switches; only the body swaps. Before this, four sibling components each
 * re-declared the same hooks and toolbar, and switching between them
 * unmounted and rebuilt the whole pane.
 *
 * Report and Pulse (log) are lightweight preview-pane views with no React
 * Flow tree; the flow canvas is wrapped in its own ReactFlowProvider so
 * report mode never repaints on flow-only store traffic.
 */
export const WorkspaceViewHost = React.memo(forwardRef<WorkflowCanvasRef, WorkflowCanvasProps>((props, ref) => {
  const {
    workspacePath,
    presetQueryId,
    currentPhase,
    onStartPhase,
    onCreatePlan,
    showChatArea = false,
    onToggleChatArea,
    toolbarOnly = false,
    sharedToolbar = false,
    chatTabsSlot,
    paneClassName = '',
    className = '',
    viewMode,
    hideToolbar = false,
    embeddedPlanOnly = false,
    openPulseOnMount = false,
  } = props

  const selectedRunFolder = useWorkflowStore(state => state.selectedRunFolder)
  const canvasViewMode = useWorkflowStore(state => state.canvasViewMode)
  const workflowWorkspaceView = useWorkflowStore(state => state.workflowWorkspaceView)
  const effectiveCanvasViewMode = viewMode || canvasViewMode

  // Legacy saved Soul state opens Pulse; Goal context now lives inside the
  // database-native Pulse workspace.
  const kind: ViewKind = !embeddedPlanOnly && workflowWorkspaceView === 'files'
    ? 'files'
    : isInspectorView(workflowWorkspaceView)
      ? 'inspector'
      : !embeddedPlanOnly && (effectiveCanvasViewMode === 'report' || effectiveCanvasViewMode === 'log' || effectiveCanvasViewMode === 'soul')
        ? 'report'
        : 'flow'

  // --- Toolbar data, loaded once for every view ---------------------------
  const planData = usePlanData(workspacePath)
  const evalData = useEvaluationPlanData(workspacePath)
  const { status } = useWorkflowExecution()
  const workspace = useWorkspaceState(workspacePath, selectedRunFolder)
  const plan = planData.plan
  const workspaceState = workspace.state
  const isLoadingWorkspaceState = workspace.loading

  // The flow canvas's VariablesSidebar edits the manifest in place; keep the
  // toolbar showing that edit until the next workspace refresh replaces it.
  const [variablesManifest, setVariablesManifest] = useState<VariablesManifest | null>(null)
  const [isLoadingVariables, setIsLoadingVariables] = useState(false)
  useEffect(() => {
    if (workspaceState) {
      setVariablesManifest(workspaceState.variables_manifest || null)
      setIsLoadingVariables(false)
    } else if (!isLoadingWorkspaceState) {
      setVariablesManifest(null)
      setIsLoadingVariables(false)
    } else {
      setIsLoadingVariables(isLoadingWorkspaceState)
    }
  }, [workspaceState, isLoadingWorkspaceState])

  const runFoldersForToolbar = useMemo(() => {
    if (!workspaceState?.run_folders) return []
    return workspaceState.run_folders.map(f => ({ name: f.name }))
  }, [workspaceState?.run_folders])
  const runFolderNames = useMemo(
    () => runFoldersForToolbar.map(folder => folder.name),
    [runFoldersForToolbar],
  )

  // --- Flow shell: the flow canvas replaces the whole pane (toolbar included)
  // while its data is loading or errored. Computed here so the toolbar and
  // the body agree.
  const [isRefreshingPlan, setIsRefreshingPlan] = useState(false)
  const flowLoading = (planData.loading && !isRefreshingPlan) || evalData.loading
  const isPlanNotFoundError = planData.error && /not found|does not exist|planning must be run first/i.test(planData.error)
  const flowError = isPlanNotFoundError ? null : planData.error
  const flowShell: FlowShell = (flowLoading || isLoadingWorkspaceState)
    ? 'loading'
    : (flowError || workspace.error)
      ? 'error'
      : 'ready'

  // --- Image export lives in the flow canvas (it needs the React Flow
  // instance); the toolbar lives here. Bridge with a registration.
  const exportHandlerRef = useRef<((format: WorkflowImageExportFormat) => Promise<void>) | null>(null)
  const registerExportHandler = useCallback((handler: ((format: WorkflowImageExportFormat) => Promise<void>) | null) => {
    exportHandlerRef.current = handler
  }, [])
  const exportFlowImage = useCallback(() => {
    void exportHandlerRef.current?.('png')
  }, [])

  const hasPlan = Boolean(plan?.steps?.length)
  const onExport = kind === 'report'
    ? dispatchReportExport
    : kind === 'flow' && flowShell === 'ready' && hasPlan
      ? (effectiveCanvasViewMode === 'report' ? dispatchReportExport : exportFlowImage)
      : undefined

  // Stable identity: the toolbar is 1,100 lines with a dozen subscriptions,
  // and an inline arrow here re-rendered it on every host render.
  const handleStartPhase = useCallback((phaseId: string, executionOptions?: ExecutionOptions) => {
    onStartPhase?.(phaseId, executionOptions)
  }, [onStartPhase])

  // --- Imperative API: shared refresh for every view; the flow canvas keeps
  // its own granular refresh and node focus and is delegated to when active.
  const flowRef = useRef<WorkflowCanvasRef>(null)
  const loadPlanRefresh = planData.refresh
  const refreshWorkspaceState = workspace.refresh
  const sharedRefresh = useCallback(async () => {
    await Promise.all([loadPlanRefresh(), refreshWorkspaceState()])
  }, [loadPlanRefresh, refreshWorkspaceState])
  useImperativeHandle(ref, () => ({
    refresh: async (changedStepIDs?: string[], deletedStepIDs?: string[]) => {
      if (kind === 'flow' && flowRef.current) {
        return flowRef.current.refresh(changedStepIDs, deletedStepIDs)
      }
      await sharedRefresh()
      return null
    },
    getStepCount: () => plan?.steps?.length ?? 0,
    focusStep: (stepId: string) => {
      flowRef.current?.focusStep(stepId)
    },
  }), [kind, sharedRefresh, plan])

  const data = useMemo<WorkspaceViewData>(() => ({
    planData,
    evalData,
    status,
    workspace,
    selectedRunFolder,
    runFolderNames,
    variablesManifest,
    isLoadingVariables,
    setVariablesManifest,
    isRefreshingPlan,
    setIsRefreshingPlan,
    flowShell,
    flowError,
    registerExportHandler,
  }), [
    planData, evalData, status, workspace, selectedRunFolder, runFolderNames,
    variablesManifest, isLoadingVariables, isRefreshingPlan, flowShell, flowError, registerExportHandler,
  ])

  const showToolbar = !hideToolbar && !(kind === 'flow' && flowShell !== 'ready')
  const gridToolbar = sharedToolbar && showChatArea

  let body: React.ReactNode
  if (kind === 'flow') {
    // The flow canvas handles `toolbarOnly` itself: its loading and error
    // screens still show in that mode, only the plan is skipped.
    body = (
      <ReactFlowProvider>
        <WorkflowCanvasInner {...props} ref={flowRef} />
      </ReactFlowProvider>
    )
  } else if (toolbarOnly) {
    body = null
  } else if (kind === 'report') {
    body = <ReportBody workspacePath={workspacePath} />
  } else if (kind === 'files') {
    body = <FilesBody />
  } else {
    body = <InspectorBody workspacePath={workspacePath} presetQueryId={presetQueryId} />
  }

  return (
    <WorkspaceViewDataContext.Provider value={data}>
      <div className={`flex flex-col h-full ${className} ${gridToolbar ? 'contents' : ''}`}>
        {showToolbar && (
          <div className={gridToolbar ? 'col-start-1 row-start-1 md:col-span-2' : ''}>
            <WorkflowToolbar
              status={status}
              hasPlan={hasPlan}
              plan={plan || undefined}
              currentPhase={currentPhase}
              workspacePath={workspacePath}
              presetQueryId={presetQueryId}
              runFolders={runFoldersForToolbar}
              variablesManifest={variablesManifest}
              isLoadingWorkspaceState={isLoadingWorkspaceState}
              onStartPhase={handleStartPhase}
              onCreatePlan={onCreatePlan || noop}
              showChatArea={showChatArea}
              onToggleChatArea={onToggleChatArea}
              onExport={onExport}
              chatTabsSlot={chatTabsSlot}
              openPulseOnMount={kind === 'flow' ? openPulseOnMount : false}
            />
          </div>
        )}
        <div
          data-tour="workflow-canvas-pane"
          data-testid="tour-workflow-canvas-pane"
          className={`${gridToolbar ? 'flex-1 col-start-1 row-start-2 md:col-start-2' : 'flex-1'} ${paneClassName} min-h-0 ${kind === 'inspector' ? 'overflow-hidden border-l border-border' : ''}`}
        >
          {body}
        </div>
      </div>
    </WorkspaceViewDataContext.Provider>
  )
}))

WorkspaceViewHost.displayName = 'WorkspaceViewHost'

export default WorkspaceViewHost
