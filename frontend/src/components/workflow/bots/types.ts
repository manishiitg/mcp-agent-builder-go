import type { GmailConfigResponse } from '../../../services/api-types'

// ── Types ──────────────────────────────────────────────────────────────────

export type ChannelKind = 'slack' | 'whatsapp'

// Shape of GET/PUT /api/whatsapp/routing entries. Same idea as ChannelRoute
// but workshop_mode is an untyped string on this endpoint.
export type WaRoute = { workflow_id: string; workspace_path?: string; workshop_mode?: string; send_full_details?: boolean }

// One route this workflow answers on, regardless of platform.
export type WorkflowRoute = {
  kind: ChannelKind
  key: string // Slack channel ID or WhatsApp slug
  workshop_mode?: string
  send_full_details?: boolean
}

// Shape of GET /api/whatsapp/status. enabled = connector started at server
// startup; paired = device identity stored; connected = live WS.
export interface WhatsAppStatus {
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

export const SLUG_RE = /^[a-z0-9-]+$/

export const normalizeGmailEmails = (values: string | string[] | undefined): string[] => {
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

export const emptyGmailConfig: GmailConfigResponse = {
  enabled: false,
  default_to: '',
  auth: { gws_installed: false, authenticated: false, has_gmail_scope: false },
  ready: false,
}

export const toggleClass = "w-11 h-6 bg-gray-200 peer-focus:outline-none rounded-full peer dark:bg-gray-700 peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all dark:border-gray-600 peer-checked:bg-blue-600"

export const routeId = (route: { kind: ChannelKind; key: string }) => `${route.kind}:${route.key}`
