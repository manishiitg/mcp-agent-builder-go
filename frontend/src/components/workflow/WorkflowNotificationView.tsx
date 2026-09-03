import { useCallback, useEffect, useState } from 'react'
import {
  AlertCircle,
  Ban,
  BellRing,
  CheckCircle2,
  Loader2,
  Mail,
  MailX,
  RefreshCw,
  Webhook,
} from 'lucide-react'
import {
  loadWorkflowNotificationInfo,
  type WorkflowNotificationInfo,
  type WorkflowNotificationState,
} from '../../services/workflow-notifications'
import { formatNotificationStateLabel } from './notificationStatus'

interface WorkflowNotificationViewProps {
  workspacePath: string | null
  onStateLoaded?: (state: WorkflowNotificationState) => void
  loadInfo?: () => Promise<WorkflowNotificationInfo>
  onSetup?: () => void
}

const iconButtonClass = 'inline-flex h-8 w-8 items-center justify-center rounded-md border border-border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50'
const setupClass = 'inline-flex items-center gap-1 rounded-md border border-border bg-muted/40 px-2.5 py-1.5 text-[11px] text-muted-foreground'

const stateBadgeClass = (state: WorkflowNotificationState): string => {
  switch (state) {
    case 'ready':
      return 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    case 'missing_secret':
    case 'invalid_secret':
      return 'border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
    default:
      return 'border-border bg-background text-muted-foreground'
  }
}

const summaryFor = (info: WorkflowNotificationInfo): string => {
  const destination = 'this workflow’s Slack webhook'
  const unconfigured = 'workflow-specific Slack destination'
  switch (info.effectiveState) {
    case 'ready': {
      const readyDestinations = [
        info.slackWebhook.state === 'ready' ? destination : null,
        info.gmail?.state === 'ready' ? 'the inherited Gmail account channel' : null,
      ].filter((value): value is string => Boolean(value))
      return `The agent can decide when a notification is useful. The backend delivers notify_user calls through ${readyDestinations.join(' and ') || 'the enabled notification channels'}.`
    }
    case 'missing_secret':
      return 'A Slack webhook is referenced, but its selected encrypted secret is missing. Use /notify to repair or replace it.'
    case 'invalid_secret':
      return 'The referenced encrypted secret is not a valid Slack Incoming Webhook URL. Use /notify to replace it safely.'
    default:
      return `No ${unconfigured} is configured. Use /notify to choose notification behavior and connect one if needed.`
  }
}

