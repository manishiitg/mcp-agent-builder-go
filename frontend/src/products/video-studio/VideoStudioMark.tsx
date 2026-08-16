import { useId, type ComponentPropsWithoutRef } from 'react'
import { cn } from '../../lib/utils'

type VideoStudioMarkProps = ComponentPropsWithoutRef<'svg'> & {
  title?: string
}

export function VideoStudioMark({
  className,
  title = 'Video Studio',
  ...props
}: VideoStudioMarkProps) {
  const id = useId().replace(/:/g, '')
  const backgroundId = `${id}-background`
  const playId = `${id}-play`

  return (
    <svg
      viewBox="0 0 64 64"
      fill="none"
      aria-hidden={title ? undefined : true}
      role={title ? 'img' : 'presentation'}
      className={cn('h-8 w-8', className)}
      {...props}
    >
      {title ? <title>{title}</title> : null}
      <defs>
        <linearGradient id={backgroundId} x1="8" y1="5" x2="56" y2="59" gradientUnits="userSpaceOnUse">
          <stop stopColor="#312E81" />
          <stop offset="0.55" stopColor="#6D28D9" />
          <stop offset="1" stopColor="#DB2777" />
        </linearGradient>
        <linearGradient id={playId} x1="24" y1="19" x2="45" y2="44" gradientUnits="userSpaceOnUse">
          <stop stopColor="#FFFFFF" />
          <stop offset="1" stopColor="#FCE7F3" />
        </linearGradient>
      </defs>
      <rect x="4" y="4" width="56" height="56" rx="17" fill={`url(#${backgroundId})`} />
      <path d="M8 22L56 13" stroke="white" strokeOpacity="0.23" strokeWidth="2" />
      <path d="M8 44L56 35" stroke="white" strokeOpacity="0.14" strokeWidth="2" />
      <circle cx="47" cy="16" r="9" fill="white" fillOpacity="0.1" />
      <path
        d="M25 20.9C25 19.25 26.82 18.25 28.22 19.13L45.19 29.78C46.5 30.6 46.5 32.5 45.19 33.32L28.22 43.97C26.82 44.85 25 43.85 25 42.2V20.9Z"
        fill={`url(#${playId})`}
      />
    </svg>
  )
}
