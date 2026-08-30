import { RoutingEdge } from './RoutingEdge'

export { RoutingEdge } from './RoutingEdge'

export const edgeTypes = {
  routing: RoutingEdge,
  // branch reuses RoutingEdge as-is, same reasoning as nodeTypes. See PLAT-259.
  branch: RoutingEdge
} as const
