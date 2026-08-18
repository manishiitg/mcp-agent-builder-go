import type {
  PulseFindingLifecycle,
  PulseReviewRecord,
} from '../../services/api-types'
import { summarizePulseModule } from './pulseModuleInspectorUtils'

export type PulseWorkspaceModuleDefinition = {
  id: string
  label: string
  description: string
}

export type PulseWorkspaceModuleSummary = PulseWorkspaceModuleDefinition & {
  findings: number
  /** Work Pulse can diagnose or repair now, including an in-progress fix. */
  active: number
  fixing: number
  awaitingVerification: number
  awaitingRun: number
  queuedForEngineering: number
  awaitingUser: number
  blocked: number
  proposals: number
  closed: number
  externalAction: number
  workflowReported: number
  recurring: number
  latestReview: PulseReviewRecord | null
}

export function normalizePulseWorkspaceModule(module?: string): string {
  const value = (module || '').trim()
  if (value === 'strategy_auditor' || value === 'goal_advisor') return 'strategic_review'
  return value
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
    const lifecycle = summarizePulseModule(moduleFindings)
    return {
      ...definition,
      findings: moduleFindings.length,
      active: lifecycle.open,
      fixing: lifecycle.fixing,
      awaitingVerification: lifecycle.awaitingVerification,
      awaitingRun: lifecycle.awaitingRun,
      queuedForEngineering: lifecycle.queuedForEngineering,
      awaitingUser: lifecycle.awaitingUser,
      blocked: lifecycle.blocked,
      proposals: lifecycle.proposals,
      closed: lifecycle.closed,
      externalAction: lifecycle.externalAction,
      workflowReported: lifecycle.workflowReported,
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
    const aPriority = (a.active + a.fixing + a.queuedForEngineering) * 100
      + (a.awaitingVerification + a.awaitingRun) * 20
      + a.awaitingUser * 10
      + a.recurring
    const bPriority = (b.active + b.fixing + b.queuedForEngineering) * 100
      + (b.awaitingVerification + b.awaitingRun) * 20
      + b.awaitingUser * 10
      + b.recurring
    if (aPriority !== bPriority) return bPriority - aPriority
    return (b.latestReview?.recorded_at || '').localeCompare(a.latestReview?.recorded_at || '')
  })
  return ranked[0]?.id || summaries[0].id
}
