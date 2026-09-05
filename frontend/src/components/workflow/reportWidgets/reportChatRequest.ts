import type { ReportHumanInputChatResult } from '../../../utils/reportHumanInputChat'
import type { ReportChatOptions, ReportChatReceipt } from './reportEmbedContext'

type Dispatch = (request: { workspacePath: string; message: string; newChat: boolean }) => Promise<ReportHumanInputChatResult>
type PendingRequest = {
  message: string
  requestId?: string
  sending: boolean
  error?: string
  result: Promise<ReportChatReceipt>
  resolve: (receipt: ReportChatReceipt) => void
  reject: (error: unknown) => void
}

/** One host-reviewed request at a time. This is a UI guard, not an iframe
 * isolation boundary or a backend execution/idempotency guarantee. */
export class ReportChatRequestController {
  private pending: PendingRequest | null = null
  private active = true
  private listeners = new Set<() => void>()
  private receipts = new Map<string, { message: string; receipt: ReportChatReceipt }>()

  readonly workspacePath: string
  private dispatch: Dispatch

  constructor(workspacePath: string, dispatch: Dispatch) {
    this.workspacePath = workspacePath
    this.dispatch = dispatch
  }

  subscribe = (listener: () => void) => {
    this.listeners.add(listener)
    return () => { this.listeners.delete(listener) }
  }

  getSnapshot = () => this.pending

  private publish(pending: PendingRequest | null) {
    this.pending = pending
    this.listeners.forEach(listener => listener())
  }

  activate = () => { this.active = true }

  dispose = () => {
    this.active = false
    this.cancel()
  }

  request = (message: string, options?: ReportChatOptions): Promise<ReportChatReceipt> => {
    if (!this.active) return Promise.reject(new Error('This report is no longer open.'))
    if (typeof message !== 'string' || !message.trim() || message.length > 12000) {
      return Promise.reject(new Error('Provide a message between 1 and 12,000 characters.'))
    }
    if (options != null && (typeof options !== 'object' || Array.isArray(options))) {
      return Promise.reject(new Error('Chat options must be an object.'))
    }
    const requestId = options?.requestId
    if (requestId !== undefined && (typeof requestId !== 'string' || !requestId.trim() || requestId.length > 200)) {
      return Promise.reject(new Error('requestId must be a non-empty string of at most 200 characters.'))
    }
    message = message.trim()
    if (requestId) {
      const previous = this.receipts.get(requestId)
      if (previous) return previous.message === message
        ? Promise.resolve(previous.receipt)
        : Promise.reject(new Error('This requestId was already used for a different message. Use a new item/version/action ID.'))
    }
    if (this.pending) {
      if (this.pending.message === message && this.pending.requestId === requestId) return this.pending.result
      return Promise.reject(new Error('Finish or cancel the open report message before starting another.'))
    }
    let resolve!: PendingRequest['resolve']
    let reject!: PendingRequest['reject']
    const result = new Promise<ReportChatReceipt>((done, fail) => { resolve = done; reject = fail })
    this.publish({ message, requestId, sending: false, result, resolve, reject })
    return result
  }

  cancel = () => {
    if (!this.pending || this.pending.sending) return
    this.pending.resolve({ status: 'cancelled' })
    this.publish(null)
  }

  send = async (message: string, newChat: boolean) => {
    const request = this.pending
    if (!this.active || !request || request.sending) return
    if (!message.trim() || message.length > 12000) {
      this.publish({ ...request, error: 'Provide a message between 1 and 12,000 characters.' })
      return
    }
    this.publish({ ...request, sending: true, error: undefined })
    try {
      const result = await this.dispatch({
        workspacePath: this.workspacePath,
        message: `From the report for ${this.workspacePath}:\n\n${message.trim()}`,
        newChat,
      })
      const receipt: ReportChatReceipt = { status: 'queued', ...result }
      if (request.requestId) {
        this.receipts.set(request.requestId, { message: request.message, receipt })
        // Bound the per-view cache. Durable exactly-once execution belongs to
        // the approval consumer, which must re-read current DB state.
        if (this.receipts.size > 100) this.receipts.delete(this.receipts.keys().next().value!)
      }
      request.resolve(receipt)
      this.publish(null)
    } catch (error) {
      if (!this.active) {
        request.reject(error)
        this.publish(null)
        return
      }
      this.publish({ ...request, sending: false, error: error instanceof Error ? error.message : 'Could not queue the message. Try again.' })
    }
  }
}
