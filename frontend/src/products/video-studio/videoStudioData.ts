import { agentApi, getApiBaseUrl, getAuthToken } from '../../services/api'
import type { LLMProvider, PlannerFile } from '../../services/api-types'
import { loadWorkspacePresentations, parseWorkspacePresentations, type WorkspacePresentation } from '../../platform/presentations/presentationData'

export const VIDEO_PROJECTS_ROOT = 'Chats/Video Studio/projects'
export const VIDEO_PROFILE_ID = 'video-studio'
export const VIDEO_PROFILE_VERSION = 2

// Mirrors the LLMProvider union in api-types.ts. Kept as a Set here (rather
// than importing one) because there is no runtime-checkable form of a
// string-literal union to import -- this is the validation half that has to
// exist somewhere for a value arriving as untyped server JSON to become a
// safely-typed LLMProvider.
const KNOWN_LLM_PROVIDERS = new Set<LLMProvider>([
  'openrouter', 'bedrock', 'openai', 'vertex', 'anthropic', 'azure', 'z-ai',
  'kimi', 'claude-code', 'codex-cli', 'cursor-cli', 'agy-cli', 'pi-cli',
  'minimax', 'minimax-coding-plan', 'elevenlabs', 'deepgram',
])

function asLLMProvider(value: string): LLMProvider | null {
  return KNOWN_LLM_PROVIDERS.has(value as LLMProvider) ? (value as LLMProvider) : null
}

export type VideoAgentProviderOption = {
  id: string
  label: string
  provider: LLMProvider
  modelId: string
  isDefault?: boolean
}

export type VideoProductCommand = {
  name: string
  description: string
  icon: string
  prompt: string
}

type AgentProfileResponse = {
  commands?: Array<{
    name?: unknown
    description?: unknown
    icon?: unknown
    prompt?: unknown
  }>
  runtime?: {
    provider_options?: Array<{
      id?: unknown
      label?: unknown
      provider?: unknown
      model_id?: unknown
      default?: unknown
    }>
    capabilities?: {
      voice?: unknown
    }
  }
}

// Slash commands the product ships with itself, declared in its product.yaml.
// A command with no prompt is dropped rather than offered: it would appear in
// the menu and then submit nothing, which reads as the product being broken.
export function parseProductCommands(profile: { commands?: Array<Record<string, unknown>> }): VideoProductCommand[] {
  return (profile.commands ?? []).flatMap((command) => {
    const name = asString(command.name)
    const prompt = asString(command.prompt)
    if (!name || !prompt) return []
    return [{
      name,
      description: asString(command.description),
      icon: asString(command.icon) || 'terminal',
      prompt,
    }]
  })
}

export async function loadVideoProductCommands(): Promise<VideoProductCommand[]> {
  const token = getAuthToken()
  const response = await fetch(`${getApiBaseUrl()}/api/agent-profiles/${encodeURIComponent(VIDEO_PROFILE_ID)}?version=${VIDEO_PROFILE_VERSION}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  })
  if (!response.ok) throw new Error(`Unable to load Video Studio commands (${response.status})`)
  return parseProductCommands(await response.json() as AgentProfileResponse)
}

// The profile endpoint returns the server's YAML-loaded profile. This keeps
// product controls declarative: adding a provider choice requires changing the
// product YAML, not a separate frontend allow-list.
export async function loadVideoAgentProviderOptions(): Promise<VideoAgentProviderOption[]> {
  const token = getAuthToken()
  const response = await fetch(`${getApiBaseUrl()}/api/agent-profiles/${encodeURIComponent(VIDEO_PROFILE_ID)}?version=${VIDEO_PROFILE_VERSION}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
  })
  if (!response.ok) throw new Error(`Unable to load Video Studio agent profile (${response.status})`)
  const profile = await response.json() as AgentProfileResponse
  const options = profile.runtime?.provider_options ?? []
  return options.flatMap((option) => {
    const id = asString(option.id)
    const label = asString(option.label)
    const provider = asLLMProvider(asString(option.provider))
    const modelId = asString(option.model_id)
    return id && label && provider && modelId ? [{ id, label, provider, modelId, isDefault: option.default === true }] : []
  })
}

