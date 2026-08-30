import { describe, expect, it } from 'vitest'
import { sectionThatGrew } from './productionPanelScroll'

describe('sectionThatGrew', () => {
  it('scrolls to the section that just grew', () => {
    // Reported bug: show_character succeeded and the Characters section
    // updated, but it sat off-screen below Videos with nothing to draw the
    // eye there.
    expect(sectionThatGrew({ videos: 1, characters: 0, references: 0, documents: 2 }, { videos: 1, characters: 1, references: 0, documents: 2 })).toBe('characters')
  })

  it('prioritizes characters over documents over videos when several grow at once', () => {
    expect(sectionThatGrew({ videos: 0, characters: 0, references: 0, documents: 0 }, { videos: 1, characters: 1, references: 1, documents: 1 })).toBe('characters')
    expect(sectionThatGrew({ videos: 0, characters: 1, references: 0, documents: 0 }, { videos: 1, characters: 1, references: 1, documents: 1 })).toBe('references')
    expect(sectionThatGrew({ videos: 0, characters: 1, references: 1, documents: 0 }, { videos: 1, characters: 1, references: 1, documents: 1 })).toBe('documents')
  })

  it('does nothing on the first known state, even if every section already has content', () => {
    // An existing project opening with videos/characters/documents already
    // present has no single new thing to jump to.
    expect(sectionThatGrew(null, { videos: 3, characters: 2, references: 5, documents: 4 })).toBeNull()
  })

  it('does nothing when nothing grew', () => {
    expect(sectionThatGrew({ videos: 2, characters: 1, references: 1, documents: 1 }, { videos: 2, characters: 1, references: 1, documents: 1 })).toBeNull()
  })

  it('does nothing when a count only shrinks', () => {
    expect(sectionThatGrew({ videos: 2, characters: 1, references: 1, documents: 1 }, { videos: 1, characters: 1, references: 1, documents: 1 })).toBeNull()
  })
})
