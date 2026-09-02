import { useCallback, useEffect, useMemo, useState } from 'react'
import { ArrowLeft, ChevronDown, ChevronRight, Lock, RefreshCw, Search, X } from 'lucide-react'
import LLMRoleSelector from '../LLMRoleSelector'
import LLMSelectionDropdown from '../LLMSelectionDropdown'
import { WorkflowProviderCredentialField } from '../WorkflowProviderCredentialField'
import { CodingAgentSection } from '../llm/CodingAgentSection'
import { APIProviderSection } from '../llm/APIProviderSection'
import { providerStatus } from '../llm/providerStatus'
import type { AgentLLMConfig, AgentLLMFallback, LLMProvider, PresetLLMConfig } from '../../services/api-types'
import { llmConfigService, type DynamicModelEntry, type ModelMetadata, type ProviderManifestEntry } from '../../services/llm-config-api'
import { useLLMStore } from '../../stores/useLLMStore'
import type { LLMOption } from '../../types/llm'
import { llmOptionsKey } from '../../utils/llmConfigDisplay'
import { resolvePiModelGroup } from '../../utils/llmDisplay'
import { getWorkflowLLMOptions, getWorkflowLLMTierDefaults, getWorkflowProviderOptions } from '../../utils/workflowLLMTierDefaults'

type RoleKey = 'tier_1' | 'tier_2' | 'tier_3' | 'builder_llm' | 'maintenance_llm' | 'pulse_llm'
const ROLE_KEYS: RoleKey[] = ['tier_1', 'tier_2', 'tier_3', 'builder_llm', 'maintenance_llm', 'pulse_llm']

type RoleRow = {
  key: RoleKey
  label: string
  description: string
  group: 'Execution' | 'Workflow agents'
}

const ROLE_ROWS: RoleRow[] = [
  { key: 'tier_1', label: 'High reasoning', description: 'First runs and complex execution.', group: 'Execution' },
  { key: 'tier_2', label: 'Medium reasoning', description: 'Execution after useful learnings exist.', group: 'Execution' },
  { key: 'tier_3', label: 'Low reasoning', description: 'Validation and mature learned tasks.', group: 'Execution' },
  { key: 'builder_llm', label: 'Builder', description: 'Chat, planning, evaluation design, and coordination.', group: 'Workflow agents' },
  { key: 'maintenance_llm', label: 'Maintenance', description: 'Harden, Goal Advisor, and deeper health reviews.', group: 'Workflow agents' },
  { key: 'pulse_llm', label: 'Pulse', description: 'Scheduled post-run QA and routine coordination.', group: 'Workflow agents' },
]

// Providers that use API keys (excludes local CLIs and hidden legacy chat providers)
type APIKeyProviderType = 'bedrock' | 'openai' | 'vertex' | 'anthropic' | 'azure'
type APIKeyStatusValue = 'idle' | 'testing' | 'valid' | 'invalid' | 'timeout'

const API_KEY_PROVIDER_IDS = new Set<string>(['bedrock', 'openai', 'vertex', 'anthropic', 'azure'])
const HIDDEN_CHAT_PROVIDER_TABS = new Set<string>([
  'openrouter', 'z-ai', 'kimi', 'minimax', 'minimax-coding-plan', 'elevenlabs', 'deepgram',
])
const CODING_AGENT_PROVIDER_ORDER = ['claude-code', 'codex-cli', 'cursor-cli', 'pi-cli']
const codingAgentProviderRank = (provider: string) => {
  const index = CODING_AGENT_PROVIDER_ORDER.indexOf(provider)
  return index === -1 ? 999 : index
}

// Pi CLI is one manifest entry that routes to several model backends (Gemini,
// Z.AI, Kimi, ...). The user thinks in terms of those backends, so the list
// shows one row per backend under a synthetic id; nothing here is a real
// manifest entry, and a selection has to be written as explicit per-role
// config because a provider profile can't say "pi-cli, but only Gemini".
const PI_GROUP_PREFIX = 'pi-cli::'
const piGroupRowId = (group: string) => `${PI_GROUP_PREFIX}${group}`
const piGroupFromRowId = (rowId: string): string | null =>
  rowId.startsWith(PI_GROUP_PREFIX) ? rowId.slice(PI_GROUP_PREFIX.length) : null

type ProviderRow = {
  id: string
  entry: ProviderManifestEntry
  name: string
  modelId: string | null
  selectable: boolean
  groupFilter?: string
}

