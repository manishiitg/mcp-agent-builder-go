import React, { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Activity, AlertTriangle, CheckCircle2, CircleAlert, CircleHelp,
  LayoutDashboard, Loader2, RefreshCw, Send, X,
} from 'lucide-react'
import { agentApi } from '../../services/api'
import type { OrgDashboardNotification, ReportHumanInput } from '../../services/api-types'
import ModalPortal from '../ui/ModalPortal'

type SummaryStatus = OrgDashboardNotification['status']

interface WorkflowDashEntry {
  workspacePath: string
  label: string
	runSummary: OrgDashboardNotification | null
	pulseSummary: OrgDashboardNotification | null
	recent: OrgDashboardNotification[]
	pendingInputs: ReportHumanInput[]
  failed: boolean
}

interface OrgDashboardProps {
  workflows: Array<{ workspacePath: string; label: string }>
  onOpenWorkflow: (workspacePath: string) => void
}

const PILL_BASE = 'font-runloop-mono inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.06em]'
const STATUS_PILL: Record<SummaryStatus, { className: string; label: string; Icon: React.ComponentType<{ className?: string }> }> = {
  completed: { className: 'border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300', label: 'Completed', Icon: CheckCircle2 },
  failed: { className: 'border-red-500/25 bg-red-500/10 text-red-600 dark:text-red-300', label: 'Failed', Icon: CircleAlert },
  blocked: { className: 'border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-300', label: 'Blocked', Icon: AlertTriangle },
  waiting_for_user: { className: 'border-violet-500/25 bg-violet-500/10 text-violet-600 dark:text-violet-300', label: 'Waiting for you', Icon: CircleHelp },
  waiting_for_platform: { className: 'border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-300', label: 'Waiting for platform', Icon: CircleAlert },
  monitoring: { className: 'border-sky-500/25 bg-sky-500/10 text-sky-600 dark:text-sky-300', label: 'Monitoring', Icon: Activity },
  informational: { className: 'border-border bg-muted/70 text-muted-foreground', label: 'Update', Icon: CircleHelp },
  no_run: { className: 'border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-300', label: 'Did not run', Icon: AlertTriangle },
}

const Pill: React.FC<{ status: SummaryStatus; label?: string }> = ({ status, label }) => {
  const config = STATUS_PILL[status] || STATUS_PILL.informational
  return <span className={`${PILL_BASE} ${config.className}`}><config.Icon className="h-3 w-3" />{label || config.label}</span>
}

