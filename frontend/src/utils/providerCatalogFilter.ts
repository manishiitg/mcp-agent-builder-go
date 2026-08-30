import type { ProviderManifestEntry } from '../services/llm-config-api'

// Extracted from LibraryTab.tsx so it can be tested without importing the
// component (which pulls in useLLMStore and the wider app graph).

// Deprecated providers (2026-08-20: direct API transport, see
// docs/design/api_transport_vs_pi_tradeoff.md) don't show in the provider
// catalog at all -- not listed with a deprecated marker, simply absent.
export function nonDeprecatedProviders(providers: ProviderManifestEntry[]): ProviderManifestEntry[] {
  return providers.filter(provider => !provider.deprecated)
}
