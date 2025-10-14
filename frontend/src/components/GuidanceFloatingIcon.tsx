import { useState } from 'react'
import { agentApi } from '../services/api'

interface GuidanceFloatingIconProps {
  sessionId: string | null
  onGuidanceChange: (guidance: string) => void
  onAddToast: (message: string, type: 'success' | 'info') => void
}

const GuidanceFloatingIcon: React.FC<GuidanceFloatingIconProps> = ({ 
  sessionId, 
  onGuidanceChange,
  onAddToast
}) => {
  const [isSubmitting, setIsSubmitting] = useState(false)

  const handleGuidanceClick = async () => {
    if (!sessionId) return
    
    setIsSubmitting(true)
    try {
      const hardcodedMessage = "call human_feedback tool next"
      const response = await agentApi.setLLMGuidance(sessionId, hardcodedMessage)
      
      if (response.status === 'success') {
        onGuidanceChange(hardcodedMessage)
        onAddToast('LLM should call ask for your feedback next', 'success')
      } else {
        console.error('Failed to set guidance:', response.message)
        onAddToast('Failed to send guidance to LLM', 'info')
      }
    } catch (error) {
      console.error('Error setting guidance:', error)
      onAddToast('Error sending guidance to LLM', 'info')
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <div className="fixed bottom-20 right-4 z-50">
      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-lg border border-gray-200 dark:border-gray-700 w-12 h-12 p-0">
        <button
          onClick={handleGuidanceClick}
          disabled={isSubmitting}
          className="w-full h-full flex items-center justify-center bg-blue-600 hover:bg-blue-700 disabled:bg-gray-400 text-white rounded-lg transition-colors duration-200"
          title="Send human feedback request to LLM"
        >
          {isSubmitting ? (
            <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin"></div>
          ) : (
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
            </svg>
          )}
        </button>
      </div>
    </div>
  )
}

export default GuidanceFloatingIcon