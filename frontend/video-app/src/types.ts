export type ProjectVisibility = 'private' | 'shared'
export type SessionStatus = 'ready' | 'working'

export interface Member {
  id: string
  name: string
  initials: string
  color: string
}

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  author: string
  body: string
  time: string
}

export interface ToolActivity {
  id: string
  name: string
  workflow?: string
  step?: string
  status: 'running' | 'completed' | 'failed'
  durationMs?: number
}

export interface ProjectAsset {
  id: string
  name: string
  kind: 'image' | 'video' | 'audio' | 'document'
  size: string
}

export interface VideoOutput {
  id: string
  title: string
  duration: string
  createdAt: string
  palette: [string, string]
  contentUrl: string
  // The agent's one-line description of what this video is — for example that
  // it is a placeholder assembly rather than finished creative.
  note?: string
}

export interface WorkflowStep {
  id: string
  title: string
  position: number
  status: 'pending' | 'running' | 'completed' | 'failed' | 'cancelled'
  summary?: string
}

export interface WorkflowRun {
  id: string
  name: string
  groupName: string
  status: 'ready' | 'running' | 'completed' | 'failed' | 'cancelled'
  currentStep?: string
  updatedAt: string
  steps: WorkflowStep[]
}

export interface ProjectWorkflow {
  name: string
  description?: string
  steps: WorkflowStep[]
  runs: WorkflowRun[]
}

export interface VideoProject {
  id: string
  title: string
  description: string
  updatedAt: string
  visibility: ProjectVisibility
  ownerId: string
  members: Member[]
  sessionStatus: SessionStatus
  palette: [string, string]
  messages: ChatMessage[]
  assets: ProjectAsset[]
  files: import('../../shared/files/ProjectFileBrowser').ProjectFileNode[]
  videos: VideoOutput[]
  workflow: ProjectWorkflow
}
