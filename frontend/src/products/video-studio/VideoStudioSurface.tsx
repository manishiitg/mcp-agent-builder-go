import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent, type ReactNode } from 'react'
import {
  AlertCircle,
  ArrowLeft,
  CheckCircle2,
  ChevronDown,
  Clock3,
  Clapperboard,
  FileImage,
  FileText,
  Film,
  FolderOpen,
  Users,
  Eye,
  EyeOff,
  KeyRound,
  Loader2,
  MessageSquareText,
  Play,
  Plus,
  RefreshCw,
  Search,
  Sparkles,
  Trash2,
  Upload,
  X,
} from 'lucide-react'
import ChatArea, { type ChatContentRendererProps } from '../../components/ChatArea'
import { CleanConversationSurface } from '../../components/CleanConversationSurface'
import { FileContentViewer } from '../../components/FileContentViewer'
import { ConversationMarkdownRenderer } from '../../components/ui/MarkdownRenderer'
import { clampPanelWidth, loadStoredPanelWidth, saveStoredPanelWidth } from './panelWidth'
import { videoStamp } from './videoStamp'
import { ProductSurfaceSwitcher } from '../../components/ProductSurfaceSwitcher'
import SecretsManagerModal from '../../components/secrets/SecretsManagerModal'
import SecretSelectionDropdown from '../../components/secrets/SecretSelectionDropdown'
import { secretsApi, type WorkflowCredentialProvider } from '../../api/secrets'
import Workspace from '../../components/Workspace'
import { WorkflowCanvas } from '../../components/workflow/canvas/WorkflowCanvas'
import { PresentationRenderer, type PresentationRendererProps } from '../../platform/presentations/PresentationRenderer'
import { registerPresentationRenderer } from '../../platform/presentations/presentationRegistry'
import { usePresentationEvents } from '../../platform/presentations/usePresentationEvents'
import { agentApi } from '../../services/api'
import { setProductCommands } from '../../commands/registry'
import { sectionThatGrew, type ProductionCounts } from './productionPanelScroll'
import { toProductCommandDefinitions } from './productCommands'
import { useAppStore } from '../../stores/useAppStore'
import { useAuthStore } from '../../stores/useAuthStore'
import { useChatStore, waitForChatStoreHydration } from '../../stores/useChatStore'
import { useModeStore } from '../../stores/useModeStore'
import { useProductSurfaceStore } from '../../stores/useProductSurfaceStore'
import { restoreSession } from '../../utils/sessionRestore'
import {
  VIDEO_PROFILE_ID,
  VIDEO_PROFILE_VERSION,
  createVideoProject,
  loadVideoAgentProviderOptions,
  loadVideoPresentations,
  loadVideoProductCommands,
  loadCharacterPresentations,
  loadDocumentPresentations,
  loadVideoProjects,
  relativeTime,
  workspaceMediaURL,
  type VideoPresentation,
  type CharacterPresentation,
  type DocumentPresentation,
  type VideoAgentProviderOption,
  type VideoProject,
} from './videoStudioData'

type WorkspacePanel = 'production' | 'files' | 'workflow'
const EMPTY_SECRET_IDS: string[] = []
const EMPTY_VIDEO_AGENT_PROVIDER_OPTIONS: VideoAgentProviderOption[] = []

type ProviderCredentialDialogCopy = {
  title: string
  /** Shown under the title, e.g. "Optional for this project. Without one, Video Studio uses your saved AgentWorks Claude login." */
  subtitle: string
  hint: ReactNode
  fallbackLabel: string
  inputPlaceholder: string
  replacePlaceholder: string
  /** Noun used in buttons, e.g. "token" or "API key". */
  noun: string
}

/**
 * Per-project credential entry for a coding-CLI provider, scoped to this
 * project's workspace path. Claude Code, Cursor, and Pi CLI share this dialog
 * because all three need the same guarantee: without a scoped credential the
 * project falls back to whichever login/key is on the server, silently
 * billing that account. The backend already treats every provider
 * identically (workflowProviderAPIKeys in workflow_provider_auth.go); this
 * keeps the frontend the same way instead of growing one dialog per provider.
 */
function ProviderCredentialDialog({ provider, copy, workspacePath, onClose }: { provider: WorkflowCredentialProvider; copy: ProviderCredentialDialogCopy; workspacePath: string; onClose: () => void }) {
  const [token, setToken] = useState('')
  const [visible, setVisible] = useState(false)
  const [configured, setConfigured] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    void secretsApi.getWorkflowProviderCredentialStatus(provider, workspacePath)
      .then((status) => { if (!cancelled) setConfigured(status.configured) })
      .catch(() => { if (!cancelled) setError(`Unable to check the saved ${copy.noun}.`) })
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [provider, workspacePath, copy.noun])

  const save = async () => {
    if (!token.trim() || saving) return
    setSaving(true)
    setError('')
    try {
      await secretsApi.storeWorkflowProviderCredential(provider, workspacePath, token.trim())
      setToken('')
      setConfigured(true)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : `Unable to save the ${copy.noun}.`)
    } finally {
      setSaving(false)
    }
  }
  const remove = async () => {
    if (deleting) return
    setDeleting(true)
    setError('')
    try {
      await secretsApi.deleteWorkflowProviderCredential(provider, workspacePath)
      setConfigured(false)
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : `Unable to remove the ${copy.noun}.`)
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-[90] grid place-items-center bg-slate-950/45 p-4 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="provider-credential-title">
      <div className="w-full max-w-md rounded-2xl border border-slate-200 bg-white p-5 shadow-2xl dark:border-slate-700 dark:bg-slate-900">
        <div className="flex items-start justify-between gap-4">
          <div><h2 id="provider-credential-title" className="text-sm font-semibold text-slate-900 dark:text-slate-100">{copy.title}</h2><p className="mt-1 text-xs leading-5 text-slate-500 dark:text-slate-400">{copy.subtitle}</p></div>
          <button type="button" onClick={onClose} className="grid h-8 w-8 place-items-center rounded-lg text-slate-400 hover:bg-slate-100 hover:text-slate-700 dark:hover:bg-slate-800 dark:hover:text-slate-200" aria-label="Close credential setup"><X className="h-4 w-4" /></button>
        </div>
        <div className="mt-4 rounded-xl border border-slate-200 bg-slate-50 p-3 text-xs leading-5 text-slate-600 dark:border-slate-700 dark:bg-slate-950/40 dark:text-slate-300">
          {copy.hint}
        </div>
        <div className="mt-4 flex items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
          {loading ? <><Loader2 className="h-3.5 w-3.5 animate-spin" />Checking saved {copy.noun}…</> : configured ? <><CheckCircle2 className="h-3.5 w-3.5 text-emerald-500" />A project {copy.noun} is saved.</> : copy.fallbackLabel}
        </div>
        <div className="relative mt-3">
          <input type={visible ? 'text' : 'password'} autoComplete="off" value={token} onChange={(event) => { setToken(event.target.value); setError('') }} placeholder={configured ? copy.replacePlaceholder : copy.inputPlaceholder} className="h-10 w-full rounded-lg border border-slate-300 bg-white px-3 pr-10 font-mono text-sm text-slate-900 outline-none placeholder:text-slate-400 focus:border-violet-500 focus:ring-2 focus:ring-violet-100 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100 dark:focus:ring-violet-950" />
          <button type="button" onClick={() => setVisible((current) => !current)} className="absolute inset-y-0 right-0 grid w-10 place-items-center text-slate-400 hover:text-slate-700 dark:hover:text-slate-200" aria-label={visible ? `Hide ${copy.noun}` : `Show ${copy.noun}`}>{visible ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}</button>
        </div>
        {error ? <p role="alert" className="mt-2 text-xs text-rose-600 dark:text-rose-400">{error}</p> : null}
        <div className="mt-5 flex items-center justify-between gap-3">
          {configured ? <button type="button" onClick={() => void remove()} disabled={deleting || saving} className="inline-flex items-center gap-1.5 text-xs font-medium text-rose-600 hover:text-rose-700 disabled:opacity-50 dark:text-rose-400"><Trash2 className="h-3.5 w-3.5" />{deleting ? 'Removing…' : `Remove ${copy.noun}`}</button> : <span />}
          <button type="button" onClick={() => void save()} disabled={!token.trim() || saving || deleting} className="inline-flex h-9 items-center rounded-lg bg-violet-600 px-3 text-xs font-semibold text-white shadow-sm hover:bg-violet-700 disabled:cursor-not-allowed disabled:opacity-50">{saving ? <Loader2 className="mr-1.5 h-3.5 w-3.5 animate-spin" /> : null}{saving ? 'Saving…' : `Save ${copy.noun}`}</button>
        </div>
      </div>
    </div>
  )
}

