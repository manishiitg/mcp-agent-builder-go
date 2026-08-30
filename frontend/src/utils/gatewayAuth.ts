export const GATEWAY_LOGIN_HEADER = 'X-AgentWorks-Login'

export function gatewayLoginTarget(status: number | undefined, headerValue: unknown): string | null {
  if (status !== 401 || typeof headerValue !== 'string') return null

  const target = headerValue.trim()
  if (!target.startsWith('/') || target.startsWith('//')) return null
  return target
}

let redirectInProgress = false

export function redirectToGatewayLogin(target: string | null): boolean {
  if (!target || typeof window === 'undefined' || redirectInProgress) return false
  redirectInProgress = true
  window.location.assign(target)
  return true
}
