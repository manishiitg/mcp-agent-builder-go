import { memo, useEffect } from 'react'
import {
  BaseEdge,
  EdgeLabelRenderer,
  getSmoothStepPath,
  Position,
  type EdgeProps
} from '@xyflow/react'

interface RoutingEdgeData extends Record<string, unknown> {
  onTraceRoute?: () => void
  traced?: boolean
  routeIndex?: number
  routeCount?: number
  routeName?: string
  selected?: boolean
  color?: string
  isLateralHandoff?: boolean
}

function getRouteLabelPosition(
  sourceX: number,
  sourceY: number,
  targetX: number,
  targetY: number,
  isLateralHandoff = false
) {
  const deltaX = targetX - sourceX
  const deltaY = targetY - sourceY
  if (isLateralHandoff) {
    return {
      x: sourceX + deltaX * 0.5,
      y: sourceY + deltaY * 0.5,
    }
  }
  const labelOffsetY = Math.min(96, Math.max(44, Math.abs(deltaY) * 0.16))

  return {
    x: sourceX + deltaX * 0.18,
    y: sourceY + (deltaY >= 0 ? labelOffsetY : -labelOffsetY)
  }
}

export const RoutingEdge = memo(({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition = Position.Bottom,
  targetPosition = Position.Top,
  style = {},
  markerEnd,
  label,
  data
}: EdgeProps) => {
  const edgeData = (data ?? {}) as RoutingEdgeData
  const color = edgeData.color || '#0f766e'
  const selectedOpacity = style.opacity ?? (edgeData.selected === false ? 0.45 : 1)
  const labelText = typeof label === 'string'
    ? label
    : edgeData.routeName
  const routeNumber = typeof edgeData.routeIndex === 'number'
    ? edgeData.routeIndex + 1
    : null

  useEffect(() => {
    console.log('[WorkflowCanvas] RoutingEdge rendered', {
      id,
      label: labelText,
      source: { x: Math.round(sourceX), y: Math.round(sourceY) },
      target: { x: Math.round(targetX), y: Math.round(targetY) },
      color,
      routeIndex: edgeData.routeIndex,
      routeCount: edgeData.routeCount,
      selected: edgeData.selected
    })
  }, [color, edgeData.routeCount, edgeData.routeIndex, edgeData.selected, id, labelText, sourceX, sourceY, targetX, targetY])

  const [edgePath] = getSmoothStepPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
    // Fan out above route bodies instead of carrying parallel wires through
    // the middle of other routes. Side handoffs keep their short lateral path.
    centerY: !edgeData.isLateralHandoff && targetY > sourceY
      ? sourceY + 48 + (edgeData.routeIndex || 0) * 24
      : undefined,
    borderRadius: 18,
    offset: 32
  })
  const labelPosition = getRouteLabelPosition(
    sourceX,
    sourceY,
    targetX,
    targetY,
    Boolean(edgeData.isLateralHandoff),
  )

  return (
    <>
      <BaseEdge
        id={id}
        path={edgePath}
        markerEnd={markerEnd}
        interactionWidth={24}
        style={{
          ...style,
          stroke: color,
          strokeWidth: style.strokeWidth ?? 2.5,
          opacity: selectedOpacity
        }}
      />

      {routeNumber !== null && (
        <EdgeLabelRenderer>
          <button
            type="button"
            disabled={!edgeData.onTraceRoute}
            onPointerDown={event => event.stopPropagation()}
            onClick={event => {
              event.stopPropagation()
              edgeData.onTraceRoute?.()
            }}
            aria-pressed={Boolean(edgeData.traced)}
            className="nodrag nopan pointer-events-auto absolute z-10 flex h-5 w-5 items-center justify-center rounded-full border text-[9px] font-semibold text-white shadow-sm enabled:cursor-pointer focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-teal-400"
            style={{
              transform: `translate(-50%, -50%) translate(${labelPosition.x}px, ${labelPosition.y}px)`,
              borderColor: color,
              background: color,
              opacity: selectedOpacity
            }}
            title={labelText ? `Trace route: ${labelText}` : `Trace route ${routeNumber}`}
            aria-label={labelText ? `Trace route ${routeNumber}: ${labelText}` : `Trace route ${routeNumber}`}
          >
            {routeNumber}
          </button>
        </EdgeLabelRenderer>
      )}
    </>
  )
})

RoutingEdge.displayName = 'RoutingEdge'
