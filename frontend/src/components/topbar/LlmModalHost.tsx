import LLMConfigurationModal from '../LLMConfigurationModal'
import { useLlmOnboarding } from './useLlmOnboarding'

/**
 * Renders the shared model configuration modal and its first-run onboarding.
 * Mount exactly once in the global top bar.
 */
export default function LlmModalHost() {
  const {
    showLLMModal,
    closeLLMConfigurationModal,
  } = useLlmOnboarding()

  return (
    <>
      <LLMConfigurationModal
        isOpen={showLLMModal}
        onClose={closeLLMConfigurationModal}
      />
    </>
  )
}
