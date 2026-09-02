import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertCircle, AlertTriangle, ArrowLeft, CheckCircle, ChevronDown, ChevronRight,
  Eye, EyeOff, Loader2, Mail, MessageSquare, Phone, Plus, RotateCcw, Trash2, X,
} from 'lucide-react'
import { Button } from '../ui/Button'
import { Card } from '../ui/Card'
import { agentApi } from '../../services/api'
import { useWorkflowManifestStore } from '../../stores/useWorkflowManifestStore'
import { READ_ONLY_TITLE, useCanWriteWorkflow } from '../../hooks/useCanWriteWorkflow'
import type {
  ChannelRoute, GmailConfigRequest, GmailConfigResponse, GmailTestResponse,
  SlackConfig, SlackConfigRequest, SlackTestResponse,
} from '../../services/api-types'

// ── Types ──────────────────────────────────────────────────────────────────

type ChannelKind = 'slack' | 'whatsapp'

// Shape of GET/PUT /api/whatsapp/routing entries. Same idea as ChannelRoute
// but workshop_mode is an untyped string on this endpoint.
type WaRoute = { workflow_id: string; workspace_path?: string; workshop_mode?: string; send_full_details?: boolean }

// One route this workflow answers on, regardless of platform.
type WorkflowRoute = {
  kind: ChannelKind
  key: string // Slack channel ID or WhatsApp slug
  workshop_mode?: string
  send_full_details?: boolean
}

// Shape of GET /api/whatsapp/status. enabled = connector started at server
// startup; paired = device identity stored; connected = live WS.
interface WhatsAppStatus {
  enabled: boolean
  paired: boolean
  connected: boolean
  own_jid: string
  qr_available: boolean
  qr_expires_at?: string
  link_code?: string
  link_code_expires_at?: string
  bound_chat_count?: number
  owner_user_id?: string
  owner_email?: string
  owner_username?: string
  owner_paired_at?: string
}

const SLUG_RE = /^[a-z0-9-]+$/

const normalizeGmailEmails = (values: string | string[] | undefined): string[] => {
  const source = Array.isArray(values) ? values : [values || '']
  const seen = new Set<string>()
  const result: string[] = []
  for (const raw of source) {
    for (const part of String(raw).split(/[\s,;]+/)) {
      const email = part.trim().toLowerCase()
      if (!email || seen.has(email)) continue
      seen.add(email)
      result.push(email)
    }
  }
  return result
}

const emptyGmailConfig: GmailConfigResponse = {
  enabled: false,
  default_to: '',
  auth: { gws_installed: false, authenticated: false, has_gmail_scope: false },
  ready: false,
}

const toggleClass = "w-11 h-6 bg-gray-200 peer-focus:outline-none rounded-full peer dark:bg-gray-700 peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all dark:border-gray-600 peer-checked:bg-blue-600"

type WorkflowBotsPanelProps = {
  workspacePath: string | null
}

