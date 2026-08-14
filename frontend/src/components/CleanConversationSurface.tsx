import { useEffect, useMemo, useRef, type ReactNode } from 'react'
import { AlertCircle, Bell, Check, Loader2, Sparkles, Wrench } from 'lucide-react'
import type { PollingEvent } from '../services/api-types'
import { buildCleanConversationItems, buildProductionActivityItems } from '../utils/cleanConversation'
import { ConversationMarkdownRenderer } from './ui/MarkdownRenderer'

export interface CleanConversationSurfaceProps {
  events: PollingEvent[]
  isStreaming: boolean
  isRestoring: boolean
  streamingText: string
  landingContent?: ReactNode
}

function messageTime(timestamp?: string): string {
  if (!timestamp) return ''
  const parsed = new Date(timestamp)
  if (!Number.isFinite(parsed.getTime())) return ''
  return parsed.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function formatTokenCount(tokens: number): string {
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(2).replace(/\.00$/, '')}M`
  if (tokens >= 1_000) return `${(tokens / 1_000).toFixed(1).replace(/\.0$/, '')}k`
  return String(tokens)
}

// These notices are overwhelmingly step completions, so they are labelled as
// such — but a failed background step is delivered through the same channel,
// and labelling that "Step complete" would state the opposite of what happened.
// Keyed off the two markers the backend writes for a failure: `status=failed`
// in the header line, and the "Error: " prefix it puts on the result body
// (buildAutoNotificationMessage, agent_go/cmd/server/background_agents.go).
function isFailedStepNotification(content: string): boolean {
  return /\bstatus=failed\b/i.test(content) || /(^|\n)\s*(- )?Result: Error:/i.test(content)
}

export function CleanConversationSurface({
  events,
  isStreaming,
  isRestoring,
  streamingText,
  landingContent,
}: CleanConversationSurfaceProps) {
  const scrollRef = useRef<HTMLDivElement>(null)
  const items = useMemo(() => buildCleanConversationItems(events), [events])
  const productionActivity = useMemo(() => buildProductionActivityItems(events), [events])

  useEffect(() => {
    const element = scrollRef.current
    if (!element) return
    const closeToBottom = element.scrollHeight - element.scrollTop - element.clientHeight < 240
    if (closeToBottom || isStreaming) element.scrollTop = element.scrollHeight
  }, [items.length, isStreaming, streamingText])

  if (isRestoring) {
    return (
      <div className="grid h-full place-items-center text-sm text-slate-500" data-testid="clean-conversation-restoring">
        <span className="flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin text-violet-500" /> Restoring conversation…</span>
      </div>
    )
  }

  if (items.length === 0 && !isStreaming) return <>{landingContent}</>

  return (
    <div ref={scrollRef} className="h-full overflow-y-auto overscroll-contain px-4 py-6 sm:px-6" aria-live="polite" data-testid="clean-conversation-surface">
      <div className="flex w-full flex-col gap-6">
        {items.map((item) => item.role === 'user' ? (
          <article key={item.id} className="ml-auto max-w-full rounded-2xl rounded-br-md bg-violet-600 px-4 py-3 text-sm leading-6 text-white shadow-sm" data-testid="clean-user-message">
            <p className="whitespace-pre-wrap break-words">{item.content}</p>
          </article>
        ) : item.role === 'reasoning' ? (
          <details key={item.id} className="w-full rounded-2xl border border-violet-900/70 bg-slate-900/80 px-4 py-3 text-slate-200 shadow-sm" open={isStreaming} data-testid="clean-reasoning-message">
            <summary className="cursor-pointer select-none text-xs font-semibold text-violet-300">Thinking</summary>
            <p className="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-slate-300">{item.content}</p>
          </details>
        ) : item.role === 'notification' ? (
          // An automatic update the runtime delivered to the agent (a background
          // step finishing), not something the user typed or the agent said. It
          // is shown rather than hidden so a reply that arrives with no prompt
          // has a visible cause. Full width with its own label, because the
          // payload is a multi-line status report — the centered pill this used
          // to be was built for a one-line notice and wrapped badly.
          // Collapsed by default: the body is orchestration bookkeeping written
          // for the agent (raw `iter=`/`step=`/`item=` context plus the step's
          // STATUS contract line), not prose for a video creator. The header
          // alone answers the question this card exists to answer — why the
          // agent just spoke — and the detail stays one click away.
          <details key={item.id} className="w-full rounded-xl border border-cyan-200/70 bg-cyan-50/60 px-4 py-3 dark:border-cyan-900/60 dark:bg-cyan-950/20" data-testid="clean-notification-message">
            <summary className="flex cursor-pointer select-none items-center gap-1.5 text-[10px] font-semibold uppercase tracking-wide text-cyan-700 dark:text-cyan-300">
              <Bell className="h-3 w-3 shrink-0" />
              {isFailedStepNotification(item.content) ? 'Step failed' : 'Step complete'}
            </summary>
            {/* The payload is authored as markdown (bold step names, `code`
                paths, bullet lists when several steps land together), so render
                it rather than printing the syntax literally. */}
            <div className="mt-2 text-xs leading-5 text-slate-600 dark:text-slate-300">
              <ConversationMarkdownRenderer content={item.content} maxHeight="none" framed={false} />
            </div>
          </details>
        ) : (
          <article key={item.id} className="flex w-full items-start gap-3" data-testid={item.role === 'error' ? 'clean-error-message' : 'clean-assistant-message'}>
            <span className={`mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-xl ${item.role === 'error' ? 'bg-red-50 text-red-600 dark:bg-red-950/50 dark:text-red-300' : 'bg-violet-100 text-violet-700 dark:bg-violet-950 dark:text-violet-300'}`}>
              {item.role === 'error' ? <AlertCircle className="h-4 w-4" /> : <Sparkles className="h-4 w-4" />}
            </span>
            <div className={`min-w-0 flex-1 rounded-2xl rounded-tl-md border px-4 py-3 shadow-sm ${item.role === 'error' ? 'border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-200' : 'border-slate-200 bg-white text-slate-700 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-200'}`}>
              {item.role === 'assistant'
                ? <ConversationMarkdownRenderer content={item.content} maxHeight="none" framed={false} />
                : <p className="whitespace-pre-wrap text-sm leading-6">{item.content}</p>}
              {(item.role === 'assistant' && item.usage) || messageTime(item.timestamp) ? (
                <div className={`mt-3 flex flex-wrap items-center justify-between gap-x-3 gap-y-1 pt-2 text-[10px] font-medium text-slate-400 ${item.role === 'assistant' && item.usage ? 'border-t border-slate-100 dark:border-slate-800' : ''}`}>
                  {item.role === 'assistant' && item.usage ? (
                    <p className="flex flex-wrap gap-x-3 gap-y-1" data-testid="clean-assistant-usage">
                      <span>Input {formatTokenCount(item.usage.inputTokens)}</span>
                      <span>Output {formatTokenCount(item.usage.outputTokens)}</span>
                      {item.usage.cacheTokens > 0 ? <span>Cache {formatTokenCount(item.usage.cacheTokens)}</span> : null}
                      {item.usage.isEstimated ? <span>Estimated</span> : null}
                    </p>
                  ) : <span />}
                  {messageTime(item.timestamp) ? <time>{messageTime(item.timestamp)}</time> : null}
                </div>
              ) : null}
            </div>
          </article>
        ))}

        {productionActivity.length > 0 ? (
          <section className="w-full rounded-2xl border border-slate-800 bg-slate-900/80 p-3" aria-label="Production activity" data-testid="clean-production-activity">
            <details open={isStreaming}>
              <summary className="flex cursor-pointer list-none items-center justify-between gap-3 select-none [&::-webkit-details-marker]:hidden">
              <span className="flex items-center gap-2 text-[11px] font-semibold text-slate-200"><Wrench className="h-3.5 w-3.5 text-violet-400" /> Production activity</span>
              <span className="text-[10px] text-slate-400">{productionActivity.length} {productionActivity.length === 1 ? 'action' : 'actions'}</span>
              </summary>
              <div className="mt-2 space-y-1.5">
              {productionActivity.map((activityItem) => (
                <div key={activityItem.id} className="flex items-start gap-2.5 rounded-xl bg-slate-950/70 px-2.5 py-2 shadow-sm" data-status={activityItem.status}>
                  <span className={`mt-0.5 grid h-5 w-5 shrink-0 place-items-center rounded-full ${activityItem.status === 'complete' ? 'bg-emerald-100 text-emerald-600 dark:bg-emerald-950 dark:text-emerald-300' : activityItem.status === 'error' ? 'bg-red-100 text-red-600 dark:bg-red-950 dark:text-red-300' : 'bg-violet-100 text-violet-600 dark:bg-violet-950 dark:text-violet-300'}`}>
                    {activityItem.status === 'complete' ? <Check className="h-3 w-3" /> : activityItem.status === 'error' ? <AlertCircle className="h-3 w-3" /> : <Loader2 className="h-3 w-3 animate-spin" />}
                  </span>
                  <div className="min-w-0 flex-1">
                    <strong className="block truncate text-[11px] font-semibold text-slate-200">{activityItem.title}</strong>
                    <small className="mt-0.5 block text-[10px] leading-4 text-slate-400">{activityItem.detail}</small>
                    {activityItem.arguments || activityItem.result ? (
                      <details className="mt-2 rounded-lg border border-slate-800 bg-black/20 px-2 py-1.5 text-[10px] text-slate-300">
                        <summary className="cursor-pointer select-none font-medium text-violet-300">Developer details</summary>
                        {activityItem.arguments ? <><p className="mt-2 font-semibold text-slate-400">Arguments</p><pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono leading-4 text-slate-200">{activityItem.arguments}</pre></> : null}
                        {activityItem.result ? <><p className="mt-2 font-semibold text-slate-400">Result</p><pre className="mt-1 max-h-48 overflow-auto whitespace-pre-wrap break-words font-mono leading-4 text-slate-200">{activityItem.result}</pre></> : null}
                      </details>
                    ) : null}
                  </div>
                </div>
              ))}
              </div>
            </details>
          </section>
        ) : null}

        {isStreaming && !streamingText.trim() ? (
          <div className="flex items-center gap-2 text-xs text-slate-400" role="status" aria-live="polite" data-testid="clean-working-indicator">
            <Loader2 className="h-3.5 w-3.5 animate-spin text-violet-400" />
            <span>Working…</span>
          </div>
        ) : null}

        {isStreaming && streamingText.trim() ? (
          <article className="flex w-full items-start gap-3" data-testid="clean-working-message">
            <span className="mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-xl bg-violet-100 text-violet-700 dark:bg-violet-950 dark:text-violet-300"><Sparkles className="h-4 w-4" /></span>
            <div className="min-w-0 flex-1 rounded-2xl rounded-tl-md border border-slate-800 bg-slate-900 px-4 py-3 shadow-sm">
              <div className="flex items-center gap-2 text-xs font-semibold text-slate-200">
                <Loader2 className="h-3.5 w-3.5 animate-spin text-violet-400" />
                Preparing response
              </div>
              <div className="mt-2 text-sm leading-6 text-slate-200">
                <ConversationMarkdownRenderer content={streamingText} maxHeight="none" framed={false} />
              </div>
            </div>
          </article>
        ) : null}
      </div>
    </div>
  )
}
