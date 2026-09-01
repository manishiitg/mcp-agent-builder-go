import React, { useCallback, useEffect, useMemo, useState } from 'react'
import { BrainCircuit, KeyRound, LoaderCircle, Monitor, Puzzle, Save, Server } from 'lucide-react'
import { ToolSelectionSection } from '../ToolSelectionSection'
import { SkillSelectionSection } from '../skills/SkillSelectionSection'
import { SecretSelectionSection } from '../secrets/SecretSelectionSection'
import SecretsManagerPanel from '../secrets/SecretsManagerPanel'
import BrowserAutomationSettings, { type BrowserAutomationMode } from '../BrowserAutomationSettings'
import WorkflowLLMConfigurationPanel from './WorkflowLLMConfigurationPanel'
import ConnectorsBrowser from '../connectors/ConnectorsBrowser'
import { agentApi, workflowManifestApi } from '../../services/api'
import type { WorkflowCapabilities } from '../../services/api-types'
import { useMCPStore } from '../../stores/useMCPStore'
import { useWorkflowManifestStore } from '../../stores/useWorkflowManifestStore'
import { useAuthStore } from '../../stores/useAuthStore'
import { isWorkflowReadOnly } from '../../utils/workflowPermissions'
import { toggleServerSelection } from '../../utils/mcpServerAlias'

export type WorkflowCapabilitySection = 'skills' | 'mcp' | 'secrets' | 'browser' | 'llm'

interface WorkflowCapabilitiesPanelProps {
  section: WorkflowCapabilitySection
  workspacePath: string | null
}

const EMPTY_CAPABILITIES: WorkflowCapabilities = {
  selected_servers: [],
  selected_tools: [],
  selected_skills: [],
  selected_secrets: [],
  selected_global_secret_names: null,
  browser_mode: 'none',
  use_code_execution_mode: false,
}

const SECTION_COPY: Record<WorkflowCapabilitySection, { title: string; description: string; Icon: typeof Puzzle }> = {
  skills: {
    title: 'Workflow skills',
    description: 'Select reusable skills for this workflow’s builder context.',
    Icon: Puzzle,
  },
  mcp: {
    title: 'Workflow MCP',
    description: 'Select the MCP servers and tools this workflow may use.',
    Icon: Server,
  },
  secrets: {
    title: 'Workflow secrets',
    description: 'Choose which workflow and global secrets this workflow may access.',
    Icon: KeyRound,
  },
  browser: {
    title: 'Browser automation',
    description: 'Control whether this workflow uses visible Chrome or managed headless browsing.',
    Icon: Monitor,
  },
  llm: {
    title: 'Workflow LLM configuration',
    description: 'Review the provider profile and any role-specific model overrides.',
    Icon: BrainCircuit,
  },
}

