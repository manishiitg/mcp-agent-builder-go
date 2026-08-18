import { LineChart } from 'lucide-react'
import { cn } from '../../lib/utils'

type DominionMarkProps = {
  className?: string
  title?: string
}

// No bespoke SVG mark: reuse
// an existing lucide identity in a gradient badge rather than commission art
// for a v1 dashboard.
export function DominionMark({ className, title = 'Dominion' }: DominionMarkProps) {
  return (
    <div
      role={title ? 'img' : 'presentation'}
      aria-label={title || undefined}
      aria-hidden={title ? undefined : true}
      className={cn(
        'flex h-8 w-8 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-indigo-600 via-violet-500 to-purple-600',
        className,
      )}
    >
      <LineChart className="h-[55%] w-[55%] text-white" strokeWidth={2.25} />
    </div>
  )
}
