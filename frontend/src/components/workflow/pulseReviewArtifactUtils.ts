import type { PlannerFile } from '../../services/api-types'

export type PulseReviewArtifact = {
  path: string
  reviewRunId: string
  module: string
  lastModified?: string
}

const HISTORICAL_MODULES: Record<string, string[]> = {
  llm_ops_review: ['cost_llm_time'],
  stores_health: ['learning_health', 'knowledgebase_health', 'db_health'],
}

function plannerFilesFromResponse(response: unknown): PlannerFile[] {
  if (Array.isArray(response)) return response as PlannerFile[]
  if (response && typeof response === 'object' && 'data' in response) {
    const data = (response as { data?: unknown }).data
    if (Array.isArray(data)) return data as PlannerFile[]
  }
  return []
}

function flattenPlannerFiles(files: PlannerFile[]): PlannerFile[] {
  const result: PlannerFile[] = []
  const walk = (items: PlannerFile[]) => {
    items.forEach((item) => {
      if (item.type !== 'folder') result.push(item)
      if (Array.isArray(item.children)) walk(item.children)
    })
  }
  walk(files)
  return result
}

function absoluteArtifactPath(rawPath: string, workspacePath: string): string {
  const path = rawPath.replace(/\\/g, '/').replace(/^\/+/, '')
  const workspace = workspacePath.replace(/\\/g, '/').replace(/^\/+|\/+$/g, '')
  if (path === workspace || path.startsWith(`${workspace}/`)) return path

  const pulseMarker = 'pulse/reviews/'
  const pulseIndex = path.indexOf(pulseMarker)
  if (pulseIndex >= 0) return `${workspace}/${path.slice(pulseIndex)}`

  return `${workspace}/pulse/reviews/${path}`
}

export function collectPulseReviewArtifacts(
  response: unknown,
  workspacePath: string,
  module: string,
): PulseReviewArtifact[] {
  const acceptedModules = new Set([module, ...(HISTORICAL_MODULES[module] || [])])
  const reviewPathPattern = /(?:^|\/)pulse\/reviews\/([^/]+)\/([^/]+)\.md$/i
  const seen = new Set<string>()

  return flattenPlannerFiles(plannerFilesFromResponse(response))
    .flatMap((file): PulseReviewArtifact[] => {
      const path = absoluteArtifactPath(file.filepath || '', workspacePath)
      const match = path.match(reviewPathPattern)
      if (!match || !acceptedModules.has(match[2]) || seen.has(path)) return []
      seen.add(path)
      return [{
        path,
        reviewRunId: match[1],
        module: match[2],
        lastModified: file.last_modified,
      }]
    })
    .sort((a, b) => {
      const byRun = b.reviewRunId.localeCompare(a.reviewRunId)
      if (byRun !== 0) return byRun
      return (b.lastModified || '').localeCompare(a.lastModified || '')
    })
}

export function pulseReviewRunDate(reviewRunId: string): Date | null {
  const match = reviewRunId.match(
    /^(\d{4}-\d{2}-\d{2})T(\d{2})-(\d{2})-(\d{2})\.(\d{3})Z(?:_|$)/,
  )
  if (!match) return null
  const parsed = new Date(`${match[1]}T${match[2]}:${match[3]}:${match[4]}.${match[5]}Z`)
  return Number.isNaN(parsed.getTime()) ? null : parsed
}
