import type { CSSProperties } from 'react'
import { Boxes } from 'lucide-react'
import { BRAND_PATHS } from './brandPaths'

interface ConnectionIconProps {
  /** Catalog `icon` slug, e.g. "notion". */
  icon?: string
  brandColor?: string
  size?: 'sm' | 'md'
  className?: string
}

/**
 * Relative luminance of a #rrggbb colour, per WCAG. Used only to decide
 * whether a brand colour survives on a dark background.
 */
function luminance(hex: string): number {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim())
  if (!m) return 1

  const channel = (v: number) => {
    const c = v / 255
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4)
  }
  const int = parseInt(m[1], 16)
  const r = channel((int >> 16) & 0xff)
  const g = channel((int >> 8) & 0xff)
  const b = channel(int & 0xff)
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

/**
 * Several brands are near-black — Notion is #000000, GitHub #181717 — which
 * disappears on a dark background. Those are lifted to a light neutral in dark
 * mode; brands that already read well keep their own colour.
 */
function markColorForDarkTheme(brandColor?: string): string | undefined {
  if (!brandColor) return undefined
  return luminance(brandColor) < 0.12 ? '#E8EAED' : brandColor
}

/**
 * A connector's mark: the real brand logo when one is vendored for it, and a
 * neutral glyph otherwise. Custom MCP servers and brands with no available mark
 * fall back rather than being given an unrelated icon, which reads as a logo
 * the service does not actually have.
 */
export default function ConnectionIcon({
  icon,
  brandColor,
  size = 'md',
  className = '',
}: ConnectionIconProps) {
  const path = icon ? BRAND_PATHS[icon] : undefined
  const box = size === 'sm' ? 'h-8 w-8' : 'h-10 w-10'
  const glyph = size === 'sm' ? 'h-4 w-4' : 'h-5 w-5'

  return (
    <span
      // A neutral tile in both themes, so legibility never depends on the brand
      // colour; the mark itself carries the recognition.
      className={`flex ${box} shrink-0 items-center justify-center rounded-lg bg-gray-100 text-gray-500 dark:bg-slate-800 dark:text-gray-400 ${
        brandColor ? 'text-[var(--brand)] dark:text-[var(--brand-dark)]' : ''
      } ${className}`}
      style={
        brandColor
          ? ({
              '--brand': brandColor,
              '--brand-dark': markColorForDarkTheme(brandColor),
            } as CSSProperties)
          : undefined
      }
    >
      {path ? (
        <svg
          viewBox="0 0 24 24"
          className={glyph}
          fill="currentColor"
          aria-hidden="true"
          focusable="false"
        >
          <path d={path} />
        </svg>
      ) : (
        <Boxes className={glyph} aria-hidden="true" />
      )}
    </span>
  )
}
