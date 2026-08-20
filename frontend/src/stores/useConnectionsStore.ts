import { create } from 'zustand'
import {
  connectionsApi,
  ConnectionError,
  type CatalogEntry,
  type Connection,
  type ConnectionsSummary,
  type ConnectPayload,
  type FriendlyError,
  type TestResult,
} from '../services/connectionsApi'

/** How long to wait for the user to finish an OAuth approval before giving up. */
const OAUTH_POLL_INTERVAL_MS = 2000
const OAUTH_POLL_TIMEOUT_MS = 5 * 60 * 1000

interface ConnectionsState {
  catalog: CatalogEntry[]
  connections: Connection[]
  summary: ConnectionsSummary

  isLoadingCatalog: boolean
  isLoadingConnections: boolean
  /** Catalog/connection id currently mid-connect, so its card can show a spinner. */
  connectingId: string | null
  /** Catalog/connection id currently being tested. */
  testingId: string | null

  loadError: FriendlyError | null
  actionError: FriendlyError | null

  loadCatalog: () => Promise<void>
  loadConnections: () => Promise<void>
  refresh: () => Promise<void>

  connect: (id: string, payload?: ConnectPayload) => Promise<ConnectOutcome>
  disconnect: (id: string) => Promise<boolean>
  remove: (id: string) => Promise<boolean>
  test: (id: string) => Promise<TestResult | null>

  clearActionError: () => void
  getConnection: (id: string) => Connection | undefined
}

export type ConnectOutcome =
  | { kind: 'connected' }
  | { kind: 'needs_client_id'; scopes?: string[]; resource?: string }
  | { kind: 'failed'; error: FriendlyError }

const emptySummary: ConnectionsSummary = { connected: 0, needs_attention: 0, total: 0 }

function toFriendly(err: unknown): FriendlyError {
  if (err instanceof ConnectionError) return err.friendly
  return {
    code: 'unknown',
    title: 'Something went wrong',
    message: 'The action could not be completed.',
    action: 'retry',
    raw: String(err),
  }
}

/**
 * Waits for an OAuth approval to land. The callback is handled server-side, so
 * the only signal available to the UI is the connection turning healthy.
 */
async function waitForConnection(
  id: string,
  isStillConnecting: () => boolean
): Promise<boolean> {
  const deadline = Date.now() + OAUTH_POLL_TIMEOUT_MS

  while (Date.now() < deadline) {
    await new Promise(resolve => setTimeout(resolve, OAUTH_POLL_INTERVAL_MS))

    // The user closed the modal or started another connect — stop polling.
    if (!isStillConnecting()) return false

    try {
      const { connections } = await connectionsApi.list()
      const match = connections.find(c => c.id === id)
      if (match?.health === 'connected') return true
    } catch {
      // Transient failure while the user is still in the OAuth popup; keep waiting.
    }
  }
  return false
}

export const useConnectionsStore = create<ConnectionsState>((set, get) => ({
  catalog: [],
  connections: [],
  summary: emptySummary,

  isLoadingCatalog: false,
  isLoadingConnections: false,
  connectingId: null,
  testingId: null,

  loadError: null,
  actionError: null,

  loadCatalog: async () => {
    set({ isLoadingCatalog: true })
    try {
      const { integrations } = await connectionsApi.getCatalog()
      set({ catalog: integrations, isLoadingCatalog: false, loadError: null })
    } catch (err) {
      // A missing catalog must not hide already-connected integrations, so this
      // records the error without clearing the connections list.
      set({ isLoadingCatalog: false, loadError: toFriendly(err) })
    }
  },

  loadConnections: async () => {
    set({ isLoadingConnections: true })
    try {
      const { connections, summary } = await connectionsApi.list()
      set({ connections, summary, isLoadingConnections: false, loadError: null })
    } catch (err) {
      set({ isLoadingConnections: false, loadError: toFriendly(err) })
    }
  },

  refresh: async () => {
    await Promise.all([get().loadCatalog(), get().loadConnections()])
  },

  connect: async (id, payload = {}) => {
    set({ connectingId: id, actionError: null })
    try {
      const result = await connectionsApi.connect(id, payload)

      if (result.status === 'needs_client_id') {
        set({ connectingId: null })
        return {
          kind: 'needs_client_id',
          scopes: result.scopes_supported,
          resource: result.resource,
        }
      }

      if (result.status === 'connected') {
        set({ connectingId: null })
        await get().loadConnections()
        return { kind: 'connected' }
      }

      // OAuth: hand the user to the provider, then wait for the callback.
      if (result.auth_url) {
        window.open(result.auth_url, '_blank', 'noopener,noreferrer')
      }

      const ok = await waitForConnection(id, () => get().connectingId === id)
      set({ connectingId: null })
      await get().loadConnections()

      if (ok) return { kind: 'connected' }

      const error: FriendlyError = {
        code: 'not_completed',
        title: 'Authentication was not completed',
        message:
          'The sign-in window was closed or timed out before access was approved. Try connecting again.',
        action: 'retry',
      }
      set({ actionError: error })
      return { kind: 'failed', error }
    } catch (err) {
      const error = toFriendly(err)
      set({ connectingId: null, actionError: error })
      return { kind: 'failed', error }
    }
  },

  disconnect: async id => {
    set({ actionError: null })
    try {
      await connectionsApi.disconnect(id)
      await get().loadConnections()
      return true
    } catch (err) {
      set({ actionError: toFriendly(err) })
      return false
    }
  },

  remove: async id => {
    set({ actionError: null })
    try {
      await connectionsApi.remove(id)
      await get().loadConnections()
      return true
    } catch (err) {
      set({ actionError: toFriendly(err) })
      return false
    }
  },

  test: async id => {
    set({ testingId: id, actionError: null })
    try {
      const result = await connectionsApi.test(id)
      set({ testingId: null })
      return result
    } catch (err) {
      set({ testingId: null, actionError: toFriendly(err) })
      return null
    }
  },

  clearActionError: () => set({ actionError: null }),

  getConnection: id => get().connections.find(c => c.id === id),
}))
