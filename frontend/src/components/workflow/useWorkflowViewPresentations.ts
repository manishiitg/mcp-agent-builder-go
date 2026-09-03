// The workflow agent can open a toolbar view for the user (the
// open_workspace_view tool emits a "workflow.view" presentation). This turns
// those events, for the chat tab on screen, into the same store call the
// toolbar buttons make. Events already present when the tab was opened are
// history and are not replayed.
import { useEffect, useRef } from 'react'
import { useChatStore } from '../../stores/useChatStore'
import { useWorkflowStore } from '../../stores/useWorkflowStore'
import { usePresentationEvents } from '../../platform/presentations/usePresentationEvents'
import { isWorkspaceViewId } from './workspaceViews'

export const WORKFLOW_VIEW_PRESENTATION_KIND = 'workflow.view'
const KINDS = [WORKFLOW_VIEW_PRESENTATION_KIND]

export function useWorkflowViewPresentations(tabId: string | null | undefined): void {
  const sessionId = useChatStore(state => (tabId ? state.chatTabs[tabId]?.sessionId : undefined))
  const openWorkspaceView = useWorkflowStore(state => state.openWorkspaceView)
  const presentations = usePresentationEvents(sessionId ?? undefined, KINDS)
  const handed = useRef<{ sessionId: string; count: number } | null>(null)
  useEffect(() => {
    if (!sessionId) return
    if (!handed.current || handed.current.sessionId !== sessionId) {
      handed.current = { sessionId, count: presentations.length }
      return
    }
    for (const p of presentations.slice(handed.current.count)) {
      const view = p.payload.view
      if (isWorkspaceViewId(view)) openWorkspaceView(view)
    }
    handed.current = { sessionId, count: presentations.length }
  }, [sessionId, presentations, openWorkspaceView])
}
