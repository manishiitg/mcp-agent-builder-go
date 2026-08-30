import { memo, useEffect } from 'react'
import {
  BaseEdge,
  EdgeLabelRenderer,
  getSmoothStepPath,
  Position,
  type EdgeProps
} from '@xyflow/react'

interface RoutingEdgeData extends Record<string, unknown> {
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
  const selectedOpacity = edgeData.selected === false ? 0.45 : 1
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
        interactionWidth={18}
        style={{
          ...style,
          stroke: color,
          strokeWidth: style.strokeWidth ?? 2.5,
          opacity: selectedOpacity
        }}
      />

      {routeNumber !== null && (
        <EdgeLabelRenderer>
          <div
            className="nodrag nopan pointer-events-none absolute z-10 flex h-5 w-5 items-center justify-center rounded-full border text-[9px] font-semibold text-white shadow-sm"
            style={{
              transform: `translate(-50%, -50%) translate(${labelPosition.x}px, ${labelPosition.y}px)`,
              borderColor: color,
              background: color,
              opacity: selectedOpacity
            }}
            title={labelText}
            aria-label={labelText ? `Route ${routeNumber}: ${labelText}` : `Route ${routeNumber}`}
          >
            {routeNumber}
          </div>
        </EdgeLabelRenderer>
      )}
    </>
  )
})

RoutingEdge.displayName = 'RoutingEdge'