export type VideoProject = {
  schemaVersion: 1
  product: 'video-studio'
  id: string
  title: string
  description: string
  sessionId: string
  workspacePath: string
  createdAt: string
  updatedAt: string
  videos: number
}

export type VideoPresentation = {
  id: string
  title: string
  path: string
  qaReportPath: string
  note: string
  verdict: string
  revision: number
  updatedAt: string
  workspacePresentation: WorkspacePresentation
}

// A character is the reference every later shot of that subject is generated
// against, so the panel shows the image and its spec together — checking a
// shot against a remembered description is exactly how identity drift gets
// missed.
export type CharacterPresentation = {
  id: string
  name: string
  imagePath: string
  specPath: string
  spec: string
  model: string
  provider: string
  note: string
  revision: number
  updatedAt: string
  workspacePresentation: WorkspacePresentation
}

export type DocumentPresentation = {
  id: string
  title: string
  path: string
  markdown: string
  note: string
  revision: number
  updatedAt: string
  workspacePresentation: WorkspacePresentation
}

export type VideoAsset = {
  path: string
  name: string
  size?: number
  mimeType?: string
  modifiedAt?: string
}

export type VideoWorkflowStep = {
  id: string
  title: string
  type: string
  routes?: Array<{ id: string; title: string; nextStepId: string }>
}

type ProductManifest = {
  schema_version?: unknown
  product?: unknown
  id?: unknown
  title?: unknown
  description?: unknown
  session_id?: unknown
  created_at?: unknown
  updated_at?: unknown
}

function responseFiles(response: unknown): PlannerFile[] {
  if (Array.isArray(response)) return response as PlannerFile[]
  if (!response || typeof response !== 'object') return []
  const data = (response as { data?: unknown }).data
  return Array.isArray(data) ? data as PlannerFile[] : []
}

function flattenFiles(items: PlannerFile[], result: PlannerFile[] = []): PlannerFile[] {
  for (const item of items) {
    result.push(item)
    if (Array.isArray(item.children)) flattenFiles(item.children, result)
  }
  return result
}

// Keeps the first entry per filepath, preserving listing order. The listing can
// describe one file from more than one branch of the tree it returns.
function dedupeByFilepath(files: PlannerFile[]): PlannerFile[] {
  const seen = new Set<string>()
  return files.filter((file) => {
    if (seen.has(file.filepath)) return false
    seen.add(file.filepath)
    return true
  })
}

function responseContent(response: unknown): { content: string; lastModified?: string } | null {
  if (!response || typeof response !== 'object') return null
  const envelope = response as { data?: unknown; content?: unknown; last_modified?: unknown }
  const value = envelope.data && typeof envelope.data === 'object'
    ? envelope.data as { content?: unknown; last_modified?: unknown }
    : envelope
  if (typeof value.content !== 'string') return null
  return {
    content: value.content,
    lastModified: typeof value.last_modified === 'string' ? value.last_modified : undefined,
  }
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : ''
}

export function projectSlug(title: string): string {
  const slug = title
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48)
  return slug || 'video-project'
}

export function parseProjectManifest(content: string, workspacePath: string, lastModified?: string): VideoProject | null {
  let raw: ProductManifest
  try {
    raw = JSON.parse(content) as ProductManifest
  } catch {
    return null
  }
  const id = asString(raw.id)
  const title = asString(raw.title)
  const sessionId = asString(raw.session_id)
  if (raw.schema_version !== 1 || raw.product !== VIDEO_PROFILE_ID || !id || !title || !sessionId) return null
  const createdAt = asString(raw.created_at) || lastModified || new Date(0).toISOString()
  const updatedAt = lastModified || asString(raw.updated_at) || createdAt
  return {
    schemaVersion: 1,
    product: VIDEO_PROFILE_ID,
    id,
    title,
    description: asString(raw.description),
    sessionId,
    workspacePath,
    createdAt,
    updatedAt,
    videos: 0,
  }
}

async function projectVideoCount(workspacePath: string): Promise<number> {
  try {
    const response = await agentApi.queryWorkflowDB(
      `${workspacePath}/db/db.sqlite`,
      "SELECT COUNT(*) AS count FROM ui_presentations WHERE kind = 'media.video' AND status = 'ready'",
    )
    const value = response.data?.rows?.[0]?.count
    return typeof value === 'number' ? value : Number(value) || 0
  } catch {
    return 0
  }
}

