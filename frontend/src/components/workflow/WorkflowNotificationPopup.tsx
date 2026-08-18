import { useCallback, useEffect, useState } from 'react'
import {
  AlertCircle,
  Ban,
  BellRing,
  Bot,
  CheckCircle2,
  Loader2,
  Mail,
  MailX,
  RefreshCw,
  Save,
  ServerCog,
  Webhook,
  X,
} from 'lucide-react'
import {
  loadWorkflowNotificationInfo,
  type WorkflowNotificationInfo,
  type WorkflowNotificationState,
} from '../../services/workflow-notifications'
import { agentApi } from '../../services/api'
import ModalPortal from '../ui/ModalPortal'
import { formatNotificationStateLabel } from './notificationStatus'

interface WorkflowNotificationPopupProps {
  isOpen: boolean
  onClose: () => void
  workspacePath: string | null
  onStateLoaded?: (state: WorkflowNotificationState) => void
  loadInfo?: () => Promise<WorkflowNotificationInfo>
  onSetup?: () => void
}

const iconButtonClass = 'inline-flex h-8 w-8 items-center justify-center rounded-md border border-border text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50'
const setupClass = 'inline-flex items-center gap-1 rounded-md border border-border bg-muted/40 px-2.5 py-1.5 text-[11px] text-muted-foreground'

// RecipientEditor edits one summary's To list. An empty list is a valid, normal
// state — it means "use the account default" — so it reads as a fallback rather
// than an error. Addresses already on a denylist are flagged here because the
// send path silently skips them, which is otherwise invisible until an expected
// email never arrives.
function RecipientEditor({
  label,
  recipients,
  onChange,
  blockedRecipients,
  accountDefault,
  inputId,
}: {
  label: string
  recipients: string[]
  onChange: (next: string[]) => void
  blockedRecipients: string[]
  accountDefault: string
  inputId: string
}) {
  const [draft, setDraft] = useState('')
  const [invalid, setInvalid] = useState<string | null>(null)
  const blockedSet = new Set(blockedRecipients.map(email => email.trim().toLowerCase()))

  const commit = (raw: string) => {
    const parts = raw.split(/[,;\s]+/).map(part => part.trim().toLowerCase()).filter(Boolean)
    if (parts.length === 0) return
    const rejected = parts.filter(part => !part.includes('@'))
    if (rejected.length > 0) {
      setInvalid(`${rejected[0]} is not an email address`)
      return
    }
    setInvalid(null)
    onChange([...recipients, ...parts.filter(part => !recipients.includes(part))])
    setDraft('')
  }

  return (
    <div className="space-y-1.5">
      <label htmlFor={inputId} className="block text-xs font-medium text-foreground">{label}</label>
      <div className="flex flex-wrap items-center gap-1.5 rounded-md border border-border bg-background px-2 py-2">
        {recipients.map(email => (
          <span
            key={email}
            className={`inline-flex max-w-full items-center gap-1 rounded-full border px-2 py-0.5 font-mono text-xs ${blockedSet.has(email)
              ? 'border-destructive/40 bg-destructive/10 text-destructive'
              : 'border-border bg-muted text-foreground'}`}
            title={blockedSet.has(email) ? `${email} is on a blocked recipients list and will be skipped` : email}
          >
            <span className="truncate">{email}</span>
            <button
              type="button"
              onClick={() => onChange(recipients.filter(value => value !== email))}
              className="text-muted-foreground transition-colors hover:text-foreground"
              aria-label={`Remove ${email}`}
            >
              <X className="h-3 w-3" />
            </button>
          </span>
        ))}
        <input
          id={inputId}
          type="text"
          inputMode="email"
          value={draft}
          onChange={event => { setDraft(event.target.value); setInvalid(null) }}
          onKeyDown={event => {
            if (event.key === 'Enter' || event.key === ',') {
              event.preventDefault()
              commit(draft)
            } else if (event.key === 'Backspace' && draft === '' && recipients.length > 0) {
              onChange(recipients.slice(0, -1))
            }
          }}
          // Commit on blur too: typing an address and clicking Save directly is
          // the obvious way to use this, and losing that entry would look like
          // the save silently dropped it.
          onBlur={() => commit(draft)}
          placeholder={recipients.length === 0 ? 'name@example.com' : 'Add another…'}
          className="min-w-[12rem] flex-1 bg-transparent px-1 py-0.5 text-sm text-foreground outline-none placeholder:text-muted-foreground"
        />
      </div>
      {invalid ? (
        <span className="block text-xs text-destructive">{invalid}</span>
      ) : recipients.length === 0 ? (
        <span className="block text-xs text-muted-foreground">
          {accountDefault ? <>Falls back to the account default: <span className="font-mono">{accountDefault}</span></> : 'No recipients set and no account default configured — email would have nowhere to go.'}
        </span>
      ) : recipients.some(email => blockedSet.has(email)) ? (
        <span className="block text-xs text-destructive">Addresses shown in red are on a blocked recipients list and are dropped from the send. The other recipients still receive it.</span>
      ) : null}
    </div>
  )
}