export default function WorkflowCapabilitiesPanel({ section, workspacePath }: WorkflowCapabilitiesPanelProps) {
  const isReadOnlyUser = useAuthStore(state => isWorkflowReadOnly(state.user, state.isMultiUserMode))
  const [capabilities, setCapabilities] = useState<WorkflowCapabilities>(EMPTY_CAPABILITIES)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [cdpPort, setCdpPort] = useState(9222)
  const [cdpConnected, setCdpConnected] = useState<boolean | null>(null)
  const [cdpError, setCdpError] = useState<string | null>(null)
  const [cdpChecking, setCdpChecking] = useState(false)
  const toolList = useMCPStore(state => state.toolList)
  // "Available to select for this workflow" means connected -- you can't
  // meaningfully pick tools from a server nobody has authenticated yet. A
  // not-yet-connected server only belongs in the "Connect a new MCP server"
  // browser below, not this checklist. Still include an already-selected
  // server even if it's since been disconnected, so it stays visible/
  // manageable here instead of silently vanishing from the workflow's config.
  const availableServers = useMemo(() => {
    const connected = toolList
      .filter(tool => tool.connection === 'connected' && tool.server)
      .map(tool => tool.server as string)
    return [...new Set([...connected, ...capabilities.selected_servers])]
  }, [toolList, capabilities.selected_servers])
  // Lets the "Connect a new MCP server" browser add/remove an already-connected
  // server from this workflow's selection directly, without needing the main
  // (now selected-only) list above -- same alias-safe logic ToolSelectionSection
  // itself uses for its own checkbox, via the shared toggleServerSelection util.
  const handleToggleServerForWorkflow = useCallback((serverName: string) => {
    setCapabilities(current => {
      const { servers, tools } = toggleServerSelection(serverName, current.selected_servers, current.selected_tools)
      return { ...current, selected_servers: servers, selected_tools: tools }
    })
  }, [])
  const copy = SECTION_COPY[section]

  const load = useCallback(async () => {
    if (!workspacePath) {
      setError('This panel needs an active workflow folder.')
      setLoading(false)
      return
    }
    setLoading(true)
    setError(null)
    try {
      const response = await workflowManifestApi.getWorkflowManifest(workspacePath)
      setCapabilities({ ...EMPTY_CAPABILITIES, ...response.manifest.capabilities })
      setCdpPort(response.manifest.capabilities.cdp_ports?.[0] || 9222)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Unable to load workflow capabilities')
    } finally {
      setLoading(false)
    }
  }, [workspacePath])

  useEffect(() => {
    void load()
  }, [load])

  const checkCdpConnection = useCallback(async (port: number) => {
    setCdpChecking(true)
    setCdpConnected(null)
    setCdpError(null)
    try {
      const result = await agentApi.checkCdpPort(port)
      setCdpConnected(result.connected)
      setCdpError(result.connected ? null : result.error || null)
    } catch {
      setCdpConnected(false)
      setCdpError('Unable to check the CDP port.')
    } finally {
      setCdpChecking(false)
    }
  }, [])

  const save = useCallback(async () => {
    if (!workspacePath) {
      setError('This panel needs an active workflow folder before it can save.')
      return
    }
    setSaving(true)
    setError(null)
    try {
      await workflowManifestApi.updateWorkflowManifest({ workspace_path: workspacePath, capabilities })
      await useWorkflowManifestStore.getState().refreshWorkflows()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Unable to save workflow capabilities')
    } finally {
      setSaving(false)
    }
  }, [capabilities, workspacePath])

  return (
    <section className="flex h-full min-h-0 w-full max-w-none flex-col bg-background">
      <header className="flex shrink-0 items-start gap-3 border-b px-4 py-3">
        <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
          <copy.Icon className="h-4 w-4" />
        </div>
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-semibold text-foreground">{copy.title}</h2>
          <p className="mt-0.5 text-xs text-muted-foreground">{copy.description}</p>
        </div>
      </header>

      <div className={`min-h-0 flex-1 p-4 ${section === 'skills' || section === 'secrets' || section === 'mcp' ? 'flex flex-col overflow-hidden' : 'overflow-y-auto'}`}>
        {loading ? (
          <div className="flex items-center justify-center gap-2 py-12 text-sm text-muted-foreground">
            <LoaderCircle className="h-4 w-4 animate-spin" /> Loading workflow settings…
          </div>
        ) : (
          <>
            {error && <p className="mb-4 rounded-md border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive">{error}</p>}
            {section === 'skills' && (
              <div className="min-h-0 flex-1">
                <SkillSelectionSection
                  selectedSkills={capabilities.selected_skills}
                  onSkillChange={(selected_skills) => setCapabilities(current => ({ ...current, selected_skills }))}
                  fillAvailableHeight
                />
              </div>
            )}
            {section === 'mcp' && (
              <div className="flex min-h-0 flex-1 flex-col">
                <div className="shrink-0 overflow-y-auto">
                  <ToolSelectionSection
                    availableServers={availableServers}
                    selectedServers={capabilities.selected_servers}
                    selectedTools={capabilities.selected_tools}
                    onServerChange={(selected_servers) => setCapabilities(current => ({ ...current, selected_servers }))}
                    onToolChange={(selected_tools) => setCapabilities(current => ({ ...current, selected_tools }))}
                    agentMode="workflow"
                    hideHeader
                    showSelectedOnly
                  />
                </div>
                <div className="mt-3 flex min-h-0 flex-1 flex-col border-t border-border pt-3">
                  <div className="shrink-0 text-sm font-medium text-muted-foreground">
                    Connect a new MCP server
                  </div>
                  <div className="mt-3 min-h-0 flex-1">
                    <ConnectorsBrowser
                      compact
                      selectedServers={capabilities.selected_servers}
                      onToggleServer={handleToggleServerForWorkflow}
                    />
                  </div>
                </div>
              </div>
            )}
            {section === 'secrets' && (
              <div className="flex min-h-0 flex-1 flex-col">
                <div className="shrink-0 overflow-y-auto">
                  <SecretSelectionSection
                    selectedSecrets={capabilities.selected_secrets}
                    onSecretChange={(selected_secrets) => setCapabilities(current => ({ ...current, selected_secrets }))}
                    selectedGlobalSecrets={capabilities.selected_global_secret_names}
                    onGlobalSecretChange={(selected_global_secret_names) => setCapabilities(current => ({ ...current, selected_global_secret_names }))}
                    workflowPath={workspacePath || ''}
                  />
                </div>
                <div className="mt-3 flex min-h-0 flex-1 flex-col pt-1">
                  <div className="shrink-0 text-sm font-medium text-muted-foreground">
                    Manage secrets
                  </div>
                  <div className="mt-3 flex min-h-0 flex-1 flex-col">
                    <SecretsManagerPanel compact />
                  </div>
                </div>
              </div>
            )}
            {section === 'browser' && (
              <BrowserAutomationSettings
                browserMode={capabilities.browser_mode as BrowserAutomationMode}
                onBrowserModeChange={(browser_mode) => setCapabilities(current => ({ ...current, browser_mode }))}
                cdpPort={cdpPort}
                onCdpPortChange={(port) => {
                  setCdpPort(port)
                  setCapabilities(current => ({ ...current, cdp_ports: [port] }))
                }}
                cdpConnected={cdpConnected}
                cdpError={cdpError}
                cdpChecking={cdpChecking}
                onCheckCdpConnection={checkCdpConnection}
              />
            )}
            {section === 'llm' && (
              <WorkflowLLMConfigurationPanel
                workspacePath={workspacePath}
                llmConfig={capabilities.llm_config}
                onChange={(llm_config) => setCapabilities(current => ({ ...current, llm_config }))}
              />
            )}
          </>
        )}
      </div>

      {/* PLAT-262: Save hidden for a read-only user — nothing in this panel
          can actually persist for that account, so hide the button that
          implies otherwise rather than let it fail after the fact. */}
      {!loading && !isReadOnlyUser && (
        <footer className="flex shrink-0 justify-end border-t px-4 py-3">
          <button
            type="button"
            onClick={() => void save()}
            disabled={saving}
            className="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-60"
          >
            {saving ? <LoaderCircle className="h-3.5 w-3.5 animate-spin" /> : <Save className="h-3.5 w-3.5" />}
            Save
          </button>
        </footer>
      )}
    </section>
  )
}
