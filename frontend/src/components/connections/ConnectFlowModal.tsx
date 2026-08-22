import { useEffect, useState } from 'react'
import { X, Loader2, CheckCircle2, ExternalLink } from 'lucide-react'
import ModalPortal from '../ui/ModalPortal'
import ConnectionIcon from './ConnectionIcon'
import ErrorNotice from './ErrorNotice'
import { useConnectionsStore } from '../../stores/useConnectionsStore'
import type { CatalogEntry, FriendlyError } from '../../services/connectionsApi'

type Step = 'review' | 'authenticating' | 'done' | 'error'

interface ConnectFlowModalProps {
  entry: CatalogEntry
  onClose: () => void
}

/**
 * The guided connect flow: review the access being granted, authenticate, and
 * land on a success state — without ever showing the user an MCP server or a
 * JSON file.
 */
export default function ConnectFlowModal({ entry, onClose }: ConnectFlowModalProps) {
  const connect = useConnectionsStore(s => s.connect)

  const [step, setStep] = useState<Step>('review')
  const [error, setError] = useState<FriendlyError | null>(null)

  // Only used by the needs_client_id fallback, when a server turns out not to
  // support dynamic client registration after all.
  const [clientId, setClientId] = useState('')
  const [needsClientId, setNeedsClientId] = useState(false)

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      // Escape must not abandon a half-finished authentication silently.
      if (e.key === 'Escape' && step !== 'authenticating') {
        onClose()
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [step, onClose])

  const canSubmit = !needsClientId || clientId.trim().length > 0

  const handleConnect = async () => {
    setStep('authenticating')
    setError(null)

    const outcome = await connect(entry.id, {
      client_id: needsClientId ? clientId.trim() : undefined,
    })

    if (outcome.kind === 'needs_client_id') {
      // The provider has no dynamic client registration — collect the id once.
      setNeedsClientId(true)
      setStep('review')
      return
    }

    if (outcome.kind === 'failed') {
      setError(outcome.error)
      setStep('error')
      return
    }

    setStep('done')
  }

  const title =
    step === 'done' ? `${entry.name} connected` : `Connect ${entry.name}`

  return (
    <ModalPortal>
      <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 p-4">
        <div
          role="dialog"
          aria-modal="true"
          aria-label={title}
          className="w-full max-w-lg overflow-hidden rounded-lg bg-white shadow-2xl dark:bg-slate-800"
        >
          {/* Header */}
          <div className="flex items-start justify-between gap-3 border-b border-gray-200 px-5 py-4 dark:border-slate-700">
            <div className="flex min-w-0 items-center gap-3">
              <ConnectionIcon icon={entry.icon} />
              <div className="min-w-0">
                <h2 className="truncate text-base font-semibold text-gray-900 dark:text-gray-100">
                  {title}
                </h2>
                {entry.tagline && (
                  <p className="truncate text-xs text-gray-500 dark:text-gray-400">
                    {entry.tagline}
                  </p>
                )}
              </div>
            </div>
            <button
              type="button"
              onClick={onClose}
              disabled={step === 'authenticating'}
              className="rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 disabled:opacity-40 dark:text-gray-400 dark:hover:bg-slate-700"
              aria-label="Close"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          <div className="max-h-[70vh] space-y-4 overflow-y-auto px-5 py-4">
            {step === 'review' && (
              <>
                <p className="text-sm text-gray-600 dark:text-gray-300">
                  Connecting opens {entry.name} in a new window so you can approve
                  access. Your agents can then use it on your behalf.
                </p>

                {needsClientId && (
                  <section className="space-y-2">
                    <label
                      htmlFor="connection-client-id"
                      className="block text-sm font-medium text-gray-700 dark:text-gray-300"
                    >
                      OAuth client ID
                    </label>
                    <p className="text-xs text-gray-500 dark:text-gray-400">
                      {entry.name} does not register applications automatically. Paste the
                      client ID from your {entry.name} OAuth app.
                    </p>
                    <input
                      id="connection-client-id"
                      type="text"
                      value={clientId}
                      onChange={e => setClientId(e.target.value)}
                      autoComplete="off"
                      className="w-full rounded-md border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 focus:border-blue-500 focus:ring-2 focus:ring-blue-500 dark:border-slate-600 dark:bg-slate-700 dark:text-gray-100"
                    />
                  </section>
                )}

                {entry.docs_url && (
                  <a
                    href={entry.docs_url}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 text-xs text-blue-600 hover:underline dark:text-blue-400"
                  >
                    {entry.name} setup documentation
                    <ExternalLink className="h-3 w-3" aria-hidden="true" />
                  </a>
                )}
              </>
            )}

            {step === 'authenticating' && (
              <div className="flex flex-col items-center gap-3 py-8 text-center">
                <Loader2 className="h-7 w-7 animate-spin text-blue-600 dark:text-blue-400" />
                <p className="text-sm font-medium text-gray-900 dark:text-gray-100">
                  Waiting for you to approve access
                </p>
                <p className="max-w-xs text-xs text-gray-500 dark:text-gray-400">
                  A {entry.name} sign-in window opened. Approve the requested access
                  there — the window closes itself and you land back here.
                </p>
              </div>
            )}

            {step === 'done' && (
              <div className="flex flex-col items-center gap-2 py-6 text-center">
                <CheckCircle2 className="h-10 w-10 text-green-600 dark:text-green-400" />
                <p className="text-sm font-medium text-gray-900 dark:text-gray-100">
                  {entry.name} is connected
                </p>
                <p className="text-xs text-gray-500 dark:text-gray-400">
                  Its actions are now available to your agents.
                </p>
              </div>
            )}

            {step === 'error' && error && (
              <ErrorNotice
                error={error}
                onAction={action => {
                  if (action === 'retry' || action === 'reconnect') {
                    setStep('review')
                    setError(null)
                  }
                }}
              />
            )}
          </div>

          {/* Footer */}
          <div className="flex items-center justify-end gap-2 border-t border-gray-200 px-5 py-3 dark:border-slate-700">
            {step === 'done' ? (
              <button
                type="button"
                onClick={onClose}
                className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700"
              >
                Done
              </button>
            ) : (
              <>
                <button
                  type="button"
                  onClick={onClose}
                  disabled={step === 'authenticating'}
                  className="rounded-md px-4 py-2 text-sm text-gray-600 transition-colors hover:text-gray-900 disabled:opacity-40 dark:text-gray-400 dark:hover:text-gray-200"
                >
                  Cancel
                </button>
                {(step === 'review' || step === 'error') && (
                  <button
                    type="button"
                    onClick={handleConnect}
                    disabled={!canSubmit}
                    className="rounded-md bg-blue-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-blue-700 disabled:cursor-not-allowed disabled:opacity-50"
                  >
                    {step === 'error' ? 'Try again' : `Connect ${entry.name}`}
                  </button>
                )}
              </>
            )}
          </div>
        </div>
      </div>
    </ModalPortal>
  )
}
