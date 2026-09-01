import { useEffect, useRef, useState, type ReactNode } from 'react'
import { BrainCircuit, Download, MoreHorizontal } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './ui/tooltip'
import LlmModalHost from './topbar/LlmModalHost'
import RuntimeHealthControl from './topbar/RuntimeHealthControl'
import NotificationsControl from './topbar/NotificationsControl'
import AccountControl from './topbar/AccountControl'
import { iconButtonClass } from './ui/IconPopover'
import { useLLMStore } from '../stores'
import { useIsElectron } from './topbar/useIsElectron'

function ToolMenuItem({
  icon,
  label,
  detail,
  onClick,
}: {
  icon: ReactNode
  label: string
  detail?: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex w-full items-center gap-3 rounded-md px-3 py-2 text-left text-gray-700 transition-colors hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-slate-700"
    >
      <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-gray-100 text-gray-500 dark:bg-slate-700 dark:text-gray-300">
        {icon}
      </span>
      <span className="min-w-0">
        <span className="block text-sm font-medium leading-5">{label}</span>
        {detail && <span className="block truncate text-xs text-gray-500 dark:text-gray-400">{detail}</span>}
      </span>
    </button>
  )
}

/**
 * WorkspaceTopBarControls - the config/account controls relocated from the
 * former left WorkspaceSidebar. A slim container that composes one focused
 * component per control; each owns its own trigger and popover/modal wiring.
 */
export default function WorkspaceTopBarControls() {
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const setShowLLMModal = useLLMStore(s => s.setShowLLMModal)
  const llmCount = useLLMStore(s => s.savedLLMs.length)
  const isElectron = useIsElectron()

  useEffect(() => {
    if (!menuOpen) return
    const onMouseDown = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setMenuOpen(false)
      }
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenuOpen(false)
    }
    document.addEventListener('mousedown', onMouseDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('mousedown', onMouseDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [menuOpen])

  return (
    <TooltipProvider delayDuration={400}>
      {/* LlmModalHost renders the LLM modals once; the trigger now lives in the
          compact workspace tools menu. */}
      <LlmModalHost />
      <div className="flex items-center gap-1.5">
        <RuntimeHealthControl />
        <NotificationsControl />
        <AccountControl />

        <div ref={menuRef} className="relative">
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                type="button"
                onClick={() => setMenuOpen(prev => !prev)}
                aria-label="Workspace tools"
                className={`${iconButtonClass} ${menuOpen ? 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-200' : ''}`}
              >
                <MoreHorizontal className="h-4 w-4" />
              </button>
            </TooltipTrigger>
            <TooltipContent side="bottom">Workspace tools</TooltipContent>
          </Tooltip>

          {menuOpen && (
            <div className="absolute right-0 top-full z-[60] mt-2 w-72 rounded-lg border border-gray-200 bg-white p-2 shadow-xl dark:border-slate-700 dark:bg-slate-800">
              <div className="px-2 pb-2 pt-1">
                <div className="text-xs font-semibold uppercase tracking-wide text-gray-400 dark:text-gray-500">Workspace Tools</div>
              </div>
              <div className="space-y-1">
                <ToolMenuItem
                  icon={<BrainCircuit className="h-4 w-4" />}
                  label="Models"
                  detail={`${llmCount} enabled`}
                  onClick={() => {
                    setMenuOpen(false)
                    setShowLLMModal(true)
                  }}
                />
                {!isElectron && (
                  <a
                    href="https://github.com/manishiitg/coding-agent-loop/releases/latest"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="flex w-full items-center gap-3 rounded-md px-3 py-2 text-left text-gray-700 transition-colors hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-slate-700"
                    onClick={() => setMenuOpen(false)}
                  >
                    <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-gray-100 text-blue-500 dark:bg-slate-700 dark:text-blue-300">
                      <Download className="h-4 w-4" />
                    </span>
                    <span className="min-w-0">
                      <span className="block text-sm font-medium leading-5">Download Mac App</span>
                      <span className="block truncate text-xs text-gray-500 dark:text-gray-400">Latest release</span>
                    </span>
                  </a>
                )}
              </div>
            </div>
          )}
        </div>
      </div>
    </TooltipProvider>
  )
}
