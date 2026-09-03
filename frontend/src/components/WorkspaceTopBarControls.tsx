import { TooltipProvider } from './ui/tooltip'
import LlmModalHost from './topbar/LlmModalHost'
import RuntimeHealthControl from './topbar/RuntimeHealthControl'
import NotificationsControl from './topbar/NotificationsControl'
import AccountControl from './topbar/AccountControl'

/**
 * WorkspaceTopBarControls - the config/account controls relocated from the
 * former left WorkspaceSidebar. A slim container that composes one focused
 * component per control; each owns its own trigger and popover/modal wiring.
 *
 * The "Download Mac App" link that used to sit here (a plain anchor to the
 * GitHub releases page, hidden inside Electron) was removed 2026-09-03 at the
 * user's request; DesktopConnectButton still carries the full install +
 * connect flow for deployments that gate on the desktop app.
 */
export default function WorkspaceTopBarControls() {
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
      </div>
    </TooltipProvider>
  )
}
