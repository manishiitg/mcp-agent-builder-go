import { afterEach, describe, expect, it, vi } from 'vitest'
import { visibleProductSurfaceIDs } from './ProductSurfaceSwitcher'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ProductSurfaceSwitcher deployment allowlist', () => {
  it('shows only AgentWorks and Video Studio on the dedicated server', () => {
    vi.stubGlobal('window', {
      __APP_RUNTIME_CONFIG__: {
        defaultProductSurface: 'video-studio',
        enabledProductSurfaces: ['agentworks', 'video-studio'],
      },
    })
    expect(visibleProductSurfaceIDs()).toEqual(['agentworks', 'video-studio'])
  })
})
