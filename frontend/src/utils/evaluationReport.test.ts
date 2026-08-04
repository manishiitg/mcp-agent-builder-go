import { describe, expect, it } from 'vitest'
import { formatEvaluationScore, hasCapturedEvaluationScore } from './evaluationReport'

describe('evaluation score presentation', () => {
  it('shows a captured zero and preserves fractional scores', () => {
    expect(hasCapturedEvaluationScore({ step_id: 'zero', score: 0, max_score: 10, score_captured: true })).toBe(true)
    expect(formatEvaluationScore({ step_id: 'zero', score: 0, max_score: 10, score_captured: true })).toBe('0/10')
    expect(formatEvaluationScore({ step_id: 'fraction', score: 7.5, max_score: 10, score_captured: true })).toBe('7.5/10')
  })

  it('does not turn a missing or skipped score into a displayed zero', () => {
    expect(hasCapturedEvaluationScore({ step_id: 'missing', score: 0, score_captured: false })).toBe(false)
    expect(formatEvaluationScore({ step_id: 'skipped', score: 0, score_captured: false, skipped: true })).toBe('')
  })

  it('keeps compatible legacy scores but rejects the legacy missing-score stub', () => {
    expect(formatEvaluationScore({ step_id: 'legacy', score: 8, max_score: 10 })).toBe('8/10')
    expect(formatEvaluationScore({ step_id: 'legacy-missing', score: 0, reasoning: 'No score captured — missing output.' })).toBe('')
  })
})
