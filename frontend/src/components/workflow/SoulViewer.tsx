import { useCallback, useEffect, useState } from 'react'
import { CheckCircle2, Loader2, ShieldAlert, Target } from 'lucide-react'
import { agentApi } from '../../services/api'
import { MarkdownRenderer } from '../ui/MarkdownRenderer'
import { extractWorkflowSoulSummary } from './soulSummaryUtils'

// Fired by the Pulse popup refresh button so goal content and module status stay aligned.
export const WORKFLOW_SOUL_REFRESH_EVENT = 'workflow-soul-refresh'

interface SoulViewerProps {
  workspacePath: string
  embedded?: boolean
  pulseSummary?: boolean
}

// SoulViewer renders the workflow's north star (soul/soul.md — ## Objective +
// ## Success Criteria). soul.md stays markdown because framework health and
// runtime objective injection parse it directly.
export function SoulViewer({ workspacePath, embedded = false, pulseSummary = false }: SoulViewerProps) {
  const [content, setContent] = useState('')
  const [exists, setExists] = useState<boolean | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!workspacePath) return
    setLoading(true)
    setError(null)
    try {
      const res = await agentApi.getBuilderDoc(workspacePath, 'soul')
      setExists(!!res.exists)
      setContent(res.content || '')
      if (!res.success && res.error) setError(res.error)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [workspacePath])

  useEffect(() => { void load() }, [load])

  useEffect(() => {
    const onRefresh = () => { void load() }
    window.addEventListener(WORKFLOW_SOUL_REFRESH_EVENT, onRefresh)
    return () => window.removeEventListener(WORKFLOW_SOUL_REFRESH_EVENT, onRefresh)
  }, [load])

  if (loading && !content) {
    if (pulseSummary) {
      return (
        <div className="flex min-h-20 items-center justify-center gap-2 rounded-xl border bg-background text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading goal and success criteria…
        </div>
      )
    }
    return (
      <div className={`flex items-center justify-center gap-2 text-sm text-muted-foreground ${embedded ? 'min-h-40' : 'h-full'}`}>
        <Loader2 className="h-4 w-4 animate-spin" /> Loading soul…
      </div>
    )
  }

  if (error) {
    if (pulseSummary) {
      return (
        <div className="rounded-xl border border-red-200 bg-red-50 p-3 text-xs text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300">
          Goal and success criteria could not be loaded: {error}
        </div>
      )
    }
    return (
      <div className={`flex items-center justify-center p-6 ${embedded ? 'min-h-40' : 'h-full'}`}>
        <div className="max-w-md rounded-md border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-800 dark:bg-red-900/20 dark:text-red-300">
          {error}
        </div>
      </div>
    )
  }

  if (exists === false || !content.trim()) {
    if (pulseSummary) {
      return (
        <div className="rounded-xl border bg-background p-4 text-xs text-muted-foreground">
          No workflow goal or success criteria yet. Run <code className="rounded bg-muted px-1">/define-success</code>.
        </div>
      )
    }
    return (
      <div className={`flex items-center justify-center p-6 text-center ${embedded ? 'min-h-40' : 'h-full'}`}>
        <div className="max-w-md text-sm text-muted-foreground">
          No soul yet — the workflow's north star. Run <code className="rounded bg-muted px-1">/define-success</code> to
          confirm the <code className="rounded bg-muted px-1">## Objective</code> and <code className="rounded bg-muted px-1">## Success Criteria</code>. Then use <code className="rounded bg-muted px-1">/pulse-setup</code> if you want recurring Pulse.
        </div>
      </div>
    )
  }

  if (pulseSummary) {
    const summary = extractWorkflowSoulSummary(content)
    return (
      <section className="overflow-hidden rounded-xl border bg-background">
        <div className="grid gap-3 px-4 py-3.5 lg:grid-cols-3 sm:px-5">
          <div className="flex min-w-0 gap-2.5">
            <Target className="mt-0.5 h-4 w-4 shrink-0 text-sky-500" />
            <div className="min-w-0">
              <div className="text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">Goal</div>
              <div className="mt-1 line-clamp-2 text-xs font-medium leading-5 text-foreground">
                {summary.goal || 'Objective is not defined in soul/soul.md.'}
              </div>
            </div>
          </div>
          <div className="flex min-w-0 gap-2.5 lg:border-l lg:pl-4">
            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-emerald-500" />
            <div className="min-w-0">
              <div className="text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">Success</div>
              <div className="mt-1 line-clamp-2 text-xs font-medium leading-5 text-foreground">
                {summary.success || 'Success criteria are not defined in soul/soul.md.'}
              </div>
            </div>
          </div>
          <div className="flex min-w-0 gap-2.5 lg:border-l lg:pl-4">
            <ShieldAlert className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
            <div className="min-w-0">
              <div className="text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">Constraints</div>
              <div className="mt-1 line-clamp-2 text-xs font-medium leading-5 text-foreground">
                {summary.constraints || 'Constraints are not defined in soul/soul.md.'}
              </div>
            </div>
          </div>
        </div>
        <details className="group border-t">
          <summary className="flex cursor-pointer list-none items-center justify-between gap-3 px-4 py-2 text-[10px] font-medium text-muted-foreground hover:bg-muted/30 sm:px-5">
            <span>Full goal and success criteria</span>
            <span className="group-open:hidden">Show</span>
            <span className="hidden group-open:inline">Hide</span>
          </summary>
          <div className="border-t px-4 py-4 sm:px-5">
            <MarkdownRenderer content={content} disablePathLinking />
          </div>
        </details>
      </section>
    )
  }

  return (
    <div className={embedded ? 'px-4 py-4 sm:px-5' : 'h-full overflow-y-auto px-6 py-5'}>
      <div className={embedded ? 'max-w-none' : 'mx-auto max-w-3xl'}>
        <MarkdownRenderer content={content} disablePathLinking />
      </div>
    </div>
  )
}

export default SoulViewer