export default function WorkflowBotsPanel({ workspacePath }: WorkflowBotsPanelProps) {
  // ── Workflow identity ─────────────────────────────────────────────────────
  const workflows = useWorkflowManifestStore(state => state.workflows)
  const refreshWorkflows = useWorkflowManifestStore(state => state.refreshWorkflows)
  const workflow = useMemo(
    () => (workspacePath ? workflows.find(w => w.workspace_path === workspacePath) : undefined),
    [workflows, workspacePath],
  )
  const workflowId = workflow?.manifest.id ?? null
  const workflowLabel = (id: string) => workflows.find(w => w.manifest.id === id)?.manifest.label || id
  // Every write here lands in shared connector config immediately -- there's
  // no Save step for the panel to gate -- so each mutating control disables.
  const readOnly = !useCanWriteWorkflow()

  useEffect(() => {
    if (workflows.length === 0) void refreshWorkflows()
  }, [refreshWorkflows, workflows.length])

  // ── Panel navigation ──────────────────────────────────────────────────────
  const [setup, setSetup] = useState<ChannelKind | null>(null)
  const [expandedChip, setExpandedChip] = useState<string | null>(null)
  const [routeSaving, setRouteSaving] = useState<string | null>(null)
  const [routeError, setRouteError] = useState<string | null>(null)
  const [newSlackChannel, setNewSlackChannel] = useState('')
  const [newWaSlug, setNewWaSlug] = useState('')
  const [addError, setAddError] = useState<Partial<Record<ChannelKind, string>>>({})

  // ── Slack ─────────────────────────────────────────────────────────────────
  const [slackConfig, setSlackConfig] = useState<SlackConfig>({ enabled: false, bot_token: '', app_token: '', channel_id: '' })
  const [slackOriginal, setSlackOriginal] = useState<SlackConfig>(slackConfig)
  const [slackLoading, setSlackLoading] = useState(true)
  const [slackSaving, setSlackSaving] = useState(false)
  const [slackTesting, setSlackTesting] = useState(false)
  const [slackError, setSlackError] = useState<string | null>(null)
  const [slackSuccess, setSlackSuccess] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<SlackTestResponse | null>(null)
  const [testReply, setTestReply] = useState<string | null>(null)
  const [pollingForReply, setPollingForReply] = useState(false)
  const [showBotToken, setShowBotToken] = useState(false)
  const [showAppToken, setShowAppToken] = useState(false)
  const [allowedEmails, setAllowedEmails] = useState('')
  const [emailsDirty, setEmailsDirty] = useState(false)
  const [emailsSaving, setEmailsSaving] = useState(false)
  const [emailsSaved, setEmailsSaved] = useState(false)

  // ── WhatsApp ──────────────────────────────────────────────────────────────
  const [waStatus, setWaStatus] = useState<WhatsAppStatus | null>(null)
  const [waError, setWaError] = useState<string | null>(null)
  const [waRouting, setWaRouting] = useState<Record<string, WaRoute>>({})
  const [waRoutingError, setWaRoutingError] = useState<string | null>(null)
  const [qrBust, setQrBust] = useState<number>(() => Date.now())
  const [qrImageURL, setQrImageURL] = useState<string | null>(null)
  const [qrLoading, setQrLoading] = useState(false)
  const [qrError, setQrError] = useState<string | null>(null)
  const [unpairConfirm, setUnpairConfirm] = useState(false)
  const [unpairing, setUnpairing] = useState(false)
  const waPollingRef = useRef<ReturnType<typeof setInterval> | null>(null)

  // ── Gmail (account-wide, shared by every workflow) ────────────────────────
  const [gmailConfig, setGmailConfig] = useState<GmailConfigResponse>(emptyGmailConfig)
  const [gmailOriginal, setGmailOriginal] = useState({ enabled: false, default_to: '', blocked_recipients: [] as string[] })
  const [gmailBlockedText, setGmailBlockedText] = useState('')
  const [gmailLoading, setGmailLoading] = useState(false)
  const [gmailChecking, setGmailChecking] = useState(false)
  const [gmailSaving, setGmailSaving] = useState(false)
  const [gmailTesting, setGmailTesting] = useState(false)
  const [gmailError, setGmailError] = useState<string | null>(null)
  const [gmailSuccess, setGmailSuccess] = useState<string | null>(null)
  const [gmailTestResult, setGmailTestResult] = useState<GmailTestResponse | null>(null)
  const [gmailTestedTo, setGmailTestedTo] = useState<string | null>(null)
  const [gmailOpen, setGmailOpen] = useState(false)

  // ── Loaders ───────────────────────────────────────────────────────────────
  const loadEmails = useCallback(async () => {
    try {
      const cfg = await agentApi.getBotConfig()
      if (Array.isArray(cfg.allowed_emails)) setAllowedEmails(cfg.allowed_emails.join(', '))
    } catch { /* ignore */ }
  }, [])

  const loadSlack = useCallback(async () => {
    try {
      setSlackLoading(true)
      setSlackError(null)
      const data = await agentApi.getSlackFeedbackConfig()
      setSlackConfig(data)
      setSlackOriginal(data)
    } catch (err) {
      setSlackError(err instanceof Error ? err.message : 'Failed to load Slack configuration')
    } finally {
      setSlackLoading(false)
    }
  }, [])

  const loadWaStatus = useCallback(async () => {
    try {
      const s = await agentApi.getWhatsAppStatus()
      setWaStatus(s)
      setWaError(null)
      return s
    } catch (err) {
      setWaError(err instanceof Error ? err.message : String(err))
      return null
    }
  }, [])

  const loadWaRouting = useCallback(async () => {
    try {
      const data = await agentApi.getWhatsAppRouting()
      setWaRouting(data.routing || {})
      setWaRoutingError(null)
    } catch (err) {
      setWaRoutingError(err instanceof Error ? err.message : String(err))
    }
  }, [])

  const loadGmail = useCallback(async (background = false) => {
    try {
      if (background) setGmailChecking(true)
      else setGmailLoading(true)
      setGmailError(null)
      const data = await agentApi.getGmailFeedbackConfig()
      if (background) {
        setGmailConfig(current => ({ ...current, auth: data.auth, ready: data.ready }))
      } else {
        const blocked = normalizeGmailEmails(data.blocked_recipients)
        setGmailConfig({ ...data, blocked_recipients: blocked })
        setGmailBlockedText(blocked.join(', '))
        setGmailOriginal({ enabled: data.enabled, default_to: data.default_to || '', blocked_recipients: blocked })
      }
    } catch (error) {
      setGmailError(error instanceof Error ? error.message : 'Failed to load Gmail configuration')
    } finally {
      setGmailLoading(false)
      setGmailChecking(false)
    }
  }, [])

  useEffect(() => {
    void loadEmails()
    void loadSlack()
    void loadWaStatus()
    void loadWaRouting()
    void loadGmail()
  }, [loadEmails, loadGmail, loadSlack, loadWaRouting, loadWaStatus])

  // ── WhatsApp: status polling while the pairing screen is open ─────────────
  // Not yet paired → poll /status every 3s so a fresh QR (rotating every
  // ~20s on the server) plus the transition to "paired" shows up without the
  // user having to refresh. Only runs while the WhatsApp setup screen is open.
  useEffect(() => {
    if (setup !== 'whatsapp') {
      if (waPollingRef.current) {
        clearInterval(waPollingRef.current)
        waPollingRef.current = null
      }
      return
    }

    let cancelled = false
    let lastExpires: string | undefined
    const tick = async () => {
      try {
        const s = await agentApi.getWhatsAppStatus()
        if (cancelled) return
        setWaStatus(s)
        setWaError(null)
        if (s.qr_expires_at !== lastExpires) {
          lastExpires = s.qr_expires_at
          setQrBust(Date.now())
        }
      } catch (err) {
        if (cancelled) return
        setWaError(err instanceof Error ? err.message : String(err))
      }
    }
    tick()
    waPollingRef.current = setInterval(tick, 3000)
    return () => {
      cancelled = true
      if (waPollingRef.current) {
        clearInterval(waPollingRef.current)
        waPollingRef.current = null
      }
    }
  }, [setup])

  useEffect(() => {
    if (setup !== 'whatsapp' || !waStatus?.enabled || waStatus.paired || !waStatus.qr_available) {
      setQrLoading(false)
      setQrError(null)
      setQrImageURL(prev => {
        if (prev) URL.revokeObjectURL(prev)
        return null
      })
      return
    }

    let cancelled = false
    setQrLoading(true)
    setQrError(null)
    agentApi.getWhatsAppPairQR(384, qrBust)
      .then(blob => {
        if (cancelled) return
        const nextURL = URL.createObjectURL(blob)
        setQrImageURL(prev => {
          if (prev) URL.revokeObjectURL(prev)
          return nextURL
        })
      })
      .catch(err => {
        if (cancelled) return
        setQrImageURL(prev => {
          if (prev) URL.revokeObjectURL(prev)
          return null
        })
        setQrError(err instanceof Error ? err.message : String(err))
      })
      .finally(() => {
        if (!cancelled) setQrLoading(false)
      })

    return () => {
      cancelled = true
    }
  }, [setup, waStatus?.enabled, waStatus?.paired, waStatus?.qr_available, qrBust])

  useEffect(() => {
    return () => {
      if (qrImageURL) URL.revokeObjectURL(qrImageURL)
    }
  }, [qrImageURL])

  // ── Routes for this workflow ──────────────────────────────────────────────
  const myRoutes = useMemo<WorkflowRoute[]>(() => {
    if (!workflowId) return []
    const slack = Object.entries(slackOriginal.channel_routing || {})
      .filter(([, r]) => r.workflow_id === workflowId)
      .map(([key, r]) => ({ kind: 'slack' as const, key, workshop_mode: r.workshop_mode, send_full_details: r.send_full_details }))
    const wa = Object.entries(waRouting)
      .filter(([, r]) => r.workflow_id === workflowId)
      .map(([key, r]) => ({ kind: 'whatsapp' as const, key, workshop_mode: r.workshop_mode, send_full_details: r.send_full_details }))
    return [...slack, ...wa]
  }, [slackOriginal.channel_routing, waRouting, workflowId])

  const routeId = (route: { kind: ChannelKind; key: string }) => `${route.kind}:${route.key}`

  // Slack routing writes: base on the last *loaded* config so unsaved edits
  // on the Set up screen never ride along, and only channel_routing changes.
  // Masked tokens round-trip as "no change" server-side.
  const saveSlackRouting = useCallback(async (next: Record<string, ChannelRoute>) => {
    const request: SlackConfigRequest = {
      enabled: slackOriginal.enabled,
      bot_token: slackOriginal.bot_token || '',
      app_token: slackOriginal.app_token || '',
      channel_id: slackOriginal.channel_id || '',
      bot_mode: slackOriginal.bot_mode || false,
      channel_routing: next,
    }
    await agentApi.updateSlackFeedbackConfig(request)
    await loadSlack()
  }, [loadSlack, slackOriginal])

  // WhatsApp routing writes: PUT replaces the whole map, so always send the
  // full map with only this workflow's entries changed.
  const saveWaRouting = useCallback(async (next: Record<string, WaRoute>) => {
    const data = await agentApi.updateWhatsAppRouting(next)
    setWaRouting(data.routing || {})
    setWaRoutingError(null)
  }, [])

  const withRouteSaving = async (id: string, fn: () => Promise<void>) => {
    setRouteSaving(id)
    setRouteError(null)
    try {
      await fn()
    } catch (err) {
      setRouteError(err instanceof Error ? err.message : 'Failed to save routing')
    } finally {
      setRouteSaving(null)
    }
  }

  const removeRoute = (route: WorkflowRoute) => withRouteSaving(routeId(route), async () => {
    if (route.kind === 'slack') {
      const next = { ...(slackOriginal.channel_routing || {}) }
      delete next[route.key]
      await saveSlackRouting(next)
    } else {
      const next = { ...waRouting }
      delete next[route.key]
      await saveWaRouting(next)
    }
    if (expandedChip === routeId(route)) setExpandedChip(null)
  })

  const updateRoute = (route: WorkflowRoute, patch: { workshop_mode?: string; send_full_details?: boolean }) => withRouteSaving(routeId(route), async () => {
    if (route.kind === 'slack') {
      const current = slackOriginal.channel_routing?.[route.key]
      if (!current) return
      const nextRoute: ChannelRoute = { ...current }
      if ('workshop_mode' in patch) {
        if (patch.workshop_mode === 'run') nextRoute.workshop_mode = 'run'
        else delete nextRoute.workshop_mode
      }
      if ('send_full_details' in patch) {
        if (patch.send_full_details) nextRoute.send_full_details = true
        else delete nextRoute.send_full_details
      }
      await saveSlackRouting({ ...(slackOriginal.channel_routing || {}), [route.key]: nextRoute })
    } else {
      const current = waRouting[route.key]
      if (!current) return
      const nextRoute: WaRoute = { ...current }
      if ('workshop_mode' in patch) nextRoute.workshop_mode = 'run'
      if ('send_full_details' in patch) {
        if (patch.send_full_details) nextRoute.send_full_details = true
        else delete nextRoute.send_full_details
      }
      await saveWaRouting({ ...waRouting, [route.key]: nextRoute })
    }
  })

  const addSlackRoute = () => {
    const channel = newSlackChannel.trim()
    if (!channel || !workflowId) return
    const existing = slackOriginal.channel_routing?.[channel]
    if (existing && existing.workflow_id !== workflowId) {
      setAddError(e => ({ ...e, slack: `Channel ${channel} is already routed to ${workflowLabel(existing.workflow_id)}.` }))
      return
    }
    if (existing) {
      setAddError(e => ({ ...e, slack: `Channel ${channel} is already routed to this workflow.` }))
      return
    }
    setAddError(e => ({ ...e, slack: undefined }))
    void withRouteSaving(`slack:${channel}`, async () => {
      const route: ChannelRoute = { workflow_id: workflowId, workspace_path: workflow?.workspace_path || '' }
      await saveSlackRouting({ ...(slackOriginal.channel_routing || {}), [channel]: route })
      setNewSlackChannel('')
    })
  }

  const addWaRoute = () => {
    const slug = newWaSlug.trim().toLowerCase()
    if (!slug || !workflowId) return
    if (!SLUG_RE.test(slug)) {
      setAddError(e => ({ ...e, whatsapp: 'Slugs can only contain a-z, 0-9 and dashes.' }))
      return
    }
    const existing = waRouting[slug]
    if (existing && existing.workflow_id !== workflowId) {
      setAddError(e => ({ ...e, whatsapp: `@${slug} is already routed to ${workflowLabel(existing.workflow_id)}.` }))
      return
    }
    if (existing) {
      setAddError(e => ({ ...e, whatsapp: `@${slug} is already routed to this workflow.` }))
      return
    }
    setAddError(e => ({ ...e, whatsapp: undefined }))
    void withRouteSaving(`whatsapp:${slug}`, async () => {
      const route: WaRoute = { workflow_id: workflowId, workspace_path: workflow?.workspace_path || '', workshop_mode: 'run' }
      await saveWaRouting({ ...waRouting, [slug]: route })
      setNewWaSlug('')
    })
  }

  // ── Slack handlers (setup screen) ─────────────────────────────────────────
  const handleEmailsSave = async () => {
    setEmailsSaving(true)
    try {
      const emails = allowedEmails.split(',').map(e => e.trim()).filter(e => e.length > 0)
      await agentApi.saveBotConfig({ allowed_emails: emails })
      setEmailsDirty(false)
      setEmailsSaved(true)
      setTimeout(() => setEmailsSaved(false), 2000)
    } catch { /* ignore */ } finally { setEmailsSaving(false) }
  }

  const handleSlackSave = async () => {
    try {
      setSlackSaving(true)
      setSlackError(null)
      setSlackSuccess(null)
      const request: SlackConfigRequest = {
        enabled: slackConfig.enabled, bot_token: slackConfig.bot_token || '',
        app_token: slackConfig.app_token || '', channel_id: slackConfig.channel_id || '',
        bot_mode: slackConfig.bot_mode || false,
        // Routing is edited from the chips, never from this screen -- always
        // carry the last-loaded map so a credentials save can't drop routes.
        channel_routing: slackOriginal.channel_routing || {},
      }
      await agentApi.updateSlackFeedbackConfig(request)
      setSlackSuccess('Saved successfully!')
      await loadSlack()
      setTimeout(() => setSlackSuccess(null), 3000)
    } catch (err) {
      setSlackError(err instanceof Error ? err.message : 'Failed to save Slack configuration')
    } finally { setSlackSaving(false) }
  }

  const pollForTestReply = (testId: string) => {
    let attempts = 0
    const poll = async () => {
      if (attempts >= 60) { setPollingForReply(false); return }
      try {
        const reply = await agentApi.getTestConnectionReply(testId)
        if (reply?.received) { setTestReply(reply.reply); setPollingForReply(false); return }
      } catch { /* ignore 204 */ }
      attempts++
      setTimeout(poll, 1000)
    }
    poll()
  }

  const handleSlackTest = async () => {
    try {
      setSlackTesting(true)
      setSlackError(null)
      setTestResult(null)
      setTestReply(null)
      setPollingForReply(false)
      // Test against the saved workspace config, not whatever is typed in the form.
      const result = await agentApi.testSlackConnection()
      setTestResult(result)
      if (result.success && result.test_id) { setPollingForReply(true); pollForTestReply(result.test_id) }
    } catch (err) {
      setTestResult({ success: false, message: err instanceof Error ? err.message : 'Connection test failed' })
    } finally { setSlackTesting(false) }
  }

  // ── WhatsApp handlers (setup screen) ──────────────────────────────────────
  // Drops the paired phone, restarts the service, and refreshes local status.
  // Two-step confirmation prevents accidental clicks.
  const handleUnpairWhatsApp = async () => {
    if (!unpairConfirm) {
      setUnpairConfirm(true)
      setTimeout(() => setUnpairConfirm(false), 5000)
      return
    }
    try {
      setUnpairing(true)
      setWaError(null)
      await agentApi.unpairWhatsApp()
      setUnpairConfirm(false)
      await loadWaStatus()
      setQrBust(Date.now())
    } catch (err) {
      setWaError(err instanceof Error ? err.message : 'Failed to unpair')
    } finally {
      setUnpairing(false)
    }
  }

  // ── Gmail derived state + save/test ───────────────────────────────────────
  const gmailCurrentBlocked = normalizeGmailEmails(gmailBlockedText)
  const gmailDefaultRecipients = normalizeGmailEmails(gmailConfig.default_to)
  const gmailBlockedDefaults = gmailDefaultRecipients.filter(email => gmailCurrentBlocked.includes(email))
  const gmailDefaultIsBlocked = gmailBlockedDefaults.length > 0
  const gmailTestPassed = gmailTestResult?.success === true
    && gmailTestedTo === (gmailConfig.default_to || '')
    && !gmailDefaultIsBlocked
  const gmailHasChanges = gmailConfig.enabled !== gmailOriginal.enabled
    || (gmailConfig.default_to || '') !== gmailOriginal.default_to
    || JSON.stringify(gmailCurrentBlocked) !== JSON.stringify(gmailOriginal.blocked_recipients)

  const saveGmail = async () => {
    try {
      setGmailSaving(true)
      setGmailError(null)
      setGmailSuccess(null)
      const request: GmailConfigRequest = {
        enabled: gmailConfig.enabled,
        default_to: gmailConfig.default_to || '',
        blocked_recipients: gmailCurrentBlocked,
      }
      const data = await agentApi.updateGmailFeedbackConfig(request)
      const blocked = normalizeGmailEmails(data.blocked_recipients)
      setGmailConfig({ ...data, blocked_recipients: blocked })
      setGmailBlockedText(blocked.join(', '))
      setGmailOriginal({ enabled: data.enabled, default_to: data.default_to || '', blocked_recipients: blocked })
      setGmailSuccess('Gmail notification settings saved.')
    } catch (error) {
      setGmailError(error instanceof Error ? error.message : 'Failed to save Gmail configuration')
    } finally {
      setGmailSaving(false)
    }
  }

  const testGmail = async () => {
    try {
      setGmailTesting(true)
      setGmailError(null)
      setGmailTestResult(null)
      const request: GmailConfigRequest = {
        enabled: gmailConfig.enabled,
        default_to: gmailConfig.default_to || '',
        blocked_recipients: gmailCurrentBlocked,
      }
      const result = await agentApi.testGmailConnection(request)
      setGmailTestResult(result)
      setGmailTestedTo(result.success ? (gmailConfig.default_to || '') : null)
    } catch (error) {
      setGmailTestResult({ success: false, message: error instanceof Error ? error.message : 'Test failed' })
      setGmailTestedTo(null)
    } finally {
      setGmailTesting(false)
    }
  }

  // ── Connection status per channel ─────────────────────────────────────────
  // Slack routing only works with the app enabled AND bot mode on -- both are
  // required before a channel can be added.
  const slackReady = slackOriginal.enabled && !!slackOriginal.bot_mode
  const slackStatusLabel = slackLoading ? 'Loading…'
    : !slackOriginal.enabled ? 'Not connected'
      : !slackOriginal.bot_mode ? 'Bot mode off'
        : 'Connected'

  const waReady = !!waStatus?.enabled && !!waStatus.paired
  const waStatusLabel = !waStatus ? (waError ? 'Unavailable' : 'Loading…')
    : !waStatus.enabled ? 'Disabled on server'
      : !waStatus.paired ? 'Not paired'
        : waStatus.connected ? 'Connected' : 'Paired, offline'

  const slackHasChanges = JSON.stringify(slackConfig) !== JSON.stringify(slackOriginal)

  // ── Drill-in: Slack setup ─────────────────────────────────────────────────

  const renderSlackSetup = () => (
    <div className="space-y-4">
      {slackLoading ? (
        <div className="flex items-center justify-center py-12"><Loader2 className="w-8 h-8 animate-spin text-primary" /></div>
      ) : (
        <>
          {slackError && (
            <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg flex items-start gap-2">
              <AlertCircle className="w-4 h-4 text-red-600 dark:text-red-400 flex-shrink-0 mt-0.5" />
              <p className="text-sm text-red-700 dark:text-red-300">{slackError}</p>
            </div>
          )}
          {slackSuccess && (
            <div className="p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg flex items-start gap-2">
              <CheckCircle className="w-4 h-4 text-green-600 dark:text-green-400 flex-shrink-0 mt-0.5" />
              <p className="text-sm text-green-700 dark:text-green-300">{slackSuccess}</p>
            </div>
          )}

          {/* Enable Slack */}
          <Card className="p-4">
            <div className="flex items-center justify-between">
              <div>
                <h3 className="text-sm font-medium text-foreground">Enable Slack app</h3>
                <p className="text-xs text-muted-foreground mt-0.5">Connect the two-way Slack bot for @mentions, threads, and human replies</p>
              </div>
              <label className="relative inline-flex items-center cursor-pointer">
                <input type="checkbox" checked={slackConfig.enabled} disabled={readOnly} onChange={e => setSlackConfig({ ...slackConfig, enabled: e.target.checked })} className="sr-only peer" />
                <div className={toggleClass}></div>
              </label>
            </div>
          </Card>

          {/* Allowed Emails */}
          <Card className="p-4">
            <div className="flex flex-col gap-1.5">
              <div className="flex items-center justify-between">
                <label className="text-sm font-medium text-foreground">Allowed Users</label>
                <button
                  onClick={handleEmailsSave}
                  disabled={readOnly || emailsSaving || (!emailsDirty && !emailsSaved)}
                  title={readOnly ? READ_ONLY_TITLE : undefined}
                  className={`px-3 py-1 text-xs rounded-md transition-colors flex items-center gap-1 ${
                    emailsSaved ? 'bg-green-600 text-white' : 'bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50'
                  }`}
                >
                  {emailsSaving ? 'Saving...' : emailsSaved ? <><CheckCircle className="w-3 h-3" /> Saved</> : 'Save'}
                </button>
              </div>
              <input
                type="text"
                value={allowedEmails}
                onChange={e => { setAllowedEmails(e.target.value); setEmailsDirty(true); setEmailsSaved(false) }}
                disabled={readOnly}
                placeholder="user@example.com, user2@example.com"
                className="w-full px-2.5 py-1.5 text-xs bg-secondary border border-border rounded focus:outline-none focus:ring-1 focus:ring-primary"
              />
              <span className="text-[10px] text-muted-foreground">Comma-separated email addresses. Leave empty to allow everyone.</span>
            </div>
          </Card>

          {slackConfig.enabled && (
            <>
              {/* Bot Mode */}
              <Card className="p-4">
                <div className="flex items-center justify-between">
                  <div>
                    <h3 className="text-sm font-medium text-foreground">Bot Mode (@mention)</h3>
                    <p className="text-xs text-muted-foreground mt-0.5">Users can @mention the bot to start agent sessions directly from Slack. Required for channel routing.</p>
                  </div>
                  <label className="relative inline-flex items-center cursor-pointer">
                    <input type="checkbox" checked={slackConfig.bot_mode || false} disabled={readOnly} onChange={e => setSlackConfig({ ...slackConfig, bot_mode: e.target.checked })} className="sr-only peer" />
                    <div className={toggleClass}></div>
                  </label>
                </div>
              </Card>

              <Card className="p-4 bg-blue-50 dark:bg-blue-900/20 border-blue-300 dark:border-blue-700">
                <details>
                  <summary className="cursor-pointer text-sm font-semibold text-blue-800 dark:text-blue-200 select-none">
                    First time? Click for step-by-step setup instructions
                  </summary>
                  <div className="mt-3 text-xs text-blue-900 dark:text-blue-100 space-y-3">
                    <div>
                      <p className="font-semibold">1. Create a Slack App</p>
                      <p className="mt-1">Go to <a href="https://api.slack.com/apps" target="_blank" rel="noreferrer" className="underline">api.slack.com/apps</a> → <b>Create New App</b> → <b>From scratch</b>. Pick a name and your workspace.</p>
                    </div>
                    <div>
                      <p className="font-semibold">2. Add Bot Token Scopes</p>
                      <p className="mt-1">In the sidebar: <b>OAuth &amp; Permissions</b> → <b>Scopes</b> → <b>Bot Token Scopes</b>. Add at minimum:</p>
                      <ul className="mt-1 ml-4 list-disc space-y-0.5">
                        <li><code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">app_mentions:read</code></li>
                        <li><code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">channels:history</code>, <code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">groups:history</code></li>
                        <li><code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">chat:write</code>, <code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">chat:write.public</code></li>
                        <li><code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">reactions:write</code> (for the hourglass "bot is working" indicator)</li>
                        <li><code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">users:read</code>, <code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">users:read.email</code> (required for per-user memory)</li>
                        <li><code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">files:read</code>, <code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">files:write</code> (optional, for attachments)</li>
                      </ul>
                    </div>
                    <div>
                      <p className="font-semibold">3. Enable Socket Mode &amp; generate App Token</p>
                      <p className="mt-1"><b>Socket Mode</b> (sidebar) → toggle <b>Enable Socket Mode</b> ON. It will prompt you to create an <b>App-Level Token</b> with the <code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">connections:write</code> scope. Copy the token — it starts with <code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">xapp-</code>. This is your <b>App Token</b> below.</p>
                    </div>
                    <div>
                      <p className="font-semibold">4. Enable Event Subscriptions</p>
                      <p className="mt-1"><b>Event Subscriptions</b> (sidebar) → toggle <b>Enable Events</b> ON. Under <b>Subscribe to bot events</b>, add: <code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">app_mention</code>, <code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">message.channels</code>, <code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">message.groups</code>. Save changes.</p>
                    </div>
                    <div>
                      <p className="font-semibold">5. Install to workspace &amp; copy Bot Token</p>
                      <p className="mt-1"><b>Install App</b> (sidebar) → <b>Install to Workspace</b> → approve. After install, go back to <b>OAuth &amp; Permissions</b> — the <b>Bot User OAuth Token</b> now appears at the top. Copy it — it starts with <code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">xoxb-</code>. This is your <b>Bot Token</b> below.</p>
                    </div>
                    <div>
                      <p className="font-semibold">6. Invite the bot &amp; get Channel ID</p>
                      <p className="mt-1">In Slack, invite the bot to a channel: <code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">/invite @YourBot</code>. Then right-click the channel → <b>View channel details</b> → scroll to the bottom — the <b>Channel ID</b> starts with <code className="bg-blue-100 dark:bg-blue-800/40 px-1 rounded font-mono">C</code>.</p>
                    </div>
                    <p className="pt-1 italic opacity-80">If you re-add scopes or events later, you must re-install the app for changes to take effect.</p>
                  </div>
                </details>
              </Card>

              <Card className="p-4 bg-amber-50 dark:bg-amber-900/20 border-amber-300 dark:border-amber-700">
                <div className="flex items-start gap-2">
                  <AlertTriangle className="w-4 h-4 text-amber-600 dark:text-amber-400 flex-shrink-0 mt-0.5" />
                  <div>
                    <p className="text-sm font-semibold text-amber-800 dark:text-amber-200">Required: Event Subscriptions</p>
                    <p className="text-xs text-amber-700 dark:text-amber-300 mt-1">
                      Enable Event Subscriptions in your Slack App and subscribe to: <code className="bg-amber-100 dark:bg-amber-800/40 px-1 rounded font-mono">app_mention</code>, <code className="bg-amber-100 dark:bg-amber-800/40 px-1 rounded font-mono">message.channels</code>, <code className="bg-amber-100 dark:bg-amber-800/40 px-1 rounded font-mono">message.groups</code>
                    </p>
                  </div>
                </div>
              </Card>

              <div className="space-y-3">
                {/* Bot Token */}
                <Card className="p-4">
                  <label className="block text-sm font-medium text-foreground mb-2">Bot Token <span className="text-red-500">*</span></label>
                  <div className="relative">
                    <input type={showBotToken ? 'text' : 'password'} value={slackConfig.bot_token || ''} onChange={e => setSlackConfig({ ...slackConfig, bot_token: e.target.value })} disabled={readOnly} placeholder="xoxb-..." className="w-full px-3 py-2 pr-10 border border-border rounded-md bg-background text-foreground focus:outline-none focus:ring-2 focus:ring-primary" />
                    <button type="button" onClick={() => setShowBotToken(!showBotToken)} className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                      {showBotToken ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                    </button>
                  </div>
                  <p className="text-xs text-muted-foreground mt-1">OAuth & Permissions → Bot User OAuth Token (starts with xoxb-)</p>
                </Card>

                {/* Channel ID */}
                <Card className="p-4">
                  <label className="block text-sm font-medium text-foreground mb-2">Channel ID <span className="text-red-500">*</span></label>
                  <input type="text" value={slackConfig.channel_id || ''} onChange={e => setSlackConfig({ ...slackConfig, channel_id: e.target.value })} disabled={readOnly} placeholder="C1234567890" className="w-full px-3 py-2 border border-border rounded-md bg-background text-foreground focus:outline-none focus:ring-2 focus:ring-primary" />
                  <p className="text-xs text-muted-foreground mt-1">Right-click channel → View channel details → Channel ID (starts with C)</p>
                </Card>

                {/* App Token */}
                <Card className="p-4">
                  <label className="block text-sm font-medium text-foreground mb-2">App Token (Socket Mode) <span className="text-red-500">*</span></label>
                  <div className="relative">
                    <input type={showAppToken ? 'text' : 'password'} value={slackConfig.app_token || ''} onChange={e => setSlackConfig({ ...slackConfig, app_token: e.target.value })} disabled={readOnly} placeholder="xapp-..." className="w-full px-3 py-2 pr-10 border border-border rounded-md bg-background text-foreground focus:outline-none focus:ring-2 focus:ring-primary" />
                    <button type="button" onClick={() => setShowAppToken(!showAppToken)} className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground">
                      {showAppToken ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                    </button>
                  </div>
                  <p className="text-xs text-muted-foreground mt-1">Basic Information → App-Level Tokens → Generate with <code className="bg-secondary px-1 rounded font-mono">connections:write</code> scope (starts with xapp-)</p>
                </Card>

                {/* Test Connection */}
                <div className="space-y-1">
                  <Button variant="outline" onClick={handleSlackTest} disabled={readOnly || !slackConfig.enabled || slackTesting || slackLoading} title={readOnly ? READ_ONLY_TITLE : undefined} className="w-full flex items-center justify-center gap-2">
                    {slackTesting ? <><Loader2 className="w-4 h-4 animate-spin" />Testing...</> : 'Test Connection'}
                  </Button>
                  <p className="text-xs text-muted-foreground text-center">
                    Tests the <b>saved</b> config from the workspace — click <b>Save</b> first if you've edited any field above.
                  </p>
                </div>

                {testResult && (
                  <div className={`p-3 border rounded-lg flex items-start gap-2 ${testResult.success ? 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800' : 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800'}`}>
                    {testResult.success ? <CheckCircle className="w-4 h-4 text-green-600 dark:text-green-400 flex-shrink-0 mt-0.5" /> : <AlertCircle className="w-4 h-4 text-red-600 dark:text-red-400 flex-shrink-0 mt-0.5" />}
                    <p className={`text-sm ${testResult.success ? 'text-green-700 dark:text-green-300' : 'text-red-700 dark:text-red-300'}`}>{testResult.message}</p>
                  </div>
                )}
                {testResult?.success && pollingForReply && !testReply && (
                  <div className="p-3 bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg flex items-center gap-2">
                    <Loader2 className="w-4 h-4 animate-spin text-blue-600 dark:text-blue-400" />
                    <p className="text-sm text-blue-800 dark:text-blue-200">Waiting for reply in Slack thread...</p>
                  </div>
                )}
                {testReply && (
                  <div className="p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg">
                    <p className="text-sm font-medium text-green-800 dark:text-green-200">Reply received: {testReply}</p>
                  </div>
                )}
              </div>
            </>
          )}

          <div className="flex items-center justify-end gap-2 border-t border-border pt-3">
            <Button onClick={handleSlackSave} disabled={readOnly || !slackHasChanges || slackSaving || slackLoading} title={readOnly ? READ_ONLY_TITLE : undefined} className="flex items-center gap-2">
              {slackSaving ? <><Loader2 className="w-4 h-4 animate-spin" />Saving...</> : <><CheckCircle className="w-4 h-4" />Save</>}
            </Button>
          </div>
        </>
      )}
    </div>
  )

  // ── Drill-in: WhatsApp setup ──────────────────────────────────────────────

  const renderWhatsAppSetup = () => (
    <div className="space-y-4">
      {waError && (
        <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg flex items-start gap-2">
          <AlertCircle className="w-4 h-4 text-red-600 dark:text-red-400 flex-shrink-0 mt-0.5" />
          <p className="text-sm text-red-700 dark:text-red-300">{waError}</p>
        </div>
      )}

      {/* Connector disabled at server startup */}
      {waStatus && !waStatus.enabled && (
        <Card className="p-4">
          <div className="flex items-start gap-3">
            <AlertTriangle className="w-5 h-5 text-amber-500 flex-shrink-0 mt-0.5" />
            <div className="space-y-1">
              <h3 className="text-sm font-medium text-foreground">WhatsApp connector is disabled</h3>
              <p className="text-xs text-muted-foreground">
                Remove <code className="px-1 py-0.5 bg-muted rounded">WHATSAPP_ENABLED=false</code> from
                the server's <code className="px-1 py-0.5 bg-muted rounded">.env</code> and restart the
                agent. The connector is enabled by default, and the per-user session directory can be
                overridden via <code className="px-1 py-0.5 bg-muted rounded">WHATSAPP_SESSION_DIR</code>.
              </p>
            </div>
          </div>
        </Card>
      )}

      {/* Status card */}
      {waStatus && waStatus.enabled && (
        <Card className="p-4">
          <div className="flex items-center justify-between">
            <div>
              <h3 className="text-sm font-medium text-foreground">Status</h3>
              <p className="text-xs text-muted-foreground mt-0.5">
                Uses the unofficial WhatsApp Web protocol (whatsmeow). Pair your personal number once
                by scanning the QR below. On Android: tap the ⋮ menu → Linked Devices → Link a device.
                On iPhone: Settings → Linked Devices → Link Device.
              </p>
            </div>
            <div className="flex flex-col items-end gap-0.5 text-xs">
              <span className="flex items-center gap-1.5">
                <span
                  className={`w-1.5 h-1.5 rounded-full ${
                    waStatus.connected ? 'bg-green-500' : waStatus.paired ? 'bg-amber-500' : 'bg-gray-400'
                  }`}
                />
                <span className="text-foreground">
                  {waStatus.connected ? 'Connected' : waStatus.paired ? 'Paired, offline' : 'Not paired'}
                </span>
              </span>
              {waStatus.own_jid && (
                <span className="text-muted-foreground font-mono text-[10px]">{waStatus.own_jid}</span>
              )}
              {(waStatus.owner_email || waStatus.owner_username || waStatus.owner_user_id) && (
                <span className="text-muted-foreground text-[10px]">
                  bound to{' '}
                  <span className="text-foreground">
                    {waStatus.owner_email || waStatus.owner_username || waStatus.owner_user_id}
                  </span>
                </span>
              )}
            </div>
          </div>
        </Card>
      )}

      {/* QR pairing card — shown while unpaired */}
      {waStatus && waStatus.enabled && !waStatus.paired && (
        <Card className="p-4">
          <div className="flex flex-col items-center gap-3">
            <h3 className="text-sm font-medium text-foreground">Scan to pair</h3>
            {waStatus.qr_available ? (
              <>
                {qrImageURL ? (
                  <img
                    src={qrImageURL}
                    alt="WhatsApp pairing QR"
                    width={256}
                    height={256}
                    className="rounded border border-border bg-white p-2"
                  />
                ) : (
                  <div className="flex h-64 w-64 items-center justify-center rounded border border-border bg-muted/30 p-4 text-center">
                    {qrLoading ? (
                      <div className="flex items-center gap-2 text-sm text-muted-foreground">
                        <Loader2 className="w-4 h-4 animate-spin" />
                        Loading QR…
                      </div>
                    ) : qrError ? (
                      <div className="flex flex-col items-center gap-2 text-sm text-red-700 dark:text-red-300">
                        <AlertCircle className="w-5 h-5" />
                        <span>{qrError}</span>
                      </div>
                    ) : (
                      <span className="text-sm text-muted-foreground">QR not available yet.</span>
                    )}
                  </div>
                )}
                <p className="text-xs text-muted-foreground text-center max-w-sm">
                  Open WhatsApp on your phone. <strong>Android</strong>: ⋮ menu → Linked Devices → Link a
                  device. <strong>iPhone</strong>: Settings → Linked Devices → Link Device. Then scan this
                  code. The QR rotates every few seconds; this page refreshes it automatically.
                </p>
              </>
            ) : (
              <div className="flex items-center gap-2 text-sm text-muted-foreground py-8">
                <Loader2 className="w-4 h-4 animate-spin" />
                Waiting for the server to generate a QR…
              </div>
            )}
          </div>
        </Card>
      )}

      {/* How-to-chat card — shown once paired. */}
      {waStatus && waStatus.enabled && waStatus.paired && (
        <Card className="p-4">
          <h3 className="text-sm font-medium text-foreground mb-1.5">How to chat</h3>
          <div className="space-y-1.5 text-xs text-muted-foreground">
            <p>
              Open WhatsApp → <strong>Message Yourself</strong> chat, or DM the paired WhatsApp number
              from another phone. First send <code>link {waStatus.link_code || '123456'}</code> from that
              chat to bind WhatsApp's current phone/LID identity. Then send messages normally. Start a
              message with <code>@slug</code> to route it to the workflow mapped for that slug.
            </p>
            {waStatus.link_code && (
              <p className="text-muted-foreground/80">
                Linked chats: {waStatus.bound_chat_count ?? 0}. Link code expires{' '}
                {waStatus.link_code_expires_at
                  ? new Date(waStatus.link_code_expires_at).toLocaleString()
                  : 'soon'}
                .
              </p>
            )}
            <p className="text-muted-foreground/80">
              For a proper separate-bot experience (like Slack's <code>@bot</code>), pair a dedicated
              WhatsApp number — a second SIM, WhatsApp Business with a different number, or a virtual
              number from Twilio. Only linked inbound DMs are handled as bot messages.
            </p>
          </div>
        </Card>
      )}

      {/* Unpair card — shown once paired */}
      {waStatus && waStatus.enabled && waStatus.paired && (
        <Card className="p-4">
          <div className="flex items-center justify-between gap-3">
            <div>
              <h3 className="text-sm font-medium text-foreground">Unpair</h3>
              <p className="text-xs text-muted-foreground mt-0.5">
                Drops the current device link and deletes the session file. You'll need to scan a new QR
                to pair again.
              </p>
            </div>
            <Button
              onClick={handleUnpairWhatsApp}
              disabled={readOnly || unpairing}
              title={readOnly ? READ_ONLY_TITLE : undefined}
              variant={unpairConfirm ? 'destructive' : 'outline'}
              size="sm"
              className="flex-shrink-0 whitespace-nowrap"
            >
              {unpairing ? (
                <><Loader2 className="w-3.5 h-3.5 animate-spin mr-1.5" /> Unpairing…</>
              ) : unpairConfirm ? (
                <><Trash2 className="w-3.5 h-3.5 mr-1.5" /> Confirm unpair</>
              ) : (
                <>Unpair</>
              )}
            </Button>
          </div>
        </Card>
      )}

      {/* Loading placeholder */}
      {!waStatus && !waError && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="w-8 h-8 animate-spin text-primary" />
        </div>
      )}
    </div>
  )

  if (setup !== null) {
    return (
      <div className="space-y-4">
        <button
          type="button"
          onClick={() => setSetup(null)}
          className="inline-flex items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to channels
        </button>
        <div className="flex items-center gap-2 text-sm font-semibold text-foreground">
          {setup === 'slack' ? <MessageSquare className="h-4 w-4" /> : <Phone className="h-4 w-4" />}
          {setup === 'slack' ? 'Slack' : 'WhatsApp'}
          <span className="text-xs font-normal text-muted-foreground">· shared by all workflows</span>
        </div>
        {setup === 'slack' ? renderSlackSetup() : renderWhatsAppSetup()}
      </div>
    )
  }

  // ── Main view ─────────────────────────────────────────────────────────────

  const renderChip = (route: WorkflowRoute) => {
    const id = routeId(route)
    const expanded = expandedChip === id
    const saving = routeSaving === id
    const label = route.kind === 'slack' ? `Slack #${route.key}` : `WhatsApp @${route.key}`
    const Icon = route.kind === 'slack' ? MessageSquare : Phone
    return (
      <div key={id} className="min-w-0">
        <span className="inline-flex max-w-full items-center gap-1.5 rounded-full border border-primary/30 bg-primary/10 py-1 pl-2 pr-1 text-xs font-medium text-primary">
          <button
            type="button"
            onClick={() => setExpandedChip(expanded ? null : id)}
            className="inline-flex min-w-0 items-center gap-1.5"
            aria-expanded={expanded}
            title="Edit route options"
          >
            <Icon className="h-3 w-3 shrink-0" />
            <span className="truncate font-mono">{label}</span>
            <ChevronDown className={`h-3 w-3 shrink-0 transition-transform ${expanded ? 'rotate-180' : ''}`} />
          </button>
          {saving ? (
            <Loader2 className="h-3 w-3 animate-spin" />
          ) : (
            <button
              type="button"
              onClick={() => void removeRoute(route)}
              disabled={readOnly}
              className="rounded-full p-0.5 text-primary/70 transition-colors hover:bg-red-500/15 hover:text-red-500 disabled:cursor-not-allowed disabled:opacity-50 disabled:hover:bg-transparent disabled:hover:text-primary/70"
              aria-label={`Stop answering on ${label}`}
              title={readOnly ? READ_ONLY_TITLE : 'Remove from this workflow'}
            >
              <X className="h-3 w-3" />
            </button>
          )}
        </span>
        {expanded && (
          <div className="mt-1.5 flex flex-wrap items-center gap-3 rounded-md border border-border bg-muted/20 px-3 py-2 text-xs">
            <label className="flex items-center gap-1.5 text-muted-foreground">
              Mode
              <select
                value={route.kind === 'slack' ? (route.workshop_mode || '') : 'run'}
                onChange={e => void updateRoute(route, { workshop_mode: e.target.value })}
                disabled={readOnly || saving || route.kind === 'whatsapp'}
                className="px-1.5 py-1 text-xs bg-secondary border border-border rounded focus:outline-none focus:ring-1 focus:ring-primary disabled:opacity-60"
                title="Bot channels always run in Run mode. 'Default' uses the automation manifest's setting (which is also Run for bot deployments)."
              >
                {route.kind === 'slack' && <option value="">Default</option>}
                <option value="run">Run</option>
              </select>
            </label>
            <label className="flex items-center gap-1.5 text-muted-foreground" title="Send detailed automation step/runtime messages to this channel">
              <input
                type="checkbox"
                checked={!!route.send_full_details}
                disabled={readOnly || saving}
                onChange={e => void updateRoute(route, { send_full_details: e.target.checked })}
                className="h-3.5 w-3.5"
              />
              Send full details
            </label>
          </div>
        )}
      </div>
    )
  }

  const renderChannelRow = (kind: ChannelKind) => {
    const ready = kind === 'slack' ? slackReady : waReady
    const statusLabel = kind === 'slack' ? slackStatusLabel : waStatusLabel
    const loading = kind === 'slack' ? slackLoading : (!waStatus && !waError)
    const Icon = kind === 'slack' ? MessageSquare : Phone
    const name = kind === 'slack' ? 'Slack' : 'WhatsApp'
    const value = kind === 'slack' ? newSlackChannel : newWaSlug
    const setValue = kind === 'slack' ? setNewSlackChannel : setNewWaSlug
    const add = kind === 'slack' ? addSlackRoute : addWaRoute
    const adding = routeSaving?.startsWith(`${kind}:`) && !myRoutes.some(r => routeId(r) === routeSaving)
    const error = addError[kind]
    return (
      <div key={kind} className="px-3 py-2.5">
        <div className="flex items-center gap-2">
          <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
          <span className="text-sm font-medium text-foreground">{name}</span>
          <span
            className={`inline-flex items-center gap-1 text-[11px] font-medium ${ready ? 'text-emerald-600 dark:text-emerald-400' : loading ? 'text-muted-foreground' : 'text-amber-600 dark:text-amber-400'}`}
            title={kind === 'slack' && slackOriginal.enabled && !slackOriginal.bot_mode ? 'Turn on Bot Mode in Set up before routing channels here.' : undefined}
          >
            <span className={`h-1.5 w-1.5 rounded-full ${ready ? 'bg-emerald-500' : loading ? 'bg-muted-foreground/40' : 'bg-amber-500'}`} />
            {statusLabel}
          </span>
          <span className="flex-1" />
          <button
            type="button"
            onClick={() => setSetup(kind)}
            className="inline-flex items-center gap-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground"
            title={`Connect or configure ${name}`}
          >
            {ready ? 'Settings' : 'Set up'}
            <ChevronRight className="h-3.5 w-3.5" />
          </button>
        </div>
        {ready && workflowId && (
          <div className="mt-2 flex items-center gap-2">
            {kind === 'whatsapp' && <span className="text-xs text-muted-foreground select-none">@</span>}
            <input
              type="text"
              value={value}
              onChange={e => {
                setValue(kind === 'whatsapp' ? e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, '') : e.target.value)
                if (error) setAddError(prev => ({ ...prev, [kind]: undefined }))
              }}
              onKeyDown={e => { if (e.key === 'Enter') add() }}
              placeholder={kind === 'slack' ? 'Channel ID (C…)' : 'slug, e.g. rca'}
              disabled={readOnly || !!adding}
              title={readOnly ? READ_ONLY_TITLE : undefined}
              className="min-w-0 flex-1 px-2 py-1 text-xs bg-secondary border border-border rounded font-mono focus:outline-none focus:ring-1 focus:ring-primary disabled:opacity-50"
            />
            <button
              type="button"
              onClick={add}
              disabled={readOnly || !value.trim() || !!adding}
              title={readOnly ? READ_ONLY_TITLE : undefined}
              className="inline-flex items-center gap-1 rounded-md border border-border px-2 py-1 text-xs font-medium text-foreground transition-colors hover:bg-muted disabled:cursor-not-allowed disabled:opacity-50"
            >
              {adding ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Plus className="h-3.5 w-3.5" />}
              Add
            </button>
          </div>
        )}
        {error && <p className="mt-1.5 text-xs text-red-600 dark:text-red-400">{error}</p>}
      </div>
    )
  }

  const renderGmail = () => (
    <div className="rounded-md border border-border">
      <button
        type="button"
        onClick={() => setGmailOpen(open => !open)}
        aria-expanded={gmailOpen}
        className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm font-medium text-foreground transition-colors hover:bg-muted/50"
      >
        <ChevronRight className={`h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform ${gmailOpen ? 'rotate-90' : ''}`} />
        <Mail className="h-4 w-4 shrink-0 text-muted-foreground" />
        Email notifications
        <span className="font-normal text-muted-foreground">— shared by all workflows</span>
        <span className="flex-1" />
        <span className={`inline-flex items-center gap-1 text-[11px] font-medium ${gmailConfig.enabled ? 'text-emerald-600 dark:text-emerald-400' : 'text-muted-foreground'}`}>
          <span className={`h-1.5 w-1.5 rounded-full ${gmailConfig.enabled ? 'bg-emerald-500' : 'bg-muted-foreground/40'}`} />
          {gmailConfig.enabled ? 'On' : 'Off'}
        </span>
      </button>
      {gmailOpen && (
        <div className="space-y-4 border-t border-border p-3">
          {gmailLoading ? (
            <div className="flex items-center justify-center py-12"><Loader2 className="w-8 h-8 animate-spin text-primary" /></div>
          ) : (
            <>
              <p className="text-xs text-muted-foreground">Account-wide one-way email delivery, shared by <code>notify_user</code> across every workflow and product chat. Turn this off to stop all outbound email. Email replies do not resume an agent.</p>
              {gmailError && (
                <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg flex items-start gap-2">
                  <AlertCircle className="w-4 h-4 text-red-600 dark:text-red-400 flex-shrink-0 mt-0.5" />
                  <p className="text-sm text-red-700 dark:text-red-300">{gmailError}</p>
                </div>
              )}
              {gmailSuccess && (
                <div className="p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg flex items-start gap-2">
                  <CheckCircle className="w-4 h-4 text-green-600 dark:text-green-400 flex-shrink-0 mt-0.5" />
                  <p className="text-sm text-green-700 dark:text-green-300">{gmailSuccess}</p>
                </div>
              )}
              <Card className="p-4">
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <h4 className="text-sm font-medium">Enable Gmail</h4>
                    <p className="mt-0.5 text-xs text-muted-foreground">Available to notify_user across workflows and product chats.</p>
                  </div>
                  <label className={`relative inline-flex items-center ${!gmailConfig.enabled && !gmailTestPassed ? 'cursor-not-allowed' : 'cursor-pointer'}`}>
                    <input type="checkbox" checked={gmailConfig.enabled} disabled={readOnly || (!gmailConfig.enabled && !gmailTestPassed)} onChange={event => setGmailConfig({ ...gmailConfig, enabled: event.target.checked })} className="peer sr-only" />
                    <div className="h-6 w-11 rounded-full bg-gray-200 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:border after:border-gray-300 after:bg-white after:transition-all after:content-[''] peer-checked:bg-blue-600 peer-checked:after:translate-x-full peer-checked:after:border-white peer-disabled:opacity-40 dark:bg-gray-700" />
                  </label>
                </div>
                {!gmailConfig.enabled && !gmailTestPassed && <p className="mt-2 text-xs text-amber-600 dark:text-amber-400">Send a successful test email before enabling.</p>}
              </Card>
              <Card className="p-4">
                <div className="flex items-center justify-between gap-2">
                  <div>
                    <h4 className="text-sm font-medium">Connection</h4>
                    <p className="mt-0.5 text-xs text-muted-foreground">Google Workspace CLI on the server host.</p>
                  </div>
                  <div className="flex items-center gap-2 text-xs">
                    {gmailChecking ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <span className={`h-2 w-2 rounded-full ${gmailConfig.auth.authenticated && gmailConfig.auth.has_gmail_scope ? 'bg-green-500' : 'bg-amber-500'}`} />}
                    <span>{!gmailConfig.auth.gws_installed ? 'gws not installed' : !gmailConfig.auth.authenticated ? 'Not connected' : !gmailConfig.auth.has_gmail_scope ? 'Missing Gmail scope' : 'Connected'}</span>
                    <button onClick={() => loadGmail(true)} disabled={gmailChecking} className="rounded p-1 text-muted-foreground hover:text-foreground" aria-label="Refresh Gmail connection"><RotateCcw className="h-3.5 w-3.5" /></button>
                  </div>
                </div>
              </Card>
              {!(gmailConfig.auth.authenticated && gmailConfig.auth.has_gmail_scope) && (
                <Card className="border-amber-300 bg-amber-50 p-4 text-xs text-amber-900 dark:border-amber-700 dark:bg-amber-900/20 dark:text-amber-100">
                  <div className="flex gap-2"><AlertTriangle className="h-4 w-4 flex-shrink-0" /><div><strong>Setup on the server host:</strong> install <code>@googleworkspace/cli</code>, then run <code>gws auth login -s gmail</code> and refresh this status.</div></div>
                </Card>
              )}
              <Card className="space-y-3 p-4">
                <div>
                  <label className="mb-2 block text-sm font-medium">Default recipients</label>
                  {/* Deliberately type="text": type="email" rejects a comma-separated
                      list, which is the whole point of this field. */}
                  <input type="text" inputMode="email" value={gmailConfig.default_to || ''} onChange={event => setGmailConfig({ ...gmailConfig, default_to: event.target.value })} disabled={readOnly} placeholder="you@example.com, teammate@example.com" className="w-full rounded-md border border-border bg-background px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-primary" />
                  <p className="mt-1 text-xs text-muted-foreground">Where notifications are emailed when a workflow has no recipients of its own. Separate several addresses with commas.</p>
                </div>
                <div>
                  <label className="mb-2 block text-sm font-medium">Disallowed recipients</label>
                  <textarea value={gmailBlockedText} onChange={event => setGmailBlockedText(event.target.value)} disabled={readOnly} rows={3} placeholder="blocked@example.com, no-notify@example.com" className="w-full resize-y rounded-md border border-border bg-background px-3 py-2 font-mono text-sm focus:outline-none focus:ring-2 focus:ring-primary" />
                  {gmailDefaultIsBlocked && <p className="mt-1 text-xs text-red-600 dark:text-red-400">{gmailBlockedDefaults.join(', ')} {gmailBlockedDefaults.length === 1 ? 'is' : 'are'} both a default recipient and disallowed.</p>}
                </div>
              </Card>
              <Button variant="outline" onClick={testGmail} disabled={readOnly || gmailTesting || !gmailConfig.default_to || gmailDefaultIsBlocked} title={readOnly ? READ_ONLY_TITLE : undefined} className="w-full">{gmailTesting ? <><Loader2 className="mr-2 h-4 w-4 animate-spin" />Sending…</> : 'Send test email'}</Button>
              {gmailTestResult && <Card className={`p-3 text-sm ${gmailTestResult.success ? 'border-green-300 bg-green-50 text-green-700 dark:border-green-700 dark:bg-green-900/20 dark:text-green-300' : 'border-red-300 bg-red-50 text-red-700 dark:border-red-700 dark:bg-red-900/20 dark:text-red-300'}`}>{gmailTestResult.message}</Card>}
              <div className="flex justify-end">
                <Button onClick={saveGmail} disabled={readOnly || !gmailHasChanges || gmailSaving || gmailLoading || gmailDefaultIsBlocked || (gmailConfig.enabled && !gmailTestPassed)} title={readOnly ? READ_ONLY_TITLE : undefined}>{gmailSaving ? <><Loader2 className="mr-2 h-4 w-4 animate-spin" />Saving…</> : 'Save'}</Button>
              </div>
            </>
          )}
        </div>
      )}
    </div>
  )

  return (
    <div className="space-y-4">
      {/* This workflow answers on */}
      <div className="rounded-lg border border-border bg-muted/20 p-3">
        <div className="mb-1.5 text-sm font-medium text-muted-foreground">This workflow answers on</div>
        {!workflowId ? (
          <p className="text-xs text-muted-foreground">This panel needs an active workflow folder.</p>
        ) : myRoutes.length === 0 ? (
          <p className="text-xs text-muted-foreground">No channels yet — add one below.</p>
        ) : (
          <div className="flex flex-wrap gap-1.5">{myRoutes.map(renderChip)}</div>
        )}
        {routeError && (
          <p className="mt-2 flex items-start gap-1.5 text-xs text-red-600 dark:text-red-400">
            <AlertCircle className="mt-0.5 h-3.5 w-3.5 shrink-0" />{routeError}
          </p>
        )}
        {waRoutingError && (
          <p className="mt-2 flex items-start gap-1.5 text-xs text-amber-600 dark:text-amber-400">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />WhatsApp routing unavailable: {waRoutingError}
          </p>
        )}
      </div>

      {/* Add a channel */}
      <div>
        <div className="mb-1.5 text-sm font-medium text-muted-foreground">Add a channel</div>
        <div className="divide-y divide-border overflow-hidden rounded-md border border-border bg-background">
          {renderChannelRow('slack')}
          {renderChannelRow('whatsapp')}
        </div>
      </div>

      {renderGmail()}
    </div>
  )
}
