import { describe, expect, it } from 'vitest'
import { extractWorkflowSoulSummary } from './soulSummaryUtils'

describe('workflow soul summary', () => {
  it('extracts the objective paragraph and first concrete numbered criterion', () => {
    const summary = extractWorkflowSoulSummary(`# Example

## Objective

Continuously monitor the complete experience.

Additional objective detail.

## Success Criteria

### Primary signal

A run is successful when ALL checks pass:

1. Voice latency is decomposed by layer with current evidence.
2. Regressions are flagged.

## Constraints

- Never mutate production infrastructure.
`)

    expect(summary).toEqual({
      goal: 'Continuously monitor the complete experience.',
      success: 'Voice latency is decomposed by layer with current evidence.',
      constraints: 'Never mutate production infrastructure.',
    })
  })

  it('supports Goal, Success, and Guardrails headings with bullet criteria', () => {
    const summary = extractWorkflowSoulSummary(`## Goal
Publish useful recommendations.

## Success
- Every recommendation includes reproducible proof.

## Guardrails
- Do not fabricate unavailable evidence.
`)

    expect(summary).toEqual({
      goal: 'Publish useful recommendations.',
      success: 'Every recommendation includes reproducible proof.',
      constraints: 'Do not fabricate unavailable evidence.',
    })
  })
})
