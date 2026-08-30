import { useEffect, useMemo, useState } from 'react'
import { ChevronDown, SlidersHorizontal, X } from 'lucide-react'
import LLMRoleSelector from '../LLMRoleSelector'
import LLMSelectionDropdown from '../LLMSelectionDropdown'
import WorkflowLLMTierPreview from '../WorkflowLLMTierPreview'
import type { AgentLLMConfig, AgentLLMFallback, LLMProvider, PresetLLMConfig } from '../../services/api-types'
import { useLLMStore } from '../../stores/useLLMStore'
import type { LLMOption } from '../../types/llm'
import { llmOptionsKey } from '../../utils/llmConfigDisplay'
import { getWorkflowLLMOptions, getWorkflowLLMTierDefaults, getWorkflowProviderOptions } from '../../utils/workflowLLMTierDefaults'

type RoleKey = 'tier_1' | 'tier_2' | 'tier_3' | 'builder_llm' | 'maintenance_llm' | 'pulse_llm'

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

type WorkflowLLMConfigurationPanelProps = {
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

export default function WorkflowLLMConfigurationPanel({ llmConfig, onChange }: WorkflowLLMConfigurationPanelProps) {
  const [expandedRole, setExpandedRole] = useState<RoleKey | null>(null)
  const availableLLMs = useLLMStore(state => state.availableLLMs)
  const providerManifest = useLLMStore(state => state.providerManifest)
  const providerManifestLoaded = useLLMStore(state => state.providerManifestLoaded)
  const loadProviderManifest = useLLMStore(state => state.loadProviderManifest)
  const loadDefaultsFromBackend = useLLMStore(state => state.loadDefaultsFromBackend)

  const providerOptions = useMemo(() => getWorkflowProviderOptions(providerManifest), [providerManifest])
  const workflowOptions = useMemo(
    () => getWorkflowLLMOptions(availableLLMs, providerManifest),
    [availableLLMs, providerManifest],
  )
  const selectedProfile = useMemo(() => {
    const provider = llmConfig?.mode === 'provider_profile'
      ? llmConfig.provider
      : llmConfig?.builder_llm?.provider
    return providerOptions.find(option => option.provider === provider) ?? null
  }, [llmConfig, providerOptions])
  const defaults = useMemo(
    () => selectedProfile ? getWorkflowLLMTierDefaults(selectedProfile, providerManifest) : null,
    [providerManifest, selectedProfile],
  )
  const advanced = llmConfig?.mode === 'explicit'

  useEffect(() => {
    if (!providerManifestLoaded) void loadProviderManifest()
  }, [loadProviderManifest, providerManifestLoaded])

  const useManagedDefaults = () => {
    if (!selectedProfile) return
    onChange({ schema_version: 2, mode: 'provider_profile', provider: selectedProfile.provider as LLMProvider })
    setExpandedRole(null)
  }

  const selectProfile = (llm: LLMOption) => {
    onChange({ schema_version: 2, mode: 'provider_profile', provider: llm.provider as LLMProvider })
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

  const resetRole = (key: RoleKey) => {
    if (!advanced || !defaults) return
    const defaultValue = key === 'tier_1' ? defaults.tier1
      : key === 'tier_2' ? defaults.tier2
        : key === 'tier_3' ? defaults.tier3
          : key === 'builder_llm' ? defaults.builder
            : key === 'maintenance_llm' ? defaults.maintenance
              : defaults.pulse
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

  const renderRole = (row: RoleRow) => {
    const value = roleConfig(llmConfig, row.key) ?? (row.key === 'tier_1' ? defaults?.tier1
      : row.key === 'tier_2' ? defaults?.tier2
        : row.key === 'tier_3' ? defaults?.tier3
          : row.key === 'builder_llm' ? defaults?.builder
            : row.key === 'maintenance_llm' ? defaults?.maintenance
              : defaults?.pulse)
    const fallbackList = value?.fallbacks ?? []
    const expanded = expandedRole === row.key
    const defaultValue = row.key === 'tier_1' ? defaults?.tier1
      : row.key === 'tier_2' ? defaults?.tier2
        : row.key === 'tier_3' ? defaults?.tier3
          : row.key === 'builder_llm' ? defaults?.builder
            : row.key === 'maintenance_llm' ? defaults?.maintenance
              : defaults?.pulse
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
            {isCustomized && (
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

  return (
    <div className="space-y-4">
      {!advanced ? (
        <div className="rounded-lg border border-border bg-muted/20 p-3">
          <label className="block text-xs font-medium text-foreground">Automation provider</label>
          <p className="mt-1 text-xs text-muted-foreground">Choose the coding agent whose managed defaults this workflow follows.</p>
          <div className="mt-3">
            <LLMSelectionDropdown
              availableLLMs={providerOptions}
              selectedLLM={selectedProfile}
              onLLMSelect={selectProfile}
              onRefresh={loadDefaultsFromBackend}
              disabled={!providerManifestLoaded}
              title="Select automation provider"
              placeholder={providerManifestLoaded ? 'Select a coding agent' : 'Loading providers…'}
            />
          </div>
          <WorkflowLLMTierPreview selectedLLM={selectedProfile} providerManifest={providerManifest} />
          <p className="mt-3 text-xs leading-relaxed text-muted-foreground">Builder, Maintenance, Pulse, and execution tiers follow the provider’s current defaults. Switch to advanced setup to pin individual roles and fallback order.</p>
          <button type="button" onClick={startAdvancedSetup} disabled={!defaults} className="mt-3 inline-flex items-center gap-1.5 text-xs font-medium text-primary hover:underline disabled:cursor-not-allowed disabled:opacity-50">
            <SlidersHorizontal className="h-3.5 w-3.5" /> Advanced automation LLM setup
          </button>
        </div>
      ) : (
        <div className="space-y-3">
          <div className="flex items-start justify-between gap-3 rounded-lg border border-border bg-muted/20 p-3">
            <div>
              <div className="text-xs font-medium text-foreground">Explicit role configuration</div>
              <div className="mt-0.5 text-[11px] leading-snug text-muted-foreground">Each workflow role is pinned here. Open a row to change its model, effort, or fallbacks.</div>
            </div>
            <button type="button" onClick={useManagedDefaults} disabled={!selectedProfile} className="shrink-0 text-xs font-medium text-primary hover:underline disabled:cursor-not-allowed disabled:opacity-50">
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
        </div>
      )}
    </div>
  )
}