// SlackChannelRow reports which Slack channel a summary posts to. A Slack
// Incoming Webhook is bound to one channel at creation and cannot be
// retargeted, so a channel here IS a webhook secret. Read-only: choosing one
// means picking an encrypted secret, which /notify does with list_secrets.
function SlackChannelRow({ webhooks, fallbackSecretName }: { webhooks: string[]; fallbackSecretName?: string }) {
  return (
    <div className="space-y-1">
      <span className="block text-xs font-medium text-foreground">Slack channel</span>
      {webhooks.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {webhooks.map(secret => (
            <span key={secret} className="w-fit max-w-full truncate rounded-full border border-border bg-muted px-2 py-0.5 font-mono text-xs text-foreground" title={`Posts through encrypted webhook secret ${secret}`}>{secret}</span>
          ))}
        </div>
      ) : (
        <span className="block text-xs text-muted-foreground">
          {fallbackSecretName
            ? <>Uses the workflow webhook <code className="font-mono">{fallbackSecretName}</code>. Set a different channel for this summary with <code className="text-foreground">/notify</code>.</>
            : <>No Slack webhook configured. Add one with <code className="text-foreground">/notify</code>.</>}
        </span>
      )}
    </div>
  )
}

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

export default function WorkflowNotificationPopup({
  isOpen,
  onClose,
  workspacePath,
  onStateLoaded,
  loadInfo,
  onSetup,
}: WorkflowNotificationPopupProps) {
  const [loading, setLoading] = useState(false)
  const [info, setInfo] = useState<WorkflowNotificationInfo | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [runInstructions, setRunInstructions] = useState('')
  const [pulseInstructions, setPulseInstructions] = useState('')
  const [runChannels, setRunChannels] = useState<string[]>([])
  const [pulseChannels, setPulseChannels] = useState<string[]>([])
  const [runRecipients, setRunRecipients] = useState<string[]>([])
  const [pulseRecipients, setPulseRecipients] = useState<string[]>([])
  const [savingInstructions, setSavingInstructions] = useState(false)

  const load = useCallback(async () => {
    if (!workspacePath && !loadInfo) return
    setLoading(true)
    setError(null)
    try {
      const next = loadInfo ? await loadInfo() : await loadWorkflowNotificationInfo(workspacePath as string)
      setInfo(next)
      setRunInstructions(next.runSummaryInstructions)
      setPulseInstructions(next.pulseSummaryInstructions)
      setRunChannels(next.runSummaryChannels)
      setPulseChannels(next.pulseSummaryChannels)
      setRunRecipients(next.runSummaryRecipients)
      setPulseRecipients(next.pulseSummaryRecipients)
      onStateLoaded?.(next.effectiveState)
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : 'Failed to load notification status')
    } finally {
      setLoading(false)
    }
  }, [loadInfo, onStateLoaded, workspacePath])

  useEffect(() => {
    if (isOpen) void load()
  }, [isOpen, load])

  // Gmail auth resolves in a background subprocess on the server (~5s), so the
  // first read returns state="checking" rather than making the popup wait. Poll
  // until it settles; this is the only field that arrives late.
  useEffect(() => {
    if (!isOpen || info?.gmail?.state !== 'checking') return
    const timer = setTimeout(() => { void load() }, 1500)
    return () => clearTimeout(timer)
  }, [isOpen, info?.gmail?.state, load])

  if (!isOpen) return null

  const state = info?.effectiveState || 'not_configured'
  const StateIcon = state === 'ready' ? CheckCircle2 : state === 'missing_secret' || state === 'invalid_secret' ? AlertCircle : BellRing
  const gmailReady = info?.gmail?.state === 'ready'
  const gmailChecking = info?.gmail?.state === 'checking'
  // Where mail actually lands. The agent may name its own recipients per send,
  // so the default is the floor, not the whole story — and the denylist applies
  // either way, which is the part worth stating outright.
  const gmailDefault = info?.gmail?.default_recipient?.trim() || ''
  const gmailBlocked = info?.gmail?.blocked_recipients || []
  const scopeName = info?.scopeLabel || workspacePath?.split('/').filter(Boolean).pop() || 'Workflow'
  const scopeLabel = 'workflow'
  const instructionsDirty = runInstructions.trim() !== (info?.runSummaryInstructions || '').trim()
    || pulseInstructions.trim() !== (info?.pulseSummaryInstructions || '').trim()
    || JSON.stringify(runChannels) !== JSON.stringify(info?.runSummaryChannels || [])
    || JSON.stringify(pulseChannels) !== JSON.stringify(info?.pulseSummaryChannels || [])
    || JSON.stringify(runRecipients) !== JSON.stringify(info?.runSummaryRecipients || [])
    || JSON.stringify(pulseRecipients) !== JSON.stringify(info?.pulseSummaryRecipients || [])

  const channelOptions = [
    { id: 'slack', label: 'Slack' },
    { id: 'gmail', label: 'Gmail' },
  ]
  const isChannelSelected = (channels: string[], channel: string) => channels.length === 0 || channels.includes(channel)
  const toggleChannel = (channels: string[], channel: string, setChannels: (next: string[]) => void) => {
    const current = channels.length === 0 ? channelOptions.map(option => option.id) : channels
    const next = current.includes(channel) ? current.filter(value => value !== channel) : [...current, channel]
    if (next.length > 0) setChannels(next)
  }

  const saveInstructions = async () => {
    if (!workspacePath || savingInstructions) return
    setSavingInstructions(true)
    setError(null)
    try {
      const saved = await agentApi.updateWorkflowManifest({
        workspace_path: workspacePath,
        run_notification_instructions: runInstructions.trim(),
        pulse_notification_instructions: pulseInstructions.trim(),
        run_notification_channels: runChannels,
        pulse_notification_channels: pulseChannels,
        run_notification_recipients: runRecipients,
        pulse_notification_recipients: pulseRecipients,
      })
      const persistedRun = saved?.manifest?.capabilities?.notifications?.run_summary_instructions || ''
      const persistedPulse = saved?.manifest?.capabilities?.notifications?.pulse_summary_instructions || ''
      const persistedRunChannels = saved?.manifest?.capabilities?.notifications?.run_summary_channels || []
      const persistedPulseChannels = saved?.manifest?.capabilities?.notifications?.pulse_summary_channels || []
      const persistedRunRecipients = saved?.manifest?.capabilities?.notifications?.run_summary_recipients || []
      const persistedPulseRecipients = saved?.manifest?.capabilities?.notifications?.pulse_summary_recipients || []
      if (persistedRun.trim() !== runInstructions.trim()
        || persistedPulse.trim() !== pulseInstructions.trim()
        || JSON.stringify(persistedRunChannels) !== JSON.stringify(runChannels)
        || JSON.stringify(persistedPulseChannels) !== JSON.stringify(pulseChannels)
        || JSON.stringify(persistedRunRecipients) !== JSON.stringify(runRecipients)
        || JSON.stringify(persistedPulseRecipients) !== JSON.stringify(pulseRecipients)) {
        throw new Error('The backend did not save these notification preferences. Restart AgentWorks to load the latest backend, then try again.')
      }
      await load()
    } catch (saveError) {
      setError(saveError instanceof Error ? saveError.message : 'Failed to save notification instructions')
    } finally {
      setSavingInstructions(false)
    }
  }

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/50 p-2 backdrop-blur-sm sm:p-4">
        <div className="relative flex max-h-[calc(100dvh-1rem)] w-full max-w-3xl flex-col rounded-lg border border-border bg-background shadow-xl sm:max-h-[86vh]">
          <div className="flex items-start justify-between gap-3 border-b border-border px-4 py-3 sm:px-5 sm:py-3.5">
            <div className="min-w-0">
              <h2 className="flex items-center gap-2 text-base font-semibold text-foreground">
                <BellRing className="h-4 w-4 text-primary" />
                Notify
              </h2>
              <p className="mt-0.5 truncate text-xs text-muted-foreground">Agentic, one-way notifications for {scopeName}</p>
            </div>
            <button onClick={onClose} className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground" aria-label="Close">
              <X className="h-4 w-4" />
            </button>
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

                  <div className="grid border-t border-border text-sm sm:grid-cols-3">
                    <div className="border-b border-border px-4 py-3 sm:border-b-0 sm:border-r">
                      <div className="flex items-center gap-1.5 text-xs text-muted-foreground"><Bot className="h-3.5 w-3.5" />Decision</div>
                      <div className="mt-1 font-medium text-foreground">Agent chooses when</div>
                    </div>
                    <div className="border-b border-border px-4 py-3 sm:border-b-0 sm:border-r">
                      <div className="flex items-center gap-1.5 text-xs text-muted-foreground"><ServerCog className="h-3.5 w-3.5" />Delivery</div>
                      <div className="mt-1 font-medium text-foreground">Backend resolves secrets</div>
                    </div>
                    <div className="px-4 py-3">
                      <div className="text-xs text-muted-foreground">Reply behavior</div>
                      <div className="mt-1 font-medium text-foreground">One-way · never blocks</div>
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
                        {(gmailDefault || gmailBlocked.length > 0) && (
                          <dl className="mt-2 space-y-1 text-xs">
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
                      <h3 className="text-sm font-semibold text-foreground">Notification content and recipients</h3>
                      <p className="mt-0.5 text-xs text-muted-foreground">Set separate content, delivery channels, and email recipients for the workflow result and Pulse's review.</p>
                    </div>
                    <div className="space-y-4 px-4 py-3">
                      <div className="block space-y-1.5">
                        <span className="text-xs font-medium text-foreground">Workflow run summary</span>
                        <span className="block text-xs text-muted-foreground">What happened in the run: outcomes, outputs, failures, goal movement, and metrics.</span>
                        <textarea
                          value={runInstructions}
                          onChange={(event) => setRunInstructions(event.target.value.slice(0, 2000))}
                          placeholder="Example: Give me a detailed result summary with the key numbers, failures, and what was delivered."
                          className="min-h-24 w-full resize-y rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none placeholder:text-muted-foreground focus:border-primary"
                          aria-label="Workflow run notification instructions"
                        />
                        <span className="block text-right text-xs text-muted-foreground">{runInstructions.length}/2000</span>
                        <span className="block text-xs font-medium text-foreground">Send through</span>
                        <div className="flex flex-wrap gap-2">
                          {channelOptions.map(channel => (
                            <label key={`run-${channel.id}`} className="inline-flex cursor-pointer items-center gap-2 rounded-md border border-border px-2.5 py-1.5 text-xs text-foreground">
                              <input type="checkbox" checked={isChannelSelected(runChannels, channel.id)} onChange={() => toggleChannel(runChannels, channel.id, setRunChannels)} className="accent-primary" />
                              {channel.label}
                            </label>
                          ))}
                        </div>
                        <RecipientEditor
                          inputId="run-summary-recipients"
                          label="Email to"
                          recipients={runRecipients}
                          onChange={setRunRecipients}
                          blockedRecipients={[...gmailBlocked, ...(info?.blockRecipients || [])]}
                          accountDefault={gmailDefault}
                        />
                        <SlackChannelRow webhooks={info?.runSummarySlackWebhooks || []} fallbackSecretName={info?.slackWebhook.secret_name} />
                      </div>
                      <div className="block space-y-1.5">
                        <span className="text-xs font-medium text-foreground">Pulse review summary</span>
                        <span className="block text-xs text-muted-foreground">What Pulse found or changed: reviews, fixes, recommendations, decisions, and next actions.</span>
                        <textarea
                          value={pulseInstructions}
                          onChange={(event) => setPulseInstructions(event.target.value.slice(0, 2000))}
                          placeholder="Example: Explain only material findings and fixes. Put decisions I need to make first."
                          className="min-h-24 w-full resize-y rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground outline-none placeholder:text-muted-foreground focus:border-primary"
                          aria-label="Pulse review notification instructions"
                        />
                        <span className="block text-right text-xs text-muted-foreground">{pulseInstructions.length}/2000</span>
                        <span className="block text-xs font-medium text-foreground">Send through</span>
                        <div className="flex flex-wrap gap-2">
                          {channelOptions.map(channel => (
                            <label key={`pulse-${channel.id}`} className="inline-flex cursor-pointer items-center gap-2 rounded-md border border-border px-2.5 py-1.5 text-xs text-foreground">
                              <input type="checkbox" checked={isChannelSelected(pulseChannels, channel.id)} onChange={() => toggleChannel(pulseChannels, channel.id, setPulseChannels)} className="accent-primary" />
                              {channel.label}
                            </label>
                          ))}
                        </div>
                        <RecipientEditor
                          inputId="pulse-summary-recipients"
                          label="Email to"
                          recipients={pulseRecipients}
                          onChange={setPulseRecipients}
                          blockedRecipients={[...gmailBlocked, ...(info?.blockRecipients || [])]}
                          accountDefault={gmailDefault}
                        />
                        <SlackChannelRow webhooks={info?.pulseSummarySlackWebhooks || []} fallbackSecretName={info?.slackWebhook.secret_name} />
                      </div>
                      <div className="flex items-center justify-between gap-3">
                        <span className="text-xs text-muted-foreground">Used by the final Notify step on every Pulse run.</span>
                        <button
                          type="button"
                          onClick={() => { void saveInstructions() }}
                          disabled={!instructionsDirty || savingInstructions}
                          className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
                        >
                          {savingInstructions ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
                          Save preferences
                        </button>
                      </div>
                    </div>
                </section>

                <div className="rounded-md border border-blue-500/20 bg-blue-500/5 px-4 py-3 text-xs text-muted-foreground">
                  Configure notification intent, the Slack destination, channel opt-outs, and blocked recipients through <code className="text-foreground">/notify</code>. This does not add a routing step. Short-lived questions that require an answer still use <code className="text-foreground">human_feedback</code> instead.
                </div>
              </div>
            ) : null}
          </div>
        </div>
      </div>
    </ModalPortal>
  )
}