type WorkflowLLMConfigurationPanelProps = {
  workspacePath: string | null
  llmConfig?: PresetLLMConfig
  onChange: (config: PresetLLMConfig) => void
}

const hasOptions = (options?: Record<string, unknown>) => Boolean(options && Object.keys(options).length > 0)

function toAgentLLMConfig(llm: LLMOption): AgentLLMConfig {
  return {
    ...(llm.id ? { published_llm_id: llm.id } : {}),
    provider: llm.provider as LLMProvider,
    model_id: llm.model,
    ...(hasOptions(llm.options) ? { options: llm.options } : {}),
  }
}

function toFallback(llm: LLMOption): AgentLLMFallback {
  const config = toAgentLLMConfig(llm)
  return {
    ...(config.published_llm_id ? { published_llm_id: config.published_llm_id } : {}),
    provider: config.provider,
    model_id: config.model_id,
    ...(hasOptions(config.options) ? { options: config.options } : {}),
  }
}

function configKey(config: { provider?: string; model_id?: string; published_llm_id?: string; options?: Record<string, unknown> }): string {
  return config.published_llm_id
    ? `id:${config.published_llm_id}`
    : `model:${config.provider}/${config.model_id}/${llmOptionsKey(config.options)}`
}

function optionKey(option: LLMOption): string {
  return option.id
    ? `id:${option.id}`
    : `model:${option.provider}/${option.model}/${llmOptionsKey(option.options)}`
}

function roleConfig(config: PresetLLMConfig | undefined, key: RoleKey): AgentLLMConfig | undefined {
  if (key === 'tier_1') return config?.tiered_config?.tier_1
  if (key === 'tier_2') return config?.tiered_config?.tier_2
  if (key === 'tier_3') return config?.tiered_config?.tier_3
  return config?.[key]
}

function configLabel(config?: AgentLLMConfig): string {
  return config ? `${config.provider}/${config.model_id}` : 'Not configured'
}

const statusToneClasses = {
  ready: { dot: 'bg-emerald-500', text: 'text-emerald-600 dark:text-emerald-400' },
  warn: { dot: 'bg-amber-500', text: 'text-amber-600 dark:text-amber-400' },
  managed: { dot: 'bg-blue-500', text: 'text-blue-600 dark:text-blue-400' },
}

function statusTone(label: string) {
  if (label === 'Ready') return statusToneClasses.ready
  if (label === 'Managed') return statusToneClasses.managed
  return statusToneClasses.warn
}

// "Ready" is the default for most rows, so spelling it out adds noise and
// buries the two rows that actually need attention. Working providers get a
// green dot and nothing else; only a provider that needs something from the
// user gets words, phrased as the thing to do.
const STATUS_ACTION_TEXT: Record<string, string> = {
  'Needs key': 'Add API key',
  'Not installed': 'Not installed',
  'Setup needed': 'Needs setup',
}

function statusActionText(label: string): string | null {
  return STATUS_ACTION_TEXT[label] ?? null
}

function statusTitle(label: string): string {
  if (label === 'Ready') return 'Installed and signed in — ready to use'
  if (label === 'Managed') return 'Configured by your administrator'
  if (label === 'Needs key') return 'Installed, but needs an API key before it can run'
  if (label === 'Not installed') return 'This CLI is not installed on this machine'
  return 'Needs setup before it can run'
}

