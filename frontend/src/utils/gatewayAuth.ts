export const GATEWAY_LOGIN_HEADER = 'X-AgentWorks-Login'

export function gatewayLoginTarget(status: number | undefined, headerValue: unknown): string | null {
  if (status !== 401 || typeof headerValue !== 'string') return null

  const target = headerValue.trim()
  if (!target.startsWith('/') || target.startsWith('//')) return null
  return target
}

let redirectInProgress = false

export function isGatewayLoginPath(pathname: string): boolean {
  return pathname === '/login' || pathname.startsWith('/login/')
}

export function redirectToGatewayLogin(target: string | null): boolean {
  // API calls already in flight during logout can return after the login
  // screen has mounted. Do not redirect the login screen to itself; doing so
  // creates an ever-growing /login?next=/login?... URL.
  if (!target || typeof window === 'undefined' || redirectInProgress || isGatewayLoginPath(window.location.pathname)) return false
  redirectInProgress = true
  window.location.assign(target)
  return true
}