const PROVIDER_CREDENTIAL_COPY: Partial<Record<WorkflowCredentialProvider, ProviderCredentialDialogCopy>> = {
  'claude-code': {
    title: 'Claude Code token',
    subtitle: 'Optional for this project. Without one, Video Studio uses your saved AgentWorks Claude login.',
    hint: <>Use a token from <code className="rounded bg-white px-1 py-0.5 font-mono text-[11px] dark:bg-slate-800">claude setup-token</code>. It is encrypted, private to this project, and never shown again.</>,
    fallbackLabel: 'Using saved AgentWorks login',
    inputPlaceholder: 'Paste Claude Code token',
    replacePlaceholder: 'Paste a replacement token',
    noun: 'token',
  },
  'cursor-cli': {
    title: 'Cursor API key',
    subtitle: 'Optional for this project. Without one, Video Studio uses your saved AgentWorks Cursor login.',
    hint: <>Paste an API key from <code className="rounded bg-white px-1 py-0.5 font-mono text-[11px] dark:bg-slate-800">cursor.com</code> settings. It is encrypted, private to this project, and never shown again.</>,
    fallbackLabel: 'Using saved AgentWorks login',
    inputPlaceholder: 'Paste Cursor API key',
    replacePlaceholder: 'Paste a replacement API key',
    noun: 'API key',
  },
  'pi-cli': {
    title: 'Pi CLI (Gemini) API key',
    subtitle: 'Optional for this project. Without one, Video Studio uses whichever Gemini key is configured on the server.',
    hint: <>Paste a Gemini API key from <code className="rounded bg-white px-1 py-0.5 font-mono text-[11px] dark:bg-slate-800">aistudio.google.com</code>. It is encrypted, private to this project, and never shown again.</>,
    fallbackLabel: 'Using the server-configured Gemini key',
    inputPlaceholder: 'Paste Gemini API key',
    replacePlaceholder: 'Paste a replacement API key',
    noun: 'API key',
  },
}

function VideoStudioHeader({ children, projectTabId }: { children?: ReactNode; projectTabId?: string | null }) {
  const user = useAuthStore((state) => state.user)
  const [showSecretsManager, setShowSecretsManager] = useState(false)
  const [showCredentialSetup, setShowCredentialSetup] = useState(false)
  const [providerOptions, setProviderOptions] = useState<VideoAgentProviderOption[]>(EMPTY_VIDEO_AGENT_PROVIDER_OPTIONS)
  // Keep the fallback reference stable. Zustand uses Object.is for selector
  // results, so allocating [] here causes useSyncExternalStore to re-render
  // forever while a project tab is still being restored.
  const selectedSecrets = useChatStore((state) => projectTabId ? state.chatTabs[projectTabId]?.config.selectedSecrets ?? EMPTY_SECRET_IDS : EMPTY_SECRET_IDS)
  const projectLLMConfig = useChatStore((state) => projectTabId ? state.chatTabs[projectTabId]?.config.llmConfig : undefined)
  const projectWorkspacePath = useChatStore((state) => projectTabId ? state.chatTabs[projectTabId]?.metadata?.agentProfileWorkspace : undefined)
  useEffect(() => {
    if (!projectTabId) return
    let cancelled = false
    void loadVideoAgentProviderOptions().then((options) => {
      if (!cancelled && options.length > 0) setProviderOptions(options)
    }).catch(() => {})
    return () => { cancelled = true }
  }, [projectTabId])
  const updateSelectedSecrets = (next: string[]) => {
    if (projectTabId) useChatStore.getState().setTabConfig(projectTabId, { selectedSecrets: next })
  }
  // Resolution is deliberately three-stage. An exact provider+model match wins;
  // failing that we match on PROVIDER ALONE, because a saved project pins a
  // model id that product.yaml can later rename (a project saved with
  // codex-cli/"codex-cli" no longer matched once the option moved to
  // gpt-5.6-terra). Without the provider-only stage that project fell through to
  // the isDefault option and the header rendered "Claude Code" while the request
  // still carried the stored provider — the label and the turn disagreed.
  const exactProviderOption = providerOptions.find((option) => option.provider === projectLLMConfig?.provider && option.modelId === projectLLMConfig?.model_id)
  const providerOnlyOption = providerOptions.find((option) => option.provider === projectLLMConfig?.provider)
  const resolvedProviderOption = exactProviderOption ?? providerOnlyOption ?? providerOptions.find((option) => option.isDefault) ?? providerOptions[0]
  const selectedProviderID = resolvedProviderOption?.id ?? ''
  const selectedProvider = resolvedProviderOption
  // Whatever the header ends up displaying is now written back as the project's
  // real configuration, so the dropdown can never be a cosmetic lie about which
  // provider the next turn will actually use.
  useEffect(() => {
    if (!projectTabId || !resolvedProviderOption || !projectLLMConfig) return
    if (projectLLMConfig.provider === resolvedProviderOption.provider && projectLLMConfig.model_id === resolvedProviderOption.modelId) return
    useChatStore.getState().setTabConfig(projectTabId, {
      llmConfig: {
        ...projectLLMConfig,
        provider: resolvedProviderOption.provider,
        model_id: resolvedProviderOption.modelId,
        fallback_models: [],
      },
    })
  }, [projectTabId, resolvedProviderOption, projectLLMConfig])
  // Only providers with a per-project credential (Claude Code, Cursor, Pi
  // CLI) show the key button; Codex has no scoped-credential story yet.
  const selectedCredentialCopy = selectedProvider ? PROVIDER_CREDENTIAL_COPY[selectedProvider.provider as WorkflowCredentialProvider] : undefined
  const updateProvider = (providerID: string) => {
    if (!projectTabId) return
    const option = providerOptions.find((candidate) => candidate.id === providerID)
    if (!option) return
    const current = useChatStore.getState().chatTabs[projectTabId]?.config.llmConfig
    useChatStore.getState().setTabConfig(projectTabId, {
      llmConfig: {
        ...current,
        provider: option.provider,
        model_id: option.modelId,
        fallback_models: [],
      },
    })
    // Prompt for the credential right away rather than leaving it to a small
    // icon in the corner: switching to a provider with no scoped credential
    // yet is exactly the moment a user needs to know one is available. Stays
    // quiet if a credential is already saved, so it doesn't nag every switch.
    const copy = PROVIDER_CREDENTIAL_COPY[option.provider as WorkflowCredentialProvider]
    if (copy && projectWorkspacePath) {
      void secretsApi.getWorkflowProviderCredentialStatus(option.provider as WorkflowCredentialProvider, projectWorkspacePath)
        .then((status) => {
          if (!status.configured) setShowCredentialSetup(true)
        })
        .catch(() => {})
    }
  }
  return (
    <header className="flex h-[62px] shrink-0 items-center justify-between border-b border-slate-200 bg-white px-4 dark:border-slate-800 dark:bg-slate-950">
      <div className="flex min-w-0 items-center gap-4">
        <ProductSurfaceSwitcher />
        {children}
      </div>
      <div className="flex items-center gap-2">
        {projectTabId ? (
          <>
            <div className="relative">
              <label className="sr-only" htmlFor={`video-agent-provider-${projectTabId}`}>Project agent provider</label>
              <select id={`video-agent-provider-${projectTabId}`} data-testid="video-studio-agent-provider-select" value={selectedProviderID} onChange={(event) => updateProvider(event.target.value)} className="h-8 appearance-none rounded-lg border border-slate-200 bg-slate-50 py-0 pl-3 pr-8 text-xs font-semibold text-slate-700 shadow-sm outline-none transition hover:border-violet-300 hover:bg-white focus:border-violet-500 focus:ring-2 focus:ring-violet-100 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200 dark:hover:border-violet-700 dark:hover:bg-slate-800 dark:focus:border-violet-500 dark:focus:ring-violet-950" aria-label="Project agent provider">
                {providerOptions.map((option) => <option key={option.id} value={option.id}>{option.label}</option>)}
              </select>
              <ChevronDown className="pointer-events-none absolute right-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
            </div>
            {selectedCredentialCopy && projectWorkspacePath ? <button type="button" onClick={() => setShowCredentialSetup(true)} className="grid h-8 w-8 place-items-center rounded-lg border border-slate-200 bg-slate-50 text-slate-500 shadow-sm transition hover:border-violet-300 hover:bg-violet-50 hover:text-violet-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300 dark:hover:border-violet-700 dark:hover:bg-violet-950/40 dark:hover:text-violet-300" aria-label={`Set up ${selectedCredentialCopy.noun}`} title={`Set up ${selectedCredentialCopy.noun}`}><KeyRound className="h-3.5 w-3.5" /></button> : null}
            <SecretSelectionDropdown
              selectedSecrets={selectedSecrets}
              onSecretToggle={(secretId) => updateSelectedSecrets(selectedSecrets.includes(secretId) ? selectedSecrets.filter((id) => id !== secretId) : [...selectedSecrets, secretId])}
              onSelectAll={updateSelectedSecrets}
              onClearAll={() => updateSelectedSecrets([])}
              placement="below"
              align="right"
            />
          </>
        ) : (
          <button
            type="button"
            onClick={() => setShowSecretsManager(true)}
            className="grid h-8 w-8 place-items-center rounded-full border border-amber-200 bg-amber-50 text-amber-700 transition hover:bg-amber-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-amber-500 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-300 dark:hover:bg-amber-950"
            aria-label="Manage secrets"
            title="Manage secrets"
          >
            <KeyRound className="h-3.5 w-3.5" />
          </button>
        )}
        <div className="hidden items-center gap-2 rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1.5 text-[11px] font-semibold text-emerald-700 sm:flex dark:border-emerald-900 dark:bg-emerald-950/50 dark:text-emerald-300">
          <span className="h-1.5 w-1.5 rounded-full bg-emerald-500" />
          {user?.username || user?.email || 'Signed in'}
        </div>
      </div>
      {showSecretsManager ? <SecretsManagerModal onClose={() => setShowSecretsManager(false)} /> : null}
      {showCredentialSetup && projectWorkspacePath && selectedProvider && selectedCredentialCopy ? <ProviderCredentialDialog provider={selectedProvider.provider as WorkflowCredentialProvider} copy={selectedCredentialCopy} workspacePath={projectWorkspacePath} onClose={() => setShowCredentialSetup(false)} /> : null}
    </header>
  )
}

