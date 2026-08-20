import {
  Boxes,
  Github,
  Mail,
  MessageSquare,
  Bug,
  ListTodo,
  FileText,
  type LucideIcon,
} from 'lucide-react'

/**
 * Maps a catalog `icon` slug to a mark. Brand SVGs are avoided so the panel
 * carries no third-party trademark assets; the brand colour does the
 * recognition work instead.
 */
const ICONS: Record<string, LucideIcon> = {
  notion: FileText,
  github: Github,
  linear: ListTodo,
  sentry: Bug,
  google: Mail,
  microsoft: Mail,
  slack: MessageSquare,
}

interface ConnectionIconProps {
  icon?: string
  brandColor?: string
  size?: 'sm' | 'md'
  className?: string
}

export default function ConnectionIcon({
  icon,
  brandColor,
  size = 'md',
  className = '',
}: ConnectionIconProps) {
  const Icon = (icon && ICONS[icon]) || Boxes
  const box = size === 'sm' ? 'h-8 w-8' : 'h-10 w-10'
  const glyph = size === 'sm' ? 'h-4 w-4' : 'h-5 w-5'

  return (
    <span
      className={`flex ${box} shrink-0 items-center justify-center rounded-lg ${className}`}
      style={{
        // A tint of the brand colour keeps cards recognisable in both themes
        // without shipping brand artwork.
        backgroundColor: brandColor ? `${brandColor}1A` : undefined,
        color: brandColor || undefined,
      }}
    >
      <Icon
        className={`${glyph} ${brandColor ? '' : 'text-gray-500 dark:text-gray-400'}`}
        aria-hidden="true"
      />
    </span>
  )
}
