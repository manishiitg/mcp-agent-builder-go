import {
  BookOpen,
  Bot,
  BrainCircuit,
  CalendarClock,
  ClipboardCheck,
  Database,
  DollarSign,
  Files,
  FileText,
  FolderOpen,
  KeyRound,
  LayoutDashboard,
  Monitor,
  Puzzle,
  Route,
  Server,
  Table2,
  type LucideIcon,
} from 'lucide-react'

/**
 * The single registry of right-side workspace views.
 *
 * Every other list of "which views exist" -- the store's view type, its
 * normalizer, the toolbar's button groups, the canvas host's dispatch, the
 * layout's pane-visibility check, and the capabilities panel's section type --
 * derives from this array. Add a view here and the type system carries it
 * everywhere; forget a dispatch branch and `assertNever` in the canvas host
 * fails to compile.
 *
 * `kind` is what the host and layout switch on:
 *   - `canvas`     the React Flow plan (`flow`), or the default builder
 *                  workspace (`builder`, an alias for "no explicit view":
 *                  nothing dispatches it, the host falls through to
 *                  `canvasViewMode`).
 *   - `preview`    lightweight preview-pane views with no React Flow tree
 *                  (`report`, `log`; `soul` is legacy and is remapped to `log`
 *                  by `normalizeWorkspaceViewId`, kept here only so existing
 *                  comparisons still type-check).
 *   - `files`      the file browser.
 *   - `inspector`  a full-pane inspector (costs, logs, learnings, ...).
 *   - `capability` a section of `WorkflowCapabilitiesPanel`.
 *
 * `pane` says whether opening the view means the workspace pane is showing
 * content (every kind except `builder`/`files`, which the layout treats as the
 * default workspace rather than an open pane).
 */
export type WorkspaceViewKind = 'canvas' | 'preview' | 'files' | 'inspector' | 'capability'

export type WorkspaceViewDef = {
  id: string
  kind: WorkspaceViewKind
  /** Toolbar label; also the aria-label and tooltip of the toolbar button. */
  label: string
  icon: LucideIcon
  /** Which toolbar cluster the view's button belongs to, if it has one. */
  toolbarGroup?: 'views' | 'pulse' | 'capabilities'
  /** Opening this view shows the workspace pane. */
  pane: boolean
  /** The section renders inside a bounded flex column and manages its own
   * overflow, rather than letting the capabilities panel scroll it. */
  managesOwnScroll?: true
}

const VIEWS = [
  // -- canvas / preview (dispatched by WorkflowCanvasWithProvider) --------
  { id: 'builder', kind: 'canvas', label: 'Builder', icon: Route, pane: false },
  { id: 'report', kind: 'preview', label: 'Report', icon: LayoutDashboard, toolbarGroup: 'views', pane: true },
  { id: 'flow', kind: 'canvas', label: 'Plan', icon: Route, toolbarGroup: 'views', pane: true },
  { id: 'log', kind: 'preview', label: 'Pulse', icon: FileText, pane: true },
  { id: 'soul', kind: 'preview', label: 'Pulse', icon: FileText, pane: true },
  // -- inspectors (toolbar "views" cluster, in button order) ---------------
  { id: 'costs', kind: 'inspector', label: 'Costs', icon: DollarSign, toolbarGroup: 'views', pane: true },
  { id: 'execution-logs', kind: 'inspector', label: 'Execution logs', icon: FileText, toolbarGroup: 'views', pane: true },
  { id: 'learnings', kind: 'inspector', label: 'Learnings', icon: BookOpen, toolbarGroup: 'views', pane: true },
  { id: 'knowledgebase', kind: 'inspector', label: 'Knowledgebase', icon: Database, toolbarGroup: 'views', pane: true },
  { id: 'database', kind: 'inspector', label: 'Database', icon: Table2, toolbarGroup: 'views', pane: true },
  // -- files (last button of the "views" cluster) --------------------------
  { id: 'files', kind: 'files', label: 'Files', icon: Files, toolbarGroup: 'views', pane: false },
  // -- inspectors (toolbar "pulse" cluster) --------------------------------
  { id: 'evaluation', kind: 'inspector', label: 'Evaluation', icon: ClipboardCheck, toolbarGroup: 'pulse', pane: true },
  { id: 'schedules', kind: 'inspector', label: 'Schedules', icon: CalendarClock, toolbarGroup: 'pulse', pane: true },
  // -- capability sections (WorkflowCapabilitiesPanel), then folders -------
  { id: 'skills', kind: 'capability', label: 'Workflow skills', icon: Puzzle, toolbarGroup: 'capabilities', pane: true, managesOwnScroll: true },
  { id: 'secrets', kind: 'capability', label: 'Workflow secrets', icon: KeyRound, toolbarGroup: 'capabilities', pane: true, managesOwnScroll: true },
  { id: 'mcp', kind: 'capability', label: 'Workflow MCP servers', icon: Server, toolbarGroup: 'capabilities', pane: true, managesOwnScroll: true },
  { id: 'browser', kind: 'capability', label: 'Browser automation', icon: Monitor, toolbarGroup: 'capabilities', pane: true },
  { id: 'llm', kind: 'capability', label: 'Workflow LLM configuration', icon: BrainCircuit, toolbarGroup: 'capabilities', pane: true },
  { id: 'bots', kind: 'capability', label: 'Workflow bots', icon: Bot, toolbarGroup: 'capabilities', pane: true },
  { id: 'folders', kind: 'inspector', label: 'Attached folders', icon: FolderOpen, toolbarGroup: 'capabilities', pane: true },
] as const satisfies readonly WorkspaceViewDef[]