function ProjectArtwork({ project, index }: { project: VideoProject; index: number }) {
  const gradients = [
    'from-indigo-700 via-violet-600 to-fuchsia-500',
    'from-slate-800 via-blue-700 to-cyan-500',
    'from-rose-800 via-orange-600 to-amber-400',
    'from-emerald-800 via-teal-600 to-sky-500',
  ]
  return (
    <div className={`relative h-36 overflow-hidden bg-gradient-to-br ${gradients[index % gradients.length]}`}>
      <div className="absolute -right-10 -top-20 h-48 w-48 rounded-full border border-white/20 shadow-[0_0_0_30px_rgba(255,255,255,0.05),0_0_0_62px_rgba(255,255,255,0.035)]" />
      <div className="absolute inset-0 opacity-20 [background-image:linear-gradient(rgba(255,255,255,.25)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,.25)_1px,transparent_1px)] [background-size:24px_24px] [mask-image:linear-gradient(120deg,#000,transparent_72%)]" />
      <div className="absolute left-1/2 top-1/2 grid h-16 w-24 -translate-x-1/2 -translate-y-1/2 place-items-center rounded-xl border border-white/30 bg-white/15 shadow-xl backdrop-blur-sm">
        <span className="grid h-9 w-9 place-items-center rounded-full bg-slate-950/35 text-white">
          <Play className="ml-0.5 h-4 w-4" fill="currentColor" />
        </span>
      </div>
      <span className="absolute bottom-3 left-3 inline-flex items-center gap-1.5 rounded-md bg-slate-950/45 px-2 py-1 text-[10px] font-semibold text-white backdrop-blur-sm">
        <Film className="h-3 w-3" /> {project.videos} {project.videos === 1 ? 'video' : 'videos'}
      </span>
    </div>
  )
}

function ProjectCard({ project, index, onOpen }: { project: VideoProject; index: number; onOpen: () => void }) {
  return (
    <button
      type="button"
      aria-label={`Open ${project.title}`}
      data-testid="video-studio-project-card"
      data-project-id={project.id}
      onClick={onOpen}
      className="group overflow-hidden rounded-2xl border border-slate-200 bg-white text-left shadow-sm transition hover:-translate-y-1 hover:border-violet-200 hover:shadow-xl hover:shadow-violet-950/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-500 dark:border-slate-800 dark:bg-slate-900"
    >
      <ProjectArtwork project={project} index={index} />
      <div className="p-4">
        <h3 className="truncate text-sm font-semibold text-slate-900 dark:text-slate-100">{project.title}</h3>
        <p className="mt-1.5 min-h-10 text-xs leading-5 text-slate-500 dark:text-slate-400">
          {project.description || 'A Video Studio project ready for its next creative brief.'}
        </p>
        <footer className="mt-4 flex items-center border-t border-slate-100 pt-3 text-[10px] font-medium text-slate-400 dark:border-slate-800">
          <Clock3 className="mr-1.5 h-3 w-3" /> {relativeTime(project.updatedAt)}
        </footer>
      </div>
    </button>
  )
}

function CreateProjectDialog({
  onClose,
  onCreate,
}: {
  onClose: () => void
  onCreate: (title: string, description: string) => Promise<void>
}) {
  const [title, setTitle] = useState('')
  const [description, setDescription] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!title.trim() || submitting) return
    setSubmitting(true)
    setError('')
    try {
      await onCreate(title.trim(), description.trim())
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not create the project.')
      setSubmitting(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-slate-950/50 p-5 backdrop-blur-sm" role="presentation">
      <form onSubmit={submit} className="relative w-full max-w-md rounded-3xl border border-white/60 bg-white p-7 shadow-2xl dark:border-slate-700 dark:bg-slate-900">
        <button type="button" onClick={onClose} disabled={submitting} aria-label="Close" className="absolute right-5 top-5 grid h-8 w-8 place-items-center rounded-lg text-slate-400 hover:bg-slate-100 disabled:opacity-50 dark:hover:bg-slate-800">
          <X className="h-4 w-4" />
        </button>
        <span className="grid h-11 w-11 place-items-center rounded-xl bg-violet-100 text-violet-700 dark:bg-violet-950 dark:text-violet-300">
          <Clapperboard className="h-5 w-5" />
        </span>
        <h2 className="mt-5 text-2xl font-semibold tracking-tight text-slate-950 dark:text-white">Create a video project</h2>
        <p className="mt-2 text-sm leading-6 text-slate-500">Conversation, source files, workflow runs, QA, and finished videos stay together.</p>
        <label className="mt-6 block text-xs font-semibold text-slate-700 dark:text-slate-300">
          Project name
          <input autoFocus data-testid="video-studio-create-project-name-input" value={title} onChange={(event) => setTitle(event.target.value)} maxLength={120} placeholder="Product story" className="mt-2 w-full rounded-xl border border-slate-200 bg-white px-3.5 py-3 text-sm font-normal outline-none focus:border-violet-400 focus:ring-4 focus:ring-violet-100 dark:border-slate-700 dark:bg-slate-950 dark:focus:ring-violet-950" />
        </label>
        <label className="mt-4 block text-xs font-semibold text-slate-700 dark:text-slate-300">
          Brief <span className="font-normal text-slate-400">(optional)</span>
          <textarea value={description} onChange={(event) => setDescription(event.target.value)} maxLength={1000} rows={3} placeholder="What are we making?" className="mt-2 w-full resize-none rounded-xl border border-slate-200 bg-white px-3.5 py-3 text-sm font-normal outline-none focus:border-violet-400 focus:ring-4 focus:ring-violet-100 dark:border-slate-700 dark:bg-slate-950 dark:focus:ring-violet-950" />
        </label>
        {error ? <p className="mt-3 flex items-center gap-2 text-xs text-red-600"><AlertCircle className="h-3.5 w-3.5" />{error}</p> : null}
        <div className="mt-6 flex justify-end gap-2">
          <button type="button" onClick={onClose} disabled={submitting} className="rounded-xl px-4 py-2.5 text-xs font-semibold text-slate-600 hover:bg-slate-100 disabled:opacity-50 dark:text-slate-300 dark:hover:bg-slate-800">Cancel</button>
          <button type="submit" data-testid="video-studio-create-project-submit" disabled={!title.trim() || submitting} className="inline-flex min-w-32 items-center justify-center gap-2 rounded-xl bg-violet-600 px-4 py-2.5 text-xs font-semibold text-white shadow-lg shadow-violet-600/20 hover:bg-violet-500 disabled:cursor-not-allowed disabled:opacity-50">
            {submitting ? <Loader2 className="h-4 w-4 animate-spin" /> : <Plus className="h-4 w-4" />}
            {submitting ? 'Creating…' : 'Create project'}
          </button>
        </div>
      </form>
    </div>
  )
}

