import { describe, expect, it } from 'vitest'
import type { SavedLLM } from '../../services/api-types'
import { effectiveLLMUnderLock, effectiveProviderUnderLock } from '../effectiveLLM'

const published = [{ provider: 'cursor-cli', model_id: 'cursor-cli' }] as unknown as SavedLLM[]

describe('effectiveProviderUnderLock', () => {
  it('replaces a saved provider with the published one under a lock', () => {
    expect(effectiveProviderUnderLock('claude-code', true, published)).toBe('cursor-cli')
  })
  it('keeps a saved provider that is itself published', () => {
    expect(effectiveProviderUnderLock('Cursor-CLI', true, published)).toBe('Cursor-CLI')
  })
  it('is a pass-through without a lock or without a published list', () => {
    expect(effectiveProviderUnderLock('claude-code', false, published)).toBe('claude-code')
    expect(effectiveProviderUnderLock('claude-code', true, [])).toBe('claude-code')
    expect(effectiveProviderUnderLock(null, false, published)).toBeNull()
  })
  it('falls back to the published provider when nothing was saved', () => {
    expect(effectiveProviderUnderLock(null, true, published)).toBe('cursor-cli')
  })
  it('agrees with the full provider+model rule', () => {
    const full = effectiveLLMUnderLock({ provider: 'claude-code', model_id: 'claude-sonnet-5' }, true, published)
    expect(full?.provider).toBe(effectiveProviderUnderLock('claude-code', true, published))
  })
})
