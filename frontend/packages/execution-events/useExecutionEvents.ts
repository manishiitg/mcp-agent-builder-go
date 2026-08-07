import { useCallback, useEffect, useReducer } from 'react'
import type { ExecutionEventsClient } from './client'
import { executionEventsReducer } from './reducer'
import type { ExecutionEvent } from './types'

interface UseExecutionEventsOptions {
  client: ExecutionEventsClient
  scopeId: string
  initialEvents?: ExecutionEvent[]
  refreshIntervalMs?: number
  enabled?: boolean
  onError?: (error: unknown) => void
}

export function useExecutionEvents({ client, scopeId, initialEvents = [], refreshIntervalMs = 5_000, enabled = true, onError }: UseExecutionEventsOptions) {
  const [events, dispatch] = useReducer(executionEventsReducer, initialEvents)

  const refresh = useCallback(async () => {
    if (!enabled || !scopeId) return
    try {
      dispatch({ type: 'replace', events: await client.list(scopeId) })
    } catch (error) {
      onError?.(error)
    }
  }, [client, enabled, onError, scopeId])

  useEffect(() => {
    dispatch({ type: 'clear' })
    if (!enabled || !scopeId) return
    void refresh()
    const timer = window.setInterval(() => void refresh(), refreshIntervalMs)
    return () => window.clearInterval(timer)
  }, [enabled, refresh, refreshIntervalMs, scopeId])

  return { events, refresh, dispatch }
}
