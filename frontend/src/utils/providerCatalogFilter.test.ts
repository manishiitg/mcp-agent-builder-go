import { describe, it, expect } from 'vitest'
import { nonDeprecatedProviders } from './providerCatalogFilter'
import type { ProviderManifestEntry } from '../services/llm-config-api'

// Minimal fixture -- only the fields nonDeprecatedProviders reads.
const provider = (id: string, deprecated?: boolean): ProviderManifestEntry =>
  ({ id, deprecated } as ProviderManifestEntry)

describe('nonDeprecatedProviders', () => {
  // The regression this exists for: 2026-08-20, direct API transport
  // (openai/anthropic/vertex/bedrock/azure) deprecated in favor of MCP-routed
  // coding CLIs (docs/design/api_transport_vs_pi_tradeoff.md). The catalog
  // must not list them at all -- no marker, simply absent.
  it('drops deprecated providers entirely', () => {
    const result = nonDeprecatedProviders([
      provider('openai', true),
      provider('pi-cli', false),
    ])
    expect(result.map(p => p.id)).toEqual(['pi-cli'])
  })

  it('keeps a provider with no deprecated field at all', () => {
    const result = nonDeprecatedProviders([provider('codex-cli')])
    expect(result).toHaveLength(1)
  })

  it('returns everything when nothing is deprecated', () => {
    const result = nonDeprecatedProviders([provider('a'), provider('b')])
    expect(result).toHaveLength(2)
  })
})
