// Keep routing cards, output handles, and connecting edges visually paired.
export const ROUTING_EDGE_COLORS = [
  '#0f766e',
  '#2563eb',
  '#7c3aed',
  '#ea580c',
  '#0891b2',
] as const

export function routeColorForIndex(index: number): string {
  return ROUTING_EDGE_COLORS[index % ROUTING_EDGE_COLORS.length]
}
