import { create } from 'zustand'
import { api, streamChat, type ApiAsset, type ApiMessage, type ApiProject, type ApiUser, type ApiVideo, type ApiWorkflowBundle } from './api'
import type { ChatMessage, ProjectAsset, ToolActivity, VideoOutput, VideoProject } from './types'

export type AppSection = 'projects' | 'settings'

function makeId(prefix: string) { return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}` }
function bytesLabel(bytes: number) { return bytes < 1024 * 1024 ? `${Math.max(1, Math.round(bytes / 1024))} KB` : `${(bytes / (1024 * 1024)).toFixed(1)} MB` }
function timeLabel(value: string) { return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' }).format(new Date(value)) }
function updatedLabel(value: string) { const elapsed = Date.now() - new Date(value).getTime(); return elapsed < 90_000 ? 'Just now' : new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric' }).format(new Date(value)) }
function paletteFor(id: string): [string, string] { const palettes: [string, string][] = [['#5b60d6', '#9b78d9'], ['#346c80', '#6eaf9a'], ['#8a4f67', '#d58b72'], ['#536a9a', '#7892d0']]; return palettes[id.charCodeAt(0) % palettes.length] }
function mapMessage(message: ApiMessage): ChatMessage { return { id: message.id, role: message.role, author: message.author, body: message.body, time: timeLabel(message.createdAt) } }
function mapAsset(asset: ApiAsset): ProjectAsset { return { id: asset.id, name: asset.name, kind: asset.kind, size: bytesLabel(asset.size) } }
function mapVideo(video: ApiVideo, palette: [string, string]): VideoOutput { return { id: video.id, title: video.name, duration: '', createdAt: updatedLabel(video.createdAt), palette, contentUrl: video.contentUrl, note: video.note } }
function mapWorkflow(bundle: ApiWorkflowBundle) { return { name: bundle.name, description: bundle.description, steps: bundle.steps, runs: bundle.runs.map((run) => ({ id: run.id, name: run.name, groupName: run.groupName, status: run.status, currentStep: run.currentStep, updatedAt: updatedLabel(run.updatedAt), steps: run.steps })) } }

async function hydrateProject(project: ApiProject): Promise<VideoProject> {
  const [messages, assets, files, videos, workflow] = await Promise.all([api.messages(project.id), api.assets(project.id), api.files(project.id), api.videos(project.id), api.workflows(project.id)])
  const palette = paletteFor(project.id)
  return {
    id: project.id, title: project.title, description: project.description || 'A video project.',
    updatedAt: updatedLabel(project.updatedAt), visibility: project.role === 'owner' ? 'private' : 'shared',
    ownerId: project.ownerId, members: [], sessionStatus: project.sessionStatus, palette,
    messages: messages.map(mapMessage), assets: assets.map(mapAsset), files, videos: videos.map((video) => mapVideo(video, palette)), workflow: mapWorkflow(workflow),
  }
}

interface VideoStore {
  user: ApiUser | null
  projects: VideoProject[]
  loading: boolean
  bootstrapping: boolean
  bootstrapped: boolean
  section: AppSection
  selectedProjectId: string | null
  creating: boolean
  streams: Record<string, string>
  toolActivities: Record<string, ToolActivity[]>
  bootstrap: () => Promise<void>
  authenticate: (username: string, password: string) => Promise<void>
  logout: () => Promise<void>
  refreshProjects: () => Promise<void>
  setSection: (section: AppSection) => void
  selectProject: (projectId: string | null) => void
  setCreating: (creating: boolean) => void
  createProject: (title: string, description: string) => Promise<void>
  sendMessage: (projectId: string, body: string) => Promise<void>
  steer: (projectId: string, body: string) => Promise<void>
  cancel: (projectId: string) => Promise<void>
  upload: (projectId: string, files: File[]) => Promise<void>
  refreshWorkflow: (projectId: string) => Promise<void>
}

export const useVideoStore = create<VideoStore>((set, get) => {
  const updateProject = (id: string, updater: (project: VideoProject) => VideoProject) => {
    set((state) => ({ projects: state.projects.map((project) => project.id === id ? updater(project) : project) }))
  }

  return {
    user: null,
    projects: [],
    loading: true,
    bootstrapping: false,
    bootstrapped: false,
    section: 'projects',
    selectedProjectId: null,
    creating: false,
    streams: {},
    toolActivities: {},

    bootstrap: async () => {
      if (get().bootstrapping || get().bootstrapped) return
      set({ bootstrapping: true })
      try {
        const user = await api.me()
        const items = await api.projects()
        set({ user, projects: await Promise.all(items.map(hydrateProject)) })
      } catch {
        set({ user: null, projects: [], selectedProjectId: null })
      } finally {
        set({ loading: false, bootstrapping: false, bootstrapped: true })
      }
    },

    authenticate: async (username, password) => {
      const user = await api.login(username, password)
      const items = await api.projects()
      set({ user, projects: await Promise.all(items.map(hydrateProject)), selectedProjectId: null })
    },

    logout: async () => {
      try { await api.logout() } finally { set({ user: null, projects: [], selectedProjectId: null, section: 'projects' }) }
    },

    refreshProjects: async () => {
      const items = await api.projects()
      set({ projects: await Promise.all(items.map(hydrateProject)) })
    },

    setSection: (section) => set({ section }),
    selectProject: (selectedProjectId) => set({ selectedProjectId }),
    setCreating: (creating) => set({ creating }),

    createProject: async (title, description) => {
      const created = await api.createProject(title, description)
      const project = await hydrateProject(created)
      set((state) => ({ projects: [project, ...state.projects], creating: false, selectedProjectId: project.id }))
    },

    sendMessage: async (projectId, body) => {
      const user = get().user
      if (!user) throw new Error('Please sign in again')
      const optimistic: ChatMessage = { id: makeId('pending'), role: 'user', author: user.name, body, time: timeLabel(new Date().toISOString()) }
      set((state) => ({
        streams: { ...state.streams, [projectId]: '' },
        toolActivities: { ...state.toolActivities, [projectId]: [] },
      }))
      updateProject(projectId, (project) => ({ ...project, messages: [...project.messages, optimistic], sessionStatus: 'working', updatedAt: 'Just now' }))
      try {
        const completed = await streamChat(projectId, body, {
          onDelta: (text) => set((state) => ({ streams: { ...state.streams, [projectId]: (state.streams[projectId] ?? '') + text } })),
          onTool: (event) => set((state) => {
            const current = state.toolActivities[projectId] ?? []
            const activity: ToolActivity = {
              id: event.callId,
              name: event.workflow ? `${event.workflow}${event.step ? ` → ${event.step}` : ''}` : event.tool,
              workflow: event.workflow,
              step: event.step,
              status: event.status,
              durationMs: event.durationMs,
            }
            const index = current.findIndex((item) => item.id === activity.id)
            const next = index < 0 ? [...current, activity] : current.map((item, itemIndex) => itemIndex === index ? activity : item)
            return { toolActivities: { ...state.toolActivities, [projectId]: next } }
          }),
        })
        updateProject(projectId, (project) => ({
          ...project,
          messages: [...project.messages.filter((message) => message.id !== optimistic.id), optimistic, mapMessage(completed.message)],
          videos: completed.videos.map((video) => mapVideo(video, project.palette)),
          sessionStatus: 'ready', updatedAt: 'Just now',
        }))
        const [workflow, messages, files] = await Promise.all([api.workflows(projectId), api.messages(projectId), api.files(projectId)])
        updateProject(projectId, (project) => ({ ...project, workflow: mapWorkflow(workflow), messages: messages.map(mapMessage), files }))
        set((state) => ({ streams: { ...state.streams, [projectId]: '' } }))
      } catch (error) {
        updateProject(projectId, (project) => ({ ...project, sessionStatus: 'ready' }))
        set((state) => ({ streams: { ...state.streams, [projectId]: '' } }))
        throw error
      }
    },

    steer: async (projectId, body) => {
      const message = await api.steer(projectId, body)
      updateProject(projectId, (project) => ({ ...project, messages: [...project.messages, mapMessage(message)] }))
    },

    cancel: async (projectId) => {
      await api.cancel(projectId)
      updateProject(projectId, (project) => ({ ...project, sessionStatus: 'ready' }))
    },

    upload: async (projectId, files) => {
      const additions = await Promise.all(files.map((file) => api.uploadAsset(projectId, file)))
      const projectFiles = await api.files(projectId)
      updateProject(projectId, (project) => ({ ...project, assets: [...additions.map(mapAsset), ...project.assets], files: projectFiles, updatedAt: 'Just now' }))
    },

    refreshWorkflow: async (projectId) => {
      const [workflow, messages, files] = await Promise.all([api.workflows(projectId), api.messages(projectId), api.files(projectId)])
      const previous = get().projects.find((project) => project.id === projectId)?.messages.length ?? 0
      updateProject(projectId, (project) => ({ ...project, workflow: mapWorkflow(workflow), messages: messages.map(mapMessage), files }))
      // Background workflow turns resume the agent outside the SSE stream, so
      // their tool events never reach onTool. Without this the debug panel keeps
      // showing the last user-initiated call — which reads as the current stage
      // and is what made a finished full run look like a single 11ms step.
      if (messages.length > previous) {
        set((state) => ({ toolActivities: { ...state.toolActivities, [projectId]: [] } }))
      }
    },
  }
})
