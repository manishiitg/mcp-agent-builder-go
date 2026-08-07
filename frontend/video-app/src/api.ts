import { createExecutionEventsClient } from '../../packages/execution-events'

const API_BASE = (import.meta.env.VITE_VIDEO_API_URL as string | undefined)?.replace(/\/$/, '') ?? 'http://127.0.0.1:8200'
const executionEventsClient = createExecutionEventsClient({
  baseURL: API_BASE,
  routeForScope: (projectId) => `/api/projects/${projectId}/execution-events`,
  credentials: 'include',
})

export interface ApiUser {
  id: string
  email: string
  name: string
  createdAt: string
}

export interface ApiProject {
  id: string
  title: string
  description: string
  ownerId: string
  role: string
  status: string
  sessionStatus: 'ready' | 'working'
  createdAt: string
  updatedAt: string
}

export interface ApiMessage {
  id: string
  projectId: string
  userId?: string
  role: 'user' | 'assistant'
  author: string
  body: string
  createdAt: string
}

export interface ApiAsset {
  id: string
  projectId: string
  name: string
  kind: 'image' | 'video' | 'audio' | 'document'
  size: number
  createdAt: string
}

export interface ApiVideo {
  id: string
  name: string
  size: number
  createdAt: string
  contentUrl: string
  note?: string
}

export interface ApiProjectFileNode {
  name: string
  path: string
  type: 'file' | 'folder'
  size: number
  children?: ApiProjectFileNode[]
}

export interface ApiWorkflowStep {
  id: string
  title: string
  position: number
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
  summary?: string
}

export interface ApiWorkflowRun {
  id: string
  projectId: string
  name: string
  groupName: string
  status: 'ready' | 'running' | 'completed' | 'failed' | 'cancelled'
  currentStep?: string
  executionId?: string
  steps: ApiWorkflowStep[]
  createdAt: string
  updatedAt: string
}

export interface ApiWorkflowDefinition {
  id: string
  name: string
  description?: string
  steps: ApiWorkflowStep[]
}

export interface ApiWorkflowBundle {
  workflows: ApiWorkflowDefinition[]
  runs: ApiWorkflowRun[]
}

export interface ChatCompleted {
  message: ApiMessage
  videos: ApiVideo[]
}

export interface ToolStreamEvent {
  callId: string
  tool: string
  workflow?: string
  step?: string
  status: 'running' | 'completed' | 'failed'
  durationMs?: number
}

interface ChatStreamHandlers {
  onDelta: (text: string) => void
  onTool?: (event: ToolStreamEvent) => void
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...init,
    credentials: 'include',
    headers: init?.body instanceof FormData ? init.headers : { 'Content-Type': 'application/json', ...init?.headers },
  })
  if (!response.ok) {
    const payload = await response.json().catch(() => ({ error: `Request failed (${response.status})` })) as { error?: string }
    throw new Error(payload.error || `Request failed (${response.status})`)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}

export const api = {
  me: () => request<ApiUser>('/api/auth/me'),
  login: (email: string, password: string) => request<ApiUser>('/api/auth/login', { method: 'POST', body: JSON.stringify({ email, password }) }),
  logout: () => request<void>('/api/auth/logout', { method: 'POST' }),
  projects: () => request<ApiProject[]>('/api/projects'),
  createProject: (title: string, description: string) => request<ApiProject>('/api/projects', { method: 'POST', body: JSON.stringify({ title, description }) }),
  messages: (projectId: string) => request<ApiMessage[]>(`/api/projects/${projectId}/messages`),
  executionEvents: executionEventsClient,
  assets: (projectId: string) => request<ApiAsset[]>(`/api/projects/${projectId}/assets`),
  files: (projectId: string) => request<ApiProjectFileNode[]>(`/api/projects/${projectId}/files`),
  videos: (projectId: string) => request<ApiVideo[]>(`/api/projects/${projectId}/videos`),
  workflows: (projectId: string) => request<ApiWorkflowBundle>(`/api/projects/${projectId}/workflows`),
  uploadAsset: (projectId: string, file: File) => {
    const body = new FormData()
    body.append('file', file)
    return request<ApiAsset>(`/api/projects/${projectId}/assets`, { method: 'POST', body })
  },
  steer: (projectId: string, message: string) => request<ApiMessage>(`/api/projects/${projectId}/chat/steer`, { method: 'POST', body: JSON.stringify({ message }) }),
  cancel: (projectId: string) => request<{ cancelled: boolean }>(`/api/projects/${projectId}/chat/cancel`, { method: 'POST' }),
  secretNames: () => request<{ names: string[] }>('/api/secrets'),
  putSecret: (name: string, value: string) => request<{ name: string }>(`/api/secrets/${encodeURIComponent(name)}`, { method: 'PUT', body: JSON.stringify({ value }) }),
  deleteSecret: (name: string) => request<void>(`/api/secrets/${encodeURIComponent(name)}`, { method: 'DELETE' }),
  providerToken: () => request<{ configured: boolean }>('/api/provider-token'),
  putProviderToken: (value: string) => request<{ configured: boolean }>('/api/provider-token', { method: 'PUT', body: JSON.stringify({ value }) }),
  deleteProviderToken: () => request<void>('/api/provider-token', { method: 'DELETE' }),
}

export async function streamChat(projectId: string, message: string, handlers: ChatStreamHandlers): Promise<ChatCompleted> {
  const response = await fetch(`${API_BASE}/api/projects/${projectId}/chat`, {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ message }),
  })
  if (!response.ok) {
    const payload = await response.json().catch(() => ({ error: `Request failed (${response.status})` })) as { error?: string }
    throw new Error(payload.error || `Request failed (${response.status})`)
  }
  if (!response.body) throw new Error('Streaming is unavailable')

  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  let completed: ChatCompleted | null = null
  while (true) {
    const { done, value } = await reader.read()
    buffer += decoder.decode(value, { stream: !done })
    const blocks = buffer.split('\n\n')
    buffer = blocks.pop() ?? ''
    for (const block of blocks) {
      let event = 'message'
      let data = ''
      for (const line of block.split('\n')) {
        if (line.startsWith('event:')) event = line.slice(6).trim()
        if (line.startsWith('data:')) data += line.slice(5).trim()
      }
      if (!data) continue
      const payload = JSON.parse(data) as Record<string, unknown>
      if (event === 'delta') handlers.onDelta(String(payload.text ?? ''))
      if (event === 'tool') handlers.onTool?.(payload as unknown as ToolStreamEvent)
      if (event === 'error') throw new Error(String(payload.error ?? 'The agent could not complete this request'))
      if (event === 'completed') completed = payload as unknown as ChatCompleted
    }
    if (done) break
  }
  if (!completed) throw new Error('The response ended before completion')
  return completed
}

export function mediaURL(path: string) {
  return path.startsWith('http') ? path : `${API_BASE}${path}`
}

export function projectFileURL(projectId: string, path: string) {
  return `${API_BASE}/api/projects/${projectId}/files/content?path=${encodeURIComponent(path)}`
}
