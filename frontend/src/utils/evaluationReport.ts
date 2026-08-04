import type { EvaluationStepScore, StepOutputContent } from '../services/api-types'

export interface EvaluationStepPlanDetails {
  id: string
  title?: string
  description?: string
}

const FINAL_SCORING_DISABLED_REASONING =
  'Final scoring is disabled; this report preserves the eval step output for metrics and review.'

const OUTPUT_CONTENT_EVIDENCE =
  "Inspect output_content for the eval step's structured verdict and evidence."

export const formatStepOutputContent = (outputContent?: StepOutputContent | null): string => {
  if (!outputContent) return ''

  const { content } = outputContent
  if (content === null || content === undefined) return ''
  if (typeof content === 'string') return content

  try {
    return JSON.stringify(content, null, 2)
  } catch {
    return String(content)
  }
}

export const hasStepOutputContent = (step?: EvaluationStepScore | null): boolean => {
  return formatStepOutputContent(step?.output_content).trim().length > 0
}

export const hasCapturedEvaluationScore = (step?: EvaluationStepScore | null): boolean => {
  if (!step || step.skipped || typeof step.score !== 'number') return false
  if (typeof step.score_captured === 'boolean') return step.score_captured
  // Compatibility for reports written before score_captured existed. The old
  // missing-score stub is stronger evidence than the legacy default score=0.
  return !(step.reasoning || '').startsWith('No score captured')
}

export const formatEvaluationScore = (step?: EvaluationStepScore | null): string => {
  if (!hasCapturedEvaluationScore(step)) return ''
  const score = Number(step!.score)
  const maxScore = step?.max_score
  return typeof maxScore === 'number' && maxScore > 0
    ? `${score}/${maxScore}`
    : String(score)
}

export const isFinalScoringPlaceholderText = (text?: string | null): boolean => {
  const normalized = (text || '').trim()
  return normalized === FINAL_SCORING_DISABLED_REASONING || normalized === OUTPUT_CONTENT_EVIDENCE
}

export const parseEvaluationPlanDetails = (evaluationPlan?: string | null): Map<string, EvaluationStepPlanDetails> => {
  const byId = new Map<string, EvaluationStepPlanDetails>()
  if (!evaluationPlan || !evaluationPlan.trim()) return byId

  try {
    const parsed = JSON.parse(evaluationPlan)
    const rawSteps = Array.isArray(parsed)
      ? parsed
      : (Array.isArray(parsed?.steps) ? parsed.steps : (Array.isArray(parsed?.eval_steps) ? parsed.eval_steps : []))

    for (const rawStep of rawSteps) {
      if (!rawStep || typeof rawStep !== 'object') continue
      const id = typeof rawStep.id === 'string' ? rawStep.id : ''
      if (!id) continue
      byId.set(id, {
        id,
        title: typeof rawStep.title === 'string' ? rawStep.title : undefined,
        description: typeof rawStep.description === 'string' ? rawStep.description : undefined,
      })
    }
  } catch {
    return byId
  }

  return byId
}
