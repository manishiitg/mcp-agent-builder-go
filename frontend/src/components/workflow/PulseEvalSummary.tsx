import { useCallback, useEffect, useMemo, useState } from 'react'
import { CheckCircle2, Loader2, MinusCircle, RefreshCw, XCircle } from 'lucide-react'
import { agentApi } from '../../services/api'
import type { EvalResultRecord } from '../../services/api-types'
import { WORKFLOW_LOG_REFRESH_EVENT } from './workflowEvents'

function recordedLabel(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

/** A step "passes" only once it has a captured score at or above half its max. */
function stepPassed(step: EvalResultRecord): boolean {
  if (step.skipped || !step.score_captured) return false
  return step.max_score <= 0 || step.score >= step.max_score / 2
}

export function PulseEvalSummary({
  workspacePath,
  className = '',
}: {
  workspacePath: string
  className?: string
}) {
  const [results, setResults] = useState<EvalResultRecord[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!workspacePath) return
    setLoading(true)
    setError(null)
    try {
      const response = await agentApi.getPulseEvalResults(workspacePath)
      if (!response.success) throw new Error(response.error || 'Failed to load evaluation results.')
      setResults(response.results || [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load evaluation results.')
    } finally {
      setLoading(false)
    }
  }, [workspacePath])

  useEffect(() => { void load() }, [load])
  useEffect(() => {
    const refresh = () => { void load() }
    window.addEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
    return () => window.removeEventListener(WORKFLOW_LOG_REFRESH_EVENT, refresh)
  }, [load])

  const runs = useMemo(() => {
    const byRun = new Map<string, EvalResultRecord[]>()
    for (const result of results) {
      const steps = byRun.get(result.run_folder) || []
      steps.push(result)
      byRun.set(result.run_folder, steps)
    }
    // Each run's own most recent generated_at orders the run list; results
    // arrive newest-first from the backend, so the first row seen per run is
    // already that run's latest timestamp.
    return Array.from(byRun.entries())
      .map(([runFolder, steps]) => ({ runFolder, steps, generatedAt: steps[0]?.generated_at || '' }))
      .sort((a, b) => b.generatedAt.localeCompare(a.generatedAt))
  }, [results])

  const latestRun = runs[0]
  const previousRuns = runs.slice(1, 6)

  return (
    <section className={`min-w-0 overflow-hidden rounded-xl border bg-background ${className}`} aria-label="Evaluation results">
      <div className="flex items-center justify-between border-b px-4 py-3">
        <div>
          <h3 className="text-sm font-semibold text-foreground">Evaluation</h3>
          <p className="mt-0.5 text-[11px] text-muted-foreground">
            What each eval step checked and its latest score, one criterion at a time
          </p>
        </div>
        <button
          type="button"
          onClick={() => { void load() }}
          disabled={loading}
          className="rounded p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-50"
          aria-label="Refresh evaluation results"
        >
          <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>

      {error ? (
        <div className="m-4 rounded border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive">{error}</div>
      ) : loading && results.length === 0 ? (
        <div className="flex min-h-32 items-center justify-center gap-2 text-xs text-muted-foreground">
          <Loader2 className="h-3.5 w-3.5 animate-spin" /> Loading evaluation results…
        </div>
      ) : !latestRun ? (
        <div className="flex min-h-32 items-center justify-center text-xs text-muted-foreground">No evaluation results yet.</div>
      ) : (
        <>
          <div className="border-b bg-muted/20 px-4 py-2 text-[11px] text-muted-foreground">
            Latest run · {latestRun.runFolder} · {recordedLabel(latestRun.generatedAt)}
          </div>
          <div className="divide-y">
            {latestRun.steps.map((step) => {
              const Icon = step.skipped ? MinusCircle : stepPassed(step) ? CheckCircle2 : XCircle
              const iconTone = step.skipped
                ? 'text-muted-foreground'
                : stepPassed(step) ? 'text-emerald-500' : 'text-red-500'
              return (
                <div key={step.step_id} className="px-4 py-3">
                  <div className="flex items-start gap-2">
                    <Icon className={`mt-0.5 h-4 w-4 shrink-0 ${iconTone}`} />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
                        <span className="font-medium text-foreground">{step.title || step.step_id}</span>
                        {step.skipped ? (
                          <span className="text-muted-foreground">skipped this run</span>
                        ) : step.score_captured ? (
                          <span className="text-muted-foreground">{step.score} / {step.max_score}</span>
                        ) : (
                          <span className="text-muted-foreground">no score captured</span>
                        )}
                      </div>
                      {step.reasoning && (
                        <p className="mt-1 text-xs leading-5 text-muted-foreground">{step.reasoning}</p>
                      )}
                      {step.evidence && (
                        <p className="mt-1 text-[11px] leading-5 text-muted-foreground/80">Evidence: {step.evidence}</p>
                      )}
                    </div>
                  </div>
                </div>
              )
            })}
          </div>

          {previousRuns.length > 0 && (
            <div className="border-t px-4 py-2.5">
              <div className="mb-1.5 text-[11px] font-medium text-muted-foreground">Previous runs</div>
              <div className="space-y-1">
                {previousRuns.map((run) => {
                  const scored = run.steps.filter((step) => !step.skipped && step.score_captured)
                  const passed = scored.filter(stepPassed).length
                  return (
                    <div key={run.runFolder} className="flex items-center justify-between text-[11px] text-muted-foreground">
                      <span className="truncate">{run.runFolder}</span>
                      <span className="shrink-0 pl-2">{recordedLabel(run.generatedAt)} · {passed}/{scored.length} passed</span>
                    </div>
                  )
                })}
              </div>
            </div>
          )}
        </>
      )}
    </section>
  )
}
