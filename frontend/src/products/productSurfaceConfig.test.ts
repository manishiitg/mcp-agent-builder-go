import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  deploymentDefaultProductSurface,
  enabledProductSurfaces,
  intersectAllowedProductSurfaces,
  isEnabledProductSurface,
  isSingleProductDeployment,
} from './productSurfaceConfig'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('product surface deployment configuration', () => {
  it('keeps the complete product suite when no deployment allowlist is configured', () => {
    expect(enabledProductSurfaces()).toEqual(['agentworks', 'video-studio', 'finance', 'dominion'])
    expect(deploymentDefaultProductSurface()).toBe('agentworks')
    expect(isSingleProductDeployment()).toBe(false)
  })

  it('constrains the dedicated host to AgentWorks and Video Studio', () => {
    vi.stubGlobal('window', {
      __APP_RUNTIME_CONFIG__: {
        defaultProductSurface: 'video-studio',
        enabledProductSurfaces: ['agentworks', 'video-studio'],
      },
    })

    expect(enabledProductSurfaces()).toEqual(['agentworks', 'video-studio'])
    expect(deploymentDefaultProductSurface()).toBe('video-studio')
    expect(isEnabledProductSurface('agentworks')).toBe(true)
    expect(isEnabledProductSurface('finance')).toBe(false)
    expect(isEnabledProductSurface('dominion')).toBe(false)
    expect(isSingleProductDeployment()).toBe(false)
  })
})

describe('intersectAllowedProductSurfaces', () => {
  it('passes the deployment list through unchanged when the user is unrestricted', () => {
    expect(intersectAllowedProductSurfaces(['dominion', 'agentworks'], null)).toEqual(['dominion', 'agentworks'])
    expect(intersectAllowedProductSurfaces(['dominion', 'agentworks'], undefined)).toEqual(['dominion', 'agentworks'])
    expect(intersectAllowedProductSurfaces(['dominion', 'agentworks'], [])).toEqual(['dominion', 'agentworks'])
  })

  it('narrows to the explicit per-user allowlist, case-insensitively', () => {
    expect(intersectAllowedProductSurfaces(['dominion', 'agentworks'], ['Dominion'])).toEqual(['dominion'])
  })
})
