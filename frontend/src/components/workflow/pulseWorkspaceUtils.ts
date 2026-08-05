import type {
  PulseFindingLifecycle,
  PulseReviewRecord,
} from '../../services/api-types'
import { isPulseFindingClosed } from './pulseModuleInspectorUtils'

export type PulseWorkspaceModuleDefinition = {
  id: string
  label: string
  description: string
}

export type PulseWorkspaceModuleSummary = PulseWorkspaceModuleDefinition & {
  findings: number
  active: number
  fixing: number
  awaitingVerification: number
  closed: number
  externalAction: number
  recurring: number
  latestReview: PulseReviewRecord | null
}

export type PulseReviewStorageSummary = {
  total: number
  migrated: number
  native: number
}

const ENGINEERING_REVIEW_ALIASES = new Set([
  'bug_review',
  'artifact_review',
  'report_health',
  'eval_health',
  'stores_health',
  'learning_health',
  'knowledgebase_health',
  'db_health',
])

/** Keep old Pulse records visible after the reviewer model is simplified. */
export function normalizePulseWorkspaceModule(module?: string): string {
  const value = (module || '').trim()
  if (ENGINEERING_REVIEW_ALIASES.has(value)) return 'workflow_review'
  if (value === 'cost_llm_time') return 'llm_ops_review'
  return value
}

export function summarizePulseReviewStorage(
  reviews: PulseReviewRecord[],
): PulseReviewStorageSummary {
  const migrated = reviews.filter((review) => (
    (review.legacy_source_path || '')
      .replaceAll('\\', '/')
      .includes('pulse/reviews/')
  )).length
  return {
    total: reviews.length,
    migrated,
    native: reviews.length - migrated,
  }
}

export function buildPulseWorkspaceModuleSummaries(
  definitions: PulseWorkspaceModuleDefinition[],
  findings: PulseFindingLifecycle[],
  reviews: PulseReviewRecord[],
): PulseWorkspaceModuleSummary[] {
  const latestReviewByModule = new Map<string, PulseReviewRecord>()
  reviews.forEach((review) => {
    const module = normalizePulseWorkspaceModule(review.module)
    const current = latestReviewByModule.get(module)
    if (!current || review.recorded_at.localeCompare(current.recorded_at) > 0) {
      latestReviewByModule.set(module, review)
    }
  })

  return definitions.map((definition) => {
    const definitionModule = normalizePulseWorkspaceModule(definition.id)
    const moduleFindings = findings.filter((finding) => (
      normalizePulseWorkspaceModule(finding.module) === definitionModule
    ))
    return {
      ...definition,
      findings: moduleFindings.length,
      active: moduleFindings.filter((finding) => (
        !isPulseFindingClosed(finding.status)
        && finding.status !== 'fixing'
        && finding.status !== 'awaiting_verification'
      )).length,
      fixing: moduleFindings.filter((finding) => finding.status === 'fixing').length,
      awaitingVerification: moduleFindings.filter((finding) => finding.status === 'awaiting_verification').length,
      closed: moduleFindings.filter((finding) => (
        isPulseFindingClosed(finding.status)
        && finding.status !== 'external_action_required'
      )).length,
      externalAction: moduleFindings.filter((finding) => finding.status === 'external_action_required').length,
      recurring: moduleFindings.filter((finding) => (
        finding.seen_count > 1
        && finding.status !== 'external_action_required'
      )).length,
      latestReview: latestReviewByModule.get(definitionModule) || null,
    }
  })
}

export function selectPulseWorkspaceModule(
  summaries: PulseWorkspaceModuleSummary[],
): string | null {
  if (summaries.length === 0) return null
  const ranked = [...summaries].sort((a, b) => {
    const aPriority = a.active * 100 + a.awaitingVerification * 20 + a.fixing * 10 + a.recurring
    const bPriority = b.active * 100 + b.awaitingVerification * 20 + b.fixing * 10 + b.recurring
    if (aPriority !== bPriority) return bPriority - aPriority
    return (b.latestReview?.recorded_at || '').localeCompare(a.latestReview?.recorded_at || '')
  })
  return ranked[0]?.id || summaries[0].id
}
