import { useState, useEffect, useCallback, useMemo } from 'react'
import { ArrowLeft, Lock } from 'lucide-react'
import { useLLMStore } from '../../stores'
import type { LLMProvider } from '../../services/api-types'
import { CodingAgentSection } from './CodingAgentSection'
import { APIProviderSection } from './APIProviderSection'
import { llmConfigService, type ModelMetadata, type ProviderManifestEntry } from '../../services/llm-config-api'
import { LibraryTab } from './LibraryTab'

// Providers that use API keys in this panel (excludes local CLIs and hidden legacy chat providers)
type APIKeyProviderType = 'bedrock' | 'openai' | 'vertex' | 'anthropic' | 'azure'

type APIKeyStatusValue = 'idle' | 'testing' | 'valid' | 'invalid' | 'timeout'

type APIKeyStatus = Record<APIKeyProviderType, APIKeyStatusValue>

type APIKeyError = Record<APIKeyProviderType, string | null>

// Pi CLI routes through several distinct model backends (Gemini, OpenRouter,
// Z.AI, ...). The manifest has exactly one 'pi-cli' provider entry -- this
// synthetic id scheme (mirrors LLMConfigurationModal's piCliGroupTabId)
// lets `activeProviderId` point at one backend group without a real
// per-group manifest entry.
const piCliGroupTabId = (group: string): string => `pi-cli::${group}`
const piCliGroupFromTabId = (tabId: string): string | null =>
  tabId.startsWith('pi-cli::') ? tabId.slice('pi-cli::'.length) : null

const HIDDEN_CHAT_PROVIDER_TABS = new Set<string>([
  'openrouter', 'z-ai', 'kimi', 'minimax', 'minimax-coding-plan', 'elevenlabs', 'deepgram'
])
const API_KEY_PROVIDER_IDS = new Set<string>(['bedrock', 'openai', 'vertex', 'anthropic', 'azure'])
const CODING_AGENT_PROVIDER_ORDER = ['claude-code', 'codex-cli', 'cursor-cli', 'pi-cli']
const CODING_AGENT_PROVIDER_RANK = new Map<string, number>(
  CODING_AGENT_PROVIDER_ORDER.map((provider, index) => [provider, index])
)
const codingAgentProviderRank = (provider: string) =>
  CODING_AGENT_PROVIDER_RANK.get(provider) ?? 999

