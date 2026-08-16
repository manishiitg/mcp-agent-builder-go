import { Users } from 'lucide-react'
import { cn } from '../../lib/utils'

type ChiefOfStaffMarkProps = {
  className?: string
  title?: string
}

// No bespoke SVG mark exists for Chief of Staff the way Video Studio has
// VideoStudioMark -- reuses the existing lucide Users identity already used
// for the 'multi-agent' mode card (frontend/src/constants/modeInfo.tsx)
// rather than commissioning new art, wrapped in a gradient badge for the
// same visual weight as the other two product tiles in the switcher.
export function ChiefOfStaffMark({ className, title = 'Chief of Staff' }: ChiefOfStaffMarkProps) {
  return (
    <div
      role={title ? 'img' : 'presentation'}
      aria-label={title || undefined}
      aria-hidden={title ? undefined : true}
      className={cn(
        'flex h-8 w-8 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-indigo-600 via-indigo-500 to-violet-600',
        className,
      )}
    >
      <Users className="h-[55%] w-[55%] text-white" strokeWidth={2.25} />
    </div>
  )
}