function ProjectChatWelcome({ project }: { project: VideoProject }) {
  return (
    <div className="flex h-full items-center justify-center px-6 py-10">
      <div className="max-w-md text-center">
        <span className="mx-auto grid h-14 w-14 place-items-center rounded-2xl bg-violet-100 text-violet-700 dark:bg-violet-950 dark:text-violet-300">
          <Sparkles className="h-6 w-6" />
        </span>
        <h2 className="mt-5 text-lg font-semibold text-slate-900 dark:text-slate-100">What should we create?</h2>
        <p className="mt-2 text-sm leading-6 text-slate-500 dark:text-slate-400">
          Describe the audience, goal, format, and source material. Video Studio will choose a direct task or the appropriate production workflow.
        </p>
        {project.description ? <p className="mt-4 rounded-xl border border-violet-100 bg-violet-50 px-4 py-3 text-xs leading-5 text-violet-800 dark:border-violet-900 dark:bg-violet-950/40 dark:text-violet-200">{project.description}</p> : null}
      </div>
    </div>
  )
}

function VideoStudioConversation(props: ChatContentRendererProps) {
  return (
    <CleanConversationSurface
      {...props}
      // The formatted response stream is safe to show as live working notes.
      // Raw tmux bytes, terminal controls, and private provider internals remain
      // outside the product surface.
    />
  )
}

function MediaVideoPresentation({ presentation, workspacePath }: PresentationRendererProps) {
  const videoRef = useRef<HTMLVideoElement>(null)
  const [loading, setLoading] = useState(true)
  const [playbackError, setPlaybackError] = useState('')
  const relativePath = typeof presentation.payload.path === 'string' ? presentation.payload.path.replace(/^\/+/, '') : ''
  if (!relativePath) return <div className="grid aspect-video place-items-center bg-slate-950 text-xs text-slate-400">Video path is unavailable.</div>
  const mediaURL = workspaceMediaURL(`${workspacePath}/${relativePath}`)
  const retryPlayback = () => {
    setPlaybackError('')
    setLoading(true)
    videoRef.current?.load()
  }
  return (
    <div className="relative aspect-video w-full bg-black">
      <video
        ref={videoRef}
        key={`${presentation.id}:${presentation.revision}`}
        data-testid="video-studio-presentation-player"
        data-presentation-id={presentation.id}
        data-presentation-revision={presentation.revision}
        controls
        playsInline
        preload="auto"
        className="h-full w-full bg-black object-contain"
        src={mediaURL}
        onLoadStart={() => { setLoading(true); setPlaybackError('') }}
        onLoadedMetadata={() => setLoading(false)}
        onCanPlay={() => setLoading(false)}
        onError={() => {
          setLoading(false)
          setPlaybackError('This video could not be loaded. Refresh it to reconnect to the project file.')
        }}
      >
        Your browser does not support video playback.
      </video>
      {loading && !playbackError ? <div className="pointer-events-none absolute inset-0 grid place-items-center bg-black/25 text-xs font-medium text-white/80"><span className="flex items-center gap-2"><Loader2 className="h-4 w-4 animate-spin" /> Loading video…</span></div> : null}
      {playbackError ? <div className="absolute inset-0 grid place-items-center bg-slate-950/95 p-6 text-center"><div><AlertCircle className="mx-auto h-6 w-6 text-amber-400" /><p className="mt-2 max-w-xs text-xs leading-5 text-slate-300">{playbackError}</p><button type="button" onClick={retryPlayback} className="mt-3 rounded-lg bg-white px-3 py-2 text-xs font-semibold text-slate-900 hover:bg-slate-100">Reload video</button></div></div> : null}
    </div>
  )
}

registerPresentationRenderer('media.video', MediaVideoPresentation)

// One collapsible section of the Production panel. Sections with nothing in
// them are not rendered at all rather than shown empty: a production moves
// through documents, then characters, then a finished video, so an empty
// section is a stage that has not happened yet, and showing it as a header
// with a zero next to it just makes the panel longer without saying anything.
function ProductionSection({ id, title, count, icon, children, forceOpenKey }: { id: string; title: string; count: number; icon: ReactNode; children: ReactNode; forceOpenKey?: number }) {
  const [open, setOpen] = useState(true)
  // A section a user collapsed should reopen when something new lands in it --
  // scrolling to a section that is still closed would show them nothing.
  const lastForceOpenKey = useRef(forceOpenKey)
  useEffect(() => {
    if (forceOpenKey !== undefined && forceOpenKey !== lastForceOpenKey.current) {
      lastForceOpenKey.current = forceOpenKey
      setOpen(true)
    }
  }, [forceOpenKey])
  return (
    <section data-testid={`video-studio-section-${id}`} data-section-id={id} data-section-count={count} className="border-b border-slate-200 last:border-b-0 dark:border-slate-800">
      <button type="button" onClick={() => setOpen((value) => !value)} aria-expanded={open} className="flex w-full items-center gap-2 px-4 py-2.5 text-left text-xs font-semibold text-slate-700 hover:bg-slate-100 dark:text-slate-200 dark:hover:bg-slate-800/60">
        <ChevronDown className={`h-3.5 w-3.5 shrink-0 text-slate-400 transition-transform ${open ? '' : '-rotate-90'}`} />
        <span className="grid h-5 w-5 shrink-0 place-items-center text-slate-400">{icon}</span>
        <span className="flex-1 truncate">{title}</span>
        <span className="shrink-0 rounded-md bg-slate-200 px-1.5 py-0.5 text-[9px] font-semibold text-slate-600 dark:bg-slate-800 dark:text-slate-300">{count}</span>
      </button>
      {open ? <div className="px-4 pb-4">{children}</div> : null}
    </section>
  )
}

function VideosSection({ project, videos }: { project: VideoProject; videos: VideoPresentation[] }) {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const selected = videos.find((video) => video.id === selectedId) || videos[0]
  useEffect(() => {
    if (videos.length === 0) setSelectedId(null)
    else if (!videos.some((video) => video.id === selectedId)) setSelectedId(videos[0].id)
  }, [selectedId, videos])
  if (!selected) return null

  const mediaURL = workspaceMediaURL(`${project.workspacePath}/${selected.path.replace(/^\/+/, '')}`)
  return (
    <ProductionSection id="videos" title="Videos" count={videos.length} icon={<Film className="h-3.5 w-3.5" />} forceOpenKey={videos.length}>
      <div data-testid="video-studio-videos-panel" data-video-count={videos.length}>
        <div className="overflow-hidden rounded-2xl bg-black shadow-lg">
          <PresentationRenderer presentation={selected.workspacePresentation} workspacePath={project.workspacePath} fallback={<div className="grid aspect-video place-items-center text-xs text-slate-400">No renderer is registered for this presentation.</div>} />
        </div>
        <div className="mt-3 flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="truncate text-sm font-semibold text-slate-900 dark:text-slate-100">{selected.title}</h3>
            <p className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-[10px] font-medium">
              <span className="flex items-center gap-1.5 text-emerald-600 dark:text-emerald-400"><CheckCircle2 className="h-3 w-3" /> QA {selected.verdict || 'passed'}</span>
              {videoStamp(selected.updatedAt).short ? <span data-testid="video-studio-video-timestamp" title={videoStamp(selected.updatedAt).full} className="text-slate-500 dark:text-slate-400">{videoStamp(selected.updatedAt).short}{selected.revision > 1 ? ` · rev ${selected.revision}` : ''}</span> : null}
            </p>
          </div>
          <a href={mediaURL} download className="shrink-0 rounded-lg border border-slate-200 px-2.5 py-1.5 text-[10px] font-semibold text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">Download</a>
        </div>
        {selected.note ? <p className="mt-3 rounded-xl bg-slate-100 p-3 text-xs leading-5 text-slate-600 dark:bg-slate-800 dark:text-slate-300">{selected.note}</p> : null}
        {videos.length > 1 ? (
          <div className="mt-4 space-y-2">
            {videos.map((video) => (
              <button key={video.id} type="button" data-testid="video-studio-video-list-item" data-video-id={video.id} data-selected={video.id === selected.id} onClick={() => setSelectedId(video.id)} className={`flex w-full items-center gap-3 rounded-xl border p-2 text-left ${video.id === selected.id ? 'border-violet-500/60 bg-slate-900 shadow-sm shadow-violet-950/30' : 'border-slate-200 bg-white hover:bg-slate-50 dark:border-slate-800 dark:bg-slate-900 dark:hover:bg-slate-800'}`}>
                <span className="grid h-9 w-12 shrink-0 place-items-center rounded-lg bg-slate-950 text-white"><Play className="h-3.5 w-3.5" fill="currentColor" /></span>
                <span className="min-w-0"><strong className={`block truncate text-xs ${video.id === selected.id ? 'text-slate-50' : 'text-slate-800 dark:text-slate-200'}`}>{video.title}</strong><small title={videoStamp(video.updatedAt).full} className={video.id === selected.id ? 'text-[10px] text-slate-300' : 'text-[10px] text-slate-400'}>{videoStamp(video.updatedAt).short || `Revision ${video.revision}`}{video.revision > 1 ? ` · rev ${video.revision}` : ''}</small></span>
              </button>
            ))}
          </div>
        ) : null}
      </div>
    </ProductionSection>
  )
}