export default function LLMLibraryPanel() {
  const {
    bedrockConfig,
    openaiConfig,
    vertexConfig,
    anthropicConfig,
    azureConfig,
    setBedrockConfig,
    setOpenaiConfig,
    setVertexConfig,
    setAnthropicConfig,
    setAzureConfig,
    testAPIKey,
    defaultsLoaded,
    loadDefaultsFromBackend,
    getProviderDynamicModels,
    isProviderSupported,
    llmConfigLocked,
    lockedProviders,
    providerManifest,
    providerManifestLoaded,
    loadProviderManifest,
  } = useLLMStore()

  const isProviderLocked = useCallback((provider: string) => {
    const basedOn = piCliGroupFromTabId(provider) ? 'pi-cli' : provider
    return lockedProviders.includes('all') || lockedProviders.includes(basedOn)
  }, [lockedProviders])

  const manifestProviderEntries = useMemo(() => (
    providerManifest.filter(entry => {
      if (entry.deprecated) return false
      if (HIDDEN_CHAT_PROVIDER_TABS.has(entry.id)) return false
      return isProviderSupported(entry.id as LLMProvider)
    })
  ), [isProviderSupported, providerManifest])

  const codingAgentProviderEntries = useMemo(
    () => manifestProviderEntries
      .filter(entry => entry.integration_kind === 'coding_agent')
      .sort((a, b) =>
        codingAgentProviderRank(a.id) - codingAgentProviderRank(b.id) ||
        a.display_name.localeCompare(b.display_name)
      ),
    [manifestProviderEntries]
  )

  // Pi CLI is one manifest entry but routes through several model backends;
  // fetch its group list once so a click on "Pi CLI" in the library can land
  // on the first backend's config screen instead of a single opaque tab.
  const [piCliGroups, setPiCliGroups] = useState<string[]>([])
  useEffect(() => {
    if (!codingAgentProviderEntries.some(entry => entry.id === 'pi-cli')) return
    let cancelled = false
    getProviderDynamicModels('pi-cli', false).then(result => {
      if (cancelled || !result?.groups?.length) return
      setPiCliGroups(result.groups)
    })
    return () => { cancelled = true }
  }, [codingAgentProviderEntries, getProviderDynamicModels])

  // The user thinks and works in terms of the underlying model backends Pi
  // routes to (mainly Gemini and the Chinese providers -- Z.AI, Kimi,
  // MiniMax, DeepSeek), not "Pi CLI" as a provider in its own right. Once
  // the group list has loaded, show one card per backend in the library
  // grid instead of a single opaque "Pi CLI" card the user has to click
  // into to discover what's actually available.
  const libraryProviderEntries = useMemo(() => {
    const piCli = manifestProviderEntries.find(entry => entry.id === 'pi-cli')
    if (!piCli || piCliGroups.length === 0) return manifestProviderEntries
    const groupEntries: ProviderManifestEntry[] = piCliGroups.map(group => ({
      ...piCli,
      id: piCliGroupTabId(group),
      display_name: group,
    }))
    return [
      ...manifestProviderEntries.filter(entry => entry.id !== 'pi-cli'),
      ...groupEntries,
    ]
  }, [manifestProviderEntries, piCliGroups])

  const providerConfigMap = useMemo(() => ({
    bedrock: { config: bedrockConfig, setConfig: setBedrockConfig },
    openai: { config: openaiConfig, setConfig: setOpenaiConfig },
    vertex: { config: vertexConfig, setConfig: setVertexConfig },
    anthropic: { config: anthropicConfig, setConfig: setAnthropicConfig },
    azure: { config: azureConfig, setConfig: setAzureConfig }
  }), [bedrockConfig, openaiConfig, vertexConfig, anthropicConfig, azureConfig,
      setBedrockConfig, setOpenaiConfig, setVertexConfig, setAnthropicConfig, setAzureConfig])

  const [metadata, setMetadata] = useState<ModelMetadata[]>([])

  // Fetch model metadata on mount -- this panel has no isOpen concept, it's
  // rendered whenever the workflow's LLM section is open.
  useEffect(() => {
    let cancelled = false
    const fetchMetadata = async () => {
      try {
        const response = await llmConfigService.getModelMetadata()
        if (!cancelled && response.models && response.models.length > 0) {
          setMetadata(response.models)
        }
      } catch (err) {
        console.error('Failed to fetch model metadata', err)
      }
    }
    fetchMetadata()
    return () => { cancelled = true }
  }, [])

  useEffect(() => {
    if (!defaultsLoaded) {
      loadDefaultsFromBackend()
    }
    if (!providerManifestLoaded) {
      loadProviderManifest()
    }
  }, [defaultsLoaded, loadDefaultsFromBackend, providerManifestLoaded, loadProviderManifest])

  const [apiKeyStatus, setApiKeyStatus] = useState<APIKeyStatus>({
    openai: 'idle',
    bedrock: 'idle',
    vertex: 'idle',
    anthropic: 'idle',
    azure: 'idle'
  })

  const [apiKeyErrors, setApiKeyErrors] = useState<APIKeyError>({
    openai: null,
    bedrock: null,
    vertex: null,
    anthropic: null,
    azure: null
  })

  const handleTestAPIKey = useCallback(async (provider: APIKeyProviderType, apiKey: string, modelId?: string, options?: Record<string, unknown>) => {
    // Allow testing without API key for Bedrock and Vertex (they support OAuth/credentials)
    if (provider !== 'bedrock' && provider !== 'vertex' && !apiKey.trim()) {
      return
    }

    setApiKeyStatus(prev => ({ ...prev, [provider]: 'testing' }))
    setApiKeyErrors(prev => ({ ...prev, [provider]: null }))

    try {
      const result = await testAPIKey(provider, apiKey, modelId, options)
      if (result.valid) {
        setApiKeyStatus(prev => ({ ...prev, [provider]: 'valid' }))
        setApiKeyErrors(prev => ({ ...prev, [provider]: null }))
      } else {
        setApiKeyStatus(prev => ({ ...prev, [provider]: 'invalid' }))
        setApiKeyErrors(prev => ({ ...prev, [provider]: result.error || 'API key validation failed' }))
      }
    } catch (err) {
      if (err instanceof Error && err.message.includes('timeout')) {
        setApiKeyStatus(prev => ({ ...prev, [provider]: 'timeout' }))
        setApiKeyErrors(prev => ({ ...prev, [provider]: 'Request timed out. Please check your connection.' }))
      } else {
        setApiKeyStatus(prev => ({ ...prev, [provider]: 'invalid' }))
        setApiKeyErrors(prev => ({ ...prev, [provider]: err instanceof Error ? err.message : 'Unknown error occurred' }))
      }
    }
  }, [testAPIKey])

  const [activeProviderId, setActiveProviderId] = useState<string | null>(null)

  const openProvider = useCallback((provider: ProviderManifestEntry) => {
    if (provider.id === 'pi-cli') {
      // 'pi-cli' alone has no real config screen -- it's split into one
      // screen per model backend. Land on the first (Gemini, when available).
      setActiveProviderId(piCliGroupTabId(piCliGroups[0] || 'Gemini'))
      return
    }
    setActiveProviderId(provider.id)
  }, [piCliGroups])

  const activeEntry = useMemo(() => {
    if (!activeProviderId) return null
    const piCliGroup = piCliGroupFromTabId(activeProviderId)
    const providerId = piCliGroup ? 'pi-cli' : activeProviderId
    const entry = providerManifest.find(p => p.id === providerId)
    if (!entry) return null
    return { entry, providerId, groupFilter: piCliGroup || undefined }
  }, [activeProviderId, providerManifest])

  if (llmConfigLocked) {
    return (
      <div className="flex items-start gap-2 rounded-md border border-dashed border-border px-4 py-5 text-sm text-muted-foreground">
        <Lock className="mt-0.5 h-4 w-4 shrink-0" />
        <div>
          <div className="font-medium text-foreground">LLM settings are locked by admin</div>
          <div className="mt-0.5 text-xs">Contact your administrator to enable new providers or models.</div>
        </div>
      </div>
    )
  }

  if (activeProviderId === null) {
    return (
      <LibraryTab
        providers={libraryProviderEntries}
        onSelectProvider={openProvider}
        isProviderLocked={isProviderLocked}
        hideSavedConfigurations
      />
    )
  }

  return (
    <div className="space-y-4">
      <button
        type="button"
        onClick={() => setActiveProviderId(null)}
        className="inline-flex items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to library
      </button>

      {!activeEntry ? (
        <div className="text-sm text-muted-foreground py-8 text-center">Loading provider info...</div>
      ) : isProviderLocked(activeEntry.providerId) ? (
        <div className="flex items-start gap-2 rounded-md border border-dashed border-border px-4 py-5 text-sm text-muted-foreground">
          <Lock className="mt-0.5 h-4 w-4 shrink-0" />
          <div>
            <div className="font-medium text-foreground">Configured by admin</div>
            <div className="mt-0.5 text-xs">
              The API key for this provider is set server-side. Contact your administrator to change it.
            </div>
            {providerConfigMap[activeEntry.providerId as APIKeyProviderType]?.config.model_id && (
              <div className="mt-2 text-xs">
                Current model: <span className="font-mono text-foreground">
                  {providerConfigMap[activeEntry.providerId as APIKeyProviderType].config.model_id}
                </span>
              </div>
            )}
          </div>
        </div>
      ) : API_KEY_PROVIDER_IDS.has(activeEntry.providerId) ? (
        (() => {
          const providerKey = activeEntry.providerId as APIKeyProviderType
          const configEntry = providerConfigMap[providerKey]
          return (
            <APIProviderSection
              provider={activeEntry.entry}
              config={configEntry.config}
              onUpdate={(config) => configEntry.setConfig(config)}
              onTestAPIKey={(apiKey, modelId, options) => handleTestAPIKey(providerKey, apiKey, modelId, options)}
              apiKeyStatus={apiKeyStatus[providerKey]}
              apiKeyError={apiKeyErrors[providerKey]}
              metadata={metadata}
            />
          )
        })()
      ) : (
        <CodingAgentSection key={activeProviderId} provider={activeEntry.entry} groupFilter={activeEntry.groupFilter} />
      )}
    </div>
  )
}
