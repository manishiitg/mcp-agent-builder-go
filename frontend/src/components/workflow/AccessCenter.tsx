import React, { useEffect, useState } from 'react'
import { ShieldCheck, Share2, Users, X } from 'lucide-react'
import ModalPortal from '../ui/ModalPortal'
import WorkflowSharePopup from './WorkflowSharePopup'
import UsersAdminPanel from '../admin/UsersAdminPanel'

export type AccessCenterTab = 'workflow' | 'users'

interface AccessCenterProps {
  isOpen: boolean
  onClose: () => void
  /** The open workflow; enables the "This workflow" tab for its owners. */
  workspacePath?: string
  /** Caller may share this workflow (owner or admin). */
  canShareWorkflow: boolean
  /** Caller may manage the deployment's accounts (admin). */
  canManageUsers: boolean
  initialTab?: AccessCenterTab
}

/**
 * AccessCenter - one place for everything about who can do what, replacing
 * two separate toolbar buttons that users found hard to tell apart:
 *
 *  - "This workflow": who may see or edit the open workflow (owners and
 *    read-only readers) -- the former Share popup.
 *  - "Users": the deployment's accounts, roles, passwords and products
 *    (admins only) -- the former Users & access panel.
 *
 * Both bodies are the existing components rendered `embedded`, so their
 * data loading and actions are unchanged; this is only the shell and tabs.
 */
const AccessCenter: React.FC<AccessCenterProps> = ({ isOpen, onClose, workspacePath, canShareWorkflow, canManageUsers, initialTab }) => {
  const workflowTab = canShareWorkflow && !!workspacePath
  const usersTab = canManageUsers
  const firstTab: AccessCenterTab = workflowTab ? 'workflow' : 'users'
  const [tab, setTab] = useState<AccessCenterTab>(initialTab ?? firstTab)

  useEffect(() => {
    if (!isOpen) return
    const wanted = initialTab ?? firstTab
    setTab((current) => {
      const available = (current === 'workflow' && workflowTab) || (current === 'users' && usersTab)
      return available ? current : wanted
    })
  }, [isOpen, initialTab, firstTab, workflowTab, usersTab])

  if (!isOpen || (!workflowTab && !usersTab)) return null

  const tabClass = (active: boolean) =>
    `inline-flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md transition-colors ${active ? 'bg-background text-foreground shadow-sm border border-border' : 'text-muted-foreground hover:text-foreground hover:bg-background/60'}`

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4" onClick={onClose}>
        <div
          className="bg-background border border-border rounded-lg shadow-xl w-full max-w-4xl max-h-[88vh] flex flex-col"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between gap-3 p-4 border-b border-border">
            <div className="flex items-center gap-2 min-w-0">
              <ShieldCheck className="w-5 h-5 text-muted-foreground shrink-0" />
              <h2 className="text-lg font-semibold">Access</h2>
            </div>
            <div className="flex items-center gap-1 rounded-lg bg-muted/60 p-1" role="tablist" aria-label="Access sections">
              {workflowTab && (
                <button type="button" role="tab" aria-selected={tab === 'workflow'} className={tabClass(tab === 'workflow')} onClick={() => setTab('workflow')}>
                  <Share2 className="w-3.5 h-3.5" /> This workflow
                </button>
              )}
              {usersTab && (
                <button type="button" role="tab" aria-selected={tab === 'users'} className={tabClass(tab === 'users')} onClick={() => setTab('users')}>
                  <Users className="w-3.5 h-3.5" /> Users
                </button>
              )}
            </div>
            <button onClick={onClose} className="p-1 rounded hover:bg-accent" aria-label="Close">
              <X className="w-4 h-4" />
            </button>
          </div>

          {tab === 'workflow' && workflowTab && workspacePath && (
            <WorkflowSharePopup embedded isOpen onClose={onClose} workspacePath={workspacePath} />
          )}
          {tab === 'users' && usersTab && (
            <UsersAdminPanel embedded isOpen onClose={onClose} />
          )}
        </div>
      </div>
    </ModalPortal>
  )
}

export default AccessCenter
