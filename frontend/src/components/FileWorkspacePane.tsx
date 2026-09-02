import { FileContentViewerBody } from './FileContentViewer'
import Workspace from './Workspace'
import { useWorkspaceStore } from '../stores/useWorkspaceStore'

type FileWorkspacePaneProps = {
  workspacePath?: string
  title?: string
  hiddenRootFolders?: string[]
  onClose: () => void
  testId?: string
}

/**
 * Shared Files surface for any product that exposes a workspace. It keeps the
 * tree mounted while a file is open, preserving navigation state, and makes
 * the file viewer occupy that product's right-side Files pane rather than
 * opening a competing full-screen overlay.
 */
export function FileWorkspacePane({
  workspacePath,
  title,
  hiddenRootFolders,
  onClose,
  testId,
}: FileWorkspacePaneProps) {
  const showFileContent = useWorkspaceStore(state => state.showFileContent)

  return (
    <div className="relative flex h-full min-h-0 flex-col bg-background" data-testid={testId}>
      <div className="min-h-0 flex-1" hidden={showFileContent}>
        <Workspace
          minimized={false}
          onToggleMinimize={onClose}
          hideMinimizeControl
          scopedWorkspacePath={workspacePath}
          hiddenRootFolders={hiddenRootFolders}
          title={title}
        />
      </div>
      {showFileContent && (
        <div className="min-h-0 flex-1">
          <FileContentViewerBody variant="pane" />
        </div>
      )}
    </div>
  )
}
