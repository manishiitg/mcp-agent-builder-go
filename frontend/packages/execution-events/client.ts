import type { ExecutionEvent } from './types'

export interface ExecutionEventsClient {
  list(scopeId: string): Promise<ExecutionEvent[]>
}

export interface ExecutionEventsClientOptions {
  baseURL: string
  routeForScope: (scopeId: string) => string
  credentials?: RequestCredentials
}

// The transport is shared while route ownership remains with the product.
// This lets a project, conversation, or workspace expose the same normalized
// contract without forcing all products into one URL hierarchy.
export function createExecutionEventsClient(options: ExecutionEventsClientOptions): ExecutionEventsClient {
  return {
    async list(scopeId) {
      const response = await fetch(`${options.baseURL}${options.routeForScope(scopeId)}`, {
        credentials: options.credentials ?? 'same-origin',
      })
      if (!response.ok) throw new Error(`Could not load execution events (${response.status})`)
      return response.json() as Promise<ExecutionEvent[]>
    },
  }
}
