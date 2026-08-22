import { Boxes } from 'lucide-react'
import { BRAND_MARKS } from './brandMarks'

interface ConnectionIconProps {
  /** Catalog `icon` slug, e.g. "notion". */
  icon?: string
  size?: 'sm' | 'md'
  className?: string
}

/**
 * A connector's mark, identified by its catalog `icon` slug alone.
 *
 * Brands whose logo genuinely carries several colours are drawn as published.
 * Brands that are single-colour by design — Notion, GitHub, Vercel — are drawn
 * in `currentColor`, so they take the tile's neutral foreground and stay legible
 * in both themes without needing a colour of their own. Anything with no
 * published mark, including custom MCP servers, falls back to a neutral glyph
 * rather than borrowing an unrelated icon.
 */
export default function ConnectionIcon({
  icon,
  size = 'md',
  className = '',
}: ConnectionIconProps) {
  const mark = icon ? BRAND_MARKS[icon] : undefined
  const box = size === 'sm' ? 'h-8 w-8' : 'h-10 w-10'
  const glyph = size === 'sm' ? 'h-4 w-4' : 'h-5 w-5'

  return (
    <span
      // A neutral tile in both themes; the mark itself carries the recognition.
      className={`flex ${box} shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-500 dark:bg-slate-800 dark:text-gray-400 ${className}`}
    >
      {mark ? (
        <svg
          viewBox={mark.viewBox}
          className={glyph}
          // Non-square marks keep their proportions rather than being stretched
          // to fill the tile.
          preserveAspectRatio="xMidYMid meet"
          fill={mark.mono ? 'currentColor' : undefined}
          aria-hidden="true"
          focusable="false"
          dangerouslySetInnerHTML={{ __html: mark.markup }}
        />
      ) : (
        <Boxes className={glyph} aria-hidden="true" />
      )}
    </span>
  )
}