export async function loadVideoProjects(): Promise<VideoProject[]> {
  const response = await agentApi.getPlannerFiles(VIDEO_PROJECTS_ROOT, -1, 2)
  // The listing returns the requested folder as a node whose children are the
  // projects, AND each project again as a top-level sibling, so a flatten walks
  // every product.json twice. Key by filepath: the same physical manifest must
  // produce one project, or the grid renders duplicate React keys and every
  // manifest is fetched twice.
  const manifests = dedupeByFilepath(
    flattenFiles(responseFiles(response))
      .filter((file) => file.type !== 'folder' && file.filepath.endsWith('/product.json')),
  )

  const projects = await Promise.all(manifests.map(async (file) => {
    try {
      const response = await agentApi.getPlannerFileContent(file.filepath)
      const document = responseContent(response)
      if (!document) return null
      const workspacePath = file.filepath.replace(/\/product\.json$/, '')
      const project = parseProjectManifest(document.content, workspacePath, document.lastModified || file.last_modified)
      if (!project) return null
      project.videos = await projectVideoCount(workspacePath)
      return project
    } catch {
      return null
    }
  }))

  return projects
    .filter((project): project is VideoProject => project !== null)
    .sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt))
}

export async function createVideoProject(title: string, description: string): Promise<VideoProject> {
  const id = globalThis.crypto.randomUUID()
  const sessionId = `video-studio:project:${id}`
  const workspacePath = `${VIDEO_PROJECTS_ROOT}/${projectSlug(title)}-${id.slice(0, 8)}`
  const now = new Date().toISOString()
  const manifest = {
    schema_version: 1,
    product: VIDEO_PROFILE_ID,
    id,
    title: title.trim(),
    description: description.trim(),
    session_id: sessionId,
    created_at: now,
    updated_at: now,
  }
  await agentApi.updatePlannerFile(
    `${workspacePath}/product.json`,
    `${JSON.stringify(manifest, null, 2)}\n`,
    `Create Video Studio project ${title.trim()}`,
  )
  return {
    schemaVersion: 1,
    product: VIDEO_PROFILE_ID,
    id,
    title: manifest.title,
    description: manifest.description,
    sessionId,
    workspacePath,
    createdAt: now,
    updatedAt: now,
    videos: 0,
  }
}

export function parsePresentations(rows: Record<string, unknown>[]): VideoPresentation[] {
  return parseWorkspacePresentations(rows).flatMap((presentation) => {
    if (presentation.kind !== 'media.video' || presentation.status !== 'ready') return []
    const path = asString(presentation.payload.path)
    if (!path) return []
    return [{
      id: presentation.id,
      title: presentation.title || path.split('/').pop() || 'Video',
      path,
      qaReportPath: asString(presentation.payload.qa_report_path),
      note: asString(presentation.payload.note),
      verdict: asString(presentation.payload.verdict),
      revision: presentation.revision,
      updatedAt: presentation.updatedAt,
      workspacePresentation: presentation,
    }]
  })
}

export async function loadVideoPresentations(project: VideoProject): Promise<VideoPresentation[]> {
  const presentations = await loadWorkspacePresentations(project.workspacePath, ['media.video'])
  return presentations.flatMap((presentation) => {
    const path = asString(presentation.payload.path)
    if (!path) return []
    return [{
      id: presentation.id,
      title: presentation.title,
      path,
      qaReportPath: asString(presentation.payload.qa_report_path),
      note: asString(presentation.payload.note),
      verdict: asString(presentation.payload.verdict),
      revision: presentation.revision,
      updatedAt: presentation.updatedAt,
      workspacePresentation: presentation,
    }]
  })
}

// A character without a reference image is not a character this panel can do
// its job with -- the image is what a later shot gets compared against -- so
// it is dropped rather than rendered as a broken tile.
export function toCharacterPresentations(presentations: WorkspacePresentation[]): CharacterPresentation[] {
  return presentations.flatMap((presentation) => {
    const imagePath = asString(presentation.payload.image_path)
    if (!imagePath) return []
    return [{
      id: presentation.id,
      name: asString(presentation.payload.name) || presentation.title,
      imagePath,
      specPath: asString(presentation.payload.spec_path),
      spec: asString(presentation.payload.spec),
      model: asString(presentation.payload.model),
      provider: asString(presentation.payload.provider),
      note: asString(presentation.payload.note),
      revision: presentation.revision,
      updatedAt: presentation.updatedAt,
      workspacePresentation: presentation,
    }]
  })
}

