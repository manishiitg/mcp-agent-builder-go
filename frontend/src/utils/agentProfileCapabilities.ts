import { getApiBaseUrl, getAuthToken } from '../services/api'

// Reads a capability declared on an agent profile's runtime.capabilities block
// (agentprofiles.RuntimeCapabilities in agent_go/pkg/agentprofiles/types.go).
// Deliberately generic over the profile id: a product opts into a shared
// capability (browser, secrets, voice, ...) by declaring it in its OWN
// product.yaml, so the frontend must not hardcode which product has which
// capability — that defeats the point of it being product.yaml-driven.

type AgentProfileCapabilitiesResponse = {
  runtime?: {
    capabilities?: Record<string, unknown>
  }
}

const capabilityCache = new Map<string, Promise<boolean>>()

/**
 * Resolves whether `profileId` declared `capability` as anything other than
 * "disabled" (or absent). Mirrors agentprofiles.CapabilityRequirement: string
 * equality rather than a closed union, so an unrecognized future requirement
 * value (e.g. a new tier between preferred/optional) still gates as enabled
 * rather than silently hiding a capability the backend actually turned on.
 *
 * Cached per (profileId, capability) for the page session — this is read at
 * composer-mount time, and a profile's declared capabilities do not change
 * without a server restart.
 */
export function loadAgentProfileCapabilityEnabled(profileId: string, capability: string): Promise<boolean> {
  if (!profileId) return Promise.resolve(false)
  const cacheKey = `${profileId}::${capability}`
  const cached = capabilityCache.get(cacheKey)
  if (cached) return cached

  const token = getAuthToken()
  const promise = fetch(`${getApiBaseUrl()}/api/agent-profiles/${encodeURIComponent(profileId)}?version=2`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  })
    .then((response) => {
      if (!response.ok) throw new Error(`Unable to load agent profile ${profileId} (${response.status})`)
      return response.json() as Promise<AgentProfileCapabilitiesResponse>
    })
    .then((profile) => {
      const value = profile.runtime?.capabilities?.[capability]
      const asStr = typeof value === 'string' ? value.trim() : ''
      return asStr !== '' && asStr !== 'disabled'
    })
    .catch(() => false)

  capabilityCache.set(cacheKey, promise)
  return promise
}
