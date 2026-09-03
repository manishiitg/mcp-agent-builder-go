import { useState } from 'react'
import { KeyRound, LogOut, User } from 'lucide-react'
import { Tooltip, TooltipContent, TooltipTrigger } from '../ui/tooltip'
import { useAuthStore } from '../../stores/useAuthStore'
import ChangePasswordDialog from './ChangePasswordDialog'

/**
 * AccountControl - signed-in user indicator, change-password, and sign-out.
 * Rendered only in multi-user mode with an authenticated user.
 */
export default function AccountControl() {
  const { user, logout, isMultiUserMode } = useAuthStore()
  const [changingPassword, setChangingPassword] = useState(false)
  if (!isMultiUserMode || !user) return null

  const buttonClass =
    'p-1.5 rounded-md text-gray-500 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors'

  return (
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <div className="w-7 h-7 rounded-full bg-primary/10 flex items-center justify-center cursor-default">
            <User className="w-4 h-4 text-primary" />
          </div>
        </TooltipTrigger>
        <TooltipContent side="bottom">
          <div className="text-xs">
            <p className="font-medium">{user.username || user.email || 'User'}</p>
            {user.email && user.username !== user.email && (
              <p className="text-gray-300">{user.email}</p>
            )}
          </div>
        </TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            onClick={() => setChangingPassword(true)}
            aria-label="Change password"
            className={`${buttonClass} hover:text-gray-900 dark:hover:text-gray-100`}
          >
            <KeyRound className="w-4 h-4" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="bottom">Change password</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            onClick={logout}
            aria-label="Sign out"
            className={`${buttonClass} hover:text-red-600 dark:hover:text-red-400`}
          >
            <LogOut className="w-4 h-4" />
          </button>
        </TooltipTrigger>
        <TooltipContent side="bottom">Sign out</TooltipContent>
      </Tooltip>
      <ChangePasswordDialog isOpen={changingPassword} onClose={() => setChangingPassword(false)} />
    </>
  )
}
