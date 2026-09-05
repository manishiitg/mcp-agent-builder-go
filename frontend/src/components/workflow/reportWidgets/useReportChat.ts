import { useEffect, useMemo } from 'react'
import { sendWorkflowMessageToChat } from '../../../utils/reportHumanInputChat'
import { ReportChatRequestController } from './reportChatRequest'

export function useReportChat(workspacePath: string) {
  const controller = useMemo(() => new ReportChatRequestController(workspacePath, sendWorkflowMessageToChat), [workspacePath])
  useEffect(() => {
    controller.activate()
    return controller.dispose
  }, [controller])
  return controller
}
