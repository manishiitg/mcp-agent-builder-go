import type { ExecutionEvent } from './types'
import './execution-events.css'

const TOOL_EVENT_TYPES = new Set(['tool_started', 'tool_completed', 'tool_failed'])

function eventRow(event: ExecutionEvent) {
  return <div className={`execution-activity-event is-${event.status || 'update'}`} key={event.id}>
    <i />
    <span><strong>{event.name}</strong><small>{event.status || event.type}</small></span>
  </div>
}

// Presentation-light by design: names and statuses are emitted by the runtime,
// not translated into product-specific stages or inferred percentages here.
export function ExecutionActivityFeed({ events, limit = 16 }: { events: ExecutionEvent[]; limit?: number }) {
  if (!events.length) return null
  const visibleEvents = events.slice(-limit)
  const toolEvents = visibleEvents.filter((event) => TOOL_EVENT_TYPES.has(event.type))
  const activityEvents = visibleEvents.filter((event) => !TOOL_EVENT_TYPES.has(event.type))
  const identifiedCalls = new Set(toolEvents.flatMap((event) => event.executionId ? [event.executionId] : []))
  const unidentifiedStarts = toolEvents.filter((event) => event.type === 'tool_started' && !event.executionId).length
  const toolCallCount = identifiedCalls.size + unidentifiedStarts || toolEvents.length
  const terminalCallIDs = new Set(toolEvents.flatMap((event) => event.type !== 'tool_started' && event.executionId ? [event.executionId] : []))
  const hasRunningTool = toolEvents.some((event) => event.type === 'tool_started' && (!event.executionId || !terminalCallIDs.has(event.executionId)))
  const toolStatus = toolEvents.some((event) => event.type === 'tool_failed') ? 'failed' : hasRunningTool ? 'running' : 'completed'

  return <section className="execution-activity-feed" aria-label="Agent activity">
    {toolEvents.length > 0 && <details className={`execution-tool-group is-${toolStatus}`}>
      <summary aria-label={`${toolCallCount} tool call${toolCallCount === 1 ? '' : 's'}`}>
        <i />
        <span><strong>{toolCallCount} tool call{toolCallCount === 1 ? '' : 's'}</strong>{toolStatus === 'running' && <small>running</small>}</span>
      </summary>
      <div className="execution-tool-details">{toolEvents.map(eventRow)}</div>
    </details>}
    {activityEvents.map(eventRow)}
  </section>
}
