import { Download } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from './ui/tooltip'
import LlmModalHost from './topbar/LlmModalHost'
import RuntimeHealthControl from './topbar/RuntimeHealthControl'
import NotificationsControl from './topbar/NotificationsControl'
import AccountControl from './topbar/AccountControl'
import { iconButtonClass } from './ui/IconPopover'
import { useIsElectron } from './topbar/useIsElectron'

/**
 * WorkspaceTopBarControls - the config/account controls relocated from the
 * former left WorkspaceSidebar. A slim container that composes one focused
 * component per control; each owns its own trigger and popover/modal wiring.
 */
export default function WorkspaceTopBarControls() {
  const isElectron = useIsElectron()

  return (
    <TooltipProvider delayDuration={400}>
      {/* LlmModalHost renders the LLM modal; it's no longer manually
          triggered from here -- only via first-run onboarding when no LLM
          is configured yet. LLM setup otherwise lives in each workflow's
          capabilities panel. */}
      <LlmModalHost />
      <div className="flex items-center gap-1.5">
        <RuntimeHealthControl />
        <NotificationsControl />
        <AccountControl />

        {!isElectron && (
          <Tooltip>
            <TooltipTrigger asChild>
              <a
                href="https://github.com/manishiitg/coding-agent-loop/releases/latest"
                target="_blank"
                rel="noopener noreferrer"
                aria-label="Download Mac App"
                className={iconButtonClass}
              >
                <Download className="h-4 w-4" />
              </a>
            </TooltipTrigger>
            <TooltipContent side="bottom">Download Mac App</TooltipContent>
          </Tooltip>
        )}
      </div>
    </TooltipProvider>
  )
}