export default function WorkflowNotificationView({
  workspacePath,
  onStateLoaded,
  loadInfo,
  onSetup,
}: WorkflowNotificationViewProps) {
  const [loading, setLoading] = useState(false)
  const [info, setInfo] = useState<WorkflowNotificationInfo | null>(null)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    if (!workspacePath && !loadInfo) return
    setLoading(true)
    setError(null)
    try {
      const next = loadInfo ? await loadInfo() : await loadWorkflowNotificationInfo(workspacePath as string)
      setInfo(next)
      onStateLoaded?.(next.effectiveState)
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Failed to load notification status')
    } finally {
      setLoading(false)
    }
  }, [loadInfo, onStateLoaded, workspacePath])

  useEffect(() => {
    void load()
  }, [load])

  // Gmail auth resolves in a background subprocess on the server (~5s), so the
  // first read returns state="checking" rather than making the popup wait. Poll
  // until it settles; this is the only field that arrives late.
  useEffect(() => {
    if (info?.gmail?.state !== 'checking') return
    const timer = setTimeout(() => { void load() }, 1500)
    return () => clearTimeout(timer)
  }, [info?.gmail?.state, load])

  const state = info?.effectiveState || 'not_configured'
  const StateIcon = state === 'ready' ? CheckCircle2 : state === 'missing_secret' || state === 'invalid_secret' ? AlertCircle : BellRing
  const gmailReady = info?.gmail?.state === 'ready'
  const gmailChecking = info?.gmail?.state === 'checking'
  // Where mail actually lands. The agent may name its own recipients per send,
  // so the default is the floor, not the whole story — and the denylist applies
  // either way, which is the part worth stating outright.
  const gmailDefault = info?.gmail?.default_recipient?.trim() || ''
  const gmailBlocked = info?.gmail?.blocked_recipients || []
  const gmailSender = info?.gmail?.default_sender?.trim() || ''
  const gmailSenderChoices = info?.gmail?.sender_choices || []
  // Only worth naming the connection when more than one account exists —
  // otherwise "Sends from" already says everything there is to say.
  const gmailSenderLabel =
    gmailSenderChoices.length > 1
      ? gmailSenderChoices.find((c) => c.is_default)?.display_name || ''
      : ''
  const scopeName = info?.scopeLabel || workspacePath?.split('/').filter(Boolean).pop() || 'Workflow'
  const scopeLabel = 'workflow'
  // A sender is shown by address when the registry knows it, else by its id.
  const senderLabel = (id: string) => {
    const choice = gmailSenderChoices.find(entry => entry.id === id)
    return choice ? (choice.email || choice.display_name || id) : id
  }
  // The two summaries the final Notify step sends. Read-only here on purpose:
  // the editable form that used to sit in this panel duplicated what /notify
  // configures in chat, with two unlabeled "Send through" pickers that looked
  // like the same control twice (user, 2026-09-03).
  const summaries = info ? [
    {
      key: 'run',
      title: 'Workflow run summary',
      description: 'What happened in the run: outcomes, outputs, failures, goal movement, and metrics.',
      instructions: info.runSummaryInstructions.trim(),
      channels: info.runSummaryChannels,
      senders: info.runSummaryGmailConnectionIds.map(senderLabel),
      recipients: info.runSummaryRecipients,
      slackWebhooks: info.runSummarySlackWebhooks,
    },
    {
      key: 'pulse',
      title: 'Pulse review summary',
      description: "What Pulse found or changed: reviews, fixes, recommendations, decisions, and next actions.",
      instructions: info.pulseSummaryInstructions.trim(),
      channels: info.pulseSummaryChannels,
      senders: info.pulseSummaryGmailConnectionIds.map(senderLabel),
      recipients: info.pulseSummaryRecipients,
      slackWebhooks: info.pulseSummarySlackWebhooks,
    },
  ] : []
  const chipClass = 'w-fit max-w-full truncate rounded-full border border-border bg-muted px-2 py-0.5 text-xs text-foreground'
  const mutedChipClass = 'w-fit rounded-full border border-border bg-background px-2 py-0.5 text-xs text-muted-foreground'
  return (
        <div className="flex h-full min-h-0 w-full max-w-none flex-col bg-background">
          <div className="flex items-start justify-between gap-3 border-b border-border px-4 py-3 sm:px-5 sm:py-3.5">
            <div className="min-w-0">
              <h2 className="flex items-center gap-2 text-base font-semibold text-foreground">
                <BellRing className="h-4 w-4 text-primary" />
                Notify
              </h2>
              <p className="mt-0.5 truncate text-xs text-muted-foreground">Agentic, one-way notifications for {scopeName}</p>
            </div>
          </div>

          {error && (
            <div className="flex items-center gap-2 bg-destructive/10 px-5 py-2 text-xs text-destructive">
              <AlertCircle className="h-3.5 w-3.5 flex-shrink-0" />
              <span className="min-w-0 flex-1">{error}</span>
            </div>
          )}

          <div className="flex-1 overflow-y-auto px-4 py-4 sm:px-5">
            {loading && !info ? (
              <div className="flex items-center justify-center py-12"><Loader2 className="h-5 w-5 animate-spin text-muted-foreground" /></div>
            ) : info ? (
              <div className="space-y-4">
                <section className="overflow-hidden rounded-md border border-border">
                  <div className="flex flex-col gap-3 bg-muted/30 px-4 py-4 sm:flex-row sm:items-start sm:justify-between">
                    <div className="min-w-0">
                      <div className="flex flex-wrap items-center gap-2">
                        <StateIcon className={`h-4 w-4 ${state === 'ready' ? 'text-emerald-500' : state === 'missing_secret' || state === 'invalid_secret' ? 'text-amber-500' : 'text-muted-foreground'}`} />
                        <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium ${stateBadgeClass(state)}`}>
                          {formatNotificationStateLabel(state)}
                        </span>
                      </div>
                      <h3 className="mt-2 text-base font-semibold text-foreground">Agentic notification delivery</h3>
                      <p className="mt-1 max-w-2xl text-sm text-muted-foreground">{summaryFor(info)}</p>
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      <button onClick={() => { void load() }} disabled={loading} className={iconButtonClass} aria-label="Refresh notification status">
                        <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
                      </button>
                      {onSetup ? (
                        <button type="button" onClick={onSetup} className={`${setupClass} transition-colors hover:bg-muted hover:text-foreground`}>Set up · test in chat with <code className="rounded bg-background px-1 font-medium text-foreground">/notify</code></button>
                      ) : (
                        <span className={setupClass}>Set up · test in chat with <code className="rounded bg-background px-1 font-medium text-foreground">/notify</code></span>
                      )}
                    </div>
                  </div>

                </section>

                <section className="rounded-md border border-border">
                  <div className="border-b border-border px-4 py-3">
                    <h3 className="text-sm font-semibold text-foreground">Effective destinations</h3>
                    <p className="mt-0.5 text-xs text-muted-foreground">The agent never reads a webhook URL. It calls notify_user; the server applies these destinations and renders Slack as rich Block Kit by default.</p>
                  </div>
                  <div className="divide-y divide-border">
                    <div className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-start sm:justify-between">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <Webhook className="h-3.5 w-3.5 text-muted-foreground" />
                          <span className="text-sm font-medium text-foreground">Workflow Slack webhook</span>
                        </div>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {info.slackWebhook.secret_name
                            ? <>Encrypted secret reference: <code>{info.slackWebhook.secret_name}</code></>
                            : info.slackWebhook.summary || `No ${scopeLabel}-specific webhook selected.`}
                        </p>
                      </div>
                      <span className={`w-fit rounded-full border px-2 py-0.5 text-xs ${stateBadgeClass(state)}`}>{formatNotificationStateLabel(state)}</span>
                    </div>

                    <div className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-start sm:justify-between">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <Mail className="h-3.5 w-3.5 text-muted-foreground" />
                          <span className="text-sm font-medium text-foreground">Gmail account channel</span>
                          <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-muted-foreground">Inherited</span>
                        </div>
                        <p className="mt-1 text-xs text-muted-foreground">
                          {gmailReady
                            ? 'Available to this workflow. The agent may supply specific recipients when explicitly configured.'
                            : info.gmail?.summary || 'Not ready at account level. Configure and test Gmail from Notification channels.'}
                        </p>
                        {(gmailSender || gmailDefault || gmailBlocked.length > 0) && (
                          <dl className="mt-2 space-y-1 text-xs">
                            {gmailSender && (
                              <div className="flex flex-wrap items-baseline gap-x-1.5">
                                <dt className="text-muted-foreground">Sends from</dt>
                                <dd className="font-mono text-foreground">{gmailSender}</dd>
                                {gmailSenderLabel && (
                                  <dd className="text-muted-foreground">({gmailSenderLabel}, inherited default)</dd>
                                )}
                              </div>
                            )}
                            {gmailDefault && (
                              <div className="flex flex-wrap items-baseline gap-x-1.5">
                                <dt className="text-muted-foreground">Sends to</dt>
                                <dd className="font-mono text-foreground">{gmailDefault}</dd>
                              </div>
                            )}
                            {gmailBlocked.length > 0 && (
                              <div className="flex flex-wrap items-baseline gap-x-1.5">
                                <dt className="text-muted-foreground">Never sends to</dt>
                                <dd className="font-mono text-foreground">{gmailBlocked.join(', ')}</dd>
                              </div>
                            )}
                          </dl>
                        )}
                      </div>
                      <span className={`w-fit rounded-full border px-2 py-0.5 text-xs ${gmailReady ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300' : 'border-border bg-background text-muted-foreground'}`}>
                        {gmailChecking ? 'Checking…' : gmailReady ? 'Available' : 'Not ready'}
                      </span>
                    </div>
                  </div>
                </section>

                <section className="rounded-md border border-border">
                  <div className="border-b border-border px-4 py-3">
                    <h3 className="text-sm font-semibold text-foreground">Per-{scopeLabel} preferences</h3>
                    <p className="mt-0.5 text-xs text-muted-foreground">Stored in <code>workflow.json</code> notifications and applied to every notify_user send. These narrow inherited account-level delivery for this {scopeLabel} only — edit through <code className="text-foreground">/notify</code>.</p>
                  </div>
                  <div className="divide-y divide-border">
                    <div className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-start sm:justify-between">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <Ban className="h-3.5 w-3.5 text-muted-foreground" />
                          <span className="text-sm font-medium text-foreground">Excluded channels</span>
                        </div>
                        <p className="mt-1 text-xs text-muted-foreground">Inherited account channels this {scopeLabel} opts out of.</p>
                      </div>
                      {info.excludeChannels.length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                          {info.excludeChannels.map(channel => (
                            <span key={channel} className="w-fit rounded-full border border-amber-500/30 bg-amber-500/10 px-2 py-0.5 text-xs capitalize text-amber-700 dark:text-amber-300">{channel}</span>
                          ))}
                        </div>
                      ) : (
                        <span className="w-fit rounded-full border border-border bg-background px-2 py-0.5 text-xs text-muted-foreground">None — all enabled channels</span>
                      )}
                    </div>

                    <div className="flex flex-col gap-2 px-4 py-3 sm:flex-row sm:items-start sm:justify-between">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <MailX className="h-3.5 w-3.5 text-muted-foreground" />
                          <span className="text-sm font-medium text-foreground">Blocked recipients</span>
                        </div>
                        <p className="mt-1 text-xs text-muted-foreground">Emails this {scopeLabel} never sends to, on top of the account-wide denylist. A blocked address is dropped from a send; the remaining recipients still get it.</p>
                      </div>
                      {info.blockRecipients.length > 0 ? (
                        <div className="flex min-w-0 flex-wrap gap-1.5">
                          {info.blockRecipients.map(email => (
                            <span key={email} className="w-fit max-w-full truncate rounded-full border border-border bg-muted px-2 py-0.5 font-mono text-xs text-foreground" title={email}>{email}</span>
                          ))}
                        </div>
                      ) : (
                        <span className="w-fit rounded-full border border-border bg-background px-2 py-0.5 text-xs text-muted-foreground">None</span>
                      )}
                    </div>
                  </div>
                </section>

                <section className="rounded-md border border-border">
                  <div className="border-b border-border px-4 py-3">
                    <h3 className="text-sm font-semibold text-foreground">Summary content, channels and recipients</h3>
                    <p className="mt-0.5 text-xs text-muted-foreground">Two summaries go out: the workflow run result, and Pulse's review of it. Each has its own instructions, channels, sender and recipients — set them in chat with <code className="text-foreground">/notify</code>, the same way as the preferences above.</p>
                  </div>
                  <div className="divide-y divide-border">
                    {summaries.map(summary => (
                      <div key={summary.key} className="space-y-2 px-4 py-3">
                        <div>
                          <span className="text-sm font-medium text-foreground">{summary.title}</span>
                          <p className="mt-0.5 text-xs text-muted-foreground">{summary.description}</p>
                        </div>
                        <dl className="space-y-1.5 text-xs">
                          <div className="flex flex-wrap items-baseline gap-x-1.5 gap-y-1">
                            <dt className="text-muted-foreground">Instructions</dt>
                            <dd className="min-w-0 text-foreground">{summary.instructions ? <span className="italic">“{summary.instructions}”</span> : <span className="text-muted-foreground">Default summary — no custom instructions.</span>}</dd>
                          </div>
                          <div className="flex flex-wrap items-baseline gap-x-1.5 gap-y-1">
                            <dt className="text-muted-foreground">Sends through</dt>
                            <dd className="flex flex-wrap gap-1.5">
                              {summary.channels.length > 0
                                ? summary.channels.map(channel => <span key={channel} className={chipClass + ' capitalize'}>{channel}</span>)
                                : <span className={mutedChipClass}>All enabled channels</span>}
                            </dd>
                          </div>
                          <div className="flex flex-wrap items-baseline gap-x-1.5 gap-y-1">
                            <dt className="text-muted-foreground">Email from</dt>
                            <dd className="flex flex-wrap gap-1.5">
                              {summary.senders.length > 0
                                ? summary.senders.map(sender => <span key={sender} className={chipClass + ' font-mono'} title={sender}>{sender}</span>)
                                : <span className={mutedChipClass}>{gmailSender ? `Account default (${gmailSender})` : 'Account default'}</span>}
                            </dd>
                          </div>
                          <div className="flex flex-wrap items-baseline gap-x-1.5 gap-y-1">
                            <dt className="text-muted-foreground">Email to</dt>
                            <dd className="flex flex-wrap gap-1.5">
                              {summary.recipients.length > 0
                                ? summary.recipients.map(email => <span key={email} className={chipClass + ' font-mono'} title={email}>{email}</span>)
                                : <span className={mutedChipClass}>{gmailDefault ? `Account default (${gmailDefault})` : 'Account default — none set'}</span>}
                            </dd>
                          </div>
                          <div className="flex flex-wrap items-baseline gap-x-1.5 gap-y-1">
                            <dt className="text-muted-foreground">Slack channel</dt>
                            <dd className="flex flex-wrap gap-1.5">
                              {summary.slackWebhooks.length > 0
                                ? summary.slackWebhooks.map(secret => <span key={secret} className={chipClass + ' font-mono'} title={`Posts through encrypted webhook secret ${secret}`}>{secret}</span>)
                                : info.slackWebhook.secret_name
                                  ? <span className={mutedChipClass}>Workflow webhook ({info.slackWebhook.secret_name})</span>
                                  : <span className={mutedChipClass}>None configured</span>}
                            </dd>
                          </div>
                        </dl>
                      </div>
                    ))}
                  </div>
                </section>

              </div>
            ) : null}
          </div>
        </div>
  )
}
