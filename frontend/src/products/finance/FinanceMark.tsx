import { Wallet } from 'lucide-react'
import { cn } from '../../lib/utils'

type FinanceMarkProps = {
  className?: string
  title?: string
}

// No bespoke SVG mark: reuse an
// existing lucide identity in a gradient badge rather than commission art
// for a v1 dashboard.
export function FinanceMark({ className, title = 'Finance' }: FinanceMarkProps) {
  return (
    <div
      role={title ? 'img' : 'presentation'}
      aria-label={title || undefined}
      aria-hidden={title ? undefined : true}
      className={cn(
        'flex h-8 w-8 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-emerald-600 via-emerald-500 to-teal-600',
        className,
      )}
    >
      <Wallet className="h-[55%] w-[55%] text-white" strokeWidth={2.25} />
    </div>
  )
}