export async function loadCharacterPresentations(project: VideoProject): Promise<CharacterPresentation[]> {
  return toCharacterPresentations(await loadWorkspacePresentations(project.workspacePath, ['media.character']))
}

export function toDocumentPresentations(presentations: WorkspacePresentation[]): DocumentPresentation[] {
  return presentations.flatMap((presentation) => {
    const path = asString(presentation.payload.path)
    if (!path) return []
    return [{
      id: presentation.id,
      title: presentation.title || path.split('/').pop() || 'Document',
      path,
      markdown: asString(presentation.payload.markdown),
      note: asString(presentation.payload.note),
      revision: presentation.revision,
      updatedAt: presentation.updatedAt,
      workspacePresentation: presentation,
    }]
  })
}

export async function loadDocumentPresentations(project: VideoProject): Promise<DocumentPresentation[]> {
  return toDocumentPresentations(await loadWorkspacePresentations(project.workspacePath, ['document.markdown']))
}

export async function loadVideoAssets(project: VideoProject): Promise<VideoAsset[]> {
  try {
    const response = await agentApi.getPlannerFiles(project.workspacePath, -1, 5)
    return flattenFiles(responseFiles(response))
      .filter((file) => file.type !== 'folder')
      .filter((file) => /\/(uploads|work|outputs|runs)\//.test(file.filepath))
      .map((file) => ({
        path: file.filepath,
        name: file.filepath.split('/').pop() || file.filepath,
        size: file.size,
        mimeType: file.mime_type,
        modifiedAt: file.last_modified,
      }))
      .sort((a, b) => (b.modifiedAt || '').localeCompare(a.modifiedAt || ''))
  } catch {
    return []
  }
}

export function parseWorkflowSteps(content: string): VideoWorkflowStep[] {
  try {
    const parsed = JSON.parse(content) as { steps?: unknown }
    if (!Array.isArray(parsed.steps)) return []
    return parsed.steps.flatMap((item) => {
      if (!item || typeof item !== 'object') return []
      const step = item as Record<string, unknown>
      const id = asString(step.id)
      if (!id) return []
      const routes = Array.isArray(step.routes)
        ? step.routes.flatMap((route) => {
            if (!route || typeof route !== 'object') return []
            const value = route as Record<string, unknown>
            return [{ id: asString(value.route_id), title: asString(value.route_name), nextStepId: asString(value.next_step_id) }]
          })
        : undefined
      return [{ id, title: asString(step.title) || id, type: asString(step.type) || 'regular', routes }]
    })
  } catch {
    return []
  }
}

export async function loadVideoWorkflow(project: VideoProject): Promise<VideoWorkflowStep[]> {
  try {
    const response = await agentApi.getPlannerFileContent(`${project.workspacePath}/planning/plan.json`)
    const document = responseContent(response)
    return document ? parseWorkflowSteps(document.content) : []
  } catch {
    return []
  }
}

function base64Path(path: string): string {
  const bytes = new TextEncoder().encode(path)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary)
}

// This URL streams from the generic authenticated AgentWorks file proxy and
// preserves HTTP Range, so seeking never requires loading a full video Blob.
export function workspaceMediaURL(path: string): string {
  const params = new URLSearchParams({ path: base64Path(path) })
  const token = getAuthToken()
  if (token) params.set('token', token)
  return `${getApiBaseUrl()}/api/public/file?${params.toString()}`
}

export function relativeTime(value: string, now = Date.now()): string {
  const timestamp = Date.parse(value)
  if (!Number.isFinite(timestamp)) return 'Recently'
  const seconds = Math.max(0, Math.round((now - timestamp) / 1000))
  if (seconds < 60) return 'Just now'
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${days}d ago`
  return new Date(timestamp).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })
}

export function formatBytes(size?: number): string {
  if (!size || size < 1) return '—'
  if (size < 1024) return `${size} B`
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(1)} MB`
  return `${(size / (1024 * 1024 * 1024)).toFixed(1)} GB`
}
