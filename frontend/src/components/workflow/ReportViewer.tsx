// HTML report viewer. A workflow report is simply the HTML page files under
// db/reports/: each file is one top-level report page. There is deliberately no
// report-plan JSON layout layer between the report and its reader.

import { createElement, memo, useCallback, useEffect, useMemo, useState } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { BarChart3, Loader2, RefreshCw } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { agentApi, workspaceApi } from '../../services/api'
import type { PlannerFile } from '../../services/api-types'
import { useReportFilePreviewStore } from '../../stores/useReportFilePreviewStore'
import {
  REPORT_PREVIEW_PREFERENCE_CHANGED_EVENT,
  readReportPreviewPreference,
  type ReportPreviewDevice,
} from '../../utils/reportPreviewPreference'
import ModalPortal from '../ui/ModalPortal'
import { FilePreviewModal } from './reportWidgets/FilePreviewModal'
import { HtmlReportFrame } from './reportWidgets/HtmlWidgetFrame'
import { ReportEmbedProvider, type ReportDataApi } from './reportWidgets/reportEmbedContext'
import { ReportHumanInputPanel } from './ReportHumanInputPanel'

const WORKFLOW_REPORT_REFRESH_EVENT = 'workflow-report-refresh-requested'

type ReportPage = {
  path: string
  label: string
  order: number
  html?: string
}

interface ReportViewerProps {
  workspacePath: string
  isOpen: boolean
  onClose: () => void
}

interface ReportViewProps {
  workspacePath: string
  selectedRunFolder?: string | null
  reviewData?: unknown
  onClose?: () => void
  focusTier?: 'mobile'
  reserveTopControlsSpace?: boolean
}

function normalizeSource(path: string): string {
  return path.replace(/\\/g, '/').replace(/^\/+/, '').replace(/\/+/g, '/')
}

function basename(path: string): string {
  return path.split('/').filter(Boolean).pop() || path
}

function fallbackLabel(path: string): string {
  const name = basename(path).replace(/\.(html?|HTML?)$/, '')
  return name.replace(/[-_]+/g, ' ').replace(/\b\w/g, character => character.toUpperCase()) || 'Report'
}

