import type { PollingEvent } from '../services/api-types'

// Pure event-ownership derivation, extracted from the (now-removed)
// EventHierarchy.tsx. Kept as the ONE canonical way to map an event to the
// terminal(s) that own it — an owner id can live in any of ~20 different
// fields depending on event type and shape (execution_id, delegation_id,
// background_agent_id, agent_id, several metadata variants, workflow-step
// composite ids, ...), so callers must not hand-roll a narrower subset.
//
// Free of React/component imports on purpose: EventDispatcher.tsx (a React
// component with store/API side effects at module scope) re-exports these
// for existing importers, but anything that only needs the pure ownership
// logic — like the terminal transcript's event scoping, and this file's own
// unit tests — should import from HERE, not from EventDispatcher, so it
// never pulls in a component's runtime dependencies.

// asRecord / getTerminalOwnerPayload used to live in the (now-removed)
// EventHierarchy.tsx. Kept here, exported, so getOwnedTerminalOwnerKeys has
// exactly one canonical way to extract an event's owner-bearing payload —
// callers must not hand-roll their own subset of this, since the candidate
// list below intentionally covers ids event data nests under several
// different shapes (data.data, data.data.fields, top-level metadata, ...).
function asRecord(value: unknown): Record<string, unknown> | undefined {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : undefined
}

export function getTerminalOwnerPayload(event: PollingEvent): Record<string, unknown> | undefined {
  const data = asRecord(event.data)
  const payload = asRecord(data?.data) || data
  return asRecord(payload?.fields) || payload
}

// Shared with EventHierarchy; keeping it beside the dispatcher avoids duplicating
// the event-owner normalization rules.
// eslint-disable-next-line react-refresh/only-export-components
export function getOwnedTerminalOwnerKeys(event: PollingEvent, payload?: Record<string, unknown>): string[] {
  const sessionId = event.session_id?.trim()
  if (!sessionId) return []

  const normalizeOwnerId = (value: unknown): string | null => {
    if (typeof value !== 'string') return null
    const trimmed = value.trim()
    if (!trimmed || trimmed.startsWith('main:')) return null
    for (const prefix of ['delegation:', 'workflow:', 'background:', 'agent:', 'batch:']) {
      if (trimmed.startsWith(prefix)) {
        const unprefixed = trimmed.slice(prefix.length).trim()
        return unprefixed && !unprefixed.startsWith('main:') ? unprefixed : null
      }
    }
    return trimmed
  }

  const addOwnerId = (keys: string[], seen: Set<string>, value: unknown) => {
    const ownerId = normalizeOwnerId(value)
    if (!ownerId) return
    const values = [ownerId]
    const workflowStepMatch = ownerId.match(/^workflow-step:(.+):([^:]+)$/)
    if (workflowStepMatch?.[2]) values.push(workflowStepMatch[2])
    const subExecMatch = ownerId.match(/^sub-exec-(.+)-\d+$/)
    if (subExecMatch?.[1]) values.push(subExecMatch[1])
    const todoSubMatch = ownerId.match(/^todo-sub-.+-(.+)$/)
    if (todoSubMatch?.[1]) values.push(todoSubMatch[1])
    const stepSubMatch = ownerId.match(/^step-\d+-sub-(.+?)-execution(?:-|$)/)
    if (stepSubMatch?.[1]) values.push(stepSubMatch[1])
    for (const id of values) {
      const key = `${sessionId}:${id}`
      if (!seen.has(key)) {
        seen.add(key)
        keys.push(key)
      }
    }
  }

  const eventRecord = event as unknown as Record<string, unknown>
  const data = event.data && typeof event.data === 'object'
    ? event.data as Record<string, unknown>
    : undefined
  const innerData = data?.data && typeof data.data === 'object'
    ? data.data as Record<string, unknown>
    : undefined
  const metadata = payload?.metadata && typeof payload.metadata === 'object'
    ? payload.metadata as Record<string, unknown>
    : innerData?.metadata && typeof innerData.metadata === 'object'
      ? innerData.metadata as Record<string, unknown>
      : data?.metadata && typeof data.metadata === 'object'
        ? data.metadata as Record<string, unknown>
        : undefined

  const candidates = [
    eventRecord.execution_id,
    metadata?.execution_id,
    payload?.execution_id,
    innerData?.execution_id,
    metadata?.owner_execution_id,
    metadata?.execution_owner_id,
    payload?.delegation_id,
    payload?.background_agent_id,
    metadata?.background_agent_id,
    payload?.agent_id,
    metadata?.agent_id,
    payload?.agent_name,
    metadata?.orchestrator_agent_name,
    payload?.correlation_id,
    innerData?.delegation_id,
    innerData?.background_agent_id,
    innerData?.agent_id,
    innerData?.correlation_id,
    data?.delegation_id,
    data?.background_agent_id,
    data?.agent_id,
    data?.execution_id,
    data?.correlation_id,
    metadata?.delegation_id,
    metadata?.execution_id,
    metadata?.workshop_step_id,
    metadata?.current_step_id,
    metadata?.orchestrator_step_id,
    metadata?.workflow_step_id,
    metadata?.step_id,
    eventRecord.correlation_id,
  ]

  const keys: string[] = []
  const seen = new Set<string>()
  for (const candidate of candidates) {
    addOwnerId(keys, seen, candidate)
  }

  // A delegation spawned by a background agent (the delegate tool's async
  // path) keeps a DISTINCT execution_id from its background agent by design
  // (backend: internal/events/event_store.go ensureExecutionOwnership) —
  // it's a real child execution, not the same one. But nothing else ever
  // stamps its own id on the delegation's forwarded content events (tool
  // calls, messages) — DelegationEventObserver tags them only with
  // correlation_id — so without this they match no terminal at all and the
  // clean transcript falls back to raw content. parent_execution_id links it
  // back to the background agent's terminal, which IS where a reader expects
  // to see this content.
  //
  // Scoped to events whose correlation_id has the delegate tool's own
  // "delegation-<depth>-<ts>" shape (the same signal the backend uses to
  // recognize these events, event_store.go's `strings.HasPrefix(delegationID,
  // "delegation-")`) rather than a bare parent_execution_id candidate — a
  // general one would also pull todo_task-dispatched sub-agent content into
  // its ORCHESTRATOR's own terminal, duplicating it across two panes.
  const correlationID = typeof eventRecord.correlation_id === 'string' ? eventRecord.correlation_id : ''
  if (correlationID.startsWith('delegation-')) {
    addOwnerId(keys, seen, eventRecord.parent_execution_id)
  }

  return keys
}
