import { useCallback, useEffect, useRef, useState } from 'react'
import { RefreshCw, Terminal } from 'lucide-react'
import { agentApi } from '../services/api'
import type { TerminalSnapshot } from '../services/api-types'
import { Button } from './ui/Button'

type MainAgentTerminalProps = {
  sessionId: string
  runtimeLabel?: string
  isAgentWorking?: boolean
}

// The product-facing raw view intentionally knows about one terminal only: the
// main coding-agent pane for the active chat. It mounts (and starts polling)
// only after the user switches away from Formatted view.
export function MainAgentTerminal({ sessionId, runtimeLabel = '', isAgentWorking = false }: MainAgentTerminalProps) {
  const [snapshot, setSnapshot] = useState<TerminalSnapshot | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const requestInFlight = useRef(false)

  const refresh = useCallback(async () => {
    if (requestInFlight.current) return
    requestInFlight.current = true
    try {
      const next = await agentApi.getMainTerminal(sessionId, { content: 'history' })
      setSnapshot(next)
      setError(null)
    } catch (cause: any) {
      if (cause?.response?.status === 404) {
        setSnapshot(null)
        setError(null)
      } else {
        setError(cause?.message || 'Could not load the main terminal.')
      }
    } finally {
      setLoading(false)
      requestInFlight.current = false
    }
  }, [sessionId])

  useEffect(() => {
    setSnapshot(null)
    setError(null)
    setLoading(true)
    void refresh()
    const timer = window.setInterval(() => { void refresh() }, snapshot?.active ? 1200 : 3500)
    return () => window.clearInterval(timer)
  }, [refresh, snapshot?.active])

  return (
    <section className="flex min-h-0 flex-1 flex-col bg-[#0b0e0d] text-[#e7e9e5]">
      <header className="flex items-center justify-between border-b border-white/10 px-4 py-2.5">
        <div className="flex min-w-0 items-center gap-2 text-sm text-neutral-300">
          <Terminal className="h-4 w-4 text-primary" />
          <span className="font-medium">Main terminal</span>
          {(snapshot?.active || isAgentWorking) && <span className="h-2 w-2 animate-pulse rounded-full bg-lime-300" aria-label="Working" />}
          <span className="truncate text-xs text-neutral-500">{runtimeLabel || snapshot?.status?.provider_label || 'Waiting for main terminal'}</span>
        </div>
        <Button type="button" variant="ghost" size="sm" onClick={() => void refresh()} aria-label="Refresh main terminal" className="px-2 text-neutral-400 hover:text-white">
          <RefreshCw className={loading ? 'h-4 w-4 animate-spin' : 'h-4 w-4'} />
        </Button>
      </header>
      <div className="min-h-0 flex-1 overflow-auto p-4">
        {error ? (
          <p className="text-sm text-red-300">{error}</p>
        ) : !snapshot ? (
          <div className="flex h-full items-center justify-center text-sm text-neutral-500">
            {loading ? 'Loading main terminal…' : 'The main terminal is not available yet.'}
          </div>
        ) : (
          <pre className="m-0 min-h-full whitespace-pre-wrap break-words font-mono text-[13px] leading-5 text-neutral-200">{snapshot.content || 'The main terminal has not produced output yet.'}</pre>
        )}
      </div>
    </section>
  )
}
