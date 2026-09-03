import type { ReactNode } from 'react'
import { AlertCircle, CheckCircle } from 'lucide-react'

// The red error / green success banner used by every Bots setup screen.
export function StatusBanner({ tone, children }: { tone: 'error' | 'success'; children: ReactNode }) {
  if (tone === 'error') {
    return (
      <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg flex items-start gap-2">
        <AlertCircle className="w-4 h-4 text-red-600 dark:text-red-400 flex-shrink-0 mt-0.5" />
        <p className="text-sm text-red-700 dark:text-red-300">{children}</p>
      </div>
    )
  }
  return (
    <div className="p-3 bg-green-50 dark:bg-green-900/20 border border-green-200 dark:border-green-800 rounded-lg flex items-start gap-2">
      <CheckCircle className="w-4 h-4 text-green-600 dark:text-green-400 flex-shrink-0 mt-0.5" />
      <p className="text-sm text-green-700 dark:text-green-300">{children}</p>
    </div>
  )
}