export type WorkspaceViewId = typeof VIEWS[number]['id']

/** A registry entry with its id narrowed to the known set and every optional
 * field present on the type, so callers can read `toolbarGroup` or
 * `managesOwnScroll` without narrowing per entry. */
export type WorkspaceView = WorkspaceViewDef & { id: WorkspaceViewId }

export const WORKSPACE_VIEWS: readonly WorkspaceView[] = VIEWS

type ViewOfKind<K extends WorkspaceViewKind> = Extract<typeof VIEWS[number], { kind: K }>['id']

export type InspectorViewId = ViewOfKind<'inspector'> | ViewOfKind<'capability'>
export type CapabilityViewId = ViewOfKind<'capability'>
export type PreviewViewId = ViewOfKind<'preview'>

const VIEW_BY_ID: ReadonlyMap<string, WorkspaceView> = new Map(
  WORKSPACE_VIEWS.map(view => [view.id, view] as const),
)

export function getWorkspaceView(id: WorkspaceViewId): WorkspaceView {
  return VIEW_BY_ID.get(id)!
}

export function isWorkspaceViewId(value: unknown): value is WorkspaceViewId {
  return typeof value === 'string' && VIEW_BY_ID.has(value)
}

/** Inspector-pane views: full-pane inspectors plus capability sections, which
 * the canvas host renders through the same inspector slot. */
export function isInspectorView(id: WorkspaceViewId | null | undefined): id is InspectorViewId {
  if (!id) return false
  const kind = VIEW_BY_ID.get(id)?.kind
  return kind === 'inspector' || kind === 'capability'
}

export function isCapabilityView(id: WorkspaceViewId | null | undefined): id is CapabilityViewId {
  return Boolean(id) && VIEW_BY_ID.get(id as string)?.kind === 'capability'
}

export function isPreviewView(id: WorkspaceViewId | null | undefined): id is PreviewViewId {
  return Boolean(id) && VIEW_BY_ID.get(id as string)?.kind === 'preview'
}

/** True when the view means the workspace pane is showing something other
 * than the default builder workspace or the file browser. */
export function isWorkspacePaneView(id: WorkspaceViewId | null | undefined): boolean {
  return Boolean(id) && VIEW_BY_ID.get(id as string)?.pane === true
}

export const CAPABILITY_VIEWS = WORKSPACE_VIEWS.filter(
  (view): view is WorkspaceView & { id: CapabilityViewId; kind: 'capability' } => view.kind === 'capability',
)

/** Legacy persisted ids that no longer exist as views of their own. */
const LEGACY_VIEW_IDS: Record<string, WorkspaceViewId> = {
  soul: 'log',
  plan: 'flow',
}

/** Coerce a persisted/unknown value to a view id, or null. Legacy ids are
 * remapped; anything unrecognised becomes null. */
export function normalizeWorkspaceViewId(value: unknown): WorkspaceViewId | null {
  if (value === null || value === undefined) return null
  if (typeof value !== 'string') return null
  const legacy = LEGACY_VIEW_IDS[value]
  if (legacy) return legacy
  return isWorkspaceViewId(value) ? value : null
}

/** Compile-time exhaustiveness helper for switches over view ids. Never
 * throws: an unhandled id is a type error, not a runtime failure. */
export function assertNeverView(_value: never): null {
  return null
}
