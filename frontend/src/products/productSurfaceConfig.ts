export const PRODUCT_SURFACES = ['agentworks', 'video-studio', 'finance', 'dominion'] as const

export type ProductSurface = (typeof PRODUCT_SURFACES)[number]

type ProductRuntimeConfig = {
  defaultProductSurface?: unknown
  enabledProductSurfaces?: unknown
}

function runtimeConfig(): ProductRuntimeConfig | undefined {
  if (typeof window === 'undefined') return undefined
  return (window as Window & { __APP_RUNTIME_CONFIG__?: ProductRuntimeConfig }).__APP_RUNTIME_CONFIG__
}

function isProductSurface(value: unknown): value is ProductSurface {
  return typeof value === 'string' && PRODUCT_SURFACES.includes(value as ProductSurface)
}

/**
 * Returns the products intentionally exposed by this deployment.  Leaving the
 * runtime setting out preserves the full product suite for desktop and normal
 * AgentWorks installs; it is an allowlist only when explicitly configured.
 */
export function enabledProductSurfaces(): ProductSurface[] {
  const configured = runtimeConfig()?.enabledProductSurfaces
  if (!Array.isArray(configured)) return [...PRODUCT_SURFACES]

  const enabled = configured.filter(isProductSurface)
  return enabled.length > 0 ? [...new Set(enabled)] : [...PRODUCT_SURFACES]
}

export function deploymentDefaultProductSurface(): ProductSurface {
  const enabled = enabledProductSurfaces()
  const configuredDefault = runtimeConfig()?.defaultProductSurface
  return isProductSurface(configuredDefault) && enabled.includes(configuredDefault)
    ? configuredDefault
    : enabled[0]
}

export function isEnabledProductSurface(surface: ProductSurface): boolean {
  return enabledProductSurfaces().includes(surface)
}

export function isSingleProductDeployment(): boolean {
  return enabledProductSurfaces().length === 1
}