export default function WorkflowLLMConfigurationPanel({ workspacePath, llmConfig, onChange }: WorkflowLLMConfigurationPanelProps) {
  const {
    availableLLMs,
    providerManifest,
    providerManifestLoaded,
    loadProviderManifest,
    defaultsLoaded,
    loadDefaultsFromBackend,
    getProviderDynamicModels,
    isProviderSupported,
    llmConfigLocked,
    lockedProviders,
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
  } = useLLMStore()

  const [expandedRole, setExpandedRole] = useState<RoleKey | null>(null)
  const [activeProviderId, setActiveProviderId] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [tokenOpen, setTokenOpen] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const [piCliModels, setPiCliModels] = useState<DynamicModelEntry[]>([])
  const [piCliGroups, setPiCliGroups] = useState<string[]>([])
  const [metadata, setMetadata] = useState<ModelMetadata[]>([])
  const [apiKeyStatus, setApiKeyStatus] = useState<Record<APIKeyProviderType, APIKeyStatusValue>>({
    openai: 'idle', bedrock: 'idle', vertex: 'idle', anthropic: 'idle', azure: 'idle',
  })
  const [apiKeyErrors, setApiKeyErrors] = useState<Record<APIKeyProviderType, string | null>>({
    openai: null, bedrock: null, vertex: null, anthropic: null, azure: null,
  })

  const advanced = llmConfig?.mode === 'explicit'
  const [advancedOpen, setAdvancedOpen] = useState(advanced)
  useEffect(() => {
    if (advanced) setAdvancedOpen(true)
  }, [advanced])

  useEffect(() => {
    if (!providerManifestLoaded) void loadProviderManifest()
    if (!defaultsLoaded) void loadDefaultsFromBackend()
  }, [defaultsLoaded, loadDefaultsFromBackend, loadProviderManifest, providerManifestLoaded])

  useEffect(() => {
    let cancelled = false
    llmConfigService.getModelMetadata()
      .then(response => {
        if (!cancelled && response.models?.length) setMetadata(response.models)
      })
      .catch(err => console.error('Failed to fetch model metadata', err))
    return () => { cancelled = true }
  }, [])

  const isProviderLocked = useCallback((provider: string) => {
    const base = piGroupFromRowId(provider) ? 'pi-cli' : provider
    return lockedProviders.includes('all') || lockedProviders.includes(base)
  }, [lockedProviders])

  const manifestEntries = useMemo(() => providerManifest.filter(entry => {
    if (entry.deprecated) return false
    if (HIDDEN_CHAT_PROVIDER_TABS.has(entry.id)) return false
    return isProviderSupported(entry.id as LLMProvider)
  }), [isProviderSupported, providerManifest])

  const hasPiCli = manifestEntries.some(entry => entry.id === 'pi-cli')
  useEffect(() => {
    if (!hasPiCli) return
    let cancelled = false
    getProviderDynamicModels('pi-cli', false).then(result => {
      if (cancelled || !result) return
      setPiCliModels(result.models || [])
      if (result.groups?.length) setPiCliGroups(result.groups)
    })
    return () => { cancelled = true }
  }, [getProviderDynamicModels, hasPiCli])

  // The group's designated default, else its first listed model.
  const piGroupDefaultModel = useCallback((group: string): string | null => {
    const inGroup = piCliModels.filter(model => model.group === group)
    if (inGroup.length === 0) return null
    return (inGroup.find(model => model.is_default) ?? inGroup[0]).model_id
  }, [piCliModels])

  const providerOptions = useMemo(() => getWorkflowProviderOptions(providerManifest), [providerManifest])
  const workflowOptions = useMemo(
    () => getWorkflowLLMOptions(availableLLMs, providerManifest),
    [availableLLMs, providerManifest],
  )

  const rows = useMemo<ProviderRow[]>(() => {
    const codingAgents = manifestEntries
      .filter(entry => entry.integration_kind === 'coding_agent')
      .sort((a, b) => codingAgentProviderRank(a.id) - codingAgentProviderRank(b.id) || a.display_name.localeCompare(b.display_name))
    const apiProviders = manifestEntries.filter(entry => entry.integration_kind === 'api_model' && API_KEY_PROVIDER_IDS.has(entry.id))

    const result: ProviderRow[] = []
    codingAgents.forEach(entry => {
      if (entry.id === 'pi-cli') {
        piCliGroups.forEach(group => {
          const modelId = piGroupDefaultModel(group)
          result.push({ id: piGroupRowId(group), entry, name: group, modelId, selectable: modelId !== null, groupFilter: group })
        })
        return
      }
      const selectable = providerOptions.some(option => option.provider === entry.id)
      result.push({
        id: entry.id,
        entry,
        name: entry.display_name,
        modelId: entry.default_tier_models?.high.model_id || entry.default_model_id || null,
        selectable,
      })
    })
    apiProviders.forEach(entry => {
      result.push({ id: entry.id, entry, name: entry.display_name, modelId: null, selectable: false })
    })
    return result
  }, [manifestEntries, piCliGroups, piGroupDefaultModel, providerOptions])

  // Which row the current config corresponds to, if any.
  const selectedRowId = useMemo<string | null>(() => {
    if (!llmConfig) return null
    if (llmConfig.mode === 'provider_profile') return llmConfig.provider ?? null
    const roles = ROLE_KEYS.map(key => roleConfig(llmConfig, key))
    if (roles.some(role => !role)) return null
    if (!roles.every(role => role!.provider === 'pi-cli')) return null
    const groups = new Set(roles.map(role => resolvePiModelGroup(role!.model_id)))
    if (groups.size !== 1) return null
    const [group] = Array.from(groups)
    return group ? piGroupRowId(group) : null
  }, [llmConfig])

  const visibleRows = useMemo(() => {
    const q = query.trim().toLowerCase()
    const filtered = q
      ? rows.filter(row => [row.name, row.id, row.modelId, row.entry.description]
          .some(value => value?.toLowerCase().includes(q)))
      : rows
    // The row this workflow is actually using bubbles to the top -- scanning
    // status dots down the list otherwise can't tell "in use" from "also ready".
    if (!selectedRowId) return filtered
    return [...filtered].sort((a, b) => {
      if (a.id === selectedRowId) return -1
      if (b.id === selectedRowId) return 1
      return 0
    })
  }, [query, rows, selectedRowId])

  const selectedRow = useMemo(() => rows.find(row => row.id === selectedRowId) ?? null, [rows, selectedRowId])
  const selectedBaseProvider = selectedRow ? (selectedRow.groupFilter ? 'pi-cli' : selectedRow.id) : null

  const selectedProfile = useMemo(() => {
    const provider = llmConfig?.mode === 'provider_profile'
      ? llmConfig.provider
      : llmConfig?.builder_llm?.provider
    return providerOptions.find(option => option.provider === provider) ?? null
  }, [llmConfig, providerOptions])

  const piGroupOption = useCallback((row: ProviderRow): LLMOption | null => {
    if (!row.groupFilter || !row.modelId) return null
    return { provider: 'pi-cli', model: row.modelId, label: row.name, section: 'published_model' }
  }, [])

  // Role defaults to compare against / reset to. For a pi backend that's the
  // backend's model for every role; otherwise the provider profile's manifest
  // defaults.
  const defaults = useMemo(() => {
    if (selectedRow?.groupFilter) {
      const option = piGroupOption(selectedRow)
      return option ? getWorkflowLLMTierDefaults(option, providerManifest) : null
    }
    return selectedProfile ? getWorkflowLLMTierDefaults(selectedProfile, providerManifest) : null
  }, [piGroupOption, providerManifest, selectedProfile, selectedRow])

  const selectRow = (row: ProviderRow) => {
    if (!row.selectable) return
    setExpandedRole(null)
    if (row.groupFilter) {
      const option = piGroupOption(row)
      if (!option) return
      const groupDefaults = getWorkflowLLMTierDefaults(option, providerManifest)
      onChange({
        schema_version: 2,
        mode: 'explicit',
        builder_llm: groupDefaults.builder,
        maintenance_llm: groupDefaults.maintenance,
        pulse_llm: groupDefaults.pulse,
        tiered_config: { tier_1: groupDefaults.tier1, tier_2: groupDefaults.tier2, tier_3: groupDefaults.tier3 },
      })
      return
    }
    onChange({ schema_version: 2, mode: 'provider_profile', provider: row.id as LLMProvider })
  }

  const useManagedDefaults = () => {
    if (!selectedProfile) return
    onChange({ schema_version: 2, mode: 'provider_profile', provider: selectedProfile.provider as LLMProvider })
    setExpandedRole(null)
  }

  const startAdvancedSetup = () => {
    if (!defaults) return
    onChange({
      schema_version: 2,
      mode: 'explicit',
      builder_llm: defaults.builder,
      maintenance_llm: defaults.maintenance,
      pulse_llm: defaults.pulse,
      tiered_config: { tier_1: defaults.tier1, tier_2: defaults.tier2, tier_3: defaults.tier3 },
    })
    setAdvancedOpen(true)
  }

  const updateRole = (key: RoleKey, next: AgentLLMConfig, preserveFallbacks = true) => {
    if (!advanced) return
    const current = roleConfig(llmConfig, key)
    const withFallbacks: AgentLLMConfig = {
      ...next,
      ...(preserveFallbacks && current?.fallbacks?.length ? { fallbacks: current.fallbacks } : {}),
    }
    const nextConfig: PresetLLMConfig = { ...llmConfig, schema_version: 2, mode: 'explicit' }
    if (key === 'tier_1' || key === 'tier_2' || key === 'tier_3') {
      nextConfig.tiered_config = {
        tier_1: llmConfig?.tiered_config?.tier_1 ?? defaults?.tier1 ?? withFallbacks,
        tier_2: llmConfig?.tiered_config?.tier_2 ?? defaults?.tier2 ?? withFallbacks,
        tier_3: llmConfig?.tiered_config?.tier_3 ?? defaults?.tier3 ?? withFallbacks,
        [key]: withFallbacks,
      }
    } else {
      nextConfig[key] = withFallbacks
    }
    onChange(nextConfig)
  }

  const defaultForRole = (key: RoleKey): AgentLLMConfig | undefined => {
    if (!defaults) return undefined
    return key === 'tier_1' ? defaults.tier1
      : key === 'tier_2' ? defaults.tier2
        : key === 'tier_3' ? defaults.tier3
          : key === 'builder_llm' ? defaults.builder
            : key === 'maintenance_llm' ? defaults.maintenance
              : defaults.pulse
  }

  const resetRole = (key: RoleKey) => {
    const defaultValue = defaultForRole(key)
    if (!advanced || !defaultValue) return
    updateRole(key, defaultValue, false)
  }

  const updateFallbacks = (key: RoleKey, fallbacks: AgentLLMFallback[]) => {
    const current = roleConfig(llmConfig, key)
    if (!current) return
    const next = { ...current }
    if (fallbacks.length) next.fallbacks = fallbacks
    else delete next.fallbacks
    updateRole(key, next, false)
  }

  const handleRefresh = async () => {
    setRefreshing(true)
    try {
      await Promise.all([loadProviderManifest(), loadDefaultsFromBackend()])
    } finally {
      setRefreshing(false)
    }
  }

  const providerConfigMap = useMemo(() => ({
    bedrock: { config: bedrockConfig, setConfig: setBedrockConfig },
    openai: { config: openaiConfig, setConfig: setOpenaiConfig },
    vertex: { config: vertexConfig, setConfig: setVertexConfig },
    anthropic: { config: anthropicConfig, setConfig: setAnthropicConfig },
    azure: { config: azureConfig, setConfig: setAzureConfig },
  }), [anthropicConfig, azureConfig, bedrockConfig, openaiConfig, vertexConfig,
    setAnthropicConfig, setAzureConfig, setBedrockConfig, setOpenaiConfig, setVertexConfig])

  const handleTestAPIKey = useCallback(async (provider: APIKeyProviderType, apiKey: string, modelId?: string, options?: Record<string, unknown>) => {
    // Bedrock and Vertex can validate without a pasted key (IAM / ADC).
    if (provider !== 'bedrock' && provider !== 'vertex' && !apiKey.trim()) return
    setApiKeyStatus(prev => ({ ...prev, [provider]: 'testing' }))
    setApiKeyErrors(prev => ({ ...prev, [provider]: null }))
    try {
      const result = await testAPIKey(provider, apiKey, modelId, options)
      setApiKeyStatus(prev => ({ ...prev, [provider]: result.valid ? 'valid' : 'invalid' }))
      setApiKeyErrors(prev => ({ ...prev, [provider]: result.valid ? null : (result.error || 'API key validation failed') }))
    } catch (err) {
      const timeout = err instanceof Error && err.message.includes('timeout')
      setApiKeyStatus(prev => ({ ...prev, [provider]: timeout ? 'timeout' : 'invalid' }))
      setApiKeyErrors(prev => ({
        ...prev,
        [provider]: timeout
          ? 'Request timed out. Please check your connection.'
          : err instanceof Error ? err.message : 'Unknown error occurred',
      }))
    }
  }, [testAPIKey])

  const activeRow = useMemo(() => rows.find(row => row.id === activeProviderId) ?? null, [activeProviderId, rows])

  // ---- Drill-in: connect / configure one provider ------------------------

  if (activeProviderId !== null) {
    const activeBaseId = activeRow ? (activeRow.groupFilter ? 'pi-cli' : activeRow.id) : activeProviderId
    const locked = isProviderLocked(activeProviderId)
    return (
      <div className="space-y-4">
        <button
          type="button"
          onClick={() => setActiveProviderId(null)}
          className="inline-flex items-center gap-1.5 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to providers
        </button>

        {!activeRow ? (
          <div className="py-8 text-center text-sm text-muted-foreground">Loading provider info...</div>
        ) : locked ? (
          <div className="flex items-start gap-2 rounded-md border border-dashed border-border px-4 py-5 text-sm text-muted-foreground">
            <Lock className="mt-0.5 h-4 w-4 shrink-0" />
            <div>
              <div className="font-medium text-foreground">Configured by admin</div>
              <div className="mt-0.5 text-xs">The credentials for this provider are set server-side. Contact your administrator to change them.</div>
            </div>
          </div>
        ) : API_KEY_PROVIDER_IDS.has(activeBaseId) ? (
          (() => {
            const providerKey = activeBaseId as APIKeyProviderType
            const configEntry = providerConfigMap[providerKey]
            return (
              <APIProviderSection
                provider={activeRow.entry}
                config={configEntry.config}
                onUpdate={config => configEntry.setConfig(config)}
                onTestAPIKey={(apiKey, modelId, options) => handleTestAPIKey(providerKey, apiKey, modelId, options)}
                apiKeyStatus={apiKeyStatus[providerKey]}
                apiKeyError={apiKeyErrors[providerKey]}
                metadata={metadata}
              />
            )
          })()
        ) : (
          <CodingAgentSection key={activeProviderId} provider={activeRow.entry} groupFilter={activeRow.groupFilter} />
        )}
      </div>
    )
  }

  // ---- Main view ----------------------------------------------------------

  const renderStatusLine = () => {
    if (selectedRow) {
      const status = providerStatus(selectedRow.entry, isProviderLocked(selectedRow.id))
      const tone = statusTone(status.label)
      const usable = status.label === 'Ready' || status.label === 'Managed'
      return (
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
          <span className="text-muted-foreground">This workflow runs on</span>
          <span className="inline-flex items-center gap-1.5 font-medium text-foreground">
            <span className={`h-2 w-2 rounded-full ${tone.dot}`} />
            {selectedRow.name}
            {selectedRow.modelId && <span className="font-normal text-muted-foreground">· {selectedRow.modelId}</span>}
          </span>
          {!usable && (
            <>
              <span className={`text-xs font-medium ${tone.text}`} title={statusTitle(status.label)}>
                {statusActionText(status.label) ?? status.label}
              </span>
              <button
                type="button"
                onClick={() => setActiveProviderId(selectedRow.id)}
                className="rounded-md bg-primary px-2 py-0.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
              >
                Set up
              </button>
            </>
          )}
        </div>
      )
    }
    if (advanced) {
      return (
        <div className="text-sm">
          <span className="text-muted-foreground">This workflow uses a </span>
          <span className="font-medium text-foreground">custom per-role setup</span>
          <span className="text-muted-foreground"> — see Advanced below.</span>
        </div>
      )
    }
    return <div className="text-sm text-muted-foreground">No provider selected — pick one below.</div>
  }

  const renderTokenLine = () => {
    if (selectedBaseProvider !== 'claude-code' && selectedBaseProvider !== 'cursor-cli') return null
    const isClaude = selectedBaseProvider === 'claude-code'
    return (
      <div className="mt-2">
        <button
          type="button"
          onClick={() => setTokenOpen(open => !open)}
          aria-expanded={tokenOpen}
          className="inline-flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
        >
          <ChevronRight className={`h-3 w-3 transition-transform ${tokenOpen ? 'rotate-90' : ''}`} />
          Workflow {isClaude ? 'token' : 'API key'}
          <span className="text-muted-foreground/70">· scoped to this workflow, falls back to the saved login</span>
        </button>
        {tokenOpen && (
          <div className="mt-2 rounded-md border border-border bg-muted/20 p-3">
            {isClaude ? (
              <WorkflowProviderCredentialField
                provider="claude-code"
                inputId="workflow-claude-code-token"
                workflowCredentialPath={workspacePath || undefined}
                copy={{
                  heading: 'Claude Code token',
                  hint: <>Use a token from <code className="rounded bg-background px-1 py-0.5 font-mono text-foreground">claude setup-token</code>, or leave this empty to use the saved Claude login.</>,
                  fallbackLabel: 'Using saved login',
                  inputPlaceholder: 'Paste Claude Code token',
                  replacePlaceholder: 'Paste a replacement token',
                  noun: 'token',
                  savedMessage: 'Workflow Claude Code token saved.',
                  removedMessage: 'Workflow Claude Code token removed; saved Claude login will be used.',
                }}
              />
            ) : (
              <WorkflowProviderCredentialField
                provider="cursor-cli"
                inputId="workflow-cursor-api-key"
                workflowCredentialPath={workspacePath || undefined}
                copy={{
                  heading: 'Cursor API key',
                  hint: <>Paste an API key from <code className="rounded bg-background px-1 py-0.5 font-mono text-foreground">cursor.com</code> settings, or leave this empty to use the saved Cursor login.</>,
                  fallbackLabel: 'Using saved login',
                  inputPlaceholder: 'Paste Cursor API key',
                  replacePlaceholder: 'Paste a replacement API key',
                  noun: 'API key',
                  savedMessage: 'Workflow Cursor API key saved.',
                  removedMessage: 'Workflow Cursor API key removed; saved Cursor login will be used.',
                }}
              />
            )}
          </div>
        )}
      </div>
    )
  }

  const renderRow = (row: ProviderRow) => {
    const status = providerStatus(row.entry, isProviderLocked(row.id))
    const tone = statusTone(status.label)
    const selected = row.id === selectedRowId
    return (
      <div key={row.id} className={`flex items-center gap-2 px-3 py-2 ${selected ? 'bg-primary/5' : ''}`}>
        {row.entry.integration_kind === 'coding_agent' ? (
          <button
            type="button"
            role="radio"
            aria-checked={selected}
            disabled={!row.selectable}
            onClick={() => selectRow(row)}
            title={row.selectable ? `Use ${row.name} for this workflow` : 'Loading models…'}
            className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full border border-border transition-colors hover:border-primary disabled:cursor-not-allowed disabled:opacity-40"
          >
            {selected && <span className="h-2 w-2 rounded-full bg-primary" />}
          </button>
        ) : (
          <span className="h-4 w-4 shrink-0" aria-hidden />
        )}
        <button
          type="button"
          onClick={() => setActiveProviderId(row.id)}
          className="flex min-w-0 flex-1 items-center gap-2 text-left"
          title={`Connect or configure ${row.name}`}
        >
          <span className={`shrink-0 text-sm ${selected ? 'font-semibold' : 'font-medium'} text-foreground`}>{row.name}</span>
          {selected && (
            <span className="shrink-0 rounded-full bg-primary/15 px-1.5 py-0.5 text-[10px] font-medium text-primary">
              In use
            </span>
          )}
          <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground">{row.modelId ?? ''}</span>
          <span
            className={`inline-flex shrink-0 items-center gap-1 text-[11px] font-medium ${tone.text}`}
            title={statusTitle(status.label)}
          >
            <span className={`h-1.5 w-1.5 rounded-full ${tone.dot}`} />
            {statusActionText(status.label)}
          </span>
          <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        </button>
      </div>
    )
  }

  const renderRole = (row: RoleRow) => {
    const value = roleConfig(llmConfig, row.key) ?? defaultForRole(row.key)
    const fallbackList = value?.fallbacks ?? []
    const expanded = expandedRole === row.key
    const defaultValue = defaultForRole(row.key)
    const isCustomized = configKey(value ?? {}) !== configKey(defaultValue ?? {}) || fallbackList.length > 0

    return (
      <div key={row.key} className="border-t border-border first:border-t-0">
        <button
          type="button"
          onClick={() => setExpandedRole(expanded ? null : row.key)}
          className="flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors hover:bg-muted/50"
          aria-expanded={expanded}
        >
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-xs font-medium text-foreground">{row.label}</span>
              <span className={`rounded-full px-1.5 py-0.5 text-[10px] font-medium ${isCustomized ? 'bg-primary/15 text-primary' : 'bg-muted text-muted-foreground'}`}>
                {isCustomized ? 'Customized' : 'Provider default'}
              </span>
            </div>
            <div className="mt-0.5 truncate text-[11px] text-muted-foreground">{row.description}</div>
          </div>
          <div className="min-w-0 max-w-[45%] text-right">
            <div className="truncate font-mono text-[11px] text-foreground" title={configLabel(value)}>{configLabel(value)}</div>
            {fallbackList.length > 0 && <div className="text-[10px] text-muted-foreground">{fallbackList.length} fallback{fallbackList.length === 1 ? '' : 's'}</div>}
          </div>
          <ChevronDown className={`h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform ${expanded ? 'rotate-180' : ''}`} />
        </button>
        {expanded && value && (
          <div className="space-y-3 border-t border-border bg-muted/20 px-3 py-3">
            <LLMRoleSelector availableLLMs={workflowOptions} value={value} onLLMSelect={llm => updateRole(row.key, toAgentLLMConfig(llm))} />
            {isCustomized && defaultValue && (
              <button type="button" onClick={() => resetRole(row.key)} className="text-xs text-primary hover:underline">
                Reset this role to provider default
              </button>
            )}
            <details className="rounded-md border border-border bg-background px-2.5 py-2">
              <summary className="cursor-pointer text-xs font-medium text-foreground">Fallbacks{fallbackList.length ? ` (${fallbackList.length})` : ''}</summary>
              <div className="mt-2 space-y-2">
                {fallbackList.map((fallback, index) => (
                  <span key={`${row.key}-${configKey(fallback)}-${index}`} className="mr-1 inline-flex items-center gap-1 rounded-full bg-muted px-2 py-0.5 text-xs text-foreground">
                    {fallback.provider}/{fallback.model_id.split('/').pop()}
                    <button type="button" onClick={() => updateFallbacks(row.key, fallbackList.filter((_, itemIndex) => itemIndex !== index))} className="text-muted-foreground hover:text-destructive" aria-label={`Remove ${row.label} fallback`}>
                      <X className="h-3 w-3" />
                    </button>
                  </span>
                ))}
                <LLMSelectionDropdown
                  availableLLMs={workflowOptions.filter(option => optionKey(option) !== configKey(value) && !fallbackList.some(fallback => configKey(fallback) === optionKey(option)))}
                  selectedLLM={null}
                  onLLMSelect={llm => updateFallbacks(row.key, [...fallbackList, toFallback(llm)])}
                  onRefresh={loadDefaultsFromBackend}
                  placeholder="+ Add fallback"
                />
              </div>
            </details>
          </div>
        )}
      </div>
    )
  }

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

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-border bg-muted/20 p-3">
        {renderStatusLine()}
        {renderTokenLine()}
      </div>

      <div className="flex items-center gap-2">
        <div className="relative min-w-0 flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <input
            type="search"
            value={query}
            onChange={event => setQuery(event.target.value)}
            placeholder="Search providers"
            aria-label="Search providers"
            className="h-9 w-full rounded-md border border-border bg-background pl-9 pr-3 text-sm text-foreground outline-none focus:border-primary"
          />
        </div>
        <button
          type="button"
          onClick={() => { void handleRefresh() }}
          disabled={refreshing}
          title="Refresh providers"
          aria-label="Refresh providers"
          className="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border text-muted-foreground transition-colors hover:text-foreground disabled:opacity-50"
        >
          <RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
        </button>
      </div>

      <div role="radiogroup" aria-label="Provider for this workflow" className="divide-y divide-border overflow-hidden rounded-md border border-border bg-background">
        {!providerManifestLoaded ? (
          <div className="py-6 text-center text-sm text-muted-foreground">Loading providers…</div>
        ) : visibleRows.length === 0 ? (
          <div className="py-6 text-center text-sm text-muted-foreground">
            {query.trim() ? `No providers match "${query}".` : 'No providers available.'}
          </div>
        ) : visibleRows.map(renderRow)}
      </div>

      <div className="rounded-md border border-border">
        <button
          type="button"
          onClick={() => setAdvancedOpen(open => !open)}
          aria-expanded={advancedOpen}
          className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm font-medium text-foreground transition-colors hover:bg-muted/50"
        >
          <ChevronRight className={`h-3.5 w-3.5 shrink-0 text-muted-foreground transition-transform ${advancedOpen ? 'rotate-90' : ''}`} />
          Advanced
          <span className="font-normal text-muted-foreground">— pin a model per role</span>
        </button>
        {advancedOpen && (
          <div className="space-y-3 border-t border-border p-3">
            {!advanced ? (
              <div className="flex flex-wrap items-center justify-between gap-2">
                <p className="text-xs text-muted-foreground">
                  Roles currently follow the selected provider's managed defaults. Pin them to choose a model, effort, and fallbacks per role.
                </p>
                <button
                  type="button"
                  onClick={startAdvancedSetup}
                  disabled={!defaults}
                  title={defaults ? undefined : 'Select a provider first'}
                  className="shrink-0 text-xs font-medium text-primary hover:underline disabled:cursor-not-allowed disabled:opacity-50"
                >
                  Pin models per role
                </button>
              </div>
            ) : (
              <>
                <div className="flex items-start justify-between gap-3">
                  <p className="text-xs text-muted-foreground">Each role is pinned. Open a row to change its model, effort, or fallbacks.</p>
                  <button
                    type="button"
                    onClick={useManagedDefaults}
                    disabled={!selectedProfile}
                    title={selectedProfile ? undefined : 'No provider profile to return to'}
                    className="shrink-0 text-xs font-medium text-primary hover:underline disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    Use managed defaults
                  </button>
                </div>
                {(['Execution', 'Workflow agents'] as const).map(group => (
                  <div key={group}>
                    <div className="mb-1.5 px-1 text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">{group}</div>
                    <div className="overflow-hidden rounded-md border border-border bg-background">
                      {ROLE_ROWS.filter(row => row.group === group).map(renderRole)}
                    </div>
                  </div>
                ))}
              </>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
