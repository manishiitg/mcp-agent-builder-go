export type ProductionCounts = { videos: number; characters: number; references: number; documents: number }

/**
 * Which section, if any, just grew and should be scrolled into view.
 * Characters take priority over documents over videos: an unapproved
 * character is the more time-sensitive thing to see, and a finished video
 * already has its own reveal (the mobile fullscreen player).
 *
 * `previous: null` means this is the first known state (e.g. the panel just
 * mounted) -- nothing "grew" relative to nothing, so there is no single
 * section to jump to even if every section already has content.
 */
export function sectionThatGrew(previous: ProductionCounts | null, current: ProductionCounts): 'characters' | 'references' | 'documents' | 'videos' | null {
  if (!previous) return null
  if (current.characters > previous.characters) return 'characters'
  if (current.references > previous.references) return 'references'
  if (current.documents > previous.documents) return 'documents'
  if (current.videos > previous.videos) return 'videos'
  return null
}