// Everything the production has made, in the order it makes it: the finished
// video when there is one, the characters every shot is generated against, and
// the written artifacts approved between stages. These were three sibling tabs,
// which hid the sequence and made Characters something you had to already know
// to click.
function ProductionPanel({ project, videos, characters, documents }: { project: VideoProject; videos: VideoPresentation[]; characters: CharacterPresentation[]; documents: DocumentPresentation[] }) {
  const [isDraggingAssets, setIsDraggingAssets] = useState(false)
  const [uploadingAssets, setUploadingAssets] = useState(false)
  const [uploadMessage, setUploadMessage] = useState('')
  const uploadInputRef = useRef<HTMLInputElement>(null)
  const uploadDestination = `${project.workspacePath.replace(/\/$/, '')}/uploads`
  const isEmpty = videos.length === 0 && characters.length === 0 && documents.length === 0
  const panelRef = useRef<HTMLDivElement>(null)

  // Sections stack Videos / Characters / Documents in one scroll, so new
  // content lower down is easy to miss -- reported directly: a character was
  // shown successfully but sat off-screen below the videos with nothing to
  // draw the eye there. When a count grows, scroll that section into view;
  // characters take priority over documents over videos, since an unapproved
  // character is the more time-sensitive thing to see. The first render is
  // excluded -- an existing project opening with content already in every
  // section has nothing single "new" to jump to.
  const knownCounts = useRef<ProductionCounts | null>(null)
  useEffect(() => {
    const previous = knownCounts.current
    const current = { videos: videos.length, characters: characters.length, documents: documents.length }
    knownCounts.current = current
    const grew = sectionThatGrew(previous, current)
    if (!grew) return
    // Sections mount their content synchronously when forced open, but give
    // the reopen a frame before measuring/scrolling to it.
    requestAnimationFrame(() => {
      panelRef.current?.querySelector(`[data-section-id="${grew}"]`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    })
  }, [videos.length, characters.length, documents.length])

  const uploadAssets = async (files: File[]) => {
    if (files.length === 0 || uploadingAssets) return
    setUploadingAssets(true)
    setUploadMessage('')
    const failed: string[] = []
    let uploaded = 0
    for (const file of files) {
      try {
        await agentApi.uploadPlannerFile(file, uploadDestination, `Add ${file.name} as a project reference`)
        uploaded += 1
      } catch (error) {
        failed.push(`${file.name}: ${error instanceof Error ? error.message : 'Upload failed'}`)
      }
    }
    setUploadingAssets(false)
    setUploadMessage(failed.length > 0
      ? (uploaded > 0 ? `${uploaded} file${uploaded === 1 ? '' : 's'} added. ${failed[0]}` : failed[0])
      : `${uploaded} reference ${uploaded === 1 ? 'file' : 'files'} added to this project.`)
  }

  const uploadDropZone = (
    <>
      <input ref={uploadInputRef} type="file" multiple className="hidden" onChange={(event) => {
        const files = event.target.files ? Array.from(event.target.files) : []
        event.target.value = ''
        void uploadAssets(files)
      }} />
      <div
        onDragOver={(event) => { event.preventDefault(); setIsDraggingAssets(true) }}
        onDragLeave={(event) => { event.preventDefault(); setIsDraggingAssets(false) }}
        onDrop={(event) => { event.preventDefault(); setIsDraggingAssets(false); void uploadAssets(Array.from(event.dataTransfer.files)) }}
        className={`rounded-xl border border-dashed p-3 transition ${isDraggingAssets ? 'border-violet-500 bg-violet-50 dark:bg-violet-950/30' : 'border-slate-300 bg-slate-50 dark:border-slate-700 dark:bg-slate-900/60'}`}
      >
        <div className="flex items-center justify-between gap-3">
          <p className="text-xs leading-5 text-slate-500 dark:text-slate-400">Drop reference files here, or browse to add images, clips, audio, or documents.</p>
          <button type="button" onClick={() => uploadInputRef.current?.click()} disabled={uploadingAssets} className="inline-flex shrink-0 items-center gap-1.5 rounded-lg bg-violet-600 px-2.5 py-1.5 text-xs font-semibold text-white shadow-sm transition hover:bg-violet-500 disabled:cursor-not-allowed disabled:opacity-60">
            {uploadingAssets ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Upload className="h-3.5 w-3.5" />}{uploadingAssets ? 'Adding…' : 'Add files'}
          </button>
        </div>
        {uploadMessage ? <p role="status" className="mt-2 text-[11px] text-slate-500 dark:text-slate-400">{uploadMessage}</p> : null}
      </div>
    </>
  )

  return (
    <div ref={panelRef} data-testid="video-studio-production-panel" data-video-count={videos.length} data-character-count={characters.length} data-document-count={documents.length} className="min-h-0 flex-1 overflow-y-auto">
      <div className="p-4 pb-3">{uploadDropZone}</div>
      {isEmpty ? (
        <div className="px-8 pb-10 pt-4 text-center">
          <span className="mx-auto grid h-14 w-14 place-items-center rounded-2xl border border-dashed border-violet-300 bg-white text-violet-500 dark:border-violet-800 dark:bg-slate-900"><Film className="h-6 w-6" /></span>
          <h2 className="mt-4 text-sm font-semibold text-slate-800 dark:text-slate-200">Nothing produced yet</h2>
          <p className="mt-1 text-xs leading-5 text-slate-400">Briefs, scripts, character references, and the finished video appear here as each stage completes.</p>
        </div>
      ) : (
        <>
          <VideosSection project={project} videos={videos} />
          <CharactersSection project={project} characters={characters} />
          <DocumentsSection documents={documents} />
        </>
      )}
    </div>
  )
}

// Characters are shown before the shots that use them exist, so this panel is
// an approval surface rather than a gallery: the reference image is displayed
// at a size worth judging a face at, beside the exact spec text that will be
// repeated verbatim into every prompt, and the model the subject's whole arc
// is committed to.
function CharactersSection({ project, characters }: { project: VideoProject; characters: CharacterPresentation[] }) {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const selected = characters.find((character) => character.id === selectedId) || characters[0]
  useEffect(() => {
    if (characters.length === 0) setSelectedId(null)
    else if (!characters.some((character) => character.id === selectedId)) setSelectedId(characters[0].id)
  }, [selectedId, characters])
  if (!selected) return null

  const imageURL = workspaceMediaURL(`${project.workspacePath}/${selected.imagePath.replace(/^\/+/, '')}`)
  return (
    <ProductionSection id="characters" title="Characters" count={characters.length} icon={<Users className="h-3.5 w-3.5" />} forceOpenKey={characters.length}>
    <div data-testid="video-studio-characters-panel" data-character-count={characters.length}>
      <div className="overflow-hidden rounded-2xl border border-slate-200 bg-slate-100 dark:border-slate-800 dark:bg-slate-900">
        <img src={imageURL} alt={`Reference image for ${selected.name}`} data-testid="video-studio-character-reference" className="max-h-[22rem] w-full bg-slate-950 object-contain" />
      </div>
      <div className="mt-3 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold text-slate-900 dark:text-slate-100">{selected.name}</h3>
          <p className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-[10px] font-medium text-slate-500 dark:text-slate-400">
            {selected.model ? <span data-testid="video-studio-character-model" className="rounded bg-slate-200 px-1.5 py-0.5 dark:bg-slate-800">{selected.provider ? `${selected.provider} · ` : ''}{selected.model}</span> : null}
            {videoStamp(selected.updatedAt).short ? <span title={videoStamp(selected.updatedAt).full}>{videoStamp(selected.updatedAt).short}{selected.revision > 1 ? ` · rev ${selected.revision}` : ''}</span> : null}
          </p>
        </div>
        <a href={imageURL} download className="shrink-0 rounded-lg border border-slate-200 px-2.5 py-1.5 text-[10px] font-semibold text-slate-600 hover:bg-slate-100 dark:border-slate-700 dark:text-slate-300 dark:hover:bg-slate-800">Download</a>
      </div>
      {selected.note ? <p className="mt-3 rounded-xl bg-slate-100 p-3 text-xs leading-5 text-slate-600 dark:bg-slate-800 dark:text-slate-300">{selected.note}</p> : null}
      {selected.spec ? (
        <div data-testid="video-studio-character-spec" className="mt-3 max-h-72 overflow-auto rounded-xl border border-slate-200 bg-white p-3 text-xs leading-5 text-slate-700 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300">
          <ConversationMarkdownRenderer content={selected.spec} maxHeight="none" framed={false} />
        </div>
      ) : null}
      {characters.length > 1 ? (
        <div className="mt-5 grid grid-cols-2 gap-2 border-t border-slate-200 pt-4 sm:grid-cols-3 dark:border-slate-800">
          {characters.map((character) => (
            <button key={character.id} type="button" data-testid="video-studio-character-list-item" data-character-id={character.id} data-selected={character.id === selected.id} onClick={() => setSelectedId(character.id)} className={`overflow-hidden rounded-xl border text-left ${character.id === selected.id ? 'border-violet-500/60 shadow-sm' : 'border-slate-200 hover:bg-slate-50 dark:border-slate-800 dark:hover:bg-slate-800'}`}>
              <img src={workspaceMediaURL(`${project.workspacePath}/${character.imagePath.replace(/^\/+/, '')}`)} alt="" className="h-20 w-full bg-slate-950 object-cover" />
              <strong className="block truncate px-2 py-1.5 text-[11px] text-slate-800 dark:text-slate-200">{character.name}</strong>
            </button>
          ))}
        </div>
      ) : null}
    </div>
    </ProductionSection>
  )
}

// The written artifacts a stage produces are what the user approves between
// stages. Rendered as plain text rather than parsed markdown: these are
// working documents read for their content, and a faithful monospace view
// cannot silently drop a heading or a table the way a partial renderer can.
function DocumentsSection({ documents }: { documents: DocumentPresentation[] }) {
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const selected = documents.find((document) => document.id === selectedId) || documents[0]
  useEffect(() => {
    if (documents.length === 0) setSelectedId(null)
    else if (!documents.some((document) => document.id === selectedId)) setSelectedId(documents[0].id)
  }, [selectedId, documents])
  if (!selected) return null

  return (
    <ProductionSection id="documents" title="Documents" count={documents.length} icon={<FileText className="h-3.5 w-3.5" />} forceOpenKey={documents.length}>
    <div data-testid="video-studio-documents-panel" data-document-count={documents.length}>
      {documents.length > 1 ? (
        <div className="mb-3 flex flex-wrap gap-1.5">
          {documents.map((document) => (
            <button key={document.id} type="button" data-testid="video-studio-document-list-item" data-document-id={document.id} data-selected={document.id === selected.id} onClick={() => setSelectedId(document.id)} className={`rounded-lg border px-2.5 py-1.5 text-[11px] font-semibold ${document.id === selected.id ? 'border-violet-500/60 bg-violet-50 text-violet-700 dark:bg-violet-950/40 dark:text-violet-300' : 'border-slate-200 text-slate-600 hover:bg-slate-50 dark:border-slate-800 dark:text-slate-300 dark:hover:bg-slate-800'}`}>{document.title}</button>
          ))}
        </div>
      ) : null}
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold text-slate-900 dark:text-slate-100">{selected.title}</h3>
          <p className="mt-0.5 truncate text-[10px] text-slate-400" title={selected.path}>{selected.path}</p>
        </div>
        {videoStamp(selected.updatedAt).short ? <span title={videoStamp(selected.updatedAt).full} className="shrink-0 text-[10px] font-medium text-slate-500 dark:text-slate-400">{videoStamp(selected.updatedAt).short}{selected.revision > 1 ? ` · rev ${selected.revision}` : ''}</span> : null}
      </div>
      {selected.note ? <p className="mt-3 rounded-xl bg-slate-100 p-3 text-xs leading-5 text-slate-600 dark:bg-slate-800 dark:text-slate-300">{selected.note}</p> : null}
      <div data-testid="video-studio-document-body" className="mt-3 max-h-[32rem] overflow-auto rounded-xl border border-slate-200 bg-white p-4 text-xs leading-5 text-slate-700 dark:border-slate-800 dark:bg-slate-900 dark:text-slate-300">
        <ConversationMarkdownRenderer content={selected.markdown} maxHeight="none" framed={false} />
      </div>
    </div>
    </ProductionSection>
  )
}

function FilesPanel({ project }: { project: VideoProject }) {
  return (
    <div className="h-full min-h-0 overflow-hidden" data-testid="video-studio-files-panel">
      <Workspace minimized={false} onToggleMinimize={() => {}} hideMinimizeControl scopedWorkspacePath={project.workspacePath} hiddenRootFolders={['.git', 'node_modules']} title="Project files" />
    </div>
  )
}

function WorkflowPanel({ project }: { project: VideoProject }) {
  return (
    <div className="h-full min-h-0 overflow-hidden" data-testid="video-studio-workflow-panel">
      <WorkflowCanvas workspacePath={project.workspacePath} presetQueryId={null} readOnly hideToolbar embeddedPlanOnly />
    </div>
  )
}

function ProjectWorkspace({ project, onBack }: { project: VideoProject; onBack: () => void }) {
  const [tabId, setTabId] = useState<string | null>(null)
  const [panel, setPanel] = useState<WorkspacePanel>('production')
  const [videos, setVideos] = useState<VideoPresentation[]>([])
  const [characters, setCharacters] = useState<CharacterPresentation[]>([])
  const [documents, setDocuments] = useState<DocumentPresentation[]>([])
  const [showVideoPlayer, setShowVideoPlayer] = useState(false)
  const [loadingProject, setLoadingProject] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [loadError, setLoadError] = useState('')
  const videoCountRef = useRef(0)

  // Production panel width: draggable, and remembered per browser rather than
  // reset every visit. Read once on mount rather than on every render -- the
  // stored value only matters as a starting point, not something to keep
  // re-syncing against.
  const [panelWidth, setPanelWidth] = useState(() => loadStoredPanelWidth())
  const [isResizingPanel, setIsResizingPanel] = useState(false)
  const layoutGridRef = useRef<HTMLDivElement>(null)
  const startPanelResize = useCallback((event: React.PointerEvent) => {
    event.preventDefault()
    const grid = layoutGridRef.current
    if (!grid) return
    setIsResizingPanel(true)
    const handlePointerMove = (moveEvent: PointerEvent) => {
      const gridRight = grid.getBoundingClientRect().right
      setPanelWidth(clampPanelWidth(gridRight - moveEvent.clientX))
    }
    const stopResize = () => {
      setIsResizingPanel(false)
      window.removeEventListener('pointermove', handlePointerMove)
      window.removeEventListener('pointerup', stopResize)
      // Persist once at the end of the drag rather than on every pointermove
      // -- localStorage.setItem on a mousemove-frequency callback is wasted
      // work for a value nobody reads until the next page load.
      setPanelWidth((current) => { saveStoredPanelWidth(current); return current })
    }
    window.addEventListener('pointermove', handlePointerMove)
    window.addEventListener('pointerup', stopResize)
  }, [])

  const refreshProject = useCallback(async (quiet = false) => {
    if (!quiet) setRefreshing(true)
    try {
      // Loaded together so one refresh reflects the whole production. Only the
      // video count auto-opens the player; a new character or document should
      // not yank the user out of whatever they were reading.
      const [nextVideos, nextCharacters, nextDocuments] = await Promise.all([
        loadVideoPresentations(project),
        loadCharacterPresentations(project),
        loadDocumentPresentations(project),
      ])
      if (nextVideos.length > videoCountRef.current) setShowVideoPlayer(true)
      videoCountRef.current = nextVideos.length
      setVideos(nextVideos)
      setCharacters(nextCharacters)
      setDocuments(nextDocuments)
      setLoadError('')
    } catch (cause) {
      // These loads now distinguish "no managed database yet" (empty, and
      // still resolves normally) from a real failure. A real one must not be
      // shown as an empty production -- keep whatever was already loaded and
      // say so, rather than blanking the panel with no way to tell why.
      setLoadError(cause instanceof Error ? cause.message : 'Could not load this production.')
    } finally {
      setRefreshing(false)
    }
  }, [project])

  useEffect(() => {
    let cancelled = false
    const prepare = async () => {
      useModeStore.getState().setModeCategory('multi-agent')
      useAppStore.getState().setAgentMode('multi-agent')
      await waitForChatStoreHydration()
      if (cancelled) return
      const chatStore = useChatStore.getState()
      let projectTab = Object.values(chatStore.chatTabs).find((tab) =>
        tab.metadata?.agentProfileId === VIDEO_PROFILE_ID &&
        tab.metadata?.agentProfileWorkspace === project.workspacePath
      )
      if (projectTab && projectTab.metadata?.agentProfileVersion !== VIDEO_PROFILE_VERSION) {
        chatStore.setTabMetadata(projectTab.tabId, { agentProfileVersion: VIDEO_PROFILE_VERSION })
        projectTab = chatStore.getTab(projectTab.tabId)
      }
      if (!projectTab) {
        const createdTabId = await chatStore.createChatTab(project.title, {
          mode: 'multi-agent',
          agentProfileId: VIDEO_PROFILE_ID,
          agentProfileVersion: VIDEO_PROFILE_VERSION,
          agentProfileWorkspace: project.workspacePath,
          agentProfileProjectId: project.id,
          agentProfileProjectTitle: project.title,
          agentProfileWorkspaceDescription: project.description,
        }, project.sessionId)
        projectTab = chatStore.getTab(createdTabId)
      }
      if (cancelled || !projectTab) return
      const restoredTabId = await restoreSession(project.sessionId, {
        title: project.title,
        source: 'video-project-open',
        skipConfigRestore: true,
      })
      if (cancelled) return
      chatStore.switchTab(restoredTabId)
      setTabId(restoredTabId)
    }
    void prepare()
    return () => { cancelled = true }
  }, [project])

  useEffect(() => {
    let cancelled = false
    void refreshProject().finally(() => { if (!cancelled) setLoadingProject(false) })
    // The 5s interval is the fallback: it works even if this tab's SSE
    // connection to project.sessionId is momentarily down, and it is what
    // was picking up new videos before the event path below existed.
    const interval = window.setInterval(() => { void refreshProject(true) }, 5000)
    return () => {
      cancelled = true
      window.clearInterval(interval)
    }
  }, [refreshProject])

  // Instant path: react to this session's own presentation_updated events
  // instead of waiting up to 5s for the poll above. Reuses the SSE stream
  // useChatStore already keeps open for the chat transcript -- see
  // usePresentationEvents for why this does not open a second connection.
  const presentationEvents = usePresentationEvents(project.sessionId, ['media.video', 'media.character', 'document.markdown'])
  const handledPresentationEventCountRef = useRef(0)
  useEffect(() => {
    if (presentationEvents.length <= handledPresentationEventCountRef.current) return
    handledPresentationEventCountRef.current = presentationEvents.length
    void refreshProject(true)
  }, [presentationEvents, refreshProject])

  return (
    <div className="flex h-screen min-h-0 flex-col overflow-hidden bg-slate-50 dark:bg-slate-950">
      <VideoStudioHeader projectTabId={tabId}>
        <div className="hidden h-7 w-px bg-slate-200 sm:block dark:bg-slate-800" />
        <button type="button" onClick={onBack} className="grid h-8 w-8 place-items-center rounded-lg border border-slate-200 text-slate-500 hover:bg-slate-50 dark:border-slate-700 dark:hover:bg-slate-800" aria-label="Back to projects"><ArrowLeft className="h-4 w-4" /></button>
        <div className="min-w-0"><span className="block text-[10px] font-medium text-slate-400">Projects /</span><h1 className="truncate text-sm font-semibold text-slate-900 dark:text-slate-100">{project.title}</h1></div>
        {videos.length > 0 ? <button type="button" onClick={() => setShowVideoPlayer(true)} className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-lg bg-violet-600 px-2.5 text-xs font-semibold text-white shadow-sm hover:bg-violet-500 lg:hidden" aria-label="Play presented video"><Play className="h-3.5 w-3.5" fill="currentColor" />Play</button> : null}
      </VideoStudioHeader>
      <div
        ref={layoutGridRef}
        className="grid min-h-0 flex-1 lg:grid-cols-[minmax(440px,1fr)_auto_var(--vs-panel-width)]"
        style={{ '--vs-panel-width': `${clampPanelWidth(panelWidth)}px` } as React.CSSProperties}
      >
        <main className="flex min-h-0 flex-col overflow-hidden bg-white dark:bg-slate-950">
          <div className="min-h-0 flex-1">
            {tabId ? (
              <ChatArea
                tabId={tabId}
                onNewChat={() => {}}
                landingContent={<ProjectChatWelcome project={project} />}
                contentRenderer={VideoStudioConversation}
                inputVariant="product"
                fullTurnStreaming
                showConversationUsage
              />
            ) : <div className="grid h-full place-items-center text-xs text-slate-400"><Loader2 className="mr-2 inline h-4 w-4 animate-spin" />Connecting project agent…</div>}
          </div>
        </main>
        <div
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize the production panel"
          onPointerDown={startPanelResize}
          className={`relative hidden w-1.5 shrink-0 cursor-col-resize touch-none lg:block ${isResizingPanel ? 'bg-violet-400 dark:bg-violet-600' : 'bg-transparent hover:bg-violet-200 dark:hover:bg-violet-900'}`}
        >
          {/* Wider than the visible bar so the drag target is comfortable to grab
              without making the resting divider look thick. */}
          <div className="absolute inset-y-0 -left-1.5 -right-1.5" />
        </div>
        <aside className="hidden min-h-0 border-l border-slate-200 bg-slate-50 lg:flex lg:flex-col dark:border-slate-800 dark:bg-slate-900/40">
          <div className="flex h-14 shrink-0 items-center justify-between border-b border-slate-200 px-3 dark:border-slate-800">
            <div className="flex h-full items-center">
              {(['production', 'files', 'workflow'] as WorkspacePanel[]).map((item) => {
                const count = item === 'production' ? videos.length + characters.length + documents.length : 0
                return (
                  <button key={item} type="button" onClick={() => setPanel(item)} className={`h-full border-b-2 px-3 text-xs font-semibold capitalize ${panel === item ? 'border-violet-600 text-violet-700 dark:text-violet-300' : 'border-transparent text-slate-400 hover:text-slate-700 dark:hover:text-slate-200'}`}>
                    {item}{count > 0 ? <span className="ml-1 rounded-md bg-violet-100 px-1.5 py-0.5 text-[9px] dark:bg-violet-950">{count}</span> : null}
                  </button>
                )
              })}
            </div>
            <button type="button" onClick={() => void refreshProject()} disabled={refreshing} aria-label="Refresh project panel" className="grid h-8 w-8 place-items-center rounded-lg text-slate-400 hover:bg-slate-200 hover:text-slate-700 disabled:opacity-50 dark:hover:bg-slate-800 dark:hover:text-slate-200"><RefreshCw className={`h-3.5 w-3.5 ${refreshing ? 'animate-spin' : ''}`} /></button>
          </div>
          {/* Production is the fallback, not workflow: a panel value this build
              no longer knows -- a renamed tab surviving a hot reload, or a
              persisted preference from an older version -- should land on the
              work itself rather than silently showing the workflow canvas where
              the user's video used to be. */}
          {/* Shown above the panel rather than replacing it: a failed refresh
              should not throw away the production the user can already see. */}
          {loadError ? (
            <div role="alert" data-testid="video-studio-load-error" className="mx-3 mt-3 shrink-0 rounded-xl border border-amber-300 bg-amber-50 p-3 text-xs leading-5 text-amber-900 dark:border-amber-900 dark:bg-amber-950/40 dark:text-amber-200">
              <p className="font-semibold">This production could not be loaded.</p>
              <p className="mt-1 break-words">{loadError}</p>
              <button type="button" onClick={() => void refreshProject()} disabled={refreshing} className="mt-2 inline-flex h-7 items-center rounded-lg bg-amber-600 px-2.5 text-[11px] font-semibold text-white hover:bg-amber-500 disabled:opacity-50">{refreshing ? 'Retrying…' : 'Retry'}</button>
            </div>
          ) : null}
          {loadingProject ? <div className="grid h-full place-items-center text-xs text-slate-400"><Loader2 className="h-5 w-5 animate-spin" /></div> : panel === 'files' ? <FilesPanel project={project} /> : panel === 'workflow' ? <WorkflowPanel project={project} /> : <ProductionPanel project={project} videos={videos} characters={characters} documents={documents} />}
        </aside>
      </div>
      {showVideoPlayer && videos.length > 0 ? (
        <div className="fixed inset-0 z-50 flex flex-col bg-slate-950 lg:hidden" role="dialog" aria-modal="true" aria-label="Presented video player">
          <div className="flex h-14 shrink-0 items-center justify-between border-b border-slate-800 px-4">
            <div className="flex items-center gap-2 text-sm font-semibold text-white"><Film className="h-4 w-4 text-violet-300" />Presented video</div>
            <button type="button" onClick={() => setShowVideoPlayer(false)} className="grid h-8 w-8 place-items-center rounded-lg text-slate-300 hover:bg-slate-800 hover:text-white" aria-label="Close video player"><X className="h-4 w-4" /></button>
          </div>
          <div className="min-h-0 flex-1 overflow-y-auto bg-white dark:bg-slate-950">
            <VideosSection project={project} videos={videos} />
          </div>
        </div>
      ) : null}
      <FileContentViewer />
    </div>
  )
}

export function VideoStudioSurface() {
  // Product slash commands come from the same profile the provider options do,
  // and are cleared on unmount so leaving Video Studio does not leave its
  // commands offered in another product's chat.
  useEffect(() => {
    let cancelled = false
    void loadVideoProductCommands()
      .then((commands) => { if (!cancelled) setProductCommands(toProductCommandDefinitions(commands)) })
      .catch(() => { if (!cancelled) setProductCommands([]) })
    return () => { cancelled = true; setProductCommands([]) }
  }, [])

  const lastProjectId = useProductSurfaceStore((state) => state.lastVideoProjectId)
  const setLastProjectId = useProductSurfaceStore((state) => state.setLastVideoProjectId)
  const [projects, setProjects] = useState<VideoProject[]>([])
  const [query, setQuery] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [openProjectId, setOpenProjectId] = useState<string | null>(lastProjectId)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const refreshProjects = useCallback(async () => {
    setError('')
    try {
      setProjects(await loadVideoProjects())
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Could not load Video Studio projects.')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void refreshProjects() }, [refreshProjects])

  const openProject = projects.find((project) => project.id === openProjectId)
  const matchingProjects = useMemo(() => {
    const normalized = query.trim().toLowerCase()
    if (!normalized) return projects
    return projects.filter((project) => `${project.title} ${project.description}`.toLowerCase().includes(normalized))
  }, [projects, query])

  const selectProject = (projectId: string | null) => {
    setOpenProjectId(projectId)
    setLastProjectId(projectId)
  }

  // key by project id so switching projects REMOUNTS rather than reusing the
  // instance. ProjectWorkspace holds per-project refs that are meaningless
  // across a switch: handledPresentationEventCountRef (a new project with
  // fewer historical events than the old counter would have its refresh
  // events ignored entirely) and videoCountRef (which decides whether to
  // auto-open the player). Resetting them individually on project change
  // fixes only the ones anyone remembered to list; remounting fixes the
  // class.
  if (openProject) return <ProjectWorkspace key={openProject.id} project={openProject} onBack={() => selectProject(null)} />

  const handleCreate = async (title: string, description: string) => {
    const project = await createVideoProject(title, description)
    setProjects((current) => [project, ...current])
    setCreateOpen(false)
    selectProject(project.id)
  }

  return (
    <div className="flex h-screen min-h-0 flex-col overflow-hidden bg-slate-50 dark:bg-slate-950">
      <VideoStudioHeader>
        <div className="hidden h-7 w-px bg-slate-200 sm:block dark:bg-slate-800" />
        <div className="hidden items-center gap-2 text-xs font-semibold text-slate-700 sm:flex dark:text-slate-300"><FolderOpen className="h-4 w-4 text-violet-600" /> Projects</div>
      </VideoStudioHeader>
      <main className="min-h-0 flex-1 overflow-y-auto">
        <div className="mx-auto max-w-7xl px-5 py-8 sm:px-8 lg:px-10 lg:py-10">
          <div className="flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
            <div><h1 className="text-3xl font-semibold tracking-tight text-foreground sm:text-4xl">Your video projects</h1><p className="mt-2 text-sm text-muted-foreground">Continue a production conversation or start a new video from an idea.</p></div>
            <button type="button" data-testid="video-studio-new-project-button" onClick={() => setCreateOpen(true)} className="inline-flex h-11 items-center justify-center gap-2 rounded-xl bg-violet-600 px-4 text-xs font-semibold text-white shadow-lg shadow-violet-600/20 transition hover:-translate-y-0.5 hover:bg-violet-500"><Plus className="h-4 w-4" /> New project</button>
          </div>
          <section className="relative mt-8 overflow-hidden rounded-3xl bg-gradient-to-br from-slate-950 via-indigo-950 to-fuchsia-950 px-7 py-8 text-white shadow-2xl shadow-indigo-950/15 sm:px-9 sm:py-10">
            <div className="absolute -right-14 -top-28 h-72 w-72 rounded-full border border-white/10 shadow-[0_0_0_46px_rgba(255,255,255,0.035),0_0_0_92px_rgba(255,255,255,0.025)]" />
            <div className="relative max-w-2xl"><span className="grid h-10 w-10 place-items-center rounded-xl border border-white/15 bg-white/10"><MessageSquareText className="h-5 w-5" /></span><h2 className="mt-5 text-2xl font-semibold tracking-tight sm:text-3xl">Make videos by talking through ideas.</h2><p className="mt-3 max-w-xl text-sm leading-6 text-white/65">Each workspace keeps its brief, source material, production workflow, QA evidence, and playable exports together.</p></div>
          </section>
          <section className="mt-9">
            <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex items-baseline gap-2"><h2 className="text-base font-semibold text-slate-900 dark:text-slate-100">All projects</h2><span className="text-xs text-slate-400">{matchingProjects.length} {matchingProjects.length === 1 ? 'project' : 'projects'}</span></div>
              <div className="flex items-center gap-2">
                <button type="button" onClick={() => void refreshProjects()} aria-label="Refresh projects" className="grid h-10 w-10 place-items-center rounded-xl border border-slate-200 bg-white text-slate-400 shadow-sm hover:text-violet-600 dark:border-slate-800 dark:bg-slate-900"><RefreshCw className="h-4 w-4" /></button>
                <label className="flex h-10 w-full items-center gap-2 rounded-xl border border-slate-200 bg-white px-3 text-slate-400 shadow-sm sm:w-64 dark:border-slate-800 dark:bg-slate-900"><Search className="h-4 w-4" /><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search projects" className="min-w-0 flex-1 bg-transparent text-xs text-slate-700 outline-none placeholder:text-slate-400 dark:text-slate-200" /></label>
              </div>
            </div>
            {loading ? <div className="grid min-h-64 place-items-center rounded-2xl border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900"><div className="text-center text-xs text-slate-400"><Loader2 className="mx-auto mb-3 h-6 w-6 animate-spin" />Loading projects…</div></div> : error ? <div className="grid min-h-64 place-items-center rounded-2xl border border-red-200 bg-red-50 p-6 text-center dark:border-red-900 dark:bg-red-950/30"><div><AlertCircle className="mx-auto h-7 w-7 text-red-500" /><h3 className="mt-3 text-sm font-semibold text-red-800 dark:text-red-200">Projects could not be loaded</h3><p className="mt-1 max-w-md text-xs text-red-600 dark:text-red-300">{error}</p><button type="button" onClick={() => void refreshProjects()} className="mt-4 rounded-lg bg-red-600 px-3 py-2 text-xs font-semibold text-white">Try again</button></div></div> : matchingProjects.length > 0 ? (
              <div className="grid gap-5 sm:grid-cols-2 xl:grid-cols-3">
                {matchingProjects.map((project, index) => <ProjectCard key={project.id} project={project} index={index} onOpen={() => selectProject(project.id)} />)}
                <button type="button" onClick={() => setCreateOpen(true)} className="flex min-h-[290px] flex-col items-center justify-center gap-2 rounded-2xl border border-dashed border-slate-300 bg-white/60 text-slate-500 transition hover:border-violet-400 hover:bg-white hover:text-violet-700 dark:border-slate-700 dark:bg-slate-900/50 dark:hover:border-violet-700 dark:hover:bg-slate-900"><span className="grid h-11 w-11 place-items-center rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-700 dark:bg-slate-800"><Plus className="h-5 w-5" /></span><strong className="mt-1 text-xs">Create another project</strong><small className="text-[10px] text-slate-400">Start with an empty workspace</small></button>
              </div>
            ) : query.trim() ? <div className="grid min-h-64 place-items-center rounded-2xl border border-dashed border-slate-300 bg-white text-center dark:border-slate-700 dark:bg-slate-900"><div><FileImage className="mx-auto h-8 w-8 text-slate-300" /><h3 className="mt-3 text-sm font-semibold">No matching projects</h3><p className="mt-1 text-xs text-slate-400">Try a different search.</p></div></div> : <div className="grid min-h-72 place-items-center rounded-2xl border border-dashed border-slate-300 bg-white p-8 text-center dark:border-slate-700 dark:bg-slate-900"><div><Clapperboard className="mx-auto h-9 w-9 text-violet-400" /><h3 className="mt-4 text-base font-semibold">Create your first video project</h3><p className="mt-1 text-xs leading-5 text-slate-400">No demo data—this list is read directly from your secure workspace.</p><button type="button" onClick={() => setCreateOpen(true)} className="mt-5 rounded-xl bg-violet-600 px-4 py-2.5 text-xs font-semibold text-white">New project</button></div></div>}
          </section>
        </div>
      </main>
      {createOpen ? <CreateProjectDialog onClose={() => setCreateOpen(false)} onCreate={handleCreate} /> : null}
    </div>
  )
}