function relativeTime(iso: string): string {
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const mins = Math.floor((Date.now() - then) / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  return hours < 24 ? `${hours}h ago` : `${Math.floor(hours / 24)}d ago`
}

function absoluteTime(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleString(undefined, { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' })
}

function summaryNeedsAttention(summary: OrgDashboardNotification | null): boolean {
	return !!summary && ['failed', 'blocked', 'waiting_for_user', 'waiting_for_platform', 'no_run'].includes(summary.status)
}

function latestOpenSummary(entry: WorkflowDashEntry): { kind: 'run' | 'pulse'; summary: OrgDashboardNotification } | null {
	const candidates = (['run', 'pulse'] as const)
		.map(kind => ({ kind, summary: kind === 'run' ? entry.runSummary : entry.pulseSummary }))
		.filter((candidate): candidate is { kind: 'run' | 'pulse'; summary: OrgDashboardNotification } => summaryNeedsAttention(candidate.summary))
		.sort((a, b) => b.summary.created_at.localeCompare(a.summary.created_at))
	return candidates[0] || null
}

const SummaryDetail: React.FC<{ title: string; summary: OrgDashboardNotification | null }> = ({ title, summary }) => (
  <section className="rounded-lg border border-border bg-card/95 p-3">
    <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
      <div className="flex flex-wrap items-center gap-2"><h3 className="text-sm font-semibold text-foreground">{title}</h3>{summary && <Pill status={summary.status} />}{summary?.route && <span className={`${PILL_BASE} border-sky-500/25 bg-sky-500/10 text-sky-600 dark:text-sky-300`}>Route · {summary.route}</span>}</div>
      {summary && <span className="font-runloop-mono text-[10px] text-muted-foreground">{absoluteTime(summary.created_at)}</span>}
    </div>
    {summary ? <div className="space-y-4">
      {summary.title && <h4 className="text-sm font-semibold text-foreground">{summary.title}</h4>}
      <p className="text-sm leading-6 text-foreground/90">{summary.message}</p>
      {!!summary.fields?.length && <div className="grid gap-2 sm:grid-cols-2">{summary.fields.map((field, index) => <div key={`${field.label}:${index}`} className="rounded-md border border-border bg-background/70 px-3 py-2"><div className="font-runloop-mono text-[10px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">{field.label}</div><div className="mt-1 text-sm text-foreground">{field.value}</div></div>)}</div>}
      {!!summary.sections?.length && <div className="space-y-3">{summary.sections.map((section, index) => <div key={`${section.heading}:${index}`}><div className="font-runloop-mono text-[10px] font-semibold uppercase tracking-[0.08em] text-muted-foreground">{section.heading}</div><p className="mt-1 whitespace-pre-wrap text-sm leading-6 text-foreground/90">{section.body}</p></div>)}</div>}
    </div> : <p className="text-sm text-muted-foreground">No {title.toLowerCase()} notification has been recorded yet.</p>}
  </section>
)

const NotificationDetailModal: React.FC<{
  entry: WorkflowDashEntry
	summary: OrgDashboardNotification
	onSelectSummary: (summary: OrgDashboardNotification) => void
  onClose: () => void
}> = ({ entry, summary, onSelectSummary, onClose }) => {
	const title = summary.kind === 'pulse_summary' ? 'Pulse update' : 'Workflow run'
  return (
  <ModalPortal><div className="fixed inset-0 z-[10000] flex items-center justify-center bg-black/55 p-4" onClick={event => { if (event.target === event.currentTarget) onClose() }}>
    <div role="dialog" aria-modal="true" className="flex max-h-[85vh] w-full max-w-2xl flex-col overflow-hidden rounded-lg border border-border bg-background shadow-2xl">
      <div className="flex items-start justify-between gap-3 border-b border-border bg-card/95 px-4 py-3"><div className="min-w-0"><div className="font-runloop-mono text-[10px] font-semibold uppercase tracking-[0.1em] text-muted-foreground">{title}</div><h2 className="mt-1 truncate text-lg font-semibold text-foreground">{entry.label}</h2><p className="mt-1 truncate text-xs text-muted-foreground">{entry.workspacePath}</p></div><button type="button" onClick={onClose} aria-label="Close details" className="inline-flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-muted"><X className="h-4 w-4" /></button></div>
	  <div className="min-h-0 flex-1 space-y-4 overflow-auto p-4"><SummaryDetail title={title} summary={summary} />
		{entry.recent.length > 1 && <section className="rounded-lg border border-border bg-card/95 p-3"><div className="mb-2 flex items-center justify-between gap-2"><h3 className="text-sm font-semibold text-foreground">Recent history</h3><span className="font-runloop-mono text-[10px] text-muted-foreground">{entry.recent.length} retained</span></div><div className="divide-y divide-border">{entry.recent.map(item => <button key={item.id} type="button" onClick={() => onSelectSummary(item)} className="flex w-full items-center gap-2 px-1 py-2.5 text-left hover:bg-muted/50"><Pill status={item.status} label={item.kind === 'pulse_summary' ? 'Pulse' : 'Run'} /><span className="min-w-0 flex-1 truncate text-xs font-medium text-foreground/90">{item.title || item.message}</span><span className="shrink-0 font-runloop-mono text-[10px] text-muted-foreground">{relativeTime(item.created_at)}</span></button>)}</div></section>}
	  </div>
    </div>
  </div></ModalPortal>
  )
}

export const OrgDashboard: React.FC<OrgDashboardProps> = ({ workflows, onOpenWorkflow }) => {
  const [entries, setEntries] = useState<WorkflowDashEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
	const [selectedNotification, setSelectedNotification] = useState<{ entry: WorkflowDashEntry; summary: OrgDashboardNotification } | null>(null)

  const load = useCallback(async () => {
    setLoading(true); setError(null)
    try {
      const paths = workflows.map(workflow => workflow.workspacePath)
      const [notifications, humanInputs] = await Promise.all([
        paths.length ? agentApi.getOrgDashboardNotifications(paths, 10) : Promise.resolve({ success: true, workflows: [] }),
        paths.length ? agentApi.listReportHumanInputsAggregate(paths, 'pending').catch(() => ({ success: false, inputs: [] as ReportHumanInput[] })) : Promise.resolve({ success: true, inputs: [] as ReportHumanInput[] }),
      ])
      const notificationByPath = new Map((notifications.workflows || []).map(item => [item.workspace_path, item]))
      const results = workflows.map((workflow): WorkflowDashEntry => {
        const notification = notificationByPath.get(workflow.workspacePath)
        return {
          workspacePath: workflow.workspacePath,
		  label: workflow.label,
		  runSummary: notification?.run_summary || null,
		  pulseSummary: notification?.pulse_summary || null,
		  recent: notification?.recent || [],
          pendingInputs: humanInputs.success ? humanInputs.inputs.filter(input => input.workspace_path === workflow.workspacePath) : [],
          failed: !notifications.success || !humanInputs.success || !!notification?.error,
        }
      })
      setEntries(results)
      if (results.some(result => result.failed)) setError('Some workflow notification data could not be loaded.')
    } catch { setError('Could not load the Org Dashboard.') }
    finally { setLoading(false) }
  }, [workflows])

  useEffect(() => { void load() }, [load])
  useEffect(() => {
    if (!selectedNotification) return
    const close = (event: KeyboardEvent) => { if (event.key === 'Escape') setSelectedNotification(null) }
    window.addEventListener('keydown', close); return () => window.removeEventListener('keydown', close)
  }, [selectedNotification])

  const triage = useMemo(() => ({
    attention: entries.filter(entry => summaryNeedsAttention(entry.runSummary) || summaryNeedsAttention(entry.pulseSummary) || entry.pendingInputs.length > 0).length,
    critical: entries.filter(entry => [entry.runSummary, entry.pulseSummary].some(summary => summary?.status === 'failed')).length,
    healthy: entries.filter(entry => !summaryNeedsAttention(entry.runSummary) && !summaryNeedsAttention(entry.pulseSummary) && [entry.runSummary, entry.pulseSummary].some(summary => summary?.status === 'completed')).length,
    awaiting: entries.filter(entry => !entry.runSummary && !entry.pulseSummary).length,
    decisions: entries.reduce((count, entry) => count + entry.pendingInputs.length, 0),
  }), [entries])

  const decisionGroups = useMemo(() => entries
    .filter(entry => entry.pendingInputs.length > 0)
    .map(entry => ({
      ...entry,
      pendingInputs: [...entry.pendingInputs].sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime()),
    }))
    .sort((a, b) => new Date(b.pendingInputs[0].updated_at).getTime() - new Date(a.pendingInputs[0].updated_at).getTime()), [entries])
  const pulseUpdates = useMemo(() => entries
	.filter((entry): entry is WorkflowDashEntry & { pulseSummary: OrgDashboardNotification } => !!entry.pulseSummary && !summaryNeedsAttention(entry.pulseSummary))
	.sort((a, b) => b.pulseSummary.created_at.localeCompare(a.pulseSummary.created_at)), [entries])
  const attentionItems = useMemo(() => entries
    .map(entry => ({ entry, active: latestOpenSummary(entry) }))
    .filter((item): item is { entry: WorkflowDashEntry; active: { kind: 'run' | 'pulse'; summary: OrgDashboardNotification } } => !!item.active)
    .sort((a, b) => b.active.summary.created_at.localeCompare(a.active.summary.created_at)), [entries])
  const activityHistory = useMemo(() => entries
    .flatMap(entry => entry.recent.map(summary => ({ entry, summary })))
    .sort((a, b) => b.summary.created_at.localeCompare(a.summary.created_at)), [entries])
  const groups = useMemo(() => {
	const grouped = new Map<string, WorkflowDashEntry[]>([['Healthy / completed', []], ['Latest activity', []], ['Awaiting first run', []]])
	entries.forEach(entry => {
	  // Open work has a dedicated section above. Keep the all-workflows view a
	  // non-duplicated activity inventory rather than a second alert list.
	  if (latestOpenSummary(entry)) return
	  const group = entry.runSummary?.status === 'completed'
		  ? 'Healthy / completed'
          : entry.runSummary
            ? 'Latest activity'
            : 'Awaiting first run'
      grouped.get(group)?.push(entry)
    })
    return Array.from(grouped.entries()).filter(([, items]) => items.length)
  }, [entries])

  const header = <div className="flex items-center justify-between gap-3 px-6 pt-6"><div className="flex items-center gap-3"><LayoutDashboard className="h-6 w-6 text-primary" /><div><h2 className="text-xl font-semibold tracking-tight text-foreground">Activity</h2><p className="mt-0.5 text-sm text-muted-foreground">Workflow runs, Pulse work, and decisions</p></div></div><button type="button" onClick={() => void load()} disabled={loading} className="inline-flex items-center gap-1.5 rounded-lg border border-border bg-background/90 px-3 py-2 text-sm font-medium text-muted-foreground shadow-sm hover:bg-muted disabled:opacity-50"><RefreshCw className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />Refresh</button></div>

  if (loading && !entries.length) return <div className="flex h-full min-h-[320px] flex-col items-center justify-center gap-3 text-muted-foreground"><Loader2 className="h-6 w-6 animate-spin" /><p className="text-sm">Loading Org Dashboard…</p></div>
  if (!workflows.length) return <div className="flex h-full min-h-[320px] flex-col">{header}<div className="flex flex-1 items-center justify-center text-sm text-muted-foreground">No automations yet</div></div>

  return <div className="flex h-full flex-col">{header}
    {error && <div className="mx-6 mt-4 rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-600">{error}</div>}
    <div className="mx-6 mt-5 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card/95 px-4 py-3 shadow-sm"><span className="font-runloop-mono rounded-md border border-border bg-background/70 px-2 py-1 text-[11px] font-semibold uppercase tracking-[0.06em] text-foreground">Attention {triage.attention}</span><Pill status="failed" label={`Failed ${triage.critical}`} /><Pill status="completed" label={`Completed ${triage.healthy}`} /><Pill status="informational" label={`Awaiting ${triage.awaiting}`} />{triage.decisions > 0 && <span className={`${PILL_BASE} border-violet-500/25 bg-violet-500/10 text-violet-600`}><CircleHelp className="h-3 w-3" />Decisions {triage.decisions}</span>}</div>
    <div className="flex-1 space-y-8 overflow-auto px-6 py-6">
      {!!decisionGroups.length && <section className="space-y-2">
        <div className="flex items-center gap-2"><CircleHelp className="h-4 w-4 text-violet-500" /><h3 className="text-sm font-semibold text-foreground">Decisions required</h3><span className="font-runloop-mono text-[11px] text-muted-foreground">{triage.decisions} across {decisionGroups.length} workflow{decisionGroups.length !== 1 ? 's' : ''}</span></div>
        <div className="grid gap-3 lg:grid-cols-2">{decisionGroups.map(entry => <article key={entry.workspacePath} className="overflow-hidden rounded-lg border border-violet-500/25 bg-card/95 shadow-sm">
          <button type="button" onClick={() => onOpenWorkflow(entry.workspacePath)} className="flex w-full items-center justify-between gap-3 border-b border-violet-500/15 px-3 py-2.5 text-left hover:bg-violet-500/[0.06]"><span className="truncate text-sm font-semibold text-foreground">{entry.label}</span><span className={`${PILL_BASE} border-violet-500/25 bg-violet-500/10 text-violet-600`}>{entry.pendingInputs.length} open</span></button>
          <div className="divide-y divide-border">{entry.pendingInputs.map(input => <button key={input.id} type="button" onClick={() => onOpenWorkflow(entry.workspacePath)} className="flex w-full items-start gap-3 px-3 py-3 text-left hover:bg-violet-500/[0.06]"><span className={`${PILL_BASE} mt-0.5 border-violet-500/25 bg-violet-500/10 text-violet-600`}>{input.priority}</span><span className="line-clamp-2 min-w-0 flex-1 text-sm text-foreground/90">{input.question}</span></button>)}</div>
        </article>)}</div>
      </section>}

      {!!attentionItems.length && <section className="space-y-2">
        <div className="flex items-center gap-2"><AlertTriangle className="h-4 w-4 text-amber-500" /><h3 className="text-sm font-semibold text-foreground">Needs attention</h3><span className="font-runloop-mono text-[11px] text-muted-foreground">{attentionItems.length} workflow{attentionItems.length !== 1 ? 's' : ''}</span></div>
		<div className="grid gap-3 lg:grid-cols-2">{attentionItems.map(({ entry, active }) => <button key={entry.workspacePath} type="button" onClick={() => setSelectedNotification({ entry, summary: active.summary })} className="rounded-lg border border-amber-500/25 bg-card/95 p-3 text-left shadow-sm hover:border-amber-500/50"><div className="flex items-start justify-between gap-3"><div className="min-w-0"><h4 className="truncate text-sm font-semibold text-foreground">{entry.label}</h4><div className="mt-2 flex flex-wrap gap-1.5"><Pill status={active.summary.status} label={active.kind === 'run' ? 'Run' : 'Pulse'} /></div></div><span className="font-runloop-mono shrink-0 text-[10px] text-muted-foreground">{relativeTime(active.summary.created_at)}</span></div><p className="mt-3 line-clamp-1 text-sm font-semibold text-foreground/90">{active.summary.title || 'Action needed'}</p><p className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">{active.summary.message}</p></button>)}</div>
      </section>}

      <section className="space-y-2">
        <div className="flex items-center gap-2"><Activity className="h-4 w-4 text-primary" /><h3 className="text-sm font-semibold text-foreground">Pulse updates</h3><span className="font-runloop-mono text-[11px] text-muted-foreground">{pulseUpdates.length} workflow{pulseUpdates.length !== 1 ? 's' : ''}</span></div>
		{pulseUpdates.length ? <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">{pulseUpdates.map(entry => <button key={entry.workspacePath} type="button" onClick={() => setSelectedNotification({ entry, summary: entry.pulseSummary })} className="rounded-lg border border-primary/20 bg-card/95 p-3 text-left shadow-sm hover:border-primary/50"><div className="flex items-start justify-between gap-2"><h4 className="truncate text-sm font-semibold text-foreground">{entry.label}</h4><span className="font-runloop-mono shrink-0 text-[10px] text-muted-foreground">{relativeTime(entry.pulseSummary.created_at)}</span></div><div className="mt-2 flex flex-wrap gap-1.5"><Pill status={entry.pulseSummary.status} />{entry.pulseSummary.route && <span className={`${PILL_BASE} border-sky-500/25 bg-sky-500/10 text-sky-600 dark:text-sky-300`}>Route · {entry.pulseSummary.route}</span>}</div><div className="mt-2 text-xs leading-5 text-muted-foreground"><p className="line-clamp-1 font-medium text-foreground/90">{entry.pulseSummary.title || 'Pulse update'}</p>{entry.pulseSummary.fields?.length ? <div className="mt-2 flex flex-wrap gap-1.5">{entry.pulseSummary.fields.slice(0, 3).map((field, index) => <span key={`${field.label}:${index}`} className="rounded border border-border bg-background/70 px-1.5 py-1 font-runloop-mono text-[10px] text-foreground/80">{field.label}: {field.value}</span>)}</div> : <p className="line-clamp-2">{entry.pulseSummary.message}</p>}</div></button>)}</div> : <div className="rounded-lg border border-dashed border-border px-3 py-5 text-center text-xs text-muted-foreground">No completed Pulse updates have been posted yet.</div>}
      </section>

      <section className="space-y-4">
        <div className="flex items-center gap-2"><LayoutDashboard className="h-4 w-4 text-muted-foreground" /><h3 className="text-sm font-semibold text-foreground">All workflows</h3><span className="font-runloop-mono text-[11px] text-muted-foreground">{entries.length} total</span></div>
        {groups.map(([group, items]) => <div key={group} className="space-y-2"><div className="flex items-center gap-2">{group === 'Needs attention' ? <AlertTriangle className="h-4 w-4 text-amber-500" /> : group === 'Awaiting first run' ? <CircleHelp className="h-4 w-4 text-muted-foreground" /> : <Activity className={`h-4 w-4 ${group === 'Latest activity' ? 'text-primary' : 'text-emerald-500'}`} />}<h4 className="text-xs font-semibold text-foreground">{group}</h4><span className="font-runloop-mono text-[10px] text-muted-foreground">{items.length}</span></div><div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">{items.map(entry => {
          const summary = entry.runSummary
		  return <button key={entry.workspacePath} type="button" disabled={!summary} onClick={() => summary && setSelectedNotification({ entry, summary })} className="rounded-lg border border-border bg-card/95 p-3 text-left shadow-sm enabled:hover:border-primary/40 disabled:cursor-default"><div className="flex justify-between gap-2"><h5 className="truncate text-sm font-semibold text-foreground">{entry.label}</h5>{summary && <span className="font-runloop-mono shrink-0 text-[10px] text-muted-foreground">{relativeTime(summary.created_at)}</span>}</div>{summary ? <><div className="mt-2"><Pill status={summary.status} label="Run" /></div><div className="mt-2 text-xs leading-5 text-muted-foreground"><p className="line-clamp-1 font-medium text-foreground/90">{summary.title || 'Workflow run'}</p><p className="line-clamp-2">{summary.message}</p></div></> : <p className="mt-2 flex items-center gap-1.5 text-xs italic text-muted-foreground"><Send className="h-3.5 w-3.5" />Waiting for the first run notification.</p>}</button>
        })}</div></div>)}
      </section>

      <section className="space-y-3 border-t border-border pt-6">
        <div className="flex items-center gap-2"><Activity className="h-4 w-4 text-primary" /><h3 className="text-base font-semibold text-foreground">Activity history</h3><span className="font-runloop-mono text-[11px] text-muted-foreground">{activityHistory.length} retained updates</span></div>
        {activityHistory.length ? <div className="grid gap-3 lg:grid-cols-2">{activityHistory.map(({ entry, summary }) => <button key={summary.id} type="button" onClick={() => setSelectedNotification({ entry, summary })} className="rounded-lg border border-border bg-card/95 p-4 text-left shadow-sm hover:border-primary/40"><div className="flex items-start justify-between gap-3"><div className="min-w-0"><div className="flex flex-wrap items-center gap-2"><h4 className="truncate text-sm font-semibold text-foreground">{entry.label}</h4><Pill status={summary.status} label={summary.kind === 'pulse_summary' ? 'Pulse' : 'Run'} /></div><p className="mt-2 line-clamp-1 text-sm font-medium text-foreground/90">{summary.title || summary.message}</p><p className="mt-1 line-clamp-2 text-sm leading-5 text-muted-foreground">{summary.message}</p></div><span className="font-runloop-mono shrink-0 text-[10px] text-muted-foreground">{relativeTime(summary.created_at)}</span></div></button>)}</div> : <div className="rounded-lg border border-dashed border-border px-3 py-5 text-center text-sm text-muted-foreground">No run or Pulse history has been recorded yet.</div>}
      </section>
	</div>{selectedNotification && <NotificationDetailModal entry={selectedNotification.entry} summary={selectedNotification.summary} onSelectSummary={summary => setSelectedNotification({ entry: selectedNotification.entry, summary })} onClose={() => setSelectedNotification(null)} />}
  </div>
}

export default OrgDashboard
