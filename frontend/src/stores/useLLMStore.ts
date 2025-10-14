import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { devtools } from 'zustand/middleware'
import type { LLMConfiguration } from '../services/api-types'
import type { LLMOption, StoreActions } from './types'
import { getAllAvailableLLMs, getAvailableModels } from '../utils/llmConfig'

interface LLMState extends StoreActions {
  // Primary LLM configuration (unified from sidebar and chat input)
  primaryConfig: LLMConfiguration
  
  // Available LLMs for selection
  availableLLMs: LLMOption[]
  
  // Loading and error states
  isLoadingLLMs: boolean
  error: string | null
  
  // Actions
  setPrimaryConfig: (config: LLMConfiguration) => void
  updateProvider: (provider: 'openrouter' | 'bedrock') => void
  updateModel: (modelId: string) => void
  updateFallbacks: (fallbacks: string[]) => void
  updateCrossProviderFallback: (fallback: LLMConfiguration['cross_provider_fallback']) => void
  refreshAvailableLLMs: () => Promise<void>
  
  // Helper methods
  getCurrentLLMOption: () => LLMOption | null
  isConfigValid: () => boolean
}

export const useLLMStore = create<LLMState>()(
  devtools(
    persist(
      (set, get) => ({
        // Initial state
        primaryConfig: {
          provider: 'openrouter',
          model_id: 'x-ai/grok-code-fast-1',
          fallback_models: ['z-ai/glm-4.5', 'openai/gpt-4o-mini'],
          cross_provider_fallback: {
            provider: 'openai',
            models: ['gpt-4o-mini']
          }
        },
        availableLLMs: [],
        isLoadingLLMs: false,
        error: null,

        // Actions
        setPrimaryConfig: (config) => {
          set({ primaryConfig: config, error: null })
        },

        updateProvider: (provider) => {
          const state = get()
          const availableModels = getAvailableModels(provider)
          
          // Set appropriate fallback models based on provider
          let fallbackModels: string[] = []
          let crossProviderFallback: LLMConfiguration['cross_provider_fallback']
          
          if (provider === 'openrouter') {
            fallbackModels = ['z-ai/glm-4.5', 'openai/gpt-4o-mini']
            crossProviderFallback = {
              provider: 'openai',
              models: ['gpt-4o-mini']
            }
          } else if (provider === 'bedrock') {
            fallbackModels = [
              'us.anthropic.claude-sonnet-4-20250514-v1:0',
              'us.anthropic.claude-3-7-sonnet-20250219-v1:0'
            ]
            crossProviderFallback = {
              provider: 'openrouter',
              models: ['x-ai/grok-code-fast-1', 'openai/gpt-4o-mini']
            }
          }

          set({
            primaryConfig: {
              ...state.primaryConfig,
              provider,
              model_id: availableModels[0] || '',
              fallback_models: fallbackModels,
              cross_provider_fallback: crossProviderFallback
            },
            error: null
          })
        },

        updateModel: (modelId) => {
          set((state) => ({
            primaryConfig: {
              ...state.primaryConfig,
              model_id: modelId
            },
            error: null
          }))
        },

        updateFallbacks: (fallbacks) => {
          set((state) => ({
            primaryConfig: {
              ...state.primaryConfig,
              fallback_models: fallbacks
            },
            error: null
          }))
        },

        updateCrossProviderFallback: (fallback) => {
          set((state) => ({
            primaryConfig: {
              ...state.primaryConfig,
              cross_provider_fallback: fallback
            },
            error: null
          }))
        },

        refreshAvailableLLMs: async () => {
          set({ isLoadingLLMs: true, error: null })
          
          try {
            const availableLLMs = getAllAvailableLLMs()
            set({ availableLLMs, isLoadingLLMs: false })
          } catch (error) {
            set({ 
              error: error instanceof Error ? error.message : 'Failed to load LLMs',
              isLoadingLLMs: false 
            })
          }
        },

        getCurrentLLMOption: () => {
          const state = get()
          const currentConfig = state.primaryConfig
          
          return {
            provider: currentConfig.provider,
            model: currentConfig.model_id,
            label: `${currentConfig.provider} - ${currentConfig.model_id}`,
            description: 'Current LLM configuration'
          }
        },

        isConfigValid: () => {
          const state = get()
          return !!(state.primaryConfig.provider && state.primaryConfig.model_id)
        },

        // Generic actions
        reset: () => {
          set({
            primaryConfig: {
              provider: 'openrouter',
              model_id: 'x-ai/grok-code-fast-1',
              fallback_models: ['z-ai/glm-4.5', 'openai/gpt-4o-mini'],
              cross_provider_fallback: {
                provider: 'openai',
                models: ['gpt-4o-mini']
              }
            },
            availableLLMs: [],
            isLoadingLLMs: false,
            error: null
          })
        },

        setLoading: (loading) => {
          set({ isLoadingLLMs: loading })
        },

        setError: (error) => {
          set({ error })
        }
      }),
      {
        name: 'llm-store',
        partialize: (state) => ({
          // Only persist the primary config, not loading states
          primaryConfig: state.primaryConfig
        })
      }
    ),
    {
      name: 'llm-store'
    }
  )
)