function parsePageMetadata(path: string, html: string): Pick<ReportPage, 'label' | 'order'> {
  const title = html.match(/<title[^>]*>\s*([^<]+?)\s*<\/title>/i)?.[1]?.trim()
  const orderRaw = html.match(/<meta\s+[^>]*name=["']report-order["'][^>]*content=["']([^"']+)["'][^>]*>/i)?.[1]
    ?? html.match(/<meta\s+[^>]*content=["']([^"']+)["'][^>]*name=["']report-order["'][^>]*>/i)?.[1]
  const order = Number(orderRaw)
  return { label: title || fallbackLabel(path), order: Number.isFinite(order) ? order : Number.MAX_SAFE_INTEGER }
}

function collectFiles(items: PlannerFile[], out: PlannerFile[] = []): PlannerFile[] {
  for (const item of items) {
    const children = Array.isArray(item.children) ? item.children : []
    if (item.type !== 'folder' && children.length === 0 && item.filepath) out.push(item)
    if (children.length > 0) collectFiles(children, out)
  }
  return out
}

async function readWorkspaceText(filepath: string): Promise<string | null> {
  try {
    const response = await agentApi.getPlannerFileContent(filepath)
    return response?.success && typeof response.data?.content === 'string' ? response.data.content : null
  } catch {
    return null
  }
}

function allowedReportPath(path: string): string {
  const normalized = normalizeSource(path)
  if (!normalized || normalized.split('/').includes('..')) return ''
  const allowedRoots = ['db/', 'knowledgebase/', 'docs/', 'planning/', 'evaluation/', 'costs/', 'variables/']
  const exactFiles = ['soul.md', 'workflow.json']
  return allowedRoots.some(root => normalized.startsWith(root)) || exactFiles.includes(normalized) ? normalized : ''
}

function useReportDataApi(workspacePath: string): ReportDataApi {
  return useMemo(() => {
    const getText = async (path: string): Promise<string | null> => {
      const allowed = allowedReportPath(path)
      return allowed ? readWorkspaceText(`${workspacePath}/${allowed}`) : null
    }
    const renderMarkdown = (markdown: string): string => {
      if (!markdown) return ''
      try {
        return `<div class="report-markdown">${renderToStaticMarkup(createElement(ReactMarkdown, { remarkPlugins: [remarkGfm] }, markdown))}</div>`
      } catch {
        return ''
      }
    }
    return {
      workspacePath,
      query: async (sql: string) => {
        const response = await agentApi.queryWorkflowDB(`${workspacePath}/db/db.sqlite`, sql)
        if (!response.success || !response.data) throw new Error(response.error || 'Query failed.')
        return response.data.rows
      },
      getText,
      get: async (path: string) => {
        const text = await getText(path)
        if (!text?.trim()) return null
        try { return JSON.parse(text) } catch { return text }
      },
      renderMarkdown,
      getHtml: async (path: string) => {
        const text = await getText(path)
        return text == null ? null : renderMarkdown(text) || null
      },
      fileUrl: async (path: string) => {
        const allowed = allowedReportPath(path)
        if (!allowed) return null
        try {
          const response = await workspaceApi.get(`/api/documents/${encodeURIComponent(`${workspacePath}/${allowed}`)}`, {
            params: { download: 'true' }, responseType: 'blob', headers: { Accept: 'application/octet-stream' }, transformResponse: [(data) => data],
          })
          const blob = response.data instanceof Blob ? response.data : new Blob([response.data])
          return URL.createObjectURL(blob)
        } catch {
          return null
        }
      },
      openFile: (path: string) => {
        const allowed = allowedReportPath(path)
        if (allowed) useReportFilePreviewStore.getState().show({ path: `${workspacePath}/${allowed}` })
      },
    }
  }, [workspacePath])
}

async function loadReportPages(workspacePath: string): Promise<ReportPage[]> {
  const response = await agentApi.getPlannerFiles(`${workspacePath}/db/reports`, 100, 1)
  const raw = Array.isArray(response) ? response : Array.isArray(response?.data) ? response.data : []
  const paths = collectFiles(raw)
    .map(file => file.filepath)
    .filter(path => /\.html?$/i.test(path))
    .filter(path => normalizeSource(path).startsWith(`${normalizeSource(workspacePath)}/db/reports/`))
    .sort((left, right) => left.localeCompare(right))
  const pages = await Promise.all(paths.map(async path => {
    const html = await readWorkspaceText(path)
    const metadata = parsePageMetadata(path, html || '')
    return { path, html: html || undefined, ...metadata }
  }))
  return pages.sort((left, right) => left.order - right.order || left.label.localeCompare(right.label))
}

function ReportPageNavigation({ pages, activePath, onSelect }: { pages: ReportPage[]; activePath: string; onSelect: (path: string) => void }) {
  if (pages.length < 2) return null
  return (
    <nav className="sticky top-0 z-20 border-b border-border/70 bg-background/95 px-2 py-2 backdrop-blur supports-[backdrop-filter]:bg-background/80 sm:px-3" aria-label="Report pages">
      <div className="flex gap-1 overflow-x-auto [scrollbar-width:thin]">
        {pages.map(page => (
          <button
            key={page.path}
            type="button"
            onClick={() => onSelect(page.path)}
            aria-current={page.path === activePath ? 'page' : undefined}
            className={page.path === activePath
              ? 'shrink-0 rounded-md border border-border bg-card px-3 py-1.5 text-sm font-semibold text-foreground shadow-sm'
              : 'shrink-0 rounded-md border border-transparent px-3 py-1.5 text-sm font-medium text-muted-foreground hover:bg-muted hover:text-foreground'}
          >
            {page.label}
          </button>
        ))}
      </div>
    </nav>
  )
}

function ReportViewComponent({ workspacePath, onClose, focusTier }: ReportViewProps) {
  const [pages, setPages] = useState<ReportPage[]>([])
  const [activePath, setActivePath] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [refreshNonce, setRefreshNonce] = useState(0)
  const [previewPreference, setPreviewPreference] = useState<ReportPreviewDevice>(() => readReportPreviewPreference(workspacePath))
  const dataApi = useReportDataApi(workspacePath)

  const refresh = useCallback(() => setRefreshNonce(value => value + 1), [])
  useEffect(() => {
    const sync = () => setPreviewPreference(readReportPreviewPreference(workspacePath))
    window.addEventListener(REPORT_PREVIEW_PREFERENCE_CHANGED_EVENT, sync)
    window.addEventListener(WORKFLOW_REPORT_REFRESH_EVENT, refresh)
    return () => {
      window.removeEventListener(REPORT_PREVIEW_PREFERENCE_CHANGED_EVENT, sync)
      window.removeEventListener(WORKFLOW_REPORT_REFRESH_EVENT, refresh)
    }
  }, [refresh, workspacePath])
  useEffect(() => setPreviewPreference(readReportPreviewPreference(workspacePath)), [workspacePath])
  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    void loadReportPages(workspacePath).then(next => {
      if (cancelled) return
      setPages(next)
      setActivePath(current => next.some(page => page.path === current) ? current : next[0]?.path || '')
    }).catch(reason => {
      if (!cancelled) setError(reason instanceof Error ? reason.message : 'Failed to load report.')
    }).finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [refreshNonce, workspacePath])

  const activePage = pages.find(page => page.path === activePath) || pages[0]
  const previewMode = focusTier || previewPreference
  const shellClass = previewMode === 'mobile' ? 'mx-auto w-full max-w-[480px] p-1.5' : 'w-full max-w-full'
  const runtime = useMemo(() => ({ data: dataApi }), [dataApi])

  return (
    <ReportEmbedProvider value={runtime}>
      <div className="flex h-full w-full flex-col overflow-hidden bg-background text-foreground">
        {onClose && <div className="flex shrink-0 justify-end border-b border-border/60 px-3 py-2"><button type="button" onClick={onClose} aria-label="Close report" className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-border text-lg text-muted-foreground hover:bg-muted hover:text-foreground">×</button></div>}
        <ReportPageNavigation pages={pages} activePath={activePage?.path || ''} onSelect={setActivePath} />
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-y-contain">
          <div className={shellClass}>
            <ReportHumanInputPanel workspacePath={workspacePath} contentMode="pending" />
            {loading && <div className="flex min-h-64 items-center justify-center gap-2 text-sm text-muted-foreground"><Loader2 className="h-4 w-4 animate-spin" /> Loading report…</div>}
            {error && <div className="m-3 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">Failed to load report: {error}</div>}
            {!loading && !error && !activePage && <div className="m-3 flex flex-col items-center gap-3 rounded-xl border border-dashed border-border p-8 text-center"><BarChart3 className="h-8 w-8 text-muted-foreground" /><div><div className="font-semibold">No HTML report yet</div><p className="mt-1 text-sm text-muted-foreground">Create an HTML page under <code>db/reports/</code>.</p></div><button type="button" onClick={refresh} className="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-sm hover:bg-muted"><RefreshCw className="h-3.5 w-3.5" /> Refresh</button></div>}
            {!loading && activePage && !activePage.html && <div className="m-3 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">Could not read {activePage.label}.</div>}
            {activePage?.html && <HtmlReportFrame html={activePage.html} title={activePage.label} autoHeight className="block w-full border-0" />}
          </div>
        </div>
        <FilePreviewModal />
      </div>
    </ReportEmbedProvider>
  )
}

export const ReportView = memo(ReportViewComponent)

export function ReportViewer({ workspacePath, isOpen, onClose }: ReportViewerProps) {
  if (!isOpen) return null
  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/60 px-2 py-3 backdrop-blur-sm sm:px-4 sm:py-6" onClick={onClose}>
        <div className="flex max-h-[94vh] w-full max-w-6xl flex-col overflow-hidden rounded-xl border border-border/70 bg-background shadow-[0_24px_80px_rgba(0,0,0,0.45)] sm:max-h-[90vh] sm:rounded-2xl" onClick={event => event.stopPropagation()}>
          <ReportView workspacePath={workspacePath} onClose={onClose} />
        </div>
      </div>
    </ModalPortal>
  )
}
