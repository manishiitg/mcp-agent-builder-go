import React from 'react'
import { X } from 'lucide-react'
import ModalPortal from '../ui/ModalPortal'

interface InspectorShellProps {
  /** Rendered inside the workspace pane (no portal, no backdrop, no close controls). */
  embedded: boolean
  /** Modal path only: render nothing while closed. */
  isOpen: boolean
  onClose: () => void
  /** The shell box's classes in the modal path (size/shape differ per view). */
  modalClassName: string
  /** The shell box's classes in the embedded path. */
  embeddedClassName: string
  /** The view's own header row content (title, controls). The close X is appended here in the modal path. */
  header: React.ReactNode
  /** Extra classes for the header row wrapper, per view. */
  headerClassName?: string
  children: React.ReactNode
}

/**
 * The pane-vs-modal frame shared by the inspector views that are still
 * opened as standalone modals elsewhere (Costs, Execution Logs). The opener
 * decides which frame it gets; the view supplies only its header and body.
 */
export default function InspectorShell({
  embedded,
  isOpen,
  onClose,
  modalClassName,
  embeddedClassName,
  header,
  headerClassName = '',
  children,
}: InspectorShellProps) {
  if (!embedded && !isOpen) return null

  const shell = (
    <div className={embedded ? embeddedClassName : modalClassName}>
      <div className={headerClassName}>
        {header}
        {!embedded && (
          <button
            onClick={onClose}
            className="p-2 rounded-full hover:bg-accent hover:text-accent-foreground transition-colors ml-4"
          >
            <X className="w-5 h-5 text-muted-foreground" />
          </button>
        )}
      </div>

      {children}

      {!embedded && (
        <div className="px-6 py-4 border-t border-border flex justify-end bg-background rounded-b-lg">
          <button
            onClick={onClose}
            className="px-4 py-2 bg-secondary text-secondary-foreground rounded-md hover:bg-secondary/80 transition-colors text-sm font-medium"
          >
            Close
          </button>
        </div>
      )}
    </div>
  )

  if (embedded) return shell

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/50 backdrop-blur-sm p-2 sm:p-4">
        {shell}
      </div>
    </ModalPortal>
  )
}
