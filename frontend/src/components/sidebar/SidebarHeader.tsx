import { Bot } from 'lucide-react'
import ThemeDropdown from '../ThemeDropdown'

export default function SidebarHeader() {
  return (
    <div className="flex items-center justify-between">
      <div className="flex items-center gap-2">
        <Bot className="w-5 h-5 text-blue-600 dark:text-blue-400 dark-plus:text-blue-400" />
        <span className="text-sm font-semibold text-gray-900 dark:text-gray-100 dark-plus:text-gray-100">AI Staff Engineer</span>
      </div>
      <ThemeDropdown />
    </div>
  )
}
