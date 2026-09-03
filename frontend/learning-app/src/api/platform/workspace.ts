// The family workspace as the SparkQuill UI sees it, served by the
// platform's workspace API through the agent server's proxy (/api/wp).
// Paths on this side are family-root-relative ("reports/progress.html");
// the proxy stamps the user, and "Chats/…" resolves to that user's tree.
import type { Activity, TreeNode } from '../../stores/types'
import type { FileContent, TreeResponse, UploadResult } from '../familyApi'

export const FAMILY_ROOT = 'Chats/SparkQuill'
export const ACTIVITIES = 'activities'

export type Requester = <T>(method: string, path: string, body?: unknown) => Promise<T>

type Document = { filepath: string; type?: string; children?: Document[]; size?: number; content?: string; is_binary?: boolean; is_image?: boolean; mime_type?: string }
type APIResponse<T> = { success?: boolean; data?: T; error?: string }

/** Turns a family-relative path into the proxy's user-relative one. */
export function workspacePath(rel: string): string {
  const clean = String(rel ?? '').replace(/^\/+/, '')
  if (clean.startsWith('_users/') || clean === FAMILY_ROOT || clean.startsWith(`${FAMILY_ROOT}/`)) return clean
  return clean ? `${FAMILY_ROOT}/${clean}` : FAMILY_ROOT
}

/** Strips the family root (and any per-user prefix) off a workspace path. */
export function familyRelative(path: string): string {
  const clean = String(path ?? '').replace(/^\/+/, '')
  const marker = `${FAMILY_ROOT}/`
  const i = clean.indexOf(marker)
  if (i >= 0) return clean.slice(i + marker.length)
  return clean === FAMILY_ROOT ? '' : clean
}

export function encodePath(path: string): string {
  return path.split('/').map(encodeURIComponent).join('/')
}

export function documentsURL(rel: string, suffix = ''): string {
  return `/api/wp/api/documents/${encodePath(workspacePath(rel))}${suffix}`
}

export class FamilyWorkspace {
  constructor(private readonly request: Requester) {}

  async readFile(rel: string): Promise<FileContent> {
    const resp = await this.request<APIResponse<Document>>('GET', documentsURL(rel))
    const d: Document = resp?.data ?? { filepath: rel }
    return { path: familyRelative(rel), is_text: !d.is_binary, content: d.content ?? '', size: d.size }
  }

  async readJSON<T>(rel: string): Promise<T | null> {
    const f = await this.readFile(rel).catch(() => null)
    if (!f?.content) return null
    try { return JSON.parse(f.content) as T } catch { return null }
  }

  async writeFile(rel: string, content: string): Promise<void> {
    await this.request('PUT', documentsURL(rel), { content })
  }

  async writeJSON(rel: string, value: unknown): Promise<void> {
    await this.writeFile(rel, JSON.stringify(value, null, 2) + '\n')
  }

  /** The family tree in the UI's shape: family-relative paths, folders first as the API returns them. */
  async tree(): Promise<TreeResponse> {
    const resp = await this.request<APIResponse<Document[]>>('GET', `/api/wp/api/documents?folder=${encodeURIComponent(FAMILY_ROOT)}&max_depth=-1`)
    let total = 0
    const convert = (docs: Document[] | undefined): TreeNode[] => (docs ?? []).map((d) => {
      const path = familyRelative(d.filepath)
      const node: TreeNode = { name: path.split('/').pop() ?? path, path, type: d.type === 'folder' ? 'dir' : 'file' }
      if (d.type === 'folder') node.children = convert(d.children)
      else { node.size = d.size; total += d.size ?? 0 }
      return node
    })
    const nodes = convert(resp?.data)
    return { nodes, total_size: total }
  }

  async upload(file: File, folderRel: string): Promise<UploadResult> {
    const fd = new FormData()
    fd.append('folder_path', workspacePath(folderRel))
    fd.append('file', file)
    const resp = await this.request<{ filepath?: string; success?: boolean; error?: string; data?: { filepath?: string } }>('POST', '/api/wp/api/upload', fd)
    const saved = resp?.filepath ?? resp?.data?.filepath ?? `${folderRel}/${file.name}`
    if (resp?.error) return { name: file.name, error: resp.error }
    return { name: saved.split('/').pop() ?? file.name, path: familyRelative(saved) }
  }

  /** The iframe scene bridge's save/load, one JSON file per key. */
  stateFile(key: string): string {
    const safe = key.replace(/[^A-Za-z0-9._-]+/g, '_').slice(0, 120) || 'state'
    return `state/${safe}.json`
  }

  async activity(dir: string): Promise<Activity | null> {
    const rel = familyRelative(dir).replace(/\/+$/, '')
    const m = await this.readJSON<{ title?: string; subject?: string; topic?: string; items?: string[]; goal?: string; persona?: string; created_at?: string }>(`${rel}/activity.json`)
    if (!m) return null
    return {
      dir: rel,
      title: m.title ?? rel.split('/').pop() ?? rel,
      subject: m.subject,
      topic: m.topic,
      items: (m.items ?? []).map((name) => ({ path: `${rel}/${name}`, name })),
      goal: m.goal,
      persona: m.persona,
      created_at: m.created_at,
      attempts: [],
    }
  }

  async activities(): Promise<Activity[]> {
    const resp = await this.request<APIResponse<Document[]>>('GET', `/api/wp/api/documents?folder=${encodeURIComponent(`${FAMILY_ROOT}/${ACTIVITIES}`)}&max_depth=1`).catch(() => null)
    const folders = (resp?.data ?? []).filter((d) => d.type === 'folder')
    const out = await Promise.all(folders.map((d) => this.activity(d.filepath)))
    return out.filter((a): a is Activity => a !== null).sort((a, b) => (b.created_at ?? '').localeCompare(a.created_at ?? ''))
  }

  async currentActivity(): Promise<Activity | null> {
    const pointer = await this.readJSON<{ dir?: string }>('current-activity.json')
    if (!pointer?.dir) return null
    return this.activity(pointer.dir)
  }

}
