import { useCallback, useEffect, useMemo, useState } from 'react'
import { CheckCircle2, Loader2, MinusCircle, RefreshCw, XCircle } from 'lucide-react'
import { agentApi } from '../../services/api'
import type { EvalResultRecord } from '../../services/api-types'
import { WORKFLOW_LOG_REFRESH_EVENT } from './workflowEvents'

/** A step "passes" only once it has a captured score at or above half its max. */
function stepPassed(step: EvalResultRecord): boolean {
  if (step.skipped || !step.score_captured) return false
  return step.max_score <= 0 || step.score >= step.max_score / 2
}

const MAX_TREND_RUNS = 8

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

  const { latestRunFolder, steps, trendsByStepID } = useMemo(() => {
    // Results arrive newest-first; the first row seen for any step_id is that
    // step's most recent run, which is also this workflow's latest eval run.
    const latestByStep = new Map<string, EvalResultRecord>()
    const runsByStep = new Map<string, EvalResultRecord[]>()
    for (const result of results) {
      if (!latestByStep.has(result.step_id)) latestByStep.set(result.step_id, result)
      const runs = runsByStep.get(result.step_id) || []
      runs.push(result)
      runsByStep.set(result.step_id, runs)
    }
    const latest = [...latestByStep.values()]
    const runFolder = latest[0]?.run_folder
    const trends = new Map<string, EvalResultRecord[]>()
    for (const [stepID, runs] of runsByStep) {
      // Oldest-first, capped, so the trend reads left-to-right as a timeline.
      trends.set(stepID, runs.slice(0, MAX_TREND_RUNS).reverse())
    }
    return { latestRunFolder: runFolder, steps: latest, trendsByStepID: trends }
  }, [results])

  return (
    <section className={`min-w-0 overflow-hidden rounded-xl border bg-background ${className}`} aria-label="Evaluation results">
      <div className="flex items-center justify-between border-b px-4 py-3">
        <div>
          <h3 className="text-sm font-semibold text-foreground">Evaluation</h3>
          <p className="mt-0.5 text-[11px] text-muted-foreground">
            {latestRunFolder ? `Latest run · ${latestRunFolder}` : 'Each criterion’s latest score and its trend across recent runs'}
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
      ) : steps.length === 0 ? (
        <div className="flex min-h-32 items-center justify-center text-xs text-muted-foreground">No evaluation results yet.</div>
      ) : (
        <div className="divide-y">
          {steps.map((step) => {
            const Icon = step.skipped ? MinusCircle : stepPassed(step) ? CheckCircle2 : XCircle
            const iconTone = step.skipped
              ? 'text-muted-foreground'
              : stepPassed(step) ? 'text-emerald-500' : 'text-red-500'
            const trend = trendsByStepID.get(step.step_id) || []
            return (
              <div key={step.step_id} className="flex items-center gap-3 px-4 py-2.5">
                <Icon className={`h-4 w-4 shrink-0 ${iconTone}`} />
                <div className="min-w-0 flex-1 truncate text-xs font-medium text-foreground">
                  {step.title || step.step_id}
                </div>
                <div className="shrink-0 text-xs text-muted-foreground">
                  {step.skipped
                    ? 'skipped'
                    : step.score_captured
                      ? `${step.score} / ${step.max_score}`
                      : 'no score'}
                </div>
                {trend.length > 1 && (
                  <div className="flex shrink-0 items-center gap-0.5" title="Score across recent runs, oldest to newest">
                    {trend.map((run) => (
                      <span
                        key={run.run_folder}
                        className={`h-1.5 w-1.5 rounded-full ${
                          run.skipped
                            ? 'bg-muted-foreground/40'
                            : run.score_captured && stepPassed(run) ? 'bg-emerald-500' : 'bg-red-500'
                        }`}
                      />
                    ))}
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </section>
  )
}
