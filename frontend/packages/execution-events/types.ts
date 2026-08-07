import contract from '../../../agent_go/internal/platformevents/contract.json'

export const CORE_EXECUTION_EVENT_TYPES = contract.coreTypes
export type CoreExecutionEventType = typeof CORE_EXECUTION_EVENT_TYPES[number]

export interface ExecutionEvent {
  id: string
  scopeId?: string
  type: CoreExecutionEventType | (string & {})
  name: string
  status?: string
  executionId?: string
  parentExecutionId?: string
  message?: string
  createdAt: string
}
