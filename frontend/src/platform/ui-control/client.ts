import { UI_CONTROL_CONTRACT } from './contract.generated'

export interface UISnapshot { view: string; revision: number; visible: boolean }
export interface UIAction {
  request_id: string
  view: string
  action: string
  target?: string
  expected_state_revision?: number
  expires_at: string
}

// A closed presentation adapter, deliberately NOT a click/selector API. Only
// first-party semantic markers are inspected, never text or credential values.
export function supportedAction(a: UIAction): boolean {
  const view = UI_CONTROL_CONTRACT.views.find(v => v.id === a.view)
  if (!view || !(view.actions as readonly string[]).includes(a.action)) return false
  return a.action === 'open' ? !a.target : (view.targets as readonly string[]).includes(a.target ?? '')
}

export function workspaceHost(workspace: string): HTMLElement | undefined {
  const hosts = Array.from(document.querySelectorAll<HTMLElement>('[data-ui-workspace]'))
    .filter(el => el.dataset.uiWorkspace === workspace)
  return hosts.find(el => el.getClientRects().length > 0 && el.querySelector('[data-ui-view-mounted]'))
    ?? hosts.find(el => el.getClientRects().length > 0)
    ?? hosts[0]
}

export async function applyUIAction(
  action: UIAction, workspace: string, open: (view: string) => void,
  state: () => UISnapshot, signal: AbortSignal,
): Promise<{ status: string; code: string }> {
  if (!supportedAction(action)) return { status: 'rejected', code: 'target_not_found' }
  if (signal.aborted || !workspaceHost(workspace)) return { status: 'cancelled', code: 'inactive_scope' }
  if (action.expected_state_revision !== undefined && action.expected_state_revision !== state().revision)
    return { status: 'rejected', code: 'stale_state' }
  const expires = Date.parse(action.expires_at)
  if (!Number.isFinite(expires) || expires <= Date.now()) return { status: 'failed', code: 'timeout' }
  open(action.view)
  // Observe actual mount/visibility, including lazy-loaded inspectors. Bounded;
  // listeners/timers are removed on every terminal outcome and on unmount.
  return new Promise(resolve => {
    let done = false
    let touched = false
    const finish = (status: string, code = '') => {
      if (done) return
      done = true
      clearTimeout(timer)
      observer.disconnect()
      signal.removeEventListener('abort', aborted)
      document.removeEventListener('pointerdown', interrupted, true)
      document.removeEventListener('keydown', interrupted, true)
      document.removeEventListener('wheel', interrupted, true)
      resolve({ status, code })
    }
    const aborted = () => finish('cancelled', 'inactive_scope')
    const interrupted = () => { touched = true; finish('cancelled', 'user_interrupted') }
    const check = () => {
      if (done || touched) return
      if (signal.aborted || document.visibilityState !== 'visible') return aborted()
      const host = workspaceHost(workspace)
      if (!host) return aborted()
      if (host.dataset.uiView !== action.view) return
      if (host.getClientRects().length === 0) return
      // The marker lives INSIDE Suspense, not in its loading fallback.
      if (!host.querySelector('[data-ui-view-mounted]')) return
      if (action.action === 'expand') {
        const details = Array.from(host.querySelectorAll<HTMLDetailsElement>('details[data-ui-instructions]'))
          .find(el => el.dataset.uiInstructions === action.target)
        if (!details) return
        details.open = true
        details.scrollIntoView({ block: 'nearest', behavior: 'instant' })
        if (!details.open || details.getClientRects().length === 0) return
      }
      finish('applied')
    }
    const observer = new MutationObserver(check)
    const timer = setTimeout(() => finish('failed', action.action === 'expand' ? 'target_not_found' : 'render_failed'), Math.min(8000, expires - Date.now()))
    observer.observe(document.body, { subtree: true, childList: true, attributes: true, attributeFilter: ['data-ui-view', 'open', 'class'] })
    signal.addEventListener('abort', aborted, { once: true })
    document.addEventListener('pointerdown', interrupted, true)
    document.addEventListener('keydown', interrupted, true)
    document.addEventListener('wheel', interrupted, { capture: true, passive: true })
    requestAnimationFrame(check)
  })
}
