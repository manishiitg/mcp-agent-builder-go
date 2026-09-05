import { useEffect, useId, useRef, useState, useSyncExternalStore } from 'react'
import ModalPortal from '../../ui/ModalPortal'
import { ReportChatRequestController } from './reportChatRequest'

export function ReportChatPanel({ controller }: { controller: ReportChatRequestController }) {
  const request = useSyncExternalStore(controller.subscribe, controller.getSnapshot)
  return request ? <ReportChatDialog key={controller.workspacePath} controller={controller} /> : null
}

function ReportChatDialog({ controller }: { controller: ReportChatRequestController }) {
  const request = useSyncExternalStore(controller.subscribe, controller.getSnapshot)!
  const [message, setMessage] = useState(request.message)
  const [newChat, setNewChat] = useState(false)
  const dialog = useRef<HTMLDialogElement>(null)
  const titleId = useId()
  const detailId = useId()
  useEffect(() => {
    const element = dialog.current
    element?.showModal()
    return () => element?.close()
  }, [])

  return <ModalPortal>
    <dialog ref={dialog} aria-labelledby={titleId} aria-describedby={detailId}
      onCancel={event => { event.preventDefault(); controller.cancel() }}
      className="m-auto w-[calc(100%-2rem)] max-w-xl rounded-xl border border-border bg-background p-5 text-foreground shadow-2xl backdrop:bg-black/60">
      <form onSubmit={event => {
        event.preventDefault()
        // Report-authored scripts must not submit the host UI with .click() or
        // dispatchEvent(). This does not replace proper iframe isolation.
        if (event.nativeEvent.isTrusted) void controller.send(message, newChat)
      }}>
        <h2 id={titleId} className="text-lg font-semibold">Send to agent</h2>
        <p className="mt-1 break-words text-sm text-muted-foreground">{controller.workspacePath}</p>
        <p id={detailId} className="mt-3 text-sm text-muted-foreground">Review the request before sending. The agent can act on this message.</p>
        <label className="mt-4 block text-sm font-medium">Message
          <textarea autoFocus value={message} onChange={event => setMessage(event.target.value)} disabled={request.sending}
            maxLength={12000} rows={7} className="mt-1 block w-full resize-y rounded-md border border-border bg-background p-3 text-sm font-normal" />
        </label>
        <label className="mt-4 flex items-center gap-2 text-sm">
          <input type="checkbox" checked={newChat} onChange={event => setNewChat(event.target.checked)} disabled={request.sending} />
          Start a new chat
        </label>
        <p className="mt-2 text-xs text-muted-foreground">{newChat
          ? 'A new chat will open for this automation.'
          : 'Uses an existing chat for this automation, or creates one if needed. If a turn is running, your message queues behind it.'}</p>
        {request.error && <p role="alert" className="mt-3 text-sm text-destructive">{request.error}</p>}
        <div className="mt-5 flex justify-end gap-2">
          <button type="button" onClick={controller.cancel} disabled={request.sending} className="rounded-md border border-border px-4 py-2 text-sm disabled:opacity-50">Cancel</button>
          <button type="submit" disabled={request.sending || !message.trim()} className="rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground disabled:opacity-50">{request.sending ? 'Queuing…' : 'Send to agent'}</button>
        </div>
      </form>
    </dialog>
  </ModalPortal>
}
