import { useShallow } from 'zustand/react/shallow'
import { useChatStore } from '../../stores'
import { ToastContainer } from './Toast'

/**
 * Single always-mounted home for the app's toasts.
 *
 * ChatArea used to render the container, so a toast raised from the top bar
 * (connectors, for example) was written to the store and then silently dropped
 * on any surface that does not mount a chat. Mounting this at the app root
 * makes toasts work regardless of which surface is active.
 */
export default function ToastHost() {
  const { toasts, removeToast } = useChatStore(
    useShallow(state => ({ toasts: state.toasts, removeToast: state.removeToast }))
  )

  // The store also carries 'warning', which the container does not render.
  const visibleToasts = toasts.filter(
    (toast): toast is { id: string; message: string; type: 'success' | 'info' | 'error' } =>
      toast.type === 'success' || toast.type === 'info' || toast.type === 'error'
  )

  return <ToastContainer toasts={visibleToasts} onRemoveToast={removeToast} />
}
