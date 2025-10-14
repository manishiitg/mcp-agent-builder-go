// Export all stores for easy importing
export { useAppStore } from './useAppStore'
export { useLLMStore } from './useLLMStore'
export { useMCPStore } from './useMCPStore'
export { useChatStore } from './useChatStore'
export { useWorkspaceStore } from './useWorkspaceStore'

// Export types
export type * from './types'

// Store initialization helper
export const initializeStores = async () => {
  // Initialize MCP store by loading tools
  const { useMCPStore } = await import('./useMCPStore')
  await useMCPStore.getState().refreshTools()
  
  // Initialize LLM store by loading available LLMs
  const { useLLMStore } = await import('./useLLMStore')
  await useLLMStore.getState().refreshAvailableLLMs()
  
  // Initialize workspace store
  const { useWorkspaceStore } = await import('./useWorkspaceStore')
  useWorkspaceStore.getState()
}
